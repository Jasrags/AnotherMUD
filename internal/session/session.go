// Package session bridges the connection layer (internal/conn) to the
// command dispatcher (internal/command), the world (internal/world), and
// the M3 login + persistence layer.
//
// In M3 a session is still one connection ↔ one character, but logged-
// in characters are tracked in a Manager so autosave and shutdown can
// iterate them. The connection/session split proper lands in M4 per
// docs/specs/session-lifecycle.md.
package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Jasrags/AnotherMUD/internal/ansi"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Config is the per-server wiring the session loop needs.
type Config struct {
	World    *world.World
	Commands *command.Registry
	Players  *player.Store
	Login    login.Config

	// Items is the runtime entity store. Item instantiation, get/drop,
	// and inventory restoration all go through it. May be nil only in
	// tests that don't exercise inventory.
	Items *entities.Store
	// Placement is the room↔item index. Used by the session layer to
	// spawn inventory items into the world without a room and by
	// get/drop handlers to move items between rooms and contents.
	Placement *entities.Placement
	// Templates is the item-template registry used at login time to
	// respawn persisted inventory entries into fresh instances.
	Templates *item.Templates
	// Slots is the equipment-slot registry. Required by the equip
	// command handler to validate slot names and look up capacities.
	// May be nil in tests that don't exercise equipment.
	Slots *slot.Registry
	// Bus is the engine event bus, passed through command.Env to
	// inventory/equipment handlers so they can publish observable
	// events after successful mutations. May be nil in tests.
	Bus *eventbus.Bus

	// StartID is the fallback starting room when a character's saved
	// location is not present in the loaded world (e.g. a room was
	// removed from content between restarts).
	StartID world.RoomID

	// ColorEnabled is the per-session default for ANSI color output.
	ColorEnabled bool

	// Manager tracks logged-in sessions for autosave + shutdown sweeps.
	// Required.
	Manager *Manager

	// Clock is the time source for time-dependent session machinery
	// (flood protection refill, idle timeouts). Defaults to
	// clock.RealClock when nil so existing tests don't have to wire it.
	Clock clock.Clock

	// Flood is the per-session rate-limit policy. Zero value disables
	// flood protection (the test default). Production wires
	// DefaultFloodConfig.
	Flood FloodConfig

	// LinkDead is the link-dead survival policy (M4.4). When Enabled
	// is false, every disconnect immediately runs full teardown (the
	// M3 behavior). When true, an involuntary connection drop parks
	// the session for TimeoutSeconds so a returning login can
	// reattach. Zero value (Enabled=false) is the test default.
	LinkDead LinkDeadConfig
}

// Handler returns a server.Handler-compatible function that drives one
// connection through login and into the game loop.
func Handler(cfg Config) func(ctx context.Context, c conn.Connection) error {
	return func(ctx context.Context, c conn.Connection) error {
		return run(ctx, c, cfg)
	}
}

func run(ctx context.Context, c conn.Connection, cfg Config) error {
	loaded, err := login.Run(ctx, c, cfg.Login)
	if err != nil {
		if errors.Is(err, login.ErrAborted) {
			return nil
		}
		// ErrPasswordCap / ErrEmailCap are terminal but not actionable
		// for the server — log + close.
		logging.From(ctx).Info("login ended", slog.Any("err", err))
		return nil
	}

	ctx = logging.With(ctx,
		slog.String("player", loaded.Player.Name),
		slog.String("account_id", loaded.Account.ID))

	clk := cfg.Clock
	if clk == nil {
		clk = clock.RealClock{}
	}

	// Reconnect check: if a session for this player already exists,
	// branch on its phase. LinkDead → reattach this new connection.
	// Playing → prompt for takeover per session-lifecycle §8.
	if existing, ok := cfg.Manager.GetByPlayerID(loaded.Player.ID); ok {
		if existing.isLinkDead() {
			return reconnect(ctx, c, cfg, existing, clk)
		}
		if !promptTakeoverConfirm(ctx, c) {
			_ = writeLine(ctx, c, renderTakeoverDeclined())
			logging.From(ctx).Info("login: takeover declined",
				slog.String("player", loaded.Player.Name))
			return nil
		}
		liveSave, _, ok := performTakeover(ctx, cfg, existing)
		if !ok {
			// Lost the takeover claim to a concurrent login.
			_ = writeLine(ctx, c, renderTakeoverRaced())
			logging.From(ctx).Info("login: takeover lost race",
				slog.String("player", loaded.Player.Name))
			return nil
		}
		// Override the stale-from-disk save loaded by login.Run with the
		// live in-memory record from the displaced session. Without this
		// any post-autosave movement / mutation on the old actor would
		// be silently dropped (session-lifecycle §8: "same entity").
		// resolveStartRoom below picks up the live Location automatically.
		if liveSave != nil {
			loaded.Player = liveSave
		}
	}

	start, err := resolveStartRoom(cfg, loaded.Player.Location)
	if err != nil {
		return fmt.Errorf("session: resolve start room: %w", err)
	}

	floodCfg := cfg.Flood
	a := &connActor{
		id:           c.ID(),
		conn:         c,
		playerID:     loaded.Player.ID,
		accountID:    loaded.Account.ID,
		room:         start,
		colorEnabled: cfg.ColorEnabled,
		save:         loaded.Player,
		players:      cfg.Players,
		items:        cfg.Items,
		equipment:    make(map[string]entities.EntityID),
		stats:        stats.New(),
		flood:        newFloodGate(floodCfg, clk),
		floodCfg:     &floodCfg,
		clk:          clk,
		lastInputAt:  clk.Now(),
	}

	// Install the persisted stat block FIRST. respawnEquipment will then
	// rebind each entry's source key from the saved entity id to the
	// fresh one as it spawns the matching ItemInstance. Order matters:
	// the block must hold the old source keys at the moment respawn
	// runs so RebindSource has something to find.
	a.stats.Restore(loaded.Player.Stats)

	// Respawn persisted inventory into live ItemInstances. Done before
	// the actor enters the manager / starts taking input so a takeover
	// or autosave can't observe a partial inventory.
	if cfg.Items != nil && cfg.Templates != nil {
		respawnInventory(ctx, a, cfg.Items, cfg.Templates, loaded.Player.Inventory)
		respawnEquipment(ctx, a, cfg.Items, cfg.Templates, loaded.Player.Equipment)
	}

	// Keep the save's location in sync with the room we actually placed
	// the player in. Mark dirty so the corrected location is flushed
	// even if the player idles and disconnects before issuing any
	// movement command (covers the saved-room-removed fallback case).
	if a.save.Location != string(start.ID) {
		a.mu.Lock()
		a.save.Location = string(start.ID)
		a.markDirtyLocked()
		a.mu.Unlock()
	}

	cfg.Manager.Add(a)

	// Announce arrival to the start room (excluding self) so anyone
	// already there sees the new player materialize.
	cfg.Manager.SendToRoom(ctx, start.ID,
		fmt.Sprintf("%s has arrived.", a.Name()), a.PlayerID())

	if err := a.Write(ctx, command.RenderRoom(start)); err != nil {
		// Initial render failed: the connection is unusable. Full
		// teardown immediately — no point parking link-dead.
		fullTeardown(ctx, cfg, a)
		return fmt.Errorf("first render: %w", err)
	}

	exit := pump(ctx, c, cfg, a, clk)
	dispatchTeardown(ctx, cfg, a, exit, clk)
	return nil
}

// pumpExit is the reason the read loop unwound, used by the teardown
// dispatcher to choose between link-dead and full teardown.
type pumpExit int

const (
	// exitClientQuit: dispatcher returned ErrQuit (the player typed
	// "quit"). Always full teardown — the player meant to leave.
	exitClientQuit pumpExit = iota
	// exitConnGone: Read returned EOF / ErrClosed — the transport
	// went away. Eligible for link-dead if the actor's disconnecting
	// latch is not set (i.e. the server didn't initiate the close).
	exitConnGone
	// exitCtxCancel: context was cancelled — server is shutting down.
	// Always full teardown so SaveAll can commit and indices clear.
	exitCtxCancel
	// exitForced: server-initiated disconnect (flood threshold). The
	// actor's disconnecting latch is set before the loop returns;
	// teardown is unconditionally full.
	exitForced
)

// pump is the per-connection read → dispatch loop. Extracted from run
// so the reconnect path can re-enter it against the new connection.
// Returns the reason for exit; teardown is dispatched by the caller.
func pump(ctx context.Context, c conn.Connection, cfg Config, a *connActor, clk clock.Clock) pumpExit {
	for {
		line, err := c.Read(ctx)
		if err != nil {
			switch {
			case errors.Is(err, io.EOF), errors.Is(err, conn.ErrClosed):
				return exitConnGone
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return exitCtxCancel
			case errors.Is(err, conn.ErrLineTooLong):
				_ = a.Write(ctx, "Input too long; truncated.")
				continue
			default:
				logging.From(ctx).Warn("read error", slog.Any("err", err))
				return exitConnGone
			}
		}

		logging.From(ctx).Debug("input received", slog.String("line", sanitizeForLog(line)))

		// Refresh the idle bookkeeping before the flood gate so even
		// dropped input still counts as activity (a flooding client is
		// not "idle"; the flood gate alone decides how it's punished).
		a.noteInput(clk.Now())

		// Flood gate runs before dispatch. The warn write happens after
		// the gate returns so no caller can accidentally re-enter the
		// gate from inside the Write path.
		decision, warn := a.flood.Check()
		if warn {
			_ = a.Write(ctx, "Slow down.")
		}
		switch decision {
		case floodAllow:
			// proceed
		case floodDrop:
			continue
		case floodDisconnect:
			_ = a.Write(ctx, "Disconnected: command flooding.")
			logging.From(ctx).Warn("session: disconnect on flood threshold",
				slog.String("player", a.PlayerName()))
			a.mu.Lock()
			a.disconnecting = true
			a.mu.Unlock()
			return exitForced
		}

		env := command.Env{
			World:       cfg.World,
			Broadcaster: cfg.Manager,
			Items:       cfg.Items,
			Placement:   cfg.Placement,
			Slots:       cfg.Slots,
			Bus:         cfg.Bus,
		}
		if err := cfg.Commands.Dispatch(ctx, env, a, line); err != nil {
			if errors.Is(err, command.ErrQuit) {
				return exitClientQuit
			}
			logging.From(ctx).Warn("command handler error", slog.Any("err", err))
		}
	}
}

// dispatchTeardown picks the right cleanup path for the actor based on
// the exit reason, the disconnecting latch, and the link-dead policy.
//
// Routing:
//   - exitClientQuit / exitCtxCancel / exitForced → full teardown.
//   - exitConnGone with disconnecting set (idle sweep closed the
//     connection from underneath us) → full teardown.
//   - exitConnGone with LinkDead disabled → full teardown.
//   - exitConnGone with LinkDead enabled and not disconnecting → park
//     in LinkDead phase, broadcast "lost their connection", keep the
//     actor in every index except byConn so a reconnect can find it.
func dispatchTeardown(ctx context.Context, cfg Config, a *connActor, exit pumpExit, clk clock.Clock) {
	a.mu.Lock()
	disc := a.disconnecting
	taken := a.takenOver
	a.mu.Unlock()

	// §6.1 stale-event guard: a taken-over session's pump has unwound
	// after the new session displaced it. Every index entry the old
	// actor held has already been reassigned or cleared by
	// performTakeover. Running fullTeardown here would re-broadcast
	// "X has left" and delete byPlayerID / byRoom entries that now
	// belong to the replacement session.
	if taken {
		logging.From(ctx).Debug("teardown: skipped (taken over)",
			slog.String("player", a.PlayerName()))
		return
	}

	if exit == exitConnGone && !disc && cfg.LinkDead.Enabled {
		enterLinkDeadTeardown(ctx, cfg, a, clk)
		return
	}
	fullTeardown(ctx, cfg, a)
}

// fullTeardown runs the M3 unwind in the canonical order
// "broadcast → remove → persist". This matches LinkDeadCleanup so a
// future refactor that consolidates the two paths cannot accidentally
// transpose the order and introduce a use-after-remove. Safe on an
// actor still in any phase; the manager's Remove is itself idempotent.
//
// After persist, any item entities the actor was carrying are
// untracked so the entity store does not grow without bound across
// reconnects. They will be respawned on the next login from the
// freshly-saved Inventory list.
func fullTeardown(ctx context.Context, cfg Config, a *connActor) {
	room := a.Room()
	if room != nil {
		cfg.Manager.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s has left.", a.Name()), a.PlayerID())
	}
	cfg.Manager.Remove(a)
	if err := a.Persist(ctx); err != nil {
		logging.From(ctx).Warn("save on disconnect failed", slog.Any("err", err))
	}
	untrackInventory(ctx, cfg.Items, a)
}

// respawnInventory creates fresh ItemInstances for each persisted
// template id and attaches them to the actor. Missing templates are
// logged and skipped — the player loses that item on this load, but
// the rest of the inventory survives. Skipped slots are also dropped
// from the save's Inventory so a successful persist clears the dead
// reference.
func respawnInventory(ctx context.Context, a *connActor, store *entities.Store, tpls *item.Templates, saved []string) {
	if len(saved) == 0 {
		return
	}
	survivors := make([]string, 0, len(saved))
	for _, tid := range saved {
		tpl, err := tpls.Get(item.TemplateID(tid))
		if err != nil {
			logging.From(ctx).Warn("inventory: dropping unknown template",
				slog.String("template_id", tid),
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
			continue
		}
		inst, err := store.Spawn(tpl)
		if err != nil {
			logging.From(ctx).Warn("inventory: spawn failed",
				slog.String("template_id", tid),
				slog.Any("err", err))
			continue
		}
		a.mu.Lock()
		a.inventory = append(a.inventory, inst.ID())
		a.mu.Unlock()
		survivors = append(survivors, tid)
	}
	a.mu.Lock()
	// If respawn dropped any entries, the on-disk save is now ahead of
	// the runtime; flip dirty so the next persist trims it.
	if len(survivors) != len(saved) {
		a.save.Inventory = survivors
		a.markDirtyLocked()
	}
	a.mu.Unlock()
}

// respawnEquipment creates fresh ItemInstances for each persisted
// equipment entry and reattaches the persisted stat-block source keys
// from the saved entity ids to the freshly-minted ones (§3.5). Slot
// keys with unknown templates are dropped; the modifier set under
// their old source key stays orphaned in the block until the next
// Persist trims via syncStatsToSaveLocked (a.stats has no entry for
// the dropped slot — the orphan is in a.save.Stats, not a.stats).
// To keep the runtime block clean we also drop the matching modifier
// set when a template lookup fails.
//
// Entries with an empty Entity id (legacy v2-migrated saves, §3.5
// open question) install the item but skip the rebind — no source
// key exists to migrate, so the modifier set is effectively absent
// for that slot until the player re-equips.
func respawnEquipment(ctx context.Context, a *connActor, store *entities.Store, tpls *item.Templates, saved map[string]player.EquippedItem) {
	if len(saved) == 0 {
		return
	}
	// Iterate slot keys in deterministic order so logs (and any future
	// dependent-modifier semantics) don't churn across restarts.
	keys := make([]string, 0, len(saved))
	for k := range saved {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	type respawned struct {
		slot  string
		newID entities.EntityID
	}
	survivors := make([]respawned, 0, len(keys))
	dropped := make([]string, 0)

	for _, slotKey := range keys {
		entry := saved[slotKey]
		tpl, err := tpls.Get(item.TemplateID(entry.Template))
		if err != nil {
			logging.From(ctx).Warn("equipment: dropping unknown template",
				slog.String("slot_key", slotKey),
				slog.String("template_id", entry.Template),
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
			dropped = append(dropped, slotKey)
			continue
		}
		inst, err := store.Spawn(tpl)
		if err != nil {
			logging.From(ctx).Warn("equipment: spawn failed",
				slog.String("slot_key", slotKey),
				slog.String("template_id", entry.Template),
				slog.Any("err", err))
			dropped = append(dropped, slotKey)
			continue
		}
		survivors = append(survivors, respawned{slot: slotKey, newID: inst.ID()})

		if entry.Entity == "" {
			// Migrated-from-v2 entry: no old source key to rebind. The
			// modifier set is absent for this slot until re-equip.
			continue
		}
		oldSrc := entities.EquipmentSourceKey(entities.EntityID(entry.Entity))
		newSrc := entities.EquipmentSourceKey(inst.ID())
		if !a.stats.RebindSource(oldSrc, newSrc) {
			// Either the persisted stat block had no entry for the old
			// id (item was equipped but contributed no modifiers — fine)
			// or the new source key collided with an existing entry
			// (programming error — log and move on; the slot is still
			// equipped, just without modifier reattachment).
			logging.From(ctx).Debug("equipment: source-key rebind no-op",
				slog.String("slot_key", slotKey),
				slog.String("old", string(oldSrc)),
				slog.String("new", string(newSrc)))
		}
	}

	a.mu.Lock()
	for _, r := range survivors {
		a.equipment[r.slot] = r.newID
	}
	if len(dropped) > 0 {
		// On-disk Equipment is now ahead of runtime; flip dirty so the
		// next persist trims dead slot entries (and any orphaned stat
		// modifier sets under their old source keys still sitting in
		// a.save.Stats — syncStatsToSaveLocked rewrites from the live
		// block, which doesn't contain them).
		a.syncEquipmentToSaveLocked()
		a.syncStatsToSaveLocked()
		a.markDirtyLocked()
	}
	a.mu.Unlock()
}

// untrackInventory removes every item entity in the actor's inventory
// and equipment from the runtime store. Called from teardown paths
// after the save has been written so the next login can respawn from
// the template ids. Safe to call with a nil store (tests) or empty
// containers.
func untrackInventory(ctx context.Context, store *entities.Store, a *connActor) {
	if store == nil {
		return
	}
	for _, id := range a.Equipment() {
		if err := store.Untrack(id); err != nil {
			logging.From(ctx).Debug("equipment: untrack on teardown",
				slog.String("entity_id", string(id)),
				slog.Any("err", err))
		}
	}
	for _, id := range a.Inventory() {
		if err := store.Untrack(id); err != nil {
			logging.From(ctx).Debug("inventory: untrack on teardown",
				slog.String("entity_id", string(id)),
				slog.Any("err", err))
		}
	}
}

// enterLinkDeadTeardown parks the actor in LinkDead phase per spec §7.2:
// drop only the connection-id index, keep entity / name / account /
// room indices intact, broadcast lost-connection. The cleanup tick
// handler reaps if no reconnect arrives within the timeout.
func enterLinkDeadTeardown(ctx context.Context, cfg Config, a *connActor, clk clock.Clock) {
	if !a.enterLinkDead(clk.Now()) {
		// Lost a race; another path already advanced the phase. Fall
		// back to full teardown so we don't leak a dangling actor.
		fullTeardown(ctx, cfg, a)
		return
	}
	cfg.Manager.RemoveConnectionOnly(a)

	room := a.Room()
	if room != nil {
		cfg.Manager.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s has lost their connection.", a.Name()), a.PlayerID())
	}
	logging.From(ctx).Info("session: link-dead",
		slog.String("player", a.PlayerName()),
		slog.Int("timeout_seconds", cfg.LinkDead.TimeoutSeconds))
}

// reconnect attaches a freshly-authenticated connection to an existing
// link-dead session and resumes the read loop. Per spec §7.4: swap
// the connection, re-install the byConn mapping, send "Reconnected.",
// re-render the room, then drop into pump.
func reconnect(ctx context.Context, c conn.Connection, cfg Config, a *connActor, clk clock.Clock) error {
	if !a.reattach(c, clk.Now()) {
		// Cleanup beat us to it; the link-dead session has been
		// reaped. Treat this connection as a fresh login fallback
		// would be too disruptive at this layer — close the new
		// connection with a friendly message and let the client try
		// again. (Vanishingly rare in practice.)
		_ = writeLine(ctx, c, "Your previous session has already ended; please reconnect.")
		return nil
	}
	if err := cfg.Manager.ReRegisterConnectionForSession(a, c.ID()); err != nil {
		logging.From(ctx).Warn("reconnect: re-register failed", slog.Any("err", err))
		// The actor is now in an inconsistent state (phase=Playing
		// but no byConn entry). Force full teardown to recover.
		a.mu.Lock()
		a.disconnecting = true
		a.mu.Unlock()
		fullTeardown(ctx, cfg, a)
		return nil
	}

	logging.From(ctx).Info("session: reconnected",
		slog.String("player", a.PlayerName()))

	if err := a.Write(ctx, renderReconnect()); err != nil {
		logging.From(ctx).Warn("reconnect: banner write failed", slog.Any("err", err))
	}
	if rendered := renderRoomForReconnect(a); rendered != "" {
		_ = a.Write(ctx, rendered)
	}

	exit := pump(ctx, c, cfg, a, clk)
	dispatchTeardown(ctx, cfg, a, exit, clk)
	return nil
}

// writeLine is a tiny helper for raw-conn writes that don't need the
// actor's color rendering (used before an actor exists, e.g. the
// "already online" rejection).
func writeLine(ctx context.Context, c conn.Connection, s string) error {
	_, err := c.Write(ctx, []byte(s+"\r\n"))
	return err
}

func resolveStartRoom(cfg Config, savedLoc string) (*world.Room, error) {
	if savedLoc != "" {
		if r, err := cfg.World.Room(world.RoomID(savedLoc)); err == nil {
			return r, nil
		}
	}
	return cfg.World.Room(cfg.StartID)
}

// connActor adapts a conn.Connection to the command.Actor interface and
// carries the player save record so SetRoom can mark it dirty.
//
// The manager back-reference is set by Manager.Add and used by SetRoom
// to keep the per-room broadcast index synchronized. Lock order is
// Manager → actor: the broadcast path (which takes actor locks via
// Write) snapshots its recipient list under the Manager lock and then
// releases before writing. SetRoom releases the actor lock before
// calling moveRoom for symmetry.
//
// playerID / accountID are immutable shadows of the save fields,
// cached at construction so the manager can read them lock-free
// during snapshot iteration. The Save record itself is only mutated
// for Location today.
type connActor struct {
	id   string
	conn conn.Connection

	playerID  string
	accountID string

	players *player.Store

	// items is the runtime entity store reference, captured at actor
	// construction so syncInventoryToSaveLocked can resolve template
	// ids without holding cfg in scope. Never reassigned after Add.
	items *entities.Store

	// inventory holds the runtime entity ids the actor is carrying.
	// Mutations go through AddToInventory / RemoveFromInventory and
	// flip the dirty bit so autosave commits the new contents.
	//
	// Invariant: an item id is in EXACTLY ONE of {inventory, equipment,
	// Placement room buckets} at any time. The get/drop/equip/unequip
	// paths pair their mutations so this never has to be reconciled
	// after the fact. Teardown only untracks ids from inventory +
	// equipment; Placement entries persist (items left in rooms stay).
	inventory []entities.EntityID
	// equipment maps slot key → equipped entity id. Slot keys are the
	// strings produced by slot.BuildKey: bare name for cap-1 slots,
	// "name:index" for multi-cap.
	equipment map[string]entities.EntityID
	// stats is the holder's sourced modifier set (M5.6). Equipment
	// modifiers apply under EquipmentSourceKey(item.ID()); on save the
	// snapshot is mirrored into a.save.Stats; on login Restore +
	// RebindSource rewires persisted source keys onto the fresh entity
	// ids spawned by respawnEquipment.
	stats *stats.Block

	mu            sync.Mutex
	room          *world.Room
	colorEnabled  bool
	save          *player.Save
	dirty         bool
	// saveGen is incremented on every mutation that flips dirty. Persist
	// captures the value at snapshot time and only clears dirty if the
	// counter hasn't advanced — guards against a concurrent equip /
	// inventory mutation getting lost because the in-flight Save wrote
	// stale state and then cleared dirty on completion. Comparing
	// individual fields (the M3-era approach, which compared only
	// Location) doesn't scale as the save shape grows.
	saveGen       uint64
	manager       *Manager
	flood         *floodGate
	floodCfg      *FloodConfig // retained so reattach() can rebuild a fresh bucket
	clk           clock.Clock  // retained for the same reason
	lastInputAt   time.Time
	idleWarned    bool
	disconnecting bool         // set when teardown (idle, flood, etc.) is in flight
	phase         sessionPhase // playing | linkDead | tearing (M4.4)
	linkDeadAt    time.Time    // when the actor entered linkDead phase
	takenOver     bool         // §6.1/§8 stale-event guard: stale teardown is a no-op
}

func (a *connActor) ID() string { return a.id }

func (a *connActor) Name() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return ""
	}
	return a.save.Name
}

// PlayerID is the immutable account-scoped identity of the character.
// Read lock-free from the shadow field so manager snapshots don't have
// to take the actor mutex.
func (a *connActor) PlayerID() string { return a.playerID }

func (a *connActor) Room() *world.Room {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.room
}

func (a *connActor) SetRoom(r *world.Room) {
	a.mu.Lock()
	var oldID world.RoomID
	if a.room != nil {
		oldID = a.room.ID
	}
	a.room = r
	if a.save != nil {
		a.save.Location = string(r.ID)
		a.markDirtyLocked()
	}
	mgr := a.manager
	a.mu.Unlock()

	if mgr != nil && oldID != r.ID {
		mgr.moveRoom(a, a.playerID, oldID, r.ID)
	}
}

func (a *connActor) ColorEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.colorEnabled
}

func (a *connActor) SetColorEnabled(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.colorEnabled = v
}

// Write expands any color markup in msg per the actor's color
// preference and writes the rendered text plus CRLF.
func (a *connActor) Write(ctx context.Context, msg string) error {
	rendered := ansi.Render(msg, a.ColorEnabled())
	_, err := a.conn.Write(ctx, []byte(rendered+"\r\n"))
	return err
}

// Persist writes the current player save through the store, but only
// when something has changed since the last write. Safe to call from
// any goroutine.
//
// The dirty flag is only cleared if no mutation occurred between
// snapshot and completion (saveGen unchanged) — otherwise an
// interleaved SetRoom / Equip / drop would be silently dropped: the
// mutation sets dirty=true while our Save is in flight, and we'd
// then clear that flag after writing the older snapshot.
//
// The snapshot deep-copies slice- and map-backed save fields so the
// YAML encoder running on snapshot cannot race a concurrent
// syncXxxToSaveLocked rewrite of a.save.
func (a *connActor) Persist(ctx context.Context) error {
	a.mu.Lock()
	if !a.dirty || a.save == nil || a.players == nil {
		a.mu.Unlock()
		return nil
	}
	snapshot := snapshotSave(a.save)
	gen := a.saveGen
	a.mu.Unlock()

	if err := a.players.Save(ctx, &snapshot); err != nil {
		return err
	}
	a.mu.Lock()
	if a.saveGen == gen {
		a.dirty = false
	}
	a.mu.Unlock()
	return nil
}

// snapshotSave produces an isolated copy of save suitable for a YAML
// encode that may run after the actor lock is released. Strings/ints
// copy by value; slices and maps need explicit duplication.
func snapshotSave(save *player.Save) player.Save {
	out := *save
	if save.Inventory != nil {
		dup := make([]string, len(save.Inventory))
		copy(dup, save.Inventory)
		out.Inventory = dup
	}
	if save.Equipment != nil {
		dup := make(map[string]player.EquippedItem, len(save.Equipment))
		for k, v := range save.Equipment {
			dup[k] = v
		}
		out.Equipment = dup
	}
	if save.Stats != nil {
		dup := make(stats.Snapshot, len(save.Stats))
		for i, e := range save.Stats {
			entryDup := e
			if e.Modifiers != nil {
				mods := make([]stats.Modifier, len(e.Modifiers))
				copy(mods, e.Modifiers)
				entryDup.Modifiers = mods
			}
			dup[i] = entryDup
		}
		out.Stats = dup
	}
	return out
}

// markDirtyLocked flips dirty and advances the save generation
// counter. Caller MUST hold a.mu. Centralized so every mutation path
// stays in step with Persist's "did anything change between snapshot
// and completion?" check; a bare `dirty = true` would set the flag
// but leave saveGen untouched and Persist would silently clear it.
func (a *connActor) markDirtyLocked() {
	a.dirty = true
	a.saveGen++
}

// Inventory returns a snapshot of the actor's currently-held item
// entity ids in pickup order. Safe to mutate.
func (a *connActor) Inventory() []entities.EntityID {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]entities.EntityID, len(a.inventory))
	copy(out, a.inventory)
	return out
}

// AddToInventory appends id to contents and marks the save dirty so
// autosave commits the new inventory list. The save's persisted
// Inventory field is updated in lockstep so a concurrent Persist
// snapshot is always self-consistent.
func (a *connActor) AddToInventory(id entities.EntityID) {
	a.mu.Lock()
	a.inventory = append(a.inventory, id)
	a.syncInventoryToSaveLocked()
	a.markDirtyLocked()
	a.mu.Unlock()
}

// RemoveFromInventory removes id from contents. Returns true on hit.
// Marks the save dirty when something was actually removed.
func (a *connActor) RemoveFromInventory(id entities.EntityID) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, e := range a.inventory {
		if e == id {
			a.inventory = append(a.inventory[:i], a.inventory[i+1:]...)
			a.syncInventoryToSaveLocked()
			a.markDirtyLocked()
			return true
		}
	}
	return false
}

// Equipment returns a snapshot of the actor's currently-equipped items
// keyed by slot key. Fresh map — safe to mutate.
func (a *connActor) Equipment() map[string]entities.EntityID {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]entities.EntityID, len(a.equipment))
	for k, v := range a.equipment {
		out[k] = v
	}
	return out
}

// Equip is the atomic equip-side mutation invoked by the equip
// command handler: removes id from inventory, installs it at slotKey,
// applies its modifiers to the holder's stat block under
// EquipmentSourceKey(id), and marks the save dirty. Returns false if
// id is not in inventory (the handler treats this as a TOCTOU loss to
// a concurrent drop and surfaces the same "you aren't carrying that"
// message).
//
// Auto-swap (§3.3 step 3) is NOT done here — the handler resolves the
// displaced slot key BEFORE calling Equip so the unequip side of the
// swap can be reported to the player. Equip is the leaf mutation.
func (a *connActor) Equip(slotKey string, id entities.EntityID, mods []stats.Modifier) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Verify id is in inventory and remove it atomically with the
	// equipment insertion.
	for i, e := range a.inventory {
		if e == id {
			a.inventory = append(a.inventory[:i], a.inventory[i+1:]...)
			a.equipment[slotKey] = id
			a.stats.Apply(entities.EquipmentSourceKey(id), mods)
			a.syncInventoryToSaveLocked()
			a.syncEquipmentToSaveLocked()
			a.syncStatsToSaveLocked()
			a.markDirtyLocked()
			return true
		}
	}
	return false
}

// Unequip is the atomic unequip-side mutation: removes the item at
// slotKey, returns it to inventory, reverses its stat modifiers by
// source key, marks dirty. Returns the entity id and true on success;
// (empty, false) if the slot is unoccupied.
func (a *connActor) Unequip(slotKey string) (entities.EntityID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id, ok := a.equipment[slotKey]
	if !ok {
		return "", false
	}
	delete(a.equipment, slotKey)
	a.inventory = append(a.inventory, id)
	a.stats.Remove(entities.EquipmentSourceKey(id))
	a.syncInventoryToSaveLocked()
	a.syncEquipmentToSaveLocked()
	a.syncStatsToSaveLocked()
	a.markDirtyLocked()
	return id, true
}

// StatsHas reports whether the holder's stat block carries any
// modifiers under src. Test-facing helper; production code goes
// through the equip/unequip mutations.
func (a *connActor) StatsHas(src entities.SourceKey) bool {
	return a.stats.Has(src)
}

// syncEquipmentToSaveLocked mirrors the runtime equipment map into the
// save's Equipment field, capturing both the template id (so respawn
// can locate the template) and the current entity id (so respawn can
// rebind the stats block's source key from old to new). Caller MUST
// hold a.mu.
func (a *connActor) syncEquipmentToSaveLocked() {
	if a.save == nil {
		return
	}
	if len(a.equipment) == 0 {
		a.save.Equipment = nil
		return
	}
	out := make(map[string]player.EquippedItem, len(a.equipment))
	for slotKey, id := range a.equipment {
		tpl, ok := a.lookupTemplateID(id)
		if !ok {
			// Untracked entity — drop from save. Matches the silent
			// drop policy in syncInventoryToSaveLocked.
			continue
		}
		out[slotKey] = player.EquippedItem{Template: tpl, Entity: string(id)}
	}
	a.save.Equipment = out
}

// syncStatsToSaveLocked rewrites a.save.Stats from the live block.
// Snapshot returns a fresh slice each call so the Persist path's
// shallow *a.save copy doesn't share backing storage.
func (a *connActor) syncStatsToSaveLocked() {
	if a.save == nil {
		return
	}
	a.save.Stats = a.stats.Snapshot()
}

// syncInventoryToSaveLocked mirrors the actor's runtime inventory
// (entity ids) into the save's persisted Inventory field (template ids)
// so the next Persist captures whatever pick-ups / drops have happened.
// Caller MUST hold a.mu.
func (a *connActor) syncInventoryToSaveLocked() {
	if a.save == nil {
		return
	}
	if len(a.inventory) == 0 {
		a.save.Inventory = nil
		return
	}
	tpls := make([]string, 0, len(a.inventory))
	for _, id := range a.inventory {
		tpl, ok := a.lookupTemplateID(id)
		if !ok {
			// Item is gone from the store between mutation and sync —
			// drop it silently. The runtime inventory still references
			// it, but persistence can only record what's resolvable.
			continue
		}
		tpls = append(tpls, tpl)
	}
	a.save.Inventory = tpls
}

// lookupTemplateID resolves an entity id back to its template id by
// asking the entity store. Returns ok=false if the store is unwired or
// the entity is unknown / not an item. The store reference is set when
// connActor is constructed (see session.run); it is never reassigned,
// so reading it without holding a.mu is safe.
func (a *connActor) lookupTemplateID(id entities.EntityID) (string, bool) {
	if a.items == nil {
		return "", false
	}
	e, ok := a.items.GetByID(id)
	if !ok {
		return "", false
	}
	it, ok := e.(*entities.ItemInstance)
	if !ok {
		return "", false
	}
	return string(it.TemplateID()), true
}

// PlayerName returns the loaded character's name, used by the autosave
// loop's structured logs. Returns "" for an actor that never finished
// login (shouldn't happen after Manager.Add, but defensive).
func (a *connActor) PlayerName() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return ""
	}
	return a.save.Name
}

func sanitizeForLog(s string) string {
	s = strings.ToValidUTF8(s, string(unicode.ReplacementChar))
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return unicode.ReplacementChar
		}
		return r
	}, s)
}

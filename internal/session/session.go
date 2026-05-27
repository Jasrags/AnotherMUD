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
	"sync/atomic"
	"time"
	"unicode"

	"github.com/Jasrags/AnotherMUD/internal/ansi"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
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
	// Contents is the container↔item index (M5.9b). Used by
	// respawnInventory to restore container nesting and by the put
	// handler to move items between actor inventory and containers.
	// May be nil only in tests that don't exercise containers.
	Contents *entities.Contents
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

	// Disposition is the room-entry hook surface (spec
	// mobs-ai-spawning §4). Passed through command.Env and called
	// directly from initial-login + link-dead-reconnect render
	// paths. May be nil in tests that don't exercise disposition.
	Disposition command.DispositionHook

	// Combat is the engage/disengage manager (spec combat §2),
	// passed through to command.Env so the kill verb (and future
	// flee / wimpy) can reach it. May be nil in tests that don't
	// exercise combat verbs.
	Combat *combat.Manager

	// Flee is the verb-driven flee primitive (M7.6). The function
	// closure captures the production FleeConfig built at the
	// composition root; command.Context.Flee receives the same shape.
	// nil in tests that don't exercise the flee verb.
	Flee func(ctx context.Context, c combat.CombatantID) combat.FleeOutcome

	// Progression is the M8.2 XP/level service. nil in tests that
	// don't exercise progression; in production the composition root
	// builds it from the pack-loaded track registry and a bus-backed
	// EventSink.
	Progression *progression.Manager

	// Races is the M8.3 race registry. Consulted at login to
	// resolve the player's race id into RacialFlags + starting
	// alignment. nil-safe: a missing registry means racial flags
	// are never applied to players (the engine falls back to
	// "racially blank" — equivalent to a player with no
	// declared race).
	Races *progression.RaceRegistry

	// Alignment is the M8.5 alignment manager. Consulted at
	// login to seed initial alignment for fresh characters
	// (race + class StartingAlignment summed) and to sync the
	// bucket tag on every login. nil-safe: a missing manager
	// leaves alignment at the persisted value with no tag
	// (disposition rules that match on alignment won't fire).
	Alignment *progression.AlignmentManager

	// Classes is the M8.4 class registry. Consulted at login so
	// the actor's classID is validated against loaded content, and
	// passed through to ClassPathProcessor + ApplyStatGrowth on
	// level-up subscriptions wired in cmd/anothermud. nil-safe:
	// missing registry means class-side effects never fire.
	Classes *progression.ClassRegistry

	// DefaultRace is the race id assigned to legacy saves with no
	// `race` field, and to fresh characters that haven't been
	// through a M12 character-creation flow yet. Empty means the
	// engine does not seed a default — players retain their
	// (empty) saved race and no flags apply. Production sets this
	// to "human" via ANOTHERMUD_DEFAULT_RACE.
	DefaultRace string

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
		contents:     cfg.Contents,
		equipment:    make(map[string]entities.EntityID),
		statBlock:    progression.NewWithBase(progression.DefaultPlayerBase()),
		progress:     progression.NewProgressionState(),
		// M7.5: vitals restore from the persisted save when present;
		// absent block (fresh character, migrated-from-v4 save) spawns
		// at full HP via NewVitals. The race/class/level inputs that
		// would derive real numbers for max HP here are M8.3/M8.4.
		vitals:      restorePlayerVitals(loaded.Player.Vitals),
		flood:       newFloodGate(floodCfg, clk),
		floodCfg:    &floodCfg,
		clk:         clk,
		lastInputAt: clk.Now(),
	}

	// M8.3: resolve the actor's race id. Save's race wins; an
	// empty saved race (legacy v7 or fresh character) falls back
	// to cfg.DefaultRace. If the resolved id isn't registered, the
	// actor stays raceless (raceID="", no flags) — see applyRace.
	applyRace(a, &cfg, loaded.Player.Race)
	// Keep save in sync with the resolved race so the next Persist
	// commits the assigned default. Only sync when the resolution
	// produced a real id — if applyRace returned with a.raceID=""
	// because the saved race isn't currently registered (content
	// removed between restarts), we PRESERVE the original id on the
	// save so re-adding the race later reattaches the character.
	// Closes m8-4 "applyClass fail-soft erases saved id" item +
	// the analogous m8-3 race-side concern in one symmetric fix.
	if a.raceID != "" && loaded.Player.Race != a.raceID {
		a.mu.Lock()
		a.save.Race = a.raceID
		a.markDirtyLocked()
		a.mu.Unlock()
	}

	// M8.4: resolve and apply class. The class id is informational
	// at session start (subscribers read it off the actor when a
	// level.up event lands); the apply step just validates against
	// the registry and snapshots the trains_available pool.
	//
	// Same fail-soft preservation as race above: an unresolved class
	// id (a.classID="") leaves the save's class field untouched so
	// re-adding the class later reattaches the character.
	applyClass(a, &cfg, loaded.Player.Class)
	a.mu.Lock()
	a.trainsAvailable = loaded.Player.TrainsAvailable
	if a.classID != "" && loaded.Player.Class != a.classID {
		a.save.Class = a.classID
		a.markDirtyLocked()
	}
	a.mu.Unlock()

	// M8.5: restore persisted alignment + sync bucket tag.
	// AlignmentManager.Bucket is idempotent and sets the tag
	// regardless of whether the integer changed, so a returning
	// character always lands with a current tag. For a fresh
	// character (loaded.New), seed initial alignment from race +
	// class StartingAlignment (spec §3.1, §4.1 "presentation
	// fields"). Set is the admin path — silent, no events, no
	// history. AlignmentManager may be nil in tests.
	a.mu.Lock()
	a.alignment = loaded.Player.Alignment
	a.mu.Unlock()
	if cfg.Alignment != nil {
		if loaded.New {
			seed := 0
			if cfg.Races != nil {
				if r, ok := cfg.Races.Get(a.raceID); ok {
					seed += r.StartingAlignment
				}
			}
			if cfg.Classes != nil {
				if c, ok := cfg.Classes.Get(a.classID); ok {
					seed += c.StartingAlignment
				}
			}
			if seed != 0 {
				cfg.Alignment.Set(ctx, a, seed, "character-created")
			} else {
				// Even at zero, install the neutral bucket tag so
				// rule matchers see a consistent tag set from
				// first login forward.
				_ = cfg.Alignment.Bucket(a)
			}
		} else {
			_ = cfg.Alignment.Bucket(a)
		}
	}

	// M8.4: publish character.created for brand-new characters so
	// the class-path processor wired in cmd/anothermud can run the
	// level-1 path grant. The M12 character-creation wizard will
	// own the canonical publish; this is the M8.4 stand-in so the
	// plumbing exists. Bus may be nil in tests — guard it.
	if loaded.New && cfg.Bus != nil {
		cfg.Bus.Publish(ctx, eventbus.CharacterCreated{
			EntityID: loaded.Player.ID,
			ClassID:  a.classID,
		})
	}

	// Seed the lock-free wimpy threshold from the persisted save.
	// Done after struct construction because atomic.Int32 has no
	// natural composite-literal initializer; this is the canonical
	// "Store the initial value before any reader can race the
	// goroutine that owns the actor" pattern.
	a.wimpyThreshold.Store(int32(clampWimpy(loaded.Player.WimpyThreshold)))

	// Install the persisted base attributes (M8.1 v6) before modifiers
	// so a level-up bump sitting on disk is in place before any equip-
	// driven modifier overlays on top.
	if len(loaded.Player.StatsBase) > 0 {
		a.statBlock.RestoreBase(loaded.Player.StatsBase)
	}
	// Install the persisted modifier set FIRST relative to respawn.
	// respawnEquipment will then rebind each entry's source key from
	// the saved entity id to the fresh one as it spawns the matching
	// ItemInstance. Order matters: the block must hold the old source
	// keys at the moment respawn runs so RebindSource has something
	// to find.
	a.statBlock.RestoreModifiers(loaded.Player.Stats)

	// M8.2: install persisted progression state. Empty snapshot is
	// a no-op (uninitialized tracks lazy-init on first interaction
	// per spec §5.3).
	if len(loaded.Player.Progression) > 0 {
		a.progress.Restore(loaded.Player.Progression)
	}

	// Respawn persisted inventory into live ItemInstances. Done before
	// the actor enters the manager / starts taking input so a takeover
	// or autosave can't observe a partial inventory.
	if cfg.Items != nil && cfg.Templates != nil {
		respawnInventory(ctx, a, cfg.Items, cfg.Contents, cfg.Templates, loaded.Player.Inventory)
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

	// Publish player.moved for the login spawn; From is empty since
	// the player wasn't previously in any room (spec §5.2 — empty
	// source room is a no-op for state clearing). Then run the
	// immediate hook before the room description.
	if cfg.Bus != nil {
		cfg.Bus.Publish(ctx, eventbus.PlayerMoved{
			PlayerID: a.PlayerID(),
			To:       start.ID,
		})
	}
	if cfg.Disposition != nil && a.PlayerID() != "" {
		cfg.Disposition.OnPlayerEnteredImmediate(ctx, a.PlayerID(), a.Name(), nil, start.ID)
	}

	if err := a.Write(ctx, command.RenderRoom(start, cfg.Placement, cfg.Items)); err != nil {
		// Initial render failed: the connection is unusable. Full
		// teardown immediately — no point parking link-dead.
		fullTeardown(ctx, cfg, a)
		return fmt.Errorf("first render: %w", err)
	}

	if cfg.Disposition != nil && a.PlayerID() != "" {
		cfg.Disposition.OnPlayerEnteredDeferred(ctx, a.PlayerID(), a.Name(), nil, start.ID)
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
			Contents:    cfg.Contents,
			Slots:       cfg.Slots,
			Bus:         cfg.Bus,
			Locator:     managerLocator{cfg.Manager},
			Disposition: cfg.Disposition,
			Combat:      cfg.Combat,
			Flee:        cfg.Flee,
			Progression: cfg.Progression,
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
	untrackInventory(ctx, cfg.Items, cfg.Contents, a)
}

// respawnInventory creates fresh ItemInstances for each persisted
// InventoryEntry and attaches them to the actor. Containers recurse
// into their Contents: each child is spawned and pushed into the
// freshly-minted container via contents.Put. Missing templates are
// logged and skipped — the player loses that item (and anything
// nested inside it, if it was a container) on this load.
//
// Whenever any entry is dropped (template gone, spawn failure, or a
// non-container template with non-empty Contents), the in-memory
// save is rebuilt from survivors and dirty is flipped so the next
// Persist trims dead references.
func respawnInventory(ctx context.Context, a *connActor, store *entities.Store, contents *entities.Contents, tpls *item.Templates, saved []player.InventoryEntry) {
	if len(saved) == 0 {
		return
	}
	survivors, dropped := spawnEntries(ctx, a, store, contents, tpls, saved, "")
	for _, id := range survivors.ids {
		a.mu.Lock()
		a.inventory = append(a.inventory, id)
		a.mu.Unlock()
	}
	if dropped {
		a.mu.Lock()
		a.save.Inventory = survivors.entries
		a.markDirtyLocked()
		a.mu.Unlock()
	}
}

// spawnedSlice carries the parallel result of a respawn pass: the
// top-level entity ids (so the caller can attach them to the
// holder), and the post-prune save entries (so the caller can trim
// the on-disk record when something didn't survive). Kept as a
// dedicated type rather than two returns so the recursive call site
// reads cleanly.
type spawnedSlice struct {
	ids     []entities.EntityID
	entries []player.InventoryEntry
}

// spawnEntries is the recursive worker behind respawnInventory.
// parentID is the container entity id to put children into; empty
// means "top-level inventory, no put". Returns the spawned entity
// ids in source order and the surviving entries; the dropped flag
// tells the caller whether any entry was pruned (so a Persist can
// trim).
//
// A non-container template carrying Contents in the save is a sign
// of either a content edit (the template's type changed since save
// time) or a corrupted save. Drop the Contents quietly and keep the
// item — losing one container is better than refusing to load.
func spawnEntries(ctx context.Context, a *connActor, store *entities.Store, contents *entities.Contents, tpls *item.Templates, saved []player.InventoryEntry, parentID entities.EntityID) (spawnedSlice, bool) {
	out := spawnedSlice{
		ids:     make([]entities.EntityID, 0, len(saved)),
		entries: make([]player.InventoryEntry, 0, len(saved)),
	}
	dropped := false
	for _, entry := range saved {
		tpl, err := tpls.Get(item.TemplateID(entry.Template))
		if err != nil {
			logging.From(ctx).Warn("inventory: dropping unknown template",
				slog.String("template_id", entry.Template),
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
			dropped = true
			continue
		}
		inst, err := store.Spawn(tpl)
		if err != nil {
			logging.From(ctx).Warn("inventory: spawn failed",
				slog.String("template_id", entry.Template),
				slog.Any("err", err))
			dropped = true
			continue
		}
		if parentID != "" && contents != nil {
			contents.Put(parentID, inst.ID())
		}

		survivor := player.InventoryEntry{Template: entry.Template}
		if len(entry.Contents) > 0 {
			if tpl.Type != "container" || contents == nil {
				// A non-container template carrying nested contents in
				// the save can't be honored at runtime. Keep the
				// outer item, drop the children.
				logging.From(ctx).Warn("inventory: dropping nested contents on non-container",
					slog.String("template_id", entry.Template),
					slog.String("template_type", tpl.Type))
				dropped = true
			} else {
				child, childDropped := spawnEntries(ctx, a, store, contents, tpls, entry.Contents, inst.ID())
				if len(child.entries) > 0 {
					survivor.Contents = child.entries
				}
				if childDropped {
					dropped = true
				}
			}
		}
		out.ids = append(out.ids, inst.ID())
		out.entries = append(out.entries, survivor)
	}
	return out, dropped
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
		if !a.statBlock.RebindSource(oldSrc, newSrc) {
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
//
// Container children (M5.9b) are swept recursively via Contents: a
// carried sack with three items inside has all four entities
// untracked. Contents entries are also cleared so the index doesn't
// retain phantom mappings to untracked ids. A nil Contents skips
// the recursion (tests that don't exercise containers).
func untrackInventory(ctx context.Context, store *entities.Store, contents *entities.Contents, a *connActor) {
	if store == nil {
		return
	}
	for _, id := range a.Equipment() {
		untrackTree(ctx, store, contents, id)
	}
	for _, id := range a.Inventory() {
		untrackTree(ctx, store, contents, id)
	}
}

// untrackTree untracks id and, if it's a container with Contents
// entries, recurses into its children. Each child is also cleared
// from the Contents index so the post-teardown state is clean.
// Logs at Debug on failure: untracking an already-gone entity is
// not a bug worth a Warn.
func untrackTree(ctx context.Context, store *entities.Store, contents *entities.Contents, id entities.EntityID) {
	if contents != nil {
		for _, childID := range contents.In(id) {
			untrackTree(ctx, store, contents, childID)
		}
		contents.Take(id)
	}
	if err := store.Untrack(id); err != nil {
		logging.From(ctx).Debug("inventory: untrack on teardown",
			slog.String("entity_id", string(id)),
			slog.Any("err", err))
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
	// Treat reconnect as a fresh room entry for the disposition
	// system (spec §5.2 — the player effectively just walked back
	// in). Publish player.moved with From=To so any prior per-room
	// state for this player is cleared, then run both hooks around
	// the re-render. Capture the room once: a.Room() can in
	// principle change between calls (reconnect runs before pump,
	// so the window is near-zero today, but the snapshot keeps the
	// immediate/deferred pair consistent regardless).
	room := a.Room()
	if room != nil {
		if cfg.Bus != nil {
			cfg.Bus.Publish(ctx, eventbus.PlayerMoved{
				PlayerID: a.PlayerID(),
				From:     room.ID,
				To:       room.ID,
			})
		}
		if cfg.Disposition != nil && a.PlayerID() != "" {
			cfg.Disposition.OnPlayerEnteredImmediate(ctx, a.PlayerID(), a.Name(), nil, room.ID)
		}
	}
	if rendered := renderRoomForReconnect(a, cfg); rendered != "" {
		_ = a.Write(ctx, rendered)
	}
	if room != nil && cfg.Disposition != nil && a.PlayerID() != "" {
		cfg.Disposition.OnPlayerEnteredDeferred(ctx, a.PlayerID(), a.Name(), nil, room.ID)
	}

	exit := pump(ctx, c, cfg, a, clk)
	dispatchTeardown(ctx, cfg, a, exit, clk)
	return nil
}

// managerLocator adapts *Manager to command.Locator. The adapter
// exists for one reason only: Manager.FindInRoom returns *connActor
// (an unexported type), and assigning a typed-nil *connActor into a
// command.Actor interface yields a non-nil interface — a classic Go
// pitfall. Routing through this adapter lets us return an untyped
// nil interface when the lookup misses.
type managerLocator struct{ m *Manager }

func (ml managerLocator) FindInRoom(roomID world.RoomID, name string) command.Actor {
	a := ml.m.FindInRoom(roomID, name)
	if a == nil {
		return nil
	}
	return a
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
	// contents is the container↔item index reference, captured at
	// actor construction so syncInventoryToSaveLocked can walk
	// container trees and untrackInventory can sweep child entries.
	// Nil only in tests that don't exercise containers.
	contents *entities.Contents

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
	// statBlock is the actor's progression-layer stat block (M8.1 —
	// docs/specs/progression.md §2). Holds base attributes (the six
	// classics + vital maxima + the combat-derived hit_mod / ac slots
	// today) plus the holder's sourced modifier set. Equipment
	// modifiers apply under EquipmentSourceKey(item.ID()); on save
	// the base and modifier snapshots are mirrored into a.save; on
	// login they're restored before respawnEquipment rewires source
	// keys onto fresh entity ids.
	//
	// statBlock subsumes the M5.6 stats.Block: the modifier-set
	// surface lives inside StatBlock and the persisted shape
	// (stats.Snapshot) round-trips unchanged. The pointer is
	// established at construction and never reassigned for the life
	// of the actor; StatBlock carries its own internal RWMutex so
	// combat tick reads of Stats() do not serialize on a.mu.
	statBlock *progression.StatBlock

	// vitals is the actor's mutable HP state (M7.1). The pointer is
	// established at login and never reassigned; combat applies damage
	// and heals through the pointer under its own lock. Persistence
	// landed with M7.5 (player.Save.Vitals).
	vitals *combat.Vitals

	// raceID is the canonical race id the actor was constructed
	// with (M8.3). Established at login from save (or the
	// configured default for legacy v7 saves / fresh characters)
	// and never reassigned for the life of the actor — race-change
	// at runtime is a M10+ admin verb. Lowercased on assignment.
	raceID string

	// racialTags is the snapshot of racial-flag strings the race
	// definition contributed at construction. Stored alongside
	// raceID so Tags() can surface them without re-resolving the
	// registry on every read. Set once at construction.
	racialTags []string

	// classID is the actor's resolved class id (M8.4). Established
	// at login from save; empty means no class (the path processor
	// and stat-growth subscriber short-circuit). Lowercased on
	// assignment for case-insensitive registry lookups. Never
	// reassigned for the life of the actor — class swaps land with
	// the M10+ admin verb / quest reward path.
	classID string

	// trainsAvailable is the actor's training pool (spec §4.6
	// step 4 + §7.1). M8.4 credits via StatGrowthSubscriber on
	// every level-up; the M8.6 train verb is the only consumer.
	// Guarded by a.mu since the credit happens off the bus
	// dispatch and Persist also reads it.
	trainsAvailable int

	// alignment is the actor's integer alignment property
	// (M8.5 — spec progression.md §6.1). Written through
	// AlignmentManager.Set / Shift; read by AI disposition
	// matching and by Persist via syncAlignmentToSaveLocked.
	// Guarded by a.mu — same actor-lock discipline as
	// trainsAvailable.
	alignment int

	// alignmentTag is the spec §6.2 bucket tag mirrored onto
	// the actor (one of TagAlignmentEvil/Neutral/Good or empty
	// for an actor the alignment manager has not touched yet).
	// Tags() appends it to racialTags so the AI evaluator's
	// PlayerView carries it for has_tag matchers.
	alignmentTag string

	// progress is the actor's progression-track state (M8.2 —
	// docs/specs/progression.md §5.2). Holds per-track (level, xp)
	// maps; mutated through progression.Manager operations and
	// persisted in player.Save.Progression. The pointer is set at
	// construction and never reassigned; ProgressionState carries
	// its own internal lock so reads from a tick goroutine don't
	// serialize on a.mu.
	progress *progression.ProgressionState

	// wimpyThreshold is the §5.1 HP%-threshold property. 0 disables
	// wimpy. Set by the `wimpy <pct>` verb; read by combat's wimpy
	// phase via the WimpyHolder interface (defined on combat package
	// side; this connActor satisfies it). Persistence lives with the
	// rest of the save shape — see player.Save.WimpyThreshold.
	//
	// Stored as atomic.Int32 so the read path (tick goroutine, every
	// combat round) does NOT take a.mu. Persist holds a.mu across the
	// autosave file write; routing every wimpy read through that lock
	// would stall the combat tick for the duration of disk I/O. The
	// write path still acquires a.mu because it also mutates a.save
	// — but the field-level read stays lock-free.
	wimpyThreshold atomic.Int32

	mu           sync.Mutex
	room         *world.Room
	colorEnabled bool
	save         *player.Save
	dirty        bool
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
	// Sync vitals into save BEFORE the dirty check so HP changes
	// from the combat tick (which never go through markDirtyLocked
	// — combat doesn't know about session) participate in autosave.
	if a.syncVitalsToSaveLocked() {
		a.markDirtyLocked()
	}
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

// cloneInventoryEntries deep-copies an InventoryEntry tree so a
// concurrent syncInventoryToSaveLocked rewrite of a.save.Inventory
// can't race the YAML encode that runs after the actor lock is
// released. The recursion stops naturally at leaves (empty
// Contents).
func cloneInventoryEntries(in []player.InventoryEntry) []player.InventoryEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]player.InventoryEntry, len(in))
	for i, e := range in {
		out[i] = player.InventoryEntry{
			Template: e.Template,
			Contents: cloneInventoryEntries(e.Contents),
		}
	}
	return out
}

// clampWimpy normalizes a persisted wimpy threshold into [0, 100].
// Anything outside the range maps to 0 (disabled) — defensively
// permissive so a hand-edited save with a nonsense value loads
// cleanly into a known-good disabled state.
func clampWimpy(pct int) int {
	if pct < 0 || pct > 100 {
		return 0
	}
	return pct
}

// restorePlayerVitals returns a fresh *combat.Vitals for a logging-in
// player. A nil persisted block (legacy v4 save, brand-new character)
// spawns at full HP with the hardcoded player max. A persisted block
// rehydrates via NewVitalsAt, which clamps to sane ranges if the saved
// values are out of bounds (HP > MaxHP, MaxHP < 1).
func restorePlayerVitals(v *player.VitalsState) *combat.Vitals {
	if v == nil {
		return combat.NewVitals(combat.DefaultPlayerMaxHP)
	}
	maxHP := v.MaxHP
	if maxHP < 1 {
		maxHP = combat.DefaultPlayerMaxHP
	}
	// Safety floor: a player whose save records HP <= 0 (killed in
	// combat, then disconnected before any recovery path ran) would
	// otherwise log back in dead, unable to act, with no way out
	// short of operator intervention. Combat spec §6.4 says the
	// player-death subscriber owns recovery (corpse, respawn, gear
	// loss); until that lands (tracked in m7-5-deferred-fixes.md #1)
	// clamp HP up to 1 on restore so login is at least playable.
	// Removing this floor is safe the moment a real §6.4 subscriber
	// guarantees no save ever serializes HP <= 0.
	hp := v.HP
	if hp <= 0 {
		hp = 1
	}
	return combat.NewVitalsAt(hp, maxHP)
}

// syncVitalsToSaveLocked rewrites a.save.Vitals from a.vitals if the
// live HP differs from what's currently in the save. Returns true if
// the save record actually changed; callers use the result to decide
// whether to bump the dirty flag.
//
// Caller MUST hold a.mu. Lives next to the other syncXxxToSaveLocked
// helpers (inventory, equipment, stats) so the persist path has a
// single read-vitals → write-save touchpoint.
func (a *connActor) syncVitalsToSaveLocked() bool {
	if a.save == nil || a.vitals == nil {
		return false
	}
	cur, max := a.vitals.Snapshot()
	if a.save.Vitals != nil && a.save.Vitals.HP == cur && a.save.Vitals.MaxHP == max {
		return false
	}
	a.save.Vitals = &player.VitalsState{HP: cur, MaxHP: max}
	return true
}

// snapshotSave produces an isolated copy of save suitable for a YAML
// encode that may run after the actor lock is released. Strings/ints
// copy by value; slices and maps need explicit duplication.
func snapshotSave(save *player.Save) player.Save {
	out := *save
	if save.Inventory != nil {
		out.Inventory = cloneInventoryEntries(save.Inventory)
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
	if save.Vitals != nil {
		v := *save.Vitals
		out.Vitals = &v
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
			a.statBlock.AddModifiers(entities.EquipmentSourceKey(id), mods)
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
	a.statBlock.RemoveBySource(entities.EquipmentSourceKey(id))
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
	return a.statBlock.HasSource(src)
}

// GrantXP grants amount XP on track via the supplied
// progression.Manager and marks the actor dirty so the next
// Persist commits the new state. Wraps Manager.GrantExperience —
// callers go through this rather than reaching into a.progress
// directly so the dirty bit is consistently flipped on every
// mutation path (admin xp verb today, future combat-driven and
// quest-driven sources later).
//
// Returns the structured result for renderers ("you reach level
// 2!"). A nil manager is a no-op (returns TrackUnknown=true);
// tests that don't wire Progression don't need to special-case.
func (a *connActor) GrantXP(ctx context.Context, mgr *progression.Manager, track, source string, amount int64) progression.GrantResult {
	if mgr == nil {
		return progression.GrantResult{Track: track, TrackUnknown: true}
	}
	res := mgr.GrantExperience(ctx, a.progress, a.CombatantIDString(), track, amount, source)
	if res.XPAdded > 0 || res.NewLevel != res.OldLevel {
		a.mu.Lock()
		a.syncProgressionToSaveLocked()
		a.markDirtyLocked()
		a.mu.Unlock()
	}
	return res
}

// DeductXP removes amount XP on track via the supplied Manager
// and marks dirty if anything was lost. Wraps
// Manager.DeductExperience.
func (a *connActor) DeductXP(ctx context.Context, mgr *progression.Manager, track string, amount int64) progression.DeductResult {
	if mgr == nil {
		return progression.DeductResult{Track: track, TrackUnknown: true}
	}
	res := mgr.DeductExperience(ctx, a.progress, a.CombatantIDString(), track, amount)
	if res.XPLost > 0 {
		a.mu.Lock()
		a.syncProgressionToSaveLocked()
		a.markDirtyLocked()
		a.mu.Unlock()
	}
	return res
}

// TrackInfo returns the structured per-track view (level, XP,
// XpToNext, etc.) for renderers. Returns (zero, false) when track
// is unknown.
func (a *connActor) TrackInfo(mgr *progression.Manager, track string) (progression.TrackInfo, bool) {
	if mgr == nil {
		return progression.TrackInfo{}, false
	}
	return mgr.GetTrackInfo(a.progress, track)
}

// CombatantIDString returns the string form of CombatantID, used
// as the entity-id payload in progression event emissions. Mirrors
// the form combat already uses for player events.
func (a *connActor) CombatantIDString() string {
	return string(a.CombatantID())
}

// RaceID returns the actor's resolved race id. Empty means
// raceless (no flags applied, no cast-cost modifier). Set once at
// construction; never mutates for the life of the actor.
func (a *connActor) RaceID() string { return a.raceID }

// Tags returns the actor's session-side tag set — today just the
// racial flags from the race definition (M8.3). Returns a fresh
// slice so callers cannot alias the backing storage. Surfaces to
// the AI disposition evaluator via session.Manager.PlayersInRoom
// and dispositionHook so `has_tag` rules can match on racial
// flags. Future per-actor tags (admin role, party affiliation,
// curse effects) will join this list as their consumers arrive.
func (a *connActor) Tags() []string {
	a.mu.Lock()
	at := a.alignmentTag
	a.mu.Unlock()
	if len(a.racialTags) == 0 && at == "" {
		return nil
	}
	out := make([]string, 0, len(a.racialTags)+1)
	out = append(out, a.racialTags...)
	if at != "" {
		out = append(out, at)
	}
	return out
}

// applyRace resolves the actor's race id from save (taking
// cfg.DefaultRace as the fallback for an empty save value) and
// snapshots the race's RacialFlags onto the actor for the life of
// the session. Called once during construction in run(); the
// resolved id is also written back to a.save.Race so the next
// Persist commits the default-substitution. If the resolved id is
// not in the registry (race removed between restarts) the actor
// stays raceless with an empty raceID — fail-soft mirrors the mob
// spawn behavior.
func applyRace(a *connActor, cfg *Config, saved string) {
	candidate := strings.ToLower(strings.TrimSpace(saved))
	if candidate == "" {
		candidate = strings.ToLower(strings.TrimSpace(cfg.DefaultRace))
	}
	if candidate == "" || cfg.Races == nil {
		return
	}
	race, ok := cfg.Races.Get(candidate)
	if !ok {
		// Race id present on save but not in the registry —
		// content removed it. Keep raceID empty so no stale flags
		// apply. The next save still records the requested id so
		// content authors who readd the race see their players
		// reconnect with it.
		return
	}
	a.raceID = race.ID
	if len(race.RacialFlags) > 0 {
		a.racialTags = make([]string, len(race.RacialFlags))
		copy(a.racialTags, race.RacialFlags)
	}
}

// ClassID returns the actor's resolved class id. Empty means
// classless (no path / no stat growth on level-up). Set once at
// construction; never mutates for the life of the actor.
func (a *connActor) ClassID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.classID
}

// TrainsAvailable returns the actor's current training pool. Read
// by the M8.6 train verb; surfaced on score panels later.
func (a *connActor) TrainsAvailable() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.trainsAvailable
}

// CreditTrains adds n to trainsAvailable and marks the actor
// dirty so the next Persist commits the new pool. Used by the
// M8.4 stat-growth subscriber on every bound-track level-up
// (spec §4.6 step 4). Negative values are clamped at zero — the
// train verb is the only path that subtracts and it handles
// underflow itself.
func (a *connActor) CreditTrains(_ context.Context, _ string, n int) {
	if n == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if n < 0 && -n > a.trainsAvailable {
		a.trainsAvailable = 0
	} else {
		a.trainsAvailable += n
	}
	if a.save != nil {
		a.save.TrainsAvailable = a.trainsAvailable
	}
	a.markDirtyLocked()
}

// Alignment returns the actor's current alignment integer
// (M8.5 — spec §6.1). Reads under a.mu so the value is consistent
// with concurrent Shift writes.
func (a *connActor) Alignment() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.alignment
}

// SetAlignment writes the alignment integer. Used by the
// AlignmentEntity adapter under the manager's per-entity lock.
// Marks the save dirty so the new value rides to disk on the
// next Persist.
func (a *connActor) SetAlignment(value int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alignment = value
	if a.save != nil {
		a.save.Alignment = value
	}
	a.markDirtyLocked()
}

// SetAlignmentTag mirrors the bucket tag onto the actor (spec
// §6.2). Empty tag clears the slot. AI disposition matching
// reads through Tags(), which appends the alignment tag to the
// racial-flag list.
func (a *connActor) SetAlignmentTag(tag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alignmentTag = tag
}

// AlignmentTag returns the actor's current bucket tag (one of
// progression.TagAlignment{Evil,Neutral,Good} or empty for an
// untouched actor). Read by the session manager to populate
// PlayerInfo.Bucket without needing a reference to the
// AlignmentManager.
func (a *connActor) AlignmentTag() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.alignmentTag
}

// HasTag reports whether the actor carries tag. Used by the
// AlignmentEntity adapter to detect the admin role bypass
// (spec §6.4 Shift step 2). Scans racial flags + alignment tag
// in a single locked section. racialTags is write-once at
// construction today, but a future verb that mutates it would
// race a tags reader without the lock — taking a.mu here keeps
// the surface consistent with Tags() and future-proofs the
// admin-tag addition (M10+ role system).
func (a *connActor) HasTag(tag string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, t := range a.racialTags {
		if t == tag {
			return true
		}
	}
	return a.alignmentTag == tag
}

// StatBlock returns the actor's progression-layer stat block. The
// returned pointer is the live block — callers MUST treat it as
// read-mostly and use StatBlock's own internal lock for any
// mutation (AdjustBase, AddModifiers, etc.). Used by the M8.4
// stat-growth subscriber to apply level-up growth dice without
// having to thread a.mu through.
func (a *connActor) StatBlock() *progression.StatBlock { return a.statBlock }

// ProgressionState returns the actor's per-track (level, xp)
// state. Same contract as StatBlock — the state has its own lock.
func (a *connActor) ProgressionState() *progression.ProgressionState { return a.progress }

// applyClass resolves the actor's class id from save. Empty saves
// stay empty (no default class today; M12 character-creation owns
// initial selection). A non-empty id that doesn't resolve in the
// registry is treated as removed-content: the actor stays
// classless with classID="" so no path/growth fires. Mirrors
// applyRace's fail-soft policy.
func applyClass(a *connActor, cfg *Config, saved string) {
	candidate := strings.ToLower(strings.TrimSpace(saved))
	if candidate == "" || cfg.Classes == nil {
		return
	}
	cls, ok := cfg.Classes.Get(candidate)
	if !ok {
		return
	}
	a.classID = cls.ID
}

// MarkContentsDirty re-runs syncInventoryToSaveLocked so the save
// tree picks up Contents mutations the actor itself didn't perform
// (M5.9b put: the handler removes from inventory, then writes into
// a container via the Contents substrate; the inventory-remove
// re-sync ran before the Contents.Put so it captured an empty
// container in the save tree). Re-syncing here closes the gap and
// flips dirty so the corrected tree reaches disk on next Persist.
//
// Cheap: the tree walker is a recursive Contents.In() scan over
// items the actor already carries.
func (a *connActor) MarkContentsDirty() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.syncInventoryToSaveLocked()
	a.markDirtyLocked()
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
	a.save.Stats = a.statBlock.ModifiersSnapshot()
	a.save.StatsBase = a.statBlock.BaseSnapshot()
}

// syncProgressionToSaveLocked rewrites a.save.Progression from the
// live state. Called from the Persist path before the dirty check
// so XP grants from the combat tick or admin commands between
// autosaves round-trip through disk. Caller MUST hold a.mu.
func (a *connActor) syncProgressionToSaveLocked() {
	if a.save == nil || a.progress == nil {
		return
	}
	a.save.Progression = a.progress.Snapshot()
}

// syncInventoryToSaveLocked mirrors the actor's runtime inventory
// into the save's persisted Inventory field, recursing into
// container Contents so nested items round-trip across restart
// (M5.9b). Caller MUST hold a.mu.
//
// Entities the store no longer knows about are dropped silently
// (same policy as the leaf-only v3 implementation): the runtime
// inventory may still reference them, but persistence can only
// record what's resolvable.
func (a *connActor) syncInventoryToSaveLocked() {
	if a.save == nil {
		return
	}
	if len(a.inventory) == 0 {
		a.save.Inventory = nil
		return
	}
	a.save.Inventory = a.buildSaveEntriesLocked(a.inventory)
}

// buildSaveEntriesLocked walks a slice of entity ids and emits the
// matching InventoryEntry list, recursing into a.contents for any
// container-typed entity. Caller MUST hold a.mu. Returns nil for an
// all-dropped slice so the caller can decide whether to write `nil`
// (top-level Inventory) or omit the Contents field on a leaf entry
// (via omitempty).
//
// Lock-order note: this function acquires a.contents.mu (via
// Contents.In) while a.mu is held. That is the canonical order
// documented on entities.Contents — actor.mu → contents.mu. New
// callers of Contents that mutate actor state in response MUST NOT
// hold contents.mu while taking actor.mu; doing so would deadlock
// against a concurrent autosave Persist on that actor.
func (a *connActor) buildSaveEntriesLocked(ids []entities.EntityID) []player.InventoryEntry {
	if len(ids) == 0 {
		return nil
	}
	out := make([]player.InventoryEntry, 0, len(ids))
	for _, id := range ids {
		tpl, ok := a.lookupTemplateID(id)
		if !ok {
			continue
		}
		entry := player.InventoryEntry{Template: tpl}
		if a.contents != nil && a.isContainerLocked(id) {
			if child := a.buildSaveEntriesLocked(a.contents.In(id)); len(child) > 0 {
				entry.Contents = child
			}
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// isContainerLocked reports whether id resolves to a container-typed
// item in the entity store. Used by the inventory tree builder to
// decide whether to recurse into Contents. Cheap: the store does a
// single map lookup. Returning false on store/type mismatch is
// correct — a non-container can never legally appear in the
// Contents index (the put handler enforces this).
func (a *connActor) isContainerLocked(id entities.EntityID) bool {
	if a.items == nil {
		return false
	}
	e, ok := a.items.GetByID(id)
	if !ok {
		return false
	}
	it, ok := e.(*entities.ItemInstance)
	if !ok {
		return false
	}
	return it.Type() == "container"
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

// CombatantID returns the combat-side identity of this actor. The
// PlayerPrefix keeps the namespace disjoint from mob ids (see
// combat.CombatantID). PlayerID is account-scoped and stable across
// reconnects so a fight started against this player survives a
// link-dead reattach.
func (a *connActor) CombatantID() combat.CombatantID {
	return combat.NewPlayerCombatantID(a.playerID)
}

// WimpyThreshold returns the actor's configured wimpy HP-percent
// threshold ([0,100]; 0 disables). Satisfies combat.WimpyHolder.
// Lock-free: the underlying atomic.Int32 makes this safe to call
// from the tick goroutine without touching a.mu, which Persist
// holds across autosave file I/O.
func (a *connActor) WimpyThreshold() int {
	return int(a.wimpyThreshold.Load())
}

// SetWimpyThreshold updates the wimpy property and marks the save
// dirty so the new value persists on the next autosave. Clamps to
// [0, 100]; values outside are silently coerced (the verb handler
// already validates input, this is defense-in-depth).
func (a *connActor) SetWimpyThreshold(pct int) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if int(a.wimpyThreshold.Load()) == pct {
		return
	}
	a.wimpyThreshold.Store(int32(pct))
	if a.save != nil {
		a.save.WimpyThreshold = pct
		a.markDirtyLocked()
	}
}

// Vitals returns the actor's mutable HP state. The pointer is set at
// construction time in run() and is never reassigned for the life of
// the connActor, so reading it without taking a.mu is safe (the
// pointer itself; the Vitals struct carries its own internal lock).
func (a *connActor) Vitals() *combat.Vitals { return a.vitals }

// Stats returns the actor's combat stat block derived from the
// progression-layer StatBlock (M8.1). HitMod, AC, and STR are read
// through the StatBlock's effective values — base attribute +
// sum-of-modifiers — so equipment-driven modifiers now flow into
// auto-attack and consider without a separate sync step.
//
// Damage and WeaponName remain unset here at M8.1; combat falls
// through to the engine's unarmed defaults via EffectiveDamage /
// EffectiveWeaponName. Real weapon-equipment plumbing arrives with
// the post-M8 equipment-stat work.
//
// LOCK NOTE: StatBlock carries its own RWMutex, so this method does
// not take a.mu — Effective reads are safe to call concurrently with
// session-side equip / unequip mutations.
func (a *connActor) Stats() combat.Stats {
	return combat.Stats{
		HitMod: a.statBlock.Effective(progression.StatHitMod),
		AC:     a.statBlock.Effective(progression.StatAC),
		STR:    a.statBlock.Effective(progression.StatSTR),
	}
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

package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/action"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ErrSessionGone is returned by ReRegisterConnectionForSession when the
// target actor has already been removed from the manager (e.g. the
// link-dead cleanup sweep ran between the reattach and the re-register).
var ErrSessionGone = errors.New("session: actor no longer registered")

// Manager is the multi-index registry of logged-in sessions. It owns
// every lookup ("which session is on this connection?", "is anyone
// named Alice online?") and every broadcast ("tell everyone in the
// town square X"). M4.1 covers the indices + room broadcasts; flood,
// idle, link-dead, and takeover land in later M4 phases per
// docs/specs/session-lifecycle.md.
//
// Lock order: callers of public methods MUST NOT hold any connActor
// lock when entering Manager methods. Manager takes its own RWMutex
// to copy snapshots out, then releases before calling back into
// actors (Write / Persist). This avoids a Manager↔actor deadlock.
type Manager struct {
	mu         sync.RWMutex
	byConn     map[string]*connActor
	byPlayerID map[string]*connActor
	byName     map[string]*connActor                  // key: lowercased name
	byAccount  map[string][]*connActor                // key: account id
	byRoom     map[world.RoomID]map[string]*connActor // roomID → pid → actor
	roomByPID  map[string]world.RoomID                // pid → current room

	// onRemove is invoked after Remove has scrubbed the indices for
	// a fully-removed actor. Set via SetOnRemove from the composition
	// root to hook session-gone events (notification unregister,
	// future per-feature teardown). nil-safe; never called from
	// RemoveConnectionOnly (link-dead transition keeps the actor
	// tracked).
	//
	// The callback receives the gone-actor's player id rather than
	// the *connActor pointer because the cleanup only needs that id
	// and a pointer-typed callback would force the composition root
	// to depend on the unexported actor type.
	onRemove func(playerID string)

	// mounts dematerializes a departing actor's live mounts on full Remove
	// (mounts.md §9, §10): a logged-out owner's retrieved mount resolves to
	// its stabled record — the live creature must leave the world so it is
	// never orphaned or duplicated on the owner's return. nil disables the
	// teardown (tests / headless). Set once at startup via SetMounts.
	mounts command.MountService

	// action-economy.md timed-action sweep. actionTracker holds in-flight
	// occupations; actionCommands + actionEnv replay a completed action's
	// command with ReplayAction set so its consumer performs the deferred
	// mutation. All nil until enableActionSweep wires them from the Config
	// (timed actions disabled otherwise). Set once at startup via Handler.
	actionTracker  *action.Tracker
	actionCommands *command.Registry
	actionEnv      command.Env
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{
		byConn:     make(map[string]*connActor),
		byPlayerID: make(map[string]*connActor),
		byName:     make(map[string]*connActor),
		byAccount:  make(map[string][]*connActor),
		byRoom:     make(map[world.RoomID]map[string]*connActor),
		roomByPID:  make(map[string]world.RoomID),
	}
}

// SetOnRemove installs a callback fired after every full Remove.
// Used by the composition root to bridge to subsystems that need
// to react to a session disappearing (e.g., notifications.Manager
// unregister). The callback receives the gone-actor's player id
// and runs outside Manager.mu. Safe to call once at startup;
// subsequent calls overwrite.
func (m *Manager) SetOnRemove(fn func(playerID string)) {
	m.mu.Lock()
	m.onRemove = fn
	m.mu.Unlock()
}

// SetMounts installs the mount lifecycle service used to dematerialize a
// departing actor's live mounts on full Remove (mounts.md §9, §10). Set once
// at startup from the composition root; nil-safe (no teardown when unset).
func (m *Manager) SetMounts(svc command.MountService) {
	m.mu.Lock()
	m.mounts = svc
	m.mu.Unlock()
}

// Add registers a freshly-logged-in actor across every index. The
// actor's manager back-reference is set so subsequent SetRoom calls
// can keep the by-room index in sync.
//
// A duplicate Add (same connection id already indexed) is a no-op
// guarded against double-registration: callers should never invoke
// it but the contract is documented and tested rather than silently
// producing diverging indices.
func (m *Manager) Add(a *connActor) {
	a.mu.Lock()
	a.manager = m
	var (
		lcName string
		roomID world.RoomID
	)
	if a.save != nil {
		lcName = strings.ToLower(a.save.Name)
	}
	if a.room != nil {
		roomID = a.room.ID
	}
	a.mu.Unlock()
	pid := a.playerID
	acct := a.accountID

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.byConn[a.id]; exists {
		return
	}
	m.byConn[a.id] = a
	if pid != "" {
		m.byPlayerID[pid] = a
	}
	if lcName != "" {
		m.byName[lcName] = a
	}
	if acct != "" {
		m.byAccount[acct] = append(m.byAccount[acct], a)
	}
	if roomID != "" && pid != "" {
		m.addToRoomLocked(roomID, pid, a)
	}
}

// Remove clears the actor from every index. Safe to call multiple
// times; absent entries are ignored.
//
// The room index is resolved from roomByPID under the manager lock,
// NOT from a.room — a.room may have already been mutated by a
// concurrent SetRoom whose moveRoom call has not yet landed. Using
// the manager-owned mapping makes Remove and moveRoom commute under
// the write lock.
func (m *Manager) Remove(a *connActor) {
	a.mu.Lock()
	var lcName string
	if a.save != nil {
		lcName = strings.ToLower(a.save.Name)
	}
	a.mu.Unlock()
	pid := a.playerID
	acct := a.accountID

	m.mu.Lock()
	delete(m.byConn, a.id)
	if pid != "" {
		delete(m.byPlayerID, pid)
		if cur, ok := m.roomByPID[pid]; ok {
			m.removeFromRoomLocked(cur, pid)
		}
	}
	if lcName != "" {
		// Only delete if it still points at this actor — guards
		// against a session that was already taken over.
		if cur, ok := m.byName[lcName]; ok && cur == a {
			delete(m.byName, lcName)
		}
	}
	if acct != "" {
		list := m.byAccount[acct]
		out := list[:0]
		for _, e := range list {
			if e != a {
				out = append(out, e)
			}
		}
		if len(out) == 0 {
			delete(m.byAccount, acct)
		} else {
			m.byAccount[acct] = out
		}
	}
	cb := m.onRemove
	mounts := m.mounts
	m.mu.Unlock()

	// Dematerialize the departing actor's live mounts (mounts.md §9, §10):
	// a logged-out owner's retrieved mount resolves to its stabled record, so
	// the live creature leaves the world rather than standing orphaned (and
	// duplicating when the owner returns and retrieves it again). Ownership
	// (the save record) is untouched. Runs outside m.mu — Dematerialize touches
	// the entity store / placement, not the manager indices.
	if mounts != nil {
		// drainLiveMounts clears the set atomically, so a concurrent stable
		// verb can't double-remove the same creature (it finds nothing left).
		for _, id := range a.drainLiveMounts() {
			mounts.Dematerialize(context.Background(), id)
		}
	}

	// Fire the post-remove callback outside the manager lock so
	// the callback can re-enter Manager (e.g., to look up the
	// player by name) without deadlocking, and so per-callback
	// I/O does not extend the index-scrub critical section.
	if cb != nil && pid != "" {
		cb(pid)
	}
}

// Count returns the number of actors indexed by connection id.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.byConn)
}

// playingActors snapshots every registered actor (playing + link-dead-
// within-window) as pointers under the manager read lock. The Manager only
// holds post-login actors — Add runs after phase=Playing — so login and
// character-creation sessions are excluded by construction (who §4 v1).
// Keyed by player id, so a single character is returned once even across a
// reconnect. Per-actor fields are read by the caller under each actor's own
// lock, never while holding m.mu (mirrors PlayersInRoom's discipline).
func (m *Manager) playingActors() []*connActor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*connActor, 0, len(m.byPlayerID))
	for _, a := range m.byPlayerID {
		out = append(out, a)
	}
	return out
}

// GetByName returns the session for the named player (case-insensitive)
// and whether one is online.
func (m *Manager) GetByName(name string) (*connActor, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.byName[strings.ToLower(strings.TrimSpace(name))]
	return s, ok
}

// GetByPlayerID returns the session for a player id, if online.
func (m *Manager) GetByPlayerID(id string) (*connActor, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.byPlayerID[id]
	return s, ok
}

// CombatantByPlayerID returns the live combat.Combatant for an online
// player id, used by the combat.Locator adapter wired in main. Returns
// (nil, false) when the player is not online — the round loop's
// §4.1 "missing target → disengage" branch then fires naturally.
//
// connActor satisfies combat.Combatant since M7.1; the public
// connActor type stays internal to this package, so the adapter
// boundary widens through this typed accessor rather than via an
// external type assertion.
func (m *Manager) CombatantByPlayerID(id string) (combat.Combatant, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.byPlayerID[id]
	if !ok {
		return nil, false
	}
	return s, true
}

// RoomOfPlayer returns the world room id of the online player with id,
// or ("", false) if the player is offline / mid-login (no room yet).
// Used by the combat.RoomLocator adapter in cmd/anothermud to power
// the spec §4.1 "different room → disengage" pre-flight check.
//
// Takes Manager.mu in read mode and then enters the actor's lock via
// Room(); the lock order is Manager.mu → actor.mu, matching every
// other adapter on this type.
func (m *Manager) RoomOfPlayer(id string) (world.RoomID, bool) {
	m.mu.RLock()
	s, ok := m.byPlayerID[id]
	m.mu.RUnlock()
	if !ok {
		return "", false
	}
	room := s.Room()
	if room == nil {
		return "", false
	}
	return room.ID, true
}

// GetByAccountID returns a snapshot of sessions bound to the account.
func (m *Manager) GetByAccountID(id string) []*connActor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	src := m.byAccount[id]
	if len(src) == 0 {
		return nil
	}
	out := make([]*connActor, len(src))
	copy(out, src)
	return out
}

// FindInRoom returns the session in roomID whose display name matches
// name (case-insensitive). Returns nil when no occupant matches. Used
// by targeted verbs (give, future tell/follow) that need to resolve a
// name argument against same-room presence.
//
// Lock order is Manager → actor (existing convention): we snapshot
// candidate pointers under the manager read lock, release it, and
// then call PlayerName() (which takes the actor lock) on the
// snapshot. Holding both locks at once would invert the established
// SendToRoom pattern.
func (m *Manager) FindInRoom(roomID world.RoomID, name string) *connActor {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return nil
	}
	m.mu.RLock()
	occupants := m.byRoom[roomID]
	snapshot := make([]*connActor, 0, len(occupants))
	for _, a := range occupants {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()
	for _, a := range snapshot {
		if strings.ToLower(a.PlayerName()) == want {
			return a
		}
	}
	return nil
}

// PlayerInfo is the read-only projection of a session that
// PlayersInRoom hands out. M8.3 grew the surface from (id, name)
// to include Tags so the disposition evaluator can match on racial
// flags (closes the M6.5 deferred "players have no Tags field yet"
// note). Future projections (alignment, class, level) extend this.
type PlayerInfo struct {
	ID        string
	Name      string
	Tags      []string
	Alignment int    // M8.5 integer alignment for disposition matching
	Bucket    string // M8.5 canonical bucket name ("evil"/"neutral"/"good")
}

// bucketFromTag translates an actor's mirrored alignment tag to
// the canonical bucket name used in PlayerView (spec
// progression.md §6.2). Empty input means the alignment manager
// has never touched this actor — disposition matching treats the
// resulting view as alignment-unknown.
func bucketFromTag(tag string) string {
	switch tag {
	case progression.TagAlignmentEvil:
		return string(progression.BucketEvil)
	case progression.TagAlignmentGood:
		return string(progression.BucketGood)
	case progression.TagAlignmentNeutral:
		return string(progression.BucketNeutral)
	default:
		return ""
	}
}

// PlayersInRoom returns a snapshot of every session currently in
// roomID as PlayerInfo. Designed for read-only consumers
// (disposition evaluator, future scent / tracking systems) that
// don't need full actor access.
//
// Result ordering is unspecified. Mirrors FindInRoom's lock
// discipline: snapshot pointers under the manager read lock,
// release it, then read per-actor fields outside the lock — each
// per-actor read takes the actor's own mutex.
func (m *Manager) PlayersInRoom(roomID world.RoomID) []PlayerInfo {
	m.mu.RLock()
	occupants := m.byRoom[roomID]
	snapshot := make([]*connActor, 0, len(occupants))
	for _, a := range occupants {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()
	out := make([]PlayerInfo, 0, len(snapshot))
	for _, a := range snapshot {
		out = append(out, PlayerInfo{
			ID:        a.PlayerID(),
			Name:      a.PlayerName(),
			Tags:      a.Tags(),
			Alignment: a.Alignment(),
			Bucket:    bucketFromTag(a.AlignmentTag()),
		})
	}
	return out
}

// roomConnActors snapshots the live connActors in roomID under the
// manager lock. Backs managerLocator.PlayersInRoom (the M17.2d₄
// command-layer player enumeration); returns the concrete *connActor
// so manager.go need not import internal/command.
func (m *Manager) roomConnActors(roomID world.RoomID) []*connActor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	occupants := m.byRoom[roomID]
	out := make([]*connActor, 0, len(occupants))
	for _, a := range occupants {
		out = append(out, a)
	}
	return out
}

// SendToRoom delivers text to every session in roomID, excluding any
// session whose player id appears in excludePlayerIDs. Snapshots the
// recipient list under the read lock and then writes outside the lock
// so Write callbacks can take their own actor mutexes safely.
func (m *Manager) SendToRoom(ctx context.Context, roomID world.RoomID, text string, excludePlayerIDs ...string) {
	excl := make(map[string]struct{}, len(excludePlayerIDs))
	for _, p := range excludePlayerIDs {
		excl[p] = struct{}{}
	}
	m.mu.RLock()
	occupants := m.byRoom[roomID]
	snapshot := make([]*connActor, 0, len(occupants))
	for pid, a := range occupants {
		if _, skip := excl[pid]; skip {
			continue
		}
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		if err := a.Write(ctx, text); err != nil {
			logging.From(ctx).Debug("SendToRoom: write failed",
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
		}
	}
}

// SendToPlayer delivers text to the named player if online. Returns
// true when delivered. Name match is case-insensitive.
func (m *Manager) SendToPlayer(ctx context.Context, name, text string) bool {
	s, ok := m.GetByName(name)
	if !ok {
		return false
	}
	if err := s.Write(ctx, text); err != nil {
		logging.From(ctx).Debug("SendToPlayer: write failed",
			slog.String("player", name),
			slog.Any("err", err))
		return false
	}
	return true
}

// SendToAll delivers text to every active session, excluding any
// player id in excludePlayerIDs. Snapshots recipients under the read
// lock — without touching actor mutexes — and writes outside the
// lock so Write callbacks cannot deadlock against future Manager
// callers that take the actor lock.
// OnlinePlayers returns a fresh entity-id → canonical-name map of
// every player currently in the byConn index. Used by the M13.6
// channel-subscribers adapter as the "every online player is
// subscribed" v1 stand-in. Canonical name matches what
// player.CanonicalName produces so it aligns with the per-player
// save-directory addressing convention.
func (m *Manager) OnlinePlayers() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]string, len(m.byConn))
	for _, a := range m.byConn {
		if a.playerID == "" {
			continue
		}
		// byName already holds the canonical (lowercased) form.
		// Pull directly from the actor's save record to avoid a
		// second map walk.
		a.mu.Lock()
		var name string
		if a.save != nil {
			name = strings.ToLower(a.save.Name)
		}
		a.mu.Unlock()
		if name == "" {
			continue
		}
		out[a.playerID] = name
	}
	return out
}

func (m *Manager) SendToAll(ctx context.Context, text string, excludePlayerIDs ...string) {
	excl := make(map[string]struct{}, len(excludePlayerIDs))
	for _, p := range excludePlayerIDs {
		excl[p] = struct{}{}
	}
	m.mu.RLock()
	snapshot := make([]*connActor, 0, len(m.byConn))
	for _, a := range m.byConn {
		if _, skip := excl[a.playerID]; skip {
			continue
		}
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		if err := a.Write(ctx, text); err != nil {
			logging.From(ctx).Debug("SendToAll: write failed",
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
		}
	}
}

// moveRoom updates the per-room index to reflect that the actor is
// now in newID. The previous room is read from the manager-owned
// roomByPID index, not from a.room — that mapping is the authoritative
// record of where each player currently lives in the broadcast index.
//
// Race guard: if Remove ran between SetRoom releasing the actor lock
// and moveRoom acquiring the manager write lock, the actor is no
// longer in byConn — performing the add anyway would orphan it in
// byRoom and leak writes to a disconnected session.
func (m *Manager) moveRoom(a *connActor, pid string, _ world.RoomID, newID world.RoomID) {
	if pid == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, live := m.byConn[a.id]; !live {
		return
	}
	if cur, ok := m.roomByPID[pid]; ok && cur != newID {
		m.removeFromRoomLocked(cur, pid)
	}
	if newID != "" {
		m.addToRoomLocked(newID, pid, a)
	}
}

func (m *Manager) addToRoomLocked(roomID world.RoomID, pid string, a *connActor) {
	occ := m.byRoom[roomID]
	if occ == nil {
		occ = make(map[string]*connActor)
		m.byRoom[roomID] = occ
	}
	occ[pid] = a
	m.roomByPID[pid] = roomID
}

func (m *Manager) removeFromRoomLocked(roomID world.RoomID, pid string) {
	occ, ok := m.byRoom[roomID]
	if !ok {
		return
	}
	delete(occ, pid)
	if len(occ) == 0 {
		delete(m.byRoom, roomID)
	}
	if cur, ok := m.roomByPID[pid]; ok && cur == roomID {
		delete(m.roomByPID, pid)
	}
}

// RemoveConnectionOnly drops only the byConn index entry for the
// actor, leaving every other index (byPlayerID, byName, byAccount,
// byRoom) intact. Used by the link-dead transition (spec §7.2 step 4)
// so a returning login can still find the parked session.
//
// Safe to call multiple times; an actor whose conn-id is not in the
// index is a no-op.
func (m *Manager) RemoveConnectionOnly(a *connActor) {
	if a == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byConn, a.id)
}

// ReRegisterConnectionForSession installs a new conn-id mapping for an
// actor whose connection was just swapped (link-dead reconnect, spec
// §7.4 step 2). Updates the actor's id field under the manager lock so
// no observer sees a half-swapped pair.
//
// Returns ErrSessionGone if the actor is not present in any non-byConn
// index — this means a concurrent Remove (e.g. the cleanup sweep) tore
// the session down while the reconnect path was running. Returns nil
// on success.
func (m *Manager) ReRegisterConnectionForSession(a *connActor, newConnID string) error {
	if a == nil {
		return ErrSessionGone
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Liveness probe via byPlayerID: cleanup's Remove clears this
	// entry, so a pointer-identity mismatch (or missing entry) means
	// the actor was reaped between reattach and re-register.
	if cur, ok := m.byPlayerID[a.playerID]; !ok || cur != a {
		return ErrSessionGone
	}
	// Drop any stale byConn entry for the old id (defensive — the
	// link-dead transition should already have removed it). a.id is
	// still the OLD id here because reattach intentionally does not
	// mutate it; we own that write under the manager lock so the
	// (id, byConn) pair never goes out of sync.
	delete(m.byConn, a.id)
	a.id = newConnID
	m.byConn[newConnID] = a
	return nil
}

// AllLinkDeadSessions returns a snapshot of every actor currently in
// LinkDead phase. Used by the cleanup tick handler. Takes the actor
// lock per candidate to read the phase, so the result reflects a
// consistent point-in-time view even under concurrent mutation.
func (m *Manager) AllLinkDeadSessions() []*connActor {
	m.mu.RLock()
	// Iterate byPlayerID rather than byConn — link-dead actors have
	// had their byConn entry removed but remain in byPlayerID.
	snapshot := make([]*connActor, 0, len(m.byPlayerID))
	for _, a := range m.byPlayerID {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	out := snapshot[:0]
	for _, a := range snapshot {
		if a.isLinkDead() {
			out = append(out, a)
		}
	}
	return out
}

// SaveAll persists every tracked actor, isolating per-actor errors so
// one bad save does not abort the batch (persistence spec §6.3). Used
// by the autosave tick handler and by graceful shutdown.
func (m *Manager) SaveAll(ctx context.Context) {
	m.mu.RLock()
	// Union byConn and byPlayerID. byConn covers live sessions;
	// byPlayerID additionally covers link-dead actors whose byConn
	// entry was removed at link-dead transition (spec §7.2). The
	// dedup uses pointer identity since the same *connActor lives in
	// both maps when playing.
	seen := make(map[*connActor]struct{}, len(m.byConn)+len(m.byPlayerID))
	for _, a := range m.byConn {
		seen[a] = struct{}{}
	}
	for _, a := range m.byPlayerID {
		seen[a] = struct{}{}
	}
	snapshot := make([]*connActor, 0, len(seen))
	for a := range seen {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		if err := a.Persist(ctx); err != nil {
			logging.From(ctx).Warn("autosave: persist failed",
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
		}
	}
}

// DrainSustenance applies one drain tick (spec economy-survival §4.4)
// to every logged-in actor and emits a throttled hunger reminder to any
// player that has dropped below the Full tier. `now` is the current
// engine tick, used to gate the per-player reminder interval. This is
// the body of the sustenance-drain world-tick handler registered at the
// composition root at the config's DrainCadence; keeping it here (rather
// than inline in main.go) gives it direct access to the actor set and
// each actor's reminder-throttle state. No-op when svc is nil.
//
// The actor snapshot is the same byConn ∪ byPlayerID union SaveAll
// uses, so link-dead actors are drained too — their sustenance persists
// like everything else, and a reminder Write to a dead connection is a
// harmless no-op.
//
// Actors holding adminRole are skipped entirely: an admin never drains,
// so they stay Full and never have to deal with hunger or famine while
// administering. An empty adminRole disables the exemption (every actor
// drains), matching how the idle-sweep admin exemption is configured.
func (m *Manager) DrainSustenance(ctx context.Context, svc *economy.SustenanceService, adminRole string, now uint64) {
	if svc == nil {
		return
	}
	m.mu.RLock()
	seen := make(map[*connActor]struct{}, len(m.byConn)+len(m.byPlayerID))
	for _, a := range m.byConn {
		seen[a] = struct{}{}
	}
	for _, a := range m.byPlayerID {
		seen[a] = struct{}{}
	}
	snapshot := make([]*connActor, 0, len(seen))
	for a := range seen {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	interval := svc.Config().ReminderIntervalTicks
	for _, a := range snapshot {
		if adminRole != "" && a.HasRole(adminRole) {
			continue
		}
		_, tier := svc.Drain(a)
		if tier == economy.TierFull {
			continue
		}
		if msg := hungerReminder(tier); msg != "" && a.shouldRemindHunger(now, interval) {
			_ = a.Write(ctx, msg)
		}
	}
}

// BurnFuel applies one fuel-burn step to every lit fuel-burning light
// source carried or equipped by a logged-in actor (light-and-darkness
// §3.2). A source that gutters out (fuel reached zero) is made unlit by
// light.Burn; this sweep then notifies the holder and publishes
// light.source.extinguished. The room-light transition a gutter may
// cause is driven by the §6 transition subscriber off that event.
//
// v1 scope: only sources held by a logged-in actor (inventory ∪
// equipment) burn — the same actor-snapshot DrainSustenance uses. A lit
// source dropped on the ground keeps its lit state but does not burn
// down until carried again; burning room-loose sources would need a
// full entity-store scan every tick. No-op when store is nil.
func (m *Manager) BurnFuel(ctx context.Context, cfg light.FuelConfig, store *entities.Store, bus *eventbus.Bus) {
	if store == nil {
		return
	}
	m.mu.RLock()
	seen := make(map[*connActor]struct{}, len(m.byConn)+len(m.byPlayerID))
	for _, a := range m.byConn {
		seen[a] = struct{}{}
	}
	for _, a := range m.byPlayerID {
		seen[a] = struct{}{}
	}
	snapshot := make([]*connActor, 0, len(seen))
	for a := range seen {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		ids := a.Inventory()
		for _, id := range a.Equipment() {
			ids = append(ids, id)
		}
		var roomID world.RoomID
		if r := a.Room(); r != nil {
			roomID = r.ID
		}
		for _, id := range ids {
			e, ok := store.GetByID(id)
			if !ok {
				continue
			}
			it, ok := e.(*entities.ItemInstance)
			if !ok {
				continue
			}
			if _, guttered, _ := light.Burn(it, cfg.BurnAmount); !guttered {
				continue
			}
			_ = a.Write(ctx, fmt.Sprintf("%s gutters out and goes dark.", capitalizeFirst(it.Name())))
			if bus != nil {
				bus.Publish(ctx, eventbus.LightSourceExtinguished{
					SourceID: it.ID(),
					HolderID: entities.EntityID(a.PlayerID()),
					RoomID:   roomID,
				})
			}
		}
	}
}

// RoomSource is the minimal world lookup the light-transition driver
// needs: resolve a room id to its *world.Room. *world.World satisfies it.
type RoomSource interface {
	Room(id world.RoomID) (*world.Room, error)
}

// Light-transition messages (light-and-darkness §6). Hardcoded for v1;
// per-direction / per-crossing configurability (§6/§11) is deferred.
const (
	lightDarkenText   = "<subtle>The light fades and shadows close in around you.</subtle>"
	lightBrightenText = "<subtle>The light grows; the world brightens around you.</subtle>"
)

// LightTransitions notifies players whose effective light level crosses
// when the day/night period changes (light-and-darkness §6). For each
// occupied room it recomputes each player's level under the previous
// and new period (per-viewer: a darkvision viewer or torch-bearer may
// feel no transition a bare human does) and sends a darkening or
// brightening line only when the level actually crosses. A period change
// that does not move a viewer's level is silent for them; an empty room
// notifies no one. No-op when resolver or rooms is nil, or the periods
// match.
func (m *Manager) LightTransitions(ctx context.Context, resolver *light.Resolver, rooms RoomSource, items *entities.Store, placement *entities.Placement, prevPeriod, newPeriod string) {
	if resolver == nil || rooms == nil || prevPeriod == newPeriod {
		return
	}
	// Snapshot the occupied-room → actors map under the lock; the
	// per-viewer resolves + Writes happen outside it.
	m.mu.RLock()
	byRoom := make(map[world.RoomID][]*connActor, len(m.byRoom))
	for roomID, actors := range m.byRoom {
		if len(actors) == 0 {
			continue
		}
		list := make([]*connActor, 0, len(actors))
		for _, a := range actors {
			list = append(list, a)
		}
		byRoom[roomID] = list
	}
	m.mu.RUnlock()

	for roomID, actors := range byRoom {
		room, err := rooms.Room(roomID)
		if err != nil || room == nil {
			continue
		}
		for _, a := range actors {
			before := command.EffectiveLightForPeriod(resolver, room, a, items, placement, prevPeriod)
			after := command.EffectiveLightForPeriod(resolver, room, a, items, placement, newPeriod)
			if before == after {
				continue
			}
			if after < before {
				_ = a.Write(ctx, lightDarkenText)
			} else {
				_ = a.Write(ctx, lightBrightenText)
			}
		}
	}
}

// capitalizeFirst upper-cases the first byte of a short ASCII string so
// an item name ("a torch") reads cleanly at the start of a sentence.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}

// RegenTick heals every logged-in player by the composed regen amount
// (spec economy-survival §4.3 × §5.5 + §5.7): base × sustenance
// multiplier × rest multiplier, plus the room's healing_rate. This is
// the body of the vitals-regen world-tick handler registered at the
// composition root; it pays the M9 "real pools + regen" deferral. No-op
// when either service is nil.
//
// A player is skipped when dead (HP ≤ 0 — revival is the death system's
// job, not regen), already at full HP, or currently in combat (combat
// HP is the combat system's concern; passive regen must not fight it).
// The healed HP rides to disk on the next autosave via Persist's
// vitals sync, exactly like combat damage.
func (m *Manager) RegenTick(ctx context.Context, sustSvc *economy.SustenanceService, restSvc *economy.RestService, cfg economy.RegenConfig) {
	if sustSvc == nil || restSvc == nil {
		return
	}
	m.mu.RLock()
	seen := make(map[*connActor]struct{}, len(m.byConn)+len(m.byPlayerID))
	for _, a := range m.byConn {
		seen[a] = struct{}{}
	}
	for _, a := range m.byPlayerID {
		seen[a] = struct{}{}
	}
	snapshot := make([]*connActor, 0, len(seen))
	for a := range seen {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		if a.InCombat() {
			continue
		}
		// Dead actors get NO regen — HP or pools. Revival is the death
		// system's job, and a dead channeler must not passively refill the
		// Power pool between death and the revival event. This guard covers
		// the whole actor (the pre-refactor structure skipped dead actors
		// before any regen); a nil-vitals actor has no death concept and
		// falls through to pool regen.
		if a.vitals != nil {
			if cur, _ := a.vitals.Snapshot(); cur <= 0 {
				continue
			}
		}
		sustMult := sustSvc.GetRegenMultiplier(a.Sustenance())
		restMult := restSvc.GetRestMultiplier(economy.RestState(a.RestState()))

		// HP — skipped when already full; the room healing_rate adds only
		// to HP (§5.7).
		if a.vitals != nil {
			if cur, max := a.vitals.Snapshot(); cur < max {
				healingRate := 0
				if room := a.Room(); room != nil {
					healingRate = room.HealingRate
				}
				if amount := economy.RegenAmount(cfg.BaseHP, sustMult, restMult, healingRate); amount > 0 {
					a.vitals.Heal(amount)
				}
			}
		}

		// Resource pools regen independently of HP fullness — a full-HP
		// channeler with drained Power must still refill. pool.Restore caps
		// at max, so a full or zero-max pool (every non-channeler today)
		// no-ops. The restored value rides to disk via Persist's pool sync.
		a.regenPool(poolKindMana, economy.RegenAmount(cfg.BaseMana, sustMult, restMult, 0))
		a.regenPool(poolKindMovement, economy.RegenAmount(cfg.BaseMovement, sustMult, restMult, 0))
	}
}

// MadnessActor is the slice of a playing actor the saidin-taint tick needs
// (WoT S2 Phase 4+). The mechanic — when a man's madness decays, when it
// manifests, which condition it inflicts — is WoT-specific and lives in the
// composition root; this interface is the neutral seam the Manager hands each
// playing actor across.
type MadnessActor interface {
	PlayerID() string
	Gender() string
	Madness() int
	AddMadness(delta int) int
	HasFeat(featID string) bool
	Write(ctx context.Context, msg string) error
}

// MadnessTick invokes fn once per PLAYING actor (the saidin-taint tick, WoT S2
// Phase 4+). It only iterates — decay, the manifestation roll, and the
// condition application are the caller's (composition-root) business, keeping
// the WoT curse out of the session package. Mirrors RegenTick's
// snapshot-then-iterate shape so a callback that logs an actor out cannot
// mutate the map mid-range.
func (m *Manager) MadnessTick(ctx context.Context, fn func(ctx context.Context, a MadnessActor)) {
	if fn == nil {
		return
	}
	for _, a := range m.playingActors() {
		fn(ctx, a)
	}
}

// hungerReminder returns the nudge message for a below-Full tier (spec
// §4.4 "hunger reminder messages"). Full returns "" (no reminder).
func hungerReminder(tier economy.Tier) string {
	switch tier {
	case economy.TierHungry:
		return "You are feeling hungry."
	case economy.TierFamished:
		return "You are famished and need to eat something soon!"
	default:
		return ""
	}
}

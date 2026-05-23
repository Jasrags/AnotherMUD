package session

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

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
	defer m.mu.Unlock()
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
}

// Count returns the number of actors indexed by connection id.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.byConn)
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

// SaveAll persists every tracked actor, isolating per-actor errors so
// one bad save does not abort the batch (persistence spec §6.3). Used
// by the autosave tick handler and by graceful shutdown.
func (m *Manager) SaveAll(ctx context.Context) {
	m.mu.RLock()
	snapshot := make([]*connActor, 0, len(m.byConn))
	for _, a := range m.byConn {
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

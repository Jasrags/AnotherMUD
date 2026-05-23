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
	byName     map[string]*connActor               // key: lowercased name
	byAccount  map[string][]*connActor             // key: account id
	byRoom     map[world.RoomID]map[string]*connActor
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{
		byConn:     make(map[string]*connActor),
		byPlayerID: make(map[string]*connActor),
		byName:     make(map[string]*connActor),
		byAccount:  make(map[string][]*connActor),
		byRoom:     make(map[world.RoomID]map[string]*connActor),
	}
}

// Add registers a freshly-logged-in actor across every index. The
// actor's manager back-reference is set so subsequent SetRoom calls
// can keep the by-room index in sync.
func (m *Manager) Add(a *connActor) {
	a.mu.Lock()
	a.manager = m
	var (
		pid, acct, lcName string
		roomID            world.RoomID
	)
	if a.save != nil {
		pid = a.save.ID
		acct = a.save.AccountID
		lcName = strings.ToLower(a.save.Name)
	}
	if a.room != nil {
		roomID = a.room.ID
	}
	a.mu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.byConn[a.id] = a
	if pid != "" {
		m.byPlayerID[pid] = a
	}
	if lcName != "" {
		m.byName[lcName] = a
	}
	if acct != "" {
		// Dedup: don't append twice for the same actor.
		list := m.byAccount[acct]
		found := false
		for _, e := range list {
			if e == a {
				found = true
				break
			}
		}
		if !found {
			m.byAccount[acct] = append(list, a)
		}
	}
	if roomID != "" && pid != "" {
		m.addToRoomLocked(roomID, pid, a)
	}
}

// Remove clears the actor from every index. Safe to call multiple
// times; absent entries are ignored.
func (m *Manager) Remove(a *connActor) {
	a.mu.Lock()
	var (
		pid, acct, lcName string
		roomID            world.RoomID
	)
	if a.save != nil {
		pid = a.save.ID
		acct = a.save.AccountID
		lcName = strings.ToLower(a.save.Name)
	}
	if a.room != nil {
		roomID = a.room.ID
	}
	a.mu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byConn, a.id)
	if pid != "" {
		delete(m.byPlayerID, pid)
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
	if roomID != "" && pid != "" {
		m.removeFromRoomLocked(roomID, pid)
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
// player id in excludePlayerIDs.
func (m *Manager) SendToAll(ctx context.Context, text string, excludePlayerIDs ...string) {
	excl := make(map[string]struct{}, len(excludePlayerIDs))
	for _, p := range excludePlayerIDs {
		excl[p] = struct{}{}
	}
	m.mu.RLock()
	snapshot := make([]*connActor, 0, len(m.byConn))
	for _, a := range m.byConn {
		a.mu.Lock()
		pid := ""
		if a.save != nil {
			pid = a.save.ID
		}
		a.mu.Unlock()
		if _, skip := excl[pid]; skip {
			continue
		}
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		_ = a.Write(ctx, text)
	}
}

// moveRoom updates the per-room index when an actor's room changes.
// Called by connActor.SetRoom after it has released the actor's lock.
// Empty oldID / newID is tolerated (initial placement, departure).
func (m *Manager) moveRoom(a *connActor, pid string, oldID, newID world.RoomID) {
	if pid == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if oldID != "" {
		m.removeFromRoomLocked(oldID, pid)
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

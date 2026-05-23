package session

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Manager tracks the set of logged-in connActors so autosave and
// shutdown sweeps can iterate them. M3 keeps the surface area tiny —
// the full session-lifecycle.md SessionManager lands in M4.
type Manager struct {
	mu      sync.Mutex
	actors  map[*connActor]struct{}
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{actors: make(map[*connActor]struct{})}
}

// Add registers a newly-logged-in actor.
func (m *Manager) Add(a *connActor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actors[a] = struct{}{}
}

// Remove deregisters an actor (typically on disconnect).
func (m *Manager) Remove(a *connActor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.actors, a)
}

// Count returns the number of currently-tracked actors. Used by tests
// and metrics.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.actors)
}

// SaveAll persists every tracked actor, isolating per-actor errors so
// one bad save does not abort the batch (persistence spec §6.3). Used
// by the autosave tick handler and by graceful shutdown.
func (m *Manager) SaveAll(ctx context.Context) {
	m.mu.Lock()
	snapshot := make([]*connActor, 0, len(m.actors))
	for a := range m.actors {
		snapshot = append(snapshot, a)
	}
	m.mu.Unlock()

	for _, a := range snapshot {
		if err := a.Persist(ctx); err != nil {
			logging.From(ctx).Warn("autosave: persist failed",
				slog.String("player", a.PlayerName()),
				slog.Any("err", err))
		}
	}
}

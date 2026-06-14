package progression

import (
	"strings"
	"sync"
)

// DefaultActionQueueLimit caps the per-entity queue depth. Spec
// abilities-and-effects §9 calls out queue-depth bounds as an open
// question; we pick a defensive ceiling that comfortably exceeds
// human queueing intent (a player typing four-or-five queued moves)
// while bounding a misbehaving client. Overridable at construction
// via ActionQueueConfig.Limit.
const DefaultActionQueueLimit = 16

// QueuedAction is one entry in an entity's action queue (spec
// abilities-and-effects §4.1). At minimum: an ability id. Optionally
// an explicit target entity id; resolution falls back to current
// combat target / self when empty (§4.4).
//
// Value-typed; the queue stores copies so callers may freely mutate
// the prototype they passed to Push.
type QueuedAction struct {
	AbilityID      string
	TargetEntityID string
	// Overchannel marks a deliberate draw past the caster's safe reserve
	// (WoT S2 — the `overchannel` verb). It tells validation to allow a
	// spell whose cost exceeds the reserve-to-begin threshold (which would
	// otherwise fizzle as insufficient_resources) and flags the resolved
	// cast as risky, so the host can exact the overchannel consequence
	// (a Fortitude save + condition cascade). False = an ordinary cast.
	Overchannel bool
}

// ActionQueueConfig is the host-side configuration for
// ActionQueueManager. Construction-time only.
type ActionQueueConfig struct {
	// Limit caps per-entity queue depth. <=0 falls back to
	// DefaultActionQueueLimit.
	Limit int
}

// ActionQueueManager tracks per-entity ordered action queues (spec
// abilities-and-effects §4.1). Process-wide; per-entity state lives
// in one id-keyed map guarded by an RWMutex. Mirrors
// ProficiencyManager / EffectManager shape.
//
// State is in-memory only. Logout calls Drop. Persistence (spec §4.1
// "naturally snapshotted by save/load") is deferred until the
// action queue is observably populated across a disconnect — a
// player with one queued action at logout currently loses it on
// reconnect, which matches mid-pulse cancellation semantics players
// already experience for link-dead during combat.
type ActionQueueManager struct {
	cfg ActionQueueConfig

	mu     sync.RWMutex
	queues map[string][]QueuedAction
}

// NewActionQueueManager returns an empty manager.
func NewActionQueueManager(cfg ActionQueueConfig) *ActionQueueManager {
	if cfg.Limit <= 0 {
		cfg.Limit = DefaultActionQueueLimit
	}
	return &ActionQueueManager{
		cfg:    cfg,
		queues: make(map[string][]QueuedAction),
	}
}

// Push appends action to entityID's queue. Returns true on success,
// false when the queue is at capacity (per spec §9 open question we
// reject rather than drop the front entry so the player sees a
// fizzle-class outcome instead of silent loss).
//
// Empty entityID or empty AbilityID are silent no-ops returning
// false — both indicate caller bugs the queue manager declines to
// paper over.
func (m *ActionQueueManager) Push(entityID string, action QueuedAction) bool {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	aid := strings.ToLower(strings.TrimSpace(action.AbilityID))
	if eid == "" || aid == "" {
		return false
	}
	action.AbilityID = aid
	action.TargetEntityID = strings.ToLower(strings.TrimSpace(action.TargetEntityID))
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.queues[eid]) >= m.cfg.Limit {
		return false
	}
	m.queues[eid] = append(m.queues[eid], action)
	return true
}

// Peek returns the front entry without removing it. The second
// return is false when the queue is empty.
func (m *ActionQueueManager) Peek(entityID string) (QueuedAction, bool) {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return QueuedAction{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	q := m.queues[eid]
	if len(q) == 0 {
		return QueuedAction{}, false
	}
	return q[0], true
}

// Pop removes and returns the front entry. The second return is
// false when the queue is empty. When the queue becomes empty the
// map slot is deleted (matches spec §4.2 "If the queue ends up
// empty the property is cleared").
func (m *ActionQueueManager) Pop(entityID string) (QueuedAction, bool) {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return QueuedAction{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	q := m.queues[eid]
	if len(q) == 0 {
		return QueuedAction{}, false
	}
	head := q[0]
	if len(q) == 1 {
		delete(m.queues, eid)
	} else {
		// Copy tail into a fresh slice so the underlying array isn't
		// retained as the queue shrinks across many Pops.
		rest := make([]QueuedAction, len(q)-1)
		copy(rest, q[1:])
		m.queues[eid] = rest
	}
	return head, true
}

// Len returns the current queue depth for entityID.
// PendingEntities returns the entity ids that currently hold at least
// one queued action, in unspecified order. The out-of-combat ability
// drain uses it to find idle casters whose queue the combat heartbeat
// (which only services engaged combatants) won't otherwise pump.
func (m *ActionQueueManager) PendingEntities() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.queues))
	for id, q := range m.queues {
		if len(q) > 0 {
			out = append(out, id)
		}
	}
	return out
}

func (m *ActionQueueManager) Len(entityID string) int {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.queues[eid])
}

// Snapshot returns a deep copy of entityID's current queue. Nil
// when the queue is empty. Used for read-only inspection (renderers,
// tests).
func (m *ActionQueueManager) Snapshot(entityID string) []QueuedAction {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	q := m.queues[eid]
	if len(q) == 0 {
		return nil
	}
	out := make([]QueuedAction, len(q))
	copy(out, q)
	return out
}

// Drop clears entityID's queue. Returns the count of dropped
// entries. Used at logout / death so the manager's working set
// stays bounded to currently-connected entities.
func (m *ActionQueueManager) Drop(entityID string) int {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.queues[eid])
	if n > 0 {
		delete(m.queues, eid)
	}
	return n
}

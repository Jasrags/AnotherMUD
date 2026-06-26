package reputation

import (
	"context"
	"sync"
	"time"
)

// Entity is the host-side adapter the Manager drives (mirrors faction.Entity /
// progression.AlignmentEntity). The composition root implements it for players
// (connActor). Same concurrency contract as faction: the Manager holds no
// per-entity lock across these callbacks, and adapters MUST NOT call back into
// the Manager's mutating methods from inside them.
type Entity interface {
	// ID is the bare entity id (no combat prefix) used for history keys and
	// event payloads.
	ID() string
	// Renown returns the entity's current stored renown score. An untouched
	// character reads at the configured Starting value (the persistence layer
	// seeds it), so there is no present/absent distinction (unlike faction's
	// per-faction bag — renown is one always-present axis).
	Renown() int
	// SetRenown writes the score. The Manager always passes a value already
	// clamped to the configured bounds.
	SetRenown(value int)
	// SetTierTag installs tierTag, first removing any existing tag with
	// TierTagPrefix (so exactly one tier tag at a time). An empty tierTag clears
	// the renown tier tag.
	SetTierTag(tierTag string)
	// HasTag reports whether tag is present (the admin-bypass check).
	HasTag(tag string) bool
}

// Sink is the event seam (reputation.md §9), bridged to eventbus by the
// composition root. Methods fire synchronously inside Shift.
type Sink interface {
	// OnShiftCheck publishes the cancellable pre-event (§4 step 2). Returns the
	// possibly-rewritten delta and whether a listener cancelled.
	OnShiftCheck(ctx context.Context, entityID, reason string, suggestedDelta int) (finalDelta int, cancelled bool)
	// OnShifted publishes the post-fact shift event (§4 step 3).
	OnShifted(ctx context.Context, entityID, reason string, oldValue, newValue, actualDelta int, tierChanged bool)
	// OnTierChanged publishes the tier-transition event (§4 step 3), in addition
	// to OnShifted, not in place of it.
	OnTierChanged(ctx context.Context, entityID, oldTier, newTier string)
}

type nopSink struct{}

func (nopSink) OnShiftCheck(_ context.Context, _, _ string, d int) (int, bool) { return d, false }
func (nopSink) OnShifted(context.Context, string, string, int, int, int, bool) {}
func (nopSink) OnTierChanged(context.Context, string, string, string)          {}

// HistoryEntry is one row in the per-entity bounded ring buffer (reputation.md
// §4 / §10).
type HistoryEntry struct {
	At       time.Time
	Delta    int
	Reason   string
	NewValue int
	NewTier  string
}

// ShiftResult is the structured Shift return (mirrors faction.ShiftResult).
type ShiftResult struct {
	Cancelled     bool
	OldValue      int
	NewValue      int
	RequestDelta  int
	ResolvedDelta int
	AppliedDelta  int
	OldTier       string
	NewTier       string
	TierChanged   bool
}

// Manager owns the reputation.md §4 operations over a single shared Config. The
// per-entity history lives here. Safe for concurrent use across distinct
// entities; per-entity Shift serialization is the caller's responsibility (same
// contract as faction.Manager / AlignmentManager).
type Manager struct {
	cfg Config
	now func() time.Time

	mu      sync.Mutex
	history map[string][]HistoryEntry

	sink Sink
}

// NewManager returns a manager over cfg (normalized so a zero-value Config is
// safe). now defaults to time.Now; tests pass a fixed clock for deterministic
// history timestamps. A nil sink discards events.
func NewManager(cfg Config, sink Sink, now func() time.Time) *Manager {
	if sink == nil {
		sink = nopSink{}
	}
	if now == nil {
		now = time.Now
	}
	return &Manager{cfg: cfg.normalize(), now: now, sink: sink, history: make(map[string][]HistoryEntry)}
}

// Config returns the manager's (normalized) configuration.
func (m *Manager) Config() Config { return m.cfg }

// Get returns the entity's current renown score. Nil-safe.
func (m *Manager) Get(e Entity) int {
	if e == nil {
		return 0
	}
	return e.Renown()
}

// Tier returns the entity's current tier name AND syncs the tier tag mirror
// (idempotent — reputation.md §3/§4). Nil-safe.
func (m *Manager) Tier(e Entity) string {
	if e == nil {
		return ""
	}
	tier := m.cfg.TierOf(e.Renown())
	e.SetTierTag(TierTag(tier))
	return tier
}

// Set is the admin / scripted / creation-seed override (reputation.md §4):
// clamps, writes, syncs the tier tag, emits NO events and appends NO history.
// Nil-safe.
func (m *Manager) Set(_ context.Context, e Entity, value int, _ string) {
	if e == nil {
		return
	}
	clamped := m.cfg.clamp(value)
	e.SetRenown(clamped)
	e.SetTierTag(TierTag(m.cfg.TierOf(clamped)))
}

// Shift is the gameplay path (reputation.md §4). Order mirrors faction.Shift:
// nil-guard → admin bypass → cancellable check → resolve delta → apply.
func (m *Manager) Shift(ctx context.Context, e Entity, delta int, reason string) ShiftResult {
	if e == nil {
		return ShiftResult{}
	}
	if e.HasTag(AdminRoleTag) {
		return ShiftResult{}
	}
	finalDelta, cancelled := m.sink.OnShiftCheck(ctx, e.ID(), reason, delta)
	if cancelled {
		return ShiftResult{Cancelled: true, RequestDelta: delta}
	}
	if finalDelta == 0 {
		return ShiftResult{RequestDelta: delta, ResolvedDelta: 0}
	}
	oldValue := e.Renown()
	newValue := m.cfg.clamp(oldValue + finalDelta)
	actual := newValue - oldValue
	if actual == 0 {
		return ShiftResult{RequestDelta: delta, ResolvedDelta: finalDelta, OldValue: oldValue, NewValue: newValue}
	}
	oldTier := m.cfg.TierOf(oldValue)
	newTier := m.cfg.TierOf(newValue)
	tierChanged := oldTier != newTier

	e.SetRenown(newValue)
	e.SetTierTag(TierTag(newTier))
	m.appendHistory(e.ID(), HistoryEntry{
		At:       m.now(),
		Delta:    actual,
		Reason:   reason,
		NewValue: newValue,
		NewTier:  newTier,
	})

	m.sink.OnShifted(ctx, e.ID(), reason, oldValue, newValue, actual, tierChanged)
	if tierChanged {
		m.sink.OnTierChanged(ctx, e.ID(), oldTier, newTier)
	}
	return ShiftResult{
		OldValue: oldValue, NewValue: newValue,
		RequestDelta: delta, ResolvedDelta: finalDelta, AppliedDelta: actual,
		OldTier: oldTier, NewTier: newTier, TierChanged: tierChanged,
	}
}

// Check is the recognition primitive (reputation.md §6): "is this character
// recognized here?" — a roll of renown MAGNITUDE + dieRoll against a
// content-defined difficulty. A renown of zero (Unknown) auto-fails: an unknown
// person cannot be recognized. dieRoll is supplied by the caller (the host
// rolls) so the primitive stays pure and testable. Nil-safe (→ false).
func (m *Manager) Check(e Entity, dieRoll, difficulty int) bool {
	if e == nil {
		return false
	}
	return Recognized(e.Renown(), dieRoll, difficulty)
}

// Recognized is the pure recognition rule (reputation.md §6): a renown MAGNITUDE
// of zero is never recognized (an unknown person cannot be); otherwise
// `|renown| + dieRoll >= difficulty`. The reusable primitive shared by
// Manager.Check (which feeds it base renown) and effective-renown consumers like
// the look-recognition surface — which must read EFFECTIVE renown (base + Fame +
// worn signifiers), so they call this directly rather than through an Entity.
func Recognized(renown, dieRoll, difficulty int) bool {
	mag := renown
	if mag < 0 {
		mag = -mag
	}
	if mag == 0 {
		return false
	}
	return mag+dieRoll >= difficulty
}

// History returns a copy of the entity's history ring (oldest first).
func (m *Manager) History(entityID string) []HistoryEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.history[entityID]
	if len(src) == 0 {
		return nil
	}
	out := make([]HistoryEntry, len(src))
	copy(out, src)
	return out
}

// appendHistory adds entry to entityID's ring, FIFO-trimming to the configured
// capacity.
func (m *Manager) appendHistory(entityID string, entry HistoryEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ring := append(m.history[entityID], entry)
	if len(ring) > m.cfg.HistoryCapacity {
		drop := len(ring) - m.cfg.HistoryCapacity
		ring = append(ring[:0], ring[drop:]...)
	}
	m.history[entityID] = ring
}

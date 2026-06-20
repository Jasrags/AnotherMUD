package faction

import (
	"context"
	"sync"
	"time"
)

// AdminRoleTag is the role tag whose presence makes an entity faction-immune
// (faction.md §4 Shift step 2 — mirrors progression §6.4 step 2).
const AdminRoleTag = "admin"

// RankTag is the rank-mirror tag format (faction.md §3.3 / §9):
// faction:<factionId>:<rank>. Centralized so the format is named one place.
func RankTag(factionID, rank string) string {
	if rank == "" {
		return ""
	}
	return "faction:" + factionID + ":" + rank
}

// RankTagPrefix is the per-faction prefix an Entity adapter strips when
// installing a new rank tag (so it removes the prior tag for THIS faction
// only — faction.md §3.3 "setting the new one removes the prior").
func RankTagPrefix(factionID string) string {
	return "faction:" + factionID + ":"
}

// Entity is the host-side adapter the Manager drives (mirrors
// progression.AlignmentEntity). The composition root implements it for players
// (connActor). Same concurrency contract as alignment: the Manager holds no
// per-entity lock across these callbacks, and adapters MUST NOT call back into
// the Manager's mutating methods from inside them.
type Entity interface {
	// ID is the bare entity id (no combat prefix) used for history keys and
	// event payloads.
	ID() string
	// Standing returns the entity's stored standing with factionID and whether
	// an entry is present. Absent → the Manager substitutes the faction's
	// starting standing (faction.md §3.1).
	Standing(factionID string) (value int, present bool)
	// SetStanding writes the standing for factionID. The Manager always passes
	// a value already clamped to the faction's bounds.
	SetStanding(factionID string, value int)
	// SetRankTag installs rankTag, first removing any existing tag with
	// RankTagPrefix(factionID) (so exactly one rank tag per faction). An empty
	// rankTag clears the faction's rank tag.
	SetRankTag(factionID, rankTag string)
	// HasTag reports whether tag is present (the admin-bypass check).
	HasTag(tag string) bool
}

// Sink is the event seam (faction.md §7), bridged to eventbus by the
// composition root. Methods fire synchronously inside Shift.
type Sink interface {
	// OnShiftCheck publishes the cancellable pre-event (§4 step 3). Returns the
	// possibly-rewritten delta and whether a listener cancelled.
	OnShiftCheck(ctx context.Context, entityID, factionID, reason string, suggestedDelta int) (finalDelta int, cancelled bool)
	// OnShifted publishes the post-fact shift event (§4.1 step 5).
	OnShifted(ctx context.Context, entityID, factionID, reason string, oldValue, newValue, actualDelta int, rankChanged bool)
	// OnRankChanged publishes the rank-transition event (§4.1 step 6), in
	// addition to OnShifted, not in place of it.
	OnRankChanged(ctx context.Context, entityID, factionID, oldRank, newRank string)
}

type nopSink struct{}

func (nopSink) OnShiftCheck(_ context.Context, _, _, _ string, d int) (int, bool) { return d, false }
func (nopSink) OnShifted(context.Context, string, string, string, int, int, int, bool) {
}
func (nopSink) OnRankChanged(context.Context, string, string, string, string) {}

// HistoryEntry is one row in the per-entity COMBINED ring buffer (faction.md
// §3.4 — a single list across all factions, each record carrying its faction
// id, bounds total growth regardless of how many factions a character touches).
type HistoryEntry struct {
	At        time.Time
	FactionID string
	Delta     int
	Reason    string
	NewValue  int
	NewRank   string
}

// ShiftResult is the structured Shift return (mirrors alignment.ShiftResult).
type ShiftResult struct {
	Cancelled     bool
	OldValue      int
	NewValue      int
	RequestDelta  int
	ResolvedDelta int
	AppliedDelta  int
	OldRank       string
	NewRank       string
	RankChanged   bool
}

// Manager owns the faction.md §4 operations over the Registry. The combined
// per-entity history lives here. Safe for concurrent use across distinct
// entities; per-entity Shift serialization is the caller's responsibility
// (same contract as AlignmentManager).
type Manager struct {
	reg *Registry
	now func() time.Time

	mu      sync.Mutex
	history map[string][]HistoryEntry

	sink Sink
}

// NewManager returns a manager over reg. now defaults to time.Now; tests pass
// a fixed clock for deterministic history timestamps. A nil sink discards
// events.
func NewManager(reg *Registry, sink Sink, now func() time.Time) *Manager {
	if sink == nil {
		sink = nopSink{}
	}
	if now == nil {
		now = time.Now
	}
	return &Manager{reg: reg, now: now, sink: sink, history: make(map[string][]HistoryEntry)}
}

// Registry returns the underlying registry (for consumers resolving a faction).
func (m *Manager) Registry() *Registry { return m.reg }

// standingOf reads the entity's standing with def, substituting the faction's
// starting standing for an absent entry (faction.md §3.1).
func (m *Manager) standingOf(e Entity, def *Definition) int {
	if v, ok := e.Standing(def.ID); ok {
		return v
	}
	return def.Starting
}

// Get returns the entity's current standing with def (the starting standing
// for an untouched faction). Nil-safe.
func (m *Manager) Get(e Entity, def *Definition) int {
	if e == nil || def == nil {
		return 0
	}
	return m.standingOf(e, def)
}

// Rank returns the entity's current rank name with def AND syncs the rank tag
// mirror (idempotent — faction.md §4 Rank). Nil-safe.
func (m *Manager) Rank(e Entity, def *Definition) string {
	if e == nil || def == nil {
		return ""
	}
	rank := def.RankOf(m.standingOf(e, def))
	e.SetRankTag(def.ID, RankTag(def.ID, rank))
	return rank
}

// Set is the admin / scripted override (faction.md §4 Set): clamps, writes,
// syncs the rank tag, emits NO events and appends NO history. Nil-safe.
func (m *Manager) Set(_ context.Context, e Entity, def *Definition, value int, _ string) {
	if e == nil || def == nil {
		return
	}
	clamped := def.clamp(value)
	e.SetStanding(def.ID, clamped)
	e.SetRankTag(def.ID, RankTag(def.ID, def.RankOf(clamped)))
}

// Shift is the gameplay path (faction.md §4 Shift). Order matches the spec:
// nil-guard → admin bypass → cancellable check → resolve delta → apply (§4.1).
func (m *Manager) Shift(ctx context.Context, e Entity, def *Definition, delta int, reason string) ShiftResult {
	if e == nil || def == nil {
		return ShiftResult{}
	}
	if e.HasTag(AdminRoleTag) {
		return ShiftResult{}
	}
	finalDelta, cancelled := m.sink.OnShiftCheck(ctx, e.ID(), def.ID, reason, delta)
	if cancelled {
		return ShiftResult{Cancelled: true, RequestDelta: delta}
	}
	if finalDelta == 0 {
		return ShiftResult{RequestDelta: delta, ResolvedDelta: 0}
	}
	oldValue := m.standingOf(e, def)
	newValue := def.clamp(oldValue + finalDelta)
	actual := newValue - oldValue
	if actual == 0 {
		return ShiftResult{RequestDelta: delta, ResolvedDelta: finalDelta, OldValue: oldValue, NewValue: newValue}
	}
	oldRank := def.RankOf(oldValue)
	newRank := def.RankOf(newValue)
	rankChanged := oldRank != newRank

	e.SetStanding(def.ID, newValue)
	e.SetRankTag(def.ID, RankTag(def.ID, newRank))
	m.appendHistory(e.ID(), HistoryEntry{
		At:        m.now(),
		FactionID: def.ID,
		Delta:     actual,
		Reason:    reason,
		NewValue:  newValue,
		NewRank:   newRank,
	})

	m.sink.OnShifted(ctx, e.ID(), def.ID, reason, oldValue, newValue, actual, rankChanged)
	if rankChanged {
		m.sink.OnRankChanged(ctx, e.ID(), def.ID, oldRank, newRank)
	}
	return ShiftResult{
		OldValue: oldValue, NewValue: newValue,
		RequestDelta: delta, ResolvedDelta: finalDelta, AppliedDelta: actual,
		OldRank: oldRank, NewRank: newRank, RankChanged: rankChanged,
	}
}

// MeetsStanding is the convenience threshold predicate (faction.md §6): true
// iff the entity's standing with def is at or above min. Nil entity or def → false.
func (m *Manager) MeetsStanding(e Entity, def *Definition, min int) bool {
	if e == nil || def == nil {
		return false
	}
	return m.standingOf(e, def) >= min
}

// ResolveRanks translates a set of rank names into a (minPtr, maxPtr) standing
// range (faction.md §6), baking thresholds at call time. Generalizes
// alignment's ResolveBuckets to a content-defined ladder: the range spans from
// the lowest-indexed selected rank's threshold (nil = open below if it is the
// bottom rank) to one below the next rank above the highest-indexed selected
// rank (nil = open above if it is the top rank). An empty set, all ranks, or
// no recognized names → (nil, nil) = matches everything.
func (m *Manager) ResolveRanks(def *Definition, rankNames []string) (minPtr, maxPtr *int) {
	if def == nil || len(def.Ranks) == 0 || len(rankNames) == 0 {
		return nil, nil
	}
	lo, hi := -1, -1
	for _, name := range rankNames {
		idx := def.rankIndex(name)
		if idx < 0 {
			continue
		}
		if lo == -1 || idx < lo {
			lo = idx
		}
		if hi == -1 || idx > hi {
			hi = idx
		}
	}
	if lo == -1 { // no recognized names
		return nil, nil
	}
	if lo == 0 && hi == len(def.Ranks)-1 { // spans the whole ladder
		return nil, nil
	}
	if lo > 0 {
		v := def.Ranks[lo].Threshold
		minPtr = &v
	}
	if hi < len(def.Ranks)-1 {
		v := def.Ranks[hi+1].Threshold - 1
		maxPtr = &v
	}
	return minPtr, maxPtr
}

// History returns a copy of the entity's combined history ring (oldest first).
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

// appendHistory adds entry to entityID's combined ring, FIFO-trimming to the
// configured capacity.
func (m *Manager) appendHistory(entityID string, entry HistoryEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	histCap := m.reg.cfg.HistoryCapacity
	ring := append(m.history[entityID], entry)
	if len(ring) > histCap {
		drop := len(ring) - histCap
		ring = append(ring[:0], ring[drop:]...)
	}
	m.history[entityID] = ring
}

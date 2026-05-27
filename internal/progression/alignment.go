package progression

import (
	"context"
	"sync"
	"time"
)

// Bucket is the spec §6.1 partition name.
type Bucket string

const (
	BucketEvil    Bucket = "evil"
	BucketNeutral Bucket = "neutral"
	BucketGood    Bucket = "good"
)

// Bucket tag strings mirrored onto entities via SetAlignmentTag
// (spec §6.2). Exposed so adapters use one source of truth.
const (
	TagAlignmentEvil    = "alignment_evil"
	TagAlignmentNeutral = "alignment_neutral"
	TagAlignmentGood    = "alignment_good"
)

// AdminRoleTag is the role tag whose presence makes an entity
// alignment-immune (spec §6.4 Shift step 2). No role system exists
// yet (M10+), so the check is defensive today.
const AdminRoleTag = "admin"

// AlignmentConfig carries the bounds and bucket thresholds. The
// zero value is invalid; callers MUST use DefaultAlignmentConfig
// or fill every field. Min/Max bound writes; EvilThreshold and
// GoodThreshold partition the buckets per spec §6.1.
//
// Invariants enforced by NewAlignmentManager:
//   - Min <= EvilThreshold
//   - EvilThreshold < GoodThreshold
//   - GoodThreshold <= Max
type AlignmentConfig struct {
	Min            int
	Max            int
	EvilThreshold  int
	GoodThreshold  int
	HistoryCapacity int
}

// DefaultAlignmentConfig returns the engine defaults: range
// [-1000, +1000], bucket thresholds ±500, history capacity 20.
// Spec §6.1 leaves the values to engine config; these are the
// tapestry-core M8.5 choice (recorded in ROADMAP).
func DefaultAlignmentConfig() AlignmentConfig {
	return AlignmentConfig{
		Min:             -1000,
		Max:             1000,
		EvilThreshold:   -500,
		GoodThreshold:   500,
		HistoryCapacity: 20,
	}
}

// AlignmentEntity is the host-side adapter the AlignmentManager
// drives. Keeps progression free of entities + session imports —
// cmd/anothermud writes adapters for mobs (MobInstance) and players
// (connActor).
//
// CONCURRENCY CONTRACT: the manager does NOT hold a per-entity
// lock across these calls. The manager's only lock (m.mu) guards
// the history map and is dropped before any sink/entity callback.
// Concurrent Shift calls on the SAME entity are the caller's
// responsibility to serialize — for the M8.5 wiring this is
// naturally true because the live entity callers (connActor /
// MobInstance) take their own per-actor / per-mob lock through
// SetAlignment / SetAlignmentTag.
//
// Implementations MUST NOT call back into the manager's mutating
// methods from these adapter callbacks; a bus subscriber that
// wants to re-shift on the same entity must defer the call out of
// the dispatch goroutine.
type AlignmentEntity interface {
	// ID returns a stable identity used for the history map key
	// and the event payloads. Bare ids (no combat prefix) per
	// progression convention.
	ID() string
	// Alignment returns the current alignment integer. Zero is
	// the valid default for an entity that has never had a
	// shift applied.
	Alignment() int
	// SetAlignment writes the alignment integer. The manager
	// always passes a value already clamped to [Min, Max].
	SetAlignment(value int)
	// SetAlignmentTag installs the bucket tag (one of
	// TagAlignment{Evil,Neutral,Good}) and removes the other two
	// (spec §6.2 "exactly one present at a time"). Passing the
	// empty string clears all three.
	SetAlignmentTag(tag string)
	// HasTag reports whether tag is present on the entity. Used
	// by Shift to detect the admin bypass per spec §6.4 step 2.
	HasTag(tag string) bool
}

// AlignmentSink is the event seam mirroring progression.EventSink
// and combat.EventSink. cmd/anothermud bridges to eventbus.Publish.
// Methods fire synchronously from inside Manager.Shift; an
// implementation that blocks will stall the gameplay path.
type AlignmentSink interface {
	// OnAlignmentShiftCheck publishes the cancellable pre-event
	// (spec §6.4 step 3). Returns (finalDelta, cancelled): the
	// delta listeners may have rewritten, and whether they
	// cancelled. The Manager treats a true cancellation as a
	// hard abort.
	OnAlignmentShiftCheck(ctx context.Context, entityID, reason string, suggestedDelta int) (finalDelta int, cancelled bool)
	// OnAlignmentShifted publishes the post-fact shift event.
	OnAlignmentShifted(ctx context.Context, entityID, reason string, oldValue, newValue, actualDelta int, bucketChanged bool)
	// OnAlignmentBucketChanged publishes the bucket-transition
	// event (fired in addition to shifted, not in place of).
	OnAlignmentBucketChanged(ctx context.Context, entityID string, oldBucket, newBucket Bucket)
}

// nopAlignmentSink discards every event. Default when
// NewAlignmentManager receives a nil sink — keeps tests free of
// boilerplate.
type nopAlignmentSink struct{}

func (nopAlignmentSink) OnAlignmentShiftCheck(_ context.Context, _, _ string, d int) (int, bool) {
	return d, false
}
func (nopAlignmentSink) OnAlignmentShifted(context.Context, string, string, int, int, int, bool) {
}
func (nopAlignmentSink) OnAlignmentBucketChanged(context.Context, string, Bucket, Bucket) {}

// HistoryEntry is one row in the per-entity ring buffer (spec
// §6.3). Timestamp is wall clock; the Manager takes a clock.Now
// closure so tests can drive deterministic timestamps.
type HistoryEntry struct {
	At         time.Time
	Delta      int
	Reason     string
	NewValue   int
	NewBucket  Bucket
}

// AlignmentManager owns the spec §6 operations. History lives
// here (runtime-only by design — the spec's open question on
// persistence resolves to "no" for M8.5 per ROADMAP).
//
// Safe for concurrent use across distinct entities; per-entity
// operations serialize on the entity's history-map slot.
type AlignmentManager struct {
	cfg AlignmentConfig
	now func() time.Time

	mu      sync.Mutex
	history map[string][]HistoryEntry

	sink AlignmentSink
}

// NewAlignmentManager returns a manager seeded with cfg. Panics on
// an invalid config (Min > Evil, Evil >= Good, Good > Max) so the
// composition root catches misconfiguration at boot rather than at
// first shift. now defaults to time.Now when nil; tests pass a
// fixed-clock closure for deterministic history timestamps.
func NewAlignmentManager(cfg AlignmentConfig, sink AlignmentSink, now func() time.Time) *AlignmentManager {
	if cfg.Min > cfg.EvilThreshold {
		panic("progression: AlignmentConfig.Min must be <= EvilThreshold")
	}
	if cfg.EvilThreshold >= cfg.GoodThreshold {
		panic("progression: AlignmentConfig.EvilThreshold must be < GoodThreshold")
	}
	if cfg.GoodThreshold > cfg.Max {
		panic("progression: AlignmentConfig.GoodThreshold must be <= Max")
	}
	if cfg.HistoryCapacity < 1 {
		cfg.HistoryCapacity = 1
	}
	if sink == nil {
		sink = nopAlignmentSink{}
	}
	if now == nil {
		now = time.Now
	}
	return &AlignmentManager{
		cfg:     cfg,
		now:     now,
		sink:    sink,
		history: make(map[string][]HistoryEntry),
	}
}

// Config returns the manager's bounds + thresholds. Exposed for
// renderers (score panel) and ResolveBuckets callers that want to
// inspect the live config.
func (m *AlignmentManager) Config() AlignmentConfig { return m.cfg }

// Get returns the entity's current alignment integer. Nil-safe:
// a nil entity returns zero (the spec §6.4 "default 0 for missing
// entities" path).
func (m *AlignmentManager) Get(e AlignmentEntity) int {
	if e == nil {
		return 0
	}
	return e.Alignment()
}

// Bucket returns the entity's current bucket name AND ensures
// the tag mirror is in sync (spec §6.4 — Bucket is idempotent if
// already in sync). Nil-safe.
func (m *AlignmentManager) Bucket(e AlignmentEntity) Bucket {
	if e == nil {
		return BucketNeutral
	}
	b := m.bucketOf(e.Alignment())
	e.SetAlignmentTag(tagForBucket(b))
	return b
}

// Set is the admin / scripted override (spec §6.4 Set). Clamps
// value to [Min, Max], writes it, syncs the bucket tag, and
// emits NO events. History is NOT appended (spec §6.3 — "Set
// operations do NOT append history"). Nil-safe.
func (m *AlignmentManager) Set(_ context.Context, e AlignmentEntity, value int, _ string) {
	if e == nil {
		return
	}
	clamped := m.clamp(value)
	e.SetAlignment(clamped)
	e.SetAlignmentTag(tagForBucket(m.bucketOf(clamped)))
}

// ShiftResult is the structured return from Shift. Cancelled
// means a listener vetoed the shift via the check event;
// AppliedDelta is the actual change (may be less than requested
// if clamped at min/max, or zero if the resolved delta was zero).
type ShiftResult struct {
	Cancelled     bool
	OldValue      int
	NewValue      int
	RequestDelta  int
	ResolvedDelta int
	AppliedDelta  int
	OldBucket     Bucket
	NewBucket     Bucket
	BucketChanged bool
}

// Shift is the gameplay path (spec §6.4 Shift). Order matches the
// spec literally:
//
//  1. Nil entity → no-op.
//  2. Admin bypass — if entity carries TagAlignmentAdmin (the
//     spec's `admin` role tag), return immediately. NO event
//     fires for admin shifts (chosen design — spec §6.4 step 2
//     bypass is before the check event in step 3).
//  3. Publish alignment.shift.check. Listeners may cancel or
//     rewrite the suggested delta.
//  4. If cancelled, return ShiftResult{Cancelled: true}.
//  5. Resolve the post-event delta. Zero → return without
//     applying or emitting alignment.shifted.
//  6. Apply (§6.5): clamp, write, retag, append history, emit
//     alignment.shifted; if bucket changed, emit
//     alignment.bucket.changed.
func (m *AlignmentManager) Shift(ctx context.Context, e AlignmentEntity, delta int, reason string) ShiftResult {
	if e == nil {
		return ShiftResult{}
	}
	if e.HasTag(AdminRoleTag) {
		return ShiftResult{}
	}
	finalDelta, cancelled := m.sink.OnAlignmentShiftCheck(ctx, e.ID(), reason, delta)
	if cancelled {
		return ShiftResult{Cancelled: true, RequestDelta: delta}
	}
	if finalDelta == 0 {
		return ShiftResult{RequestDelta: delta, ResolvedDelta: 0}
	}
	oldValue := e.Alignment()
	newValue := m.clamp(oldValue + finalDelta)
	actual := newValue - oldValue
	if actual == 0 {
		// Floor/ceiling hit — clamp produced no net change. Spec
		// §6.5 step 3: "if zero, return". No event.
		return ShiftResult{
			RequestDelta:  delta,
			ResolvedDelta: finalDelta,
			OldValue:      oldValue,
			NewValue:      newValue,
		}
	}
	oldBucket := m.bucketOf(oldValue)
	newBucket := m.bucketOf(newValue)
	bucketChanged := oldBucket != newBucket

	e.SetAlignment(newValue)
	e.SetAlignmentTag(tagForBucket(newBucket))
	m.appendHistory(e.ID(), HistoryEntry{
		At:        m.now(),
		Delta:     actual,
		Reason:    reason,
		NewValue:  newValue,
		NewBucket: newBucket,
	})

	m.sink.OnAlignmentShifted(ctx, e.ID(), reason, oldValue, newValue, actual, bucketChanged)
	if bucketChanged {
		m.sink.OnAlignmentBucketChanged(ctx, e.ID(), oldBucket, newBucket)
	}
	return ShiftResult{
		OldValue:      oldValue,
		NewValue:      newValue,
		RequestDelta:  delta,
		ResolvedDelta: finalDelta,
		AppliedDelta:  actual,
		OldBucket:     oldBucket,
		NewBucket:     newBucket,
		BucketChanged: bucketChanged,
	}
}

// History returns a copy of the entity's history ring (oldest
// first). Empty for entities the manager has never shifted.
func (m *AlignmentManager) History(entityID string) []HistoryEntry {
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

// ResolveBuckets translates a set of bucket names into a
// (minPtr, maxPtr) range per spec §6.6. Nil endpoint means
// open-ended on that side. Lookup is case-insensitive against
// the canonical bucket strings.
//
// The mapping (spec §6.6):
//
//	{evil}            → (nil, evilThreshold)
//	{good}            → (goodThreshold, nil)
//	{neutral}         → (evilThreshold+1, goodThreshold-1)
//	{evil, neutral}   → (nil, goodThreshold-1)
//	{good, neutral}   → (evilThreshold+1, nil)
//	{evil, good}      → (nil, nil) (degenerate: matches everything)
//	{}, {evil,good,neutral} → (nil, nil)
//
// Resolution snapshots the live thresholds at call time — the
// spec calls this out explicitly so content that registers
// alignment-gated rules at load can bake the thresholds in.
func (m *AlignmentManager) ResolveBuckets(set []string) (minPtr, maxPtr *int) {
	hasEvil, hasNeutral, hasGood := false, false, false
	for _, s := range set {
		switch Bucket(s) {
		case BucketEvil:
			hasEvil = true
		case BucketNeutral:
			hasNeutral = true
		case BucketGood:
			hasGood = true
		}
	}
	all := (hasEvil && hasNeutral && hasGood) || (!hasEvil && !hasNeutral && !hasGood)
	if all {
		return nil, nil
	}
	// Degenerate: {evil, good} → everything (the gap is neutral,
	// which isn't excluded — spec §6.6 lists this explicitly).
	if hasEvil && hasGood && !hasNeutral {
		return nil, nil
	}
	evilCeil := m.cfg.EvilThreshold
	goodFloor := m.cfg.GoodThreshold
	neutralLow := evilCeil + 1
	neutralHigh := goodFloor - 1

	switch {
	case hasEvil && hasNeutral:
		v := neutralHigh
		return nil, &v
	case hasGood && hasNeutral:
		v := neutralLow
		return &v, nil
	case hasEvil:
		v := evilCeil
		return nil, &v
	case hasGood:
		v := goodFloor
		return &v, nil
	case hasNeutral:
		lo := neutralLow
		hi := neutralHigh
		return &lo, &hi
	}
	return nil, nil
}

// bucketOf returns the bucket for value at the configured
// thresholds. Inclusive at both ends per spec §6.1.
func (m *AlignmentManager) bucketOf(value int) Bucket {
	if value <= m.cfg.EvilThreshold {
		return BucketEvil
	}
	if value >= m.cfg.GoodThreshold {
		return BucketGood
	}
	return BucketNeutral
}

// clamp constrains value to [Min, Max].
func (m *AlignmentManager) clamp(value int) int {
	if value < m.cfg.Min {
		return m.cfg.Min
	}
	if value > m.cfg.Max {
		return m.cfg.Max
	}
	return value
}

// appendHistory adds entry to entityID's ring, trimming the oldest
// when capacity is exceeded. Capacity is enforced as a hard cap;
// trim is FIFO.
func (m *AlignmentManager) appendHistory(entityID string, entry HistoryEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ring := m.history[entityID]
	ring = append(ring, entry)
	if len(ring) > m.cfg.HistoryCapacity {
		// Drop the oldest. Reslice rather than reallocate so steady
		// state stays at capacity without churn (each Shift only
		// allocates one entry; the underlying array reuses).
		drop := len(ring) - m.cfg.HistoryCapacity
		ring = append(ring[:0], ring[drop:]...)
	}
	m.history[entityID] = ring
}

// tagForBucket maps a Bucket to its tag string. Centralized so
// the spec §6.2 tag set is named exactly one place.
func tagForBucket(b Bucket) string {
	switch b {
	case BucketEvil:
		return TagAlignmentEvil
	case BucketGood:
		return TagAlignmentGood
	default:
		return TagAlignmentNeutral
	}
}

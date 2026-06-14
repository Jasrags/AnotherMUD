package progression

import (
	"strings"
	"sync"
)

// Cast is one entity's in-flight timed weave (WoT S2 — the channel interrupt
// game). It captures what the ability phase needs to RE-validate and resolve
// the weave when its warmup elapses: the queued action's fields (so resolution
// re-resolves the current target and re-checks the reserve/cost, exactly as a
// fresh cast would) plus the display name for the begin/disrupt messaging.
//
// Remaining is the countdown in combat ROUNDS, not ticks: the ability phase
// runs once per combat round (combat cadence), so the tracker counts driver
// invocations rather than tick numbers. This keeps cast timing independent of
// the tick/cadence mapping — a 2-round weave is 2 ability phases regardless of
// how many 100ms ticks a round spans.
type Cast struct {
	AbilityID      string
	AbilityName    string
	TargetEntityID string
	// Overchannel is the caster's intent flag (the `overchannel` verb), carried
	// so re-validation at resolve still allows a below-reserve weave.
	Overchannel bool
	// OverchannelDeficit locks the begin-time overdraw (reserve − mana) so the
	// Fortitude DC reflects the reach the player COMMITTED to, not a deficit
	// re-derived after mana regenerated across the warmup. > 0 iff this is a
	// genuine overchannel; 0 for an ordinary cast. WoT S2.
	OverchannelDeficit int
	Remaining          int
}

// CastTracker records each entity's single in-flight timed cast. Process-wide;
// per-entity state lives in one id-keyed map guarded by a mutex. Mirrors
// PulseDelayTracker's shape. An entity casts at most one weave at a time — a
// second Begin while one is active overwrites it (the caller is expected to
// gate on Active first, but overwrite is the safe degenerate).
//
// State is ephemeral, like the cooldown tracker: logout calls Drop, and an
// in-flight cast does not survive a reconnect (it is simply lost, which matches
// the mid-pulse cancellation players already experience link-dead).
type CastTracker struct {
	mu     sync.RWMutex
	active map[string]Cast // entityID -> in-flight cast
}

// NewCastTracker returns an empty tracker.
func NewCastTracker() *CastTracker {
	return &CastTracker{active: make(map[string]Cast)}
}

// Begin records c as entityID's in-flight cast. A non-positive Remaining is
// clamped to 1 (a timed cast always occupies at least the round it began,
// resolving on the next) — Begin is only called for CastTime > 0 abilities, so
// this is a guard, not a path.
func (t *CastTracker) Begin(entityID string, c Cast) {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return
	}
	if c.Remaining < 1 {
		c.Remaining = 1
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active[eid] = c
}

// Active returns entityID's in-flight cast without mutating it. The second
// return is false when the entity has no cast in progress.
func (t *CastTracker) Active(entityID string) (Cast, bool) {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return Cast{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	c, ok := t.active[eid]
	return c, ok
}

// IsCasting reports whether entityID has a weave in progress.
func (t *CastTracker) IsCasting(entityID string) bool {
	_, ok := t.Active(entityID)
	return ok
}

// Advance decrements entityID's in-flight cast by one round and reports the
// result. The returns are:
//
//   - active=false: the entity had no cast in progress (the common idle case).
//   - active=true, ready=false: the cast is still warming up; its decremented
//     state is retained for the next round. The returned Cast reflects the new
//     Remaining.
//   - active=true, ready=true: the warmup elapsed THIS round; the cast has been
//     CLEARED from the tracker and the returned Cast is what the caller must now
//     resolve.
//
// One locked operation, so there is no decrement-then-read race against a
// concurrent Interrupt from another goroutine.
func (t *CastTracker) Advance(entityID string) (Cast, bool, bool) {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return Cast{}, false, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	c, ok := t.active[eid]
	if !ok {
		return Cast{}, false, false
	}
	c.Remaining--
	if c.Remaining <= 0 {
		delete(t.active, eid)
		return c, true, true
	}
	t.active[eid] = c
	return c, false, true
}

// Interrupt clears entityID's in-flight cast and returns it, reporting whether
// there was one to interrupt. The host calls this when the caster is hit (WoT
// S2 interrupt game) so it can message the disruption; under the spend-on-
// success model no Power was spent on an unresolved cast, so the interrupt is a
// free refund (cost was tempo, not Power).
func (t *CastTracker) Interrupt(entityID string) (Cast, bool) {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return Cast{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	c, ok := t.active[eid]
	if ok {
		delete(t.active, eid)
	}
	return c, ok
}

// CastingEntities returns the ids of every entity with a weave in progress.
// The out-of-combat ability drain unions this with the action queue's pending
// set: once a timed cast begins, its queue entry is popped, so a casting-but-
// idle entity would otherwise vanish from the drain and its warmup would never
// advance. Order is unspecified. Returns an empty slice when nobody is casting.
func (t *CastTracker) CastingEntities() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.active) == 0 {
		return nil
	}
	ids := make([]string, 0, len(t.active))
	for id := range t.active {
		ids = append(ids, id)
	}
	return ids
}

// Drop removes any in-flight cast for entityID (logout cleanup). No-op when the
// entity isn't casting.
func (t *CastTracker) Drop(entityID string) {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.active, eid)
}

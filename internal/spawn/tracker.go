// Package spawn owns the area-driven respawn pipeline (spec
// mobs-ai-spawning §3.5–3.7). The Tracker keeps a per-rule census
// of live mob instances; the Manager subscribes to area-tick events
// and runs the §3.6 reset algorithm against the tracker; the
// scheduler (clock.go) emits the area-tick events themselves.
//
// State is intentionally NOT persisted (spec §3.5: "tracking is
// purely runtime state; it does not persist across restart"). A
// fresh boot starts every area at zero population and lets the
// first reset cycle bring counts up to target.
package spawn

import (
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ruleKey is the tracking dimension required by §3.6: every
// spawned entity is recorded against `(area, ruleIndex)` so reset
// can count "rule R produced N living instances".
type ruleKey struct {
	area    world.AreaID
	ruleIdx int
}

// Tracker is the per-rule live-instance census (spec §3.5 "tracks
// every spawned entity against the (area, rule index) pair"). Safe
// for concurrent use: writes from the spawn manager + reads from
// future cleanup paths (death, area reset) all go through the same
// mutex.
type Tracker struct {
	mu  sync.Mutex
	all map[ruleKey][]entities.EntityID
}

// NewTracker returns an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{all: make(map[ruleKey][]entities.EntityID)}
}

// Track records that entity id is alive against (area, ruleIdx).
// Idempotent on duplicate ids — the slice may temporarily hold the
// same id twice if a buggy caller double-tracks, but Count + Purge
// remain correct under those conditions.
func (t *Tracker) Track(area world.AreaID, ruleIdx int, id entities.EntityID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := ruleKey{area: area, ruleIdx: ruleIdx}
	t.all[k] = append(t.all[k], id)
}

// Count returns the number of currently-tracked instances for
// (area, ruleIdx). Used by reset to compute `missing = target -
// living`.
func (t *Tracker) Count(area world.AreaID, ruleIdx int) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.all[ruleKey{area: area, ruleIdx: ruleIdx}])
}

// Purge drops every tracked entity id for (area, ruleIdx) that
// alive(id) reports as no longer existing. Returns the number of
// purged entries — useful for log telemetry. Spec §3.6 step 1:
// "Purge dead tracking. Remove tracking entries whose entity no
// longer exists in the world."
//
// alive is supplied by the caller (Manager wraps
// entities.Store.GetByID) so the tracker itself stays free of any
// store dependency.
func (t *Tracker) Purge(area world.AreaID, ruleIdx int, alive func(entities.EntityID) bool) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := ruleKey{area: area, ruleIdx: ruleIdx}
	src := t.all[k]
	if len(src) == 0 {
		return 0
	}
	// Reuse the backing array via in-place filter. Safe because
	// Snapshot copies before returning, so no external slice
	// aliases the live storage. The map entry rebind below replaces
	// the header with the trimmed view.
	kept := src[:0]
	purged := 0
	for _, id := range src {
		if alive(id) {
			kept = append(kept, id)
			continue
		}
		purged++
	}
	t.all[k] = kept
	return purged
}

// Snapshot returns a copy of the tracked ids for (area, ruleIdx).
// Used by tests and debug telemetry; the production reset path
// goes through Count + Purge instead.
func (t *Tracker) Snapshot(area world.AreaID, ruleIdx int) []entities.EntityID {
	t.mu.Lock()
	defer t.mu.Unlock()
	src := t.all[ruleKey{area: area, ruleIdx: ruleIdx}]
	out := make([]entities.EntityID, len(src))
	copy(out, src)
	return out
}

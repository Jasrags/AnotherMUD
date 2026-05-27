package progression

import (
	"strings"
	"sync"
)

// PulseDelayTracker records per-entity, per-ability cooldown
// expirations as next-ready pulse numbers (spec
// abilities-and-effects §2.2 "pulse-delay cooldown ... recorded
// only in memory (not persisted)"). The tracker is the seam between
// validation (§4.3 step 8 reads "is the entity still cooling
// down?") and resolution (§4.5 step 3 records "next-ready =
// currentPulse + delay + 1").
//
// Process-wide; per-entity state lives in one id-keyed map guarded
// by an RWMutex. Mirrors the other M9 managers' shape.
//
// State is ephemeral. Logout calls Drop. Spec §9 flags long
// cooldowns surviving a reconnect as an open question.
type PulseDelayTracker struct {
	mu      sync.RWMutex
	entries map[string]map[string]int64 // entityID -> abilityID -> next-ready pulse
}

// NewPulseDelayTracker returns an empty tracker.
func NewPulseDelayTracker() *PulseDelayTracker {
	return &PulseDelayTracker{entries: make(map[string]map[string]int64)}
}

// Record writes the next-ready pulse for (entityID, abilityID).
// Spec §4.5 step 3: callers pass `currentPulse + delay + 1`. A
// non-positive readyAtPulse clears the entry instead of writing a
// no-op cooldown.
func (t *PulseDelayTracker) Record(entityID, abilityID string, readyAtPulse int64) {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	aid := strings.ToLower(strings.TrimSpace(abilityID))
	if eid == "" || aid == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if readyAtPulse <= 0 {
		if entry, ok := t.entries[eid]; ok {
			delete(entry, aid)
			if len(entry) == 0 {
				delete(t.entries, eid)
			}
		}
		return
	}
	if t.entries[eid] == nil {
		t.entries[eid] = make(map[string]int64)
	}
	t.entries[eid][aid] = readyAtPulse
}

// IsCoolingDown reports whether (entityID, abilityID) has a recorded
// expiration after currentPulse. Spec §4.3 step 8: the validation
// pipeline rejects when this returns true. Stale entries (readyAt
// <= currentPulse) are treated as expired but NOT garbage-collected
// here — Record clears them on next use, and Sweep handles bulk
// cleanup.
func (t *PulseDelayTracker) IsCoolingDown(entityID, abilityID string, currentPulse int64) bool {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	aid := strings.ToLower(strings.TrimSpace(abilityID))
	if eid == "" || aid == "" {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	readyAt, ok := t.entries[eid][aid]
	if !ok {
		return false
	}
	return readyAt > currentPulse
}

// ReadyAt returns the next-ready pulse for (entityID, abilityID).
// The second return is false when no entry exists.
func (t *PulseDelayTracker) ReadyAt(entityID, abilityID string) (int64, bool) {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	aid := strings.ToLower(strings.TrimSpace(abilityID))
	if eid == "" || aid == "" {
		return 0, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	v, ok := t.entries[eid][aid]
	return v, ok
}

// Sweep evicts every entry whose readyAt <= currentPulse for
// entityID. Returns the count of evicted entries. Optional hygiene
// pass — IsCoolingDown's correctness doesn't depend on it, but
// long-lived sessions accumulate stale entries otherwise.
func (t *PulseDelayTracker) Sweep(entityID string, currentPulse int64) int {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.entries[eid]
	if !ok {
		return 0
	}
	var removed int
	for aid, readyAt := range entry {
		if readyAt <= currentPulse {
			delete(entry, aid)
			removed++
		}
	}
	if len(entry) == 0 {
		delete(t.entries, eid)
	}
	return removed
}

// Drop removes every entry for entityID. Used at logout.
func (t *PulseDelayTracker) Drop(entityID string) {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, eid)
}

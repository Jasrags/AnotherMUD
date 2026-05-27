package combat

import (
	"sync"
	"sync/atomic"
)

// FleeCooldowns tracks per-combatant flee cooldowns (spec combat
// §5.3). Cooldowns are tick-stamped: an entry holds the tick value
// at which the cooldown expires. Active reports whether the current
// tick is below that expiry.
//
// The cooldown source-of-truth is the engine tick counter, not wall
// clock. M7.6 sets cooldowns via Start at flee time using whatever
// "current tick + duration" value the caller computes; the heartbeat
// (or whoever holds the live tick count) advances Now via SetNow.
// Decoupling Now from a real clock keeps the tracker testable —
// tests drive Now manually and don't have to mock time.
//
// Cleanup: an entry past its expiry isn't auto-removed (no scan
// goroutine). Expired entries are filtered out the next time they're
// Active-checked or overwritten by a fresh Start. Practical bound:
// at most one entry per combatant currently in combat; even with
// thousands of fleers per server-lifetime the memory cost is
// negligible. A Purge method may land later if churn becomes
// observable.
type FleeCooldowns struct {
	now atomic.Uint64

	mu      sync.Mutex
	expires map[CombatantID]uint64
}

// NewFleeCooldowns returns an empty cooldown tracker. The initial
// "now" is zero; SetNow advances it.
func NewFleeCooldowns() *FleeCooldowns {
	return &FleeCooldowns{expires: make(map[CombatantID]uint64)}
}

// SetNow advances the tracker's notion of the current tick. Called
// by the heartbeat (or any other tick-aware caller) at the top of
// every combat round. Idempotent — SetNow with a value below the
// current "now" is silently ignored to avoid backward drift if two
// callers race the update.
func (f *FleeCooldowns) SetNow(tick uint64) {
	for {
		cur := f.now.Load()
		if tick <= cur {
			return
		}
		if f.now.CompareAndSwap(cur, tick) {
			return
		}
	}
}

// Now returns the most recently set tick.
func (f *FleeCooldowns) Now() uint64 { return f.now.Load() }

// Start records a cooldown for c that expires at tick `now +
// durationTicks`. A zero or negative duration is silently ignored —
// the spec says cooldown is positive and combat §5.3 leaves the
// duration to configuration; the tracker treats "no duration" as
// "no cooldown" rather than an error.
//
// Overwrites any existing entry for c. A re-flee while still under
// cooldown extends (or shortens) the cooldown to the new expiry.
func (f *FleeCooldowns) Start(c CombatantID, durationTicks uint64) {
	if durationTicks == 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.expires[c] = f.now.Load() + durationTicks
}

// Active reports whether c is currently under a flee cooldown.
// Expired entries are removed lazily on the same call.
func (f *FleeCooldowns) Active(c CombatantID) bool {
	now := f.now.Load()
	f.mu.Lock()
	defer f.mu.Unlock()
	exp, ok := f.expires[c]
	if !ok {
		return false
	}
	if exp <= now {
		delete(f.expires, c)
		return false
	}
	return true
}

// Clear drops c's cooldown entry if any. Used by the combat-ended
// pathway in M7.6 so a combatant who naturally leaves combat (e.g.
// every opponent died) doesn't carry a stale cooldown into the next
// engagement attempt.
func (f *FleeCooldowns) Clear(c CombatantID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.expires, c)
}

package command

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
)

// BadInputEntry is one verb's record in the bad-input tracker (§6): how many
// times an unknown verb was seen and the first/last time it was.
type BadInputEntry struct {
	Verb      string
	Count     int
	FirstSeen time.Time
	LastSeen  time.Time
}

// BadInputTracker records every unknown PLAYER verb the router rejects
// (commands-and-dispatch §6). It is purely informational — the router never
// consults it to change routing — and lives for the process lifetime unless
// Clear'd. Mob unknown verbs never reach it (the mob route is separate from
// Dispatch). Safe for concurrent Record/Snapshot/Clear.
type BadInputTracker struct {
	clk     clock.Clock
	mu      sync.Mutex
	entries map[string]*BadInputEntry
}

// NewBadInputTracker returns an empty tracker stamping timestamps from clk
// (defaults to the real clock when nil, F3).
func NewBadInputTracker(clk clock.Clock) *BadInputTracker {
	if clk == nil {
		clk = clock.RealClock{}
	}
	return &BadInputTracker{clk: clk, entries: make(map[string]*BadInputEntry)}
}

// Record increments the count for verb (lower-cased + trimmed), inserting it
// on first sight. Increment and insert are atomic under one lock so
// concurrent input never loses a count. A nil receiver is a no-op so the
// dispatcher needn't guard the call.
func (t *BadInputTracker) Record(verb string) {
	if t == nil {
		return
	}
	v := strings.ToLower(strings.TrimSpace(verb))
	if v == "" {
		return
	}
	now := t.clk.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.entries[v]; ok {
		e.Count++
		e.LastSeen = now
		return
	}
	t.entries[v] = &BadInputEntry{Verb: v, Count: 1, FirstSeen: now, LastSeen: now}
}

// Snapshot returns a copy of every entry, sorted by count descending (ties
// broken by verb so the order is stable). Used by the `badinput` admin verb
// and any future dashboard. A nil receiver returns nil.
func (t *BadInputTracker) Snapshot() []BadInputEntry {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]BadInputEntry, 0, len(t.entries))
	for _, e := range t.entries {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Verb < out[j].Verb
	})
	return out
}

// Clear drops every entry (tests / operator triage). A nil receiver is a
// no-op.
func (t *BadInputTracker) Clear() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = make(map[string]*BadInputEntry)
}

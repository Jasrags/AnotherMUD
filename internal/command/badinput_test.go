package command

import (
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
)

func TestBadInputTracker_CountsTimestampsAndOrder(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mc := clock.NewManual(t0)
	tr := NewBadInputTracker(mc)

	tr.Record("  Foo  ") // case-insensitive + trimmed
	mc.Advance(time.Minute)
	tr.Record("foo")
	tr.Record("bar")
	tr.Record("") // empty after trim → ignored

	snap := tr.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("entries = %d, want 2", len(snap))
	}
	// Sorted by count desc: foo(2) before bar(1).
	if snap[0].Verb != "foo" || snap[0].Count != 2 {
		t.Errorf("snap[0] = %+v, want foo×2", snap[0])
	}
	if snap[1].Verb != "bar" || snap[1].Count != 1 {
		t.Errorf("snap[1] = %+v, want bar×1", snap[1])
	}
	// foo: first seen at t0, last seen a minute later.
	if !snap[0].FirstSeen.Equal(t0) || !snap[0].LastSeen.Equal(t0.Add(time.Minute)) {
		t.Errorf("foo timestamps = first %v last %v", snap[0].FirstSeen, snap[0].LastSeen)
	}
}

func TestBadInputTracker_Clear(t *testing.T) {
	tr := NewBadInputTracker(clock.NewManual(time.Unix(0, 0)))
	tr.Record("x")
	tr.Clear()
	if got := tr.Snapshot(); len(got) != 0 {
		t.Errorf("after clear: %v, want empty", got)
	}
}

func TestBadInputTracker_NilReceiverSafe(t *testing.T) {
	var tr *BadInputTracker
	tr.Record("x") // must not panic
	if tr.Snapshot() != nil {
		t.Error("nil Snapshot should be nil")
	}
	tr.Clear() // must not panic
}

// Concurrent Record must not lose a count (§6).
func TestBadInputTracker_ConcurrentNoLostCounts(t *testing.T) {
	tr := NewBadInputTracker(clock.NewManual(time.Unix(0, 0)))
	const goroutines, each = 8, 100
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < each; j++ {
				tr.Record("spam")
			}
		}()
	}
	wg.Wait()
	snap := tr.Snapshot()
	if len(snap) != 1 || snap[0].Count != goroutines*each {
		t.Errorf("snapshot = %+v, want spam×%d", snap, goroutines*each)
	}
}

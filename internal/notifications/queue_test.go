package notifications

import (
	"testing"
	"time"
)

// fixedTime returns a deterministic time offset from a shared base so
// tests can build orderings that don't depend on wall clock.
var testBase = time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

func at(seconds int) time.Time {
	return testBase.Add(time.Duration(seconds) * time.Second)
}

func mk(id string, p Priority, sec int) Notification {
	return Notification{
		ID:          id,
		Recipients:  []string{"alice"},
		Priority:    p,
		Kind:        "test",
		Text:        id,
		PublishedAt: at(sec),
	}
}

func TestQueue_AppendBelowCap(t *testing.T) {
	q := NewQueue(5)

	res := q.Append(mk("a", PriorityChannel, 0))

	if res.Refused {
		t.Fatalf("Refused = true, want false on append below cap")
	}
	if res.Evicted != nil {
		t.Fatalf("Evicted = %+v, want nil on append below cap", res.Evicted)
	}
	if q.Len() != 1 {
		t.Fatalf("Len = %d, want 1", q.Len())
	}
}

func TestQueue_DrainPriorityOrder(t *testing.T) {
	q := NewQueue(10)

	// Append in mixed order; expect drain in priority order
	// (system > tell > channel), FIFO within tier.
	q.Append(mk("c1", PriorityChannel, 0))
	q.Append(mk("t1", PriorityTell, 1))
	q.Append(mk("s1", PrioritySystem, 2))
	q.Append(mk("c2", PriorityChannel, 3))
	q.Append(mk("t2", PriorityTell, 4))

	got := q.DrainAll()

	wantIDs := []string{"s1", "t1", "t2", "c1", "c2"}
	if len(got) != len(wantIDs) {
		t.Fatalf("DrainAll returned %d entries, want %d", len(got), len(wantIDs))
	}
	for i, n := range got {
		if n.ID != wantIDs[i] {
			t.Errorf("DrainAll[%d].ID = %q, want %q", i, n.ID, wantIDs[i])
		}
	}

	if q.Len() != 0 {
		t.Errorf("Len after DrainAll = %d, want 0", q.Len())
	}
}

func TestQueue_FIFOWithinTier(t *testing.T) {
	q := NewQueue(10)

	q.Append(mk("b", PriorityChannel, 5)) // later published
	q.Append(mk("a", PriorityChannel, 1)) // earlier published

	got := q.DrainAll()

	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("FIFO order broken: got %q,%q want a,b", got[0].ID, got[1].ID)
	}
}

func TestQueue_EvictOldestLowestTier(t *testing.T) {
	q := NewQueue(3)

	// Queue: [s@0, t@1, c@2] (sorted: s, t, c)
	q.Append(mk("s", PrioritySystem, 0))
	q.Append(mk("t", PriorityTell, 1))
	q.Append(mk("c", PriorityChannel, 2))

	// New tell @3: cap is 3, must evict. Lowest tier present is
	// Channel (c@2). Evict c.
	res := q.Append(mk("t2", PriorityTell, 3))

	if res.Refused {
		t.Fatalf("Refused = true, want eviction not refusal")
	}
	if res.Evicted == nil {
		t.Fatalf("Evicted = nil, want c@2 evicted")
	}
	if res.Evicted.ID != "c" {
		t.Errorf("Evicted.ID = %q, want %q", res.Evicted.ID, "c")
	}
	if q.Len() != 3 {
		t.Errorf("Len = %d, want 3 (cap held)", q.Len())
	}
}

func TestQueue_EvictOldestAtSameTier(t *testing.T) {
	q := NewQueue(2)

	// All same tier; oldest evicts.
	q.Append(mk("c1", PriorityChannel, 0))
	q.Append(mk("c2", PriorityChannel, 1))

	res := q.Append(mk("c3", PriorityChannel, 2))

	if res.Refused {
		t.Fatalf("Refused, want eviction (same-tier oldest)")
	}
	if res.Evicted == nil || res.Evicted.ID != "c1" {
		t.Fatalf("Evicted = %v, want c1", res.Evicted)
	}
	if q.Len() != 2 {
		t.Errorf("Len = %d, want 2", q.Len())
	}
}

func TestQueue_RefuseWhenAllHigherPriority(t *testing.T) {
	q := NewQueue(2)

	q.Append(mk("s1", PrioritySystem, 0))
	q.Append(mk("s2", PrioritySystem, 1))

	// New channel @ full cap of system entries: nothing lower
	// to evict, must refuse.
	res := q.Append(mk("c1", PriorityChannel, 2))

	if !res.Refused {
		t.Fatalf("Refused = false, want refusal (all entries higher priority)")
	}
	if res.Evicted != nil {
		t.Fatalf("Evicted = %v, want nil on refusal", res.Evicted)
	}
	if q.Len() != 2 {
		t.Errorf("Len = %d, want 2 (unchanged on refusal)", q.Len())
	}
}

func TestQueue_RefuseWhenAllSamePriorityAndNewIsLower(t *testing.T) {
	q := NewQueue(2)

	q.Append(mk("t1", PriorityTell, 0))
	q.Append(mk("t2", PriorityTell, 1))

	// New channel against full-of-tells: tells are higher than
	// channel; refuse.
	res := q.Append(mk("c1", PriorityChannel, 2))

	if !res.Refused {
		t.Fatalf("Refused = false, want refusal (all entries higher than new)")
	}
}

func TestQueue_EvictWhenNewIsSystemAndCapFullOfChannel(t *testing.T) {
	q := NewQueue(2)

	q.Append(mk("c1", PriorityChannel, 0))
	q.Append(mk("c2", PriorityChannel, 1))

	// New system: evict oldest channel.
	res := q.Append(mk("s1", PrioritySystem, 2))

	if res.Refused {
		t.Fatalf("Refused, want eviction (new higher than queue)")
	}
	if res.Evicted == nil || res.Evicted.ID != "c1" {
		t.Errorf("Evicted = %v, want c1", res.Evicted)
	}
}

func TestQueue_Snapshot_NonMutating(t *testing.T) {
	q := NewQueue(5)
	q.Append(mk("a", PriorityChannel, 0))
	q.Append(mk("b", PriorityTell, 1))

	snap := q.Snapshot()

	if len(snap) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(snap))
	}
	if q.Len() != 2 {
		t.Errorf("Snapshot mutated queue: Len = %d, want 2", q.Len())
	}
	// Snapshot order is priority order (same as DrainAll without
	// mutating).
	if snap[0].ID != "b" || snap[1].ID != "a" {
		t.Errorf("Snapshot order = [%q,%q], want [b,a]", snap[0].ID, snap[1].ID)
	}
}

func TestQueue_Restore_RoundTrip(t *testing.T) {
	src := NewQueue(5)
	src.Append(mk("a", PriorityChannel, 0))
	src.Append(mk("b", PriorityTell, 1))
	src.Append(mk("c", PrioritySystem, 2))

	snap := src.Snapshot()

	dst := NewQueue(5)
	if err := dst.Restore(snap); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	got := dst.DrainAll()
	if len(got) != 3 {
		t.Fatalf("DrainAll len = %d, want 3", len(got))
	}
	wantIDs := []string{"c", "b", "a"}
	for i, n := range got {
		if n.ID != wantIDs[i] {
			t.Errorf("got[%d].ID = %q, want %q", i, n.ID, wantIDs[i])
		}
	}
}

func TestQueue_Restore_RejectsOverCap(t *testing.T) {
	q := NewQueue(2)
	tooMany := []Notification{
		mk("a", PriorityChannel, 0),
		mk("b", PriorityChannel, 1),
		mk("c", PriorityChannel, 2),
	}
	if err := q.Restore(tooMany); err == nil {
		t.Errorf("Restore over cap: err = nil, want error")
	}
}

func TestQueue_EmptyDrain(t *testing.T) {
	q := NewQueue(5)
	got := q.DrainAll()
	if got != nil && len(got) != 0 {
		t.Errorf("Empty DrainAll = %v, want nil/empty", got)
	}
}

func TestQueue_AppendZeroIsRefused(t *testing.T) {
	// Cap of zero is a degenerate but valid case: nothing fits.
	q := NewQueue(0)
	res := q.Append(mk("a", PrioritySystem, 0))
	if !res.Refused {
		t.Errorf("Refused = false at cap 0, want refusal")
	}
	if q.Len() != 0 {
		t.Errorf("Len = %d at cap 0 after refused append, want 0", q.Len())
	}
}

func TestPriority_DrainsFirstReturnsHighestTier(t *testing.T) {
	// Sanity check on the enum ordering: System > Tell > Channel.
	if !(PrioritySystem > PriorityTell && PriorityTell > PriorityChannel) {
		t.Errorf("Priority ordering wrong: system=%d tell=%d channel=%d",
			PrioritySystem, PriorityTell, PriorityChannel)
	}
}

func TestPriority_String(t *testing.T) {
	cases := map[Priority]string{
		PrioritySystem:  "system",
		PriorityTell:    "tell",
		PriorityChannel: "channel",
		Priority(99):    "unknown",
	}
	for p, want := range cases {
		if got := p.String(); got != want {
			t.Errorf("Priority(%d).String() = %q, want %q", p, got, want)
		}
	}
}

func TestQueue_NewQueueNegativeCapTreatedAsZero(t *testing.T) {
	q := NewQueue(-5)
	if q.Cap() != 0 {
		t.Errorf("Cap = %d, want 0 for negative input", q.Cap())
	}
	if !q.Append(mk("a", PrioritySystem, 0)).Refused {
		t.Errorf("Append on negative-cap queue not refused")
	}
}

func TestQueue_SnapshotEmpty(t *testing.T) {
	q := NewQueue(5)
	if got := q.Snapshot(); got != nil {
		t.Errorf("Snapshot empty = %v, want nil", got)
	}
}

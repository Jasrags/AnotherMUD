package action

import (
	"slices"
	"sync"
	"testing"
)

const reload Kind = "reload"

func TestFreshTrackerIsIdle(t *testing.T) {
	tr := NewTracker()
	if tr.IsBusy("a") {
		t.Fatal("fresh tracker reports busy")
	}
	if _, ok := tr.Active("a"); ok {
		t.Fatal("fresh tracker has an active action")
	}
	if got := tr.BusyEntities(); got != nil {
		t.Fatalf("BusyEntities = %v, want nil", got)
	}
}

func TestBeginRecordsAndRefusesSecond(t *testing.T) {
	tr := NewTracker()
	first := Action{Kind: reload, ReadyAt: 10, Label: "reloading"}
	if !tr.Begin("a", first) {
		t.Fatal("Begin on idle actor returned false")
	}
	if !tr.IsBusy("a") {
		t.Fatal("actor not busy after Begin")
	}
	got, ok := tr.Active("a")
	if !ok || got != first {
		t.Fatalf("Active = %+v, %v; want %+v, true", got, ok, first)
	}

	// A second Begin is refused and leaves the first intact.
	second := Action{Kind: reload, ReadyAt: 99, Label: "other"}
	if tr.Begin("a", second) {
		t.Fatal("second Begin while busy returned true")
	}
	if got, _ := tr.Active("a"); got != first {
		t.Fatalf("first action mutated by refused Begin: %+v", got)
	}
}

func TestBeginBlankIDNoOp(t *testing.T) {
	tr := NewTracker()
	if tr.Begin("   ", Action{Kind: reload}) {
		t.Fatal("Begin with blank id returned true")
	}
	if tr.BusyEntities() != nil {
		t.Fatal("blank-id Begin recorded an action")
	}
}

func TestIDNormalization(t *testing.T) {
	tr := NewTracker()
	tr.Begin("  Player-A  ", Action{Kind: reload, ReadyAt: 5})
	if !tr.IsBusy("player-a") {
		t.Fatal("normalized lookup missed the trimmed/lowercased id")
	}
}

func TestCompleteReadyTiming(t *testing.T) {
	tr := NewTracker()
	tr.Begin("a", Action{Kind: reload, ReadyAt: 10, Label: "reloading"})

	if _, ok := tr.CompleteReady("a", 9); ok {
		t.Fatal("CompleteReady fired before ReadyAt")
	}
	if !tr.IsBusy("a") {
		t.Fatal("action cleared by an early CompleteReady")
	}

	got, ok := tr.CompleteReady("a", 10)
	if !ok || got.Kind != reload {
		t.Fatalf("CompleteReady at ReadyAt = %+v, %v; want the reload, true", got, ok)
	}
	// Single-winner: a second completion gets nothing.
	if _, ok := tr.CompleteReady("a", 11); ok {
		t.Fatal("CompleteReady completed twice")
	}
	if tr.IsBusy("a") {
		t.Fatal("actor still busy after completion")
	}
}

func TestInterruptHonorsFlag(t *testing.T) {
	tr := NewTracker()

	// Interruptible: cleared and returned.
	tr.Begin("a", Action{Kind: reload, ReadyAt: 10, Interruptible: true, Label: "reloading"})
	got, ok := tr.Interrupt("a")
	if !ok || got.Kind != reload {
		t.Fatalf("Interrupt(interruptible) = %+v, %v; want it cleared", got, ok)
	}
	if tr.IsBusy("a") {
		t.Fatal("interruptible action survived Interrupt")
	}

	// Non-interruptible: Interrupt is a no-op, action remains.
	tr.Begin("b", Action{Kind: reload, ReadyAt: 10, Interruptible: false, Label: "channeling"})
	if _, ok := tr.Interrupt("b"); ok {
		t.Fatal("Interrupt cleared a non-interruptible action")
	}
	if !tr.IsBusy("b") {
		t.Fatal("non-interruptible action lost after a no-op Interrupt")
	}

	// Drop clears it regardless of the flag.
	got, ok = tr.Drop("b")
	if !ok || got.Kind != reload {
		t.Fatalf("Drop(non-interruptible) = %+v, %v; want it cleared", got, ok)
	}
	if tr.IsBusy("b") {
		t.Fatal("Drop left the action in place")
	}
}

func TestInterruptAndDropIdle(t *testing.T) {
	tr := NewTracker()
	if _, ok := tr.Interrupt("nobody"); ok {
		t.Fatal("Interrupt on idle actor returned true")
	}
	if _, ok := tr.Drop("nobody"); ok {
		t.Fatal("Drop on idle actor returned true")
	}
}

func TestBusyEntities(t *testing.T) {
	tr := NewTracker()
	tr.Begin("a", Action{Kind: reload, ReadyAt: 1})
	tr.Begin("b", Action{Kind: reload, ReadyAt: 1})
	got := tr.BusyEntities()
	slices.Sort(got)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("BusyEntities = %v, want [a b]", got)
	}
	tr.Drop("a")
	if got := tr.BusyEntities(); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("BusyEntities after drop = %v, want [b]", got)
	}
}

func TestPayloadRoundTrips(t *testing.T) {
	tr := NewTracker()
	type slotRef struct {
		itemID string
		slot   string
	}
	want := slotRef{itemID: "crossbow-1", slot: "wield"}
	tr.Begin("a", Action{Kind: reload, ReadyAt: 3, Payload: want})
	got, _ := tr.CompleteReady("a", 3)
	if got.Payload.(slotRef) != want {
		t.Fatalf("Payload = %+v, want %+v", got.Payload, want)
	}
}

// TestConcurrentAccess exercises begin / complete / interrupt across goroutines
// on overlapping ids under -race.
func TestConcurrentAccess(t *testing.T) {
	tr := NewTracker()
	ids := []string{"a", "b", "c", "d"}
	var wg sync.WaitGroup
	for i := range 200 {
		id := ids[i%len(ids)]
		wg.Add(4)
		go func() { defer wg.Done(); tr.Begin(id, Action{Kind: reload, ReadyAt: 1, Interruptible: true}) }()
		go func() { defer wg.Done(); tr.CompleteReady(id, 2) }()
		go func() { defer wg.Done(); tr.Interrupt(id) }()
		go func() { defer wg.Done(); _ = tr.BusyEntities(); _ = tr.IsBusy(id) }()
	}
	wg.Wait()
}

// A nil tracker is safe (a build without the substrate wired).
func TestNilTracker(t *testing.T) {
	var tr *Tracker
	if tr.Begin("a", Action{}) {
		t.Fatal("nil Begin returned true")
	}
	if tr.IsBusy("a") {
		t.Fatal("nil IsBusy returned true")
	}
	if _, ok := tr.CompleteReady("a", 0); ok {
		t.Fatal("nil CompleteReady returned true")
	}
	if _, ok := tr.Interrupt("a"); ok {
		t.Fatal("nil Interrupt returned true")
	}
	if _, ok := tr.Drop("a"); ok {
		t.Fatal("nil Drop returned true")
	}
	if tr.BusyEntities() != nil {
		t.Fatal("nil BusyEntities non-nil")
	}
}

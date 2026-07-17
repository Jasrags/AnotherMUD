package karma

import (
	"sync"
	"testing"
)

func TestGrant_RaisesCurrentAndTotal(t *testing.T) {
	l := NewLedger()
	if got := l.Grant(30); got != 30 {
		t.Fatalf("Grant returned current %d, want 30", got)
	}
	l.Grant(20)
	if l.Current() != 50 {
		t.Errorf("current = %d, want 50", l.Current())
	}
	if l.Total() != 50 {
		t.Errorf("total = %d, want 50", l.Total())
	}
}

func TestGrant_NonPositiveIsNoop(t *testing.T) {
	l := NewLedger()
	l.Grant(10)
	l.Grant(0)
	l.Grant(-5)
	if l.Current() != 10 || l.Total() != 10 {
		t.Fatalf("current=%d total=%d, want 10/10 (non-positive grants ignored)", l.Current(), l.Total())
	}
}

func TestSpend_DeductsCurrentLeavesTotal(t *testing.T) {
	l := NewLedger()
	l.Grant(50)
	if !l.Spend(30) {
		t.Fatal("Spend(30) refused with balance 50")
	}
	if l.Current() != 20 {
		t.Errorf("current = %d, want 20", l.Current())
	}
	if l.Total() != 50 {
		t.Errorf("total = %d, want 50 (spending never lowers lifetime)", l.Total())
	}
}

func TestSpend_RefusesInsufficientOrNonPositive(t *testing.T) {
	l := NewLedger()
	l.Grant(10)
	if l.Spend(11) {
		t.Error("Spend(11) succeeded with balance 10")
	}
	if l.Spend(0) {
		t.Error("Spend(0) succeeded")
	}
	if l.Spend(-1) {
		t.Error("Spend(-1) succeeded")
	}
	if l.Current() != 10 {
		t.Errorf("current = %d, want 10 (refused spends do not mutate)", l.Current())
	}
}

func TestSpend_ExactBalance(t *testing.T) {
	l := NewLedger()
	l.Grant(25)
	if !l.Spend(25) {
		t.Fatal("Spend(25) refused with exact balance 25")
	}
	if l.Current() != 0 {
		t.Errorf("current = %d, want 0", l.Current())
	}
}

func TestSnapshotRestore_RoundTrip(t *testing.T) {
	l := NewLedger()
	l.Grant(100)
	l.Spend(40)
	snap := l.Snapshot()
	if snap.Current != 60 || snap.Total != 100 {
		t.Fatalf("snapshot = %+v, want {Current:60 Total:100}", snap)
	}
	restored := NewLedger()
	restored.Restore(snap)
	if restored.Current() != 60 || restored.Total() != 100 {
		t.Errorf("restored current=%d total=%d, want 60/100", restored.Current(), restored.Total())
	}
}

func TestRestore_ClampsNegative(t *testing.T) {
	l := NewLedger()
	l.Restore(Snapshot{Current: -5, Total: -10})
	if l.Current() != 0 || l.Total() != 0 {
		t.Errorf("current=%d total=%d, want 0/0 (negatives clamped)", l.Current(), l.Total())
	}
}

func TestConcurrentGrantSpend_RaceSafe(t *testing.T) {
	l := NewLedger()
	l.Grant(1000)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); l.Grant(2) }()
		go func() { defer wg.Done(); l.Spend(1) }()
	}
	wg.Wait()
	// 1000 + 50*2 granted = 1100 total; spends removed at most 50.
	if l.Total() != 1100 {
		t.Errorf("total = %d, want 1100", l.Total())
	}
	if l.Current() < 0 {
		t.Errorf("current went negative: %d", l.Current())
	}
}

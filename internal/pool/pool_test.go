package pool

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestNew_ClampsMaxToFloor(t *testing.T) {
	tests := []struct {
		name        string
		max         int
		rules       Rules
		wantCurrent int
		wantMax     int
	}{
		{"plain hp", 20, Rules{Floor: 0}, 20, 20},
		{"max below floor raised", 3, Rules{Floor: 10}, 10, 10},
		{"max equals floor", 5, Rules{Floor: 5}, 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New("hp", tt.max, tt.rules)
			cur, max := p.Snapshot()
			if cur != tt.wantCurrent || max != tt.wantMax {
				t.Fatalf("New(%d,%+v) = (cur %d, max %d); want (%d, %d)",
					tt.max, tt.rules, cur, max, tt.wantCurrent, tt.wantMax)
			}
		})
	}
}

func TestNewAt_ClampsCurrent(t *testing.T) {
	tests := []struct {
		name        string
		current     int
		max         int
		rules       Rules
		wantCurrent int
	}{
		{"in range", 12, 20, Rules{}, 12},
		{"above max clamps down", 30, 20, Rules{}, 20},
		{"below floor clamps up", -5, 20, Rules{Floor: 0}, 0},
		{"below custom floor", 2, 20, Rules{Floor: 5}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAt("hp", tt.current, tt.max, tt.rules)
			if got := p.Current(); got != tt.wantCurrent {
				t.Fatalf("NewAt current = %d; want %d", got, tt.wantCurrent)
			}
		})
	}
}

func TestApplyDamage(t *testing.T) {
	tests := []struct {
		name         string
		start        int
		max          int
		rules        Rules
		amount       int
		wantCurrent  int
		wantOverflow int
		wantCrossed  bool
	}{
		{"partial", 20, 20, Rules{Floor: 0}, 5, 15, 0, false},
		{"zero damage on living does not cross", 20, 20, Rules{Floor: 0}, 0, 20, 0, false},
		{"exact to floor crosses", 20, 20, Rules{Floor: 0}, 20, 0, 0, true},
		{"over floor produces overflow + crosses", 10, 20, Rules{Floor: 0}, 13, 0, 3, true},
		{"hit on already-floored does not re-cross", 0, 20, Rules{Floor: 0}, 5, 0, 5, false},
		{"negative amount is no-op", 20, 20, Rules{Floor: 0}, -4, 20, 0, false},
		{"custom floor", 12, 20, Rules{Floor: 10}, 5, 10, 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAt("hp", tt.start, tt.max, tt.rules)
			cur, overflow, crossed := p.ApplyDamage(tt.amount)
			if cur != tt.wantCurrent || overflow != tt.wantOverflow || crossed != tt.wantCrossed {
				t.Fatalf("ApplyDamage(%d) = (cur %d, overflow %d, crossed %v); want (%d, %d, %v)",
					tt.amount, cur, overflow, crossed, tt.wantCurrent, tt.wantOverflow, tt.wantCrossed)
			}
		})
	}
}

func TestTrySpend(t *testing.T) {
	tests := []struct {
		name        string
		start       int
		rules       Rules
		amount      int
		wantOK      bool
		wantCurrent int
	}{
		{"sufficient", 10, Rules{Floor: 0}, 4, true, 6},
		{"exact", 10, Rules{Floor: 0}, 10, true, 0},
		{"insufficient leaves untouched", 3, Rules{Floor: 0}, 4, false, 3},
		{"would breach custom floor", 12, Rules{Floor: 10}, 3, false, 12},
		{"non-positive succeeds noop", 10, Rules{Floor: 0}, 0, true, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAt("mana", tt.start, 20, tt.rules)
			if ok := p.TrySpend(tt.amount); ok != tt.wantOK {
				t.Fatalf("TrySpend(%d) ok = %v; want %v", tt.amount, ok, tt.wantOK)
			}
			if got := p.Current(); got != tt.wantCurrent {
				t.Fatalf("after TrySpend current = %d; want %d", got, tt.wantCurrent)
			}
		})
	}
}

func TestDeductFloorsNotUnderflow(t *testing.T) {
	p := NewAt("movement", 5, 20, Rules{Floor: 0})
	if got := p.Deduct(8); got != 0 {
		t.Fatalf("Deduct(8) from 5 = %d; want 0 (floored)", got)
	}
}

func TestRestoreCapsAtMax(t *testing.T) {
	p := NewAt("hp", 15, 20, Rules{Floor: 0})
	if got := p.Restore(100); got != 20 {
		t.Fatalf("Restore(100) = %d; want 20 (capped)", got)
	}
}

func TestRestoreDeltaReturnsActualRestored(t *testing.T) {
	// Partial heal below max: delta is the full amount.
	p := NewAt("hp", 5, 20, Rules{Floor: 0})
	if restored, cur, max := p.RestoreDelta(9); restored != 9 || cur != 14 || max != 20 {
		t.Fatalf("RestoreDelta(9) from 5 = (restored %d, cur %d, max %d); want (9, 14, 20)", restored, cur, max)
	}
	// Overheal past max: delta is only the HP that fit, not the requested amount.
	p2 := NewAt("hp", 15, 20, Rules{Floor: 0})
	if restored, cur, _ := p2.RestoreDelta(100); restored != 5 || cur != 20 {
		t.Fatalf("RestoreDelta(100) from 15/20 = (restored %d, cur %d); want (5, 20)", restored, cur)
	}
	// Already full: no restore, zero delta.
	p3 := NewAt("hp", 20, 20, Rules{Floor: 0})
	if restored, _, _ := p3.RestoreDelta(10); restored != 0 {
		t.Fatalf("RestoreDelta on a full pool = %d; want 0", restored)
	}
}

func TestSetMaxClampsCurrentDown(t *testing.T) {
	p := New("hp", 20, Rules{Floor: 0}) // current 20
	p.SetMax(12)
	if cur, max := p.Snapshot(); cur != 12 || max != 12 {
		t.Fatalf("after SetMax(12) = (cur %d, max %d); want (12, 12)", cur, max)
	}
	// Raising max does not auto-fill.
	p.SetMax(30)
	if cur, max := p.Snapshot(); cur != 12 || max != 30 {
		t.Fatalf("after SetMax(30) = (cur %d, max %d); want (12, 30)", cur, max)
	}
}

func TestDeplete(t *testing.T) {
	p := New("hp", 20, Rules{Floor: 0})
	if !p.Deplete() {
		t.Fatal("Deplete on a living pool should report wasAbove=true")
	}
	if !p.IsEmpty() {
		t.Fatal("pool should be empty after Deplete")
	}
	if p.Deplete() {
		t.Fatal("Deplete on an already-floored pool should report wasAbove=false")
	}
}

func TestDeplete_DepletesExactlyOnceUnderRace(t *testing.T) {
	const goroutines = 64
	p := New("hp", 100, Rules{Floor: 0})
	var aboveCount int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if p.Deplete() {
				atomic.AddInt64(&aboveCount, 1)
			}
		}()
	}
	wg.Wait()
	if aboveCount != 1 {
		t.Fatalf("Deplete reported wasAbove=true %d times; want exactly 1", aboveCount)
	}
}

func TestPercent(t *testing.T) {
	tests := []struct {
		name    string
		current int
		max     int
		rules   Rules
		want    float64
	}{
		{"half hp", 10, 20, Rules{Floor: 0}, 0.5},
		{"empty", 0, 20, Rules{Floor: 0}, 0},
		{"degenerate range", 0, 0, Rules{Floor: 0}, 0},
		{"floored range", 10, 30, Rules{Floor: 10}, 0}, // current at floor
		{"half of floored range", 20, 30, Rules{Floor: 10}, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAt("hp", tt.current, tt.max, tt.rules)
			if got := p.Percent(); got != tt.want {
				t.Fatalf("Percent() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestIsEmpty(t *testing.T) {
	if !NewAt("hp", 0, 20, Rules{Floor: 0}).IsEmpty() {
		t.Fatal("0/20 floor 0 should be empty")
	}
	if NewAt("hp", 1, 20, Rules{Floor: 0}).IsEmpty() {
		t.Fatal("1/20 should not be empty")
	}
	if !NewAt("hp", 10, 20, Rules{Floor: 10}).IsEmpty() {
		t.Fatal("at floor should be empty")
	}
}

// TestApplyDamage_CrossesExactlyOnceUnderRace asserts the killing-blow
// guarantee: when N goroutines each apply lethal damage concurrently,
// exactly one observes crossed=true. This is the property the combat
// death flow relies on to emit VitalDepleted once (replacing
// Vitals.ApplyDamageIfAlive's wasAlive).
func TestApplyDamage_CrossesExactlyOnceUnderRace(t *testing.T) {
	const goroutines = 64
	p := New("hp", 100, Rules{Floor: 0})

	var crossedCount int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if _, _, crossed := p.ApplyDamage(100); crossed {
				atomic.AddInt64(&crossedCount, 1)
			}
		}()
	}
	wg.Wait()

	if crossedCount != 1 {
		t.Fatalf("crossed observed %d times; want exactly 1", crossedCount)
	}
	if !p.IsEmpty() {
		t.Fatal("pool should be empty after lethal hits")
	}
}

// TestPool_ConcurrentMixedOps is the -race smoke test: damage, restore,
// spend, and reads all hammering one pool from many goroutines must not
// race and must leave Current within [Floor, Max].
func TestPool_ConcurrentMixedOps(t *testing.T) {
	p := New("hp", 1000, Rules{Floor: 0})
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 100 {
				switch (n + j) % 4 {
				case 0:
					p.ApplyDamage(7)
				case 1:
					p.Restore(5)
				case 2:
					p.TrySpend(3)
				case 3:
					_, _ = p.Snapshot()
				}
			}
		}(i)
	}
	wg.Wait()

	cur, max := p.Snapshot()
	if cur < 0 || cur > max {
		t.Fatalf("current %d out of range [0, %d] after concurrent ops", cur, max)
	}
}

// Refill sets current to max in one step — used at character creation, where
// SetMax raised the ceiling (0→N) without touching current.
func TestPool_Refill(t *testing.T) {
	p := NewAt(Kind("mana"), 0, 30, Rules{Floor: 0})
	p.Refill()
	if c := p.Current(); c != 30 {
		t.Fatalf("Current after Refill = %d; want 30", c)
	}
	// Refill on a full pool is a no-op (stays at max).
	p.Refill()
	if c := p.Current(); c != 30 {
		t.Fatalf("Current after second Refill = %d; want 30", c)
	}
}

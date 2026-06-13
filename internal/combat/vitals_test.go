package combat

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestApplyDamageIfAlive_KillingBlowExactlyOnce locks the once-only death
// guarantee at the Vitals facade (not just the pool layer): when N
// goroutines each apply lethal damage, exactly one observes the combat
// emit condition (wasAlive && remaining <= 0) that drives a single
// VitalDepleted. Regression guard for the crossed||cur>0 reconstruction.
func TestApplyDamageIfAlive_KillingBlowExactlyOnce(t *testing.T) {
	const goroutines = 64
	v := NewVitals(100)
	var killingBlows int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			remaining, wasAlive := v.ApplyDamageIfAlive(100)
			if wasAlive && remaining <= 0 { // exact autoattack.go emit gate
				atomic.AddInt64(&killingBlows, 1)
			}
		}()
	}
	wg.Wait()
	if killingBlows != 1 {
		t.Fatalf("killing blow observed %d times; want exactly 1", killingBlows)
	}
	if !v.IsDead() {
		t.Fatal("vitals should be dead after lethal hits")
	}
}

func TestNewVitalsClampsNonPositiveMax(t *testing.T) {
	v := NewVitals(0)
	if cur, max := v.Snapshot(); cur != 1 || max != 1 {
		t.Errorf("NewVitals(0) = (%d, %d), want (1, 1)", cur, max)
	}
	v = NewVitals(-5)
	if cur, max := v.Snapshot(); cur != 1 || max != 1 {
		t.Errorf("NewVitals(-5) = (%d, %d), want (1, 1)", cur, max)
	}
}

func TestNewVitalsAtClampsRange(t *testing.T) {
	cases := []struct {
		name             string
		hp, max          int
		wantCur, wantMax int
	}{
		{"hp above max clamps to max", 50, 20, 20, 20},
		{"negative hp clamps to zero", -3, 20, 0, 20},
		{"max below 1 clamps to 1", 5, 0, 1, 1},
		{"hp inside range preserved", 5, 20, 5, 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := NewVitalsAt(tc.hp, tc.max)
			if cur, max := v.Snapshot(); cur != tc.wantCur || max != tc.wantMax {
				t.Errorf("got (%d, %d), want (%d, %d)", cur, max, tc.wantCur, tc.wantMax)
			}
		})
	}
}

func TestApplyDamageClampsAtZero(t *testing.T) {
	v := NewVitals(10)
	if got := v.ApplyDamage(3); got != 7 {
		t.Errorf("ApplyDamage(3) = %d, want 7", got)
	}
	if got := v.ApplyDamage(50); got != 0 {
		t.Errorf("ApplyDamage(50) past zero = %d, want 0", got)
	}
	if !v.IsDead() {
		t.Errorf("IsDead() = false after damage past zero, want true")
	}
}

func TestDepleteKillsOnceThenReportsAlreadyDead(t *testing.T) {
	v := NewVitals(10)
	// First call drains a living combatant and reports it was alive.
	if wasAlive := v.Deplete(); !wasAlive {
		t.Error("first Deplete() = false, want true (was alive)")
	}
	if got := v.Current(); got != 0 {
		t.Errorf("HP after Deplete = %d, want 0", got)
	}
	if !v.IsDead() {
		t.Error("IsDead() = false after Deplete, want true")
	}
	// Second call (the concurrent-killer / double-emit guard) reports it
	// was already dead so the caller does NOT emit a second VitalDepleted.
	if wasAlive := v.Deplete(); wasAlive {
		t.Error("second Deplete() = true, want false (already dead — guards double-emit)")
	}
}

func TestApplyDamageNegativeIsZero(t *testing.T) {
	v := NewVitals(10)
	if got := v.ApplyDamage(-5); got != 10 {
		t.Errorf("ApplyDamage(-5) = %d, want 10 (negative treated as zero)", got)
	}
}

func TestHealClampsAtMax(t *testing.T) {
	v := NewVitalsAt(2, 10)
	if got := v.Heal(3); got != 5 {
		t.Errorf("Heal(3) = %d, want 5", got)
	}
	if got := v.Heal(100); got != 10 {
		t.Errorf("Heal(100) past max = %d, want 10", got)
	}
}

func TestHealNegativeIsZero(t *testing.T) {
	v := NewVitalsAt(2, 10)
	if got := v.Heal(-5); got != 2 {
		t.Errorf("Heal(-5) = %d, want 2 (negative treated as zero)", got)
	}
}

func TestPercentDeadIsZeroNotNaN(t *testing.T) {
	v := NewVitals(10)
	v.ApplyDamage(20)
	if got := v.Percent(); got != 0 {
		t.Errorf("Percent() on dead = %f, want 0", got)
	}
}

func TestPercentRange(t *testing.T) {
	v := NewVitalsAt(3, 10)
	if got := v.Percent(); got < 0.29 || got > 0.31 {
		t.Errorf("Percent() = %f, want ~0.30", got)
	}
}

func TestSetMaxClampsCurrentDown(t *testing.T) {
	v := NewVitalsAt(50, 100)
	v.SetMax(30)
	if cur, max := v.Snapshot(); cur != 30 || max != 30 {
		t.Errorf("after SetMax(30) from 50/100 got (%d, %d), want (30, 30)", cur, max)
	}
}

func TestSetMaxAboveCurrentPreservesCurrent(t *testing.T) {
	v := NewVitalsAt(20, 50)
	v.SetMax(100)
	if cur, max := v.Snapshot(); cur != 20 || max != 100 {
		t.Errorf("after SetMax(100) from 20/50 got (%d, %d), want (20, 100)", cur, max)
	}
}

func TestSetCurrentClampsToRange(t *testing.T) {
	v := NewVitalsAt(50, 100)

	if got := v.SetCurrent(30); got != 30 {
		t.Errorf("SetCurrent(30) = %d, want 30", got)
	}
	if cur, _ := v.Snapshot(); cur != 30 {
		t.Errorf("current after SetCurrent(30) = %d, want 30", cur)
	}
	// Above max clamps to max.
	if got := v.SetCurrent(999); got != 100 {
		t.Errorf("SetCurrent(999) = %d, want 100 (clamped to max)", got)
	}
	// Negative clamps to zero.
	if got := v.SetCurrent(-5); got != 0 {
		t.Errorf("SetCurrent(-5) = %d, want 0 (clamped to floor)", got)
	}
}

func TestApplyDamageIfAliveOnLiving(t *testing.T) {
	v := NewVitalsAt(10, 10)
	remaining, wasAlive := v.ApplyDamageIfAlive(3)
	if !wasAlive {
		t.Errorf("wasAlive = false on living combatant, want true")
	}
	if remaining != 7 {
		t.Errorf("remaining = %d, want 7", remaining)
	}
}

func TestApplyDamageIfAliveDealsKillingBlow(t *testing.T) {
	v := NewVitalsAt(5, 10)
	remaining, wasAlive := v.ApplyDamageIfAlive(100)
	if !wasAlive {
		t.Errorf("wasAlive = false; the killing blow's victim was alive at swing time, want true")
	}
	if remaining != 0 {
		t.Errorf("remaining = %d, want 0", remaining)
	}
}

func TestApplyDamageIfAliveSkipsDead(t *testing.T) {
	v := NewVitalsAt(0, 10)
	remaining, wasAlive := v.ApplyDamageIfAlive(50)
	if wasAlive {
		t.Errorf("wasAlive = true on dead combatant, want false")
	}
	if remaining != 0 {
		t.Errorf("remaining = %d, want 0 (unchanged)", remaining)
	}
}

func TestApplyDamageIfAliveNegativeClampsToZero(t *testing.T) {
	v := NewVitalsAt(10, 10)
	remaining, wasAlive := v.ApplyDamageIfAlive(-5)
	if !wasAlive || remaining != 10 {
		t.Errorf("got (%d, %v), want (10, true)", remaining, wasAlive)
	}
}

// TestVitalsConcurrency confirms the internal mutex serializes
// concurrent mutation. With -race the harness fails fast if the
// mutex were missing. We do not assert a specific final HP; the
// point is to exercise the lock from N goroutines.
func TestVitalsConcurrency(t *testing.T) {
	v := NewVitalsAt(1000, 1000)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			v.ApplyDamage(1)
		}()
		go func() {
			defer wg.Done()
			_ = v.Current()
		}()
	}
	wg.Wait()
	if got := v.Current(); got != 900 {
		t.Errorf("after 100 ApplyDamage(1) calls got %d, want 900", got)
	}
}

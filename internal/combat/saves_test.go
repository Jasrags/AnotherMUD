package combat

import "testing"

// fixedRoller (damage_test.go) programs the RAW IntN return, so a d20 face
// of N is programmed as N-1.
func d20(face int) *fixedRoller { return &fixedRoller{values: []int{face - 1}} }

func TestResolveSave_MeetsDCSucceeds(t *testing.T) {
	// roll 12 + bonus 3 = 15 vs DC 15 → success (>= is a pass).
	out := ResolveSave(d20(12), 3, 15)
	if !out.Success {
		t.Fatalf("expected success: %+v", out)
	}
	if out.Roll != 12 || out.Bonus != 3 || out.Total != 15 || out.DC != 15 {
		t.Errorf("detail mismatch: %+v", out)
	}
	if out.Natural1 || out.Natural20 {
		t.Errorf("unexpected natural flag: %+v", out)
	}
}

func TestResolveSave_BelowDCFails(t *testing.T) {
	// roll 10 + bonus 2 = 12 vs DC 15 → fail.
	out := ResolveSave(d20(10), 2, 15)
	if out.Success {
		t.Fatalf("expected failure: %+v", out)
	}
	if out.Total != 12 {
		t.Errorf("Total = %d, want 12", out.Total)
	}
}

func TestResolveSave_Natural1AlwaysFails(t *testing.T) {
	// Enormous bonus would otherwise crush any DC; natural 1 still fails.
	out := ResolveSave(d20(1), 100, 5)
	if out.Success {
		t.Fatalf("natural 1 must fail regardless of bonus: %+v", out)
	}
	if !out.Natural1 || out.Natural20 {
		t.Errorf("flags wrong: %+v", out)
	}
}

func TestResolveSave_Natural20AlwaysSucceeds(t *testing.T) {
	// Bonus + roll can't reach the DC, but natural 20 auto-succeeds.
	out := ResolveSave(d20(20), -50, 100)
	if !out.Success {
		t.Fatalf("natural 20 must succeed regardless of DC: %+v", out)
	}
	if !out.Natural20 || out.Natural1 {
		t.Errorf("flags wrong: %+v", out)
	}
}

func TestResolveSave_NegativeBonus(t *testing.T) {
	// roll 14 + (-2) = 12 vs DC 12 → success at the boundary.
	out := ResolveSave(d20(14), -2, 12)
	if !out.Success || out.Total != 12 {
		t.Errorf("boundary with negative bonus failed: %+v", out)
	}
}

// Deterministic under a seeded/scripted roller: the same programmed faces
// always produce the same outcome (no hidden randomness or global state).
func TestResolveSave_DeterministicUnderSeededRoller(t *testing.T) {
	a := ResolveSave(&fixedRoller{values: []int{6, 9}}, 1, 10) // first roll = face 7
	b := ResolveSave(&fixedRoller{values: []int{6, 9}}, 1, 10)
	if a != b {
		t.Errorf("non-deterministic: %+v vs %+v", a, b)
	}
	if a.Roll != 7 || a.Total != 8 || a.Success {
		t.Errorf("unexpected first-roll outcome: %+v", a)
	}
}

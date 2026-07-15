package progression

import "testing"

// seededRoller serves a programmed sequence of raw IntN results (a d20 face N
// is programmed as N-1, since ResolveSkillCheck does IntN(20)+1).
type seededRoller struct {
	seq []int
	idx int
}

func (r *seededRoller) IntN(int) int {
	v := r.seq[r.idx]
	r.idx++
	return v
}

func d20Roll(face int) *seededRoller { return &seededRoller{seq: []int{face - 1}} }

func TestSkillBonus_ProficiencyAndStat(t *testing.T) {
	cfg := DefaultSkillConfig() // scale 0.2
	// proficiency 50 → +10 ; Dex 14 → AbilityModifier +2 ; total +12.
	if got := SkillBonus(50, 14, cfg); got != 12 {
		t.Errorf("SkillBonus(50,14) = %d, want 12", got)
	}
	// Untrained (proficiency 0) with average stat → +0.
	if got := SkillBonus(0, 10, cfg); got != 0 {
		t.Errorf("SkillBonus(0,10) = %d, want 0", got)
	}
	// Master (100) → +20 ; a stat penalty subtracts.
	if got := SkillBonus(100, 8, cfg); got != 19 { // 20 + (-1)
		t.Errorf("SkillBonus(100,8) = %d, want 19", got)
	}
}

// ProficiencyBonus is the proficiency-only term (no attribute modifier) the
// weapon-skill to-hit model uses (skills §7): the attribute is already in the
// attack channel, so only the rating contributes here.
func TestProficiencyBonus_RatingOnly(t *testing.T) {
	cfg := DefaultSkillConfig() // scale 0.2
	cases := []struct {
		prof, want int
	}{
		{0, 0},    // untrained → 0 (the caller applies a default penalty)
		{1, 0},    // freshly granted → neutral, no penalty
		{50, 10},  // journeyman → +10
		{100, 20}, // master → +20
	}
	for _, c := range cases {
		if got := ProficiencyBonus(c.prof, cfg); got != c.want {
			t.Errorf("ProficiencyBonus(%d) = %d, want %d", c.prof, got, c.want)
		}
	}
}

func TestResolveSkillCheck_MeetsDCSucceeds(t *testing.T) {
	// roll 12 + bonus 8 = 20 vs DC 20 → success (>= passes).
	out := ResolveSkillCheck(d20Roll(12), 8, 20)
	if !out.Success || out.Total != 20 || out.Roll != 12 || out.DC != 20 {
		t.Fatalf("unexpected: %+v", out)
	}
	if out.Natural1 || out.Natural20 {
		t.Errorf("no natural flag expected: %+v", out)
	}
}

func TestResolveSkillCheck_BelowDCFails(t *testing.T) {
	out := ResolveSkillCheck(d20Roll(10), 2, 20) // 12 < 20
	if out.Success {
		t.Fatalf("expected failure: %+v", out)
	}
}

func TestResolveSkillCheck_Natural1AlwaysFails(t *testing.T) {
	out := ResolveSkillCheck(d20Roll(1), 100, 5) // huge bonus, trivial DC
	if out.Success || !out.Natural1 {
		t.Errorf("natural 1 must fail: %+v", out)
	}
}

func TestResolveSkillCheck_Natural20AlwaysSucceeds(t *testing.T) {
	out := ResolveSkillCheck(d20Roll(20), -50, 100) // can't reach DC by math
	if !out.Success || !out.Natural20 {
		t.Errorf("natural 20 must succeed: %+v", out)
	}
}

func TestResolveSkillCheck_DeterministicUnderSeededRoller(t *testing.T) {
	a := ResolveSkillCheck(&seededRoller{seq: []int{6}}, 3, 12) // face 7 → 10 < 12
	b := ResolveSkillCheck(&seededRoller{seq: []int{6}}, 3, 12)
	if a != b {
		t.Errorf("non-deterministic: %+v vs %+v", a, b)
	}
	if a.Roll != 7 || a.Total != 10 || a.Success {
		t.Errorf("unexpected: %+v", a)
	}
}

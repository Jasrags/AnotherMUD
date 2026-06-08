package progression

import "testing"

// fixedRoller is declared in level_up_test.go (returns v % n); reused here.

func newGainMgr(t *testing.T, baseChance int) *ProficiencyManager {
	t.Helper()
	reg := NewAbilityRegistry()
	if err := reg.Register(&Ability{
		ID: "smithing", DisplayName: "Smithing",
		Type: AbilityPassive, Category: AbilitySkill,
		DefaultCap: 100, GainBaseChance: baseChance,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	return NewProficiencyManager(reg, DefaultProficiencyConfig())
}

func TestRollUseGain_LearnedAbilityGains(t *testing.T) {
	m := newGainMgr(t, 100) // base chance 100 → threshold high at low prof
	m.Learn("e1", "smithing", 1)
	// roller IntN(100) returns 0 → 0+1=1 <= threshold → gains.
	if !m.RollUseGain("e1", "smithing", true, fixedRoller{v: 0}, nil) {
		t.Fatal("expected a gain at base chance 100, prof 1")
	}
	if prof, _ := m.Proficiency("e1", "smithing"); prof != 2 {
		t.Errorf("proficiency = %d, want 2 after one gain", prof)
	}
}

func TestRollUseGain_UnlearnedNeverGains(t *testing.T) {
	m := newGainMgr(t, 100)
	// Not learned → no gain regardless of roll.
	if m.RollUseGain("e1", "smithing", true, fixedRoller{v: 0}, nil) {
		t.Error("unlearned ability should not gain")
	}
}

func TestRollUseGain_UnknownAbilityNeverGains(t *testing.T) {
	m := newGainMgr(t, 100)
	m.Learn("e1", "smithing", 1)
	if m.RollUseGain("e1", "nonexistent", true, fixedRoller{v: 0}, nil) {
		t.Error("unknown ability should not gain")
	}
}

func TestRollUseGain_NilRoller(t *testing.T) {
	m := newGainMgr(t, 100)
	m.Learn("e1", "smithing", 1)
	if m.RollUseGain("e1", "smithing", true, nil, nil) {
		t.Error("nil roller should not gain")
	}
}

func TestRollUseGain_HighRollMisses(t *testing.T) {
	m := newGainMgr(t, 30) // modest base chance
	m.Learn("e1", "smithing", 1)
	// roller returns 99 → 99+1=100 > threshold (well under 100) → no gain.
	if m.RollUseGain("e1", "smithing", true, fixedRoller{v: 99}, nil) {
		t.Error("a roll above the threshold should not gain")
	}
}

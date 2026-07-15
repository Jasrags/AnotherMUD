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

func TestRollUseGain_GainObserverFiresOnIncrement(t *testing.T) {
	m := newGainMgr(t, 100)
	m.Learn("e1", "smithing", 5)

	type call struct {
		entity, ability string
		oldP, newP      int
	}
	var got []call
	m.SetGainObserver(func(entityID, abilityID string, oldProf, newProf int) {
		got = append(got, call{entityID, abilityID, oldProf, newProf})
	})

	// A gain fires the observer with the before/after proficiency.
	if !m.RollUseGain("e1", "smithing", true, fixedRoller{v: 0}, nil) {
		t.Fatal("expected a gain")
	}
	if len(got) != 1 || got[0] != (call{"e1", "smithing", 5, 6}) {
		t.Fatalf("observer calls = %+v, want one (e1, smithing, 5, 6)", got)
	}

	// A non-gain (high roll → threshold miss) must NOT fire the observer.
	if m.RollUseGain("e1", "smithing", true, fixedRoller{v: 99}, nil) {
		t.Fatal("expected no gain on a high roll at prof 6")
	}
	if len(got) != 1 {
		t.Errorf("observer fired on a non-gain: %+v", got)
	}
}

func TestRollUseGain_NilGainObserverIsSafe(t *testing.T) {
	m := newGainMgr(t, 100)
	m.Learn("e1", "smithing", 1)
	// No observer set — a gain must not panic.
	if !m.RollUseGain("e1", "smithing", true, fixedRoller{v: 0}, nil) {
		t.Fatal("expected a gain")
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

// fakeStats returns a fixed value for every stat query.
type fakeStats struct{ v int }

func (f fakeStats) StatValue(string, StatType) int { return f.v }

func TestRollUseGain_GainStatRaisesChance(t *testing.T) {
	// Ability with a low base chance but a gain_stat: a high stat value
	// should lift the threshold enough that a roll which misses without the
	// stat factor now hits.
	reg := NewAbilityRegistry()
	if err := reg.Register(&Ability{
		ID: "smithing", DisplayName: "Smithing",
		Type: AbilityPassive, Category: AbilitySkill,
		DefaultCap: 100, GainBaseChance: 10,
		GainStat: StatDEX, GainStatScale: 0.1,
	}); err != nil {
		t.Fatal(err)
	}
	m := NewProficiencyManager(reg, DefaultProficiencyConfig())
	m.Learn("e1", "smithing", 1)

	// roller returns 49 → 49+1 = 50. Base threshold ~10 (miss); with a DEX
	// of 50 the factor is 1+50*0.1 = 6, lifting the threshold well above 50.
	if m.RollUseGain("e1", "smithing", true, fixedRoller{v: 49}, nil) {
		t.Fatal("without a stat reader, a roll of 50 vs base chance 10 should miss")
	}
	// Re-learn to reset proficiency (the miss didn't change it; still 1).
	if !m.RollUseGain("e1", "smithing", true, fixedRoller{v: 49}, fakeStats{v: 50}) {
		t.Error("with DEX 50 the gain-stat factor should lift the threshold past 50")
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

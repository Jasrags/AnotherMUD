package gathering

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

func TestGainFromUse_NilProfIsNoop(t *testing.T) {
	s := NewService(coreLadder(), nil, fixedRoller{v: 0}, DefaultConfig(), nil, nil, nil)
	if s.GainFromUse("p1") {
		t.Error("GainFromUse with no proficiency manager should be a no-op (false)")
	}
}

func TestGainFromUse_IncrementsLearnedProficiency(t *testing.T) {
	abilities := progression.NewAbilityRegistry()
	if err := abilities.Register(&progression.Ability{
		ID: GatheringAbility, DisplayName: "Gathering",
		Type: progression.AbilityPassive, Category: progression.AbilitySkill,
		DefaultCap: 100, GainBaseChance: 50,
	}); err != nil {
		t.Fatal(err)
	}
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())
	prof.Learn("p1", GatheringAbility, 5)

	// fixedRoller{v:0} → IntN(100)=0 → roll 1 ≤ threshold → a gain.
	s := NewService(coreLadder(), prof, fixedRoller{v: 0}, DefaultConfig(), nil, nil, nil)
	if !s.GainFromUse("p1") {
		t.Fatal("GainFromUse should report a gain with a min roll + positive base chance")
	}
	if v, _ := prof.Proficiency("p1", GatheringAbility); v != 6 {
		t.Errorf("gathering proficiency = %d, want 6 (gained once)", v)
	}
}

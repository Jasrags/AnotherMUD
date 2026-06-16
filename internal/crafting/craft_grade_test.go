package crafting

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// gradeForgeFixture builds a smithing service whose output (a blade) declares a
// quality_grades map (masterwork §7/§9). smithSkill controls the rolled tier.
func gradeForgeFixture(t *testing.T, smithSkill int) (*Service, *fakeCrafter, *entities.Store) {
	t.Helper()
	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "core:ingot", Name: "an ingot", Type: "item"})
	tpls.Add(&item.Template{
		ID: "core:blade", Name: "a blade", Type: "weapon", WeaponDamage: "1d8",
		Properties: map[string]any{
			"quality_grades": map[string]any{
				"uncommon": "masterwork",
				"rare":     "power-wrought",
			},
		},
	})
	store := entities.NewStore()

	recipes := recipe.NewRegistry()
	_ = recipes.TryAdd(&recipe.Recipe{
		ID: "core:forge", DisplayName: "forge a blade", Discipline: "smithing", SkillFloor: 1,
		Inputs: []recipe.Ingredient{{Template: "core:ingot", Quantity: 1}},
		Output: recipe.Output{Template: "core:blade", Quantity: 1},
	})
	known := recipe.NewKnownManager(recipes)
	known.Learn("p1", "core:forge")

	abilities := progression.NewAbilityRegistry()
	_ = abilities.Register(&progression.Ability{
		ID: "smithing", DisplayName: "Smithing",
		Type: progression.AbilityPassive, Category: progression.AbilitySkill, DefaultCap: 100,
	})
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())
	prof.Learn("p1", "smithing", smithSkill)

	s := NewService(tpls, store, recipes, known, prof, coreLadder(), fixedRoller{}, noBand(), nil)

	crafter := &fakeCrafter{id: "p1", failRemoveAt: -1}
	inst, err := store.Spawn(&item.Template{ID: "core:ingot", Name: "an ingot", Type: "item"})
	if err != nil {
		t.Fatal(err)
	}
	crafter.AddToInventory(inst.ID())
	return s, crafter, store
}

// A high-quality craft stamps the quality grade onto the result, so the blade
// is mechanically masterwork (masterwork §7).
func TestCraft_StampsGradeAtQuality(t *testing.T) {
	// High skill at Tier 0 → output caps at uncommon → uncommon's grade
	// (masterwork) is stamped as the instance's grade.
	s, crafter, store := gradeForgeFixture(t, 100)
	res := s.Craft(context.Background(), crafter, "forge a blade", nil)
	if res.Outcome != CraftOK {
		t.Fatalf("outcome = %v (%q)", res.Outcome, res.Message)
	}
	if res.QualityKey != "uncommon" {
		t.Fatalf("quality = %q, want uncommon (Tier-0 ceiling)", res.QualityKey)
	}
	out := craftedOutput(t, crafter, store)
	if got := out.Grade(); got != "masterwork" {
		t.Errorf("crafted blade grade = %q, want masterwork", got)
	}
}

// A common-quality craft (below the lowest mapped tier) stamps no grade —
// an ordinary blade.
func TestCraft_NoGradeBelowLowestTier(t *testing.T) {
	s, crafter, store := gradeForgeFixture(t, 1)
	res := s.Craft(context.Background(), crafter, "forge a blade", nil)
	if res.Outcome != CraftOK {
		t.Fatalf("outcome = %v (%q)", res.Outcome, res.Message)
	}
	if res.QualityKey != "common" {
		t.Fatalf("quality = %q, want common", res.QualityKey)
	}
	out := craftedOutput(t, crafter, store)
	if got := out.Grade(); got != "" {
		t.Errorf("common blade should be ungraded, got %q", got)
	}
}

// qualityGradeFor picks the grade of the highest tier at or below the rolled
// quality (mirrors qualityEffectFor).
func TestQualityGradeFor_PicksHighestTierAtOrBelow(t *testing.T) {
	s := svc(coreLadder(), fixedRoller{}, noBand())
	tpl := &item.Template{Properties: map[string]any{
		"quality_grades": map[string]any{"uncommon": "masterwork", "legendary": "power-wrought"},
	}}
	if got := s.qualityGradeFor(tpl, "rare"); got != "masterwork" {
		t.Errorf("qualityGradeFor(rare) = %q, want masterwork (highest at/below)", got)
	}
	if got := s.qualityGradeFor(tpl, "legendary"); got != "power-wrought" {
		t.Errorf("qualityGradeFor(legendary) = %q, want power-wrought", got)
	}
	if got := s.qualityGradeFor(tpl, "common"); got != "" {
		t.Errorf("qualityGradeFor(common) = %q, want \"\"", got)
	}
}

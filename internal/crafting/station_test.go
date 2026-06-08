package crafting

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// stationFixture builds a service for a recipe with the given station_tier
// and skill, no RNG band (deterministic), common ingredient.
func stationFixture(t *testing.T, recipeStationTier, skill int) (*Service, *fakeCrafter) {
	t.Helper()
	tpls := item.NewTemplates()
	in := &item.Template{ID: "core:iron", Name: "iron", Type: "item"}
	tpls.Add(in)
	tpls.Add(&item.Template{ID: "core:sword", Name: "a sword", Type: "item"})
	store := entities.NewStore()

	recipes := recipe.NewRegistry()
	_ = recipes.TryAdd(&recipe.Recipe{
		ID: "core:forge", DisplayName: "forge a sword", Discipline: "smithing",
		SkillFloor: 1, StationTier: recipeStationTier,
		Inputs: []recipe.Ingredient{{Template: "core:iron", Quantity: 1}},
		Output: recipe.Output{Template: "core:sword", Quantity: 1},
	})
	known := recipe.NewKnownManager(recipes)
	known.Learn("p1", "core:forge")

	abilities := progression.NewAbilityRegistry()
	_ = abilities.Register(&progression.Ability{
		ID: "smithing", DisplayName: "Smithing",
		Type: progression.AbilityPassive, Category: progression.AbilitySkill, DefaultCap: 100,
	})
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())
	prof.Learn("p1", "smithing", skill)

	s := NewService(tpls, store, recipes, known, prof, coreLadder(), fixedRoller{}, noBand(), nil)

	crafter := &fakeCrafter{id: "p1", failRemoveAt: -1}
	inst, _ := store.Spawn(in)
	crafter.AddToInventory(inst.ID())
	return s, crafter
}

func tierFn(n int) StationTierFunc { return func(string) int { return n } }

func TestCraft_StationGateRefusesBelowMinimum(t *testing.T) {
	s, crafter := stationFixture(t, 2, 50) // recipe needs Tier 2
	res := s.Craft(context.Background(), crafter, "forge a sword", tierFn(1))
	if res.Outcome != CraftNoStation {
		t.Fatalf("Tier-1 present vs Tier-2 recipe: outcome = %v (%q), want CraftNoStation", res.Outcome, res.Message)
	}
	// Input untouched (no consume on a refused craft).
	if len(crafter.inv) != 1 {
		t.Errorf("inventory changed on refused craft: %d items", len(crafter.inv))
	}
}

func TestCraft_StationGatePassesAtMinimum(t *testing.T) {
	s, crafter := stationFixture(t, 2, 50)
	res := s.Craft(context.Background(), crafter, "forge a sword", tierFn(2))
	if res.Outcome != CraftOK {
		t.Fatalf("Tier-2 present vs Tier-2 recipe: outcome = %v (%q), want CraftOK", res.Outcome, res.Message)
	}
}

func TestCraft_HigherStationRaisesQuality(t *testing.T) {
	// Same station_tier-0 recipe + max skill + common ingredient: a Tier-2
	// station reaches a higher tier than the field (Tier 0), demonstrating
	// the station ceiling (the §4 "town beats field" property).
	sField, cField := stationFixture(t, 0, 100)
	field := sField.Craft(context.Background(), cField, "forge a sword", tierFn(0))

	sForge, cForge := stationFixture(t, 0, 100)
	forge := sForge.Craft(context.Background(), cForge, "forge a sword", tierFn(2))

	if field.Outcome != CraftOK || forge.Outcome != CraftOK {
		t.Fatalf("both should succeed: field=%v forge=%v", field.Outcome, forge.Outcome)
	}
	if ladderPosition(coreLadder(), forge.QualityKey) <= ladderPosition(coreLadder(), field.QualityKey) {
		t.Errorf("station should raise quality: field=%q forge=%q", field.QualityKey, forge.QualityKey)
	}
}

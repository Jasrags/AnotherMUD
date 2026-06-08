package crafting

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// cookFixture builds a cooking service whose output (cooked-meal) declares a
// quality_effects map. cookSkill controls the rolled tier (no RNG band).
func cookFixture(t *testing.T, cookSkill int) (*Service, *fakeCrafter, *entities.Store) {
	t.Helper()
	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "core:raw-meat", Name: "raw meat", Type: "item"})
	tpls.Add(&item.Template{
		ID: "core:meal", Name: "a cooked meal", Type: "item",
		Properties: map[string]any{
			"consume_method":   "eat",
			"sustenance_value": 40,
			"quality_effects": map[string]any{
				"uncommon": "well-fed-minor",
				"rare":     "well-fed",
			},
		},
	})
	store := entities.NewStore()

	recipes := recipe.NewRegistry()
	_ = recipes.TryAdd(&recipe.Recipe{
		ID: "core:cook", DisplayName: "cook a meal", Discipline: "cooking", SkillFloor: 1,
		Inputs: []recipe.Ingredient{{Template: "core:raw-meat", Quantity: 1}},
		Output: recipe.Output{Template: "core:meal", Quantity: 1},
	})
	known := recipe.NewKnownManager(recipes)
	known.Learn("p1", "core:cook")

	abilities := progression.NewAbilityRegistry()
	_ = abilities.Register(&progression.Ability{
		ID: "cooking", DisplayName: "Cooking",
		Type: progression.AbilityPassive, Category: progression.AbilitySkill, DefaultCap: 100,
	})
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())
	prof.Learn("p1", "cooking", cookSkill)

	s := NewService(tpls, store, recipes, known, prof, coreLadder(), fixedRoller{}, noBand())

	crafter := &fakeCrafter{id: "p1", failRemoveAt: -1}
	inst, err := store.Spawn(&item.Template{ID: "core:raw-meat", Name: "raw meat", Type: "item"})
	if err != nil {
		t.Fatal(err)
	}
	crafter.AddToInventory(inst.ID())
	return s, crafter, store
}

func craftedOutput(t *testing.T, crafter *fakeCrafter, store *entities.Store) *entities.ItemInstance {
	t.Helper()
	if len(crafter.inv) != 1 {
		t.Fatalf("inventory has %d items, want 1 output", len(crafter.inv))
	}
	e, ok := store.GetByID(crafter.inv[0])
	if !ok {
		t.Fatal("output not in store")
	}
	return e.(*entities.ItemInstance)
}

func TestCraft_CookingStampsWellFedAtQuality(t *testing.T) {
	// High cooking skill at Tier 0 → output caps at uncommon → the
	// uncommon well-fed effect is stamped as the instance's effect_id.
	s, crafter, store := cookFixture(t, 100)
	res := s.Craft(context.Background(), crafter, "cook a meal")
	if res.Outcome != CraftOK {
		t.Fatalf("outcome = %v (%q)", res.Outcome, res.Message)
	}
	if res.QualityKey != "uncommon" {
		t.Fatalf("quality = %q, want uncommon (Tier-0 ceiling)", res.QualityKey)
	}
	out := craftedOutput(t, crafter, store)
	if v, ok := out.Property("effect_id"); !ok || v != "well-fed-minor" {
		t.Errorf("effect_id = %v (ok=%v), want well-fed-minor", v, ok)
	}
}

func TestCraft_CookingColdRationNoEffect(t *testing.T) {
	// Low skill + common ingredient → common quality, below the lowest
	// declared tier (uncommon) → no effect stamped (cold ration).
	s, crafter, store := cookFixture(t, 1)
	res := s.Craft(context.Background(), crafter, "cook a meal")
	if res.Outcome != CraftOK {
		t.Fatalf("outcome = %v (%q)", res.Outcome, res.Message)
	}
	if res.QualityKey != "common" {
		t.Fatalf("quality = %q, want common", res.QualityKey)
	}
	out := craftedOutput(t, crafter, store)
	if v, ok := out.Property("effect_id"); ok {
		t.Errorf("effect_id = %v, want none for a common cold ration", v)
	}
}

func TestQualityEffectFor_PicksHighestTierAtOrBelow(t *testing.T) {
	s := svc(coreLadder(), fixedRoller{}, noBand())
	tpl := &item.Template{Properties: map[string]any{
		"quality_effects": map[string]any{"uncommon": "minor", "legendary": "major"},
	}}
	// rare is above uncommon but below legendary → picks uncommon's effect.
	if got := s.qualityEffectFor(tpl, "rare"); got != "minor" {
		t.Errorf("qualityEffectFor(rare) = %q, want minor", got)
	}
	// legendary → its own.
	if got := s.qualityEffectFor(tpl, "legendary"); got != "major" {
		t.Errorf("qualityEffectFor(legendary) = %q, want major", got)
	}
	// common is below the lowest declared tier → none.
	if got := s.qualityEffectFor(tpl, "common"); got != "" {
		t.Errorf("qualityEffectFor(common) = %q, want \"\"", got)
	}
}

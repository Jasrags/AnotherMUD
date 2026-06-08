package crafting

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// toolFixture builds a service for a recipe that uses a "hammer" tool. The
// crafter always carries the input; withHammer adds a legendary-rarity
// hammer (tag "hammer") that should NOT be consumed but should raise quality.
func toolFixture(t *testing.T, withHammer bool) (*Service, *fakeCrafter, entities.EntityID) {
	t.Helper()
	tpls := item.NewTemplates()
	in := &item.Template{ID: "core:iron", Name: "iron", Type: "item"}
	tpls.Add(in)
	tpls.Add(&item.Template{ID: "core:sword", Name: "a sword", Type: "item"})
	store := entities.NewStore()

	recipes := recipe.NewRegistry()
	_ = recipes.TryAdd(&recipe.Recipe{
		ID: "core:forge", DisplayName: "forge a sword", Discipline: "smithing",
		SkillFloor: 1, Tool: "hammer",
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
	prof.Learn("p1", "smithing", 40)

	s := NewService(tpls, store, recipes, known, prof, coreLadder(), fixedRoller{}, noBand())

	crafter := &fakeCrafter{id: "p1", failRemoveAt: -1}
	ironInst, _ := store.Spawn(in)
	// Legendary ingredient so the soft ceiling doesn't bind — isolates the
	// tool's contribution to the roll.
	ironInst.SetProperty("rarity", "legendary")
	crafter.AddToInventory(ironInst.ID())

	var hammerID entities.EntityID
	if withHammer {
		hammer, _ := store.Spawn(&item.Template{
			ID: "core:hammer", Name: "a fine hammer", Type: "item",
			Tags: []string{"hammer"},
		})
		hammer.SetProperty("rarity", "legendary")
		crafter.AddToInventory(hammer.ID())
		hammerID = hammer.ID()
	}
	return s, crafter, hammerID
}

func TestCraft_ToolQualityRaisesQuality(t *testing.T) {
	// Same skill + legendary ingredients at a Tier-2 station; a legendary
	// hammer should produce a strictly higher tier than no hammer.
	sNo, cNo, _ := toolFixture(t, false)
	noTool := sNo.Craft(context.Background(), cNo, "forge a sword", tierFn(2))

	sYes, cYes, hammerID := toolFixture(t, true)
	withTool := sYes.Craft(context.Background(), cYes, "forge a sword", tierFn(2))

	if noTool.Outcome != CraftOK || withTool.Outcome != CraftOK {
		t.Fatalf("both should succeed: no=%v with=%v", noTool.Outcome, withTool.Outcome)
	}
	if ladderPosition(coreLadder(), withTool.QualityKey) <= ladderPosition(coreLadder(), noTool.QualityKey) {
		t.Errorf("tool should raise quality: noTool=%q withTool=%q", noTool.QualityKey, withTool.QualityKey)
	}
	// The hammer is a tool, not an ingredient — it must NOT be consumed.
	if _, ok := sYes.store.GetByID(hammerID); !ok {
		t.Error("the hammer (tool) was consumed by the craft")
	}
}

func TestToolTierKey_BestMatchAndNoTool(t *testing.T) {
	s, c, _ := toolFixture(t, true)
	if got := s.toolTierKey(c, "hammer"); got != "legendary" {
		t.Errorf("toolTierKey(hammer) = %q, want legendary", got)
	}
	if got := s.toolTierKey(c, ""); got != "" {
		t.Errorf("toolTierKey(no tool) = %q, want \"\"", got)
	}
	if got := s.toolTierKey(c, "anvil"); got != "" {
		t.Errorf("toolTierKey(absent tool) = %q, want \"\"", got)
	}
}

package pack

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/slot"
)

// addValuedItem registers a template carrying just a `value` property — the
// only field validateRecipeEconomy reads.
func addValuedItem(regs *Registries, id string, value int) {
	regs.Items.Add(&item.Template{
		ID:         item.TemplateID(id),
		Properties: map[string]any{"value": value},
	})
}

// TestValidateRecipeEconomy_FlagsMoneyLosers checks the D3 guardrail flags a
// recipe whose output is worth no more than its inputs, and leaves a
// value-adding recipe alone (crafting-and-cooking §8 / plan D2.1).
func TestValidateRecipeEconomy_FlagsMoneyLosers(t *testing.T) {
	regs := NewRegistries()
	// Inputs and outputs across the cases.
	addValuedItem(regs, "p:ore", 3)
	addValuedItem(regs, "p:ingot", 8)    // healthy output (> 2*3)
	addValuedItem(regs, "p:trinket", 5)  // money-loser output (< 2*3)
	addValuedItem(regs, "p:break-even", 6) // exactly equal -> still flagged (<=)

	healthy := &recipe.Recipe{
		ID:     "p:smelt",
		Output: recipe.Output{Template: "p:ingot", Quantity: 1},
		Inputs: []recipe.Ingredient{{Template: "p:ore", Quantity: 2}},
	}
	loser := &recipe.Recipe{
		ID:     "p:waste",
		Output: recipe.Output{Template: "p:trinket", Quantity: 1},
		Inputs: []recipe.Ingredient{{Template: "p:ore", Quantity: 2}},
	}
	breakEven := &recipe.Recipe{
		ID:     "p:even",
		Output: recipe.Output{Template: "p:break-even", Quantity: 1},
		Inputs: []recipe.Ingredient{{Template: "p:ore", Quantity: 2}},
	}
	regs.Recipes.Add(healthy)
	regs.Recipes.Add(loser)
	regs.Recipes.Add(breakEven)

	warns := validateRecipeEconomy(regs)

	flagged := map[recipe.RecipeID]recipeEconomyWarning{}
	for _, w := range warns {
		flagged[w.Recipe] = w
	}
	if _, ok := flagged["p:smelt"]; ok {
		t.Errorf("healthy recipe p:smelt was flagged (output 8 > inputs 6)")
	}
	w, ok := flagged["p:waste"]
	if !ok {
		t.Fatalf("money-loser p:waste was NOT flagged")
	}
	if w.OutputValue != 5 || w.InputValue != 6 {
		t.Errorf("p:waste warning = out %d in %d, want out 5 in 6", w.OutputValue, w.InputValue)
	}
	if _, ok := flagged["p:even"]; !ok {
		t.Errorf("break-even recipe p:even was NOT flagged (output == inputs is still a smell)")
	}
}

// TestValidateRecipeEconomy_SkipsUnknownOutput confirms a recipe whose output
// template is unknown is skipped (can't assess), not flagged.
func TestValidateRecipeEconomy_SkipsUnknownOutput(t *testing.T) {
	regs := NewRegistries()
	addValuedItem(regs, "p:ore", 3)
	regs.Recipes.Add(&recipe.Recipe{
		ID:     "p:ghost",
		Output: recipe.Output{Template: "p:nonexistent", Quantity: 1},
		Inputs: []recipe.Ingredient{{Template: "p:ore", Quantity: 2}},
	})
	if warns := validateRecipeEconomy(regs); len(warns) != 0 {
		t.Errorf("unknown-output recipe was flagged: %+v", warns)
	}
}

// TestValidateRecipeEconomy_CorePackClean is the regression guard: every
// recipe in the shipped core pack must add value (the D2.1 content discipline
// Milestone C established). If a future content edit breaks it, this fails.
func TestValidateRecipeEconomy_CorePackClean(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("register engine baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("register engine baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load core: %v", err)
	}

	if warns := validateRecipeEconomy(regs); len(warns) != 0 {
		for _, w := range warns {
			t.Errorf("core recipe %s loses money: output %d <= inputs %d", w.Recipe, w.OutputValue, w.InputValue)
		}
	}
}

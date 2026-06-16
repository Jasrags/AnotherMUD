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
	addValuedItem(regs, "p:ingot", 8)      // healthy output (> 2*3)
	addValuedItem(regs, "p:trinket", 5)    // money-loser output (< 2*3)
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

// TestValidateRecipeEconomy_UnknownInputContributesZero confirms a recipe is
// still assessed when one input template is unknown — the unknown input
// counts as 0 (lenient), so it never inflates the input sum into a false
// positive, and a genuine money-loser among the KNOWN inputs is still caught.
func TestValidateRecipeEconomy_UnknownInputContributesZero(t *testing.T) {
	regs := NewRegistries()
	addValuedItem(regs, "p:ore", 3)
	addValuedItem(regs, "p:cheap", 1) // output worth less than the known input

	// One known input (value 3) + one unknown input (counts 0) → sumIn = 3.
	// Output value 1 ≤ 3, so the recipe is still flagged: the unknown input
	// neither skipped the assessment nor masked the real shortfall.
	regs.Recipes.Add(&recipe.Recipe{
		ID:     "p:mixed",
		Output: recipe.Output{Template: "p:cheap", Quantity: 1},
		Inputs: []recipe.Ingredient{
			{Template: "p:ore", Quantity: 1},
			{Template: "p:missing", Quantity: 5},
		},
	})

	warns := validateRecipeEconomy(regs)
	if len(warns) != 1 {
		t.Fatalf("got %d warnings, want 1: %+v", len(warns), warns)
	}
	if warns[0].Recipe != "p:mixed" || warns[0].InputValue != 3 {
		t.Errorf("warning = %+v, want recipe p:mixed with InputValue 3 (unknown input = 0)", warns[0])
	}
}

// TestValidateRecipeEconomy_CorePackClean is the regression guard: every
// recipe in each shipped world must add value (the D2.1 content discipline
// Milestone C established). If a future content edit breaks it, this fails.
// Each world is loaded on its own — a boot selects one world, and the two
// world packs share bare-global biome ids that would collide if co-loaded.
func TestValidateRecipeEconomy_CorePackClean(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	for _, world := range []string{"starter-world", "wot"} {
		t.Run(world, func(t *testing.T) {
			regs := NewRegistries()
			if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
				t.Fatalf("register engine baseline properties: %v", err)
			}
			if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
				t.Fatalf("register engine baseline slots: %v", err)
			}
			if err := Load(context.Background(), root, []string{world}, regs, nil, nil, nil); err != nil {
				t.Fatalf("Load %s: %v", world, err)
			}
			for _, w := range validateRecipeEconomy(regs) {
				t.Errorf("%s recipe %s loses money: output %d <= inputs %d", world, w.Recipe, w.OutputValue, w.InputValue)
			}
		})
	}
}

package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// learnFixture wires the pieces the learn verb reads: a smithing
// discipline ability + proficiency manager, a recipe registry with one
// baseline and one common smithing recipe, a known-recipe manager, and a
// training manager whose in-room trainer teaches smithing.
func learnFixture(t *testing.T, trainer bool) (*trainActor, *command.Context) {
	t.Helper()

	abilities := progression.NewAbilityRegistry()
	if err := abilities.Register(&progression.Ability{
		ID: "smithing", DisplayName: "Smithing",
		Type: progression.AbilityPassive, Category: progression.AbilitySkill,
		DefaultCap: 25,
	}); err != nil {
		t.Fatalf("register ability: %v", err)
	}
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())

	recipes := recipe.NewRegistry()
	base := &recipe.Recipe{
		ID: "core:reforge", DisplayName: "reforge", Discipline: "smithing",
		Acquisition: recipe.AcqBaseline,
		Inputs:      []recipe.Ingredient{{Template: "core:rusty", Quantity: 1}},
		Output:      recipe.Output{Template: "core:sword", Quantity: 1},
	}
	common := &recipe.Recipe{
		ID: "core:fancy", DisplayName: "fancy", Discipline: "smithing",
		Acquisition: recipe.AcqCommon,
		Inputs:      []recipe.Ingredient{{Template: "core:ingot", Quantity: 1}},
		Output:      recipe.Output{Template: "core:plate", Quantity: 1},
	}
	if err := recipes.TryAdd(base); err != nil {
		t.Fatal(err)
	}
	if err := recipes.TryAdd(common); err != nil {
		t.Fatal(err)
	}
	known := recipe.NewKnownManager(recipes)

	var tm *progression.TrainingManager
	if trainer {
		tm, _ = newTrainingMgr(t, &progression.TrainerConfig{
			Tier: progression.CapApprentice, Teach: []string{"smithing"},
		}, "Brandr")
	} else {
		tm, _ = newTrainingMgr(t, nil, "")
	}

	a := newTrainActor()
	c := &command.Context{
		Actor: a, Verb: "learn",
		Training: tm, Proficiency: prof, Abilities: abilities,
		Recipes: recipes, Known: known,
	}
	return a, c
}

func TestLearn_AtTrainerGrantsDisciplineAndBaselineRecipes(t *testing.T) {
	a, c := learnFixture(t, true)
	c.Args = []string{"smithing"}
	if err := command.LearnHandler(context.Background(), c); err != nil {
		t.Fatalf("LearnHandler: %v", err)
	}
	line := a.lastLine()
	if !strings.Contains(line, "Smithing") {
		t.Errorf("output = %q, want it to mention Smithing", line)
	}
	// Proficiency seeded.
	if !c.Proficiency.Has(c.Actor.ID(), "smithing") {
		t.Error("smithing proficiency not seeded")
	}
	// Baseline recipe known; common one not.
	if !c.Known.Knows(c.Actor.ID(), "core:reforge") {
		t.Error("baseline recipe not granted")
	}
	if c.Known.Knows(c.Actor.ID(), "core:fancy") {
		t.Error("common recipe should NOT be granted on learn")
	}
	if !strings.Contains(line, "1 starting recipe") {
		t.Errorf("output = %q, want '1 starting recipe'", line)
	}
}

func TestLearn_NoTrainerRefused(t *testing.T) {
	a, c := learnFixture(t, false)
	c.Args = []string{"smithing"}
	_ = command.LearnHandler(context.Background(), c)
	if !strings.Contains(strings.ToLower(a.lastLine()), "no one here") {
		t.Errorf("output = %q, want 'no one here'", a.lastLine())
	}
	if c.Known.Knows(c.Actor.ID(), "core:reforge") {
		t.Error("recipe granted without a trainer")
	}
}

func TestLearn_AlreadyKnown(t *testing.T) {
	a, c := learnFixture(t, true)
	c.Proficiency.Learn(c.Actor.ID(), "smithing", 1)
	c.Args = []string{"smithing"}
	_ = command.LearnHandler(context.Background(), c)
	if !strings.Contains(strings.ToLower(a.lastLine()), "already know") {
		t.Errorf("output = %q, want 'already know'", a.lastLine())
	}
}

func TestLearn_UnknownCraft(t *testing.T) {
	a, c := learnFixture(t, true)
	c.Args = []string{"basketweaving"}
	_ = command.LearnHandler(context.Background(), c)
	if !strings.Contains(strings.ToLower(a.lastLine()), "no such craft") {
		t.Errorf("output = %q, want 'no such craft'", a.lastLine())
	}
}

func TestLearn_UnregisteredDisciplineAbilityRefused(t *testing.T) {
	// A recipe references a discipline with no registered ability (content
	// bug). learn must refuse, NOT seed a default-cap proficiency.
	a, c := learnFixture(t, true)
	// Add a recipe whose discipline ("alchemy") has no registered ability.
	_ = c.Recipes.TryAdd(&recipe.Recipe{
		ID: "core:brew", DisplayName: "brew", Discipline: "alchemy",
		Acquisition: recipe.AcqBaseline,
		Inputs:      []recipe.Ingredient{{Template: "core:herb", Quantity: 1}},
		Output:      recipe.Output{Template: "core:potion", Quantity: 1},
	})
	c.Args = []string{"alchemy"}
	_ = command.LearnHandler(context.Background(), c)
	if !strings.Contains(strings.ToLower(a.lastLine()), "no such craft") {
		t.Errorf("output = %q, want 'no such craft'", a.lastLine())
	}
	if c.Proficiency.Has(c.Actor.ID(), "alchemy") {
		t.Error("alchemy proficiency was seeded despite no registered ability")
	}
}

func TestLearn_NoArgs(t *testing.T) {
	a, c := learnFixture(t, true)
	_ = command.LearnHandler(context.Background(), c)
	if !strings.Contains(strings.ToLower(a.lastLine()), "learn what") {
		t.Errorf("output = %q, want 'learn what'", a.lastLine())
	}
}

func TestLearn_NotEnabled(t *testing.T) {
	a := newTrainActor()
	c := &command.Context{Actor: a, Verb: "learn", Args: []string{"smithing"}}
	_ = command.LearnHandler(context.Background(), c)
	if !strings.Contains(strings.ToLower(a.lastLine()), "not enabled") {
		t.Errorf("output = %q, want 'not enabled'", a.lastLine())
	}
}

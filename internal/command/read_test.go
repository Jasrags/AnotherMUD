package command_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// readRig builds a recipe registry with one recipe, a KnownManager, a store
// holding one item (from props), and a testActor carrying it. Tests
// dispatch through the registry so the ArgInventory `item` arg resolves.
func readRig(t *testing.T, props map[string]any) (command.Env, *testActor, *recipe.KnownManager) {
	t.Helper()
	recipes := recipe.NewRegistry()
	if err := recipes.TryAdd(&recipe.Recipe{
		ID: "core:forge-sword", DisplayName: "forge a sword", Discipline: "smithing",
		Inputs: []recipe.Ingredient{{Template: "core:iron", Quantity: 1}},
		Output: recipe.Output{Template: "core:sword", Quantity: 1},
	}); err != nil {
		t.Fatal(err)
	}
	known := recipe.NewKnownManager(recipes)

	tpl := &item.Template{ID: "scroll", Name: "a recipe scroll", Type: "item", Keywords: []string{"scroll"}, Properties: props}
	store := entities.NewStore()
	inst, err := store.Spawn(tpl)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a := newTestActor(&world.Room{ID: "x:1"})
	a.playerID = "p1"
	a.inventory = []entities.EntityID{inst.ID()}

	return command.Env{Items: store, Recipes: recipes, Known: known}, a, known
}

func dispatchRead(t *testing.T, env command.Env, a *testActor, line string) {
	t.Helper()
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if err := r.Dispatch(context.Background(), env, a, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func TestReadVerb_LearnsRecipeAndConsumesScroll(t *testing.T) {
	env, a, known := readRig(t, map[string]any{"recipe": "core:forge-sword"})
	dispatchRead(t, env, a, "read scroll")

	if !known.Knows("p1", "core:forge-sword") {
		t.Error("reading the scroll should have taught the recipe")
	}
	if len(a.inventory) != 0 {
		t.Errorf("inventory = %v, want empty (scroll consumed)", a.inventory)
	}
	if a.lastLine() != "You study a recipe scroll and learn how to forge a sword." {
		t.Errorf("output = %q", a.lastLine())
	}
}

func TestReadVerb_AlreadyKnownRefusedAndKept(t *testing.T) {
	env, a, known := readRig(t, map[string]any{"recipe": "core:forge-sword"})
	known.Learn("p1", "core:forge-sword") // already known

	dispatchRead(t, env, a, "read scroll")

	if len(a.inventory) != 1 {
		t.Errorf("inventory = %v, want 1 (known scroll is kept)", a.inventory)
	}
	if a.lastLine() != "You already know how to forge a sword." {
		t.Errorf("output = %q, want already-known message", a.lastLine())
	}
}

func TestReadVerb_NoRecipePropertyKept(t *testing.T) {
	env, a, _ := readRig(t, map[string]any{"weight": 1})
	dispatchRead(t, env, a, "read scroll")

	if len(a.inventory) != 1 {
		t.Error("an item with no recipe property must not be consumed")
	}
	if a.lastLine() != "There's nothing to learn from a recipe scroll." {
		t.Errorf("output = %q, want nothing-to-learn message", a.lastLine())
	}
}

func TestReadVerb_UnknownRecipeIDKept(t *testing.T) {
	env, a, known := readRig(t, map[string]any{"recipe": "core:does-not-exist"})
	dispatchRead(t, env, a, "read scroll")

	if known.Knows("p1", "core:does-not-exist") {
		t.Error("a scroll naming an absent recipe must not teach it")
	}
	if len(a.inventory) != 1 {
		t.Error("a scroll for an absent recipe must be kept, not destroyed")
	}
	if a.lastLine() != "The instructions on a recipe scroll make no sense to you." {
		t.Errorf("output = %q, want make-no-sense message", a.lastLine())
	}
}

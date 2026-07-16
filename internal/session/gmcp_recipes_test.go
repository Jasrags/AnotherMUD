package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/crafting"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// recipeFormService builds a crafting.Service over store with one known recipe
// (2 iron bars → a sword, smithing floor 1) that playerID knows and is skilled
// for. Returns the service and a spawned iron-bar item id the caller can add to
// the actor's bag. A nil rarity/roller is fine — CraftForm never rolls quality
// and the recipe declares no ingredient min-quality.
func recipeFormService(t *testing.T, store *entities.Store, playerID string) *crafting.Service {
	t.Helper()

	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "core:iron", Name: "an iron bar", Type: "item"})
	tpls.Add(&item.Template{ID: "core:sword", Name: "a sword", Type: "item"})

	recipes := recipe.NewRegistry()
	if err := recipes.TryAdd(&recipe.Recipe{
		ID: "core:forge-sword", DisplayName: "forge a sword", Discipline: "smithing",
		SkillFloor: 1,
		Inputs:     []recipe.Ingredient{{Template: "core:iron", Quantity: 2}},
		Output:     recipe.Output{Template: "core:sword", Quantity: 1},
	}); err != nil {
		t.Fatal(err)
	}

	known := recipe.NewKnownManager(recipes)
	known.Learn(playerID, "core:forge-sword")

	abilities := progression.NewAbilityRegistry()
	_ = abilities.Register(&progression.Ability{
		ID: "smithing", DisplayName: "Smithing",
		Type: progression.AbilityPassive, Category: progression.AbilitySkill,
		DefaultCap: 100, GainBaseChance: 30,
	})
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())
	prof.Learn(playerID, "smithing", 50)

	return crafting.NewService(tpls, store, recipes, known, prof, nil, nil, crafting.DefaultConfig(), nil)
}

// spawnIron spawns an iron-bar input into store and returns its id.
func spawnIron(t *testing.T, store *entities.Store) entities.EntityID {
	t.Helper()
	inst, err := store.Spawn(&item.Template{ID: "core:iron", Name: "an iron bar", Type: "item"})
	if err != nil {
		t.Fatalf("spawn iron: %v", err)
	}
	return inst.ID()
}

// recipeFrames decodes the fake conn's Char.Recipes frames.
func recipeFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CharRecipes {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CharRecipes, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCharRecipes {
			continue
		}
		var cr gmcp.CharRecipes
		if err := json.Unmarshal(f.payload, &cr); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, cr)
	}
	return out
}

func TestFlushGmcpRecipes_NilServiceNoOp(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	a.flushGmcpRecipes(context.Background(), nil)
	if got := len(recipeFrames(t, fc)); got != 0 {
		t.Errorf("nil craft service emitted %d recipe frames, want 0", got)
	}
}

func TestFlushGmcpRecipes_NoSendBeforeActivation(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	svc := recipeFormService(t, store, "p-1")
	a.flushGmcpRecipes(context.Background(), svc) // active=false
	if got := len(recipeFrames(t, fc)); got != 0 {
		t.Errorf("pre-activation emitted %d recipe frames, want 0", got)
	}
}

func TestFlushGmcpRecipes_CraftableWhenIngredientsPresent(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	svc := recipeFormService(t, store, "p-1")
	// Two iron bars → the recipe's inputs are satisfied.
	a.AddToInventory(spawnIron(t, store))
	a.AddToInventory(spawnIron(t, store))
	fc.setActive(true)

	a.flushGmcpRecipes(context.Background(), svc)
	frames := recipeFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush sent %d recipe frames, want 1", len(frames))
	}
	if len(frames[0].Recipes) != 1 {
		t.Fatalf("recipes in frame = %d, want 1", len(frames[0].Recipes))
	}
	r := frames[0].Recipes[0]
	if !r.Craftable || !r.StationMet || !r.SkillMet {
		t.Errorf("recipe should be craftable: %+v", r)
	}
	if r.Cmd != "craft forge-sword" {
		t.Errorf("submit cmd = %q, want %q (local-part of the id)", r.Cmd, "craft forge-sword")
	}
	if len(r.Ingredients) != 1 || r.Ingredients[0].Have != 2 || r.Ingredients[0].Need != 2 {
		t.Errorf("ingredient = %+v, want an iron bar 2/2", r.Ingredients)
	}
	if r.Ingredients[0].Name != "an iron bar" {
		t.Errorf("ingredient name = %q, want an iron bar", r.Ingredients[0].Name)
	}
}

func TestFlushGmcpRecipes_ShortfallBlocksAndReemitsOnChange(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	svc := recipeFormService(t, store, "p-1")
	// Only one iron bar — short of the 2 the recipe needs.
	iron := spawnIron(t, store)
	a.AddToInventory(iron)
	fc.setActive(true)

	a.flushGmcpRecipes(context.Background(), svc)
	frames := recipeFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush frames = %d, want 1", len(frames))
	}
	r := frames[0].Recipes[0]
	if r.Craftable {
		t.Errorf("recipe reports craftable despite shortfall: %+v", r)
	}
	if r.Blocked != "missing ingredients" {
		t.Errorf("blocked reason = %q, want %q", r.Blocked, "missing ingredients")
	}
	if r.Ingredients[0].Have != 1 {
		t.Errorf("have = %d, want 1", r.Ingredients[0].Have)
	}

	// A no-op flush emits nothing new (poll-and-diff).
	a.flushGmcpRecipes(context.Background(), svc)
	if got := len(recipeFrames(t, fc)); got != 1 {
		t.Errorf("redundant flush added a frame (total %d), want 1", got)
	}

	// Add the missing bar → the form flips to craftable and re-emits.
	a.AddToInventory(spawnIron(t, store))
	a.flushGmcpRecipes(context.Background(), svc)
	frames = recipeFrames(t, fc)
	if len(frames) != 2 {
		t.Fatalf("post-change frames = %d, want 2", len(frames))
	}
	if !frames[1].Recipes[0].Craftable {
		t.Errorf("recipe should be craftable after adding the second bar: %+v", frames[1].Recipes[0])
	}
}

func TestFlushGmcpRecipes_ShadowResetForcesResend(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	svc := recipeFormService(t, store, "p-1")
	fc.setActive(true)
	a.flushGmcpRecipes(context.Background(), svc) // baseline
	pre := len(recipeFrames(t, fc))

	a.resetGmcpItemsShadow() // clears the recipes shadow too (reattach seam)
	a.flushGmcpRecipes(context.Background(), svc)
	if got := len(recipeFrames(t, fc)) - pre; got != 1 {
		t.Errorf("post-reset added %d frames, want 1", got)
	}
}

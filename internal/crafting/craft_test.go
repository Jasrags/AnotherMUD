package crafting

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// fakeCrafter is an in-memory Crafter. failRemoveAt >= 0 makes the
// (failRemoveAt+1)-th RemoveFromInventory call fail, to exercise the
// consume-rollback path.
type fakeCrafter struct {
	id           string
	inv          []entities.EntityID
	failRemoveAt int
	removeCalls  int
}

func (f *fakeCrafter) PlayerID() string { return f.id }
func (f *fakeCrafter) ID() string       { return f.id }
func (f *fakeCrafter) Inventory() []entities.EntityID {
	return append([]entities.EntityID(nil), f.inv...)
}
func (f *fakeCrafter) AddToInventory(id entities.EntityID) { f.inv = append(f.inv, id) }
func (f *fakeCrafter) RemoveFromInventory(id entities.EntityID) bool {
	if f.failRemoveAt >= 0 && f.removeCalls >= f.failRemoveAt {
		f.removeCalls++
		return false
	}
	f.removeCalls++
	for i, x := range f.inv {
		if x == id {
			f.inv = append(f.inv[:i], f.inv[i+1:]...)
			return true
		}
	}
	return false
}

// craftFixture builds a service with a single recipe (inputQty of
// in-template → 1 out-template, discipline smithing) and a crafter who
// knows it, has the smithing proficiency, and carries inputQty inputs.
func craftFixture(t *testing.T, inputQty int, failRemoveAt int) (*Service, *fakeCrafter, *entities.Store, []entities.EntityID) {
	t.Helper()

	tpls := item.NewTemplates()
	inTpl := &item.Template{ID: "core:iron", Name: "an iron bar", Type: "item"}
	outTpl := &item.Template{ID: "core:sword", Name: "a sword", Type: "item"}
	tpls.Add(inTpl)
	tpls.Add(outTpl)

	store := entities.NewStore()

	recipes := recipe.NewRegistry()
	rec := &recipe.Recipe{
		ID: "core:forge-sword", DisplayName: "forge a sword", Discipline: "smithing",
		SkillFloor: 1,
		Inputs:     []recipe.Ingredient{{Template: "core:iron", Quantity: inputQty}},
		Output:     recipe.Output{Template: "core:sword", Quantity: 1},
	}
	if err := recipes.TryAdd(rec); err != nil {
		t.Fatal(err)
	}

	known := recipe.NewKnownManager(recipes)
	known.Learn("p1", "core:forge-sword")

	abilities := progression.NewAbilityRegistry()
	_ = abilities.Register(&progression.Ability{
		ID: "smithing", DisplayName: "Smithing",
		Type: progression.AbilityPassive, Category: progression.AbilitySkill,
		DefaultCap: 100, GainBaseChance: 30,
	})
	prof := progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())
	prof.Learn("p1", "smithing", 50)

	s := NewService(tpls, store, recipes, known, prof, coreLadder(), fixedRoller{}, DefaultConfig())

	// Spawn the inputs into the store and the crafter's bag.
	crafter := &fakeCrafter{id: "p1", failRemoveAt: failRemoveAt}
	var inputIDs []entities.EntityID
	for i := 0; i < inputQty; i++ {
		inst, err := store.Spawn(inTpl)
		if err != nil {
			t.Fatalf("spawn input: %v", err)
		}
		crafter.AddToInventory(inst.ID())
		inputIDs = append(inputIDs, inst.ID())
	}
	return s, crafter, store, inputIDs
}

func TestCraft_SuccessConsumesInputsProducesStampedOutput(t *testing.T) {
	s, crafter, store, inputIDs := craftFixture(t, 1, -1)

	res := s.Craft(context.Background(), crafter, "forge a sword", nil)
	if res.Outcome != CraftOK {
		t.Fatalf("Craft outcome = %v (%q), want CraftOK", res.Outcome, res.Message)
	}

	// Inputs destroyed (untracked from store) and gone from the bag.
	for _, id := range inputIDs {
		if _, ok := store.GetByID(id); ok {
			t.Errorf("input %s still tracked after craft", id)
		}
		for _, held := range crafter.inv {
			if held == id {
				t.Errorf("input %s still in inventory after craft", id)
			}
		}
	}

	// Exactly one output, in the bag, stamped with the rolled rarity.
	if len(crafter.inv) != 1 {
		t.Fatalf("inventory has %d items after craft, want 1 (the output)", len(crafter.inv))
	}
	outID := crafter.inv[0]
	e, ok := store.GetByID(outID)
	if !ok {
		t.Fatal("output not tracked in store")
	}
	out := e.(*entities.ItemInstance)
	if string(out.TemplateID()) != "core:sword" {
		t.Errorf("output template = %q, want core:sword", out.TemplateID())
	}
	if res.QualityKey != "" {
		if v, ok := out.Property("rarity"); !ok || v != res.QualityKey {
			t.Errorf("output rarity property = %v (ok=%v), want %q", v, ok, res.QualityKey)
		}
	}
}

func TestCraft_RollbackOnPartialRemoveLosesNothing(t *testing.T) {
	// Recipe needs 2 inputs; the 2nd remove fails (failRemoveAt=1).
	s, crafter, store, inputIDs := craftFixture(t, 2, 1)

	res := s.Craft(context.Background(), crafter, "forge a sword", nil)
	if res.Outcome != CraftInterrupted {
		t.Fatalf("outcome = %v (%q), want CraftInterrupted", res.Outcome, res.Message)
	}

	// Both inputs must still be live in the store (nothing destroyed)...
	for _, id := range inputIDs {
		if _, ok := store.GetByID(id); !ok {
			t.Errorf("input %s was destroyed despite rollback", id)
		}
	}
	// ...and both must be back in the bag (the removed one re-added).
	if len(crafter.inv) != 2 {
		t.Errorf("inventory has %d items after rollback, want 2", len(crafter.inv))
	}
	// No output was produced.
	for _, held := range crafter.inv {
		if e, ok := store.GetByID(held); ok {
			if it, ok := e.(*entities.ItemInstance); ok && string(it.TemplateID()) == "core:sword" {
				t.Error("an output was produced despite the interrupted craft")
			}
		}
	}
}

func TestCraft_MissingIngredients(t *testing.T) {
	s, crafter, _, _ := craftFixture(t, 1, -1)
	// Drain the bag so the input is absent.
	crafter.inv = nil
	res := s.Craft(context.Background(), crafter, "forge a sword", nil)
	if res.Outcome != CraftMissingIngredients {
		t.Errorf("outcome = %v (%q), want CraftMissingIngredients", res.Outcome, res.Message)
	}
}

func TestCraft_UnknownRecipe(t *testing.T) {
	s, crafter, _, _ := craftFixture(t, 1, -1)
	res := s.Craft(context.Background(), crafter, "summon dragon", nil)
	if res.Outcome != CraftUnknownRecipe {
		t.Errorf("outcome = %v, want CraftUnknownRecipe", res.Outcome)
	}
}

func TestCraft_NotSkilledBelowFloor(t *testing.T) {
	s, crafter, _, _ := craftFixture(t, 1, -1)
	// Recreate proficiency below the floor: forget then relearn at... the
	// floor is 1 and Learn seeds at 1, so instead make a recipe-floor case
	// by clearing the proficiency entirely (prof 0 < floor 1).
	s.prof.Forget("p1", "smithing")
	res := s.Craft(context.Background(), crafter, "forge a sword", nil)
	if res.Outcome != CraftNotSkilled {
		t.Errorf("outcome = %v (%q), want CraftNotSkilled", res.Outcome, res.Message)
	}
}

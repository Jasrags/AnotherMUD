package quest

import "testing"

// recRecipes records the recipe ids the dispatcher teaches.
type recRecipes struct{ got []string }

func (r *recRecipes) GrantRecipe(_, recipeID string) { r.got = append(r.got, recipeID) }

func TestDispatch_GrantsRecipes(t *testing.T) {
	rec := &recRecipes{}
	d := NewDispatcher(WithRecipes(rec))
	d.Dispatch(&fakePlayer{id: "p1"}, Reward{Recipes: []string{"core:a", "core:b"}})

	if len(rec.got) != 2 || rec.got[0] != "core:a" || rec.got[1] != "core:b" {
		t.Errorf("granted recipes = %v, want [core:a core:b]", rec.got)
	}
}

func TestDispatch_EmptyRecipesNoop(t *testing.T) {
	rec := &recRecipes{}
	d := NewDispatcher(WithRecipes(rec))
	d.Dispatch(&fakePlayer{id: "p1"}, Reward{XP: 5})

	if len(rec.got) != 0 {
		t.Errorf("granted %v recipes, want none", rec.got)
	}
}

func TestDispatch_NopRecipeNoPanic(t *testing.T) {
	// A dispatcher with no recipe teacher must not panic on a recipe reward.
	d := NewDispatcher()
	d.Dispatch(&fakePlayer{id: "p1"}, Reward{Recipes: []string{"core:x"}})
}

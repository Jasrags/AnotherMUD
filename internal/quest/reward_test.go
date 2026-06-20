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

// recFaction records the faction standing shifts the dispatcher routes.
type recFaction struct {
	got []FactionReward
	ids []string // entity ids the shift targeted
}

func (r *recFaction) ShiftStanding(entityID, factionID string, delta int, _ string) {
	r.got = append(r.got, FactionReward{Faction: factionID, Delta: delta})
	r.ids = append(r.ids, entityID)
}

func TestDispatch_GrantsFaction(t *testing.T) {
	rec := &recFaction{}
	d := NewDispatcher(WithFaction(rec))
	d.Dispatch(&fakePlayer{id: "p1"}, Reward{Faction: []FactionReward{
		{Faction: "wot:queens-guard", Delta: 50},
		{Faction: "wot:darkfriends", Delta: -25},
	}})

	if len(rec.got) != 2 {
		t.Fatalf("shifts = %v, want 2", rec.got)
	}
	if rec.got[0] != (FactionReward{Faction: "wot:queens-guard", Delta: 50}) {
		t.Errorf("shift[0] = %+v", rec.got[0])
	}
	if rec.got[1] != (FactionReward{Faction: "wot:darkfriends", Delta: -25}) {
		t.Errorf("shift[1] = %+v", rec.got[1])
	}
	if rec.ids[0] != "p1" || rec.ids[1] != "p1" {
		t.Errorf("shift targets = %v, want both p1", rec.ids)
	}
}

func TestDispatch_FactionSkipsEmptyOrZero(t *testing.T) {
	rec := &recFaction{}
	d := NewDispatcher(WithFaction(rec))
	// Empty faction id and zero delta are both no-ops; only the valid one applies.
	d.Dispatch(&fakePlayer{id: "p1"}, Reward{Faction: []FactionReward{
		{Faction: "", Delta: 10},                // no id
		{Faction: "wot:queens-guard", Delta: 0}, // no change
		{Faction: "wot:queens-guard", Delta: 5},
	}})

	if len(rec.got) != 1 || rec.got[0].Delta != 5 {
		t.Errorf("shifts = %v, want a single +5", rec.got)
	}
}

func TestDispatch_NopFactionNoPanic(t *testing.T) {
	// A dispatcher with no faction shifter must not panic on a faction reward.
	d := NewDispatcher()
	d.Dispatch(&fakePlayer{id: "p1"}, Reward{Faction: []FactionReward{{Faction: "wot:x", Delta: 1}}})
}

// recRenown records the renown shifts the dispatcher routes.
type recRenown struct {
	deltas []int
	ids    []string
}

func (r *recRenown) ShiftRenown(entityID string, delta int, _ string) {
	r.deltas = append(r.deltas, delta)
	r.ids = append(r.ids, entityID)
}

func TestDispatch_GrantsRenown(t *testing.T) {
	rec := &recRenown{}
	d := NewDispatcher(WithRenown(rec))
	d.Dispatch(&fakePlayer{id: "p1"}, Reward{Reputation: 150})

	if len(rec.deltas) != 1 || rec.deltas[0] != 150 {
		t.Fatalf("renown shifts = %v, want [150]", rec.deltas)
	}
	if rec.ids[0] != "p1" {
		t.Errorf("renown target = %q, want p1", rec.ids[0])
	}
}

func TestDispatch_RenownSkipsZero(t *testing.T) {
	rec := &recRenown{}
	d := NewDispatcher(WithRenown(rec))
	d.Dispatch(&fakePlayer{id: "p1"}, Reward{XP: 5}) // no Reputation field
	if len(rec.deltas) != 0 {
		t.Errorf("zero renown reward should not shift: %v", rec.deltas)
	}
}

func TestDispatch_NopRenownNoPanic(t *testing.T) {
	// A dispatcher with no renown shifter must not panic on a renown reward.
	d := NewDispatcher()
	d.Dispatch(&fakePlayer{id: "p1"}, Reward{Reputation: 50})
}

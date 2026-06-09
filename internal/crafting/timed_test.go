package crafting

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// fakeBusyCrafter is a fakeCrafter that also models the CraftBusy
// occupation state, so the timed-craft path (BeginCraft → SetPendingCraft →
// CompleteReady) can be driven end to end in memory.
type fakeBusyCrafter struct {
	fakeCrafter
	pending PendingCraft
	hasPend bool
}

func (f *fakeBusyCrafter) PendingCraft() (PendingCraft, bool) { return f.pending, f.hasPend }
func (f *fakeBusyCrafter) SetPendingCraft(p PendingCraft) bool {
	if f.hasPend {
		return false
	}
	f.pending, f.hasPend = p, true
	return true
}
func (f *fakeBusyCrafter) ClearPendingCraft() (PendingCraft, bool) {
	p, had := f.pending, f.hasPend
	f.pending, f.hasPend = PendingCraft{}, false
	return p, had
}

// timedFixture mirrors craftFixture but sets the recipe's TimePulses and
// returns a busy crafter so the timed path can be exercised.
func timedFixture(t *testing.T, timePulses int) (*Service, *fakeBusyCrafter, *entities.Store, entities.EntityID) {
	t.Helper()
	s, base, store, inputIDs := craftFixture(t, 1, -1)
	rec, err := s.recipes.Get("core:forge-sword")
	if err != nil {
		t.Fatalf("get recipe: %v", err)
	}
	rec.TimePulses = timePulses

	bc := &fakeBusyCrafter{fakeCrafter: *base}
	return s, bc, store, inputIDs[0]
}

func TestBeginCraft_OKReturnsTokenWithoutMutating(t *testing.T) {
	s, bc, _, inputID := timedFixture(t, 30)

	begin := s.BeginCraft(context.Background(), bc, "forge a sword", nil)
	if begin.Outcome != CraftOK {
		t.Fatalf("BeginCraft outcome = %v (%q), want CraftOK", begin.Outcome, begin.Message)
	}
	if begin.TimePulses != 30 {
		t.Errorf("TimePulses = %d, want 30", begin.TimePulses)
	}
	if begin.RecipeID != "core:forge-sword" {
		t.Errorf("RecipeID = %q, want core:forge-sword", begin.RecipeID)
	}
	if begin.DisplayName != "forge a sword" {
		t.Errorf("DisplayName = %q, want 'forge a sword'", begin.DisplayName)
	}
	// Read-only: the input is still in the bag and tracked.
	if len(bc.inv) != 1 || bc.inv[0] != inputID {
		t.Errorf("inventory changed by BeginCraft: %v", bc.inv)
	}
}

func TestBeginCraft_RefusesMissingIngredients(t *testing.T) {
	s, bc, _, _ := timedFixture(t, 30)
	bc.inv = nil // drain the bag

	begin := s.BeginCraft(context.Background(), bc, "forge a sword", nil)
	if begin.Outcome != CraftMissingIngredients {
		t.Errorf("outcome = %v (%q), want CraftMissingIngredients", begin.Outcome, begin.Message)
	}
}

func TestBeginCraft_RefusesUnknownRecipe(t *testing.T) {
	s, bc, _, _ := timedFixture(t, 30)
	begin := s.BeginCraft(context.Background(), bc, "summon dragon", nil)
	if begin.Outcome != CraftUnknownRecipe {
		t.Errorf("outcome = %v, want CraftUnknownRecipe", begin.Outcome)
	}
}

func TestCompleteReady_NotDueLeavesPending(t *testing.T) {
	s, bc, _, _ := timedFixture(t, 30)
	bc.SetPendingCraft(PendingCraft{RecipeID: "core:forge-sword", ReadyAt: 100, StationTier: 0})

	res, done := s.CompleteReady(context.Background(), bc, 50)
	if done {
		t.Fatalf("CompleteReady reported done before ReadyAt (%q)", res.Message)
	}
	if _, ok := bc.PendingCraft(); !ok {
		t.Error("pending craft cleared while not due")
	}
}

func TestCompleteReady_DueCraftsAndClears(t *testing.T) {
	s, bc, store, inputID := timedFixture(t, 30)
	bc.SetPendingCraft(PendingCraft{RecipeID: "core:forge-sword", ReadyAt: 100, StationTier: 0})

	res, done := s.CompleteReady(context.Background(), bc, 100)
	if !done || res.Outcome != CraftOK {
		t.Fatalf("CompleteReady = (%v, done=%v), want CraftOK done", res.Outcome, done)
	}
	// Pending cleared.
	if _, ok := bc.PendingCraft(); ok {
		t.Error("pending craft not cleared after completion")
	}
	// Input consumed, output in the bag.
	if _, ok := store.GetByID(inputID); ok {
		t.Error("input still tracked after completion")
	}
	if len(bc.inv) != 1 {
		t.Fatalf("inventory has %d items, want 1 (the output)", len(bc.inv))
	}
	if e, ok := store.GetByID(bc.inv[0]); !ok || string(e.(*entities.ItemInstance).TemplateID()) != "core:sword" {
		t.Error("output sword not produced")
	}
}

func TestCompleteReady_MissingIngredientsFailsCleanly(t *testing.T) {
	s, bc, store, inputID := timedFixture(t, 30)
	bc.SetPendingCraft(PendingCraft{RecipeID: "core:forge-sword", ReadyAt: 100, StationTier: 0})
	// The crafter dropped the ingredient while the craft was in flight.
	bc.inv = nil

	res, done := s.CompleteReady(context.Background(), bc, 100)
	if !done || res.Outcome != CraftMissingIngredients {
		t.Fatalf("CompleteReady = (%v, done=%v), want CraftMissingIngredients done", res.Outcome, done)
	}
	// Pending cleared (no infinite retry) and nothing produced.
	if _, ok := bc.PendingCraft(); ok {
		t.Error("pending craft not cleared after a failed completion")
	}
	if len(bc.inv) != 0 {
		t.Errorf("inventory grew on a failed craft: %v", bc.inv)
	}
	// The original input was never touched by us (it's gone because the
	// test drained it, not because the craft consumed it).
	if _, ok := store.GetByID(inputID); !ok {
		t.Error("the still-tracked input instance vanished")
	}
}

func TestCompleteReady_NoPending(t *testing.T) {
	s, bc, _, _ := timedFixture(t, 30)
	if _, done := s.CompleteReady(context.Background(), bc, 100); done {
		t.Error("CompleteReady reported done with no pending craft")
	}
}

func TestCompleteReady_StationTierCaptured(t *testing.T) {
	// A recipe needing Tier-2 station completes using the captured tier
	// even though CompleteReady is given no StationTierFunc.
	s, bc, _, _ := timedFixture(t, 30)
	rec, _ := s.recipes.Get("core:forge-sword")
	rec.StationTier = 2
	bc.SetPendingCraft(PendingCraft{RecipeID: "core:forge-sword", ReadyAt: 1, StationTier: 2})

	res, done := s.CompleteReady(context.Background(), bc, 1)
	if !done || res.Outcome != CraftOK {
		t.Fatalf("CompleteReady = (%v, done=%v), want CraftOK done", res.Outcome, done)
	}
}

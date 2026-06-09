package session

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/crafting"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// The CraftBusy adapter round-trips the transient occupation state and
// refuses a second start while one is in flight.
func TestConnActor_PendingCraftLifecycle(t *testing.T) {
	a, _ := newFakeActor("c1", "p1", "acc1", "Smith", &world.Room{ID: "core:forge"})

	if _, ok := a.PendingCraft(); ok {
		t.Fatal("a fresh actor should have no pending craft")
	}
	p := crafting.PendingCraft{RecipeID: "core:reforge", ReadyAt: 42, StationTier: 2, DisplayName: "reforge a sword"}
	if !a.SetPendingCraft(p) {
		t.Fatal("SetPendingCraft should succeed on an idle actor")
	}
	if a.SetPendingCraft(p) {
		t.Error("SetPendingCraft should refuse while a craft is in flight")
	}
	got, ok := a.PendingCraft()
	if !ok || got != p {
		t.Errorf("PendingCraft = (%+v, %v), want (%+v, true)", got, ok, p)
	}
	cleared, had := a.ClearPendingCraft()
	if !had || cleared != p {
		t.Errorf("ClearPendingCraft = (%+v, %v), want (%+v, true)", cleared, had, p)
	}
	if _, ok := a.PendingCraft(); ok {
		t.Error("PendingCraft should be empty after Clear")
	}
}

// Moving rooms breaks an in-flight craft and notifies the crafter; a
// same-room SetRoom leaves it running.
func TestConnActor_SetRoomCancelsCraft(t *testing.T) {
	a, fc := newFakeActor("c1", "p1", "acc1", "Smith", &world.Room{ID: "core:forge"})
	a.SetPendingCraft(crafting.PendingCraft{RecipeID: "core:reforge", ReadyAt: 99, DisplayName: "reforge a sword"})

	// Same-room SetRoom: craft survives.
	a.SetRoom(&world.Room{ID: "core:forge"})
	if _, ok := a.PendingCraft(); !ok {
		t.Error("a same-room SetRoom should not cancel the craft")
	}

	// Actual move: craft cancels with a notice.
	a.SetRoom(&world.Room{ID: "core:market"})
	if _, ok := a.PendingCraft(); ok {
		t.Error("moving rooms should cancel the in-flight craft")
	}
	if !anyContains(fc.writes(), "set your work aside") {
		t.Errorf("expected a move-interrupt notice, got %v", fc.writes())
	}
}

// CancelCraft drops the craft and writes a notice when one was active, and
// is a quiet no-op otherwise.
func TestConnActor_CancelCraft(t *testing.T) {
	a, fc := newFakeActor("c1", "p1", "acc1", "Smith", &world.Room{ID: "core:forge"})

	if a.CancelCraft(context.Background()) {
		t.Error("CancelCraft with no pending craft should report false")
	}
	a.SetPendingCraft(crafting.PendingCraft{RecipeID: "core:reforge", DisplayName: "reforge a sword"})
	if !a.CancelCraft(context.Background()) {
		t.Error("CancelCraft with a pending craft should report true")
	}
	if !anyContains(fc.writes(), "concentration breaks") {
		t.Errorf("expected an interrupt notice, got %v", fc.writes())
	}
}

func anyContains(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// moveCostWorld builds a two-room corridor A <-> B with no light or door
// complications, so the only gate exercised is the movement-cost gate.
func moveCostWorld() (*world.World, *world.Room, *world.Room) {
	a := &world.Room{ID: "a", Name: "Road", Description: "A dusty road.", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "b"}}}
	b := &world.Room{ID: "b", Name: "Road", Description: "More road.", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "a"}}}
	w := world.New()
	w.AddRoom(a)
	w.AddRoom(b)
	return w, a, b
}

// A character with a movement pool spends a point per step.
func TestMove_SpendsMovementPoint(t *testing.T) {
	w, a, _ := moveCostWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	actor.mvMax, actor.mv = 10, 10

	if err := r5dDispatch(w, store, place, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "b" {
		t.Fatalf("move blocked unexpectedly; room = %q, want b", actor.Room().ID)
	}
	if actor.Movement() != 9 {
		t.Fatalf("movement after one step = %d; want 9", actor.Movement())
	}
}

// An empty movement pool refuses the step and leaves the mover in place.
func TestMove_BlockedWhenWinded(t *testing.T) {
	w, a, _ := moveCostWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	actor.mvMax, actor.mv = 10, 0

	if err := r5dDispatch(w, store, place, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "a" {
		t.Fatalf("winded move should be refused; room = %q, want a", actor.Room().ID)
	}
	if got := actor.lastLine(); !strings.Contains(got, "too winded") {
		t.Fatalf("expected winded refusal, got %q", got)
	}
}

// A character with no movement pool (max 0 — every character before the
// pool is granted) moves for free: the gate is a no-op.
func TestMove_NoPoolMovesFree(t *testing.T) {
	w, a, _ := moveCostWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a) // mvMax defaults to 0

	if err := r5dDispatch(w, store, place, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "b" {
		t.Fatalf("free move blocked; room = %q, want b", actor.Room().ID)
	}
	if actor.Movement() != 0 {
		t.Fatalf("no-pool movement should stay 0, got %d", actor.Movement())
	}
}

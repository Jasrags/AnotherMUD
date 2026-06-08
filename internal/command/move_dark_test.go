package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// darkMoveWorld builds a lit outdoors room A with a north exit into an
// underground cave B (and B's south exit back). B can be marked
// dark_blocked by the caller.
func darkMoveWorld(blockB bool) (*world.World, *world.Room, *world.Room) {
	a := &world.Room{ID: "a", Name: "Square", Description: "A lit plaza.", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "b"}}}
	bProps := map[string]any{}
	if blockB {
		bProps[command.PropRoomDarkBlocked] = true
	}
	b := &world.Room{ID: "b", Name: "Cave", Description: "A black cave.", Terrain: world.TerrainUnderground,
		Properties: bProps,
		Exits:      map[world.Direction]world.Exit{world.DirSouth: {Target: "a"}}}
	w := world.New()
	w.AddRoom(a)
	w.AddRoom(b)
	return w, a, b
}

func darkMoveEnv(w *world.World, store *entities.Store, place *entities.Placement) command.Env {
	return command.Env{
		World:     w,
		Items:     store,
		Placement: place,
		Light:     newLightResolver(gameclock.PeriodDay),
	}
}

func TestMove_IntoDarkRoomAllowedByDefault(t *testing.T) {
	w, a, _ := darkMoveWorld(false)
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)

	if err := r5dDispatch(w, store, place, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "b" {
		t.Fatalf("move into dark room blocked; room = %q, want b", actor.Room().ID)
	}
	// The black destination suppresses detail but still tells the mover
	// the way back (escape invariant).
	joined := strings.Join(actorLines(actor), "\n")
	if !strings.Contains(joined, "can see nothing") {
		t.Fatalf("dark arrival should suppress the room:\n%s", joined)
	}
	if !strings.Contains(joined, "feel your way back south") {
		t.Fatalf("dark arrival should name the way back:\n%s", joined)
	}
}

func TestMove_DarkBlockedRefusedWhenBlind(t *testing.T) {
	w, a, _ := darkMoveWorld(true)
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)

	if err := r5dDispatch(w, store, place, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "a" {
		t.Fatalf("dark_blocked move should be refused; room = %q, want a", actor.Room().ID)
	}
	if got := actor.lastLine(); !strings.Contains(got, "too dark to risk") {
		t.Fatalf("expected dark-block refusal, got %q", got)
	}
}

func TestMove_DarkBlockedAllowedWithLight(t *testing.T) {
	w, a, _ := darkMoveWorld(true)
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	// A lit torch in the light slot lets the mover see the hazard.
	torch, err := store.Spawn(&item.Template{
		ID: "x:torch", Name: "a torch", Type: "light",
		Properties: map[string]any{"light": "gloom"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	torch.SetProperty("lit", true)
	actor.AddToInventory(torch.ID())
	if !actor.Equip([]string{"light"}, torch.ID(), nil) {
		t.Fatal("equip torch into light slot failed")
	}

	if err := r5dDispatch(w, store, place, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "b" {
		t.Fatalf("dark_blocked move with a torch should succeed; room = %q, want b", actor.Room().ID)
	}
}

// r5dDispatch dispatches a movement line through a fresh registry.
func r5dDispatch(w *world.World, store *entities.Store, place *entities.Placement, a *testActor, line string) error {
	reg := command.New()
	if err := command.RegisterBuiltins(reg); err != nil {
		return err
	}
	return reg.Dispatch(context.Background(), darkMoveEnv(w, store, place), a, line)
}

// actorLines returns a copy of everything written to the actor.
func actorLines(a *testActor) []string { return a.allLines() }

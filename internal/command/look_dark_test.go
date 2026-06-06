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

// darkLookEnv builds an env with a light resolver, an entity store, and
// placement so the examination gate can be driven end to end.
func darkLookEnv(store *entities.Store, place *entities.Placement) command.Env {
	return command.Env{
		Items:     store,
		Placement: place,
		Light:     newLightResolver(gameclock.PeriodDay),
	}
}

func TestLook_RoomItemBlockedInDark(t *testing.T) {
	store := entities.NewStore()
	place := entities.NewPlacement()
	cave := &world.Room{ID: "x:cave", Name: "A Cave", Terrain: world.TerrainUnderground}
	a := newTestActor(cave)

	well, err := store.Spawn(&item.Template{
		ID: "x:well", Name: "a stone well", Type: "fixture", Keywords: []string{"well"},
		Description: "Mossy stones ring a dark shaft.",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	place.Place(well.ID(), cave.ID)

	r := newRegistry(t)
	dispatch(t, r, darkLookEnv(store, place), a, "look well")
	if got := a.lastLine(); !strings.Contains(got, "too dark") {
		t.Fatalf("look at room item in the dark = %q, want too-dark", got)
	}
	if strings.Contains(a.lastLine(), "Mossy") {
		t.Fatalf("dark look leaked the item description: %q", a.lastLine())
	}
}

func TestLook_HeldItemAllowedInDark(t *testing.T) {
	store := entities.NewStore()
	place := entities.NewPlacement()
	cave := &world.Room{ID: "x:cave", Name: "A Cave", Terrain: world.TerrainUnderground}
	a := newTestActor(cave)

	ring, err := store.Spawn(&item.Template{
		ID: "x:ring", Name: "a plain ring", Type: "ring", Keywords: []string{"ring"},
		Description: "A simple band, smooth to the touch.",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	a.AddToInventory(ring.ID())

	r := newRegistry(t)
	dispatch(t, r, darkLookEnv(store, place), a, "look ring")
	if got := a.lastLine(); !strings.Contains(got, "smooth to the touch") {
		t.Fatalf("look at held item in the dark = %q, want its description (felt by hand)", got)
	}
}

func TestLook_RoomItemVisibleWhenLit(t *testing.T) {
	store := entities.NewStore()
	place := entities.NewPlacement()
	// Outdoors by day → lit, so the room item is examinable.
	square := &world.Room{ID: "x:square", Name: "Square", Terrain: world.TerrainOutdoors}
	a := newTestActor(square)
	well, _ := store.Spawn(&item.Template{
		ID: "x:well", Name: "a stone well", Type: "fixture", Keywords: []string{"well"},
		Description: "Mossy stones ring a dark shaft.",
	})
	place.Place(well.ID(), square.ID)

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), darkLookEnv(store, place), a, "look well"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "Mossy") {
		t.Fatalf("look at room item when lit = %q, want its description", got)
	}
}

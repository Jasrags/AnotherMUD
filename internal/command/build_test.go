package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// buildFixture wires a room, store, placement, and an actor carrying one
// firewood (fuel-tagged) item. terrain/weather are caller-controlled.
type buildFixture struct {
	actor  *testActor
	ctx    *command.Context
	store  *entities.Store
	place  *entities.Placement
	room   *world.Room
	fuelID entities.EntityID
}

func newBuildFixture(t *testing.T, terrain, weather string) *buildFixture {
	t.Helper()
	store := entities.NewStore()
	place := entities.NewPlacement()
	room := &world.Room{ID: "tapestry-core:meadow", AreaID: "tapestry-core:wild", Name: "Meadow", Description: "x", Terrain: terrain}

	a := newNamedTestActor("Pyre", "p-pyre", room)

	fuel, err := store.Spawn(&item.Template{
		ID: "tapestry-core:firewood", Name: "a bundle of firewood", Type: "item",
		Tags: []string{"fuel"},
	})
	if err != nil {
		t.Fatalf("spawn fuel: %v", err)
	}
	a.AddToInventory(fuel.ID())

	c := &command.Context{
		Actor:     a,
		Verb:      "build",
		Items:     store,
		Placement: place,
	}
	if weather != "" {
		c.WeatherState = func(world.AreaID) string { return weather }
	}
	return &buildFixture{actor: a, ctx: c, store: store, place: place, room: room, fuelID: fuel.ID()}
}

func TestBuild_CampfireHappyPathConsumesFuelAndPlacesStation(t *testing.T) {
	f := newBuildFixture(t, world.TerrainOutdoors, "clear")
	f.ctx.Args = []string{"campfire"}

	if err := command.BuildHandler(context.Background(), f.ctx); err != nil {
		t.Fatalf("BuildHandler: %v", err)
	}
	// Fuel removed from the bag AND destroyed (untracked).
	if len(f.actor.Inventory()) != 0 {
		t.Errorf("inventory = %v, want empty (fuel consumed)", f.actor.Inventory())
	}
	if _, ok := f.store.GetByID(f.fuelID); ok {
		t.Error("fuel still tracked after a successful build")
	}
	// A campfire is now in the room and makes it a Tier-1 cooking station.
	if n := len(f.place.InRoom(f.room.ID)); n != 1 {
		t.Fatalf("room has %d entities, want 1 (the campfire)", n)
	}
}

func TestBuild_NoFuelRefused(t *testing.T) {
	f := newBuildFixture(t, world.TerrainOutdoors, "clear")
	// Drain the fuel.
	f.actor.RemoveFromInventory(f.fuelID)
	f.ctx.Args = []string{"campfire"}

	_ = command.BuildHandler(context.Background(), f.ctx)
	if got := f.actor.lastLine(); got == "" || !strings.Contains(got, "firewood") {
		t.Errorf("output = %q, want a firewood-needed message", got)
	}
	if len(f.place.InRoom(f.room.ID)) != 0 {
		t.Error("a campfire was placed without fuel")
	}
}

func TestBuild_IndoorsRefusedFuelKept(t *testing.T) {
	f := newBuildFixture(t, world.TerrainIndoors, "clear")
	f.ctx.Args = []string{"campfire"}

	_ = command.BuildHandler(context.Background(), f.ctx)
	if len(f.place.InRoom(f.room.ID)) != 0 {
		t.Error("a fire was built indoors")
	}
	// Refused before the fuel gate — fuel untouched.
	if len(f.actor.Inventory()) != 1 {
		t.Errorf("fuel consumed on a refused (indoors) build: inv=%v", f.actor.Inventory())
	}
}

func TestBuild_WetWeatherRefusedFuelKept(t *testing.T) {
	f := newBuildFixture(t, world.TerrainOutdoors, "rain")
	f.ctx.Args = []string{"campfire"}

	_ = command.BuildHandler(context.Background(), f.ctx)
	if len(f.place.InRoom(f.room.ID)) != 0 {
		t.Error("a fire was built in the rain")
	}
	if len(f.actor.Inventory()) != 1 {
		t.Errorf("fuel consumed on a refused (rain) build: inv=%v", f.actor.Inventory())
	}
}

func TestBuild_OneFirePerRoom(t *testing.T) {
	f := newBuildFixture(t, world.TerrainOutdoors, "clear")
	// Give a second fuel so a second build would otherwise succeed.
	fuel2, _ := f.store.Spawn(&item.Template{ID: "tapestry-core:firewood", Name: "firewood", Type: "item", Tags: []string{"fuel"}})
	f.actor.AddToInventory(fuel2.ID())
	f.ctx.Args = []string{"campfire"}

	_ = command.BuildHandler(context.Background(), f.ctx) // first: builds
	_ = command.BuildHandler(context.Background(), f.ctx) // second: refused

	if n := len(f.place.InRoom(f.room.ID)); n != 1 {
		t.Errorf("room has %d campfires, want 1 (second build should refuse)", n)
	}
	// Only one fuel consumed.
	if len(f.actor.Inventory()) != 1 {
		t.Errorf("inventory = %v, want 1 fuel left (only one consumed)", f.actor.Inventory())
	}
}

func TestBuild_UnknownThing(t *testing.T) {
	f := newBuildFixture(t, world.TerrainOutdoors, "clear")
	f.ctx.Args = []string{"castle"}
	_ = command.BuildHandler(context.Background(), f.ctx)
	if got := f.actor.lastLine(); !strings.Contains(got, "don't know how to build") {
		t.Errorf("output = %q, want unknown-build message", got)
	}
}

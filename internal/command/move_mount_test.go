package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/biome"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// rideMount spawns a mount into the store, owned by the actor and bound as its
// ride (mounts.md §4.3/§5). Returns the live mount so the test can read its
// travel pool.
func rideMount(t *testing.T, store *entities.Store, actor *testActor, spec *mob.MountSpec) *entities.MobInstance {
	t.Helper()
	m, err := store.SpawnMob(&mob.Template{ID: "test:steed", Name: "a steed", Type: "npc", Mount: spec})
	if err != nil {
		t.Fatalf("SpawnMob mount: %v", err)
	}
	m.SetOwner(actor.PlayerID())
	actor.SetMountID(m.ID())
	return m
}

// While ridden, a step spends the MOUNT's travel pool, not the rider's
// movement pool (mounts.md §5.1).
func TestMove_MountedSpendsTravelNotMovement(t *testing.T) {
	w, a, _ := moveCostWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	actor.playerID = "p-1"
	actor.mvMax, actor.mv = 10, 10
	steed := rideMount(t, store, actor, &mob.MountSpec{TravelMax: 10})

	if err := r5dDispatch(w, store, place, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "b" {
		t.Fatalf("mounted move blocked; room = %q, want b", actor.Room().ID)
	}
	if steed.Travel() != 9 {
		t.Errorf("mount travel after one step = %d, want 9 (the mount paid)", steed.Travel())
	}
	if actor.Movement() != 10 {
		t.Errorf("rider movement = %d, want 10 (untouched — the mount is the metered mover)", actor.Movement())
	}
}

// An exhausted mount refuses the step (no relocation), and the rider's own pool
// is never charged (mounts.md §5.4). The rider can dismount and walk.
func TestMove_MountedBlownRefuses(t *testing.T) {
	w, a, _ := moveCostWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	actor.playerID = "p-1"
	actor.mvMax, actor.mv = 10, 10
	steed := rideMount(t, store, actor, &mob.MountSpec{TravelMax: 5})
	if !steed.TrySpendTravel(5) { // drain it
		t.Fatal("could not drain the mount's travel pool")
	}

	if err := r5dDispatch(w, store, place, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "a" {
		t.Fatalf("blown mount should refuse the step; room = %q, want a", actor.Room().ID)
	}
	if !strings.Contains(actor.lastLine(), "blown") {
		t.Errorf("reply = %q, want a blown-mount refusal", actor.lastLine())
	}
	if actor.Movement() != 10 {
		t.Errorf("rider movement = %d, want 10 (a blown mounted step never charges the rider)", actor.Movement())
	}
}

// A step costlier than the mount's FULL travel ceiling is refused (blown), not
// granted free — unlike the on-foot pool, a mount always has a pool, so an
// unaffordable step surfaces as a refusal (mounts.md §5.4). Here a forest
// (move_cost 3) exceeds the mount's travel_max of 2.
func TestMove_MountedOversizedStepRefused(t *testing.T) {
	a := &world.Room{ID: "a", Name: "Edge", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "wood"}}}
	wood := &world.Room{ID: "wood", Name: "Wood", Terrain: "forest",
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "a"}}}
	w := world.New()
	w.AddRoom(a)
	w.AddRoom(wood)
	biomes := biome.NewRegistry()
	_ = biomes.RegisterEngine(&biome.Biome{ID: "forest", MoveCost: 3})
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	actor.playerID = "p-1"
	steed := rideMount(t, store, actor, &mob.MountSpec{TravelMax: 2})

	reg := command.New()
	if err := command.RegisterBuiltins(reg); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	env := command.Env{World: w, Items: store, Placement: place, Biomes: biomes, DefaultMoveCost: 1}
	if err := reg.Dispatch(context.Background(), env, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "a" {
		t.Fatalf("an unaffordable mounted step should be refused; room = %q, want a", actor.Room().ID)
	}
	if !strings.Contains(actor.lastLine(), "blown") {
		t.Errorf("reply = %q, want a blown refusal (not a silent free pass)", actor.lastLine())
	}
	if steed.Travel() != 2 {
		t.Errorf("mount travel = %d, want 2 (a refused step never charges)", steed.Travel())
	}
}

// A destination on a mount's per-type impassable list refuses the mounted step
// (mounts.md §5.3); the mount's pool is not charged.
func TestMove_MountImpassableTerrain(t *testing.T) {
	a := &world.Room{ID: "a", Name: "Road", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "cave"}}}
	cave := &world.Room{ID: "cave", Name: "Cave", Terrain: "cave",
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "a"}}}
	w := world.New()
	w.AddRoom(a)
	w.AddRoom(cave)
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	actor.playerID = "p-1"
	steed := rideMount(t, store, actor, &mob.MountSpec{TravelMax: 10, Impassable: []string{"cave"}})

	if err := r5dDispatch(w, store, place, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "a" {
		t.Fatalf("mount-impassable step should be refused; room = %q, want a", actor.Room().ID)
	}
	if !strings.Contains(actor.lastLine(), "can't go that way") {
		t.Errorf("reply = %q, want a mount-impassable refusal", actor.lastLine())
	}
	if steed.Travel() != 10 {
		t.Errorf("mount travel = %d, want 10 (a refused step never charges)", steed.Travel())
	}
}

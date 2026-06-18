package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/biome"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
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

// moveCostBiomeDispatch dispatches one line with a biome registry and a flat
// default wired into the Env, so the cost gate can resolve the destination
// biome's MoveCost.
func moveCostBiomeDispatch(w *world.World, biomes *biome.Registry, defaultCost int, a *testActor, line string) error {
	reg := command.New()
	if err := command.RegisterBuiltins(reg); err != nil {
		return err
	}
	env := command.Env{World: w, Biomes: biomes, DefaultMoveCost: defaultCost}
	return reg.Dispatch(context.Background(), env, a, line)
}

// A destination biome's MoveCost overrides the flat default: stepping into a
// forest (move_cost 2) spends 2, not the Env default of 1.
func TestMove_BiomeWeightedCost(t *testing.T) {
	a := &world.Room{ID: "a", Name: "Road", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "b"}}}
	b := &world.Room{ID: "b", Name: "Wood", Terrain: "forest",
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "a"}}}
	w := world.New()
	w.AddRoom(a)
	w.AddRoom(b)

	biomes := biome.NewRegistry()
	if err := biomes.RegisterEngine(&biome.Biome{ID: "forest", MoveCost: 2}); err != nil {
		t.Fatalf("register forest biome: %v", err)
	}

	actor := newTestActor(a)
	actor.mvMax, actor.mv = 10, 10

	if err := moveCostBiomeDispatch(w, biomes, 1, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Room().ID != "b" {
		t.Fatalf("move into forest blocked; room = %q, want b", actor.Room().ID)
	}
	if actor.Movement() != 8 {
		t.Fatalf("forest step should cost 2 (mv 10 -> 8), got %d", actor.Movement())
	}
}

// Stepping onto rougher terrain than the room just left surfaces the
// "going is hard" hint; walking within the same terrain stays silent.
func TestMove_HardGoingHintOnRougherTerrain(t *testing.T) {
	// road(outdoors) -e-> wood(forest) -e-> deep(forest), and back west.
	road := &world.Room{ID: "road", Name: "Road", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirEast: {Target: "wood"}}}
	wood := &world.Room{ID: "wood", Name: "Wood", Terrain: "forest",
		Exits: map[world.Direction]world.Exit{world.DirEast: {Target: "deep"}, world.DirWest: {Target: "road"}}}
	deep := &world.Room{ID: "deep", Name: "Deep Wood", Terrain: "forest",
		Exits: map[world.Direction]world.Exit{world.DirWest: {Target: "wood"}}}
	w := world.New()
	w.AddRoom(road)
	w.AddRoom(wood)
	w.AddRoom(deep)

	biomes := biome.NewRegistry()
	if err := biomes.RegisterEngine(&biome.Biome{ID: "forest", MoveCost: 2}); err != nil {
		t.Fatalf("register forest biome: %v", err)
	}

	actor := newTestActor(road)
	actor.mvMax, actor.mv = 20, 20

	// road (cost 1) -> wood (forest, cost 2): rougher, so the hint fires.
	if err := moveCostBiomeDispatch(w, biomes, 1, actor, "e"); err != nil {
		t.Fatalf("move east: %v", err)
	}
	if joined := strings.Join(actorLines(actor), "\n"); !strings.Contains(joined, "going is hard") {
		t.Fatalf("entering forest from open ground should hint hard going:\n%s", joined)
	}

	// wood -> deep: same terrain (forest -> forest), so no new hint.
	actor.lines = nil
	if err := moveCostBiomeDispatch(w, biomes, 1, actor, "e"); err != nil {
		t.Fatalf("move east again: %v", err)
	}
	if joined := strings.Join(actorLines(actor), "\n"); strings.Contains(joined, "going is hard") {
		t.Fatalf("walking within forest should not repeat the hint:\n%s", joined)
	}
}

// A mover with no movement pool is not charged for the step, so it gets no
// hard-going hint even when the destination terrain is rougher.
func TestMove_NoHintWhenUnmetered(t *testing.T) {
	road := &world.Room{ID: "road", Name: "Road", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirEast: {Target: "wood"}}}
	wood := &world.Room{ID: "wood", Name: "Wood", Terrain: "forest",
		Exits: map[world.Direction]world.Exit{world.DirWest: {Target: "road"}}}
	w := world.New()
	w.AddRoom(road)
	w.AddRoom(wood)

	biomes := biome.NewRegistry()
	if err := biomes.RegisterEngine(&biome.Biome{ID: "forest", MoveCost: 2}); err != nil {
		t.Fatalf("register forest biome: %v", err)
	}

	actor := newTestActor(road) // mvMax defaults to 0 — unmetered, moves free

	if err := moveCostBiomeDispatch(w, biomes, 1, actor, "e"); err != nil {
		t.Fatalf("move east: %v", err)
	}
	if actor.Room().ID != "wood" {
		t.Fatalf("unmetered move blocked; room = %q, want wood", actor.Room().ID)
	}
	if joined := strings.Join(actorLines(actor), "\n"); strings.Contains(joined, "going is hard") {
		t.Fatalf("an unmetered mover should not get the hard-going hint:\n%s", joined)
	}
}

// encumbranceDispatch dispatches one move with an item store wired (so
// carried weight resolves) and a flat default cost.
func encumbranceDispatch(w *world.World, store *entities.Store, defaultCost int, a *testActor, line string) error {
	reg := command.New()
	if err := command.RegisterBuiltins(reg); err != nil {
		return err
	}
	env := command.Env{World: w, Items: store, DefaultMoveCost: defaultCost}
	return reg.Dispatch(context.Background(), env, a, line)
}

// loadActor spawns a single item of the given weight into the store and the
// actor's inventory.
func loadActor(t *testing.T, store *entities.Store, a *testActor, weight int) {
	t.Helper()
	inst, err := store.Spawn(&item.Template{ID: "x:ballast", Name: "ballast", Type: "junk"})
	if err != nil {
		t.Fatalf("spawn weighted item: %v", err)
	}
	inst.SetProperty("weight", weight)
	a.AddToInventory(inst.ID())
}

// Encumbrance surcharges each step by tier as load nears carry capacity:
// burdened (≥50%) adds 1, heavily burdened (≥90%) adds 2, on top of the
// terrain cost.
func TestMove_EncumbranceSurcharge(t *testing.T) {
	cases := []struct {
		name      string
		carryMax  int
		weight    int
		wantSpent int // terrain default 1 + tier surcharge
	}{
		{"unburdened (40%)", 10, 4, 1},       // 1 + 0
		{"burdened (50%)", 10, 5, 2},         // 1 + 1
		{"heavily burdened (90%)", 10, 9, 3}, // 1 + 2
		{"no carry cap → inert", 0, 100, 1},  // 1 + 0 (dormant without carry_max)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, a, _ := moveCostWorld()
			store := entities.NewStore()
			actor := newTestActor(a)
			actor.mvMax, actor.mv = 20, 20
			actor.carryMax = tc.carryMax
			loadActor(t, store, actor, tc.weight)

			if err := encumbranceDispatch(w, store, 1, actor, "n"); err != nil {
				t.Fatalf("move: %v", err)
			}
			if actor.Room().ID != "b" {
				t.Fatalf("move blocked; room = %q, want b", actor.Room().ID)
			}
			if got := 20 - actor.Movement(); got != tc.wantSpent {
				t.Fatalf("step spent %d movement; want %d (terrain 1 + tier)", got, tc.wantSpent)
			}
		})
	}
}

// With no explicit carry_max, encumbrance keys off the STR-derived
// capacity (carryPerStrength × STR = 8 × 10 = 80): a load at half of that
// burdens the mover and surcharges the step.
func TestMove_EncumbranceFromStrengthDerivedCapacity(t *testing.T) {
	w, a, _ := moveCostWorld()
	store := entities.NewStore()
	actor := newTestActor(a)
	actor.mvMax, actor.mv = 20, 20
	actor.str = 10                 // derived capacity 80; no explicit carry_max
	loadActor(t, store, actor, 40) // 50% of 80 → burdened, +1

	if err := encumbranceDispatch(w, store, 1, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if got := 20 - actor.Movement(); got != 2 {
		t.Fatalf("burdened step spent %d; want 2 (terrain 1 + burdened 1)", got)
	}
}

// The encumbrance surcharge stacks on top of biome-weighted terrain cost.
func TestMove_EncumbranceStacksWithBiome(t *testing.T) {
	road := &world.Room{ID: "road", Name: "Road", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirEast: {Target: "wood"}}}
	wood := &world.Room{ID: "wood", Name: "Wood", Terrain: "forest",
		Exits: map[world.Direction]world.Exit{world.DirWest: {Target: "road"}}}
	w := world.New()
	w.AddRoom(road)
	w.AddRoom(wood)

	biomes := biome.NewRegistry()
	if err := biomes.RegisterEngine(&biome.Biome{ID: "forest", MoveCost: 2}); err != nil {
		t.Fatalf("register forest biome: %v", err)
	}
	store := entities.NewStore()
	actor := newTestActor(road)
	actor.mvMax, actor.mv = 20, 20
	actor.carryMax = 10
	loadActor(t, store, actor, 9) // 90% → heavily burdened, +2

	reg := command.New()
	if err := command.RegisterBuiltins(reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	env := command.Env{World: w, Items: store, Biomes: biomes, DefaultMoveCost: 1}
	if err := reg.Dispatch(context.Background(), env, actor, "e"); err != nil {
		t.Fatalf("move east: %v", err)
	}
	// forest terrain 2 + heavy surcharge 2 = 4.
	if got := 20 - actor.Movement(); got != 4 {
		t.Fatalf("loaded forest step spent %d; want 4 (terrain 2 + heavy 2)", got)
	}
}

// Encumbrance must not leak into the difficulty hint: a burdened mover
// walking within one terrain (no roughening) still gets no hint, because
// the surcharge adds equally to the source and destination cost.
func TestMove_EncumbranceDoesNotTriggerHint(t *testing.T) {
	wood := &world.Room{ID: "wood", Name: "Wood", Terrain: "forest",
		Exits: map[world.Direction]world.Exit{world.DirEast: {Target: "deep"}}}
	deep := &world.Room{ID: "deep", Name: "Deep Wood", Terrain: "forest",
		Exits: map[world.Direction]world.Exit{world.DirWest: {Target: "wood"}}}
	w := world.New()
	w.AddRoom(wood)
	w.AddRoom(deep)

	biomes := biome.NewRegistry()
	if err := biomes.RegisterEngine(&biome.Biome{ID: "forest", MoveCost: 2}); err != nil {
		t.Fatalf("register forest biome: %v", err)
	}
	store := entities.NewStore()
	actor := newTestActor(wood)
	actor.mvMax, actor.mv = 20, 20
	actor.carryMax = 10
	loadActor(t, store, actor, 9) // heavily burdened

	reg := command.New()
	if err := command.RegisterBuiltins(reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	env := command.Env{World: w, Items: store, Biomes: biomes, DefaultMoveCost: 1}
	if err := reg.Dispatch(context.Background(), env, actor, "e"); err != nil {
		t.Fatalf("move east: %v", err)
	}
	if joined := strings.Join(actorLines(actor), "\n"); strings.Contains(joined, "going is hard") {
		t.Fatalf("encumbrance must not trigger the terrain hint within one biome:\n%s", joined)
	}
}

// When the destination biome sets no MoveCost, the Env's flat default applies.
func TestMove_DefaultCostWhenBiomeUnset(t *testing.T) {
	w, a, _ := moveCostWorld() // both rooms are bare outdoors, no biome registered
	actor := newTestActor(a)
	actor.mvMax, actor.mv = 10, 10

	// DefaultMoveCost 3, no biome registry: the step costs the flat default.
	if err := moveCostBiomeDispatch(w, nil, 3, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if actor.Movement() != 7 {
		t.Fatalf("uncosted-terrain step should use the default 3 (mv 10 -> 7), got %d", actor.Movement())
	}
}

// Armor speed penalty (armor-depth §7): heavier armor adds a movement surcharge.
// equipment.md gives medium/heavy body armor Speed 20 (vs 30 baseline) → +1 per
// step on top of terrain. A heavily-armored mover spends 2 where an unarmored one
// spends 1.
func TestMove_ArmorSpeedSurcharge(t *testing.T) {
	store := entities.NewStore()
	// A heavy armor (armor_speed 20) and a light one (armor_speed 30, no penalty).
	heavy, err := store.Spawn(&item.Template{
		ID: "core:plate", Name: "plate", Type: "item",
		EligibleSlots: []string{"body"}, ArmorTier: "heavy", ArmorBonus: 8, ArmorSpeed: 20,
	})
	if err != nil {
		t.Fatalf("spawn plate: %v", err)
	}
	light, _ := store.Spawn(&item.Template{
		ID: "core:padded", Name: "padded", Type: "item",
		EligibleSlots: []string{"body"}, ArmorTier: "light", ArmorBonus: 1, ArmorSpeed: 30,
	})

	step := func(armorID entities.EntityID) int {
		w, a, _ := moveCostWorld()
		actor := newTestActor(a)
		actor.mvMax, actor.mv = 10, 10
		actor.equipment = map[string]entities.EntityID{}
		if armorID != "" {
			actor.equipment["body"] = armorID
		}
		reg := command.New()
		if err := command.RegisterBuiltins(reg); err != nil {
			t.Fatalf("builtins: %v", err)
		}
		env := command.Env{World: w, Items: store, DefaultMoveCost: 1}
		if err := reg.Dispatch(context.Background(), env, actor, "n"); err != nil {
			t.Fatalf("move: %v", err)
		}
		if actor.Room().ID != "b" {
			t.Fatalf("move blocked; room = %q", actor.Room().ID)
		}
		return actor.mvMax - actor.Movement() // points spent on the step
	}

	if got := step(""); got != 1 {
		t.Errorf("unarmored step cost = %d, want 1 (terrain only)", got)
	}
	if got := step(light.ID()); got != 1 {
		t.Errorf("light-armored step cost = %d, want 1 (speed 30, no penalty)", got)
	}
	if got := step(heavy.ID()); got != 2 {
		t.Errorf("heavy-armored step cost = %d, want 2 (terrain 1 + armor-speed surcharge 1)", got)
	}
}

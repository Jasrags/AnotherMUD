package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// twoAreaWorld builds two named areas joined by a single boundary exit:
// village:square --north--> wild:edge (and back). Both rooms are lit
// outdoors so the render is unobstructed.
func twoAreaWorld() (*world.World, *world.Room) {
	w := world.New()
	w.AddArea(&world.Area{ID: "village", Name: "Emond's Field"})
	w.AddArea(&world.Area{ID: "wild", Name: "the Westwood"})
	sq := &world.Room{ID: "village:square", AreaID: "village", Name: "The Green",
		Description: "A broad common.", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "wild:edge"}}}
	edge := &world.Room{ID: "wild:edge", AreaID: "wild", Name: "Forest Edge",
		Description: "Trees crowd the road.", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "village:square"}}}
	w.AddRoom(sq)
	w.AddRoom(edge)
	return w, sq
}

func twoAreaDispatch(w *world.World, a *testActor, line string) error {
	reg := command.New()
	if err := command.RegisterBuiltins(reg); err != nil {
		return err
	}
	env := command.Env{World: w, Items: entities.NewStore(), Placement: entities.NewPlacement(),
		Light: newLightResolver(gameclock.PeriodDay)}
	return reg.Dispatch(context.Background(), env, a, line)
}

func TestMove_CrossingAreaNarratesZoneLine(t *testing.T) {
	w, start := twoAreaWorld()
	actor := newTestActor(start)
	// Seed last-seen with the start area (as the session spawn does), so
	// the crossing has a real "from".
	actor.SetLastAreaSeen("village")

	if err := twoAreaDispatch(w, actor, "n"); err != nil {
		t.Fatalf("move north: %v", err)
	}
	if actor.Room().ID != "wild:edge" {
		t.Fatalf("did not cross; room = %q", actor.Room().ID)
	}
	joined := strings.Join(actorLines(actor), "\n")
	if !strings.Contains(joined, "You leave") || !strings.Contains(joined, "Emond's Field") ||
		!strings.Contains(joined, "enter") || !strings.Contains(joined, "the Westwood") {
		t.Fatalf("crossing should narrate leave/enter with area names:\n%s", joined)
	}
	if actor.LastAreaSeen() != "wild" {
		t.Errorf("last-seen area = %q, want wild after crossing", actor.LastAreaSeen())
	}
}

func TestMove_IntraAreaNoZoneLine(t *testing.T) {
	// A move within one area must not narrate a crossing.
	w := world.New()
	w.AddArea(&world.Area{ID: "village", Name: "Emond's Field"})
	a := &world.Room{ID: "village:a", AreaID: "village", Name: "Lane", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "village:b"}}}
	b := &world.Room{ID: "village:b", AreaID: "village", Name: "Square", Terrain: world.TerrainOutdoors,
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "village:a"}}}
	w.AddRoom(a)
	w.AddRoom(b)
	actor := newTestActor(a)
	actor.SetLastAreaSeen("village")

	if err := twoAreaDispatch(w, actor, "n"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if strings.Contains(strings.Join(actorLines(actor), "\n"), "You leave") {
		t.Errorf("intra-area move should not narrate a crossing:\n%s", strings.Join(actorLines(actor), "\n"))
	}
}

func TestMove_FirstEntryBannerFiresOnceThenPairsWithZoneLine(t *testing.T) {
	w, start := twoAreaWorld()
	actor := newTestActor(start)
	actor.SetLastAreaSeen("village")
	actor.MarkAreaSeen("village") // already home; only wild is new

	// First crossing into wild: zone-line AND the once-ever banner.
	if err := twoAreaDispatch(w, actor, "n"); err != nil {
		t.Fatalf("move north: %v", err)
	}
	joined := strings.Join(actorLines(actor), "\n")
	if !strings.Contains(joined, "You leave") {
		t.Errorf("first crossing should still narrate the zone-line:\n%s", joined)
	}
	if !strings.Contains(joined, "for the first time") || !strings.Contains(joined, "the Westwood") {
		t.Errorf("first crossing should show the first-entry banner:\n%s", joined)
	}

	// Go back, then cross into wild AGAIN: zone-line only, no banner.
	if err := twoAreaDispatch(w, actor, "s"); err != nil {
		t.Fatalf("move south: %v", err)
	}
	actor.clearLines()
	if err := twoAreaDispatch(w, actor, "n"); err != nil {
		t.Fatalf("re-enter wild: %v", err)
	}
	re := strings.Join(actorLines(actor), "\n")
	if !strings.Contains(re, "You leave") {
		t.Errorf("re-entry should narrate the zone-line:\n%s", re)
	}
	if strings.Contains(re, "for the first time") {
		t.Errorf("re-entry must NOT repeat the first-entry banner:\n%s", re)
	}
}

func TestMove_FirstCrossingFromSpawnHasNoLeaveLine(t *testing.T) {
	// With no seeded last-seen area (lastArea == ""), the first crossing
	// has no "from" and must suppress the leave/enter line — but it still
	// records the new area so the NEXT crossing narrates.
	w, start := twoAreaWorld()
	actor := newTestActor(start) // lastArea defaults to ""

	if err := twoAreaDispatch(w, actor, "n"); err != nil {
		t.Fatalf("move north: %v", err)
	}
	if strings.Contains(strings.Join(actorLines(actor), "\n"), "You leave") {
		t.Errorf("first crossing with no prior area should not narrate a leave line")
	}
	if actor.LastAreaSeen() != "wild" {
		t.Errorf("last-seen area = %q, want wild (recorded even when suppressed)", actor.LastAreaSeen())
	}
}

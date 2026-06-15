package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// hiddenExitWorld builds room A with a single HIDDEN north exit (difficulty
// 10) into B, plus B's south exit back. A has no other exits, so the exits
// line is "none" until the secret passage is discovered.
func hiddenExitWorld() (*world.World, *world.Room, *world.Room) {
	a := &world.Room{ID: "a", Name: "Study", Description: "A dusty study.", Terrain: world.TerrainIndoors,
		Exits: map[world.Direction]world.Exit{
			world.DirNorth: {Target: "b", Hidden: true, SearchDifficulty: 10},
		}}
	b := &world.Room{ID: "b", Name: "Vault", Description: "A hidden vault.", Terrain: world.TerrainIndoors,
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "a"}}}
	w := world.New()
	w.AddRoom(a)
	w.AddRoom(b)
	return w, a, b
}

func searchEnv(w *world.World, store *entities.Store, place *entities.Placement) command.Env {
	return command.Env{
		World:     w,
		Items:     store,
		Placement: place,
		Light:     newLightResolver(gameclock.PeriodDay),
	}
}

// A winning search discovers the hidden exit: it enters the actor's discovery
// memory, announces the direction, and emits exit.discovered.
func TestSearch_DiscoversHiddenExitOnWin(t *testing.T) {
	w, a, _ := hiddenExitWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventExitDiscovered)
	env := searchEnv(w, store, place)
	env.Bus = bus
	env.SkillRoller = pickRoller{raw: 9} // face 10 + 0 perception + 5 search bonus = 15 ≥ 10

	dispatchActor(t, newRegistry(t), env, actor, "search")

	if !actor.IsExitDiscovered(world.DirNorth) {
		t.Fatal("a winning search must record the hidden exit as discovered")
	}
	if out := actor.lastLine(); !strings.Contains(strings.ToLower(out), "hidden passage leading north") {
		t.Errorf("discovery message = %q, want a north discovery line", out)
	}
	if len(*got) != 1 {
		t.Fatalf("ExitDiscovered fired %d times, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.ExitDiscovered); ev.Direction != "n" || ev.TargetRoom != "b" {
		t.Errorf("ExitDiscovered = %+v, want dir=n target=b", ev)
	}
}

// A failing search finds nothing — no discovery, no event, empty-result line.
func TestSearch_FailFindsNothing(t *testing.T) {
	w, a, _ := hiddenExitWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventExitDiscovered)
	env := searchEnv(w, store, place)
	env.Bus = bus
	env.SkillRoller = pickRoller{raw: 0} // face 1 + 5 = 6 < 10 → fail

	dispatchActor(t, newRegistry(t), env, actor, "search")

	if actor.IsExitDiscovered(world.DirNorth) {
		t.Error("a failing search must not discover the exit")
	}
	if len(*got) != 0 {
		t.Error("a failing search must not emit exit.discovered")
	}
	if out := actor.lastLine(); !strings.Contains(strings.ToLower(out), "find nothing") {
		t.Errorf("empty-result line = %q", out)
	}
}

// detect_hidden auto-discovers with no contest (visibility §4.3 / hidden-exits §3.3).
func TestSearch_DetectHiddenAutoDiscovers(t *testing.T) {
	w, a, _ := hiddenExitWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	actor.tags = []string{command.DetectHiddenFlag}
	env := searchEnv(w, store, place)
	env.Bus = eventbus.New()
	// No roller: detect_hidden must not need a contest.

	dispatchActor(t, newRegistry(t), env, actor, "search")
	if !actor.IsExitDiscovered(world.DirNorth) {
		t.Error("detect_hidden must auto-discover a hidden exit without a contest")
	}
}

// An undiscovered hidden exit is unwalkable: typing its direction fails
// exactly like a non-existent exit (hidden-exits §4.1). After discovery the
// same direction moves normally.
func TestMove_HiddenExitGatedUntilDiscovered(t *testing.T) {
	w, a, _ := hiddenExitWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	env := searchEnv(w, store, place)
	reg := newRegistry(t)

	// Undiscovered: silent-fail with the no-exit message; room unchanged.
	dispatchActor(t, reg, env, actor, "n")
	if actor.Room().ID != "a" {
		t.Fatalf("an undiscovered hidden exit must not be walkable; room = %q", actor.Room().ID)
	}
	if out := actor.lastLine(); !strings.Contains(out, "cannot go that way") {
		t.Errorf("undiscovered hidden-exit move = %q, want the no-exit message (indistinguishable)", out)
	}

	// Discover it, then the same direction works.
	actor.DiscoverExit(world.DirNorth)
	dispatchActor(t, reg, env, actor, "n")
	if actor.Room().ID != "b" {
		t.Fatalf("after discovery the hidden exit must be walkable; room = %q", actor.Room().ID)
	}
}

// The exits line omits an undiscovered hidden exit and shows it once found
// (hidden-exits §5.1).
func TestLook_HiddenExitFilteredFromExitsLine(t *testing.T) {
	w, a, _ := hiddenExitWorld()
	store, place := entities.NewStore(), entities.NewPlacement()
	actor := newTestActor(a)
	env := searchEnv(w, store, place)
	reg := newRegistry(t)

	dispatchActor(t, reg, env, actor, "look")
	if out := actor.lastLine(); strings.Contains(strings.ToLower(out), "north") {
		t.Errorf("undiscovered hidden exit must not appear in the exits line: %q", out)
	}

	actor.DiscoverExit(world.DirNorth)
	dispatchActor(t, reg, env, actor, "look")
	if out := actor.lastLine(); !strings.Contains(strings.ToLower(out), "north") {
		t.Errorf("a discovered hidden exit must appear in the exits line: %q", out)
	}
}

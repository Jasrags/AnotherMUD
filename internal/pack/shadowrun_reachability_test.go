package pack

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// reachableFrom returns every room reachable from start by walking directional
// exits (the on-foot movement graph — doors don't remove an edge, they gate it).
func reachableFrom(w *world.World, start world.RoomID) map[world.RoomID]bool {
	seen := map[world.RoomID]bool{start: true}
	queue := []world.RoomID{start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		r, err := w.Room(id)
		if err != nil {
			continue
		}
		for _, ex := range r.Exits {
			if !seen[ex.Target] {
				seen[ex.Target] = true
				queue = append(queue, ex.Target)
			}
		}
	}
	return seen
}

// TestShadowrun_StarterAreaReachability pins the onboarding path: a player who
// spawns in the safehouse (shadowrun:the-flop, the make run-shadowrun start) can
// WALK through the whole world on foot — the graduation into Downtown, the deep
// ripperdoc (Scalpel's Chrome Den in Puyallup), and the tutorial chop-doc's strip
// (Loveland) — with no teleport and no stranded content.
func TestShadowrun_StarterAreaReachability(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, []string{"shadowrun"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load shadowrun: %v", err)
	}

	const start = world.RoomID("shadowrun:the-flop")
	if _, err := regs.World.Room(start); err != nil {
		t.Fatalf("the safehouse start room %s does not exist: %v", start, err)
	}
	reachable := reachableFrom(regs.World, start)

	for _, want := range []world.RoomID{
		"shadowrun:the-fixers-table", // the safehouse gear shop
		"shadowrun:the-back-room",    // the tutorial chop-doc room
		"shadowrun:westlake-plaza",   // graduation into Downtown
		"shadowrun:chrome-den",       // the first real street-doc (Scalpel's clinic)
		"shadowrun:loveland",         // the tutorial chop-doc's strip
	} {
		if !reachable[want] {
			t.Errorf("%s is NOT reachable on foot from %s — stranded content", want, start)
		}
	}

	// Graduation is a two-way stairwell: the flop drops to Westlake, Westlake
	// climbs back up to the flop (so a graduated runner can revisit the fixer).
	flop, err := regs.World.Room(start)
	if err != nil {
		t.Fatalf("flop room missing: %v", err)
	}
	if down, ok := flop.Exits[world.DirDown]; !ok || down.Target != "shadowrun:westlake-plaza" {
		t.Errorf("the-flop down-exit = %v (ok=%v), want shadowrun:westlake-plaza", down.Target, ok)
	}
	plaza, err := regs.World.Room("shadowrun:westlake-plaza")
	if err != nil {
		t.Fatalf("westlake-plaza room missing: %v", err)
	}
	if up, ok := plaza.Exits[world.DirUp]; !ok || up.Target != "shadowrun:the-flop" {
		t.Errorf("westlake-plaza up-exit = %v (ok=%v), want shadowrun:the-flop", up.Target, ok)
	}

	// The Chrome Den's back-and-forth wiring is symmetric (you can leave again).
	den, err := regs.World.Room("shadowrun:chrome-den")
	if err != nil {
		t.Fatalf("chrome-den room missing: %v", err)
	}
	if up, ok := den.Exits[world.DirUp]; !ok || up.Target != "shadowrun:hells-kitchen" {
		t.Errorf("chrome-den up-exit = %v (ok=%v), want shadowrun:hells-kitchen", up.Target, ok)
	}
}

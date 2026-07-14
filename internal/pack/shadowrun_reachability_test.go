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

// TestShadowrun_RipperdocReachableFromStart pins the reachability concern behind
// the first-real-street-doc build: a player who spawns at the real Downtown start
// (shadowrun:westlake-plaza, the make run-shadowrun start) can WALK to Scalpel's
// Chrome Den in the Puyallup Barrens — no teleport, no stranded content. Also
// confirms the tutorial chop-doc's room (Loveland) is reachable.
func TestShadowrun_RipperdocReachableFromStart(t *testing.T) {
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

	const start = world.RoomID("shadowrun:westlake-plaza")
	if _, err := regs.World.Room(start); err != nil {
		t.Fatalf("the Downtown start room %s does not exist: %v", start, err)
	}
	reachable := reachableFrom(regs.World, start)

	for _, want := range []world.RoomID{
		"shadowrun:chrome-den", // the first real street-doc (Scalpel's clinic)
		"shadowrun:loveland",   // the tutorial chop-doc's strip
	} {
		if !reachable[want] {
			t.Errorf("%s is NOT reachable on foot from %s — the doc is stranded", want, start)
		}
	}

	// The Chrome Den's back-and-forth wiring is symmetric (you can leave again).
	den, err := regs.World.Room("shadowrun:chrome-den")
	if err != nil {
		t.Fatalf("chrome-den room missing: %v", err)
	}
	up, ok := den.Exits[world.DirUp]
	if !ok || up.Target != "shadowrun:hells-kitchen" {
		t.Errorf("chrome-den up-exit = %v (ok=%v), want shadowrun:hells-kitchen", up.Target, ok)
	}
}

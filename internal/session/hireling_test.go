package session

import (
	"strconv"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// The durable hireling-ownership surface (hireable-mobs.md §9): add records,
// count them for the cap, list them, and drop one by template.
func TestConnActor_HirelingOwnership(t *testing.T) {
	a := &connActor{save: &player.Save{}}
	if a.HirelingCount() != 0 {
		t.Fatalf("fresh count = %d, want 0", a.HirelingCount())
	}
	a.AddHireling("sw:sellsword")
	a.AddHireling("sw:archer")
	if a.HirelingCount() != 2 {
		t.Fatalf("count after 2 adds = %d, want 2", a.HirelingCount())
	}
	got := a.OwnedHirelingTemplates()
	if len(got) != 2 || got[0] != "sw:sellsword" || got[1] != "sw:archer" {
		t.Fatalf("owned = %v, want [sw:sellsword sw:archer] in order", got)
	}
	if !a.RemoveHireling("sw:sellsword") {
		t.Error("RemoveHireling should report the record was dropped")
	}
	if a.RemoveHireling("sw:nobody") {
		t.Error("RemoveHireling of an unknown template should report false")
	}
	if got := a.OwnedHirelingTemplates(); len(got) != 1 || got[0] != "sw:archer" {
		t.Fatalf("owned after remove = %v, want [sw:archer]", got)
	}
}

// The transient live overlay: track a materialized hireling, resolve it by
// template, untrack it.
func TestConnActor_LiveHirelingTracking(t *testing.T) {
	a := &connActor{}
	a.TrackLiveHireling("h-1", "sw:sellsword")
	id, ok := a.LiveHireling("sw:sellsword")
	if !ok || id != "h-1" {
		t.Fatalf("LiveHireling = (%q,%v), want (h-1, true)", id, ok)
	}
	if _, ok := a.LiveHireling("sw:missing"); ok {
		t.Error("LiveHireling of an unmaterialized template should be false")
	}
	if tmpl, ok := a.UntrackLiveHireling("h-1"); !ok || tmpl != "sw:sellsword" {
		t.Fatalf("Untrack = (%q,%v), want (sw:sellsword, true)", tmpl, ok)
	}
	if _, ok := a.LiveHireling("sw:sellsword"); ok {
		t.Error("hireling should be gone after untrack")
	}
}

// drainLiveHirelings must be race-safe against the dismiss-path mutator
// (UntrackLiveHireling) on the same actor (hireable-mobs.md §9) — the
// snapshot-and-clear stops a logout from double-removing a hireling a concurrent
// `dismiss` is also removing. Run under -race.
func TestConnActor_DrainLiveHirelingsConcurrent(t *testing.T) {
	a := &connActor{}
	const n = 200
	ids := make([]entities.EntityID, n)
	for i := range ids {
		ids[i] = entities.EntityID("h-" + strconv.Itoa(i))
		a.TrackLiveHireling(ids[i], "test:merc")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = a.drainLiveHirelings() }()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id entities.EntityID) { defer wg.Done(); a.UntrackLiveHireling(id) }(ids[i])
	}
	wg.Wait()
	if _, ok := a.LiveHireling("test:merc"); ok {
		t.Error("live hirelings should be empty after concurrent drain+untrack")
	}
}

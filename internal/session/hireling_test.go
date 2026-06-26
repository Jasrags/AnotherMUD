package session

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
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

// PullHirelings relocates an owner's live hirelings to the owner's new room
// (hireable-mobs.md §5) — the hireling is bound, always co-located.
func TestPullHirelings_FollowsOwner(t *testing.T) {
	from, to := world.RoomID("z:a"), world.RoomID("z:b")
	place := entities.NewPlacement()
	mgr := NewManager()
	mgr.actionEnv = command.Env{Placement: place}

	owner := &connActor{id: "c-boss", playerID: "boss", room: &world.Room{ID: from}}
	mgr.Add(owner)

	const hid = entities.EntityID("h-1")
	place.Place(hid, from)
	owner.TrackLiveHireling(hid, "sw:sellsword")

	// Owner walks from→to; the hireling is pulled along.
	mgr.PullHirelings(context.Background(), "boss", from, to)
	if got, _ := place.RoomOf(hid); got != to {
		t.Fatalf("hireling room = %q, want %q (it should follow the owner)", got, to)
	}
}

// An owner with no live hireling is a no-op (no panic, nothing to move).
func TestPullHirelings_NoneIsNoop(t *testing.T) {
	mgr := NewManager()
	mgr.actionEnv = command.Env{Placement: entities.NewPlacement()}
	owner := &connActor{id: "c-solo", playerID: "solo", room: &world.Room{ID: "z:a"}}
	mgr.Add(owner)
	mgr.PullHirelings(context.Background(), "solo", "z:a", "z:b") // must not panic
}

// OnHirelingDeath ends the contract for a slain hireling and is a no-op for an
// ordinary mob death (hireable-mobs.md §6.2).
func TestOnHirelingDeath_EndsContract(t *testing.T) {
	mgr := NewManager()
	owner := &connActor{id: "c-boss", playerID: "boss", room: &world.Room{ID: "z:a"},
		save: &player.Save{}, conn: &fakeConn{id: "boss"}}
	mgr.Add(owner)
	owner.AddHireling("sw:sellsword")
	owner.TrackLiveHireling("h-1", "sw:sellsword")

	if !mgr.OnHirelingDeath(context.Background(), "h-1") {
		t.Fatal("OnHirelingDeath should report the slain mob was a hireling")
	}
	if owner.HirelingCount() != 0 {
		t.Errorf("contract should be gone after death, count = %d", owner.HirelingCount())
	}
	if _, ok := owner.LiveHireling("sw:sellsword"); ok {
		t.Error("the live hireling should be untracked after death")
	}
	if mgr.OnHirelingDeath(context.Background(), "wild-mob") {
		t.Error("an ordinary (non-hireling) mob death should report false")
	}
}

// HirelingCombatantsOf returns the owner's live hireling entity ids for the
// combat-assist seam (hireable-mobs.md §6.1); an owner with none, or an unknown
// owner, yields nothing.
func TestHirelingCombatantsOf(t *testing.T) {
	mgr := NewManager()
	owner := &connActor{id: "c-boss", playerID: "boss", room: &world.Room{ID: "z:a"}}
	mgr.Add(owner)
	if got := mgr.HirelingCombatantsOf("boss"); got != nil {
		t.Fatalf("no hirelings → %v, want nil", got)
	}
	owner.TrackLiveHireling("h-1", "sw:sellsword")
	got := mgr.HirelingCombatantsOf("boss")
	if len(got) != 1 || got[0] != "h-1" {
		t.Fatalf("got %v, want [h-1]", got)
	}
	if got := mgr.HirelingCombatantsOf("ghost"); got != nil {
		t.Fatalf("unknown owner → %v, want nil", got)
	}
}

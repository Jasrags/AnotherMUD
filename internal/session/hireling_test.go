package session

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
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

// When a bound hireling relocates with its owner, bystanders in the old and new
// rooms see it leave and arrive (hireable-mobs.md §5).
func TestPullHirelings_BroadcastsDepartureAndArrival(t *testing.T) {
	from, to := world.RoomID("z:a"), world.RoomID("z:b")
	w := world.New()
	w.AddRoom(&world.Room{ID: from, Name: "A",
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: to}}})
	w.AddRoom(&world.Room{ID: to, Name: "B",
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: from}}})

	place := entities.NewPlacement()
	store := entities.NewStore()
	hireling, err := store.SpawnMob(&mob.Template{ID: "sw:sellsword", Name: "a grizzled sellsword", Type: "npc"})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}

	mgr := NewManager()
	mgr.actionEnv = command.Env{Placement: place, Items: store, World: w, Broadcaster: mgr}

	// Owner arrives in `to`; a bystander stays in `from`, another waits in `to`.
	owner, ownerConn := newFakeActor("c-boss", "boss", "acc", "Captara", &world.Room{ID: to})
	stayer, stayerConn := newFakeActor("c-stay", "stay", "acc", "Stayer", &world.Room{ID: from})
	greeter, greeterConn := newFakeActor("c-greet", "greet", "acc", "Greeter", &world.Room{ID: to})
	mgr.Add(owner)
	mgr.Add(stayer)
	mgr.Add(greeter)

	place.Place(hireling.ID(), from)
	owner.TrackLiveHireling(hireling.ID(), "sw:sellsword")

	mgr.PullHirelings(context.Background(), "boss", from, to)

	if lines := stayerConn.writes(); len(lines) != 1 || !strings.Contains(lines[0], "a grizzled sellsword follows Captara north.") {
		t.Errorf("bystander left behind saw %v, want a follow-north line", stayerConn.writes())
	}
	if lines := greeterConn.writes(); len(lines) != 1 || !strings.Contains(lines[0], "a grizzled sellsword arrives, following Captara.") {
		t.Errorf("bystander in the new room saw %v, want an arrival line", greeterConn.writes())
	}
	// The owner is excluded from the arrival line (no per-step self-spam).
	if lines := ownerConn.writes(); len(lines) != 0 {
		t.Errorf("owner should not see their own hireling's arrival line, got %v", lines)
	}
}

// A non-adjacent owner move (recall/teleport — no exit linking the rooms) drops
// the direction and uses the "slips away" departure phrasing (hireable-mobs.md §5).
func TestPullHirelings_NonAdjacentDeparturePhrase(t *testing.T) {
	from, to := world.RoomID("z:a"), world.RoomID("z:far")
	w := world.New()
	w.AddRoom(&world.Room{ID: from, Name: "A"}) // no exit to `to` → non-adjacent
	w.AddRoom(&world.Room{ID: to, Name: "Far"})

	place := entities.NewPlacement()
	store := entities.NewStore()
	hireling, err := store.SpawnMob(&mob.Template{ID: "sw:sellsword", Name: "a grizzled sellsword", Type: "npc"})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}

	mgr := NewManager()
	mgr.actionEnv = command.Env{Placement: place, Items: store, World: w, Broadcaster: mgr}
	owner, _ := newFakeActor("c-boss", "boss", "acc", "Captara", &world.Room{ID: to})
	stayer, stayerConn := newFakeActor("c-stay", "stay", "acc", "Stayer", &world.Room{ID: from})
	mgr.Add(owner)
	mgr.Add(stayer)
	place.Place(hireling.ID(), from)
	owner.TrackLiveHireling(hireling.ID(), "sw:sellsword")

	mgr.PullHirelings(context.Background(), "boss", from, to)

	if lines := stayerConn.writes(); len(lines) != 1 || !strings.Contains(lines[0], "a grizzled sellsword slips away, following Captara.") {
		t.Errorf("non-adjacent departure saw %v, want a slips-away line", stayerConn.writes())
	}
}

// A stay/guard hireling holds its room when the owner moves (hireable-mobs.md §8):
// only follow-stance hirelings trail.
func TestPullHirelings_StanceHoldsPosition(t *testing.T) {
	from, to := world.RoomID("z:a"), world.RoomID("z:b")
	place := entities.NewPlacement()
	mgr := NewManager()
	mgr.actionEnv = command.Env{Placement: place}
	owner := &connActor{id: "c-boss", playerID: "boss", room: &world.Room{ID: from}}
	mgr.Add(owner)

	const stayer = entities.EntityID("h-stay")
	const trailer = entities.EntityID("h-follow")
	place.Place(stayer, from)
	place.Place(trailer, from)
	owner.TrackLiveHireling(stayer, "sw:sellsword")
	owner.TrackLiveHireling(trailer, "sw:guard")
	owner.SetHirelingStance(stayer, command.HirelingStanceStay)

	mgr.PullHirelings(context.Background(), "boss", from, to)

	if got, _ := place.RoomOf(stayer); got != from {
		t.Errorf("stay hireling moved to %q, want %q (it should hold)", got, from)
	}
	if got, _ := place.RoomOf(trailer); got != to {
		t.Errorf("follow hireling room = %q, want %q (it should trail)", got, to)
	}
}

// A follow hireling that is mid-combat is NOT yanked along when the owner moves
// (hireable-mobs.md §5/§6) — it holds its ground; the owner is told. A second,
// non-fighting follow hireling still trails normally.
func TestPullHirelings_SkipsFightingHireling(t *testing.T) {
	from, to := world.RoomID("z:a"), world.RoomID("z:b")
	place := entities.NewPlacement()

	// A real combat manager; engage the fighter so InCombat reports true.
	const fighter = entities.EntityID("h-fighter")
	const idler = entities.EntityID("h-idle")
	fighterCID := combat.NewMobCombatantID(string(fighter))
	foeCID := combat.NewMobCombatantID("foe-1")
	loc := combat.MapLocator{
		fighterCID: &fakeCombatant{id: fighterCID, name: "the fighter", vitals: combat.NewVitals(20)},
		foeCID:     &fakeCombatant{id: foeCID, name: "a bandit", vitals: combat.NewVitals(20)},
	}
	cm := combat.NewManager(loc, nil)
	if _, ok := cm.EngageWithReason(context.Background(), fighterCID, foeCID, from); !ok {
		t.Fatal("could not engage the fighter for the test")
	}

	mgr := NewManager()
	mgr.actionEnv = command.Env{Placement: place, Combat: cm}
	owner := &connActor{id: "c-boss", playerID: "boss", room: &world.Room{ID: to},
		conn: &fakeConn{id: "boss"}}
	mgr.Add(owner)

	place.Place(fighter, from)
	place.Place(idler, from)
	owner.TrackLiveHireling(fighter, "sw:sellsword") // follow by default
	owner.TrackLiveHireling(idler, "sw:guard")       // follow by default

	mgr.PullHirelings(context.Background(), "boss", from, to)

	if got, _ := place.RoomOf(fighter); got != from {
		t.Errorf("fighting hireling moved to %q, want %q (it should hold its fight)", got, from)
	}
	if got, _ := place.RoomOf(idler); got != to {
		t.Errorf("idle hireling room = %q, want %q (it should still trail)", got, to)
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
	const room = world.RoomID("z:a")
	mgr := NewManager()
	owner := &connActor{id: "c-boss", playerID: "boss", room: &world.Room{ID: room}}
	mgr.Add(owner)
	if got := mgr.HirelingCombatantsOf("boss", room); got != nil {
		t.Fatalf("no hirelings → %v, want nil", got)
	}
	owner.TrackLiveHireling("h-1", "sw:sellsword") // defaults to follow → assists
	got := mgr.HirelingCombatantsOf("boss", room)
	if len(got) != 1 || got[0] != "h-1" {
		t.Fatalf("got %v, want [h-1]", got)
	}
	// A stay-stance hireling stands down — excluded from the assist set (§8).
	owner.SetHirelingStance("h-1", command.HirelingStanceStay)
	if got := mgr.HirelingCombatantsOf("boss", room); got != nil {
		t.Fatalf("stay hireling assisted → %v, want nil", got)
	}
	// Guard re-enters the assist set (placement unwired here, so the room gate is
	// skipped — guard's room filtering is exercised in the live test).
	owner.SetHirelingStance("h-1", command.HirelingStanceGuard)
	if got := mgr.HirelingCombatantsOf("boss", room); len(got) != 1 || got[0] != "h-1" {
		t.Fatalf("guard hireling → %v, want [h-1]", got)
	}
	if got := mgr.HirelingCombatantsOf("ghost", room); got != nil {
		t.Fatalf("unknown owner → %v, want nil", got)
	}
}

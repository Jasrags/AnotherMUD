package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/action"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// loaderActor is a combatActor that also satisfies the reload-state +
// ammoConsumer seams the `load` / crossbow path needs.
type loaderActor struct {
	*combatActor
	loaded bool
	ammo   int
}

func newLoaderActor(name, pid string, room *world.Room, reloadTicks, ammo int) *loaderActor {
	a := &loaderActor{combatActor: newCombatActor(name, pid, room), ammo: ammo}
	s := combat.DefaultPlayerStats()
	s.RangedClass = combat.RangedProjectile
	s.AmmoKind = "bolt"
	s.ReloadTicks = reloadTicks
	a.stats = s
	return a
}

func (a *loaderActor) IsWeaponLoaded() bool  { return a.loaded }
func (a *loaderActor) SetWeaponLoaded() bool { a.loaded = true; return true }
func (a *loaderActor) ClearWeaponLoaded()    { a.loaded = false }
func (a *loaderActor) ConsumeAmmo(kind string) (string, bool) {
	if a.ammo <= 0 {
		return "", false
	}
	a.ammo--
	return "", true
}

// load with no timed-action substrate (Actions nil) loads instantly: consumes a
// bolt and chambers the weapon (action-economy.md §7.1).
func TestLoad_InstantWhenNoTracker(t *testing.T) {
	room := &world.Room{ID: "z:a", Name: "Road"}
	a := newLoaderActor("Alice", "p-1", room, 20, 2)
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), command.Env{}, a, "load"); err != nil {
		t.Fatal(err)
	}
	if !a.loaded {
		t.Error("instant load should chamber the weapon")
	}
	if a.ammo != 1 {
		t.Errorf("ammo = %d, want 1 (one bolt consumed)", a.ammo)
	}
}

// load is a two-phase timed action when the substrate is wired: phase 1 arms the
// reload (weapon NOT yet loaded, no ammo spent), the replay chambers it.
func TestLoad_TwoPhaseTimed(t *testing.T) {
	room := &world.Room{ID: "z:a", Name: "Road"}
	a := newLoaderActor("Alice", "p-1", room, 20, 2)
	r := newRegistry(t)
	tr := action.NewTracker()
	var now uint64 = 100
	env := command.Env{Actions: tr, NowTick: func() uint64 { return now }}
	ctx := context.Background()

	// Phase 1: arm — not loaded, no bolt spent, action in flight.
	if err := r.Dispatch(ctx, env, a, "load"); err != nil {
		t.Fatal(err)
	}
	if a.loaded {
		t.Error("phase 1 should not chamber the weapon yet")
	}
	if a.ammo != 2 {
		t.Errorf("phase 1 should not spend a bolt; ammo = %d, want 2", a.ammo)
	}
	act, busy := tr.Active("p-1")
	if !busy || act.Kind != command.KindReload || act.ReadyAt != now+20 {
		t.Fatalf("want a reload action ReadyAt=120, got %+v busy=%v", act, busy)
	}

	// Phase 2: the sweep claims + replays.
	if _, due := tr.CompleteReady("p-1", now+20); !due {
		t.Fatal("reload should be due")
	}
	env.ReplayAction = true
	if err := r.Dispatch(ctx, env, a, "load"); err != nil {
		t.Fatal(err)
	}
	if !a.loaded {
		t.Error("the replay should chamber the weapon")
	}
	if a.ammo != 1 {
		t.Errorf("the replay should spend one bolt; ammo = %d, want 1", a.ammo)
	}
}

// load refuses (no chamber, no bolt spent) when out of ammunition at completion.
func TestLoad_OutOfAmmo(t *testing.T) {
	room := &world.Room{ID: "z:a", Name: "Road"}
	a := newLoaderActor("Alice", "p-1", room, 20, 0) // no bolts
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), command.Env{}, a, "load"); err != nil {
		t.Fatal(err)
	}
	if a.loaded {
		t.Error("load with no ammo must not chamber the weapon")
	}
	if !strings.Contains(strings.Join(actorLines(a.combatActor.testActor), "\n"), "no bolt") {
		t.Errorf("expected an out-of-bolts message, got %v", actorLines(a.combatActor.testActor))
	}
}

// A non-reload weapon (ReloadTicks 0, a freely-firing bow) reports nothing to load.
func TestLoad_NotAReloadWeapon(t *testing.T) {
	room := &world.Room{ID: "z:a", Name: "Road"}
	a := newLoaderActor("Alice", "p-1", room, 0, 5)
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), command.Env{}, a, "load"); err != nil {
		t.Fatal(err)
	}
	if a.loaded {
		t.Error("a non-reload weapon should not become loaded")
	}
}

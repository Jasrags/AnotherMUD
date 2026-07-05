package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/rangedflavor"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// shootFixture builds the smallest cross-room environment for the `shoot`
// verb (ranged-combat Model C slice 1): two rooms A <-> B joined by a
// north/south exit, an entity store + placement, and a guard mob standing in
// room B (the target room). The shooter stands in room A.
type shootFixture struct {
	world *world.World
	roomA *world.Room
	roomB *world.Room
	store *entities.Store
	place *entities.Placement
	guard *entities.MobInstance
}

func newShootFixture(t *testing.T) *shootFixture {
	t.Helper()
	w := world.New()
	a := &world.Room{ID: "z:a", Name: "Road", Description: "A road.",
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "z:b"}}}
	b := &world.Room{ID: "z:b", Name: "Field", Description: "A field.",
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "z:a"}}}
	w.AddRoom(a)
	w.AddRoom(b)
	store := entities.NewStore()
	place := entities.NewPlacement()
	guard, err := store.SpawnMob(guardTplForConsider())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	place.Place(guard.ID(), b.ID)
	return &shootFixture{world: w, roomA: a, roomB: b, store: store, place: place, guard: guard}
}

// archer returns a combatActor standing in room A with a projectile weapon
// profile (so attacker.Stats() reports a bow). ammoKind is the consumed
// ammunition; the bare combatActor does not satisfy the ammoConsumer seam, so
// it fires freely — the ammo-spend paths are covered with ammoArcher below.
func (f *shootFixture) archer(ammoKind string) *combatActor {
	a := newCombatActor("Alice", "p-1", f.roomA)
	s := combat.DefaultPlayerStats()
	s.RangedClass = combat.RangedProjectile
	s.AmmoKind = ammoKind
	a.stats = s
	return a
}

// shootEnv assembles the command.Env: world/store/placement, a real combat
// Manager (so the verb's non-nil guard passes), a capturing ResolveAttack, and
// a recording Broadcaster for the two-room narration.
func (f *shootFixture) shootEnv(a combat.Combatant) (command.Env, *shotCapture, *recordingBroadcaster) {
	shot := &shotCapture{}
	rec := &recordingBroadcaster{}
	env := command.Env{World: f.world, Items: f.store, Placement: f.place, Broadcaster: rec}
	env.Combat = combat.NewManager(combat.MapLocator{
		a.CombatantID():       a,
		f.guard.CombatantID(): f.guard,
	}, &recordingSink{})
	env.ResolveAttack = func(_ context.Context, attacker, target combat.CombatantID, room world.RoomID) bool {
		shot.called = true
		shot.attacker, shot.target, shot.room = attacker, target, room
		return true
	}
	return env, shot, rec
}

type shotCapture struct {
	called           bool
	attacker, target combat.CombatantID
	room             world.RoomID
}

func (b *recordingBroadcaster) toRoom(room world.RoomID) (string, bool) {
	for _, c := range b.calls {
		if c.roomID == room {
			return c.text, true
		}
	}
	return "", false
}

func TestShoot_NoProjectileWielded(t *testing.T) {
	f := newShootFixture(t)
	a := newCombatActor("Alice", "p-1", f.roomA) // default melee stats
	env, shot, _ := f.shootEnv(a)
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "shoot guard north")

	if shot.called {
		t.Error("ResolveAttack must not fire without a projectile weapon")
	}
	if got := a.lastLine(); got != "You aren't wielding anything you can shoot." {
		t.Errorf("line = %q, want the no-projectile refusal", got)
	}
}

func TestShoot_NoExitThatWay(t *testing.T) {
	f := newShootFixture(t)
	a := f.archer("")
	env, shot, _ := f.shootEnv(a)
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "shoot guard east")

	if shot.called {
		t.Error("ResolveAttack must not fire when there is no exit that way")
	}
	if got := a.lastLine(); got != "There's no way to shoot to the east." {
		t.Errorf("line = %q, want the no-exit refusal", got)
	}
}

func TestShoot_ClosedDoorBlocks(t *testing.T) {
	f := newShootFixture(t)
	// Shut a door on the north exit.
	f.roomA.Exits[world.DirNorth] = world.Exit{Target: "z:b", Door: &world.DoorState{Name: "gate", Closed: true}}
	a := f.archer("")
	env, shot, _ := f.shootEnv(a)
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "shoot guard north")

	if shot.called {
		t.Error("ResolveAttack must not fire through a closed door")
	}
	if got := a.lastLine(); got != "The way north is closed; you can't shoot through it." {
		t.Errorf("line = %q, want the closed-door refusal", got)
	}
}

func TestShoot_TargetNotInAdjacentRoom(t *testing.T) {
	f := newShootFixture(t)
	a := f.archer("")
	env, shot, _ := f.shootEnv(a)
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "shoot dragon north")

	if shot.called {
		t.Error("ResolveAttack must not fire when no such target is in the adjacent room")
	}
	if got := a.lastLine(); got != "You don't see anyone like that to the north." {
		t.Errorf("line = %q, want the no-target refusal", got)
	}
}

func TestShoot_HitsCrossRoomAndStampsTargetRoom(t *testing.T) {
	f := newShootFixture(t)
	a := f.archer("")
	env, shot, rec := f.shootEnv(a)
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "shoot guard north")

	if !shot.called {
		t.Fatal("ResolveAttack was not called on a valid cross-room shot")
	}
	if shot.attacker != a.CombatantID() || shot.target != f.guard.CombatantID() {
		t.Errorf("ResolveAttack ids = (%v,%v), want (%v,%v)", shot.attacker, shot.target, a.CombatantID(), f.guard.CombatantID())
	}
	// The swing must be stamped to the TARGET room so the third-person
	// announce lands where the target is (the two-room render decision).
	if shot.room != f.roomB.ID {
		t.Errorf("ResolveAttack room = %q, want the target room %q", shot.room, f.roomB.ID)
	}
	// Outbound flavor in the shooter's room, inbound flavor in the target's.
	if got, _ := rec.toRoom(f.roomA.ID); !strings.Contains(got, "looses a shot to the north") {
		t.Errorf("shooter-room broadcast = %q, want an outbound 'looses a shot to the north'", got)
	}
	if got, _ := rec.toRoom(f.roomB.ID); !strings.Contains(got, "streaks in from the south") {
		t.Errorf("target-room broadcast = %q, want an inbound 'streaks in from the south'", got)
	}
}

// ammoArcher satisfies the verb's ammoConsumer seam with a finite quiver.
type ammoArcher struct {
	*combatActor
	arrows int
}

func (a *ammoArcher) ConsumeAmmo(kind string) (string, bool) {
	if a.arrows <= 0 {
		return "", false
	}
	a.arrows--
	return "", true
}

func TestShoot_OutOfAmmoDoesNotFire(t *testing.T) {
	f := newShootFixture(t)
	base := f.archer("arrow")
	a := &ammoArcher{combatActor: base, arrows: 0}
	env, shot, _ := f.shootEnv(base) // locator keyed by the embedded combatID
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "shoot guard north")

	if shot.called {
		t.Error("ResolveAttack must not fire when out of ammo")
	}
	// No RangedFlavor registry wired → the neutral engine floor (rangedflavor),
	// which substitutes the ammo kind. The firearm-flavored "*click*" is gone.
	if got := a.lastLine(); got != "You are out of arrow." {
		t.Errorf("line = %q, want the floor out-of-ammo line", got)
	}
}

// With a bow ranged-flavor style wired, the out-of-ammo line reads in the bow's
// voice — proving the weapon's ranged_style routes through the resolver end to
// end (item → combat.Stats → shoot handler → rangedflavor).
func TestShoot_OutOfAmmoUsesRangedStyle(t *testing.T) {
	f := newShootFixture(t)
	base := f.archer("arrow")
	base.stats.RangedStyle = "bow"
	a := &ammoArcher{combatActor: base, arrows: 0}
	env, shot, _ := f.shootEnv(base)
	reg := rangedflavor.NewRegistry()
	reg.Register(rangedflavor.Style{ID: "bow", Msgs: map[string]rangedflavor.Line{
		rangedflavor.KeyDry: {Self: "You reach for another arrow, but you're out."},
	}})
	env.RangedFlavor = reg
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "shoot guard north")

	if shot.called {
		t.Error("ResolveAttack must not fire when out of ammo")
	}
	if got := a.lastLine(); got != "You reach for another arrow, but you're out." {
		t.Errorf("line = %q, want the bow-style out-of-ammo line", got)
	}
}

func TestShoot_LivingMobGetsRetaliationGrudge(t *testing.T) {
	f := newShootFixture(t)
	a := f.archer("")
	env, shot, _ := f.shootEnv(a) // ResolveAttack returns true (target alive)
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "shoot guard north")

	if !shot.called {
		t.Fatal("ResolveAttack should fire on a valid shot")
	}
	// A surviving mob bears a grudge: the shooter's id + the room the shot came
	// from, stamped for the AI retaliation step.
	tgt, _ := f.guard.Property(entities.PropRetaliateTarget)
	if tgt != "p-1" {
		t.Errorf("retaliate target = %v, want the shooter's player id p-1", tgt)
	}
	room, _ := f.guard.Property(entities.PropRetaliateRoom)
	if room != string(f.roomA.ID) {
		t.Errorf("retaliate room = %v, want the shooter's room %q", room, f.roomA.ID)
	}
}

func TestShoot_KilledMobGetsNoGrudge(t *testing.T) {
	f := newShootFixture(t)
	a := f.archer("")
	env, _, _ := f.shootEnv(a)
	// Override ResolveAttack to report the target died (alive=false).
	env.ResolveAttack = func(context.Context, combat.CombatantID, combat.CombatantID, world.RoomID) bool {
		return false
	}
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "shoot guard north")

	if tgt, ok := f.guard.Property(entities.PropRetaliateTarget); ok && tgt != "" {
		t.Errorf("a killed mob must not bear a grudge, got %v", tgt)
	}
}

func TestShoot_ConsumesOneAmmoOnShot(t *testing.T) {
	f := newShootFixture(t)
	base := f.archer("arrow")
	a := &ammoArcher{combatActor: base, arrows: 2}
	env, shot, _ := f.shootEnv(base)
	r := newRegistry(t)
	dispatchActor(t, r, env, a, "shoot guard north")

	if !shot.called {
		t.Fatal("ResolveAttack was not called on a shot with ammo in the quiver")
	}
	if a.arrows != 1 {
		t.Errorf("arrows left = %d, want exactly one spent (1)", a.arrows)
	}
}

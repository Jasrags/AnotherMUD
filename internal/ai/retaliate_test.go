package ai

import (
	"context"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// aggroRecorder captures MobAggro events so a retaliation test can assert the
// mob engaged its shooter through the existing aggro→Engage wiring.
type aggroRecorder struct {
	events []eventbus.MobAggro
}

func newAggroBus() (*eventbus.Bus, *aggroRecorder) {
	bus := eventbus.New()
	rec := &aggroRecorder{}
	bus.Subscribe(eventbus.EventMobAggro, func(_ context.Context, ev eventbus.Event) {
		if e, ok := ev.(eventbus.MobAggro); ok {
			rec.events = append(rec.events, e)
		}
	})
	return bus, rec
}

func setGrudge(m *entities.MobInstance, playerID string, room world.RoomID) {
	m.SetRetaliation(playerID, string(room))
}

// retaliateDeps is f.deps() plus a MobAggro-capturing bus.
func (f *wanderFixture) retaliateDeps() (Deps, *aggroRecorder) {
	bus, rec := newAggroBus()
	deps := f.deps()
	deps.Bus = bus
	return deps, rec
}

func TestRetaliate_NoGrudgeFallsThrough(t *testing.T) {
	f := newWanderFixture(t)
	m := f.spawn(t) // placed in core:a, no grudge
	deps, _ := f.retaliateDeps()

	if tryRetaliate(context.Background(), m, deps) {
		t.Error("a mob with no grudge must fall through to its normal behavior")
	}
}

func TestRetaliate_StepsTowardShooterAndEngages(t *testing.T) {
	f := newWanderFixture(t) // core:a --north--> core:b
	m := f.spawn(t)          // in core:a
	setGrudge(m, "p-1", "core:b")
	deps, rec := f.retaliateDeps()

	if !tryRetaliate(context.Background(), m, deps) {
		t.Fatal("tryRetaliate should report it handled a live grudge")
	}
	// The mob charged into the shooter's room.
	if got, _ := f.place.RoomOf(m.ID()); got != "core:b" {
		t.Errorf("mob room = %q, want it to have charged into core:b", got)
	}
	// Charge broadcasts: depart A, arrive B.
	if len(f.bcast.sent) != 2 {
		t.Fatalf("broadcasts = %d, want 2 (charge out + charge in): %+v", len(f.bcast.sent), f.bcast.sent)
	}
	if f.bcast.sent[0].Room != "core:a" || f.bcast.sent[0].Text != "a guard charges off to the north." {
		t.Errorf("departure charge wrong: %+v", f.bcast.sent[0])
	}
	if f.bcast.sent[1].Room != "core:b" || f.bcast.sent[1].Text != "a guard charges in from the south!" {
		t.Errorf("arrival charge wrong: %+v", f.bcast.sent[1])
	}
	// Engaged once, against the shooter, in the shooter's room.
	if len(rec.events) != 1 {
		t.Fatalf("MobAggro count = %d, want 1: %+v", len(rec.events), rec.events)
	}
	if rec.events[0].PlayerID != "p-1" || rec.events[0].RoomID != "core:b" || rec.events[0].MobID != m.ID() {
		t.Errorf("MobAggro = %+v, want {Mob:%v Player:p-1 Room:core:b}", rec.events[0], m.ID())
	}
	// Grudge settled.
	if hasRetaliation(m) {
		t.Error("grudge should be cleared once the mob engages")
	}
}

func TestRetaliate_AlreadyColocatedEngagesWithoutMoving(t *testing.T) {
	f := newWanderFixture(t)
	m := f.spawn(t)
	f.place.Remove(m.ID())
	f.place.Place(m.ID(), "core:b") // already with the shooter
	setGrudge(m, "p-1", "core:b")
	deps, rec := f.retaliateDeps()

	if !tryRetaliate(context.Background(), m, deps) {
		t.Fatal("tryRetaliate should handle a co-located grudge")
	}
	if got, _ := f.place.RoomOf(m.ID()); got != "core:b" {
		t.Errorf("mob moved (%q) when it was already with the shooter", got)
	}
	if len(f.bcast.sent) != 0 {
		t.Errorf("no charge broadcast expected when already co-located: %+v", f.bcast.sent)
	}
	if len(rec.events) != 1 || rec.events[0].RoomID != "core:b" {
		t.Fatalf("MobAggro = %+v, want one in core:b", rec.events)
	}
	if hasRetaliation(m) {
		t.Error("grudge should be cleared after engaging")
	}
}

func TestRetaliate_ClosedDoorKeepsGrudge(t *testing.T) {
	f := newWanderFixture(t)
	// Slam a door shut on the A->north exit, between the mob and its shooter.
	ra, err := f.world.Room("core:a")
	if err != nil {
		t.Fatalf("Room(core:a): %v", err)
	}
	ra.Exits[world.DirNorth] = world.Exit{Target: "core:b", Door: &world.DoorState{Name: "gate", Closed: true}}

	m := f.spawn(t) // core:a
	setGrudge(m, "p-1", "core:b")
	deps, rec := f.retaliateDeps()

	if !tryRetaliate(context.Background(), m, deps) {
		t.Fatal("a blocked-but-live grudge is still 'handled' (skip normal behavior)")
	}
	if got, _ := f.place.RoomOf(m.ID()); got != "core:a" {
		t.Errorf("mob moved through a closed door: now in %q", got)
	}
	if len(rec.events) != 0 {
		t.Errorf("no engage expected behind a closed door: %+v", rec.events)
	}
	if !hasRetaliation(m) {
		t.Error("grudge must persist behind a closed door so the mob retries until timeout")
	}
}

func TestRetaliate_NotAdjacentDropsGrudge(t *testing.T) {
	f := newWanderFixture(t)
	m := f.spawn(t) // core:a, whose only exit leads to core:b
	setGrudge(m, "p-1", "core:z")
	deps, rec := f.retaliateDeps()

	if !tryRetaliate(context.Background(), m, deps) {
		t.Fatal("an unreachable grudge is still handled this tick")
	}
	if got, _ := f.place.RoomOf(m.ID()); got != "core:a" {
		t.Errorf("mob moved toward an unreachable room: now in %q", got)
	}
	if len(rec.events) != 0 {
		t.Errorf("no engage expected for an unreachable shooter: %+v", rec.events)
	}
	if hasRetaliation(m) {
		t.Error("an unreachable grudge (not adjacent) should be dropped; multi-room pursuit is deferred")
	}
}

func TestRetaliate_ExpiryDropsGrudge(t *testing.T) {
	f := newWanderFixture(t)
	f.clk.Advance(100 * time.Second) // now = 100s
	m := f.spawn(t)                  // core:a, shooter adjacent in core:b
	setGrudge(m, "p-1", "core:b")
	// Pre-arm an expiry already in the past (50s < now 100s).
	m.SetProperty(propRetaliateExpireAt, int64(50*time.Second))
	deps, rec := f.retaliateDeps()

	if !tryRetaliate(context.Background(), m, deps) {
		t.Fatal("an expired grudge is still handled (and cleared) this tick")
	}
	if got, _ := f.place.RoomOf(m.ID()); got != "core:a" {
		t.Errorf("mob pursued on an expired grudge: now in %q", got)
	}
	if len(rec.events) != 0 {
		t.Errorf("no engage expected on an expired grudge: %+v", rec.events)
	}
	if hasRetaliation(m) {
		t.Error("an expired grudge should be dropped")
	}
}

func TestDispatcher_RetaliationPreemptsBehavior(t *testing.T) {
	f := newWanderFixture(t)
	m := f.spawn(t) // behavior=wander, core:a
	setGrudge(m, "p-1", "core:b")
	deps, rec := f.retaliateDeps()

	reg := NewRegistry()
	if err := reg.Register(BehaviorNameWander, BehaviorWander); err != nil {
		t.Fatalf("Register: %v", err)
	}
	disp := NewDispatcher(reg, deps)
	f.store.SwapTagIndex()
	disp.Tick(context.Background(), 1)

	// Retaliation ran (it engages); wander never publishes MobAggro, so its
	// presence proves the retaliation path preempted the normal behavior.
	if len(rec.events) != 1 {
		t.Fatalf("MobAggro count = %d, want 1 (retaliation, not wander): %+v", len(rec.events), rec.events)
	}
	if got, _ := f.place.RoomOf(m.ID()); got != "core:b" {
		t.Errorf("mob room = %q, want core:b (charged at the shooter)", got)
	}
	if hasRetaliation(m) {
		t.Error("grudge should be cleared after the retaliation engaged")
	}
}

func TestDispatcher_InCombatClearsGrudge(t *testing.T) {
	f := newWanderFixture(t)
	m := f.spawn(t)
	setGrudge(m, "p-1", "core:b")
	deps, rec := f.retaliateDeps()
	deps.Combat = &fakeCombatGate{inCombat: true}

	reg := NewRegistry()
	if err := reg.Register(BehaviorNameWander, BehaviorWander); err != nil {
		t.Fatalf("Register: %v", err)
	}
	disp := NewDispatcher(reg, deps)
	f.store.SwapTagIndex()
	disp.Tick(context.Background(), 1)

	// A mob already fighting has its grudge settled by the round loop — the
	// lingering intent is cleared so it doesn't re-pursue after combat ends.
	if hasRetaliation(m) {
		t.Error("an in-combat mob's grudge should be cleared")
	}
	if got, _ := f.place.RoomOf(m.ID()); got != "core:a" {
		t.Errorf("in-combat mob moved (%q); the combat gate must skip all AI", got)
	}
	if len(rec.events) != 0 {
		t.Errorf("no retaliation engage expected for an in-combat mob: %+v", rec.events)
	}
}

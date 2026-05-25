package ai

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ----- Registry tests -----

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	called := false
	fn := func(context.Context, *entities.MobInstance, Deps) error {
		called = true
		return nil
	}
	if err := r.Register("idle", fn); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.Get("idle")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = got(context.Background(), nil, Deps{})
	if !called {
		t.Error("retrieved behavior was not the one we registered")
	}
}

func TestRegistry_DuplicateRejected(t *testing.T) {
	r := NewRegistry()
	fn := func(context.Context, *entities.MobInstance, Deps) error { return nil }
	if err := r.Register("idle", fn); err != nil {
		t.Fatalf("first: %v", err)
	}
	err := r.Register("idle", fn)
	if !errors.Is(err, ErrDuplicateBehavior) {
		t.Errorf("err = %v, want ErrDuplicateBehavior", err)
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	_, err := NewRegistry().Get("ghost")
	if !errors.Is(err, ErrUnknownBehavior) {
		t.Errorf("err = %v, want ErrUnknownBehavior", err)
	}
}

func TestRegisterEngineBaseline_RegistersBuiltIns(t *testing.T) {
	r := NewRegistry()
	if err := RegisterEngineBaseline(r); err != nil {
		t.Fatalf("RegisterEngineBaseline: %v", err)
	}
	if !r.Has(BehaviorNameStationary) {
		t.Errorf("missing %q baseline", BehaviorNameStationary)
	}
	if !r.Has(BehaviorNameWander) {
		t.Errorf("missing %q baseline", BehaviorNameWander)
	}
}

// ----- Dispatcher tests -----

func TestDispatcher_InvokesRegisteredBehaviorForEveryMob(t *testing.T) {
	store := entities.NewStore()
	_, _ = store.SpawnMob(&mob.Template{
		ID: "tapestry-core:a", Name: "a", Type: "npc", Behavior: "track",
	})
	_, _ = store.SpawnMob(&mob.Template{
		ID: "tapestry-core:b", Name: "b", Type: "npc", Behavior: "track",
	})
	store.SwapTagIndex()

	var count int
	var mu sync.Mutex
	reg := NewRegistry()
	_ = reg.Register("track", func(_ context.Context, _ *entities.MobInstance, _ Deps) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})

	d := NewDispatcher(reg, Deps{Store: store})
	d.Tick(context.Background(), 1)

	if count != 2 {
		t.Errorf("behavior invocation count = %d, want 2", count)
	}
}

// TestDispatcher_DispatchesEachMobToItsOwnBehavior pins the
// per-mob lookup: mob A's PropBehavior selects handler A, mob B's
// PropBehavior selects handler B. Catches dispatch-table bugs
// where a single shared handler runs for every mob.
func TestDispatcher_DispatchesEachMobToItsOwnBehavior(t *testing.T) {
	store := entities.NewStore()
	_, _ = store.SpawnMob(&mob.Template{
		ID: "core:a", Name: "a", Type: "npc", Behavior: "alpha",
	})
	_, _ = store.SpawnMob(&mob.Template{
		ID: "core:b", Name: "b", Type: "npc", Behavior: "beta",
	})
	store.SwapTagIndex()

	var alphaCalls, betaCalls int
	reg := NewRegistry()
	_ = reg.Register("alpha", func(context.Context, *entities.MobInstance, Deps) error {
		alphaCalls++
		return nil
	})
	_ = reg.Register("beta", func(context.Context, *entities.MobInstance, Deps) error {
		betaCalls++
		return nil
	})

	d := NewDispatcher(reg, Deps{Store: store})
	d.Tick(context.Background(), 1)
	if alphaCalls != 1 || betaCalls != 1 {
		t.Errorf("alpha=%d beta=%d, want 1/1 (each mob to its own behavior)", alphaCalls, betaCalls)
	}
}

func TestDispatcher_UnknownBehaviorIsLoggedNotFatal(t *testing.T) {
	store := entities.NewStore()
	_, _ = store.SpawnMob(&mob.Template{
		ID: "tapestry-core:lost", Name: "lost", Type: "npc", Behavior: "no-such",
	})
	_, _ = store.SpawnMob(&mob.Template{
		ID: "tapestry-core:ok", Name: "ok", Type: "npc", Behavior: "track",
	})
	store.SwapTagIndex()

	called := false
	reg := NewRegistry()
	_ = reg.Register("track", func(context.Context, *entities.MobInstance, Deps) error {
		called = true
		return nil
	})

	d := NewDispatcher(reg, Deps{Store: store})
	d.Tick(context.Background(), 1)
	if !called {
		t.Error("unknown-behavior mob blocked the known-behavior mob")
	}
}

func TestDispatcher_BehaviorErrorDoesNotStopOthers(t *testing.T) {
	store := entities.NewStore()
	_, _ = store.SpawnMob(&mob.Template{
		ID: "tapestry-core:boom", Name: "boom", Type: "npc", Behavior: "fail",
	})
	_, _ = store.SpawnMob(&mob.Template{
		ID: "tapestry-core:after", Name: "after", Type: "npc", Behavior: "fail",
	})
	store.SwapTagIndex()

	var count int
	reg := NewRegistry()
	_ = reg.Register("fail", func(context.Context, *entities.MobInstance, Deps) error {
		count++
		return errors.New("boom")
	})

	d := NewDispatcher(reg, Deps{Store: store})
	d.Tick(context.Background(), 1)
	if count != 2 {
		t.Errorf("invocation count = %d, want 2 (first error shouldn't abort loop)", count)
	}
}

func TestDispatcher_SkipsMobsWithEmptyBehavior(t *testing.T) {
	// A mob with PropBehavior = "" (or missing) shouldn't be sent to
	// the registry. Defensive against malformed templates.
	store := entities.NewStore()
	inst, _ := store.SpawnMob(&mob.Template{
		ID: "tapestry-core:nameless", Name: "n", Type: "npc", Behavior: "x",
	})
	// Clear the behavior property after spawn to simulate the
	// corrupted state.
	inst.Properties()[entities.PropBehavior] = ""
	store.SwapTagIndex()

	called := false
	reg := NewRegistry()
	_ = reg.Register("x", func(context.Context, *entities.MobInstance, Deps) error {
		called = true
		return nil
	})

	d := NewDispatcher(reg, Deps{Store: store})
	d.Tick(context.Background(), 1)
	if called {
		t.Error("empty PropBehavior should not dispatch to any handler")
	}
}

// ----- Stationary -----

func TestBehaviorStationary_IsNoOp(t *testing.T) {
	if err := BehaviorStationary(context.Background(), nil, Deps{}); err != nil {
		t.Errorf("stationary returned err = %v, want nil", err)
	}
}

// ----- Wander -----

// wanderFixture builds the world + store + placement + clock + rand
// needed to drive BehaviorWander deterministically.
type wanderFixture struct {
	world *world.World
	store *entities.Store
	place *entities.Placement
	clk   *clock.ManualClock
	bcast *fakeBroadcaster
}

func newWanderFixture(t *testing.T) *wanderFixture {
	t.Helper()
	w := world.New()
	a := &world.Room{ID: "core:a", Name: "A", Description: "x",
		Exits: map[world.Direction]world.Exit{world.DirNorth: {Target: "core:b"}}}
	b := &world.Room{ID: "core:b", Name: "B", Description: "x",
		Exits: map[world.Direction]world.Exit{world.DirSouth: {Target: "core:a"}}}
	w.AddRoom(a)
	w.AddRoom(b)
	clk := clock.NewManual(time.Unix(0, 0))
	return &wanderFixture{
		world: w,
		store: entities.NewStore(),
		place: entities.NewPlacement(),
		clk:   clk,
		bcast: &fakeBroadcaster{},
	}
}

type fakeBroadcaster struct {
	sent []sentMessage
}

type sentMessage struct {
	Room world.RoomID
	Text string
}

func (b *fakeBroadcaster) SendToRoom(_ context.Context, r world.RoomID, text string, _ ...string) {
	b.sent = append(b.sent, sentMessage{Room: r, Text: text})
}

func (f *wanderFixture) spawn(t *testing.T) *entities.MobInstance {
	t.Helper()
	m, err := f.store.SpawnMob(&mob.Template{
		ID: "core:guard", Name: "a guard", Type: "npc", Behavior: BehaviorNameWander,
	})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	f.place.Place(m.ID(), "core:a")
	return m
}

func (f *wanderFixture) deps() Deps {
	return Deps{
		World:       f.world,
		Placement:   f.place,
		Store:       f.store,
		Broadcaster: f.bcast,
		Clock:       f.clk,
		// Deterministic Rand: PCG with stable seed so IntN(1) → 0.
		Rand: rand.New(rand.NewPCG(1, 2)),
	}
}

func TestBehaviorWander_MovesMobAndBroadcasts(t *testing.T) {
	f := newWanderFixture(t)
	m := f.spawn(t)

	if err := BehaviorWander(context.Background(), m, f.deps()); err != nil {
		t.Fatalf("wander: %v", err)
	}

	// Placement updated: mob now in B.
	if got, _ := f.place.RoomOf(m.ID()); got != "core:b" {
		t.Errorf("mob moved to %q, want core:b", got)
	}
	// Two broadcasts: departure from A, arrival in B.
	if len(f.bcast.sent) != 2 {
		t.Fatalf("broadcasts = %d, want 2: %+v", len(f.bcast.sent), f.bcast.sent)
	}
	if f.bcast.sent[0].Room != "core:a" || f.bcast.sent[0].Text != "a guard heads north." {
		t.Errorf("departure broadcast wrong: %+v", f.bcast.sent[0])
	}
	if f.bcast.sent[1].Room != "core:b" || f.bcast.sent[1].Text != "a guard arrives from the south." {
		t.Errorf("arrival broadcast wrong: %+v", f.bcast.sent[1])
	}
}

func TestBehaviorWander_GatesByInterval(t *testing.T) {
	// First call moves; second call within the interval no-ops.
	f := newWanderFixture(t)
	m := f.spawn(t)

	if err := BehaviorWander(context.Background(), m, f.deps()); err != nil {
		t.Fatalf("first wander: %v", err)
	}
	beforeRoom, _ := f.place.RoomOf(m.ID())

	// Advance the clock by less than the interval; mob must not move.
	f.clk.Advance(DefaultWanderInterval / 2)
	if err := BehaviorWander(context.Background(), m, f.deps()); err != nil {
		t.Fatalf("second wander: %v", err)
	}
	afterRoom, _ := f.place.RoomOf(m.ID())
	if afterRoom != beforeRoom {
		t.Errorf("mob moved within interval window (%q → %q)", beforeRoom, afterRoom)
	}
}

func TestBehaviorWander_AdvancesAfterIntervalElapsed(t *testing.T) {
	f := newWanderFixture(t)
	m := f.spawn(t)

	if err := BehaviorWander(context.Background(), m, f.deps()); err != nil {
		t.Fatalf("first wander: %v", err)
	}
	firstRoom, _ := f.place.RoomOf(m.ID())

	// Advance past the interval; mob is eligible to move again.
	f.clk.Advance(DefaultWanderInterval + time.Second)
	if err := BehaviorWander(context.Background(), m, f.deps()); err != nil {
		t.Fatalf("second wander: %v", err)
	}
	secondRoom, _ := f.place.RoomOf(m.ID())
	// Each test room has one exit going back to the other, so
	// successive wanders ping-pong between A and B.
	if secondRoom == firstRoom {
		t.Errorf("mob did not advance after interval elapsed: still %q", secondRoom)
	}
}

func TestBehaviorWander_NoExitsLeavesMobInPlace(t *testing.T) {
	w := world.New()
	w.AddRoom(&world.Room{ID: "core:island", Name: "Island", Description: "alone"})
	clk := clock.NewManual(time.Unix(0, 0))
	store := entities.NewStore()
	place := entities.NewPlacement()
	m, _ := store.SpawnMob(&mob.Template{ID: "core:m", Name: "m", Type: "npc", Behavior: BehaviorNameWander})
	place.Place(m.ID(), "core:island")

	deps := Deps{World: w, Placement: place, Store: store, Clock: clk}
	if err := BehaviorWander(context.Background(), m, deps); err != nil {
		t.Fatalf("wander: %v", err)
	}
	if got, _ := place.RoomOf(m.ID()); got != "core:island" {
		t.Errorf("mob moved from exit-less room to %q", got)
	}
}

func TestBehaviorWander_NilDepsReturnError(t *testing.T) {
	if err := BehaviorWander(context.Background(), nil, Deps{}); err == nil {
		t.Error("wander with empty Deps should return programmer-error err")
	}
}

func TestBehaviorWander_NilBroadcasterTolerated(t *testing.T) {
	// A real headless / test setup may not wire a Broadcaster. Move
	// must still happen; announce just silently no-ops.
	f := newWanderFixture(t)
	m := f.spawn(t)
	deps := f.deps()
	deps.Broadcaster = nil
	if err := BehaviorWander(context.Background(), m, deps); err != nil {
		t.Fatalf("wander: %v", err)
	}
	if got, _ := f.place.RoomOf(m.ID()); got != "core:b" {
		t.Errorf("mob did not move under nil broadcaster: at %q", got)
	}
}

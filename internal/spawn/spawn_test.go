package spawn

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ---- test doubles ----

// stubSpawner records every Spawn call and returns synthetic ids.
// Optional `fail` controls whether to surface an error.
type stubSpawner struct {
	mu     sync.Mutex
	calls  []spawnCall
	nextID int
	fail   error
}

type spawnCall struct {
	template string
	room     world.RoomID
	id       entities.EntityID
}

func (s *stubSpawner) Spawn(_ context.Context, templateID string, roomID world.RoomID) (entities.EntityID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return "", s.fail
	}
	s.nextID++
	id := entities.EntityID(toEntID(s.nextID))
	s.calls = append(s.calls, spawnCall{template: templateID, room: roomID, id: id})
	return id, nil
}

func (s *stubSpawner) callsCopy() []spawnCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]spawnCall, len(s.calls))
	copy(out, s.calls)
	return out
}

func toEntID(n int) string {
	// Avoid importing strconv just for tests; small range.
	digits := []byte{}
	if n == 0 {
		return "e0"
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return "e" + string(digits)
}

// fakeStore is a set of "alive" entity ids. Used by Tracker.Purge.
type fakeStore struct {
	alive map[entities.EntityID]struct{}
}

func newFakeStore(ids ...entities.EntityID) *fakeStore {
	s := &fakeStore{alive: make(map[entities.EntityID]struct{})}
	for _, id := range ids {
		s.alive[id] = struct{}{}
	}
	return s
}

func (s *fakeStore) GetByID(id entities.EntityID) (entities.Entity, bool) {
	_, ok := s.alive[id]
	return nil, ok
}

func (s *fakeStore) kill(id entities.EntityID) { delete(s.alive, id) }

// worldWith builds a one-area world with rooms named r1..rN. Spawn
// rules are supplied by tests; not all rules need exist as rooms
// (callers use this to drive both happy paths and missing-room).
func worldWith(t *testing.T, areaID world.AreaID, roomIDs ...world.RoomID) *world.World {
	t.Helper()
	w := world.New()
	w.AddArea(&world.Area{ID: areaID})
	for _, id := range roomIDs {
		w.AddRoom(&world.Room{ID: id, AreaID: areaID})
	}
	return w
}

// ---- Tracker ----

func TestTracker_TrackAndCount(t *testing.T) {
	tr := NewTracker()
	tr.Track("a", 0, "e1")
	tr.Track("a", 0, "e2")
	tr.Track("a", 1, "e3")
	if got := tr.Count("a", 0); got != 2 {
		t.Errorf("Count(a,0) = %d, want 2", got)
	}
	if got := tr.Count("a", 1); got != 1 {
		t.Errorf("Count(a,1) = %d, want 1", got)
	}
	if got := tr.Count("b", 0); got != 0 {
		t.Errorf("Count(b,0) = %d, want 0", got)
	}
}

func TestTracker_PurgeRemovesDead(t *testing.T) {
	tr := NewTracker()
	tr.Track("a", 0, "alive1")
	tr.Track("a", 0, "dead")
	tr.Track("a", 0, "alive2")
	alive := func(id entities.EntityID) bool { return id != "dead" }
	purged := tr.Purge("a", 0, alive)
	if purged != 1 {
		t.Errorf("Purge returned %d, want 1", purged)
	}
	if got := tr.Count("a", 0); got != 2 {
		t.Errorf("Count after purge = %d, want 2", got)
	}
	snap := tr.Snapshot("a", 0)
	for _, id := range snap {
		if id == "dead" {
			t.Errorf("snapshot still contains 'dead': %v", snap)
		}
	}
}

// ---- Manager reset algorithm ----

func TestManager_FirstResetBringsUpFromZero(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:r1", MobTemplateID: "core:guard", Count: 3},
	}
	sp := &stubSpawner{}
	mgr := NewManager(Config{
		World: w, Tracker: NewTracker(), Spawner: sp, Bus: eventbus.New(),
	})

	mgr.Reset(context.Background(), "town")

	if got := len(sp.callsCopy()); got != 3 {
		t.Errorf("spawn calls = %d, want 3", got)
	}
}

func TestManager_ResetIsTopUpOnly(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:r1", MobTemplateID: "core:guard", Count: 3},
	}
	sp := &stubSpawner{}
	mgr := NewManager(Config{
		World: w, Tracker: NewTracker(), Spawner: sp, Bus: eventbus.New(),
	})

	mgr.Reset(context.Background(), "town")
	mgr.Reset(context.Background(), "town")

	if got := len(sp.callsCopy()); got != 3 {
		t.Errorf("spawn calls after two resets = %d, want 3 (no double-spawn)", got)
	}
}

func TestManager_PersistentSkipsWhenAtTarget(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:r1", MobTemplateID: "core:guard", Count: 2, Tags: []string{TagPersistent}},
	}
	sp := &stubSpawner{}
	tr := NewTracker()
	mgr := NewManager(Config{
		World: w, Tracker: tr, Spawner: sp, Bus: eventbus.New(),
	})

	mgr.Reset(context.Background(), "town")
	if got := tr.Count("town", 0); got != 2 {
		t.Fatalf("after first reset: count=%d want 2", got)
	}

	// Force an over-target tracker state and re-reset.
	tr.Track("town", 0, "manual-extra")
	mgr.Reset(context.Background(), "town")
	if got := len(sp.callsCopy()); got != 2 {
		t.Errorf("persistent rule re-spawned: total calls=%d", got)
	}
}

func TestManager_PurgeDeadBeforeCounting(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:r1", MobTemplateID: "core:guard", Count: 2},
	}
	store := newFakeStore()
	sp := &stubSpawner{}
	tr := NewTracker()
	mgr := NewManager(Config{
		World: w, Tracker: tr, Spawner: sp, Store: store, Bus: eventbus.New(),
	})

	// Fake an existing population that's "alive" in the store.
	tr.Track("town", 0, "e10")
	tr.Track("town", 0, "e11")
	store.alive["e10"] = struct{}{}
	store.alive["e11"] = struct{}{}

	// Both alive → no spawn needed.
	mgr.Reset(context.Background(), "town")
	if got := len(sp.callsCopy()); got != 0 {
		t.Errorf("expected no spawns when full; got %d", got)
	}

	// Kill one; next reset should top up by one.
	store.kill("e10")
	mgr.Reset(context.Background(), "town")
	calls := sp.callsCopy()
	if len(calls) != 1 {
		t.Errorf("expected one top-up spawn; got %d", len(calls))
	}
}

func TestManager_RareSwapHonorsChance(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{
			RoomID: "core:r1", MobTemplateID: "core:guard",
			Count: 100, Rare: "core:captain", RareChance: 0.3,
		},
	}
	sp := &stubSpawner{}
	// Deterministic RNG: PCG with stable seed.
	rng := rand.New(rand.NewPCG(7, 11))
	mgr := NewManager(Config{
		World: w, Tracker: NewTracker(), Spawner: sp, Bus: eventbus.New(), Rng: rng,
	})

	mgr.Reset(context.Background(), "town")
	calls := sp.callsCopy()
	if len(calls) != 100 {
		t.Fatalf("calls = %d, want 100", len(calls))
	}
	rares := 0
	for _, c := range calls {
		if c.template == "core:captain" {
			rares++
		}
	}
	// Deterministic under the fixed PCG seed, so this test is NOT
	// flaky. The 30% × 100 = ~30 expectation is checked with a
	// wide [15,45] band because the test's job is to catch
	// algorithm regressions (e.g. rare-swap fires always or never),
	// not to pin the exact RNG draws — a stricter bound would force
	// every seed-source change to update this number for no gain.
	if rares < 15 || rares > 45 {
		t.Errorf("rare count = %d out of 100, want roughly 30", rares)
	}
}

func TestManager_RareWithZeroChanceNeverFires(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:r1", MobTemplateID: "core:guard", Count: 10, Rare: "core:rare", RareChance: 0},
	}
	sp := &stubSpawner{}
	mgr := NewManager(Config{
		World: w, Tracker: NewTracker(), Spawner: sp, Bus: eventbus.New(),
	})
	mgr.Reset(context.Background(), "town")
	for _, c := range sp.callsCopy() {
		if c.template == "core:rare" {
			t.Errorf("rare fired with chance=0: %v", sp.callsCopy())
		}
	}
}

func TestManager_SpawnFailureSkipsTrackingAndContinues(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:r1", MobTemplateID: "core:bad", Count: 2},
	}
	tr := NewTracker()
	sp := &stubSpawner{fail: errors.New("template missing")}
	mgr := NewManager(Config{
		World: w, Tracker: tr, Spawner: sp, Bus: eventbus.New(),
	})
	mgr.Reset(context.Background(), "town")
	if got := tr.Count("town", 0); got != 0 {
		t.Errorf("failed spawn was still tracked: count=%d", got)
	}
}

func TestManager_MissingRoomSkipsRule(t *testing.T) {
	w := worldWith(t, "town" /* no rooms */)
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:ghost", MobTemplateID: "core:guard", Count: 1},
	}
	sp := &stubSpawner{}
	mgr := NewManager(Config{
		World: w, Tracker: NewTracker(), Spawner: sp, Bus: eventbus.New(),
	})
	mgr.Reset(context.Background(), "town")
	if got := len(sp.callsCopy()); got != 0 {
		t.Errorf("missing room produced %d spawns, want 0", got)
	}
}

func TestManager_BusSubscriptionFiresReset(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:r1", MobTemplateID: "core:guard", Count: 1},
	}
	bus := eventbus.New()
	sp := &stubSpawner{}
	NewManager(Config{
		World: w, Tracker: NewTracker(), Spawner: sp, Bus: bus,
	})
	bus.Publish(context.Background(), eventbus.AreaTick{AreaID: "town"})
	if got := len(sp.callsCopy()); got != 1 {
		t.Errorf("bus-driven reset spawned %d, want 1", got)
	}
}

// ---- Scheduler ----

type stubPresence map[world.AreaID]int

func (p stubPresence) PlayerCountInArea(id world.AreaID) int { return p[id] }

func TestScheduler_FiresOnceCadenceElapses(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:r1", MobTemplateID: "core:guard", Count: 1},
	}
	area.ResetInterval = 3 // fire every 3 steps

	bus := eventbus.New()
	var fires int
	bus.Subscribe(eventbus.EventAreaTick, func(_ context.Context, _ eventbus.Event) {
		fires++
	})

	sched := NewScheduler(SchedulerConfig{
		World: w, Bus: bus, DefaultReset: 99, OccupiedModifier: 1.0,
	})
	for i := 0; i < 9; i++ {
		sched.Step(context.Background(), 1)
	}
	if fires != 3 {
		t.Errorf("fires = %d after 9 steps with cadence 3; want 3", fires)
	}
}

func TestScheduler_OccupiedModifierSlowsCadenceWhenPlayersPresent(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:r1", MobTemplateID: "core:guard", Count: 1},
	}
	area.ResetInterval = 2 // base cadence

	bus := eventbus.New()
	var fires int
	bus.Subscribe(eventbus.EventAreaTick, func(_ context.Context, _ eventbus.Event) {
		fires++
	})

	// Modifier 3.0 with occupants → effective cadence 6.
	sched := NewScheduler(SchedulerConfig{
		World: w, Bus: bus, DefaultReset: 99,
		OccupiedModifier: 3.0,
		Presence:         stubPresence{"town": 1},
	})
	for i := 0; i < 6; i++ {
		sched.Step(context.Background(), 1)
	}
	if fires != 1 {
		t.Errorf("fires = %d after 6 steps (cadence 2*3=6); want 1", fires)
	}
}

func TestScheduler_PerAreaModifierOverridesGlobal(t *testing.T) {
	w := worldWith(t, "town", "core:r1")
	area, _ := w.Area("town")
	area.SpawnRules = []world.SpawnRule{
		{RoomID: "core:r1", MobTemplateID: "core:guard", Count: 1},
	}
	area.ResetInterval = 2

	bus := eventbus.New()
	var fires int
	bus.Subscribe(eventbus.EventAreaTick, func(_ context.Context, _ eventbus.Event) {
		fires++
	})

	sched := NewScheduler(SchedulerConfig{
		World: w, Bus: bus, DefaultReset: 99,
		OccupiedModifier: 10.0,
		Presence:         stubPresence{"town": 1},
	})
	// Per-area override: 0.5 → cadence 2*0.5=1 (clamped via floor).
	sched.SetAreaOccupiedModifier("town", 0.5)

	for i := 0; i < 4; i++ {
		sched.Step(context.Background(), 1)
	}
	if fires != 4 {
		t.Errorf("fires = %d after 4 steps; want 4 (cadence floored to 1)", fires)
	}
}

func TestScheduler_AreasWithoutRulesAreIgnored(t *testing.T) {
	w := worldWith(t, "empty", "core:r1")
	bus := eventbus.New()
	var fires int
	bus.Subscribe(eventbus.EventAreaTick, func(_ context.Context, _ eventbus.Event) { fires++ })
	sched := NewScheduler(SchedulerConfig{World: w, Bus: bus, DefaultReset: 1})
	for i := 0; i < 5; i++ {
		sched.Step(context.Background(), 1)
	}
	if fires != 0 {
		t.Errorf("areas without spawn_rules should not fire; got %d", fires)
	}
}

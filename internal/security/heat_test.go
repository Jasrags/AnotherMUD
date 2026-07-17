package security

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// harness wires a HeatTracker over controllable fake deps and records the
// responders it spawns.
type harness struct {
	t        *HeatTracker
	tier     Tier
	curRoom  world.RoomID
	online   bool
	validSIN bool
	now      uint64 // the scheduling clock (Deps.Now)
	spawns   []spawnCall
}

type spawnCall struct {
	room     world.RoomID
	targetID string
}

func newHarness(cfg Config) *harness {
	h := &harness{tier: TierAA, online: true}
	h.t = New(cfg, Deps{
		TierOf:      func(world.RoomID) Tier { return h.tier },
		PlayerRoom:  func(string) (world.RoomID, bool) { return h.curRoom, h.online },
		HasValidSIN: func(string) bool { return h.validSIN },
		SpawnResponder: func(_ context.Context, room world.RoomID, target string) {
			h.spawns = append(h.spawns, spawnCall{room, target})
		},
		Now: func() uint64 { return h.now },
	})
	return h
}

func enabledCfg() Config {
	return Config{Enabled: true, DecayPerSweep: 5, Policies: DefaultPolicies()}
}

// heatOf / pendingCount are test-only introspection into the tracker's guarded
// state.
func (t *HeatTracker) heatOf(playerID string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.heat[playerID]
}

func (t *HeatTracker) pendingCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.pending)
}

func TestParseTier(t *testing.T) {
	cases := map[string]Tier{"AAA": TierAAA, "aa": TierAA, " A ": TierA, "Z": TierZ, "": TierNone, "x": TierNone}
	for in, want := range cases {
		if got := ParseTier(in); got != want {
			t.Errorf("ParseTier(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestOnKill_UnpolicedNoHeat(t *testing.T) {
	h := newHarness(enabledCfg())
	h.tier = TierZ // barrens — no law
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		h.t.OnCrime(ctx, "p1", "barrens", CrimeMurder)
	}
	if got := h.t.heatOf("p1"); got != 0 {
		t.Fatalf("heat in Z = %d, want 0", got)
	}
	h.t.Sweep(ctx, 100)
	if len(h.spawns) != 0 {
		t.Fatalf("Z zone spawned responders: %v", h.spawns)
	}
}

func TestOnKill_AccruesAndSchedules(t *testing.T) {
	h := newHarness(enabledCfg())
	ctx := context.Background()
	pol := DefaultPolicies()[TierAA] // heat 40, threshold 60

	// One kill (40) is below the AA threshold (60) — no response yet.
	h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
	if h.t.pendingCount() != 0 {
		t.Fatalf("scheduled a response below threshold")
	}
	// A second kill (80 >= 60) crosses it — one response scheduled.
	h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
	if h.t.pendingCount() != 1 {
		t.Fatalf("no response scheduled after crossing threshold")
	}
	// A third crime does not stack a second concurrent response.
	h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
	if h.t.pendingCount() != 1 {
		t.Fatalf("stacked a second concurrent response")
	}
	_ = pol
}

func TestSweep_DecaysHeat(t *testing.T) {
	h := newHarness(enabledCfg()) // decay 5/sweep
	ctx := context.Background()
	h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder) // +40
	h.t.Sweep(ctx, 1)                               // 35
	if got := h.t.heatOf("p1"); got != 35 {
		t.Fatalf("heat after one decay = %d, want 35", got)
	}
	for i := 0; i < 10; i++ {
		h.t.Sweep(ctx, uint64(i+2))
	}
	if got := h.t.heatOf("p1"); got != 0 {
		t.Fatalf("heat did not decay to zero: %d", got)
	}
}

func TestSchedule_DelayMeasuredFromCrimeTick(t *testing.T) {
	h := newHarness(enabledCfg())
	ctx := context.Background()
	pol := DefaultPolicies()[TierAA]

	// The crime happens at tick 1000 — the response must fire at 1000+delay, not
	// relative to the last sweep (which would shorten the delay).
	h.now = 1000
	h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
	h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder) // crosses the AA threshold

	// One tick before the deadline: nothing fires.
	h.t.Sweep(ctx, 1000+pol.DelayTicks-1)
	if len(h.spawns) != 0 {
		t.Fatalf("response fired early: %d spawns", len(h.spawns))
	}
	// At the deadline: it fires.
	h.t.Sweep(ctx, 1000+pol.DelayTicks)
	if len(h.spawns) == 0 {
		t.Fatal("response did not fire at the scheduled tick")
	}
}

func TestFire_ValidSINHuntsCurrentRoom(t *testing.T) {
	h := newHarness(enabledCfg())
	h.validSIN = true
	h.online = true
	h.curRoom = "elsewhere" // the offender fled here
	ctx := context.Background()

	// Cross the AA threshold at the crime scene, then let the delay elapse.
	h.t.OnCrime(ctx, "p1", "crime-scene", CrimeMurder)
	h.t.OnCrime(ctx, "p1", "crime-scene", CrimeMurder)
	pol := DefaultPolicies()[TierAA]
	h.t.Sweep(ctx, pol.DelayTicks+1)

	if len(h.spawns) != pol.Responders {
		t.Fatalf("spawned %d responders, want %d", len(h.spawns), pol.Responders)
	}
	for _, s := range h.spawns {
		if s.room != "elsewhere" {
			t.Errorf("SIN-carrying offender hunted at %q, want current room 'elsewhere'", s.room)
		}
		if s.targetID != "p1" {
			t.Errorf("responder grudge target = %q, want p1", s.targetID)
		}
	}
	// Heat is spent after the response fires.
	if h.t.heatOf("p1") != 0 {
		t.Errorf("heat not reset after response fired")
	}
}

func TestFire_SINlessHuntsCrimeScene(t *testing.T) {
	h := newHarness(enabledCfg())
	h.validSIN = false // off the grid
	h.curRoom = "elsewhere"
	ctx := context.Background()

	h.t.OnCrime(ctx, "p1", "crime-scene", CrimeMurder)
	h.t.OnCrime(ctx, "p1", "crime-scene", CrimeMurder)
	h.t.Sweep(ctx, DefaultPolicies()[TierAA].DelayTicks+1)

	if len(h.spawns) == 0 {
		t.Fatalf("no responders spawned")
	}
	for _, s := range h.spawns {
		if s.room != "crime-scene" {
			t.Errorf("SINless offender hunted at %q, want crime scene", s.room)
		}
	}
}

func TestFire_ValidSINOfflineFallsBackToScene(t *testing.T) {
	h := newHarness(enabledCfg())
	h.validSIN = true
	h.online = false // no live grid fix
	ctx := context.Background()

	h.t.OnCrime(ctx, "p1", "crime-scene", CrimeMurder)
	h.t.OnCrime(ctx, "p1", "crime-scene", CrimeMurder)
	h.t.Sweep(ctx, DefaultPolicies()[TierAA].DelayTicks+1)

	for _, s := range h.spawns {
		if s.room != "crime-scene" {
			t.Errorf("offline offender hunted at %q, want crime-scene fallback", s.room)
		}
	}
}

func TestOnCrime_KindWeighting(t *testing.T) {
	h := newHarness(enabledCfg()) // AA HeatPerCrime 40; weights: violence .25, burn .5
	ctx := context.Background()

	h.t.OnCrime(ctx, "murderer", "downtown", CrimeMurder)
	h.t.OnCrime(ctx, "brawler", "downtown", CrimeViolence)
	h.t.OnCrime(ctx, "forger", "downtown", CrimeBurn)

	if got := h.t.heatOf("murderer"); got != 40 {
		t.Errorf("murder heat = %d, want 40", got)
	}
	if got := h.t.heatOf("brawler"); got != 10 { // 40 * 0.25
		t.Errorf("violence heat = %d, want 10", got)
	}
	if got := h.t.heatOf("forger"); got != 20 { // 40 * 0.5
		t.Errorf("burn heat = %d, want 20", got)
	}
}

func TestEscalation_WavesGrowWithWanted(t *testing.T) {
	h := newHarness(enabledCfg())
	ctx := context.Background()
	pol := DefaultPolicies()[TierAA] // Responders 2

	fireOnce := func() int {
		before := len(h.spawns)
		h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
		h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder) // cross threshold
		h.now += pol.DelayTicks + 1
		h.t.Sweep(ctx, h.now)
		return len(h.spawns) - before
	}

	if w1 := fireOnce(); w1 != pol.Responders {
		t.Fatalf("first wave = %d, want %d (base)", w1, pol.Responders)
	}
	if w2 := fireOnce(); w2 != pol.Responders+1 {
		t.Fatalf("second wave = %d, want %d (escalated)", w2, pol.Responders+1)
	}
	if _, wanted := h.t.Status("p1"); wanted != 2 {
		t.Errorf("wanted level = %d, want 2 after two responses", wanted)
	}
}

func TestEscalation_ResponderCap(t *testing.T) {
	cfg := enabledCfg()
	cfg.ResponderCap = 3
	h := newHarness(cfg)
	ctx := context.Background()
	pol := DefaultPolicies()[TierAA]

	for i := 0; i < 6; i++ {
		before := len(h.spawns)
		h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
		h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
		h.now += pol.DelayTicks + 1
		h.t.Sweep(ctx, h.now)
		if wave := len(h.spawns) - before; wave > cfg.ResponderCap {
			t.Fatalf("wave %d exceeded cap %d", wave, cfg.ResponderCap)
		}
	}
}

func TestClearHeat_DeEscalates(t *testing.T) {
	h := newHarness(enabledCfg())
	ctx := context.Background()
	h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
	h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder) // heat 80, pending scheduled

	cleared := h.t.ClearHeat("p1")
	if cleared != 80 {
		t.Errorf("ClearHeat returned %d, want 80", cleared)
	}
	if heat, _ := h.t.Status("p1"); heat != 0 {
		t.Errorf("heat after clear = %d, want 0", heat)
	}
	if h.t.pendingCount() != 0 {
		t.Errorf("pending response survived a heat clear")
	}
	// A fired response never comes now.
	h.t.Sweep(ctx, 1_000_000)
	if len(h.spawns) != 0 {
		t.Errorf("a cleared offender was still hunted")
	}
}

func TestWanted_FadesWhenCool(t *testing.T) {
	cfg := enabledCfg()
	cfg.WantedDecaySweeps = 1 // decay every sweep for the test
	h := newHarness(cfg)
	ctx := context.Background()
	pol := DefaultPolicies()[TierAA]

	// Provoke one response to reach wanted level 1, then clear heat so the offender
	// is cool (no heat, no pending).
	h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
	h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
	h.now += pol.DelayTicks + 1
	h.t.Sweep(ctx, h.now)
	if _, w := h.t.Status("p1"); w != 1 {
		t.Fatalf("wanted after one wave = %d, want 1", w)
	}
	// Heat was spent on firing; a cool sweep now fades the wanted level.
	h.now++
	h.t.Sweep(ctx, h.now)
	if _, w := h.t.Status("p1"); w != 0 {
		t.Errorf("wanted did not fade when cool: %d", w)
	}
}

func TestSeed_RestoresState(t *testing.T) {
	h := newHarness(enabledCfg())

	h.t.Seed("p1", 45, 2)
	if heat, wanted := h.t.Status("p1"); heat != 45 || wanted != 2 {
		t.Fatalf("Status after seed = (%d,%d), want (45,2)", heat, wanted)
	}
	// Seeding zeros leaves the player clean (a cooled character isn't resurrected).
	h.t.Seed("p1", 0, 0)
	if heat, wanted := h.t.Status("p1"); heat != 0 || wanted != 0 {
		t.Fatalf("Status after zero seed = (%d,%d), want (0,0)", heat, wanted)
	}
}

func TestDisabled_NoOp(t *testing.T) {
	cfg := enabledCfg()
	cfg.Enabled = false
	h := newHarness(cfg)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		h.t.OnCrime(ctx, "p1", "downtown", CrimeMurder)
	}
	h.t.Sweep(ctx, 10_000)
	if h.t.heatOf("p1") != 0 || len(h.spawns) != 0 {
		t.Fatalf("disabled tracker acted: heat=%d spawns=%d", h.t.heatOf("p1"), len(h.spawns))
	}
}

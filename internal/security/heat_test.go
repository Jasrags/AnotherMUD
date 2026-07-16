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
		h.t.OnKill(ctx, "p1", "barrens")
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
	h.t.OnKill(ctx, "p1", "downtown")
	if h.t.pendingCount() != 0 {
		t.Fatalf("scheduled a response below threshold")
	}
	// A second kill (80 >= 60) crosses it — one response scheduled.
	h.t.OnKill(ctx, "p1", "downtown")
	if h.t.pendingCount() != 1 {
		t.Fatalf("no response scheduled after crossing threshold")
	}
	// A third crime does not stack a second concurrent response.
	h.t.OnKill(ctx, "p1", "downtown")
	if h.t.pendingCount() != 1 {
		t.Fatalf("stacked a second concurrent response")
	}
	_ = pol
}

func TestSweep_DecaysHeat(t *testing.T) {
	h := newHarness(enabledCfg()) // decay 5/sweep
	ctx := context.Background()
	h.t.OnKill(ctx, "p1", "downtown") // +40
	h.t.Sweep(ctx, 1)                 // 35
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
	h.t.OnKill(ctx, "p1", "downtown")
	h.t.OnKill(ctx, "p1", "downtown") // crosses the AA threshold

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
	h.t.OnKill(ctx, "p1", "crime-scene")
	h.t.OnKill(ctx, "p1", "crime-scene")
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

	h.t.OnKill(ctx, "p1", "crime-scene")
	h.t.OnKill(ctx, "p1", "crime-scene")
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

	h.t.OnKill(ctx, "p1", "crime-scene")
	h.t.OnKill(ctx, "p1", "crime-scene")
	h.t.Sweep(ctx, DefaultPolicies()[TierAA].DelayTicks+1)

	for _, s := range h.spawns {
		if s.room != "crime-scene" {
			t.Errorf("offline offender hunted at %q, want crime-scene fallback", s.room)
		}
	}
}

func TestDisabled_NoOp(t *testing.T) {
	cfg := enabledCfg()
	cfg.Enabled = false
	h := newHarness(cfg)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		h.t.OnKill(ctx, "p1", "downtown")
	}
	h.t.Sweep(ctx, 10_000)
	if h.t.heatOf("p1") != 0 || len(h.spawns) != 0 {
		t.Fatalf("disabled tracker acted: heat=%d spawns=%d", h.t.heatOf("p1"), len(h.spawns))
	}
}

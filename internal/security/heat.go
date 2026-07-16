package security

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Deps are the injected collaborators (security-response.md §6). Keeping them as
// closures decouples the package from world / entities / session / economy — the
// composition root wires the concrete lookups.
type Deps struct {
	// TierOf resolves the enforcement tier of the room a crime happened in (its
	// area's `security` value). Never nil.
	TierOf func(roomID world.RoomID) Tier
	// PlayerRoom returns the offender's current room and true when they are online
	// and placed — the grid fix used to hunt a SIN-carrying offender (§4). ok=false
	// ⇒ offline/unplaced (fall back to the crime scene).
	PlayerRoom func(playerID string) (world.RoomID, bool)
	// HasValidSIN reports whether the offender carries a valid (unburned) credential
	// (sin-and-legality §3). True ⇒ the law tracks them to their current room;
	// false ⇒ they are off the grid and hunted at the crime scene (§4).
	HasValidSIN func(playerID string) bool
	// SpawnResponder spawns one responder mob into roomID and stamps it with a
	// grudge against targetPlayerID so it pursues+engages that player (§4). Best
	// effort — it logs and returns on any failure (never panics).
	SpawnResponder func(ctx context.Context, roomID world.RoomID, targetPlayerID string)
	// Now returns the current monotonic tick — the scheduling clock. OnKill reads
	// it so a response's fire tick is measured from the crime, not from the last
	// sweep (which would systematically shorten the delay). Wired to
	// tick.Loop.TickCount.
	Now func() uint64
}

// Config tunes the engine (security-response.md §5). Policies defaults to
// DefaultPolicies when nil.
type Config struct {
	// Enabled is the master kill-switch. When false, OnKill / Sweep are no-ops.
	Enabled bool
	// DecayPerSweep is the heat removed from every tracked player each sweep.
	DecayPerSweep int
	// Policies is the per-tier response table.
	Policies map[Tier]Policy
}

// pendingResponse is one scheduled patrol response (one per offender at a time).
type pendingResponse struct {
	scene    world.RoomID // the crime scene — the SINless hunt location (§4)
	tier     Tier
	fireTick uint64 // the tick at/after which the response fires
}

// HeatTracker owns the crime → heat → response loop (security-response.md). It is
// safe for concurrent use: OnKill runs on the crime (death-handler) path and Sweep
// on the tick loop; both take the mutex, and spawning happens outside the lock.
type HeatTracker struct {
	mu      sync.Mutex
	heat    map[string]int             // playerID → accumulated heat
	pending map[string]pendingResponse // playerID → scheduled response (one each)

	cfg  Config
	deps Deps
}

// New builds a HeatTracker. A nil Policies map falls back to DefaultPolicies. When
// the tracker is Enabled, every Deps closure must be wired — New panics on a nil
// one so the misconfiguration fails fast at construction (boot) rather than as a
// nil-func panic on the later crime/tick hot path.
func New(cfg Config, deps Deps) *HeatTracker {
	if cfg.Policies == nil {
		cfg.Policies = DefaultPolicies()
	}
	if cfg.DecayPerSweep <= 0 {
		cfg.DecayPerSweep = 1
	}
	if cfg.Enabled {
		switch {
		case deps.TierOf == nil:
			panic("security.New: Deps.TierOf is nil")
		case deps.PlayerRoom == nil:
			panic("security.New: Deps.PlayerRoom is nil")
		case deps.HasValidSIN == nil:
			panic("security.New: Deps.HasValidSIN is nil")
		case deps.SpawnResponder == nil:
			panic("security.New: Deps.SpawnResponder is nil")
		case deps.Now == nil:
			panic("security.New: Deps.Now is nil")
		}
	}
	return &HeatTracker{
		heat:    map[string]int{},
		pending: map[string]pendingResponse{},
		cfg:     cfg,
		deps:    deps,
	}
}

// OnKill records a player's kill as a crime (security-response.md §2): it adds the
// zone tier's heat and, on crossing the tier threshold, schedules one response. A
// kill in an unpoliced zone (Z / unset) or by no responsible player is a no-op.
func (t *HeatTracker) OnKill(ctx context.Context, playerID string, roomID world.RoomID) {
	if !t.cfg.Enabled || playerID == "" {
		return
	}
	tier := t.deps.TierOf(roomID)
	pol := t.cfg.Policies[tier]
	if pol.HeatPerCrime <= 0 {
		return // unpoliced: crime in the barrens is just Tuesday
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.heat[playerID] += pol.HeatPerCrime
	if _, already := t.pending[playerID]; !already && t.heat[playerID] >= pol.Threshold {
		t.pending[playerID] = pendingResponse{scene: roomID, tier: tier, fireTick: t.deps.Now() + pol.DelayTicks}
		logging.From(ctx).Info("security.heat.threshold",
			slog.String("player_id", playerID),
			slog.String("tier", tier.String()),
			slog.Int("heat", t.heat[playerID]),
			slog.String("room_id", string(roomID)))
	}
}

// Sweep is the recurring scheduler handler (security-response.md §3): it decays
// every tracked player's heat and fires any response whose scheduled tick has
// arrived. Spawning runs outside the lock so a slow spawn never blocks OnKill.
func (t *HeatTracker) Sweep(ctx context.Context, tickCount uint64) {
	if !t.cfg.Enabled {
		return
	}
	type firing struct {
		playerID string
		resp     pendingResponse
	}
	var due []firing

	t.mu.Lock()
	for pid, h := range t.heat {
		if h -= t.cfg.DecayPerSweep; h <= 0 {
			delete(t.heat, pid)
		} else {
			t.heat[pid] = h
		}
	}
	for pid, r := range t.pending {
		if tickCount >= r.fireTick {
			due = append(due, firing{pid, r})
			delete(t.pending, pid)
			delete(t.heat, pid) // heat spent — a fresh spree must re-earn it
		}
	}
	t.mu.Unlock()

	for _, f := range due {
		t.fire(ctx, f.playerID, f.resp)
	}
}

// fire dispatches one patrol response (security-response.md §4). The hunt room is
// the offender's current room when they carry a valid SIN (grid-tracked), else the
// crime scene (SINless / burned — evadable by having moved on).
func (t *HeatTracker) fire(ctx context.Context, playerID string, r pendingResponse) {
	room := r.scene
	tracked := false
	if t.deps.HasValidSIN(playerID) {
		if cur, ok := t.deps.PlayerRoom(playerID); ok {
			room, tracked = cur, true
		}
	}
	pol := t.cfg.Policies[r.tier]
	n := pol.Responders
	if n < 1 {
		n = 1
	}
	logging.From(ctx).Info("security.response.fire",
		slog.String("player_id", playerID),
		slog.String("tier", r.tier.String()),
		slog.String("room_id", string(room)),
		slog.Bool("grid_tracked", tracked),
		slog.Int("responders", n))
	for i := 0; i < n; i++ {
		t.deps.SpawnResponder(ctx, room, playerID)
	}
}

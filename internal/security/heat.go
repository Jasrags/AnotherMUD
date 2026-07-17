package security

import (
	"context"
	"log/slog"
	"math"
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

// CrimeKind classifies a crime for heat weighting (security-response.md §2, v2).
// The tier sets the base heat of a full crime (a murder); lesser crimes scale it.
type CrimeKind int

const (
	// CrimeMurder — killing a lawful / civilian mob: full heat (weight 1).
	CrimeMurder CrimeKind = iota
	// CrimeViolence — killing a hostile mob (a ganger): self-defence-ish, low heat.
	CrimeViolence
	// CrimeBurn — a fake SIN caught/burned at a scan (sin-and-legality §7): petty,
	// but the tightest tie between identity and heat.
	CrimeBurn
)

// Config tunes the engine (security-response.md §5). Policies defaults to
// DefaultPolicies when nil.
type Config struct {
	// Enabled is the master kill-switch. When false, OnCrime / Sweep are no-ops.
	Enabled bool
	// DecayPerSweep is the heat removed from every tracked player each sweep.
	DecayPerSweep int
	// Policies is the per-tier response table.
	Policies map[Tier]Policy
	// ViolenceWeight / BurnWeight scale a tier's base heat for a non-murder crime
	// (murder = 1.0). Defaults (when <= 0): ViolenceWeight 0.25, BurnWeight 0.5.
	ViolenceWeight float64
	BurnWeight     float64
	// ResponderCap bounds a single escalated wave's size (wanted-level growth).
	// Default (when <= 0): 6.
	ResponderCap int
	// WantedDecaySweeps is how many sweeps a cooled offender's wanted level takes
	// to drop by one. Default (when <= 0): 30.
	WantedDecaySweeps int
}

// weightFor returns the tier-base multiplier for a crime kind.
func (c Config) weightFor(kind CrimeKind) float64 {
	switch kind {
	case CrimeViolence:
		return c.ViolenceWeight
	case CrimeBurn:
		return c.BurnWeight
	default: // CrimeMurder
		return 1.0
	}
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
	wanted  map[string]int             // playerID → wanted level (escalation, §7 v2)
	sweepN  uint64                     // sweep counter, drives slow wanted decay

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
	if cfg.ViolenceWeight <= 0 {
		cfg.ViolenceWeight = 0.25
	}
	if cfg.BurnWeight <= 0 {
		cfg.BurnWeight = 0.5
	}
	if cfg.ResponderCap <= 0 {
		cfg.ResponderCap = 6
	}
	if cfg.WantedDecaySweeps <= 0 {
		cfg.WantedDecaySweeps = 30
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
		wanted:  map[string]int{},
		cfg:     cfg,
		deps:    deps,
	}
}

// OnCrime records a player's crime (security-response.md §2, v2): it adds the zone
// tier's heat scaled by the crime kind and, on crossing the tier threshold,
// schedules one response. A crime in an unpoliced zone (Z / unset) or by no
// responsible player is a no-op. The crime kind lets a murder (a lawful/civilian
// victim) count for full heat while a ganger kill (CrimeViolence) or a caught fake
// (CrimeBurn) counts for a fraction.
func (t *HeatTracker) OnCrime(ctx context.Context, playerID string, roomID world.RoomID, kind CrimeKind) {
	if !t.cfg.Enabled || playerID == "" {
		return
	}
	tier := t.deps.TierOf(roomID)
	pol := t.cfg.Policies[tier]
	if pol.HeatPerCrime <= 0 {
		return // unpoliced: crime in the barrens is just Tuesday
	}
	add := int(math.Round(float64(pol.HeatPerCrime) * t.cfg.weightFor(kind)))
	if add <= 0 {
		return // a crime too petty for this tier to notice
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.heat[playerID] += add
	if _, already := t.pending[playerID]; !already && t.heat[playerID] >= pol.Threshold {
		t.pending[playerID] = pendingResponse{scene: roomID, tier: tier, fireTick: t.deps.Now() + pol.DelayTicks}
		logging.From(ctx).Info("security.heat.threshold",
			slog.String("player_id", playerID),
			slog.String("tier", tier.String()),
			slog.Int("heat", t.heat[playerID]),
			slog.Int("wanted", t.wanted[playerID]),
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
	t.sweepN++
	decayWanted := t.sweepN%uint64(t.cfg.WantedDecaySweeps) == 0
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
	// Wanted level fades slowly once an offender has cooled off (no heat, no
	// pending response) — notoriety lingers while you're hot, then decays.
	if decayWanted {
		for pid, w := range t.wanted {
			if _, hot := t.heat[pid]; hot {
				continue
			}
			if _, chased := t.pending[pid]; chased {
				continue
			}
			if w <= 1 {
				delete(t.wanted, pid)
			} else {
				t.wanted[pid] = w - 1
			}
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
	// Escalation (§7 v2): each response an offender provokes raises their wanted
	// level, and the wave grows with it — the second wave is bigger than the first.
	// The bump is taken under the lock; the spawn loop runs unlocked.
	pol := t.cfg.Policies[r.tier]
	t.mu.Lock()
	t.wanted[playerID]++
	wanted := t.wanted[playerID]
	t.mu.Unlock()

	n := pol.Responders + (wanted - 1) // wave 1 = base, wave 2 = base+1, …
	if n < 1 {
		n = 1
	}
	if n > t.cfg.ResponderCap {
		n = t.cfg.ResponderCap
	}
	logging.From(ctx).Info("security.response.fire",
		slog.String("player_id", playerID),
		slog.String("tier", r.tier.String()),
		slog.String("room_id", string(room)),
		slog.Bool("grid_tracked", tracked),
		slog.Int("wanted", wanted),
		slog.Int("responders", n))
	for i := 0; i < n; i++ {
		t.deps.SpawnResponder(ctx, room, playerID)
	}
}

// Status returns an offender's current heat and wanted level (security-response.md
// §7 v2) — read by the `wanted` verb. Zero for an untracked (clean) player.
func (t *HeatTracker) Status(playerID string) (heat, wanted int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.heat[playerID], t.wanted[playerID]
}

// Seed restores a persisted offender's heat + wanted level at login (security-
// response.md §7 v2 persistence). Zero values leave the player untracked (clean),
// so a fresh / cooled character is never resurrected as wanted.
func (t *HeatTracker) Seed(playerID string, heat, wanted int) {
	if !t.cfg.Enabled || playerID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if heat > 0 {
		t.heat[playerID] = heat
	} else {
		delete(t.heat, playerID)
	}
	if wanted > 0 {
		t.wanted[playerID] = wanted
	} else {
		delete(t.wanted, playerID)
	}
}

// ClearHeat wipes an offender's heat and cancels a pending response, and eases
// their wanted level by one — the de-escalation primitive a bribe / lying low
// spends (security-response.md §7 v2). Returns the heat that was cleared (0 when
// already clean) so the caller can price a bribe / report the effect.
func (t *HeatTracker) ClearHeat(playerID string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	had := t.heat[playerID]
	delete(t.heat, playerID)
	delete(t.pending, playerID)
	if w := t.wanted[playerID]; w <= 1 {
		delete(t.wanted, playerID)
	} else {
		t.wanted[playerID] = w - 1
	}
	return had
}

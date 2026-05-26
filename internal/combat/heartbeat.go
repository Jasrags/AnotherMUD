package combat

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Heartbeat is the round-resolution tick handler described in spec
// combat §3. It runs at the combat cadence (configured ticks-per-
// round, not every tick), snapshots the set of combatants once at the
// start of the round, and executes the four phases (ability, auto-
// attack, effects, wimpy) in a fixed priority order over that
// snapshot.
//
// M7.3 scope: the bucket + ordering + snapshot semantics. The phases
// themselves are caller-supplied PhaseFunc fields and are nil in the
// production wiring today — the M7.4 auto-attack swing, M7.5/M7.6
// effects/wimpy, and M9 abilities each fill in one slot when they
// land. A nil phase is silently skipped so the heartbeat can ship
// before any phase logic exists.
//
// Iteration safety (spec §3 "MUST tolerate combatants being removed
// mid-iteration"): each phase iterates a copy of the snapshot taken
// at round start. Per step the heartbeat re-checks Manager.InCombat
// to skip combatants who disengaged or died during an earlier phase
// in this same round. Combatants who Engage mid-round are NOT picked
// up until the next round — the spec calls this out as the desired
// behavior (a fresh join doesn't get a free first-round swing).
//
// Concurrency: Heartbeat is immutable after construction. Tick is
// safe to call from the tick loop goroutine; phase callbacks may
// re-enter Manager freely (lock-order: Manager.mu is strictly inner
// to whatever lock the phase already holds, which is none today).
// The underlying Manager handles its own locking.
//
// Phases are taken at construction (NewHeartbeat) and never mutated
// afterwards — the alternative (exported mutable fields) would race
// with Tick if any future caller wired a phase after the loop went
// live. M7.4-M7.6 + M9 will pass their phase callbacks in through
// NewHeartbeat at boot.
type Heartbeat struct {
	mgr    *Manager
	phases Phases
}

// Phases bundles the four per-round resolution callbacks combat §3
// names. Any subset may be nil — a nil phase is silently skipped, so
// the heartbeat can ship before any phase logic exists.
type Phases struct {
	// Ability runs first (§3.1). At most one queued ability action per
	// combatant resolves per round. Wired by M9 abilities.
	Ability PhaseFunc

	// AutoAttack runs second (§3.2 / §4). Pre-flight disengage on
	// dead/missing/distant target, swing count = 1 + extra-attacks,
	// d20 hit / dice damage. Wired by M7.4.
	AutoAttack PhaseFunc

	// Effects runs third (§3.3). DoT / HoT / ticking debuffs advance
	// one step. Wired by M9 effects.
	Effects PhaseFunc

	// Wimpy runs fourth (§3.4 / §5). HP% threshold check + flee
	// attempt. Wired by M7.6.
	Wimpy PhaseFunc
}

// PhaseFunc is the per-combatant resolution callback for a single
// phase within a round. It receives the live Manager so it can
// query/mutate combat state (PrimaryTargetOf, Engage, Disengage) but
// the round loop has already verified InCombat at the call site, so
// implementations may assume the combatant was still engaged at the
// moment iteration reached them.
//
// Implementations MUST tolerate the combatant being absent from the
// engine's runtime tables — a logged-out player or a despawned mob
// — by falling back to a no-op rather than dereferencing nil. The
// Manager-side cleanup (§4.1 missing-target → disengage) lands in
// the AutoAttack phase in M7.4; until then a nil resolution path is
// the right default.
type PhaseFunc func(ctx context.Context, c CombatantID, mgr *Manager)

// NewHeartbeat constructs a heartbeat that will run phases against
// mgr. The Phases value is captured by copy — the four PhaseFunc
// fields are immutable after this point, so a phase wired in by
// M7.4 (auto-attack) cannot race the tick goroutine. A zero Phases
// is valid (every round is a no-op); each milestone fills in its
// slot.
func NewHeartbeat(mgr *Manager, phases Phases) *Heartbeat {
	return &Heartbeat{mgr: mgr, phases: phases}
}

// Tick is the tick.Handler shape. The tick count is passed for
// logging only — round identity is implicit in the cadence, not in
// the tick count, and is not exposed to phase callbacks (a phase
// that wanted "rounds since engage" would track its own counter).
//
// Tick is safe to call against an empty Manager: AllCombatants
// returns an empty slice and every phase loop is a no-op. The
// production wiring registers Tick at the combat cadence on the tick
// loop; tests drive it directly.
func (h *Heartbeat) Tick(ctx context.Context, tickCount uint64) {
	if h.mgr == nil {
		return
	}
	combatants := h.mgr.AllCombatants()
	if len(combatants) == 0 {
		return
	}
	// Spec §4.1: "Player combatants SHOULD be ordered before mob
	// combatants so that ties (e.g. mutual lethal swings on the same
	// round) resolve in the player's favor." AllCombatants returns
	// map-iteration order, so the snapshot needs a stable partition
	// before any phase runs.
	SortPlayersFirst(combatants)

	log := logging.From(ctx)
	log.Debug("combat round",
		slog.Uint64("tick", tickCount),
		slog.Int("combatants", len(combatants)),
	)

	// Phase order is fixed by §3. The slice exists so a future spec
	// revision that introduces a new phase only touches one literal
	// rather than four call sites.
	phases := []struct {
		name string
		fn   PhaseFunc
	}{
		{"ability", h.phases.Ability},
		{"auto-attack", h.phases.AutoAttack},
		{"effects", h.phases.Effects},
		{"wimpy", h.phases.Wimpy},
	}

	for _, p := range phases {
		if p.fn == nil {
			continue
		}
		h.runPhase(ctx, p.name, p.fn, combatants)
	}
}

// runPhase iterates the round-start snapshot, re-checking liveness
// per combatant so a death/disengage from an earlier phase (or from
// a prior combatant within the SAME phase, e.g. a mutual kill in
// auto-attack) skips the now-departed combatant gracefully.
//
// Panic isolation: a panicking phase callback for one combatant does
// not abort the rest of the iteration. This mirrors tick.Loop's
// safeCall behavior — one bad combatant should not stop the round
// for the others.
func (h *Heartbeat) runPhase(ctx context.Context, name string, fn PhaseFunc, snapshot []CombatantID) {
	for _, c := range snapshot {
		if !h.mgr.InCombat(c) {
			continue
		}
		h.safeCallPhase(ctx, name, c, fn)
	}
}

func (h *Heartbeat) safeCallPhase(ctx context.Context, name string, c CombatantID, fn PhaseFunc) {
	defer func() {
		if rec := recover(); rec != nil {
			logging.From(ctx).Error("combat phase panicked",
				slog.String("phase", name),
				slog.String("combatant", string(c)),
				slog.Any("panic", rec),
			)
		}
	}()
	fn(ctx, c, h.mgr)
}

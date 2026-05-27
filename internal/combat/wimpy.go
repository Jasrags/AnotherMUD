package combat

import "context"

// WimpyHolder is the optional surface a combatant exposes for the
// §5.1 wimpy threshold check. Returns the configured threshold as a
// percent of max HP in [0, 100]; 0 disables wimpy (the default —
// "no wimpy property set" maps to threshold zero, which the §5.1
// check skips). Implementations live outside the combat package
// (connActor for players, MobInstance for mobs); combat consumes the
// interface via a type assertion against the Locator's result.
//
// The accessor is intentionally read-only on this interface so the
// hot tick path doesn't accidentally mutate per-combatant state. A
// separate setter lives on each concrete implementation and is
// called by the `wimpy <pct>` verb in M7.6d, never by the heartbeat.
type WimpyHolder interface {
	WimpyThreshold() int
}

// NewWimpy returns the §5.1 wimpy-check PhaseFunc. Configured with
// the same FleeConfig the verb-driven flee uses; the phase runs
// every round and triggers Flee on combatants whose HP percent has
// dropped to or below their declared threshold.
//
// All cfg fields except CooldownTicks must be non-nil at the call
// site (matching combat.Flee's contract). The phase silently no-ops
// when the cooldown tracker is nil — wimpy without cooldown gating
// would loop a fleer back into combat immediately, which is exactly
// the situation §5.3 was written to prevent.
func NewWimpy(cfg FleeConfig) PhaseFunc {
	if cfg.Mgr == nil {
		panic("combat.NewWimpy: nil Manager")
	}
	if cfg.Locator == nil {
		panic("combat.NewWimpy: nil Locator")
	}
	if cfg.RoomLocator == nil {
		panic("combat.NewWimpy: nil RoomLocator")
	}
	if cfg.Rooms == nil {
		panic("combat.NewWimpy: nil RoomSource")
	}
	if cfg.Mover == nil {
		panic("combat.NewWimpy: nil Mover")
	}

	return func(ctx context.Context, c CombatantID, mgr *Manager) {
		cb, ok := cfg.Locator.LookupCombatant(c)
		if !ok {
			return
		}
		// §5.1 explicitly: skip dead combatants — death handles them,
		// not flee. Defensive even though the heartbeat already
		// re-checks InCombat between phases.
		v := cb.Vitals()
		if v == nil || v.IsDead() {
			return
		}
		holder, ok := cb.(WimpyHolder)
		if !ok {
			return
		}
		threshold := holder.WimpyThreshold()
		if threshold <= 0 {
			return
		}
		// §5.1: compare current HP PERCENT against the threshold,
		// not absolute HP. Percent() returns a [0,1] fraction; scale
		// to [0,100] for the percentage comparison.
		pct := int(v.Percent() * 100)
		if pct > threshold {
			return
		}
		Flee(ctx, c, cfg)
	}
}

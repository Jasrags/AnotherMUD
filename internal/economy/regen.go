package economy

// Regen is the M11.5 vitals-regen heartbeat config (spec
// economy-survival §4.3/§5.5/§5.7 — "the regen-driving feature"). The
// heartbeat heals each living player per tick by a base amount scaled
// by the sustenance multiplier (§4.3) AND the rest multiplier (§5.5),
// then adds the room's healing_rate as a flat bonus (§5.7). This is the
// composition point the sustenance and rest features only expose
// multipliers for; it also pays the M9 "real pools + regen land with
// M11" deferral.
//
// The orchestration lives in session.Manager.RegenTick (it needs the
// actor set + each actor's vitals + room), reading the multipliers off
// the SustenanceService / RestService. This package owns only the
// numeric knobs.

// RegenConfig holds the regen heartbeat parameters.
type RegenConfig struct {
	// BaseHP is the baseline HP healed per regen tick at the full /
	// awake tier (multiplier 1.0). Tiers and rest scale it; the room
	// healing_rate adds to it.
	BaseHP int
	// BaseMana is the baseline mana / One-Power restored per regen tick
	// at the awake tier. Deliberately slow (channeling should not be
	// spammable, per the WoTMUD prior art); scaled by the same sustenance
	// and rest multipliers as HP, but with no room healing_rate term (that
	// is an HP-specific affordance). Zero-max pools (non-channelers) no-op.
	BaseMana int
	// BaseMovement is the baseline movement restored per regen tick at the
	// awake tier, scaled like BaseMana.
	BaseMovement int
	// Cadence is the regen interval in engine ticks. The world-tick
	// subscriber registers at this cadence.
	Cadence uint64
}

// DefaultRegenConfig returns gentle out-of-combat regen: 2 HP every 100
// ticks (10s at the default 100ms tick), so a full+awake player in a
// plain room recovers 2 HP per cycle, a resting one 4, a sleeping one
// 6, and a famished one nothing. Mana regenerates slower (1/cycle — the
// One Power should not refill faster than it is spent). Movement
// regenerates a touch faster than HP (3/cycle) so the higher per-step
// travel cost (moderate-friction tuning) recovers in a short pause rather
// than a long wait — full from empty in ~100s.
func DefaultRegenConfig() RegenConfig {
	return RegenConfig{BaseHP: 2, BaseMana: 1, BaseMovement: 3, Cadence: 100}
}

// RegenAmount composes the per-tick heal (spec §4.3 × §5.5 + §5.7).
// sustMult and restMult are multiplicative; healingRate is an additive
// room bonus. The result is floored at zero — a famished player
// (sustMult 0) regenerates nothing even in a healing room, because the
// multiplicative term zeroes the base and there is no separate room
// term to survive it... except the additive room bonus, which the spec
// frames as a bonus "regen features may apply" to a resting entity. We
// gate the whole heal on a positive multiplicative term so a famished
// player gets nothing regardless of room — hunger trumps the inn.
func RegenAmount(baseHP int, sustMult, restMult float64, healingRate int) int {
	scaled := float64(baseHP) * sustMult * restMult
	if scaled <= 0 {
		return 0
	}
	amount := int(scaled+0.5) + healingRate
	if amount < 0 {
		return 0
	}
	return amount
}

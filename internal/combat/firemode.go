package combat

// Firing modes (ranged-combat §5.5). A projectile's active firing mode trades
// ammunition and accuracy for damage: burst and full-auto put more rounds on the
// target (a damage bonus) at the cost of consuming several rounds per attack and
// suffering an uncompensated recoil to-hit penalty. The penalty is what recoil
// compensation (a later slice) offsets.
//
// The effect magnitudes are host policy (Config.FireModes); the weapon declares
// which modes it *supports* (content), and the attacker picks the active one
// (`firemode`), clamped to the wielded weapon's set. Only projectiles consult
// this — melee/thrown ignore it.

// Firing-mode names — the content contract + the Config.FireModes keys. Mirror
// the item package's vocabulary (combat keeps its own copy to avoid importing
// the item feature, like RangedThrown/RangedProjectile).
const (
	FireModeSingle = "single"
	FireModeBurst  = "burst"
	FireModeAuto   = "auto"
)

// FireModeEffect is what an active firing mode does to a projectile attack.
type FireModeEffect struct {
	// Rounds is how many ammo units the attack consumes (host-side, via the
	// AmmoFor consumer). 1 for single.
	Rounds int
	// DamageBonus is added to the attack's damage (more lead on target).
	DamageBonus int
	// Recoil is the to-hit penalty (a non-negative magnitude) the attacker
	// suffers from the burst's climb — offset later by recoil compensation.
	Recoil int
}

// DefaultFireModes is the documented starting policy: single is free and
// accurate; burst spends 3 rounds for +2 damage at -2 to-hit; full-auto spends 6
// for +4 damage at -4 to-hit. The composition root may override.
func DefaultFireModes() map[string]FireModeEffect {
	return map[string]FireModeEffect{
		FireModeSingle: {Rounds: 1, DamageBonus: 0, Recoil: 0},
		FireModeBurst:  {Rounds: 3, DamageBonus: 2, Recoil: 2},
		FireModeAuto:   {Rounds: 6, DamageBonus: 4, Recoil: 4},
	}
}

// FireModeEffectFor resolves mode against a fire-mode table, falling back to the
// single-fire identity (1 round, no bonus, no recoil) for an empty/unknown mode
// or a nil table. This is the SINGLE source of truth for the fallback — shared by
// combat (FireModeEffectOf) and the host's ammo consumer — so the two never drift
// on how an unknown mode resolves (notably: 1 round, not 0).
func FireModeEffectFor(modes map[string]FireModeEffect, mode string) FireModeEffect {
	if mode == "" {
		mode = FireModeSingle
	}
	if eff, ok := modes[mode]; ok {
		return eff
	}
	return FireModeEffect{Rounds: 1}
}

// FireModeEffectOf resolves the effect for mode from cfg's table (see
// FireModeEffectFor) — so an unconfigured resolver leaves ranged fire unchanged.
func (c AutoAttackConfig) FireModeEffectOf(mode string) FireModeEffect {
	return FireModeEffectFor(c.FireModes, mode)
}

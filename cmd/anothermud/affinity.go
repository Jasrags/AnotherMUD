package main

import "strings"

// WoT S2 Phase 3 — affinities & the Five Powers.
//
// A channeler's strength in each of the Five Powers (Air, Earth, Fire, Water,
// Spirit) is derived from gender — the saidin/saidar split: men are strong in
// Earth/Fire/Spirit, women in Air/Water/Spirit; every other Power is weak. A
// weave (an ability carrying `elements`) is woven at full potency only when
// the channeler is strong in ALL of its Powers — its weakest element governs
// (diversifying into a Power you lack drags the whole weave down). A weak weave
// still casts: this is SOFT scaling, never a hard gate — the
// "specialize-vs-diversify" build identity — but its numeric payload (damage /
// heal) is multiplied by a configurable weak factor < 1.
//
// This is WoT-specific mechanics that cannot be content-authored (the Five
// Powers and the gender split are setting law), so it lives in the composition
// root beside the Phase 2 overchannel / stilling wiring rather than in a
// setting-agnostic engine package. It is inert outside the WoT pack: a
// non-weave ability carries no elements (→ full potency), as does a character
// with no gender. The same potency factor scales both halves of a weave: its
// damage/heal payload here in the ability.used handler, and its effect payload
// (save-gated entry DC, recurring-save DC, and installed stat modifiers) via
// the resolver's injected PotencyProvider (WoT S2 Phase 4) — so bonds-of-air is
// easier to resist and warding's buff is smaller when woven off-affinity.

// maleAffinity / femaleAffinity are the strong-Power sets per gender (the
// saidin / saidar element split). A Power absent from the set is "weak".
var (
	maleAffinity   = map[string]bool{"earth": true, "fire": true, "spirit": true}
	femaleAffinity = map[string]bool{"air": true, "water": true, "spirit": true}
)

// affinityPotency returns the magnitude multiplier for weaving a spell with the
// given elements at the channeler's gender-derived affinity. Returns 1.0 (full)
// when the channeler is strong in EVERY element, or when affinity does not
// apply — no elements, or an unset/unknown gender (safe degradation: missing
// data is never a penalty). Otherwise returns weakFactor (the weakest element
// governs the whole weave).
func affinityPotency(gender string, elements []string, weakFactor float64) float64 {
	if len(elements) == 0 {
		return 1.0
	}
	var strong map[string]bool
	switch strings.ToLower(strings.TrimSpace(gender)) {
	case "male":
		strong = maleAffinity
	case "female":
		strong = femaleAffinity
	default:
		return 1.0 // unset/unknown gender → no affinity, full potency
	}
	for _, el := range elements {
		if !strong[strings.ToLower(strings.TrimSpace(el))] {
			return weakFactor
		}
	}
	return 1.0
}

// scaleByPotency applies a potency multiplier to a rolled amount, rounding to
// the nearest whole. A potency below 1 weakens the payload (affinity woven off
// one's gender-derived strength); a potency above 1 amplifies it (a same-gender
// angreal/sa'angreal held while channeling — wot-the-one-power.md S2). A
// potency of exactly 1.0 (the inert / full-strength case) returns the amount
// unchanged. The caller still floors the result at 1 (a landed weave is never
// for nothing). Unlike the resolver's effect-path PotencyFunc — contracted
// weaken-only, (0,1] — this damage/heal scaler is the one path angreal can
// amplify in slice 1; effect-path amplification (bigger save DCs / buffs) is a
// deferred follow-up that would relax that contract.
func scaleByPotency(amount int, potency float64) int {
	if potency == 1.0 {
		return amount
	}
	return int(float64(amount)*potency + 0.5)
}

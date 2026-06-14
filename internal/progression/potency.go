package progression

import "github.com/Jasrags/AnotherMUD/internal/stats"

// PotencyFunc is the host's optional hook for setting-specific weave potency:
// it returns a magnitude multiplier (expected in (0, 1]) for a (caster,
// ability) pair. On a landed effect the resolver scales that weave's installed
// magnitudes — a save-gated entry DC, the effect's recurring-save DC, and its
// stat-modifier values — by this factor, so a weave woven outside the
// channeler's strength is both easier to resist and weaker once it lands.
//
// This keeps the engine setting-agnostic: the WoT One-Power affinity rule
// (gender-derived strength in the Five Powers) lives in the composition root
// and is injected here, exactly as SetSaveResolver injects the save bridge. A
// nil provider — or a multiplier ≥ 1 — leaves every magnitude untouched, so
// fantasy packs and every pre-affinity test resolve byte-identically.
//
// The damage/heal payload of a weave is scaled by the same host factor in the
// ability.used handler; this seam covers the OTHER half (effects + save DCs)
// the resolver owns directly.
type PotencyFunc func(sourceID, abilityID string) float64

// scaleMagnitude scales an integer magnitude by potency, rounding half away
// from zero and preserving sign. A potency ≥ 1 (the inert / full-strength case)
// returns v unchanged. Sign-safe so a future debuff weave carrying negative
// modifiers scales its magnitude rather than flipping it.
func scaleMagnitude(v int, potency float64) int {
	if potency >= 1.0 {
		return v
	}
	if potency <= 0 {
		// A non-positive multiplier zeroes the magnitude. PotencyFunc is
		// contracted to (0, 1] and the live WoT provider is env-clamped there,
		// but a future provider returning ≤ 0 must not flip a buff's sign — a
		// dead weave is weightless, not inverted.
		return 0
	}
	if v < 0 {
		return -scaleMagnitude(-v, potency)
	}
	return int(float64(v)*potency + 0.5)
}

// scaleDC scales a saving-throw DC by potency, flooring the result at 1. A DC
// below 1 is a degenerate auto-fail — a weak weave is easy to resist, not
// impossible to land — so the floor keeps a scaled-down control weave a real,
// if generous, save.
func scaleDC(dc int, potency float64) int {
	if scaled := scaleMagnitude(dc, potency); scaled > 1 {
		return scaled
	}
	return 1
}

// scaledBy returns a copy of the template with its installed magnitudes scaled
// by potency: every stat-modifier value and the recurring-save DC (when
// present). The original template is never mutated — it is shared across every
// cast of the ability. Potency governs strength, not persistence: duration,
// flags, id, and refresh semantics pass through unchanged. A potency ≥ 1
// returns an equivalent full-strength copy.
func (t EffectTemplate) scaledBy(potency float64) EffectTemplate {
	if potency >= 1.0 {
		return t
	}
	out := t // value copy; replace the shared slice/pointer fields we scale
	if len(t.Modifiers) > 0 {
		mods := make([]stats.Modifier, len(t.Modifiers))
		for i, m := range t.Modifiers {
			m.Value = scaleMagnitude(m.Value, potency)
			mods[i] = m
		}
		out.Modifiers = mods
	}
	if t.RecurringSave != nil {
		rs := *t.RecurringSave
		rs.DC = scaleDC(rs.DC, potency)
		out.RecurringSave = &rs
	}
	return out
}

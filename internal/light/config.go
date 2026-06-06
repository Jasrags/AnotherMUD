package light

import "github.com/Jasrags/AnotherMUD/internal/gameclock"

// Config holds the externally-tunable light policy (spec §11). None of
// these magnitudes are fixed by the spec; DefaultConfig supplies the
// documented starting point and the composition root may override.
type Config struct {
	// AmbientByPeriod maps a time-of-day period (gameclock period
	// names) to the sky's ambient level. Per §2.2 ambient is NEVER
	// black — AmbientFor floors any entry (and any unknown period) at
	// Gloom, the darkest natural sky.
	AmbientByPeriod map[string]Level
	// IndoorCap is the ceiling on ambient reaching an `indoors` room
	// (windows let some sky through, §2.3).
	IndoorCap Level
	// DarkvisionFloor is the per-viewer minimum a darkvision race
	// sees in any room; DarkvisionCap bounds how bright darkvision
	// alone can make a room (monochrome/shape-only, never daylight —
	// §4). Floor ≤ Cap.
	DarkvisionFloor Level
	DarkvisionCap   Level
	// AutoLightOnEquip, when true, lights a fuel-bearing source as it
	// is equipped into the light slot (§3.1). Extinguishing is always
	// explicit regardless, to conserve fuel. Off by default — a player
	// chooses when to spend fuel.
	AutoLightOnEquip bool
	// EffectFloors maps an active-effect flag to the per-viewer light
	// floor it grants while present (§4 light/sight effects). A
	// cast-light or infravision effect carries one of these flags;
	// EffectFloorFor reads them. Empty by default — content adds a
	// light effect and the operator lists its flag here so the floor
	// it grants is configurable, not hardcoded (§11).
	EffectFloors map[string]Level
	// CombatHitPenalty maps an attacker's effective light level to the
	// to-hit penalty (a non-negative magnitude) they suffer in combat
	// (§5.3): brighter is better, `lit` is zero, and the penalty only
	// degrades accuracy — it never blocks a swing. Applied as a
	// negative delta to the attacker's hit roll.
	CombatHitPenalty map[Level]int
}

// DefaultConfig is the spec's documented starting point: full daylight,
// twilight one rung down, night at the gloom floor; an indoor cap of
// dim; darkvision floored and capped at gloom.
func DefaultConfig() Config {
	return Config{
		AmbientByPeriod: map[string]Level{
			gameclock.PeriodDay:   Lit,
			gameclock.PeriodDawn:  Dim,
			gameclock.PeriodDusk:  Dim,
			gameclock.PeriodNight: Gloom,
		},
		IndoorCap:       Dim,
		DarkvisionFloor: Gloom,
		DarkvisionCap:   Gloom,
		CombatHitPenalty: map[Level]int{
			Lit:   0,
			Dim:   1,
			Gloom: 2,
			Black: 4,
		},
	}
}

// HitPenalty returns the to-hit penalty (a non-negative magnitude) an
// attacker at the given effective light suffers (§5.3). Absent entries
// and a nil table return 0 (no penalty), so combat is never harder than
// configured and an unconfigured resolver leaves accuracy untouched.
func (c Config) HitPenalty(lvl Level) int {
	p := c.CombatHitPenalty[lvl]
	if p < 0 {
		return 0
	}
	return p
}

// AmbientFor returns the sky's ambient level for the given period,
// enforcing the §2.2 invariant that ambient is never black: a
// configured-below-gloom entry, or an unknown period, both floor at
// Gloom. An unknown period failing safe to Gloom (not Black) keeps the
// "night ≠ black" guarantee even if the clock emits a period the
// config forgot.
func (c Config) AmbientFor(period string) Level {
	lvl, ok := c.AmbientByPeriod[period]
	if !ok || lvl < Gloom {
		return Gloom
	}
	return clamp(lvl)
}

// DarkvisionViewerFloor returns the per-viewer floor a darkvision
// viewer gets (DarkvisionFloor clamped down to DarkvisionCap), or
// Black when the viewer has no darkvision. Kept on Config so the
// floor/cap policy lives in one place; the viewer's darkvision flag is
// read by the caller (§4).
func (c Config) DarkvisionViewerFloor(hasDarkvision bool) Level {
	if !hasDarkvision {
		return Black
	}
	floor := c.DarkvisionFloor
	if floor > c.DarkvisionCap {
		floor = c.DarkvisionCap
	}
	return clamp(floor)
}

package progression

// Saving throws (Fortitude / Reflex / Will) — EPIC sub-epic S6, spec
// docs/specs/saves.md §2. A save value is a DERIVED read: a class-granted
// base bonus (strong/weak level progression) plus the d20 modifier of a
// governing ability read off the stat block. Nothing here is persisted —
// the values recompute on demand, exactly like the weapon-proficiency set
// (weapon-identity §7).
//
// This file is the pure progression-layer primitive: types, the base-bonus
// composition, and the ability-modifier add. The resolve check
// (d20 + save vs DC) lives in the combat layer (saves §3); the runtime
// composition for a live actor lives in session.

// SaveType identifies one of the three saving throws (saves §2).
type SaveType string

const (
	SaveFortitude SaveType = "fortitude"
	SaveReflex    SaveType = "reflex"
	SaveWill      SaveType = "will"
)

// SaveTypes is the canonical ordered set, for callers (score sheet, GMCP)
// that draw every axis. Ordered Fort/Reflex/Will to match the source table.
var SaveTypes = [...]SaveType{SaveFortitude, SaveReflex, SaveWill}

// GoverningStat returns the canonical ability whose d20 modifier feeds a
// save (saves §2): Fortitude←Constitution, Reflex←Dexterity, Will←Wisdom.
// An unrecognized SaveType returns the empty stat (modifier reads as the
// no-stat default).
func (s SaveType) GoverningStat() StatType {
	switch s {
	case SaveFortitude:
		return StatCON
	case SaveReflex:
		return StatDEX
	case SaveWill:
		return StatWIS
	default:
		return ""
	}
}

// SaveProgression marks a class's base-save curve for one axis (saves §2).
// An axis a class does not declare defaults to weak (the broadly-applicable
// "poor save" track), so minimal class content still produces working saves.
type SaveProgression string

const (
	SaveWeak   SaveProgression = "weak"
	SaveStrong SaveProgression = "strong"
)

// AbilityModifier is the d20 ability modifier `(score - 10) / 2` (integer
// division, may be negative). It is the same convention combat.STRBonus
// applies to damage; saves reuse it so a buff that raises CON/DEX/WIS moves
// the matching save for free. Kept in the progression domain because saves
// are a progression concept; the two trivial copies each read clearly in
// their own package.
func AbilityModifier(score int) int {
	return (score - 10) / 2
}

// SaveProgressionCurve maps a character level to a base-save bonus via the
// d20 closed form `base + level/divisor` (integer division). Strong saves
// use the "good save" shape (base 2, divisor 2 → 2,3,3,4,…); weak saves the
// "poor save" shape (base 0, divisor 3 → 0,0,1,1,…). A non-positive divisor
// degrades to a flat `base` at every level. All magnitudes are config, not
// hardcoded balance (saves §6).
type SaveProgressionCurve struct {
	Base    int
	Divisor int
}

// BonusAt returns the base-save bonus at the given level. Levels below 1 are
// treated as level 1 (a level-0 character still has its base term).
func (c SaveProgressionCurve) BonusAt(level int) int {
	if level < 1 {
		level = 1
	}
	if c.Divisor <= 0 {
		return c.Base
	}
	return c.Base + level/c.Divisor
}

// SaveConfig holds the strong and weak base-save curves (saves §6). One
// config governs every class; a class only declares which axis rides which
// curve.
type SaveConfig struct {
	Strong SaveProgressionCurve
	Weak   SaveProgressionCurve
}

// DefaultSaveConfig returns the engine-default d20 good/poor save curves.
// Packs may override the magnitudes; the prose (saves §2) names the
// behavior, the numbers live here.
func DefaultSaveConfig() SaveConfig {
	return SaveConfig{
		Strong: SaveProgressionCurve{Base: 2, Divisor: 2},
		Weak:   SaveProgressionCurve{Base: 0, Divisor: 3},
	}
}

// Saves is the resolved (or base-only) trio of save values. Used both for
// the class-granted base set (ClassBaseSaves) and the final derived set
// (DeriveSaves); the difference is whether the ability modifier has been
// folded in.
type Saves struct {
	Fortitude int
	Reflex    int
	Will      int
}

// Get returns the value for one axis; an unknown SaveType returns 0.
func (s Saves) Get(t SaveType) int {
	switch t {
	case SaveFortitude:
		return s.Fortitude
	case SaveReflex:
		return s.Reflex
	case SaveWill:
		return s.Will
	default:
		return 0
	}
}

// set writes one axis (internal helper for the builders below).
func (s *Saves) set(t SaveType, v int) {
	switch t {
	case SaveFortitude:
		s.Fortitude = v
	case SaveReflex:
		s.Reflex = v
	case SaveWill:
		s.Will = v
	}
}

// ClassSaveInput pairs a class with the character's level in that class (its
// bound-track level) for base-save composition (saves §2). A multiclass
// character supplies one input per class; a single-class character one.
type ClassSaveInput struct {
	Class *Class
	Level int
}

// Note on negative configs: ClassBaseSaves floors a no-contributing-class
// axis at zero (the mob path supplies its base directly). With the default
// curves every bonus is >= 0, so this is moot; a pack that configures a
// negative base term would see a class still contribute it (the loop tracks
// the best seen value), only the *no-class* case floors at zero.

// ClassBaseSaves composes the class-granted base-save bonuses across a
// character's classes, taking the STRONGEST contributing class per axis
// (saves §2, multiclass best-per-axis). An axis a class does not declare
// rides the weak curve. With no inputs every axis is zero (the mob path
// supplies its base directly instead).
func ClassBaseSaves(inputs []ClassSaveInput, cfg SaveConfig) Saves {
	var out Saves
	for _, axis := range SaveTypes {
		best := 0
		seen := false
		for _, in := range inputs {
			if in.Class == nil {
				continue
			}
			curve := cfg.Weak
			if in.Class.SaveProgressions[axis] == SaveStrong {
				curve = cfg.Strong
			}
			b := curve.BonusAt(in.Level)
			if !seen || b > best {
				best, seen = b, true
			}
		}
		out.set(axis, best)
	}
	return out
}

// DeriveSaves folds the governing ability modifier into a base-save set
// (saves §2): for each axis, value = base + AbilityModifier(score(stat)).
// score reads the governing ability's effective value (a player passes
// StatBlock.Effective; a mob passes its template lookup), decoupling the
// derivation from how scores are stored. A nil score function contributes
// no modifier (base-only saves) — safe for tests.
func DeriveSaves(base Saves, score func(StatType) int) Saves {
	out := base
	if score == nil {
		return out
	}
	for _, axis := range SaveTypes {
		mod := AbilityModifier(score(axis.GoverningStat()))
		out.set(axis, base.Get(axis)+mod)
	}
	return out
}

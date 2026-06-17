package feat

import "strings"

// GrantKind discriminates what a feat confers (EPIC S4 Phase 3 —
// docs/proposals/wot-feats.md §2.4). Kinds are added as their consuming
// surface is wired; an unknown kind is a decode error. Phase 3a ships
// save_bonus (the saves trio); later slices add max_hp, hit/crit per weapon,
// skill, and ability-gate kinds.
type GrantKind string

const (
	// GrantSaveBonus adds Magnitude to a saving-throw axis (Target is the axis
	// "fortitude" / "reflex" / "will"). Consumer: the derived-saves path.
	GrantSaveBonus GrantKind = "save_bonus"
	// GrantMaxHP adds Magnitude to the character's maximum HP (Toughness).
	// Target is unused. Consumer: an hp_max stat modifier under srckey.Feat,
	// which the existing OnMaxChange→vitals binding turns into a real ceiling.
	GrantMaxHP GrantKind = "max_hp"
	// GrantHitBonus adds Magnitude to-hit with a weapon CATEGORY (Weapon
	// Focus). The category is the take's Param (a per-parameter feat), not the
	// grant Target. Consumer: connActor.Stats() HitMod for the wielded weapon.
	GrantHitBonus GrantKind = "hit_bonus"
	// GrantCritThreat widens the critical threat range by Magnitude faces with
	// a weapon CATEGORY (Improved Critical) — the take's Param. Consumer:
	// connActor.Stats() lowers CritThreatLow for the wielded weapon.
	GrantCritThreat GrantKind = "crit_threat"
	// GrantSkillBonus adds Magnitude to a SKILL's check (Skill Emphasis). The
	// skill ability id is the take's Param. Consumer: the skill-check sites.
	GrantSkillBonus GrantKind = "skill_bonus"
	// GrantAbility teaches an ability (Power Attack). Target is the ability id;
	// Magnitude unused. Consumer: prof.Learn at grant time (applyFeatGrants).
	GrantAbility GrantKind = "ability"
	// GrantTwoWeaponHit reduces the two-weapon to-hit penalty on BOTH hands by
	// Magnitude (Two-Weapon Fighting — two-weapon-fighting §4.1). A GLOBAL grant
	// (Target unused; not per-weapon). Consumer: connActor.Stats() subtracts it
	// from the main- and off-hand two-weapon penalties (clamped at zero).
	GrantTwoWeaponHit GrantKind = "two_weapon_hit"
	// GrantOffHandHit reduces the OFF-HAND two-weapon penalty by Magnitude
	// (Ambidexterity — removes the off-hand-specific extra, two-weapon-fighting
	// §4.1). A GLOBAL grant (Target unused). Consumer: connActor.Stats()
	// subtracts it from the off-hand penalty only (clamped at zero).
	GrantOffHandHit GrantKind = "off_hand_hit"
)

// ValidGrantKind reports whether k is a known grant kind.
func ValidGrantKind(k GrantKind) bool {
	switch k {
	case GrantSaveBonus, GrantMaxHP, GrantHitBonus, GrantCritThreat, GrantSkillBonus, GrantAbility,
		GrantTwoWeaponHit, GrantOffHandHit:
		return true
	}
	return false
}

// IsPerWeaponOrSkill reports whether a grant kind's target is supplied by the
// take's Param (a per-parameter feat) rather than the grant Target. The decode
// requires such a feat to be multi_take: per_param.
func IsPerWeaponOrSkill(k GrantKind) bool {
	switch k {
	case GrantHitBonus, GrantCritThreat, GrantSkillBonus:
		return true
	}
	return false
}

// saveAxes is the valid Target set for a GrantSaveBonus. Mirrors the engine's
// Fort/Reflex/Will axes (progression.SaveType) — kept here as a small stable
// vocabulary so decode can reject a typo'd axis without the leaf feat package
// importing progression.
var saveAxes = map[string]bool{"fortitude": true, "reflex": true, "will": true}

// ValidSaveAxis reports whether s names a save axis (case-insensitive).
func ValidSaveAxis(s string) bool {
	return saveAxes[strings.ToLower(strings.TrimSpace(s))]
}

// Grant is one bonus a feat confers (§2.4). The meaning of Target / Magnitude
// depends on Kind (for GrantSaveBonus: Target = axis, Magnitude = the bonus).
type Grant struct {
	Kind      GrantKind
	Target    string
	Magnitude int
}

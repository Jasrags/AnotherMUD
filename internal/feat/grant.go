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
	// GrantOffHandAttack adds Magnitude EXTRA off-hand strikes per round (Improved
	// Two-Weapon Fighting — two-weapon-fighting §3.1). A GLOBAL grant (Target
	// unused). Consumer: connActor.Stats() raises OffHandProfile.Attacks; each
	// strike after the first takes the cumulative secondary off-hand penalty (§4.3).
	GrantOffHandAttack GrantKind = "off_hand_attack"
	// GrantDamageBonus adds Magnitude to melee damage with a weapon CATEGORY
	// (Weapon Specialization) — the take's Param, the damage sibling of
	// GrantHitBonus. Consumer: connActor.Stats() raises DamageBonus for the
	// wielded weapon of that category.
	GrantDamageBonus GrantKind = "damage_bonus"
	// GrantACBonus adds Magnitude to the character's Armor Class (Dodge). A
	// GLOBAL grant (Target unused), the AC sibling of GrantMaxHP. Consumer: an
	// `ac` stat modifier under srckey.Feat, which the channel-map defense formula
	// (`ac` / `ac + dex_ac`) reads — so it lands for both baseline and WoT.
	GrantACBonus GrantKind = "ac_bonus"
	// GrantWeaponProficiency makes the actor proficient with a weapon CATEGORY
	// (Target = the category id, e.g. "light-crossbow"; Militia). Magnitude
	// unused. A fixed-target grant (like save_bonus), NOT per-param. Consumer:
	// connActor.IsWeaponProficient unions these granted categories with the
	// class proficiency set, so a weapon outside the class's proficiencies no
	// longer takes the non-proficient to-hit penalty.
	GrantWeaponProficiency GrantKind = "weapon_proficiency"
	// GrantRenownBonus adds Magnitude to the character's EFFECTIVE renown (Fame —
	// reputation.md §7). A GLOBAL grant (Target unused), the renown sibling of
	// GrantACBonus. Consumer: connActor.EffectiveRenown folds it onto the stored
	// score for display and (later) recognition checks.
	GrantRenownBonus GrantKind = "renown_bonus"
	// GrantInfamy flags the character as infamous (Infamy — reputation.md §7,
	// PD-5): reactions resolve as feared/reviled regardless of the score's sign.
	// A boolean GLOBAL grant (Target/Magnitude unused). Consumer: the disposition
	// reaction (R6) and the score sheet's infamy marker.
	GrantInfamy GrantKind = "infamy"
	// GrantLowProfile scales DOWN subsequent renown gains (Low Profile —
	// reputation.md §7): a famous-but-discreet character accrues fame slowly.
	// A boolean GLOBAL grant (Target/Magnitude unused). Consumer: the
	// reputation.shift.check subscriber scales a positive suggested delta.
	GrantLowProfile GrantKind = "low_profile"
)

// ValidGrantKind reports whether k is a known grant kind.
func ValidGrantKind(k GrantKind) bool {
	switch k {
	case GrantSaveBonus, GrantMaxHP, GrantHitBonus, GrantCritThreat, GrantSkillBonus, GrantAbility,
		GrantTwoWeaponHit, GrantOffHandHit, GrantOffHandAttack, GrantDamageBonus, GrantACBonus,
		GrantWeaponProficiency, GrantRenownBonus, GrantInfamy, GrantLowProfile:
		return true
	}
	return false
}

// IsPerWeaponOrSkill reports whether a grant kind's target is supplied by the
// take's Param (a per-parameter feat) rather than the grant Target. The decode
// requires such a feat to be multi_take: per_param.
func IsPerWeaponOrSkill(k GrantKind) bool {
	switch k {
	case GrantHitBonus, GrantCritThreat, GrantSkillBonus, GrantDamageBonus:
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

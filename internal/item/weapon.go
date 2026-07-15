package item

// Weapon-identity vocabularies (spec weapon-identity §2). v1 ships these
// as engine baselines — the WoT pack's simple/martial/exotic proficiency
// tiers and the fixed bludgeoning/piercing/slashing damage types. Making
// the tier vocabulary pack-declarable is a later extension; for now an
// unlisted tier or damage type is an authoring error caught at pack load.

import "slices"

// weaponTiers is the ordered proficiency-tier vocabulary, LOW→HIGH, so a
// future graduated non-proficient penalty can read the tier distance.
var weaponTiers = []string{"simple", "martial", "exotic"}

// WeaponTierNames returns a copy of the ordered proficiency-tier
// vocabulary (low→high). Used for validation error messages.
func WeaponTierNames() []string { return append([]string(nil), weaponTiers...) }

// LowestTier is the broadly-usable tier every character is proficient
// with (weapon-identity §3). A weapon with no declared tier is treated as
// this tier.
func LowestTier() string { return weaponTiers[0] }

// ValidTier reports whether name is a known proficiency tier. The empty
// string ("untiered") is NOT a tier name — callers treat absence as
// LowestTier separately.
func ValidTier(name string) bool {
	return slices.Contains(weaponTiers, name)
}

// The damage-type set (weapon-identity §2). The physical trio is the
// weapon-identity baseline; the environmental types are consumed by armor
// resistances vs. biome hazards (area-effects §4.6 — radiation/toxic are the
// keys the Shadowrun toxic/ashfall zones deal). Making the vocabulary
// pack-declarable (armor-depth §9) is a later extension; for now an unlisted
// type is an authoring error caught at pack load.
const (
	DamageBludgeoning = "bludgeoning"
	DamagePiercing    = "piercing"
	DamageSlashing    = "slashing"
	DamageRadiation   = "radiation"
	DamageToxic       = "toxic"
	// DamageFire is an energy damage type (a flamethrower, a fire spell). Like
	// radiation/toxic it is a first-class type: a weapon deals it and armor may
	// resist it (resistances: {fire: N}). It is soaked by general armor
	// (Mitigation) the same as any type — bypassing armor is the separate,
	// unmodeled AP mechanic — but fire-resistant gear soaks it specifically.
	DamageFire = "fire"
)

var damageTypes = []string{
	DamageBludgeoning, DamagePiercing, DamageSlashing,
	DamageRadiation, DamageToxic, DamageFire,
}

// DamageTypeNames returns a copy of the valid damage-type vocabulary.
// Used for validation error messages.
func DamageTypeNames() []string { return append([]string(nil), damageTypes...) }

// ValidDamageType reports whether name is a known damage type.
func ValidDamageType(name string) bool {
	return slices.Contains(damageTypes, name)
}

// Ranged weapon classes (ranged-combat §2). A weapon that declares no
// ranged class is melee, exactly as today. "thrown" means the weapon
// itself is hurled (a knife, a spear) and lands recoverable in the room;
// "projectile" means it launches separate ammunition (a bow, a crossbow,
// a sling) consumed one unit per shot.
const (
	RangedThrown     = "thrown"
	RangedProjectile = "projectile"
)

var rangedClasses = []string{RangedThrown, RangedProjectile}

// RangedClassNames returns a copy of the valid ranged-class vocabulary.
// Used for validation error messages.
func RangedClassNames() []string { return append([]string(nil), rangedClasses...) }

// Firing modes (ranged-combat §5.5). A weapon's FireModes is a subset of these.
// "single" is always available implicitly (the default); "burst"/"auto" trade
// ammo + accuracy for damage. The names are the content contract + the combat
// effect-map keys.
const (
	FireModeSingle = "single"
	FireModeBurst  = "burst"
	FireModeAuto   = "auto"
)

var fireModes = []string{FireModeSingle, FireModeBurst, FireModeAuto}

// ValidFireMode reports whether name is a known firing mode.
func ValidFireMode(name string) bool { return slices.Contains(fireModes, name) }

// FireModeNames returns a copy of the valid firing-mode vocabulary (for
// validation error messages and the `firemode` verb's usage).
func FireModeNames() []string { return append([]string(nil), fireModes...) }

// ValidRangedClass reports whether name is a known ranged class. The empty
// string ("melee") is NOT a ranged class — callers treat absence as melee
// separately.
func ValidRangedClass(name string) bool {
	return slices.Contains(rangedClasses, name)
}

// RangedDamageBonus applies the ranged-combat §4 Strength rule to a
// weapon's base Strength-derived damage bonus, returning the bonus that
// should actually be added to the rolled dice.
//
//   - Thrown (and melee): the FULL bonus — you put your body into the throw,
//     exactly like a melee swing.
//   - Plain projectile (strRating nil): NO positive bonus — the bowstring
//     does the work — but a NEGATIVE modifier still applies (too weak to draw
//     it cleanly).
//   - Strength-rated projectile (strRating non-nil): a positive bonus CAPPED
//     at the rating (a composite bow built to a draw); a negative modifier
//     still applies in full.
//
// base is the holder's already-composed Strength damage bonus (the channel
// layer's damage_bonus; in the baseline trunc((str-10)/2)). rangedClass is
// the wielded weapon's class ("" / thrown / projectile).
func RangedDamageBonus(rangedClass string, strRating *int, base int) int {
	if rangedClass != RangedProjectile {
		// Thrown adds the full Strength bonus; a melee weapon is unchanged.
		return base
	}
	if base <= 0 {
		// A negative Strength modifier still applies to a bow (and zero is
		// already nothing to cap).
		return base
	}
	if strRating == nil {
		// Plain projectile: the string does the work, no positive bonus.
		return 0
	}
	if base > *strRating {
		return *strRating
	}
	return base
}

// Proficient reports whether a wielder whose class grants the given weapon
// tiers and categories may use a weapon of weaponTier / weaponCategory
// without the non-proficient penalty (weapon-identity §3). Every wielder is
// proficient with the lowest tier — and with an untiered weapon, which is
// treated as the lowest tier — so an empty or lowest-tier weapon is always
// proficient regardless of grants. Otherwise the weapon is proficient when
// its tier is in the granted tier set OR its category is in the granted
// category set. All inputs are assumed already normalized (lowercased) by
// the pack loader.
func Proficient(grantedTiers, grantedCategories []string, weaponTier, weaponCategory string) bool {
	if weaponTier == "" || weaponTier == LowestTier() {
		return true
	}
	if slices.Contains(grantedTiers, weaponTier) {
		return true
	}
	if weaponCategory != "" {
		if slices.Contains(grantedCategories, weaponCategory) {
			return true
		}
	}
	return false
}

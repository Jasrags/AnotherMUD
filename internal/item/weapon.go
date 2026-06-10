package item

// Weapon-identity vocabularies (spec weapon-identity §2). v1 ships these
// as engine baselines — the WoT pack's simple/martial/exotic proficiency
// tiers and the fixed bludgeoning/piercing/slashing damage types. Making
// the tier vocabulary pack-declarable is a later extension; for now an
// unlisted tier or damage type is an authoring error caught at pack load.

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
	for _, t := range weaponTiers {
		if t == name {
			return true
		}
	}
	return false
}

// The fixed damage-type set (weapon-identity §2). Recorded on weapons;
// inert until the armor-depth slice gives damage types an effect.
const (
	DamageBludgeoning = "bludgeoning"
	DamagePiercing    = "piercing"
	DamageSlashing    = "slashing"
)

var damageTypes = []string{DamageBludgeoning, DamagePiercing, DamageSlashing}

// DamageTypeNames returns a copy of the valid damage-type vocabulary.
// Used for validation error messages.
func DamageTypeNames() []string { return append([]string(nil), damageTypes...) }

// ValidDamageType reports whether name is a known damage type.
func ValidDamageType(name string) bool {
	for _, d := range damageTypes {
		if d == name {
			return true
		}
	}
	return false
}

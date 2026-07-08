package item

// Armor-depth vocabularies (spec armor-depth §2, §5). v1 ships the armor
// proficiency-tier vocabulary as an engine baseline — the WoT pack's
// light/medium/heavy tiers, the mirror of the weapon tiers in weapon.go.
// Making the vocabulary pack-declarable is a later extension; for now an
// unlisted armor tier is an authoring error caught at pack load.
//
// The per-type damage-resistance map (Template.Resistances) is keyed over
// the damage-type vocabulary in weapon.go (ValidDamageType) — the same set
// weapons declare, so a resistance and the damage it soaks share names. That
// set is the fixed bludgeoning/piercing/slashing today and grows (e.g. with
// elemental types) when a ruleset that needs them lands.

import "slices"

// armorTiers is the ordered armor proficiency-tier vocabulary, LIGHT→HEAVY,
// so a future graduated rule can read the tier distance. Distinct from the
// weapon tiers (simple/martial/exotic).
var armorTiers = []string{"light", "medium", "heavy"}

// ArmorTierNames returns a copy of the ordered armor-tier vocabulary
// (light→heavy). Used for validation error messages.
func ArmorTierNames() []string { return append([]string(nil), armorTiers...) }

// ValidArmorTier reports whether name is a known armor tier. The empty
// string ("untiered") is NOT a tier name — callers treat absence separately.
func ValidArmorTier(name string) bool {
	return slices.Contains(armorTiers, name)
}

// ArmorProficient reports whether a wearer whose class(es) grant grantedTiers
// may wear an armor of armorTier without the non-proficient consequence
// (armor-depth §5). Untiered armor ("") is always proficient; a tiered armor
// is proficient only when its tier is in the granted set. The mirror of
// Proficient for weapons, minus the category axis (armor has no categories)
// and the lowest-tier-is-free rule (armor §5 grants nothing for free).
func ArmorProficient(grantedTiers []string, armorTier string) bool {
	if armorTier == "" {
		return true
	}
	return slices.Contains(grantedTiers, armorTier)
}

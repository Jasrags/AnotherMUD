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
	for _, t := range armorTiers {
		if t == name {
			return true
		}
	}
	return false
}

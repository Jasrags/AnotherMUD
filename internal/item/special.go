package item

// Special-weapon tags (special-weapons.md §2) — the starter set of increment J.
// A weapon names the special-maneuver behaviors it unlocks; an unlisted tag is
// an authoring error caught at pack load. Later J slices extend this vocabulary
// (set, entangle, …); the validation seam is the point.
//
//   - reach:  the weapon strikes at the `near` range band, not just melee (§3).
//   - trip:   the weapon makes the `trip` maneuver land harder, by trip_bonus (§4).
//   - disarm: the weapon makes the `disarm` maneuver land harder, by disarm_bonus (§5).
const (
	SpecialReach  = "reach"
	SpecialTrip   = "trip"
	SpecialDisarm = "disarm"
)

var specialTags = []string{SpecialReach, SpecialTrip, SpecialDisarm}

// SpecialTagNames returns a copy of the valid special-weapon-tag vocabulary.
// Used for validation error messages.
func SpecialTagNames() []string { return append([]string(nil), specialTags...) }

// ValidSpecialTag reports whether name is a known special-weapon tag.
func ValidSpecialTag(name string) bool {
	for _, t := range specialTags {
		if t == name {
			return true
		}
	}
	return false
}

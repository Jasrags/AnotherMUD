package item

// Special-weapon tags (special-weapons.md §2) — the maneuver behaviors a weapon
// unlocks. A weapon names the maneuvers it enables; an unlisted tag is an
// authoring error caught at pack load. Later J slices extend this vocabulary
// (set, entangle, …); the validation seam is the point.
//
//   - trip:   the weapon makes the `trip` maneuver land harder, by trip_bonus (§4).
//   - disarm: the weapon makes the `disarm` maneuver land harder, by disarm_bonus (§5).
//
// NOTE: reach is NOT a special tag — it is a numeric weapon stat (Template.Reach,
// special-weapons §3), shared across rulesets (WoT reads `reach > 0` = strikes at
// the near band; a Shadowrun pack reads net reach as a defense-roll modifier).
const (
	SpecialTrip   = "trip"
	SpecialDisarm = "disarm"
)

var specialTags = []string{SpecialTrip, SpecialDisarm}

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

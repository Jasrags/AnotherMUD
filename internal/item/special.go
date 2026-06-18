package item

// Special-weapon tags (special-weapons.md §2) — the maneuver behaviors a weapon
// unlocks. A weapon names the maneuvers it enables; an unlisted tag is an
// authoring error caught at pack load. Later J slices extend this vocabulary
// (set, entangle, …); the validation seam is the point.
//
//   - trip:   the weapon makes the `trip` maneuver land harder, by trip_bonus (§4).
//   - disarm: the weapon makes the `disarm` maneuver land harder, by disarm_bonus (§5).
//
// The set / net / whip / entangle tags are RECORDED-ONLY for now — part of the
// equipment.md authoring surface (a pike sets vs a charge, a net entangles, a
// whip trips at range). They validate at load so content can be authored once,
// but no combat consumer reads them yet; each lights up in its own later slice
// (the deferred special-weapon tail), exactly as trip/disarm shipped inert ahead
// of their consumers. Adding the tag here is the whole cost of authoring the data.
//
// NOTE: reach is NOT a special tag — it is a numeric weapon stat (Template.Reach,
// special-weapons §3), shared across rulesets (WoT reads `reach > 0` = strikes at
// the near band; a Shadowrun pack reads net reach as a defense-roll modifier).
const (
	SpecialTrip   = "trip"
	SpecialDisarm = "disarm"
	// Recorded-only (no consumer yet):
	SpecialSet      = "set"      // set vs a charge (pike/lance/bill)
	SpecialNet      = "net"      // a thrown net that entangles
	SpecialWhip     = "whip"     // reach-at-range subdual lash
	SpecialEntangle = "entangle" // immobilize/bind on hit
)

var specialTags = []string{
	SpecialTrip, SpecialDisarm,
	SpecialSet, SpecialNet, SpecialWhip, SpecialEntangle,
}

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

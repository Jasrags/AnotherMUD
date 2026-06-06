// Package light is the per-viewer effective-light surface from
// docs/specs/light-and-darkness.md. It computes, on demand and never
// stored, how well a specific viewer can see in a specific room right
// now — from time-of-day ambient, terrain sky-gating, an authored room
// override, lit light sources, and a per-viewer floor (darkvision /
// sight effects).
//
// The core (Level, Config, Resolve) is a pure leaf: Resolve operates
// on a gathered Inputs struct and imports only the world terrain
// vocabulary. Call sites (render, combat, movement) assemble Inputs
// from the room + viewer in hand, mirroring how combat assembles its
// Stats before rollHit.
package light

// Level is the ordinal effective-light scale (spec §2.1). The four
// names are fixed vocabulary — events, GMCP, and content reference
// them — so they are exported and stable. Higher means brighter.
type Level int

const (
	// Black: the viewer sees nothing.
	Black Level = iota
	// Gloom: shapes and directions, not detail.
	Gloom
	// Dim: everything, presented muted.
	Dim
	// Lit: everything (full render).
	Lit
)

// String returns the fixed lowercase vocabulary name for the level.
// An out-of-range value (which Resolve never produces) renders as
// "black" so a stray value fails safe to the most-restricted name.
func (l Level) String() string {
	switch l {
	case Lit:
		return "lit"
	case Dim:
		return "dim"
	case Gloom:
		return "gloom"
	default:
		return "black"
	}
}

// ParseLevel maps a vocabulary name (case-sensitive lowercase, the
// form content and the wire use) to its Level. ok is false for any
// other string, so a typo'd room `light` override is treated as "no
// valid override" rather than silently pinning black.
func ParseLevel(s string) (Level, bool) {
	switch s {
	case "lit":
		return Lit, true
	case "dim":
		return Dim, true
	case "gloom":
		return Gloom, true
	case "black":
		return Black, true
	default:
		return Black, false
	}
}

// clamp bounds l to the valid [Black, Lit] range (spec §2.2 final
// clamp). Resolve combines via max over in-range terms so this only
// bites if a caller hands in an override/source above Lit.
func clamp(l Level) Level {
	if l < Black {
		return Black
	}
	if l > Lit {
		return Lit
	}
	return l
}

// max returns the brighter of two levels — the combine operator
// throughout the resolver (§2.2 "the combine is a maximum").
func max(a, b Level) Level {
	if a > b {
		return a
	}
	return b
}

package render

// ColorTier is the per-session color-capability tier derived from
// TTYPE (spec networking-protocols.md §7.2). The renderer uses
// this to choose between no-color / ANSI-16 / 256-color / 24-bit
// truecolor SGR emission.
//
// M16.6a defines the tier and plumbs it through the connection
// abstraction; the render layer continues to emit ANSI-16 for
// every tier ≥ Basic until M16.6b lands tier-aware emission.
type ColorTier uint8

const (
	// ColorTierNone — no TTYPE received; emit no color.
	ColorTierNone ColorTier = iota
	// ColorTierBasic — ANSI-16 (the M0-era default).
	ColorTierBasic
	// ColorTierExtended — 256-color palette (xterm-256color and
	// known MUD clients per spec §7.2).
	ColorTierExtended
	// ColorTierTrueColor — 24-bit RGB SGR (TRUECOLOR TTYPE hint).
	ColorTierTrueColor
)

// String returns the tier's canonical short name for log fields.
func (t ColorTier) String() string {
	switch t {
	case ColorTierNone:
		return "none"
	case ColorTierBasic:
		return "basic"
	case ColorTierExtended:
		return "extended"
	case ColorTierTrueColor:
		return "truecolor"
	default:
		return "unknown"
	}
}

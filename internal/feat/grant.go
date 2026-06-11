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
)

// ValidGrantKind reports whether k is a known grant kind.
func ValidGrantKind(k GrantKind) bool {
	switch k {
	case GrantSaveBonus:
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

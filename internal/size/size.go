// Package size is the engine's creature/weapon size vocabulary and the
// size-relative wield-mode rule (size-and-wielding §2–§4). It is a leaf
// package — it imports only the standard library — so item, progression, mob,
// and session can all share it without a dependency cycle (the same role
// srckey and grade play for their domains).
//
// The engine compares sizes only by ordered DISTANCE, never by name (§2). v1
// ships the ordered vocabulary as an engine baseline (tiny … huge); making it
// pack-declarable is a later extension, mirroring how the armor-tier vocabulary
// shipped (internal/item/armor.go).
package size

import (
	"math"
	"slices"
	"strings"
)

// Baseline is the size a creature or weapon resolves to when it declares none
// (size-and-wielding §2) — the size of most playable races. A weapon authored
// before this feature is therefore one-handed for a baseline wielder, exactly
// its prior behavior.
const Baseline = "medium"

// sizes is the ordered size vocabulary, smallest → largest.
var sizes = []string{"tiny", "small", "medium", "large", "huge"}

// Names returns a copy of the ordered size vocabulary (smallest → largest).
func Names() []string { return append([]string(nil), sizes...) }

// Valid reports whether name is a known size. The empty string is NOT a size
// name — callers treat absence as Baseline separately (Resolve does this).
func Valid(name string) bool {
	return slices.Contains(sizes, name)
}

// Resolve normalizes a declared size to its ordinal: empty ⇒ Baseline, a known
// name ⇒ its index, an unknown non-empty name ⇒ Baseline too (validation
// happens at pack load; this stays total so a stray value can never panic a
// combat read). The bool reports whether name was a recognized size.
func resolve(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = Baseline
	}
	for i, s := range sizes {
		if s == name {
			return i
		}
	}
	// Unknown — fall back to the baseline ordinal (defensive; load validates).
	for i, s := range sizes {
		if s == Baseline {
			return i
		}
	}
	return 0
}

// Distance is the signed step distance (weaponSize − wielderSize) in the size
// vocabulary; empty or unknown names resolve to Baseline (§2). Negative ⇒ the
// weapon is smaller than the wielder.
func Distance(weaponSize, wielderSize string) int {
	return resolve(weaponSize) - resolve(wielderSize)
}

// WieldMode is the size-relative grip a (weapon, wielder) pair resolves to (§3).
type WieldMode int

const (
	// Light — weapon smaller than the wielder: off-hand eligible, no two-handed
	// Strength bonus.
	Light WieldMode = iota
	// OneHanded — same size: the ordinary grip, off hand free.
	OneHanded
	// TwoHanded — one step larger: ties up the off hand, grants the two-handed
	// Strength bonus.
	TwoHanded
	// TooLarge — two or more steps larger: unwieldy, the equip is refused.
	TooLarge
)

func (m WieldMode) String() string {
	switch m {
	case Light:
		return "light"
	case OneHanded:
		return "one-handed"
	case TwoHanded:
		return "two-handed"
	default:
		return "too large"
	}
}

// MaxWieldableStep is the largest (weapon − wielder) step distance still
// wieldable (§3); above it the weapon is TooLarge. One step larger is the
// two-handed band; two or more steps larger is unwieldy.
const MaxWieldableStep = 1

// UnarmedOffset is how many steps smaller than the wielder an unarmed strike
// counts as (§2.2) — so unarmed always resolves Light.
const UnarmedOffset = 2

// Mode resolves the wield mode for a (weaponSize, wielderSize) pair from the
// signed size distance (§3): smaller ⇒ Light, same ⇒ OneHanded, one step
// larger ⇒ TwoHanded, two or more steps larger ⇒ TooLarge.
func Mode(weaponSize, wielderSize string) WieldMode {
	switch d := Distance(weaponSize, wielderSize); {
	case d < 0:
		return Light
	case d == 0:
		return OneHanded
	case d <= MaxWieldableStep:
		return TwoHanded
	default:
		return TooLarge
	}
}

// DefaultTwoHandedStrFactor is the multiplier on the Strength damage
// contribution for a two-handed melee wield (§4.2) — the WoT value (1.5×).
const DefaultTwoHandedStrFactor = 1.5

// DefaultOffHandStrFactor is the multiplier on the Strength damage contribution
// for an off-hand melee wield (two-weapon-fighting §4.2) — the WoT value (½×).
const DefaultOffHandStrFactor = 0.5

// StrBonusDelta returns the adjustment to ADD to the normal 1× Strength damage
// contribution to leave only the factor× share: floor(strBonus × factor) −
// strBonus (round down). It is the general primitive behind both the two-handed
// (factor > 1 ⇒ positive, more Strength) and off-hand (factor < 1 ⇒ negative,
// less Strength) rules — callers add it so only the Strength term is scaled;
// grade and other flat bonuses stay at 1×. Returns 0 when factor == 1.
func StrBonusDelta(strBonus int, factor float64) int {
	return int(math.Floor(float64(strBonus)*factor)) - strBonus
}

// TwoHandedStrBonus returns the EXTRA Strength-derived melee damage from
// wielding two-handed (§4.2): StrBonusDelta with the two-handed factor.
func TwoHandedStrBonus(strBonus int, factor float64) int {
	return StrBonusDelta(strBonus, factor)
}

// Package mount is the engine's mount vocabulary: the temperament ladder
// (how readily a mount carries its rider into danger, mounts.md §7.2) and the
// identity of a mount's travel-resource pool (the renewable movement budget a
// ridden mount spends instead of the rider's, §5).
//
// It is a near-leaf package — it imports only the standard library and the
// leaf pool package — so mob, entities, command, and session can all share it
// without a dependency cycle (the same role size and grade play for their
// domains). The mount's *behavior* (riding, mounted travel, barding, stabling)
// lives in the consumers; this package owns only the shared vocabulary.
package mount

import (
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/pool"
)

// PoolKindTravel is the stable identity of a mount's travel-resource pool
// (mounts.md §5.1). A ridden mount spends this pool per mounted step through
// the same cost gate a walking character spends their movement pool through —
// mounted travel re-points *who pays*, it does not invent a second metering
// model (§5 non-goal). Kept here, in the shared leaf, so the seeder (entities),
// the regen tick (session), and any reader agree on one string.
const PoolKindTravel = pool.Kind("travel")

// Temperament governs whether a mount will carry its rider *into* danger
// (mounts.md §7.2). It gates danger entry only, never ordinary travel.
type Temperament string

const (
	// War — a war-trained mount (a warhorse) tolerates combat: the rider may
	// ride into a hostile room and fight from the saddle.
	War Temperament = "war"
	// Steady — a hardy working animal (a mule) is willing where a horse balks;
	// it tolerates danger a skittish mount won't.
	Steady Temperament = "steady"
	// Skittish — an ordinary riding animal balks at entering a room with active
	// hostiles, or when its rider opens combat from its back.
	Skittish Temperament = "skittish"
)

// DefaultTravelRegen is the per-regen-tick travel-pool restore applied to a
// mount whose content left travel_regen unset (mounts.md §5.4). A mount always
// recovers SOME travel out of combat so a blown mount is never permanently
// stuck (the rider can also always dismount and walk, §6).
const DefaultTravelRegen = 5

// Default is the temperament a mount resolves to when its content declares
// none (mounts.md §7.2): the cautious ordinary riding animal. A mount authored
// without a temperament is therefore skittish, never accidentally war-trained.
const Default = Skittish

// temperaments is the recognized vocabulary, steadiest → flightiest.
var temperaments = []Temperament{War, Steady, Skittish}

// Names returns a copy of the recognized temperament names (for error text).
func Names() []Temperament { return append([]Temperament(nil), temperaments...) }

// Valid reports whether name is a known temperament. The empty string is NOT
// valid — callers treat absence as Default separately (Resolve does this).
func Valid(name string) bool {
	t := Temperament(strings.ToLower(strings.TrimSpace(name)))
	for _, k := range temperaments {
		if k == t {
			return true
		}
	}
	return false
}

// Resolve normalizes a declared temperament: empty ⇒ Default, a known name ⇒
// itself, an unknown non-empty name ⇒ Default too (validation happens at pack
// load; this stays total so a stray value can never panic a travel read).
func Resolve(name string) Temperament {
	t := Temperament(strings.ToLower(strings.TrimSpace(name)))
	if t == "" {
		return Default
	}
	for _, k := range temperaments {
		if k == t {
			return t
		}
	}
	return Default
}

// ToleratesDanger reports whether a mount of this temperament will carry its
// rider into a hostile room / let the rider open combat from its back
// (mounts.md §7.2). War and Steady tolerate danger; only Skittish balks. A
// balk never blocks the rider's own movement (the never-strand rule, §6) — the
// rider can always dismount and proceed on foot.
func (t Temperament) ToleratesDanger() bool {
	return Resolve(string(t)) != Skittish
}

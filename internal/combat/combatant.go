// Package combat owns the engage/disengage state, the per-round
// resolution loop, and the death flow described in docs/specs/combat.md.
//
// M7.1 scope is the prerequisite slice only: the Combatant interface,
// the mutable Vitals type, and the value-typed Stats block. The
// CombatManager, heartbeat bucket, auto-attack swings, and death-check
// pipeline arrive in M7.2-M7.5.
//
// The package is deliberately decoupled from internal/entities: combat
// consumes a small interface (Combatant) that both MobInstance and
// connActor satisfy from their own packages, rather than reaching down
// into a concrete entity type. This lets players keep their session-
// owned state (color, save record, link-dead phase) while still
// participating in the same combat loop as mobs.
package combat

import (
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/pool"
)

// CombatantID is the identity used by combat-side code to refer to a
// combatant. The string space is shared by mobs and players but kept
// disjoint at construction time: mob ids carry the MobPrefix and
// player ids carry the PlayerPrefix. When players become first-class
// entities (the M5.8 deferred resolution) the two namespaces
// collapse, but until then the prefix makes lookup unambiguous and
// prevents an accidental cross-table hit.
//
// Construct via NewMobCombatantID / NewPlayerCombatantID — a freshly
// concatenated `"mob:" + id` string from a caller that did not import
// the constant is a typo waiting to happen ("mobs:foo" would not
// match anything but would not error either).
type CombatantID string

// Prefix constants for the two CombatantID namespaces. Exported so
// future lookup code can do prefix-based dispatch without hardcoding
// the literal strings a second time.
const (
	MobPrefix    = "mob:"
	PlayerPrefix = "player:"
)

// NewMobCombatantID builds a CombatantID from a mob's runtime entity
// id (the EntityID assigned by the entity store at spawn).
func NewMobCombatantID(entityID string) CombatantID {
	return CombatantID(MobPrefix + entityID)
}

// NewPlayerCombatantID builds a CombatantID from a player's stable
// account-scoped identity (player.Save.ID).
func NewPlayerCombatantID(playerID string) CombatantID {
	return CombatantID(PlayerPrefix + playerID)
}

// EntityIDOf strips the namespace prefix from a CombatantID, yielding
// the bare runtime entity id (mob store id or player.Save.ID). The
// inverse of NewMobCombatantID / NewPlayerCombatantID. An id with no
// recognized prefix is returned unchanged — callers that need to know
// the namespace should dispatch on the prefix constants directly.
//
// Used by the M9.4 ability phase: combat tracks targets as prefixed
// CombatantIDs, but the progression layer (effect manager, resolver)
// keys on the bare entity id.
func EntityIDOf(c CombatantID) string {
	s := string(c)
	if strings.HasPrefix(s, MobPrefix) {
		return s[len(MobPrefix):]
	}
	if strings.HasPrefix(s, PlayerPrefix) {
		return s[len(PlayerPrefix):]
	}
	return s
}

// Combatant is the surface combat.Manager and the round loop consult.
// Both MobInstance (mob-side) and connActor (player-side) satisfy it
// from outside this package — keeping the interface tiny is what lets
// those two very different types coexist as combatants without combat
// having to know about either of them.
//
// Vitals returns a *pointer* (not a value) because hit-point state is
// genuinely mutable across a fight: the round loop reads and writes it
// from a heartbeat-tick goroutine, while a status command may read it
// concurrently from a session goroutine. The pointer carries its own
// mutex; callers do not need to coordinate locks.
//
// Stats is returned by value: equipment changes between rounds replace
// the whole block, so combat reads a fresh copy each round and never
// races a modification mid-round. Per-damage-type AC is intentionally
// absent — the M8 progression slice will widen Stats when it lands.
type Combatant interface {
	CombatantID() CombatantID
	Name() string
	Vitals() *Vitals
	Stats() Stats
	// Pools is the combatant's full resource-pool set — the destination for
	// a typed attack's Stats.TargetPool routing (shadowrun-mvp SR-M2: a Stun
	// monitor a stun weapon fills). hp lives in Vitals, not here, so the
	// canonical damage path never touches this; only a non-empty TargetPool
	// routes through it. May return nil for a combatant with no pool set (a
	// bare test actor, or a mob in a world that declares no extra monitors) —
	// callers treat nil as "the destination monitor does not exist" and the
	// swing lands without moving a vital.
	Pools() *pool.Set
}

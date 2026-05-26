package combat

import "github.com/Jasrags/AnotherMUD/internal/world"

// Locator resolves a CombatantID back to a live Combatant. The Manager
// stores only CombatantIDs internally so a logged-out player (or an
// untracked mob) drops out of the combat loop automatically via the
// spec §4.1 "missing target → disengage" branch — no out-of-band
// teardown contract needed.
//
// Implementations live outside the combat package because resolution
// is inherently cross-cutting: mob ids resolve through entities.Store,
// player ids resolve through session.Manager, and combat must not
// import either. The production wiring lives in cmd/anothermud and
// dispatches on the CombatantID prefix (MobPrefix vs PlayerPrefix).
//
// LookupCombatant MUST be safe to call from any goroutine — the
// round loop will call it from the heartbeat-tick goroutine while
// session goroutines call it through consider / kill / status verbs.
type Locator interface {
	LookupCombatant(id CombatantID) (Combatant, bool)
}

// MapLocator is a tiny Locator backed by a fixed map. Intended for
// tests — production code wires the live entities.Store + session
// .Manager adapter. Safe to call from any goroutine because the map
// is treated as read-only after construction; tests that need to
// add combatants mid-run should rebuild the locator.
type MapLocator map[CombatantID]Combatant

// LookupCombatant implements Locator.
func (m MapLocator) LookupCombatant(id CombatantID) (Combatant, bool) {
	c, ok := m[id]
	return c, ok
}

// RoomLocator resolves a CombatantID to the world room the combatant
// currently occupies. The auto-attack phase consults this for the spec
// §4.1 "different room" pre-flight check that pairwise-disengages a
// target who has moved or been removed from the world.
//
// Kept as a separate interface from Locator so the test-only MapLocator
// can stay a plain map literal — autoattack tests that need rooms wire
// a small mapRoomLocator helper alongside MapLocator. Production
// wiring (cmd/anothermud combatLocator) satisfies both interfaces from
// a single struct.
//
// RoomOf returns ok=false when the combatant is not in any tracked
// room (logged-out player, despawned mob). The auto-attack phase
// treats this identically to "different room" — the spec lumps
// "missing" and "different room" together at §4.1.
type RoomLocator interface {
	RoomOf(id CombatantID) (world.RoomID, bool)
}

// MapRoomLocator is a tiny RoomLocator backed by a fixed map.
// Intended for tests; production wiring uses the entities.Placement +
// session.Manager adapter inside cmd/anothermud.
type MapRoomLocator map[CombatantID]world.RoomID

// RoomOf implements RoomLocator.
func (m MapRoomLocator) RoomOf(id CombatantID) (world.RoomID, bool) {
	r, ok := m[id]
	return r, ok
}

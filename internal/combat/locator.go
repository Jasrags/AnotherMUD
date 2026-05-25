package combat

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

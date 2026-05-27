package session

import "github.com/Jasrags/AnotherMUD/internal/progression"

// EffectTargetResolver implements progression.TargetResolver for the
// production session layer. Given a player id, returns the live
// *connActor wrapped as a progression.EffectTarget (connActor
// satisfies the interface via EntityID + AddModifiers +
// RemoveBySource directly).
//
// Mob targets land here when M9.4 wires combat — the resolver will
// fan out to the mob store on a miss against the session manager.
// M9.2 keeps the surface player-only per the M9.2 scope choice.
type EffectTargetResolver struct {
	mgr *Manager
}

// NewEffectTargetResolver returns a resolver bound to mgr. A nil
// manager produces a resolver that always misses — useful in
// tests that exercise the manager-side bookkeeping without
// constructing a session manager.
func NewEffectTargetResolver(mgr *Manager) *EffectTargetResolver {
	return &EffectTargetResolver{mgr: mgr}
}

// ResolveTarget implements progression.TargetResolver. Returns
// (target, true) when the entity id is a currently-connected
// player; (nil, false) otherwise (logged out, disconnected, or
// mob — mob support arrives in M9.4).
func (r *EffectTargetResolver) ResolveTarget(entityID string) (progression.EffectTarget, bool) {
	if r == nil || r.mgr == nil || entityID == "" {
		return nil, false
	}
	a, ok := r.mgr.GetByPlayerID(entityID)
	if !ok {
		return nil, false
	}
	return a, true
}

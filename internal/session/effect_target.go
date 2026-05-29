package session

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// EffectTargetResolver implements progression.TargetResolver for the
// production session layer. It resolves an effect target id to a live
// progression.EffectTarget: first a connected player (*connActor), then
// — on a miss — a live mob in the entity store (*entities.MobInstance).
// Both satisfy the interface via EntityID + AddModifiers + RemoveBySource.
//
// Player ids and mob ids occupy disjoint namespaces (player ids come
// from the save; mob ids are store-minted), so trying the manager then
// the store cannot mis-resolve one as the other.
type EffectTargetResolver struct {
	mgr   *Manager
	store *entities.Store
}

// NewEffectTargetResolver returns a resolver over the session manager
// (players) and the entity store (mobs). Either may be nil — a nil
// dependency simply means that target class never resolves, which keeps
// manager-only and store-only tests cheap.
func NewEffectTargetResolver(mgr *Manager, store *entities.Store) *EffectTargetResolver {
	return &EffectTargetResolver{mgr: mgr, store: store}
}

// ResolveTarget implements progression.TargetResolver. Returns
// (target, true) when the id is a connected player or a live mob;
// (nil, false) otherwise (logged out, despawned, or unknown).
func (r *EffectTargetResolver) ResolveTarget(entityID string) (progression.EffectTarget, bool) {
	if r == nil || entityID == "" {
		return nil, false
	}
	if r.mgr != nil {
		if a, ok := r.mgr.GetByPlayerID(entityID); ok {
			return a, true
		}
	}
	// Mob fallback: a MobInstance is StatBlock-backed and satisfies
	// EffectTarget, so a poison/bless cast on a mob now installs its
	// modifiers (closes the m8-1 / m9-4 / m9-6 mob-effect deferrals).
	if r.store != nil {
		if e, ok := r.store.GetByID(entities.EntityID(entityID)); ok {
			if m, ok := e.(*entities.MobInstance); ok {
				return m, true
			}
		}
	}
	return nil, false
}

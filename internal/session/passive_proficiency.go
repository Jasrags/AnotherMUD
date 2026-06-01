package session

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// PassiveProficiency is the host composite that feeds combat's passive
// abilities (abilities-and-effects §6) the proficiency of ANY combatant
// — player or mob — behind one entity-id-keyed surface. It mirrors
// EffectTargetResolver's player-then-mob fallback so progression's
// PassiveResolver never has to know players from mobs (M9.5 #3).
//
// It satisfies the read surface the PassiveResolver type-asserts —
// Has / Proficiency / Cap — and the write surface ProficiencyMutator
// (AddProficiency). Player ids resolve through the persistent
// ProficiencyManager; mob ids resolve through the entity store to a
// MobInstance's immutable, content-defined proficiency map.
//
// Player ids (from the save) and mob ids (store-minted) occupy disjoint
// namespaces, so trying the manager then the store cannot mis-resolve
// one as the other — the same guarantee EffectTargetResolver relies on.
//
// Mobs do not train: AddProficiency is routed to the manager only for
// non-mob ids. Routing a mob id into the manager would CREATE a
// per-mob entry there (AddProficiency seeds on first touch), leaking an
// entry that nothing drops on despawn — exactly what keeping mob
// proficiency on the instance avoids. So mob gain is a deliberate no-op.
type PassiveProficiency struct {
	players *progression.ProficiencyManager
	store   *entities.Store
}

// NewPassiveProficiency builds the composite over the player
// proficiency manager and the entity store. Either may be nil — a nil
// dependency simply means that class of entity never resolves, keeping
// manager-only and store-only tests cheap (mirrors EffectTargetResolver).
func NewPassiveProficiency(players *progression.ProficiencyManager, store *entities.Store) *PassiveProficiency {
	return &PassiveProficiency{players: players, store: store}
}

// Has reports whether entityID knows abilityID at all. Players first,
// then a mob fallback. Part of the ProficiencyReader interface.
func (p *PassiveProficiency) Has(entityID, abilityID string) bool {
	if p == nil {
		return false
	}
	if p.players != nil && p.players.Has(entityID, abilityID) {
		return true
	}
	if m, ok := p.mob(entityID); ok {
		if _, known := m.Proficiency(abilityID); known {
			return true
		}
	}
	return false
}

// Proficiency returns (value, true) when entityID holds a proficiency
// for abilityID, else (0, false). Players first, then mob fallback.
// This is the richer accessor proficiencyValueOf type-asserts.
func (p *PassiveProficiency) Proficiency(entityID, abilityID string) (int, bool) {
	if p == nil {
		return 0, false
	}
	if p.players != nil {
		if v, ok := p.players.Proficiency(entityID, abilityID); ok {
			return v, ok
		}
	}
	if m, ok := p.mob(entityID); ok {
		return m.Proficiency(abilityID)
	}
	return 0, false
}

// Cap returns the effective proficiency cap for (entityID, abilityID).
// Players read their per-ability cap from the manager; mobs have no
// per-ability caps, so they take the global ceiling (100) — which makes
// the §3.5 gain guard a no-op for them anyway (they never train). This
// is the Cap accessor effectiveCapValueOf type-asserts.
func (p *PassiveProficiency) Cap(entityID, abilityID string) int {
	if p == nil {
		return 100
	}
	if p.players != nil && p.players.Has(entityID, abilityID) {
		return p.players.Cap(entityID, abilityID)
	}
	return 100
}

// AddProficiency routes passive §6.3 gain. Players train through the
// persistent manager; mob ids are a deliberate no-op (mobs use fixed
// content proficiency, and seeding the player manager with a mob id
// would leak an entry). Part of the ProficiencyMutator interface.
func (p *PassiveProficiency) AddProficiency(entityID, abilityID string, delta int) {
	if p == nil || p.players == nil {
		return
	}
	if _, isMob := p.mob(entityID); isMob {
		return
	}
	p.players.AddProficiency(entityID, abilityID, delta)
}

// mob resolves entityID to a live MobInstance, or (nil, false) when the
// id is not a tracked mob (a player id, a despawned mob, or unknown).
func (p *PassiveProficiency) mob(entityID string) (*entities.MobInstance, bool) {
	if p.store == nil || entityID == "" {
		return nil, false
	}
	e, ok := p.store.GetByID(entities.EntityID(entityID))
	if !ok {
		return nil, false
	}
	m, ok := e.(*entities.MobInstance)
	return m, ok
}

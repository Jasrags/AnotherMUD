package session

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// PassiveStatReader resolves an entity id to its current effective stat
// value for the passive proficiency-gain stat factor (abilities-and-
// effects §3.5 step 3). It is the host seam progression.PassiveResolver
// needs: the active resolver reads the gain-stat straight off its
// ResolutionSource, but a passive fires off a bare entity id, so the
// host has to resolve player-or-mob here.
//
// It mirrors EffectTargetResolver / PassiveProficiency's player-then-mob
// fallback: a player id reads off the live connActor's stat block, a mob
// id off the MobInstance's StatBlock. Player ids (from the save) and mob
// ids (store-minted) occupy disjoint namespaces, so the two-step lookup
// cannot mis-resolve one as the other. Either dependency may be nil — a
// nil mgr means players never resolve, a nil store means mobs never do,
// keeping focused tests cheap. An unresolved id ⇒ 0, which the resolver
// reads as "no stat factor" (the conservative active-path default).
type PassiveStatReader struct {
	mgr   *Manager
	store *entities.Store
}

// NewPassiveStatReader builds the reader over the session manager (for
// connected players) and the entity store (for mobs).
func NewPassiveStatReader(mgr *Manager, store *entities.Store) *PassiveStatReader {
	return &PassiveStatReader{mgr: mgr, store: store}
}

// StatValue returns entityID's effective value for stat, or 0 when the
// id resolves to neither a connected player nor a tracked mob. Satisfies
// progression.StatReader.
func (r *PassiveStatReader) StatValue(entityID string, stat progression.StatType) int {
	if r == nil || entityID == "" {
		return 0
	}
	if r.mgr != nil {
		if a, ok := r.mgr.GetByPlayerID(entityID); ok {
			// GetByPlayerID releases the manager lock before returning the
			// actor; StatValue then reads the StatBlock, which carries its
			// own RWMutex (no a.mu needed). Safe to read lock-free after the
			// manager lock drops — the same invariant CombatantByPlayerID and
			// the active resolver's source.StatValue reads rely on, both also
			// called on the tick goroutine.
			return a.StatValue(stat)
		}
	}
	if r.store != nil {
		if e, ok := r.store.GetByID(entities.EntityID(entityID)); ok {
			if m, ok := e.(*entities.MobInstance); ok {
				return m.StatBlock().Effective(stat)
			}
		}
	}
	return 0
}

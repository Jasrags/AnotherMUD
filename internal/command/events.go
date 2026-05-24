package command

import "github.com/Jasrags/AnotherMUD/internal/entities"

// holderEntityIDForPlayer adapts a session player id to the
// HolderID field on inventory/equipment events.
//
// **Namespace caveat.** Player ids and entity ids are different
// namespaces today: PlayerID comes from the player save (a uuid or
// account-character handle), and entities.EntityID is the runtime
// store key produced by entities.Store.Spawn. The cast here is a
// structural string-to-string move, not a semantic claim that the
// player IS a tracked entity. Subscribers MUST NOT pass HolderID
// into entities.Store.GetByID for a player-holder event — the
// lookup will always miss because players aren't registered there.
//
// This gets resolved properly when players-as-entities lands
// (likely M6, when mobs become holders too and the bus needs a
// uniform "who did this" type). At that point this helper either
// becomes a real lookup or the events grow a separate PlayerID
// field. Centralizing the cast here means the future fix is
// grep-able to one site.
func holderEntityIDForPlayer(playerID string) entities.EntityID {
	return entities.EntityID(playerID)
}

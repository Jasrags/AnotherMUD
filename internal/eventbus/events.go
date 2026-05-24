package eventbus

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Event-name constants. Spec text uses spaces ("entity equipped");
// the bus uses dots ("entity.equipped") because identifiers carry
// better through code. The mapping is one-to-one and lives here.
const (
	EventItemPickedUp    = "entity.item_picked_up"
	EventItemDropped     = "entity.item_dropped"
	EventEntityEquipped  = "entity.equipped"
	EventEntityUnequipped = "entity.unequipped"
)

// ItemPickedUp fires after GetHandler successfully moves an item
// from a room into a holder's contents (spec
// inventory-equipment-items §4.2 → "entity item picked up").
//
// Payload reflects the post-state: the item is now in HolderID's
// contents, no longer in RoomID's Placement entries.
type ItemPickedUp struct {
	HolderID entities.EntityID
	RoomID   world.RoomID
	ItemID   entities.EntityID
}

// Name implements Event.
func (ItemPickedUp) Name() string { return EventItemPickedUp }

// ItemDropped fires after DropHandler successfully moves an item
// from a holder's contents into a room (spec §4.3 → "entity item
// dropped"). Payload reflects the post-state: the item is in
// RoomID's Placement entries, no longer in HolderID's contents.
type ItemDropped struct {
	HolderID entities.EntityID
	RoomID   world.RoomID
	ItemID   entities.EntityID
}

// Name implements Event.
func (ItemDropped) Name() string { return EventItemDropped }

// EntityEquipped fires after EquipHandler successfully places an
// item in a slot (spec §3.3 step 7 → "entity equipped"). SlotName
// is the BASE slot name (no `:index` suffix) per §3.3 — the index
// is an internal disambiguator, not user-facing.
type EntityEquipped struct {
	HolderID entities.EntityID
	RoomID   world.RoomID
	ItemID   entities.EntityID
	SlotName string
}

// Name implements Event.
func (EntityEquipped) Name() string { return EventEntityEquipped }

// EntityUnequipped fires after UnequipHandler successfully removes
// an item from a slot (spec §3.4 step 4 → "entity unequipped").
// SlotName carries the base name only, matching §3.4's requirement
// that listeners see the base slot, never the index.
//
// The §3.4 `silent` mode (used by cleanup paths like dying entity
// drops everything) suppresses this event at the publisher's
// discretion. The bus has no notion of silent — that's a
// publisher-side choice.
type EntityUnequipped struct {
	HolderID entities.EntityID
	RoomID   world.RoomID
	ItemID   entities.EntityID
	SlotName string
}

// Name implements Event.
func (EntityUnequipped) Name() string { return EventEntityUnequipped }

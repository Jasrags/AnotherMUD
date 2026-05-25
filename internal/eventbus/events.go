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
	EventItemGiven       = "entity.item_given"
	// Cancellable pre-event fired before a put-in-container commits.
	// Spec inventory-equipment-items §4.5 step 5 — listeners can flip
	// the cancel flag to veto (locks, quest gates, etc.).
	EventContainerItemAdding = "container.item_adding"
	// Post-fact notification fired after a successful put commits.
	// Spec §4.5 step 7. Payload mirrors the pre-event.
	EventContainerItemAdded = "container.item_added"
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

// ItemGiven fires after GiveHandler successfully moves an item from
// one holder's contents into another's (spec inventory-equipment-
// items §4.4 step 4 → "entity item given"). Payload reflects the
// post-state: ItemID is in RecipientID's contents, no longer in
// GiverID's. RoomID is where the transfer happened (both holders
// must be in the same room). TemplateID carries the originating
// template so a future loot / quest listener can match without
// having to round-trip back to the entity store.
type ItemGiven struct {
	GiverID     entities.EntityID
	RecipientID entities.EntityID
	RoomID      world.RoomID
	ItemID      entities.EntityID
	ItemName    string
	TemplateID  string
}

// Name implements Event.
func (ItemGiven) Name() string { return EventItemGiven }

// ContainerItemAdding is the cancellable pre-event fired by
// PutHandler before the actor → container transfer commits (spec
// inventory-equipment-items §4.5 step 5). Listeners that flip the
// embedded CancelFlag abort the operation; the handler then returns
// the "cancelled" failure reason and emits no post-event.
//
// Payload reflects the *intended* post-state: ItemID is about to be
// placed inside ContainerID by ActorID. RoomID is the room where the
// put is happening (the actor's current room — useful for
// room-scoped quest listeners).
//
// The CancelFlag is a pointer so siblings later in the dispatch loop
// can observe an earlier listener's veto (per §dispatch semantics in
// internal/eventbus/event.go).
type ContainerItemAdding struct {
	*CancelFlag
	ActorID     entities.EntityID
	ContainerID entities.EntityID
	ItemID      entities.EntityID
	RoomID      world.RoomID
}

// Name implements Event.
func (ContainerItemAdding) Name() string { return EventContainerItemAdding }

// NewContainerItemAdding wires up the cancel flag so the publisher
// (PutHandler) does not have to remember to allocate it. Idiomatic
// constructor mirroring how cancellable events should be built —
// passing a zero-value struct would yield a nil CancelFlag and panic
// the moment a listener calls Cancel().
func NewContainerItemAdding(actor, container, item entities.EntityID, room world.RoomID) *ContainerItemAdding {
	return &ContainerItemAdding{
		CancelFlag:  &CancelFlag{},
		ActorID:     actor,
		ContainerID: container,
		ItemID:      item,
		RoomID:      room,
	}
}

// ContainerItemAdded fires after a successful put-in-container
// commits (spec §4.5 step 7). Post-state: ItemID is in ContainerID's
// contents, no longer in ActorID's inventory.
type ContainerItemAdded struct {
	ActorID     entities.EntityID
	ContainerID entities.EntityID
	ItemID      entities.EntityID
	RoomID      world.RoomID
}

// Name implements Event.
func (ContainerItemAdded) Name() string { return EventContainerItemAdded }

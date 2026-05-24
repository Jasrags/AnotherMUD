// Package entities owns the runtime entity store and the tag-index
// double-buffer required by world-rooms-movement §4. Templates and
// other content registries live in internal/item (etc.); this package
// is where instantiated things live, are tracked, and are queried by
// id / tag / type.
//
// M5.2 scope: ItemInstance is the only Entity implementation. Mobs
// (MobInstance) plug in during M6 as another Entity.
//
// The Store is the third lock holder in the engine (alongside
// world.World, which is read-only post-boot, and session.Manager).
// State is disjoint from the other two; the three locks do not nest.
package entities

// EntityID is the runtime identity of a tracked entity. Distinct from
// content ids (RoomID, item.TemplateID). Per inventory-equipment-items
// §2.3, instance ids are freshly assigned and unique within a run.
type EntityID string

// Entity is anything the Store tracks: items now, mobs later, possibly
// players in a later milestone. The interface is deliberately tiny —
// just what the indices need — so concrete types stay free to expose
// their own richer API.
//
// Tags() MUST return a fresh slice the caller is free to read or
// mutate; the Store indexes by tag and cannot tolerate aliasing
// surprises if a caller later writes to the returned slice. The cost
// of the copy is acceptable: Tags() is only consulted at Track/Untrack
// time, never on the query hot path.
type Entity interface {
	ID() EntityID
	Type() string
	Tags() []string
}

package command

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// M17.2d — the production ResolveContext adapter.
//
// The M17.2b/c resolvers are decoupled from concrete entity types
// behind the ItemCandidate / EntityCandidate / ContainerCandidate /
// DoorScope interfaces. This file is the one place that bridges those
// interfaces to the live runtime: *entities.ItemInstance,
// *entities.MobInstance, and *world.World. Keeping the adapters here
// means argresolve.go / argresolve_entity.go never import entities or
// world.

// itemCandidate adapts *entities.ItemInstance to ItemCandidate (and,
// because every ItemInstance can answer IsContainer, to
// ContainerCandidate). The instance's native ID() / TemplateID()
// return typed values; the resolver contract wants plain strings, so
// the adapter narrows them.
type itemCandidate struct{ inst *entities.ItemInstance }

func (a itemCandidate) Name() string       { return a.inst.Name() }
func (a itemCandidate) Keywords() []string { return a.inst.Keywords() }
func (a itemCandidate) EntityID() string   { return string(a.inst.ID()) }
func (a itemCandidate) TemplateID() string { return string(a.inst.TemplateID()) }

// IsContainer mirrors the put pipeline's §4.5 step-1 test: an item is
// a container iff its template type is the container type. Keeping the
// definition identical to PutHandler's check means the `container`
// arg type and the hand-rolled put verb agree on what counts.
func (a itemCandidate) IsContainer() bool { return a.inst.Type() == itemTypeContainer }

// mobCandidate adapts *entities.MobInstance to EntityCandidate. Every
// Placement-tracked entity the room scope surfaces as an entity is a
// mob today (players reach the room through the Locator, which cannot
// yet enumerate a room — see BuildResolveContext), so EntityType is
// the constant entityTypeMob.
type mobCandidate struct{ inst *entities.MobInstance }

func (a mobCandidate) Name() string       { return a.inst.Name() }
func (a mobCandidate) Keywords() []string { return a.inst.Keywords() }
func (a mobCandidate) EntityID() string   { return a.inst.EntityID() }
func (a mobCandidate) EntityType() string { return entityTypeMob }

// worldDoorScope adapts *world.World + the actor's room to the
// DoorScope the door resolver consults. It mirrors the M15.1 door
// verbs' resolution chain (world.ResolveDoorTarget → world.GetDoor)
// so the arg-typing path and the hand-rolled open/close/lock/unlock
// verbs agree on which door a token names.
//
// Single-token contract (resolves the M17.2c deferral): the door
// resolver passes one token, so a multi-word door phrase like
// "iron gate" resolves via its first matching keyword token, exactly
// as item args do. Directions ("n" / "north") and single-keyword
// doors — the real content shapes — round-trip unchanged. A future
// multi-word door arg would need the driver to slurp tokens and
// report a larger Consumed count; deferred until content needs it.
type worldDoorScope struct {
	world  *world.World
	roomID world.RoomID
}

func (s worldDoorScope) ResolveDoor(arg string) (DoorRef, bool, bool) {
	res := s.world.ResolveDoorTarget(s.roomID, arg)
	if res.Ambiguous {
		return DoorRef{}, false, true
	}
	if !res.Ok {
		return DoorRef{}, false, false
	}
	door, ok := s.world.GetDoor(s.roomID, res.Direction)
	if !ok {
		return DoorRef{}, false, false
	}
	return DoorRef{
		Direction: res.Direction.Short(),
		Door: DoorInfo{
			Name:   door.Name,
			Closed: door.Closed,
			Locked: door.Locked,
			KeyID:  door.KeyID,
		},
	}, true, false
}

// BuildResolveContext assembles the M17.2b/c ResolveContext from the
// live handler Context: the actor's carried items, the non-actor items
// and mobs in the current room, a door scope over the world, and the
// actor's identity for the `visible` self tag.
//
// Nil-safe for the test/bootstrap paths that pass a partial Context: a
// nil Actor returns the zero ResolveContext; a nil Items store or nil
// room leaves the item/entity scopes empty; a nil World leaves the
// DoorScope nil (the door resolver then returns ErrNoSuchDoor).
//
// KNOWN GAP: room PLAYERS are not enumerated. The Locator surface is
// name-only (FindInRoom) and cannot list a room's players for keyword
// matching, so RoomEntities carries mobs only. The entity / player /
// visible resolvers therefore won't surface other players in
// production until the Locator gains a room-enumeration method
// (tracked for M17.2d₂). This matches today's mob-first / player-by-
// exact-name asymmetry in findCombatantInRoom.
func (c *Context) BuildResolveContext() ResolveContext {
	if c.Actor == nil {
		return ResolveContext{}
	}

	rc := ResolveContext{
		ActorName: c.Actor.Name(),
		ActorID:   c.Actor.PlayerID(),
	}

	// Inventory scope: the actor's carried items.
	if c.Items != nil {
		for _, inst := range collectItems(c.Items, c.Actor.Inventory()) {
			rc.Inventory = append(rc.Inventory, itemCandidate{inst})
		}
	}

	// Room scopes: items and mobs placed in the current room. A single
	// Placement pass splits the two by concrete type.
	room := c.Actor.Room()
	if room != nil && c.Items != nil && c.Placement != nil {
		for _, id := range c.Placement.InRoom(room.ID) {
			e, ok := c.Items.GetByID(id)
			if !ok {
				continue
			}
			switch inst := e.(type) {
			case *entities.ItemInstance:
				rc.RoomItems = append(rc.RoomItems, itemCandidate{inst})
			case *entities.MobInstance:
				rc.RoomEntities = append(rc.RoomEntities, mobCandidate{inst})
			}
		}
	}

	// Door scope: a lookup over the world graph from the actor's room.
	if c.World != nil && room != nil {
		rc.Doors = worldDoorScope{world: c.World, roomID: room.ID}
	}

	return rc
}

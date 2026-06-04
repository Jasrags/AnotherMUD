package command

import (
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/quest"
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

// playerCandidate adapts a command.Actor (another player in the room)
// to EntityCandidate. Players carry no authored keyword list, so
// Keywords is nil and keyword.Resolve falls through to name-substring
// matching — which is what makes players keyword/partial-matchable
// ("give sword al" → Alice), the M17.2d₄ behavior change. EntityID is
// the stable PlayerID so handlers can re-fetch the live actor.
type playerCandidate struct{ actor Actor }

func (a playerCandidate) Name() string       { return a.actor.Name() }
func (a playerCandidate) Keywords() []string { return nil }
func (a playerCandidate) EntityID() string   { return a.actor.PlayerID() }
func (a playerCandidate) EntityType() string { return entityTypePlayer }

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

// EnumerateDoors lists every door reachable from the actor's room for
// completion (tab-completion §4), satisfying the optional doorEnumerator
// capability. One DoorRef per exit that carries a door, ordered by the
// direction short string so the result is deterministic (§7) despite the
// map iteration underneath. Mirrors ResolveDoor's DoorRef shape so a
// completed direction round-trips through the resolver unchanged.
func (s worldDoorScope) EnumerateDoors() []DoorRef {
	if s.world == nil {
		return nil
	}
	// RoomDoors snapshots each DoorState by value under the world's read
	// lock, so reading Closed/Locked here can't race a concurrent door
	// mutation (the bug a direct room.Exits read would have).
	doors := s.world.RoomDoors(s.roomID)
	out := make([]DoorRef, 0, len(doors))
	for _, d := range doors {
		out = append(out, DoorRef{
			Direction: d.Direction.Short(),
			Door: DoorInfo{
				Name:     d.Door.Name,
				Closed:   d.Door.Closed,
				Locked:   d.Door.Locked,
				KeyID:    d.Door.KeyID,
				Keywords: append([]string(nil), d.Door.Keywords...),
			},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Direction < out[j].Direction })
	return out
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
// Room players (M17.2d₄): when the Locator supports enumeration,
// RoomEntities also carries the OTHER players in the room (the actor
// itself is excluded so entity/player can't target self — §5.2). Mobs
// are appended before players so a mob with an exact keyword still
// wins a tie over a player matched only by name-substring.
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
		// Equipped scope (ArgEquipped completion for `unequip`): the worn
		// items, resolved from the slot-key → entity-id equipment map.
		// Walked in sorted slot-key order so the candidate list (and any
		// ordinal token for duplicate-keyword worn items) is stable across
		// calls — mirrors unequip's own deterministic slot-key sort.
		eq := c.Actor.Equipment()
		if len(eq) > 0 {
			slotKeys := make([]string, 0, len(eq))
			for k := range eq {
				slotKeys = append(slotKeys, k)
			}
			sort.Strings(slotKeys)
			ids := make([]entities.EntityID, 0, len(eq))
			for _, k := range slotKeys {
				ids = append(ids, eq[k])
			}
			for _, inst := range collectItems(c.Items, ids) {
				rc.Equipped = append(rc.Equipped, itemCandidate{inst})
			}
		}
	}

	// Room scopes: items and mobs placed in the current room. A single
	// Placement pass splits the two by concrete type.
	room := c.Actor.Room()
	// giverTemplateIDs accumulates the template ids of room mobs so the
	// quest scope below can ask each for its offers (ArgQuest completion).
	// Collected in the same pass that builds RoomEntities — free.
	var giverTemplateIDs []string
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
				giverTemplateIDs = append(giverTemplateIDs, string(inst.TemplateID()))
			}
		}
	}

	// Player entities (appended after mobs so mobs win exact-keyword
	// ties). The actor itself is excluded — entity/player must not
	// target self (§5.2); self-reference is the handler's concern.
	if room != nil && c.Locator != nil {
		selfID := c.Actor.PlayerID()
		for _, p := range c.Locator.PlayersInRoom(room.ID) {
			if p == nil || (selfID != "" && p.PlayerID() == selfID) {
				continue
			}
			rc.RoomEntities = append(rc.RoomEntities, playerCandidate{p})
		}
	}

	// Door scope: a lookup over the world graph from the actor's room.
	if c.World != nil && room != nil {
		rc.Doors = worldDoorScope{world: c.World, roomID: room.ID}
	}

	// Quest scope (ArgQuest / ArgActiveQuest completion). Lazy like
	// Doors — the service calls run only when completeQuest enumerates,
	// so non-quest dispatches pay nothing beyond capturing the giver-id
	// slice already built above. Populated whenever quests are wired and
	// the actor is a quest.Player: the offers set (accept) needs room
	// givers, but the active set (abandon) does not, so this is NOT gated
	// on givers being present.
	if c.Quests != nil {
		if player, ok := c.Actor.(quest.Player); ok {
			rc.Quests = questScope{svc: c.Quests, player: player, givers: giverTemplateIDs}
		}
	}

	return rc
}

// questScope is the production QuestScope adapter. It asks the quest
// service for the room givers' offers (EnumerateAcceptable) and the
// actor's active quests (EnumerateActive). Lives here (not in
// argresolve_entity.go) so ResolveContext stays free of the quest import,
// mirroring how worldDoorScope keeps the world dependency out of the
// resolver types.
type questScope struct {
	svc    *quest.Service
	player quest.Player
	givers []string // room mob template ids (offers only)
}

// EnumerateAcceptable returns the offers the room's givers extend to the
// actor (accept). OffersFrom already filters to acceptable+eligible.
func (s questScope) EnumerateAcceptable() []QuestRef {
	if s.svc == nil || s.player == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []QuestRef
	for _, g := range s.givers {
		for _, o := range s.svc.OffersFrom(s.player, g) {
			if seen[o.QuestID] {
				continue
			}
			seen[o.QuestID] = true
			out = append(out, QuestRef{BareID: bareQuestID(o.QuestID), Name: o.Name})
		}
	}
	return out
}

// EnumerateActive returns the actor's active, abandonable quests
// (abandon). Non-abandonable quests are omitted because `abandon` refuses
// them — suggesting them would be a dead end. A nil Definition (orphaned
// active quest after a content edit) is treated as abandonable, matching
// AbandonHandler.
func (s questScope) EnumerateActive() []QuestRef {
	if s.svc == nil || s.player == nil {
		return nil
	}
	st := s.svc.Snapshot(s.player.EntityID())
	if st == nil {
		return nil
	}
	var out []QuestRef
	for i := range st.Active {
		qid := st.Active[i].QuestID
		def, _ := s.svc.Definition(qid)
		if def != nil && !def.Abandonable {
			continue
		}
		name := qid
		if def != nil && def.Name != "" {
			name = def.Name
		}
		out = append(out, QuestRef{BareID: bareQuestID(qid), Name: name})
	}
	return out
}

// bareQuestID strips the pack namespace from a quest id ("pack:gate-patrol"
// → "gate-patrol"). The bare id is the completion token because
// quest.Service.ResolveID matches it exactly (registry.go ResolveID).
func bareQuestID(id string) string {
	if i := strings.LastIndex(id, ":"); i >= 0 {
		return id[i+1:]
	}
	return id
}

package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Reserved property keys consulted by PutHandler (spec
// inventory-equipment-items §4.5 + container_capacity /
// container_weight_limit / public on the template).
const (
	propContainerCapacity = "container_capacity"
	propWeightLimit       = "container_weight_limit"
	propWeight            = "weight"
	propPublic            = "public"
	itemTypeContainer     = "container"
)

// PutHandler implements `put <item> [in[to]] <container>` (spec
// inventory-equipment-items §4.5).
//
// Validation order matches §4.5:
//  1. Resolve item against the actor's inventory.
//  2. Resolve container against accessible candidates per §4.5
//     step 2 (carried, room-placed top-level, or public).
//  3. Container check — target type MUST be "container".
//  4. Capacity check — non-positive limits are disabled.
//  5. Weight check — items with no `weight` contribute zero.
//  6. Cancellable pre-event `container.item_adding`.
//  7. Transfer (inventory.Remove → contents.Put).
//  8. Post-event `container.item_added`.
//
// Currency auto-convert (spec §4.1) is not consulted here — put is
// not a currency hook surface per §4.5. The drop / give paths are.
func PutHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Contents == nil {
		return c.Actor.Write(ctx, "You can't put anything anywhere right now.")
	}
	itemArg, containerArg, ok := parsePutArgs(c.Args)
	if !ok {
		return c.Actor.Write(ctx, "Put what in what?")
	}

	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You float in formless void.")
	}

	carried := collectItems(c.Items, c.Actor.Inventory())
	if len(carried) == 0 {
		return c.Actor.Write(ctx, "You aren't carrying anything.")
	}
	itemMatch := keyword.Resolve(asNamed(carried), itemArg)
	if itemMatch == nil {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	item := itemMatch.(*entities.ItemInstance)

	containerCandidates := accessibleContainers(c, room.ID)
	if len(containerCandidates) == 0 {
		return c.Actor.Write(ctx, "There is nothing here you could put that into.")
	}
	containerMatch := keyword.Resolve(asNamed(containerCandidates), containerArg)
	if containerMatch == nil {
		return c.Actor.Write(ctx, "You don't see that container here.")
	}
	container := containerMatch.(*entities.ItemInstance)

	// Reject putting an item into itself. The keyword resolver can
	// hit the same instance when the actor types `put sack sack`
	// and a carried sack is the only candidate on both sides.
	// Catching this before the type check yields a clearer message
	// than the generic "isn't a container".
	if container.ID() == item.ID() {
		return c.Actor.Write(ctx, "You can't put something inside itself.")
	}

	// §4.5 step 1 — type check.
	if container.Type() != itemTypeContainer {
		return c.Actor.Write(ctx, fmt.Sprintf("%s isn't a container.", container.Name()))
	}

	// §4.5 step 3 — capacity check. A non-positive declared cap
	// means unlimited (the property is absent or zero).
	current := c.Contents.In(container.ID())
	if cap := intProp(container, propContainerCapacity); cap > 0 && len(current) >= cap {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is full.", container.Name()))
	}

	// §4.5 step 4 — weight check. Items with no `weight` property
	// contribute zero per spec; a missing weight_limit disables the
	// check entirely.
	if limit := intProp(container, propWeightLimit); limit > 0 {
		incoming := intProp(item, propWeight)
		used := 0
		for _, childID := range current {
			if e, ok := c.Items.GetByID(childID); ok {
				if child, ok := e.(*entities.ItemInstance); ok {
					used += intProp(child, propWeight)
				}
			}
		}
		if used+incoming > limit {
			return c.Actor.Write(ctx, fmt.Sprintf("%s is too heavy to fit in %s.", item.Name(), container.Name()))
		}
	}

	// §4.5 step 5 — cancellable pre-event. Listeners can veto by
	// flipping the cancel flag (quest gates, locked containers).
	actorEID := holderEntityIDForPlayer(c.Actor.PlayerID())
	pre := eventbus.NewContainerItemAdding(actorEID, container.ID(), item.ID(), room.ID)
	if c.Bus != nil {
		if c.Bus.PublishCancellable(ctx, pre) {
			// Spec returns "cancelled" — the user-visible message is
			// intentionally generic so listeners can decide whether
			// to add their own (lock state, quest-gate) message.
			return c.Actor.Write(ctx, fmt.Sprintf("You can't put %s in %s right now.", item.Name(), container.Name()))
		}
	}

	// §4.5 step 6 — transfer. Order matters: Remove from inventory
	// FIRST (atomic ownership claim, same TOCTOU shape as
	// GetHandler / GiveHandler).
	if !c.Actor.RemoveFromInventory(item.ID()) {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	// Container TOCTOU re-check: between accessibleContainers and
	// here, another player in the room could `get` the container
	// (Placement.Remove succeeds) or someone could mutate Contents
	// to nest it. The item-side Remove is our claim on the item;
	// there is no analogous claim on the container because put
	// doesn't move it. Re-verify accessibility and roll back the
	// inventory remove if the container slipped away — better to
	// hand the actor's item back than to record a Contents entry
	// against a container that's now in another player's bag.
	if !stillAccessible(c, room.ID, container) {
		c.Actor.AddToInventory(item.ID())
		return c.Actor.Write(ctx, fmt.Sprintf("%s is no longer here.", container.Name()))
	}
	c.Contents.Put(container.ID(), item.ID())
	// RemoveFromInventory already re-synced the save tree, but at
	// that moment the item was nowhere — it had just left inventory
	// and not yet landed in the container. The Contents.Put above
	// is invisible to the inventory tree builder unless we ask the
	// actor to re-sync. Without this, the save persists an empty
	// container and the item is lost on logout.
	c.Actor.MarkContentsDirty()

	_ = c.Actor.Write(ctx, fmt.Sprintf("You put %s in %s.", item.Name(), container.Name()))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s puts %s in %s.", c.Actor.Name(), item.Name(), container.Name()),
			c.Actor.PlayerID())
	}
	c.Publish(ctx, eventbus.ContainerItemAdded{
		ActorID:     actorEID,
		ContainerID: container.ID(),
		ItemID:      item.ID(),
		RoomID:      room.ID,
	})
	return nil
}

// accessibleContainers returns the item instances reachable from the
// actor's current position that are valid `put` targets per §4.5
// step 2. The union of three sources, in this order:
//
//  1. Containers in the actor's own inventory ("my pouch").
//  2. Containers placed in the current room with no parent
//     container ("the chest on the floor"). Items currently nested
//     inside another container are excluded by §4.5's "no parent
//     container" clause.
//  3. Containers in the room with the `public` property set true
//     ("the town mailbox"). The spec phrases this as "anywhere
//     with public=true" but Placement is the only world-index we
//     consult; a truly cross-room public container would need a
//     separate registry, and the M5 content set doesn't ship one.
//
// Order matters only when the keyword resolver ties — earlier
// sources win. That biases toward the actor's own pouch, which is
// what players expect from `put gem sack`.
//
// De-duplication: an item could in theory match more than one
// source (e.g. a public container also placed in the room with no
// parent). The seen map collapses duplicates so the resolver
// doesn't see the same instance twice and break ordinal selection.
func accessibleContainers(c *Context, roomID world.RoomID) []*entities.ItemInstance {
	out := make([]*entities.ItemInstance, 0)
	seen := make(map[entities.EntityID]struct{})

	add := func(it *entities.ItemInstance) {
		if it == nil {
			return
		}
		if _, dup := seen[it.ID()]; dup {
			return
		}
		seen[it.ID()] = struct{}{}
		out = append(out, it)
	}

	// (1) carried containers
	for _, it := range collectItems(c.Items, c.Actor.Inventory()) {
		if it.Type() == itemTypeContainer {
			add(it)
		}
	}

	// (2) room-placed top-level containers + (3) public-in-room
	if c.Placement != nil {
		for _, id := range c.Placement.InRoom(roomID) {
			e, ok := c.Items.GetByID(id)
			if !ok {
				continue
			}
			it, ok := e.(*entities.ItemInstance)
			if !ok || it.Type() != itemTypeContainer {
				continue
			}
			// "No parent container" — Placement carries containers
			// the room owns directly. The contents-of-a-container
			// case is handled separately via c.Contents.ContainerOf,
			// but a container in Placement is by definition NOT
			// inside another container (the put step removes from
			// Placement before adding to Contents — see the
			// invariant in contents.go). The check is therefore
			// redundant in practice but cheap and a guardrail
			// against a future code path that violates it.
			if c.Contents != nil {
				if _, nested := c.Contents.ContainerOf(it.ID()); nested {
					continue
				}
			}
			add(it)
			// Public is implicitly satisfied by the same room-scan;
			// kept here as a comment so the §4.5 step-2 mapping is
			// visible in code.
			_ = propPublic
		}
	}

	return out
}

// stillAccessible re-checks at commit time that the container
// resolved by accessibleContainers is still reachable. Covers the
// race window between the candidate scan and Contents.Put:
//
//   - Carried: the container is still in the actor's inventory.
//     The single pump goroutine per session means this is only
//     observably false if a teardown path raced the put, which
//     would have torn the connection down anyway — but the check
//     costs nothing and protects against future cross-session
//     mutation paths (group inventory, mob trade).
//   - Room-placed: the container is still in Placement under the
//     actor's current room and not nested in any other container.
//     This is the common race: another player in the same room
//     calls `get` on the container between our scan and our Put.
//
// Returns true if either source still vouches for the container.
// A container in neither source is gone; the put should roll back.
func stillAccessible(c *Context, roomID world.RoomID, container *entities.ItemInstance) bool {
	if c.Contents != nil {
		// If something nested the container into another container
		// between our scan and now, treat it as gone — accessible
		// containers in §4.5 step 2 explicitly exclude "in another
		// container."
		if _, nested := c.Contents.ContainerOf(container.ID()); nested {
			return false
		}
	}
	for _, id := range c.Actor.Inventory() {
		if id == container.ID() {
			return true
		}
	}
	if c.Placement != nil {
		if r, ok := c.Placement.RoomOf(container.ID()); ok && r == roomID {
			return true
		}
	}
	return false
}

// parsePutArgs splits the verb's arguments into (itemArg,
// containerArg). Returns ok=false when fewer than two meaningful
// tokens are present, mirroring parseGiveArgs' shape.
//
// Forms accepted:
//
//	put gem sack          → item="gem",       container="sack"
//	put red gem in sack   → item="red gem",   container="sack"
//	put red gem into sack → item="red gem",   container="sack"
//
// The explicit preposition (in or into) wins when present;
// otherwise the last token is the container. Standalone "in" /
// "into" as the literal container (`put x in`) is treated as
// missing.
func parsePutArgs(args []string) (itemArg, containerArg string, ok bool) {
	if len(args) < 2 {
		return "", "", false
	}
	for i := len(args) - 1; i >= 1; i-- {
		w := strings.ToLower(args[i])
		if w != "in" && w != "into" {
			continue
		}
		if i == len(args)-1 {
			return "", "", false
		}
		itemArg = strings.Join(args[:i], " ")
		containerArg = strings.Join(args[i+1:], " ")
		if itemArg == "" || containerArg == "" {
			return "", "", false
		}
		return itemArg, containerArg, true
	}
	itemArg = strings.Join(args[:len(args)-1], " ")
	containerArg = args[len(args)-1]
	return itemArg, containerArg, true
}

// intProp returns the integer-typed property value at key on item's
// per-instance Properties bag, or 0 if absent / non-numeric. YAML
// decoding produces int / int64 / float64 depending on the literal;
// the switch normalizes all three.
func intProp(item *entities.ItemInstance, key string) int {
	v, ok := item.Properties()[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

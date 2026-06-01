package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Reserved property keys consulted by PutHandler (spec
// inventory-equipment-items §4.5 + container_capacity /
// container_weight_limit on the template).
const (
	propContainerCapacity = "container_capacity"
	propWeightLimit       = "container_weight_limit"
	propWeight            = "weight"
	itemTypeContainer     = "container"
)

// PutHandler implements `put <item> [in[to]] <container>` (spec
// inventory-equipment-items §4.5).
//
// Steps 1-3 (resolve the inventory item, resolve an accessible
// container, confirm it IS a container) are now handled by the §5
// arg pipeline before this runs: the `item` arg is ArgInventory and
// the `container` arg is ArgContainer (inventory-first then room,
// filtered to ContainerCandidate). The remaining §4.5 steps stay
// here, operating on the two re-fetched live instances:
//  4. Capacity check — non-positive limits are disabled.
//  5. Weight check — items with no `weight` contribute zero.
//  6. Cancellable pre-event `container.item_adding`.
//  7. Transfer (inventory.Remove → contents.Put), with a
//     stillAccessible TOCTOU re-check on the container.
//  8. Post-event `container.item_added`.
//
// Currency auto-convert (spec §4.1) is not consulted here — put is
// not a currency hook surface per §4.5. The drop / give paths are.
func PutHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Contents == nil {
		return c.Actor.Write(ctx, "You can't put anything anywhere right now.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You float in formless void.")
	}

	// M17.2d₃: the item (inventory) and container (inventory-first
	// then room) args are resolved by the §5 pipeline before this
	// runs; we re-fetch both live instances by id. The container
	// resolver only yields ContainerCandidate items, so §4.5 step 1's
	// "is it a container?" test is now an invariant of resolution and
	// the explicit Type() check is gone.
	item, ok := resolvedItemInstance(c, "item")
	if !ok {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	container, ok := resolvedItemInstance(c, "container")
	if !ok {
		return c.Actor.Write(ctx, "You don't see that container here.")
	}

	// Reject putting an item into itself. The resolver can hit the
	// same instance when the actor types `put sack sack` and a carried
	// sack matches both the inventory item arg and the container arg.
	if container.ID() == item.ID() {
		return c.Actor.Write(ctx, "You can't put something inside itself.")
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

// stillAccessible re-checks at commit time that the container the
// `container` arg resolved is still reachable. Covers the race window
// between resolution and Contents.Put:
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

// intProp returns the integer-typed property value at key on item's
// per-instance Properties bag, or 0 if absent / non-numeric. YAML
// decoding produces int / int64 / float64 depending on the literal;
// the switch normalizes all three.
func intProp(item *entities.ItemInstance, key string) int {
	v, ok := item.Property(key)
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

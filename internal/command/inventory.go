package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
)

// Reserved tags that gate `get` (spec inventory-equipment-items §4.2
// step 1). Items carrying either tag never leave a room via pick-up;
// fixture is for in-world dressing (signs, statues), no_get is for
// quest-bound objects that should appear in inventory listings only
// when granted by another mechanism.
const (
	tagFixture = "fixture"
	tagNoGet   = "no_get"
)

// GetHandler implements the `get <item>` verb (spec §4.2).
//
// Resolves the argument against items currently in the actor's room via
// the shared keyword resolver, validates the tag gate, moves the item
// from room → inventory, and broadcasts a single observable event.
//
// Failure messages are deliberately phrased so observers don't learn
// which fixture refused them (avoid information leak about world
// metadata that the player can't see).
func GetHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Placement == nil {
		// Sub-system not wired; fail closed rather than panic.
		return c.Actor.Write(ctx, "You can't pick anything up right now.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Get what?")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "There is nothing here.")
	}

	candidates := collectItems(c.Items, c.Placement.InRoom(room.ID))
	if len(candidates) == 0 {
		return c.Actor.Write(ctx, "There is nothing here to get.")
	}

	match := keyword.Resolve(asNamed(candidates), strings.Join(c.Args, " "))
	if match == nil {
		return c.Actor.Write(ctx, "You don't see that here.")
	}
	item := match.(*entities.ItemInstance)

	if hasAnyTag(item, tagFixture, tagNoGet) {
		return c.Actor.Write(ctx, fmt.Sprintf("You can't take %s.", item.Name()))
	}

	// Mutation order: drop placement first so concurrent `look` won't
	// see the item in both places. AddToInventory before broadcast so
	// the actor can immediately reference it.
	c.Placement.Remove(item.ID())
	c.Actor.AddToInventory(item.ID())

	_ = c.Actor.Write(ctx, fmt.Sprintf("You pick up %s.", item.Name()))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s picks up %s.", c.Actor.Name(), item.Name()),
			c.Actor.PlayerID())
	}
	return nil
}

// DropHandler implements the `drop <item>` verb (spec §4.3).
//
// Resolves against the actor's inventory, removes from contents, places
// in the current room, and broadcasts. Drop is unconditional in this
// spec (no weight gate, no no_drop tag) — gates would be policy layered
// on top.
func DropHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Placement == nil {
		return c.Actor.Write(ctx, "You can't drop anything right now.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Drop what?")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You float in formless void; there is nowhere to drop anything.")
	}

	candidates := collectItems(c.Items, c.Actor.Inventory())
	if len(candidates) == 0 {
		return c.Actor.Write(ctx, "You aren't carrying anything.")
	}

	match := keyword.Resolve(asNamed(candidates), strings.Join(c.Args, " "))
	if match == nil {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	item := match.(*entities.ItemInstance)

	if !c.Actor.RemoveFromInventory(item.ID()) {
		// Vanishingly rare: keyword match found it but the inventory
		// changed between Resolve and Remove. Treat as failure.
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	c.Placement.Place(item.ID(), room.ID)

	_ = c.Actor.Write(ctx, fmt.Sprintf("You drop %s.", item.Name()))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s drops %s.", c.Actor.Name(), item.Name()),
			c.Actor.PlayerID())
	}
	return nil
}

// collectItems resolves ids through store and filters to ItemInstances,
// preserving order. Unknown ids and non-item entities are skipped
// silently — they represent index corruption, not user-visible errors.
func collectItems(store *entities.Store, ids []entities.EntityID) []*entities.ItemInstance {
	out := make([]*entities.ItemInstance, 0, len(ids))
	for _, id := range ids {
		e, ok := store.GetByID(id)
		if !ok {
			continue
		}
		item, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func asNamed(items []*entities.ItemInstance) []keyword.Named {
	out := make([]keyword.Named, len(items))
	for i, it := range items {
		out[i] = it
	}
	return out
}

func hasAnyTag(item *entities.ItemInstance, tags ...string) bool {
	owned := item.Tags()
	for _, want := range tags {
		for _, have := range owned {
			if strings.EqualFold(have, want) {
				return true
			}
		}
	}
	return false
}

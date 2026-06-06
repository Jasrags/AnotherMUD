package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/light"
)

// Light-source verbs (light-and-darkness §3.1). `light <item>` ignites
// a source; `extinguish <item>` puts it out. Both resolve the item over
// the union of the actor's inventory and equipped slots — you can light
// a torch sitting in your pack or one already held in your light slot.
//
// Lit/unlit state lives on the item instance (PropItemLit), so it
// travels with the item across pickup/drop/give/store and is
// admin-settable. Lighting and extinguishing are ordinary command
// results, not bus events — only the fuel gutter-out case (§3.2)
// publishes an event, and that lives in the fuel loop.

// LightHandler implements `light <item>`.
func LightHandler(ctx context.Context, c *Context) error {
	return lightVerb(ctx, c, true)
}

// ExtinguishHandler implements `extinguish <item>`.
func ExtinguishHandler(ctx context.Context, c *Context) error {
	return lightVerb(ctx, c, false)
}

func lightVerb(ctx context.Context, c *Context, ignite bool) error {
	verb := "extinguish"
	if ignite {
		verb = "light"
	}
	if c.Items == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("You can't %s anything right now.", verb))
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s what?", capitalize(verb)))
	}

	item, ok := resolveCarriedOrEquipped(c, strings.Join(c.Args, " "))
	if !ok {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

	if !light.IsSource(item) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is not a light source.", capitalize(item.Name())))
	}

	already := light.IsLit(item)
	if ignite {
		if already {
			return c.Actor.Write(ctx, fmt.Sprintf("%s is already lit.", capitalize(item.Name())))
		}
		// A spent fuel-burning source (fuel present and zero) cannot be
		// relit. A permanent source (no fuel property) always lights.
		if fuel, ok := item.Property(light.PropItemFuel); ok {
			if n, _ := fuel.(int); n <= 0 {
				return c.Actor.Write(ctx, fmt.Sprintf("%s is spent; there is no fuel left.", capitalize(item.Name())))
			}
		}
		item.SetProperty(light.PropItemLit, true)
		_ = c.Actor.Write(ctx, fmt.Sprintf("You light %s.", item.Name()))
		broadcastLight(ctx, c, fmt.Sprintf("%s lights %s.", c.Actor.Name(), item.Name()))
		return nil
	}

	if !already {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is not lit.", capitalize(item.Name())))
	}
	item.SetProperty(light.PropItemLit, false)
	_ = c.Actor.Write(ctx, fmt.Sprintf("You extinguish %s.", item.Name()))
	broadcastLight(ctx, c, fmt.Sprintf("%s extinguishes %s.", c.Actor.Name(), item.Name()))
	return nil
}

// broadcastLight sends a lit/extinguish observation to the rest of the
// room, nil-guarding the broadcaster and a nameless actor.
func broadcastLight(ctx context.Context, c *Context, msg string) {
	room := c.Actor.Room()
	if c.Broadcaster == nil || room == nil || c.Actor.Name() == "" {
		return
	}
	c.Broadcaster.SendToRoom(ctx, room.ID, msg, c.Actor.PlayerID())
}

// resolveCarriedOrEquipped resolves a keyword phrase against the union
// of the actor's top-level inventory and currently-equipped items,
// inventory first (deterministic). Returns the live instance. Mirrors
// unequip's manual resolution but widened to include carried items so
// the light verbs work on a torch in the pack or in the light slot.
func resolveCarriedOrEquipped(c *Context, phrase string) (*entities.ItemInstance, bool) {
	seen := make(map[entities.EntityID]struct{})
	var cands []*entities.ItemInstance
	add := func(id entities.EntityID) {
		if _, dup := seen[id]; dup {
			return
		}
		if e, ok := c.Items.GetByID(id); ok {
			if it, ok := e.(*entities.ItemInstance); ok {
				seen[id] = struct{}{}
				cands = append(cands, it)
			}
		}
	}
	for _, id := range c.Actor.Inventory() {
		add(id)
	}
	for _, k := range sortedSlotKeys(c.Actor.Equipment()) {
		add(c.Actor.Equipment()[k])
	}
	if len(cands) == 0 {
		return nil, false
	}
	match := keyword.Resolve(asNamed(cands), phrase)
	if match == nil {
		return nil, false
	}
	it, ok := match.(*entities.ItemInstance)
	return it, ok
}

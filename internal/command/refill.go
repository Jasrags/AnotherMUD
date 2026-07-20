package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
)

// refill restocks a spent medkit from a carried box of medkit supplies
// (healing-detailed.md — a medkit "even if currently out of supplies" is
// still a medkit; supplies are what heal). It is the counterpart to treat:
// treat spends charges, refill replenishes them. `refill` with no argument
// picks a carried medkit that isn't full; `refill <kit>` names one. One box
// of supplies tops the kit back to its max and is consumed.

// propMedkitSupplies flags an item as a box of medkit refill supplies.
const propMedkitSupplies = "medkit_supplies"

// RefillHandler implements `refill [<kit>]`.
func RefillHandler(ctx context.Context, c *Context) error {
	if c.Items == nil {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}
	carried := collectItems(c.Items, c.Actor.Inventory())

	// Resolve the kit: a named item must be a medkit; otherwise pick the
	// first carried medkit that isn't already full (falling back to the first
	// kit so a full one still gets the "already stocked" message).
	var kit *entities.ItemInstance
	token := strings.TrimSpace(strings.Join(c.Args, " "))
	if token != "" {
		match := keyword.Resolve(asNamed(carried), token)
		if match == nil {
			return c.Actor.Write(ctx, "You aren't carrying that.")
		}
		it, _ := match.(*entities.ItemInstance) // carried are all ItemInstances
		if it == nil {
			return c.Actor.Write(ctx, "You aren't carrying that.")
		}
		if !isFirstAidKit(it) {
			return c.Actor.Write(ctx, fmt.Sprintf("%s isn't a medkit.", capitalize(it.Name())))
		}
		kit = it
	} else {
		var firstKit *entities.ItemInstance
		for _, it := range carried {
			if !isFirstAidKit(it) {
				continue
			}
			if firstKit == nil {
				firstKit = it
			}
			if intProp(it, economy.PropCharges) < medkitMax(it) {
				kit = it
				break
			}
		}
		if kit == nil {
			kit = firstKit
		}
		if kit == nil {
			return c.Actor.Write(ctx, "You have no medkit to refill.")
		}
	}

	max := medkitMax(kit)
	if max <= 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s can't be refilled.", capitalize(kit.Name())))
	}
	if intProp(kit, economy.PropCharges) >= max {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is already fully stocked.", capitalize(kit.Name())))
	}

	supplies, ok := findMedkitSupplies(c)
	if !ok {
		return c.Actor.Write(ctx, "You have no medkit supplies to refill it with.")
	}

	// Top the kit back to its max and consume one box of supplies (single-use).
	kit.SetProperty(economy.PropCharges, max)
	c.Actor.RemoveFromInventory(supplies.ID())
	_ = c.Items.Untrack(supplies.ID())

	return c.Actor.Write(ctx, fmt.Sprintf("You restock %s with fresh supplies. (%d/%d)", kit.Name(), max, max))
}

// medkitMax reads a kit's max_charges (the refill ceiling); 0 when the kit
// declares none — a kit with no max can't be refilled.
func medkitMax(it *entities.ItemInstance) int {
	return intProp(it, economy.PropMaxCharges)
}

// findMedkitSupplies returns a carried box of medkit supplies, if any.
func findMedkitSupplies(c *Context) (*entities.ItemInstance, bool) {
	for _, it := range collectItems(c.Items, c.Actor.Inventory()) {
		if v, ok := it.Property(propMedkitSupplies); ok {
			if b, _ := v.(bool); b {
				return it, true
			}
		}
	}
	return nil, false
}

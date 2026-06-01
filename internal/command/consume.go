package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/economy"
)

// Consume verbs (spec economy-survival §6). `eat`, `drink`, and `use`
// each resolve an item by keyword from the actor's TOP-LEVEL inventory
// (nested-in-container items are not consumable, §6.5) and route it
// through the ConsumableService with the verb's method as the gate
// (§6.1 — consume_method decides which command can consume an item).

// EatHandler implements `eat <item>` (consume_method "eat").
func EatHandler(ctx context.Context, c *Context) error {
	return consumeVerb(ctx, c, "eat")
}

// DrinkHandler implements `drink <item>` (consume_method "drink").
func DrinkHandler(ctx context.Context, c *Context) error {
	return consumeVerb(ctx, c, "drink")
}

// UseHandler implements `use <item>` (consume_method "use").
func UseHandler(ctx context.Context, c *Context) error {
	return consumeVerb(ctx, c, "use")
}

func consumeVerb(ctx context.Context, c *Context, method string) error {
	if c.Consumable == nil || c.Items == nil {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}

	consumer, ok := c.Actor.(economy.Consumer)
	if !ok {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}

	// M17.2d: the `item` arg (ArgInventory) resolves against the
	// actor's TOP-LEVEL inventory before this runs — nested-in-
	// container items are excluded by §6.5 because BuildResolveContext
	// builds the inventory scope from Actor.Inventory() only. Re-fetch
	// the live instance by the resolved id.
	item, ok := resolvedItemInstance(c, "item")
	if !ok {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

	actorID := holderEntityIDForPlayer(c.Actor.PlayerID())
	res := c.Consumable.Consume(ctx, consumer, actorID, item.ID(), method)
	switch res.Outcome {
	case economy.ConsumeOK:
		return c.Actor.Write(ctx, fmt.Sprintf("You %s %s.", method, res.ItemName))
	case economy.ConsumeWrongMethod:
		return c.Actor.Write(ctx, fmt.Sprintf("You can't %s %s.", method, item.Name()))
	case economy.ConsumeNoCharges:
		return c.Actor.Write(ctx, fmt.Sprintf("%s is empty.", capitalize(item.Name())))
	case economy.ConsumeCancelled:
		return c.Actor.Write(ctx, "Something stops you.")
	default:
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
}

// capitalize upper-cases the first byte of a short ASCII string (verb
// prompts, item names at sentence start).
func capitalize(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}

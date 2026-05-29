package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
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
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s what?", capitalize(method)))
	}

	consumer, ok := c.Actor.(economy.Consumer)
	if !ok {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}

	carried := collectItems(c.Items, c.Actor.Inventory())
	if len(carried) == 0 {
		return c.Actor.Write(ctx, "You aren't carrying anything.")
	}
	match := keyword.Resolve(asNamed(carried), strings.Join(c.Args, " "))
	if match == nil {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	item := match.(*entities.ItemInstance)

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

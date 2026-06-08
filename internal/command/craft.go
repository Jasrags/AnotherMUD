package command

import (
	"context"
	"sort"
	"strings"
)

// CraftHandler implements `craft [<recipe>]` (crafting-and-cooking §3).
// With no argument it lists the recipes the player knows; with one it
// resolves a known recipe and routes to the crafting service, which
// atomically consumes the inputs and produces the quality-rolled output.
func CraftHandler(ctx context.Context, c *Context) error {
	if c.Craft == nil {
		return c.Actor.Write(ctx, "Crafting is not enabled in this build.")
	}

	entityID := c.Actor.PlayerID()
	if entityID == "" {
		entityID = c.Actor.ID()
	}

	// No-arg form: list what the player can make.
	if len(c.Args) == 0 {
		names := c.Craft.KnownRecipeNames(entityID)
		if len(names) == 0 {
			return c.Actor.Write(ctx, "You don't know any recipes. Find a trainer to learn a craft.")
		}
		sort.Strings(names)
		return c.Actor.Write(ctx, "You know how to craft:\n  "+strings.Join(names, "\n  "))
	}

	query := strings.Join(c.Args, " ")
	res := c.Craft.Craft(ctx, c.Actor, query)
	return c.Actor.Write(ctx, res.Message)
}

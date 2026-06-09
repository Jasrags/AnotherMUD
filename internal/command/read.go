package command

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// ReadHandler implements `read <item>` (crafting-and-cooking §7). The first
// thing worth reading is a recipe scroll/page: an ordinary inventory item
// carrying a `recipe` property (recipe.PropRecipeID) that names a recipe id.
// Reading it teaches that recipe (the breadth gate, §1.2) and consumes the
// scroll. Policy:
//   - no recipe property        → nothing to learn, scroll kept
//   - recipe absent from content → make-no-sense, scroll kept (§9)
//   - already known             → refused, scroll KEPT (resell/gift it)
//   - otherwise                 → learn, then consume the scroll
//
// The scroll is a plain item, so common (shop-sold) and rare (looted)
// recipes are the same mechanism placed differently by content (§7).
func ReadHandler(ctx context.Context, c *Context) error {
	if c.Known == nil || c.Recipes == nil || c.Items == nil {
		return c.Actor.Write(ctx, "You can't read anything right now.")
	}

	// The `item` arg (ArgInventory) pre-resolves against the actor's
	// top-level inventory; re-fetch the live instance by the resolved id.
	it, ok := resolvedItemInstance(c, "item")
	if !ok {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

	recipeID, ok := recipeIDProperty(it)
	if !ok {
		return c.Actor.Write(ctx, "There's nothing to learn from "+it.Name()+".")
	}

	rec, err := c.Recipes.Get(recipeID)
	if err != nil || rec == nil {
		// The scroll names a recipe no longer in content (§9). Keep it —
		// don't destroy a scroll for a recipe the player can't gain.
		return c.Actor.Write(ctx, "The instructions on "+it.Name()+" make no sense to you.")
	}

	entityID := c.Actor.PlayerID()
	if entityID == "" {
		entityID = c.Actor.ID()
	}

	// Already known → refuse and KEEP the scroll (chosen policy: a known
	// recipe wastes nothing; the page stays sellable/giftable).
	if c.Known.Knows(entityID, recipeID) {
		return c.Actor.Write(ctx, "You already know how to "+rec.DisplayName+".")
	}

	// Consume the scroll as the claim, THEN learn — the same claim-first
	// discipline as the get/drop/craft paths. RemoveFromInventory is the
	// single winner: if it returns false (a concurrent path already took the
	// scroll) we neither learn nor untrack, so there is no learn-for-free.
	// On success nothing after it can fail (Learn only no-ops on
	// already-known, ruled out above), so the recipe is never lost.
	if !c.Actor.RemoveFromInventory(it.ID()) {
		return c.Actor.Write(ctx, "You no longer have that.")
	}
	_ = c.Items.Untrack(it.ID())
	c.Known.Learn(entityID, recipeID)
	return c.Actor.Write(ctx, "You study "+it.Name()+" and learn how to "+rec.DisplayName+".")
}

// recipeIDProperty reads the recipe id a scroll teaches off its
// PropRecipeID property, trimming and rejecting non-string/empty values.
func recipeIDProperty(it *entities.ItemInstance) (recipe.RecipeID, bool) {
	v, ok := it.Property(recipe.PropRecipeID)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	if s = strings.TrimSpace(s); s == "" {
		return "", false
	}
	return recipe.RecipeID(s), true
}

package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

const (
	// tagCurrency marks an item whose pickup auto-converts to gold
	// (spec economy-survival §2.3 step 2).
	tagCurrency = "currency"
	// propValue is the item property holding the gold value the
	// auto-convert hook credits (§2.3 step 3).
	propValue = "value"
)

// GoldHandler implements the `gold` verb (spec economy-survival §2.2
// Read). Reports the actor's current balance. The actor is a
// currency holder iff it satisfies economy.Entity (the connActor
// does; test stubs that don't carry gold report zero via the
// fall-through).
func GoldHandler(ctx context.Context, c *Context) error {
	holder, ok := c.Actor.(economy.Entity)
	if !ok {
		return c.Actor.Write(ctx, "You have no gold.")
	}
	// Prefer the service read (honors any future caching/derivation
	// layer); fall back to the direct read only when currency is
	// unwired (tests). Avoids a discarded lock round-trip in prod
	// where c.Currency is always set.
	amount := holder.Gold()
	if c.Currency != nil {
		amount = c.Currency.Read(holder)
	}
	if amount == 0 {
		return c.Actor.Write(ctx, "You have no gold.")
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You have %d gold.", amount))
}

// tryAutoConvert implements the spec §2.3 pickup/give auto-convert
// hook (referenced by inventory-equipment-items §4.1). When dest is
// a player (satisfies economy.Entity) and it is a `currency`-tagged
// item with a positive `value`, the value is credited as gold, the
// item is untracked from the world, and (value, true) is returned so
// the caller skips adding the item to inventory and suppresses its
// own pickup/give event (§2.3 step 7). Otherwise returns (0, false)
// and the caller proceeds normally.
//
// Precondition: the item has already been claimed out of its prior
// location (off the floor for get, out of the giver's inventory for
// give) but not yet placed into dest's inventory. The hook's job is
// to redirect that placement into the gold account.
func tryAutoConvert(ctx context.Context, c *Context, dest Actor, it *entities.ItemInstance) (int, bool) {
	if c.Currency == nil || c.Items == nil {
		return 0, false
	}
	holder, ok := dest.(economy.Entity)
	if !ok {
		// Destination has no gold account (§2.3 step 1 — only
		// players auto-convert). Not a player → fall through.
		return 0, false
	}
	if !hasAnyTag(it, tagCurrency) {
		return 0, false
	}
	value := intProp(it, propValue)
	if value <= 0 {
		return 0, false
	}
	// Coins become abstract gold: untrack the item entity from the
	// world (§2.3 steps 4-5) and credit the holder (step 6). Reason
	// records the source template per spec.
	_ = c.Items.Untrack(it.ID())
	c.Currency.AddGold(ctx, holder, value, "pickup:"+string(it.TemplateID()))
	return value, true
}

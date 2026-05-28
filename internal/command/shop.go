package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// shopProp is the mob property key carrying the shop's config block
// (spec economy-survival §3.1).
const shopProp = "shop"

// BuyHandler implements `buy <item>` (spec §3.5). Finds the shop in
// the room, resolves the item against its stock, and runs the
// purchase through the shop service.
func BuyHandler(ctx context.Context, c *Context) error {
	shopper, npc, cfg, ok := shopContext(ctx, c)
	if !ok {
		return nil
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Buy what?")
	}
	res := c.Shop.Buy(ctx, shopper, string(npc.ID()), cfg, strings.Join(c.Args, " "))
	switch res.Outcome {
	case economy.ShopOK:
		return c.Actor.Write(ctx, fmt.Sprintf("You buy %s for %d gold. You have %d gold left.", res.ItemName, res.Price, res.Gold))
	case economy.ShopInsufficientGold:
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %d gold; you only have %d.", res.ItemName, res.Price, res.Gold))
	default:
		return c.Actor.Write(ctx, "The shop doesn't sell that.")
	}
}

// SellHandler implements `sell <item>` (spec §3.6).
func SellHandler(ctx context.Context, c *Context) error {
	shopper, npc, cfg, ok := shopContext(ctx, c)
	if !ok {
		return nil
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Sell what?")
	}
	res := c.Shop.Sell(ctx, shopper, string(npc.ID()), cfg, strings.Join(c.Args, " "))
	switch res.Outcome {
	case economy.ShopOK:
		return c.Actor.Write(ctx, fmt.Sprintf("You sell %s for %d gold. You now have %d gold.", res.ItemName, res.Price, res.Gold))
	case economy.ShopItemIsNoSell:
		return c.Actor.Write(ctx, fmt.Sprintf("You can't sell %s here.", res.ItemName))
	case economy.ShopItemValueZero:
		return c.Actor.Write(ctx, fmt.Sprintf("The shop won't give you anything for %s.", res.ItemName))
	case economy.ShopItemNotInInventory:
		return c.Actor.Write(ctx, "You aren't carrying that.")
	default:
		return c.Actor.Write(ctx, "The shop refuses to buy that.")
	}
}

// ValueHandler implements `value <item>` (spec §3.9). Reports what an
// item is worth — the sell price for a held item, else the buy price
// for stock.
func ValueHandler(ctx context.Context, c *Context) error {
	shopper, _, cfg, ok := shopContext(ctx, c)
	if !ok {
		return nil
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Value what?")
	}
	res := c.Shop.Value(ctx, shopper, cfg, strings.Join(c.Args, " "))
	switch {
	case res.Outcome != economy.ShopOK:
		return c.Actor.Write(ctx, "The shop doesn't deal in that.")
	case res.Scope == economy.ScopeInventory:
		return c.Actor.Write(ctx, fmt.Sprintf("The shop would give you %d gold for %s.", res.Price, res.ItemName))
	default:
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %d gold here.", res.ItemName, res.Price))
	}
}

// ListHandler implements `list` (spec §3.4) — the shop's stock.
func ListHandler(ctx context.Context, c *Context) error {
	_, _, cfg, ok := shopContext(ctx, c)
	if !ok {
		return nil
	}
	rows := c.Shop.Listings(cfg)
	if len(rows) == 0 {
		return c.Actor.Write(ctx, "The shop has nothing for sale.")
	}
	var b strings.Builder
	b.WriteString("The shop offers:")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("\n  %s — %d gold", r.Name, r.BuyPrice))
	}
	return c.Actor.Write(ctx, b.String())
}

// shopContext resolves the common preconditions every shop verb needs:
// the service is wired, the actor is a Shopper, and a shop NPC is
// present in the room. On any miss it writes the appropriate message
// and returns ok=false so the caller can bail. cfg is parsed from the
// located NPC's properties.
func shopContext(ctx context.Context, c *Context) (economy.Shopper, *entities.MobInstance, economy.ShopConfig, bool) {
	if c.Shop == nil {
		_ = c.Actor.Write(ctx, "There is no shop here.")
		return nil, nil, economy.ShopConfig{}, false
	}
	shopper, ok := c.Actor.(economy.Shopper)
	if !ok {
		_ = c.Actor.Write(ctx, "You can't trade right now.")
		return nil, nil, economy.ShopConfig{}, false
	}
	room := c.Actor.Room()
	if room == nil {
		_ = c.Actor.Write(ctx, "There is no shop here.")
		return nil, nil, economy.ShopConfig{}, false
	}
	npc := findShopInRoom(c, room.ID)
	if npc == nil {
		_ = c.Actor.Write(ctx, "There is no shop here.")
		return nil, nil, economy.ShopConfig{}, false
	}
	return shopper, npc, shopConfigFromMob(npc), true
}

// findShopInRoom returns the first shop-tagged mob in roomID, or nil
// (spec §3.2 — "the first NPC carrying the shop tag"). Placement
// preserves insertion order, so "first" is deterministic.
func findShopInRoom(c *Context, roomID world.RoomID) *entities.MobInstance {
	if c.Items == nil || c.Placement == nil {
		return nil
	}
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		mob, ok := e.(*entities.MobInstance)
		if !ok {
			continue
		}
		if mobHasTag(mob, economy.TagShop) {
			return mob
		}
	}
	return nil
}

func mobHasTag(mob *entities.MobInstance, tag string) bool {
	for _, t := range mob.Tags() {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// shopConfigFromMob reads the shop config block off the mob's
// properties (spec §3.1). The pack loader normalizes nested YAML maps
// to map[string]any. Missing / malformed fields fall back to the
// global defaults (an empty Sells list yields an empty shop).
func shopConfigFromMob(mob *entities.MobInstance) economy.ShopConfig {
	raw, ok := mob.Property(shopProp)
	if !ok {
		return economy.ShopConfig{}
	}
	block, ok := raw.(map[string]any)
	if !ok {
		return economy.ShopConfig{}
	}
	return economy.ShopConfig{
		Sells:        stringSlice(block["sells"]),
		BuyMarkup:    floatProp(block["buy_markup"]),
		SellDiscount: floatProp(block["sell_discount"]),
	}
}

// stringSlice coerces a YAML list (decoded as []any) into []string,
// dropping non-string entries.
func stringSlice(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// floatProp coerces a numeric YAML scalar to float64, zero when
// absent / non-numeric (zero means "fall back to the global default").
func floatProp(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

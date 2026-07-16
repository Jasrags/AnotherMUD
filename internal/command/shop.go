package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/faction"
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
	res := c.Shop.Buy(ctx, shopper, string(npc.ID()), cfg, strings.Join(c.Args, " "), shopSkillChecker(c), shopStandingFunc(c))
	switch res.Outcome {
	case economy.ShopOK:
		return c.Actor.Write(ctx, fmt.Sprintf("You buy %s for %s. You have %s left.", res.ItemName, c.Money.Format64(res.Price), c.Money.Format(res.Gold)))
	case economy.ShopInsufficientGold:
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %s; you only have %s.", res.ItemName, c.Money.Format64(res.Price), c.Money.Format(res.Gold)))
	case economy.ShopSkillTooLow:
		return c.Actor.Write(ctx, fmt.Sprintf("%s requires %s skill %d before you can buy it.", capitalize(res.ItemName), res.RequiredSkill, res.RequiredLevel))
	case economy.ShopStandingTooLow:
		return c.Actor.Write(ctx, "The shopkeeper refuses to deal with the likes of you.")
	default:
		return c.Actor.Write(ctx, "The shop doesn't sell that.")
	}
}

// shopSkillChecker builds the §7 purchase-skill predicate for the buyer
// from the proficiency manager, or nil when proficiency isn't wired (no
// gating). The economy package stays free of the progression import — the
// command layer owns the proficiency lookup (mirrors craftStationTier).
func shopSkillChecker(c *Context) economy.SkillChecker {
	if c.Proficiency == nil {
		return nil
	}
	eid := c.Actor.PlayerID()
	if eid == "" {
		eid = c.Actor.ID()
	}
	return func(discipline string, level int) bool {
		have, _ := c.Proficiency.Proficiency(eid, discipline)
		return have >= level
	}
}

// shopStandingFunc builds the faction §6 standing resolver for the buyer from
// the faction manager, or nil when faction isn't wired / the actor carries no
// standing surface (no gate, no price scaling). The economy package stays free
// of the faction import — the command layer owns the lookup (mirrors
// shopSkillChecker). Returns ok=false for a faction not in content so the shop
// fails open on a content typo.
func shopStandingFunc(c *Context) economy.StandingFunc {
	if c.Faction == nil {
		return nil
	}
	fe, ok := c.Actor.(faction.Entity)
	if !ok {
		return nil
	}
	return func(factionID string) (int, bool) {
		def, ok := c.Faction.Registry().Get(factionID)
		if !ok {
			return 0, false
		}
		return c.Faction.Get(fe, def), true
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
	res := c.Shop.Sell(ctx, shopper, string(npc.ID()), cfg, strings.Join(c.Args, " "), shopStandingFunc(c))
	switch res.Outcome {
	case economy.ShopOK:
		return c.Actor.Write(ctx, fmt.Sprintf("You sell %s for %s. You now have %s.", res.ItemName, c.Money.Format64(res.Price), c.Money.Format(res.Gold)))
	case economy.ShopItemIsNoSell:
		return c.Actor.Write(ctx, fmt.Sprintf("You can't sell %s here.", res.ItemName))
	case economy.ShopItemValueZero:
		return c.Actor.Write(ctx, fmt.Sprintf("The shop won't give you anything for %s.", res.ItemName))
	case economy.ShopItemNotInInventory:
		return c.Actor.Write(ctx, "You aren't carrying that.")
	case economy.ShopStandingTooLow:
		return c.Actor.Write(ctx, "The shopkeeper refuses to deal with the likes of you.")
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
	res := c.Shop.Value(ctx, shopper, cfg, strings.Join(c.Args, " "), shopStandingFunc(c))
	switch {
	case res.Outcome != economy.ShopOK:
		return c.Actor.Write(ctx, "The shop doesn't deal in that.")
	case res.Scope == economy.ScopeInventory:
		return c.Actor.Write(ctx, fmt.Sprintf("The shop would give you %s for %s.", c.Money.Format64(res.Price), res.ItemName))
	default:
		return c.Actor.Write(ctx, fmt.Sprintf("%s costs %s here.", res.ItemName, c.Money.Format64(res.Price)))
	}
}

// ListHandler implements `list` (spec §3.4) — the shop's stock.
func ListHandler(ctx context.Context, c *Context) error {
	_, _, cfg, ok := shopContext(ctx, c)
	if !ok {
		return nil
	}
	rows := c.Shop.Listings(cfg, shopSkillChecker(c), shopStandingFunc(c))
	if len(rows) == 0 {
		return c.Actor.Write(ctx, "The shop has nothing for sale.")
	}
	var b strings.Builder
	b.WriteString("The shop offers:")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("\n  %s — %s", r.Name, c.Money.Format64(r.BuyPrice)))
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
		if c.questSpawnBlockedFrom(e) {
			continue // foreign quest spawn — not interactable (quest-spawns.md Phase 2)
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

// ShopConfigFromMob reads the shop config block off the mob's properties (spec
// §3.1), exported so the session layer can build the same config for the
// Char.Shop trade form (web-client-plan P3 Slice B+) without duplicating the
// property-parsing. The pack loader normalizes nested YAML maps to
// map[string]any; missing/malformed fields fall back to the global defaults (an
// empty Sells list yields an empty shop).
func ShopConfigFromMob(mob *entities.MobInstance) economy.ShopConfig {
	return shopConfigFromMob(mob)
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
		// Faction §6: affiliation + optional access floor + favored-customer
		// pricing. The faction id is written fully-qualified in the block (the
		// property map is not namespace-resolved). min_standing is a pointer so
		// 0 is a real floor, distinct from "no gate".
		Faction:      blockString(block["faction"]),
		MinStanding:  blockIntPtr(block["min_standing"]),
		AllyStanding: blockInt(block["ally_standing"]),
		AllyDiscount: floatProp(block["ally_discount"]),
	}
}

// blockString coerces a shop-block value to a trimmed string, "" when absent /
// non-string. (Distinct from fill.go's instance-property stringProp.)
func blockString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// blockInt coerces a numeric shop-block scalar to int (int / int64 / float64),
// zero when absent / non-numeric.
func blockInt(v any) int {
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

// blockIntPtr coerces a numeric shop-block scalar to *int, nil when the key is
// absent (so a missing min_standing means "no access gate" while an explicit 0
// is a real floor).
func blockIntPtr(v any) *int {
	switch n := v.(type) {
	case int:
		return &n
	case int64:
		m := int(n)
		return &m
	case float64:
		m := int(n)
		return &m
	default:
		return nil
	}
}

// stringSlice coerces a list property into []string. yaml.v3 into a
// map[string]any always yields []any for sequences, but a typed
// decoder or an in-process config builder may hand over []string
// directly — accept both so a non-YAML caller can't silently empty
// the shop's stock. Non-string entries in the []any form are dropped.
func stringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
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

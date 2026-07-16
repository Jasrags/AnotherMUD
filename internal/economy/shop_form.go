package economy

import (
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// ShopOffer is one read-only buy- or sell-side row of a shop form
// (web-client-plan P3 Slice B+). It carries everything a rich client needs to
// render + submit a trade row without a round-trip. Prices are RAW (int64); the
// caller formats them through the world's CurrencyLabel — the economy package
// stays free of display vocabulary, exactly as the CLI shop verbs do.
type ShopOffer struct {
	// Name is the item's display name.
	Name string
	// Token is the keyword to buy/sell by — guaranteed to round-trip through
	// Buy/Sell (a stock row uses the template's short id, a sell row the item's
	// first keyword), so the client submits `buy <token>` / `sell <token>`.
	Token string
	// Price is the buy price (buy row) or the per-item sell price (sell row).
	Price int64
	// Qty is how many identical items the player carries (sell rows only); 0 on
	// buy rows and single sell rows (a client reads absent as 1).
	Qty int
	// Affordable is price <= the shopper's balance (buy rows). Always true on
	// sell rows (the shop pays the player).
	Affordable bool
}

// ShopForm is the read-only projection of the shop the player is standing at
// (web-client-plan P3 Slice B+) — the buy-side stock and the sell-side inventory
// with prices, mirroring what `list` + `value` show. Built without mutating.
type ShopForm struct {
	// Balance is the shopper's current money (raw; the caller formats it).
	Balance int64
	// Refused is true when the shop's faction access floor turns this shopper
	// away (faction.md §6): no trade at all, so Buy/Sell are empty.
	Refused bool
	// Buy is the shop's skill-passable stock (an item the buyer fails the §7
	// skill gate for is omitted, exactly as `list` omits it).
	Buy []ShopOffer
	// Sell is the player's carried sellable items, grouped by template with a
	// qty. No-sell and zero-value items are omitted (they can't be sold).
	Sell []ShopOffer
}

// ShopForm projects the shop into read-only buy/sell rows (web-client-plan P3
// Slice B+). It mirrors the gates the buy/sell verbs apply — faction access
// (§6), the §7 buy skill gate, no-sell/zero-value on the sell side — and prices
// through the same buyPrice/sellPrice helpers, so the panel matches what a
// player would get. check/standing are the buyer's skill + faction predicates
// (nil = ungated); a nil service/store returns a zero form.
func (s *ShopService) ShopForm(sh Shopper, shop ShopConfig, check SkillChecker, standing StandingFunc) ShopForm {
	if s == nil || s.tpls == nil || s.store == nil || s.currency == nil {
		return ShopForm{}
	}
	balance := int64(s.currency.Read(sh))
	// A hostile shopper is refused all trade (faction.md §6) — surface the refusal
	// with the balance but no offers, so the client can show "closed to you".
	if shop.refusesStanding(standing) {
		return ShopForm{Balance: balance, Refused: true, Buy: []ShopOffer{}, Sell: []ShopOffer{}}
	}
	return ShopForm{
		Balance: balance,
		Buy:     s.buyOffers(shop, check, standing, balance),
		Sell:    s.sellOffers(sh, shop, standing),
	}
}

// buyOffers builds the buy-side rows from the shop's sells list: skip unknown
// templates, non-positive value, and items the buyer fails the §7 skill gate for
// (meetsSkill) — the same drops `listings` makes — pricing each through buyPrice
// and marking affordability against the balance. Order follows shop.Sells.
func (s *ShopService) buyOffers(shop ShopConfig, check SkillChecker, standing StandingFunc, balance int64) []ShopOffer {
	out := make([]ShopOffer, 0, len(shop.Sells))
	for _, id := range shop.Sells {
		tpl, err := s.tpls.Get(item.TemplateID(id))
		if err != nil {
			continue
		}
		value := templateValue(tpl)
		if value <= 0 || !meetsSkill(tpl, check) {
			continue
		}
		price := buyPrice(value, shop, s.cfg, standing)
		out = append(out, ShopOffer{
			Name:       tpl.Name,
			Token:      shortID(string(tpl.ID)),
			Price:      price,
			Affordable: price <= balance,
		})
	}
	return out
}

// sellOffers builds the sell-side rows from the player's CARRIED items, grouped
// by template id (identical items collapse into one row with a qty, matching how
// a player thinks — "sell rounds"). No-sell and zero-value items are omitted (a
// `sell` on them is refused). Each row's per-item price runs through sellPrice
// with the shopper's standing. Order follows first appearance in inventory.
//
// Scope (deliberate): only carried items are listed, though the `sell` verb also
// accepts an EQUIPPED item (auto-unequipping it first, §3.6 step 6). Surfacing
// worn gear as one-click "sell" rows in a panel is an easy way to fumble away
// your armor, so the form stays carried-only; a player can still sell equipped
// gear by typing `sell <it>`. See m30-web-shop-form-deferred-fixes if the panel
// should later grow an explicit "sell equipped" affordance.
//
// Grouping assumes value is template-uniform: the whole group is priced off the
// first-seen instance. That holds today (nothing mutates an instance's `value`),
// but a future per-instance value (durability/masterwork on PropValue) would
// misprice a qty>1 row — recorded in the same deferred-fixes note.
func (s *ShopService) sellOffers(sh Shopper, shop ShopConfig, standing StandingFunc) []ShopOffer {
	rows := make([]ShopOffer, 0)
	index := make(map[item.TemplateID]int) // template id → rows index
	for _, id := range sh.Inventory() {
		inst := s.itemInstance(id)
		if inst == nil {
			continue
		}
		if instanceHasTag(inst, TagNoSell) {
			continue
		}
		value := instanceValue(inst)
		if value <= 0 {
			continue
		}
		if i, seen := index[inst.TemplateID()]; seen {
			rows[i].Qty++
			continue
		}
		index[inst.TemplateID()] = len(rows)
		rows = append(rows, ShopOffer{
			Name:       inst.Name(),
			Token:      sellToken(inst),
			Price:      sellPrice(value, shop, s.cfg, standing),
			Qty:        1,
			Affordable: true,
		})
	}
	return rows
}

// sellToken is the keyword a player would type to sell inst: its first content
// keyword, else the last word of its name (its noun), else the whole name.
// Mirrors keyword resolution so `sell <token>` finds this item.
func sellToken(inst *entities.ItemInstance) string {
	if kw := inst.Keywords(); len(kw) > 0 {
		return kw[0]
	}
	if words := strings.Fields(inst.Name()); len(words) > 0 {
		return words[len(words)-1]
	}
	return inst.Name()
}

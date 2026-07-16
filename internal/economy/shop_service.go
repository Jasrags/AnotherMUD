package economy

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
)

// Shopper is the player-side surface the shop service buys for and
// sells from (spec §3.5 / §3.6). The session connActor satisfies it;
// the command handler type-asserts its Actor to this interface (same
// pattern as economy.Entity in the auto-convert path). Items are
// addressed by entity id and resolved through the service's store.
type Shopper interface {
	Entity
	// Inventory returns the carried (not equipped) item ids.
	Inventory() []entities.EntityID
	// AddToInventory / RemoveFromInventory mutate carried items.
	AddToInventory(entities.EntityID)
	RemoveFromInventory(entities.EntityID) bool
	// Equipment returns slot key → equipped item id. Sell resolves
	// against equipped items too (§3.6 step 6 auto-unequip).
	Equipment() map[string]entities.EntityID
	// Unequip moves the item at slotKey back to inventory and
	// reverses its modifiers; returns the id and true on success.
	Unequip(slotKey string) (entities.EntityID, bool)
}

// ShopSink publishes the cancellable shop pre-events (spec §3.10).
// cmd/anothermud bridges it to eventbus.PublishCancellable. Each
// method returns whether a listener vetoed the transaction. A nil
// sink (NewShopService default) never cancels.
type ShopSink interface {
	OnShopBuy(ctx context.Context, actorID, npcID, templateID string, price int64) (cancelled bool)
	OnShopSell(ctx context.Context, actorID, npcID, templateID string, price int64) (cancelled bool)
}

type nopShopSink struct{}

func (nopShopSink) OnShopBuy(context.Context, string, string, string, int64) bool  { return false }
func (nopShopSink) OnShopSell(context.Context, string, string, string, int64) bool { return false }

// ShopService owns the spec §3 buy/sell/value operations. It prices
// through the shop + global config, resolves stock/inventory, fires
// the cancellable events, and moves gold through the currency
// service. Item creation/destruction goes through the entity store.
type ShopService struct {
	tpls     *item.Templates
	store    *entities.Store
	currency *CurrencyService
	cfg      EconomyConfig
	sink     ShopSink
}

// NewShopService wires the service. A nil sink becomes a nop (no
// transaction is ever cancelled); a zero-value cfg is replaced with
// the documented defaults so a caller can't accidentally price every
// item at zero markup.
func NewShopService(tpls *item.Templates, store *entities.Store, currency *CurrencyService, cfg EconomyConfig, sink ShopSink) *ShopService {
	if sink == nil {
		sink = nopShopSink{}
	}
	if cfg.BuyMarkup <= 0 && cfg.SellDiscount <= 0 {
		cfg = DefaultEconomyConfig()
	}
	return &ShopService{tpls: tpls, store: store, currency: currency, cfg: cfg, sink: sink}
}

// Listings returns the shop's offered stock (spec §3.4). check is the
// buyer's §7 skill predicate; items the buyer can't yet buy are omitted
// (nil check lists everything).
func (s *ShopService) Listings(shop ShopConfig, check SkillChecker, standing StandingFunc) []Listing {
	return listings(s.tpls, shop, s.cfg, check, standing)
}

// StockNamed returns the shop's sellable stock as keyword.Named — the
// SAME scope resolveStock matches a buy query against (spec §3.7). The
// completion layer disambiguates over it, so a suggested token is
// guaranteed to round-trip through Buy. Items the buyer fails the §7 skill
// gate for are omitted so completion never suggests an unbuyable item (nil
// check includes everything). Order follows shop.Sells.
func (s *ShopService) StockNamed(shop ShopConfig, check SkillChecker) []keyword.Named {
	if s.tpls == nil {
		return nil
	}
	out := make([]keyword.Named, 0, len(shop.Sells))
	for _, id := range shop.Sells {
		tpl, err := s.tpls.Get(item.TemplateID(id))
		if err != nil {
			continue
		}
		if !meetsSkill(tpl, check) {
			continue
		}
		out = append(out, namedTemplate{tpl})
	}
	return out
}

// BuyResult is the structured outcome of Buy (spec §3.5).
type BuyResult struct {
	Outcome  ShopOutcome
	ItemID   entities.EntityID
	ItemName string
	Price    int64
	Gold     int
	// RequiredSkill / RequiredLevel carry the §7 purchase gate on a
	// ShopSkillTooLow outcome so the caller can name what the buyer lacks.
	RequiredSkill string
	RequiredLevel int
	// RequiredStanding / Faction carry the faction access floor on a
	// ShopStandingTooLow outcome (faction.md §6) so the caller can report it.
	RequiredStanding int
	Faction          string
}

// Buy purchases an item from the shop's stock (spec §3.5). The player
// is charged BEFORE the item is created and is NOT refunded if
// creation fails (spec §9 open question, kept as-is). check is the buyer's
// §7 skill predicate: a gated item below the buyer's proficiency is refused
// (ShopSkillTooLow) before any charge. A nil check never gates.
func (s *ShopService) Buy(ctx context.Context, sh Shopper, npcID string, shop ShopConfig, query string, check SkillChecker, standing StandingFunc) BuyResult {
	// faction.md §6 access gate: a hostile buyer is refused all trade before
	// any stock resolution / pricing / charge.
	if shop.refusesStanding(standing) {
		return BuyResult{Outcome: ShopStandingTooLow, Faction: shop.Faction, RequiredStanding: *shop.MinStanding}
	}
	gold := s.currency.Read(sh)
	tpl := resolveStock(s.tpls, shop, query)
	if tpl == nil {
		return BuyResult{Outcome: ShopItemNotForSale}
	}
	// §7 availability by skill level: refuse a gated item the buyer's
	// proficiency can't meet, before pricing/charging.
	if !meetsSkill(tpl, check) {
		disc, level, _ := skillRequirement(tpl)
		return BuyResult{Outcome: ShopSkillTooLow, ItemName: tpl.Name, RequiredSkill: disc, RequiredLevel: level}
	}
	price := buyPrice(templateValue(tpl), shop, s.cfg, standing)

	if int64(gold) < price {
		return BuyResult{Outcome: ShopInsufficientGold, ItemName: tpl.Name, Price: price, Gold: gold}
	}
	if s.sink.OnShopBuy(ctx, sh.ID(), npcID, string(tpl.ID), price) {
		return BuyResult{Outcome: ShopItemNotForSale, ItemName: tpl.Name, Price: price, Gold: gold}
	}

	// Charge atomically (spec §3.5 step 6). The early gate above proved
	// price <= gold <= MaxInt (gold is an int balance), so int(price)
	// cannot truncate here. Debit re-checks funds under the lock to
	// close the gate→charge TOCTOU; a balance that dropped since the
	// gate (a concurrent charge) yields ok=false → InsufficientGold.
	newGold, ok := s.currency.Debit(ctx, sh, int(price), "shop_buy:"+npcID)
	if !ok {
		return BuyResult{Outcome: ShopInsufficientGold, ItemName: tpl.Name, Price: price, Gold: newGold}
	}

	inst, err := s.store.Spawn(tpl)
	if err != nil {
		// Charged but no item — no refund (spec §9). Report as
		// not-for-sale; the gold is already gone.
		return BuyResult{Outcome: ShopItemNotForSale, ItemName: tpl.Name, Price: price, Gold: newGold}
	}
	sh.AddToInventory(inst.ID())
	return BuyResult{Outcome: ShopOK, ItemID: inst.ID(), ItemName: tpl.Name, Price: price, Gold: newGold}
}

// SellResult is the structured outcome of Sell (spec §3.6).
type SellResult struct {
	Outcome  ShopOutcome
	ItemName string
	Price    int64
	Gold     int
	// Faction / RequiredStanding carry the access floor on a ShopStandingTooLow
	// outcome (faction.md §6) so the caller can report the refusal.
	Faction          string
	RequiredStanding int
}

// Sell sells a player-held item to the shop (spec §3.6). An equipped
// match is auto-unequipped silently before transfer. A hostile seller (below
// the shop's faction access floor) is refused (faction.md §6).
func (s *ShopService) Sell(ctx context.Context, sh Shopper, npcID string, shop ShopConfig, query string, standing StandingFunc) SellResult {
	if shop.refusesStanding(standing) {
		return SellResult{Outcome: ShopStandingTooLow, Faction: shop.Faction, RequiredStanding: *shop.MinStanding}
	}
	inst, slotKey := s.resolveInventory(sh, query)
	if inst == nil {
		return SellResult{Outcome: ShopItemNotInInventory}
	}
	if instanceHasTag(inst, TagNoSell) {
		return SellResult{Outcome: ShopItemIsNoSell, ItemName: inst.Name()}
	}
	// §3.6a category gate: the shop only buys goods in the trades it deals in
	// (a ripperdoc won't buy your AK). Checked before value/pricing.
	if !s.acceptsSale(shop, inst) {
		return SellResult{Outcome: ShopItemNotAccepted, ItemName: inst.Name()}
	}
	value := instanceValue(inst)
	if value <= 0 {
		return SellResult{Outcome: ShopItemValueZero, ItemName: inst.Name()}
	}
	price := sellPrice(value, shop, s.cfg, standing)

	if s.sink.OnShopSell(ctx, sh.ID(), npcID, string(inst.TemplateID()), price) {
		return SellResult{Outcome: ShopItemNotForSale, ItemName: inst.Name(), Price: price}
	}

	// Auto-unequip a worn item back into inventory first so the
	// remove below sees it (spec §3.6 step 6).
	if slotKey != "" {
		sh.Unequip(slotKey)
	}
	sh.RemoveFromInventory(inst.ID())
	_ = s.store.Untrack(inst.ID())

	newGold := s.currency.AddGold(ctx, sh, int(price), "shop_sell:"+npcID)
	return SellResult{Outcome: ShopOK, ItemName: inst.Name(), Price: price, Gold: newGold}
}

// acceptsSale reports whether shop will buy inst, per its §3.6a category gate.
// The accepted category set is ShopConfig.Buys when authored, else derived from
// the shop's Sells stock. An empty accepted set falls open (the shop buys
// anything), so ungated / uncategorized shops keep their prior behavior.
func (s *ShopService) acceptsSale(shop ShopConfig, inst *entities.ItemInstance) bool {
	accepted := s.acceptedCategories(shop)
	if len(accepted) == 0 {
		return true
	}
	for _, t := range inst.Tags() {
		if accepted[strings.ToLower(strings.TrimSpace(t))] {
			return true
		}
	}
	return false
}

// acceptedCategories resolves a shop's buy-side category allowlist (§3.6a). An
// explicit ShopConfig.Buys wins and is matched verbatim (any tag the author
// lists). With no Buys, the set is DERIVED from the category-vocabulary tags
// (categoryTags) carried by the shop's Sells templates — so a shop that sells
// armor + chrome buys back armor + chrome with no extra authoring. Returns an
// empty map when nothing can be derived; the caller treats that as "buys
// anything".
func (s *ShopService) acceptedCategories(shop ShopConfig) map[string]bool {
	return deriveAcceptedCategories(s.tpls, shop)
}

// deriveAcceptedCategories is the tpls-parameterized core of acceptedCategories,
// shared with the boot-time ShopBuysAnything audit so both read the same vocab.
func deriveAcceptedCategories(tpls *item.Templates, shop ShopConfig) map[string]bool {
	if len(shop.Buys) > 0 {
		set := make(map[string]bool, len(shop.Buys))
		for _, t := range shop.Buys {
			if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
				set[t] = true
			}
		}
		return set
	}
	if tpls == nil {
		return nil
	}
	set := map[string]bool{}
	for _, id := range shop.Sells {
		tpl, err := tpls.Get(item.TemplateID(id))
		if err != nil {
			continue
		}
		for _, t := range tpl.Tags {
			if lt := strings.ToLower(strings.TrimSpace(t)); categoryTags[lt] {
				set[lt] = true
			}
		}
	}
	return set
}

// ShopBuysAnything reports whether a shop's §3.6a buy gate is effectively open —
// no explicit Buys and no category derivable from its Sells stock — so it will
// buy any item. Boot-time callers warn on these so a shop that silently falls
// open (stock tagged only with descriptors, or a category tag outside the
// categoryTags vocabulary) surfaces at load rather than going quiet.
func ShopBuysAnything(tpls *item.Templates, shop ShopConfig) bool {
	return len(deriveAcceptedCategories(tpls, shop)) == 0
}

// ValueScope distinguishes which price Value returned (spec §3.9).
type ValueScope int

const (
	// ScopeInventory — the player holds the item; price is what the
	// shop would pay (sell price).
	ScopeInventory ValueScope = iota
	// ScopeStock — the shop stocks the item; price is what the player
	// would pay (buy price).
	ScopeStock
)

// ValueResult is the structured outcome of Value (spec §3.9).
type ValueResult struct {
	Outcome  ShopOutcome
	ItemName string
	Price    int64
	Scope    ValueScope
}

// Value answers "what's this worth?" (spec §3.9). Inventory is tried
// first (sell price), then stock (buy price). A held item shows the
// price the player would receive, not the shop's asking price.
func (s *ShopService) Value(_ context.Context, sh Shopper, shop ShopConfig, query string, standing StandingFunc) ValueResult {
	// A held item quotes the shop's payout only if the shop actually deals in it
	// (§3.6a); otherwise fall through to stock so the query can still resolve as
	// a buy price, and failing that report not-for-sale.
	if inst, _ := s.resolveInventory(sh, query); inst != nil && s.acceptsSale(shop, inst) {
		return ValueResult{
			Outcome:  ShopOK,
			ItemName: inst.Name(),
			Price:    sellPrice(instanceValue(inst), shop, s.cfg, standing),
			Scope:    ScopeInventory,
		}
	}
	if tpl := resolveStock(s.tpls, shop, query); tpl != nil {
		return ValueResult{
			Outcome:  ShopOK,
			ItemName: tpl.Name,
			Price:    buyPrice(templateValue(tpl), shop, s.cfg, standing),
			Scope:    ScopeStock,
		}
	}
	return ValueResult{Outcome: ShopItemNotForSale}
}

// resolveInventory resolves query against the player's carried and
// equipped items using the shared keyword rules (exact keyword → prefix
// keyword → name substring, inventory-equipment-items §6.1), so a held
// "a leather cap" answers to `cap` the same way it does to look/get/wear.
// Carried items are scanned before equipped; the first pool with a match
// wins (spec §3.8 — no ambiguity detection on the inventory side).
// Returns the matched instance and, when the match is an equipped item,
// the slot key it occupies (empty for a carried match) so Sell can
// unequip it.
func (s *ShopService) resolveInventory(sh Shopper, query string) (*entities.ItemInstance, string) {
	if s.store == nil || strings.TrimSpace(query) == "" {
		return nil, ""
	}
	// Carried first (§3.8).
	if inst := s.resolvePool(sh.Inventory(), query); inst != nil {
		return inst, ""
	}
	// Then equipped, tracking each item's slot for auto-unequip on sell.
	eq := sh.Equipment()
	ids := make([]entities.EntityID, 0, len(eq))
	slotOf := make(map[entities.EntityID]string, len(eq))
	for slotKey, id := range eq {
		ids = append(ids, id)
		slotOf[id] = slotKey
	}
	if inst := s.resolvePool(ids, query); inst != nil {
		return inst, slotOf[inst.ID()]
	}
	return nil, ""
}

// resolvePool resolves query against the item instances behind ids via
// keyword.Resolve (first tiered match wins). ItemInstance satisfies
// keyword.Named, so its content keywords and name both participate.
func (s *ShopService) resolvePool(ids []entities.EntityID, query string) *entities.ItemInstance {
	cands := make([]keyword.Named, 0, len(ids))
	for _, id := range ids {
		if inst := s.itemInstance(id); inst != nil {
			cands = append(cands, inst)
		}
	}
	if m := keyword.Resolve(cands, query); m != nil {
		return m.(*entities.ItemInstance)
	}
	return nil
}

// itemInstance resolves id through the store to a *ItemInstance, or nil
// when absent or not an item.
func (s *ShopService) itemInstance(id entities.EntityID) *entities.ItemInstance {
	e, ok := s.store.GetByID(id)
	if !ok {
		return nil
	}
	inst, _ := e.(*entities.ItemInstance)
	return inst
}

// instanceValue reads the integer `value` property off an item
// instance (int / int64 / float64), zero when absent.
func instanceValue(inst *entities.ItemInstance) int {
	v, _ := inst.Property(PropValue)
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

// instanceHasTag reports whether inst carries tag (case-insensitive).
func instanceHasTag(inst *entities.ItemInstance, tag string) bool {
	for _, t := range inst.Tags() {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

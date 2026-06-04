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

// Listings returns the shop's offered stock (spec §3.4).
func (s *ShopService) Listings(shop ShopConfig) []Listing {
	return listings(s.tpls, shop, s.cfg)
}

// StockNamed returns the shop's sellable stock as keyword.Named — the
// SAME scope resolveStock matches a buy query against (spec §3.7). The
// completion layer disambiguates over it, so a suggested token is
// guaranteed to round-trip through Buy. Order follows shop.Sells.
func (s *ShopService) StockNamed(shop ShopConfig) []keyword.Named {
	if s.tpls == nil {
		return nil
	}
	out := make([]keyword.Named, 0, len(shop.Sells))
	for _, id := range shop.Sells {
		tpl, err := s.tpls.Get(item.TemplateID(id))
		if err != nil {
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
}

// Buy purchases an item from the shop's stock (spec §3.5). The player
// is charged BEFORE the item is created and is NOT refunded if
// creation fails (spec §9 open question, kept as-is).
func (s *ShopService) Buy(ctx context.Context, sh Shopper, npcID string, shop ShopConfig, query string) BuyResult {
	gold := s.currency.Read(sh)
	tpl := resolveStock(s.tpls, shop, query)
	if tpl == nil {
		return BuyResult{Outcome: ShopItemNotForSale}
	}
	price := buyPrice(templateValue(tpl), shop, s.cfg)

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
}

// Sell sells a player-held item to the shop (spec §3.6). An equipped
// match is auto-unequipped silently before transfer.
func (s *ShopService) Sell(ctx context.Context, sh Shopper, npcID string, shop ShopConfig, query string) SellResult {
	inst, slotKey := s.resolveInventory(sh, query)
	if inst == nil {
		return SellResult{Outcome: ShopItemNotInInventory}
	}
	if instanceHasTag(inst, TagNoSell) {
		return SellResult{Outcome: ShopItemIsNoSell, ItemName: inst.Name()}
	}
	value := instanceValue(inst)
	if value <= 0 {
		return SellResult{Outcome: ShopItemValueZero, ItemName: inst.Name()}
	}
	price := sellPrice(value, shop, s.cfg)

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
func (s *ShopService) Value(_ context.Context, sh Shopper, shop ShopConfig, query string) ValueResult {
	if inst, _ := s.resolveInventory(sh, query); inst != nil {
		return ValueResult{
			Outcome:  ShopOK,
			ItemName: inst.Name(),
			Price:    sellPrice(instanceValue(inst), shop, s.cfg),
			Scope:    ScopeInventory,
		}
	}
	if tpl := resolveStock(s.tpls, shop, query); tpl != nil {
		return ValueResult{
			Outcome:  ShopOK,
			ItemName: tpl.Name,
			Price:    buyPrice(templateValue(tpl), shop, s.cfg),
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

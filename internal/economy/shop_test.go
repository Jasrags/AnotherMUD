package economy

import (
	"context"
	"maps"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// --- fakes -----------------------------------------------------------

type fakeShopper struct {
	id    string
	gold  int
	inv   []entities.EntityID
	equip map[string]entities.EntityID
}

func newShopper(id string, gold int) *fakeShopper {
	return &fakeShopper{id: id, gold: gold, equip: map[string]entities.EntityID{}}
}

func (f *fakeShopper) ID() string    { return f.id }
func (f *fakeShopper) Gold() int     { return f.gold }
func (f *fakeShopper) SetGold(v int) { f.gold = v }

func (f *fakeShopper) Inventory() []entities.EntityID {
	return append([]entities.EntityID(nil), f.inv...)
}
func (f *fakeShopper) AddToInventory(id entities.EntityID) { f.inv = append(f.inv, id) }
func (f *fakeShopper) RemoveFromInventory(id entities.EntityID) bool {
	for i, e := range f.inv {
		if e == id {
			f.inv = append(f.inv[:i], f.inv[i+1:]...)
			return true
		}
	}
	return false
}
func (f *fakeShopper) Equipment() map[string]entities.EntityID {
	out := make(map[string]entities.EntityID, len(f.equip))
	maps.Copy(out, f.equip)
	return out
}
func (f *fakeShopper) Unequip(slotKey string) (entities.EntityID, bool) {
	id, ok := f.equip[slotKey]
	if !ok {
		return "", false
	}
	delete(f.equip, slotKey)
	f.inv = append(f.inv, id)
	return id, true
}

type fakeShopSink struct {
	cancelBuy, cancelSell bool
	buys, sells           int
}

func (s *fakeShopSink) OnShopBuy(context.Context, string, string, string, int64) bool {
	s.buys++
	return s.cancelBuy
}
func (s *fakeShopSink) OnShopSell(context.Context, string, string, string, int64) bool {
	s.sells++
	return s.cancelSell
}

func valTpl(id, name string, value int, tags ...string) *item.Template {
	return &item.Template{
		ID:         item.TemplateID(id),
		Name:       name,
		Type:       "item",
		Tags:       tags,
		Properties: map[string]any{"value": value},
	}
}

// shopFixture wires templates + store + service for buy/sell tests.
type shopFixture struct {
	tpls  *item.Templates
	store *entities.Store
	sink  *fakeShopSink
	svc   *ShopService
}

func newShopFixture(t *testing.T, cfg EconomyConfig) *shopFixture {
	t.Helper()
	tpls := item.NewTemplates()
	store := entities.NewStore()
	sink := &fakeShopSink{}
	svc := NewShopService(tpls, store, NewCurrencyService(nil), cfg, sink)
	return &shopFixture{tpls: tpls, store: store, sink: sink, svc: svc}
}

// --- pricing ---------------------------------------------------------

func TestPricing(t *testing.T) {
	global := DefaultEconomyConfig() // 1.2 / 0.5
	tests := []struct {
		name     string
		value    int
		cfg      ShopConfig
		wantBuy  int64
		wantSell int64
	}{
		{name: "global defaults", value: 100, cfg: ShopConfig{}, wantBuy: 120, wantSell: 50},
		{name: "per-shop override", value: 100, cfg: ShopConfig{BuyMarkup: 2.0, SellDiscount: 0.25}, wantBuy: 200, wantSell: 25},
		{name: "zero override falls through", value: 100, cfg: ShopConfig{BuyMarkup: 0, SellDiscount: 0}, wantBuy: 120, wantSell: 50},
		{name: "buy floors at 1", value: 0, cfg: ShopConfig{}, wantBuy: 1, wantSell: 1},
		{name: "sell floors at 1", value: 1, cfg: ShopConfig{SellDiscount: 0.1}, wantBuy: 1, wantSell: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buyPrice(tt.value, tt.cfg, global, nil); got != tt.wantBuy {
				t.Errorf("buyPrice = %d, want %d", got, tt.wantBuy)
			}
			if got := sellPrice(tt.value, tt.cfg, global, nil); got != tt.wantSell {
				t.Errorf("sellPrice = %d, want %d", got, tt.wantSell)
			}
		})
	}
}

// --- stock resolution ------------------------------------------------

func TestResolveStock(t *testing.T) {
	tpls := item.NewTemplates()
	tpls.Add(valTpl("core:healing-draught", "a healing draught", 20))
	tpls.Add(valTpl("core:short-sword", "a short sword", 50))
	tpls.Add(valTpl("core:short-bow", "a short bow", 60))
	cfg := ShopConfig{Sells: []string{"core:healing-draught", "core:short-sword", "core:short-bow"}}

	tests := []struct {
		name  string
		query string
		want  string // template id, "" = no match
	}{
		{name: "by name prefix", query: "healing", want: "core:healing-draught"},
		{name: "by short id prefix", query: "short sword", want: "core:short-sword"},
		{name: "article stripped", query: "a healing draught", want: "core:healing-draught"},
		{name: "ambiguous prefix matches two", query: "short", want: ""},
		{name: "no match", query: "wand", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveStock(tpls, cfg, tt.query)
			if tt.want == "" {
				if got != nil {
					t.Errorf("resolveStock(%q) = %q, want nil", tt.query, got.ID)
				}
				return
			}
			if got == nil || string(got.ID) != tt.want {
				t.Errorf("resolveStock(%q) = %v, want %q", tt.query, got, tt.want)
			}
		})
	}
}

// kwTpl is valTpl plus a keyword list, for keyword-resolution tests.
func kwTpl(id, name string, value int, keywords ...string) *item.Template {
	t := valTpl(id, name, value)
	t.Keywords = keywords
	return t
}

func TestResolveStock_ByKeyword(t *testing.T) {
	tpls := item.NewTemplates()
	// "cap" is an interior word of the name but a defined keyword — it
	// must resolve the way look/get/wear do, not just by name prefix.
	tpls.Add(kwTpl("core:leather-cap", "a leather cap", 12, "cap", "leather"))
	tpls.Add(kwTpl("core:healing-draught", "a healing draught", 20, "draught"))
	cfg := ShopConfig{Sells: []string{"core:leather-cap", "core:healing-draught"}}

	tests := []struct {
		name, query, want string
	}{
		{"exact keyword (interior word)", "cap", "core:leather-cap"},
		{"keyword over name", "draught", "core:healing-draught"},
		{"name substring still works", "leather cap", "core:leather-cap"},
		{"short id still works", "leather-cap", "core:leather-cap"},
		{"short id spaced still works", "healing draught", "core:healing-draught"},
		{"no match", "wand", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveStock(tpls, cfg, tt.query)
			if tt.want == "" {
				if got != nil {
					t.Errorf("resolveStock(%q) = %q, want nil", tt.query, got.ID)
				}
				return
			}
			if got == nil || string(got.ID) != tt.want {
				t.Errorf("resolveStock(%q) = %v, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestValue_StockByKeyword(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(kwTpl("core:leather-cap", "a leather cap", 12, "cap", "leather"))
	sh := newShopper("p1", 0) // holds nothing
	cfg := ShopConfig{Sells: []string{"core:leather-cap"}}

	res := f.svc.Value(context.Background(), sh, cfg, "cap", nil)
	if res.Outcome != ShopOK || res.Scope != ScopeStock {
		t.Fatalf("value cap = %v/%v, want OK/stock", res.Outcome, res.Scope)
	}
}

func TestValue_InventoryByKeyword(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	inst, _ := f.store.Spawn(kwTpl("core:leather-cap", "a leather cap", 12, "cap", "leather"))
	sh := newShopper("p1", 0)
	sh.AddToInventory(inst.ID())

	// Held item answers to its keyword; inventory (sell) price wins.
	res := f.svc.Value(context.Background(), sh, ShopConfig{}, "cap", nil)
	if res.Outcome != ShopOK || res.Scope != ScopeInventory {
		t.Fatalf("value cap (held) = %v/%v, want OK/inventory", res.Outcome, res.Scope)
	}
}

// --- listings --------------------------------------------------------

func TestListings(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:potion", "a potion", 20))
	f.tpls.Add(valTpl("core:freebie", "a freebie", 0)) // zero value dropped
	cfg := ShopConfig{Sells: []string{"core:potion", "core:freebie", "core:missing"}}

	got := f.svc.Listings(cfg, nil, nil)
	if len(got) != 1 {
		t.Fatalf("listings = %d rows, want 1 (zero-value + missing dropped): %+v", len(got), got)
	}
	if got[0].TemplateID != "core:potion" || got[0].BuyPrice != 24 {
		t.Errorf("listing = %+v, want potion @ 24", got[0])
	}
}

// --- buy -------------------------------------------------------------

func TestBuy_Success(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:potion", "a potion", 20))
	cfg := ShopConfig{Sells: []string{"core:potion"}}
	sh := newShopper("p1", 100)

	res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "potion", nil, nil, nil)
	if res.Outcome != ShopOK {
		t.Fatalf("outcome = %v, want ShopOK", res.Outcome)
	}
	if res.Price != 24 {
		t.Errorf("price = %d, want 24", res.Price)
	}
	if sh.gold != 76 {
		t.Errorf("gold = %d, want 76 (100-24)", sh.gold)
	}
	if len(sh.inv) != 1 || sh.inv[0] != res.ItemID {
		t.Errorf("inventory = %v, want [%q]", sh.inv, res.ItemID)
	}
	if _, ok := f.store.GetByID(res.ItemID); !ok {
		t.Error("bought item not tracked in store")
	}
	if f.sink.buys != 1 {
		t.Errorf("shop.buy event count = %d, want 1", f.sink.buys)
	}
}

func TestBuy_InsufficientGold(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:potion", "a potion", 20))
	cfg := ShopConfig{Sells: []string{"core:potion"}}
	sh := newShopper("p1", 10) // needs 24

	res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "potion", nil, nil, nil)
	if res.Outcome != ShopInsufficientGold {
		t.Fatalf("outcome = %v, want ShopInsufficientGold", res.Outcome)
	}
	if res.Price != 24 {
		t.Errorf("price = %d, want 24 (so caller can report it)", res.Price)
	}
	if sh.gold != 10 {
		t.Errorf("gold = %d, want 10 (not charged)", sh.gold)
	}
	if f.sink.buys != 0 {
		t.Error("shop.buy must not fire before the funds gate passes")
	}
}

func TestBuy_CancelledEvent(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.sink.cancelBuy = true
	f.tpls.Add(valTpl("core:potion", "a potion", 20))
	cfg := ShopConfig{Sells: []string{"core:potion"}}
	sh := newShopper("p1", 100)

	res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "potion", nil, nil, nil)
	if res.Outcome != ShopItemNotForSale {
		t.Fatalf("outcome = %v, want ShopItemNotForSale", res.Outcome)
	}
	if sh.gold != 100 {
		t.Errorf("gold = %d, want 100 (cancel before charge)", sh.gold)
	}
	if len(sh.inv) != 0 {
		t.Error("cancelled buy must not grant an item")
	}
}

func TestBuy_StockMiss(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	cfg := ShopConfig{Sells: []string{}}
	sh := newShopper("p1", 100)
	res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "potion", nil, nil, nil)
	if res.Outcome != ShopItemNotForSale {
		t.Errorf("outcome = %v, want ShopItemNotForSale", res.Outcome)
	}
}

// --- sell ------------------------------------------------------------

func TestSell_Success(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	inst, _ := f.store.Spawn(valTpl("core:gem", "a ruby", 100))
	sh := newShopper("p1", 0)
	sh.AddToInventory(inst.ID())

	res := f.svc.Sell(context.Background(), sh, "npc1", ShopConfig{}, "ruby", nil)
	if res.Outcome != ShopOK {
		t.Fatalf("outcome = %v, want ShopOK", res.Outcome)
	}
	if res.Price != 50 || sh.gold != 50 {
		t.Errorf("price/gold = %d/%d, want 50/50", res.Price, sh.gold)
	}
	if len(sh.inv) != 0 {
		t.Error("sold item should leave inventory")
	}
	if _, ok := f.store.GetByID(inst.ID()); ok {
		t.Error("sold item should be untracked")
	}
}

func TestSell_NoSellTag(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	inst, _ := f.store.Spawn(valTpl("core:relic", "a relic", 100, "no_sell"))
	sh := newShopper("p1", 0)
	sh.AddToInventory(inst.ID())

	res := f.svc.Sell(context.Background(), sh, "npc1", ShopConfig{}, "relic", nil)
	if res.Outcome != ShopItemIsNoSell {
		t.Fatalf("outcome = %v, want ShopItemIsNoSell", res.Outcome)
	}
	if len(sh.inv) != 1 {
		t.Error("no_sell item must stay in inventory")
	}
}

func TestSell_ValueZero(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	inst, _ := f.store.Spawn(valTpl("core:junk", "some junk", 0))
	sh := newShopper("p1", 0)
	sh.AddToInventory(inst.ID())

	// Prefix-match on the full name (§3.8): "some" leads "some junk".
	res := f.svc.Sell(context.Background(), sh, "npc1", ShopConfig{}, "some junk", nil)
	if res.Outcome != ShopItemValueZero {
		t.Errorf("outcome = %v, want ShopItemValueZero", res.Outcome)
	}
}

func TestSell_NotInInventory(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	sh := newShopper("p1", 0)
	res := f.svc.Sell(context.Background(), sh, "npc1", ShopConfig{}, "ruby", nil)
	if res.Outcome != ShopItemNotInInventory {
		t.Errorf("outcome = %v, want ShopItemNotInInventory", res.Outcome)
	}
}

func TestSell_AutoUnequipsEquipped(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	inst, _ := f.store.Spawn(valTpl("core:sword", "a short sword", 80))
	sh := newShopper("p1", 0)
	sh.equip["wield"] = inst.ID() // worn, not carried

	// "short" prefixes the article-stripped name "short sword" (§3.8).
	res := f.svc.Sell(context.Background(), sh, "npc1", ShopConfig{}, "short", nil)
	if res.Outcome != ShopOK {
		t.Fatalf("outcome = %v, want ShopOK (equipped item sellable)", res.Outcome)
	}
	if _, ok := sh.equip["wield"]; ok {
		t.Error("equipped slot should be cleared by auto-unequip")
	}
	if len(sh.inv) != 0 {
		t.Error("item should be removed after unequip+sell")
	}
	if _, ok := f.store.GetByID(inst.ID()); ok {
		t.Error("sold item should be untracked")
	}
}

func TestSell_CancelledEvent(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.sink.cancelSell = true
	inst, _ := f.store.Spawn(valTpl("core:gem", "a ruby", 100))
	sh := newShopper("p1", 0)
	sh.AddToInventory(inst.ID())

	res := f.svc.Sell(context.Background(), sh, "npc1", ShopConfig{}, "ruby", nil)
	if res.Outcome != ShopItemNotForSale {
		t.Fatalf("outcome = %v, want ShopItemNotForSale", res.Outcome)
	}
	if sh.gold != 0 || len(sh.inv) != 1 {
		t.Error("cancelled sell must not credit or remove the item")
	}
}

// --- value -----------------------------------------------------------

func TestValue_InventoryFirst(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	// Same item both held and stocked; inventory (sell) price wins.
	f.tpls.Add(valTpl("core:gem", "a ruby", 100))
	inst, _ := f.store.Spawn(valTpl("core:gem", "a ruby", 100))
	sh := newShopper("p1", 0)
	sh.AddToInventory(inst.ID())
	cfg := ShopConfig{Sells: []string{"core:gem"}}

	res := f.svc.Value(context.Background(), sh, cfg, "ruby", nil)
	if res.Outcome != ShopOK || res.Scope != ScopeInventory {
		t.Fatalf("outcome/scope = %v/%v, want OK/inventory", res.Outcome, res.Scope)
	}
	if res.Price != 50 {
		t.Errorf("price = %d, want 50 (sell price)", res.Price)
	}
}

func TestValue_StockFallback(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:gem", "a ruby", 100))
	sh := newShopper("p1", 0) // holds nothing
	cfg := ShopConfig{Sells: []string{"core:gem"}}

	res := f.svc.Value(context.Background(), sh, cfg, "ruby", nil)
	if res.Outcome != ShopOK || res.Scope != ScopeStock {
		t.Fatalf("outcome/scope = %v/%v, want OK/stock", res.Outcome, res.Scope)
	}
	if res.Price != 120 {
		t.Errorf("price = %d, want 120 (buy price)", res.Price)
	}
}

func TestValue_Miss(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	sh := newShopper("p1", 0)
	res := f.svc.Value(context.Background(), sh, ShopConfig{}, "ruby", nil)
	if res.Outcome != ShopItemNotForSale {
		t.Errorf("outcome = %v, want ShopItemNotForSale", res.Outcome)
	}
}

// TestBuy_ExactKeywordBeatsScrollNameSubstring locks the §3.7 tier-aware
// resolution fix: a rusty dagger (exact keyword "dagger") and a recipe
// scroll whose NAME contains "dagger" coexist in stock, and `buy dagger`
// resolves to the dagger rather than refusing as a false ambiguity.
func TestBuy_ExactKeywordBeatsScrollNameSubstring(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(kwTpl("core:rusty-dagger", "a rusty dagger", 1, "dagger", "rusty"))
	f.tpls.Add(kwTpl("core:scroll", "a recipe scroll - forging an iron dagger", 100, "scroll", "recipe"))
	cfg := ShopConfig{Sells: []string{"core:rusty-dagger", "core:scroll"}}
	sh := newShopper("p1", 1000)

	res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "dagger", nil, nil, nil)
	if res.Outcome != ShopOK || res.ItemName != "a rusty dagger" {
		t.Fatalf("buy dagger = %v/%q, want OK/rusty dagger (exact keyword wins)", res.Outcome, res.ItemName)
	}
	// The scroll still resolves uniquely by its own keyword.
	if res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "scroll", nil, nil, nil); res.Outcome != ShopOK || res.ItemName != "a recipe scroll - forging an iron dagger" {
		t.Errorf("buy scroll = %v/%q, want OK/the scroll", res.Outcome, res.ItemName)
	}
}

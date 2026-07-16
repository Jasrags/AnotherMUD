package economy

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// seedSaleItem spawns a tagged item into the shopper's inventory and returns
// its query keyword (the item name works for keyword.Resolve).
func seedSaleItem(t *testing.T, f *shopFixture, sh *fakeShopper, id, name string, value int, tags ...string) {
	t.Helper()
	inst, err := f.store.Spawn(valTpl(id, name, value, tags...))
	if err != nil {
		t.Fatalf("spawn %s: %v", id, err)
	}
	sh.AddToInventory(inst.ID())
}

// TestSell_CategoryGate covers the §3.6a buy gate: explicit Buys, sells-derived
// defaults, and the fall-open when nothing categorizes.
func TestSell_CategoryGate(t *testing.T) {
	// A ripperdoc-style shop: explicit Buys of chrome only.
	t.Run("explicit buys refuses off-category", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		sh := newShopper("p1", 0)
		seedSaleItem(t, f, sh, "sr:ak-97", "an AK-97", 100, "weapon", "firearm")
		cfg := ShopConfig{Buys: []string{"cyberware"}}
		res := f.svc.Sell(context.Background(), sh, "doc", cfg, "AK-97", nil)
		if res.Outcome != ShopItemNotAccepted {
			t.Fatalf("Outcome = %v, want ShopItemNotAccepted", res.Outcome)
		}
	})

	t.Run("explicit buys accepts in-category", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		sh := newShopper("p1", 0)
		seedSaleItem(t, f, sh, "sr:cybereyes", "a set of cybereyes", 100, "cyberware", "augmentation")
		cfg := ShopConfig{Buys: []string{"cyberware", "armor"}}
		res := f.svc.Sell(context.Background(), sh, "doc", cfg, "cybereyes", nil)
		if res.Outcome != ShopOK {
			t.Fatalf("Outcome = %v, want ShopOK", res.Outcome)
		}
	})

	// Sells-derived: a doc that sells armor + chrome buys them back but refuses
	// a weapon — with NO Buys authored.
	t.Run("sells-derived accepts sold category", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		f.tpls.Add(valTpl("sr:armored-jacket", "an armored jacket", 200, "armor", "wearable"))
		f.tpls.Add(valTpl("sr:cybereyes", "a set of cybereyes", 300, "cyberware"))
		sh := newShopper("p1", 0)
		seedSaleItem(t, f, sh, "sr:armor-vest", "an armor vest", 90, "armor", "wearable")
		cfg := ShopConfig{Sells: []string{"sr:armored-jacket", "sr:cybereyes"}}
		res := f.svc.Sell(context.Background(), sh, "doc", cfg, "vest", nil)
		if res.Outcome != ShopOK {
			t.Fatalf("Outcome = %v, want ShopOK", res.Outcome)
		}
	})

	t.Run("sells-derived refuses unsold category", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		f.tpls.Add(valTpl("sr:armored-jacket", "an armored jacket", 200, "armor", "wearable"))
		f.tpls.Add(valTpl("sr:cybereyes", "a set of cybereyes", 300, "cyberware"))
		sh := newShopper("p1", 0)
		seedSaleItem(t, f, sh, "sr:ak-97", "an AK-97", 100, "weapon", "firearm")
		cfg := ShopConfig{Sells: []string{"sr:armored-jacket", "sr:cybereyes"}}
		res := f.svc.Sell(context.Background(), sh, "doc", cfg, "AK-97", nil)
		if res.Outcome != ShopItemNotAccepted {
			t.Fatalf("Outcome = %v, want ShopItemNotAccepted", res.Outcome)
		}
	})

	// Descriptor-only overlap must NOT leak: a shop selling leather armor should
	// not accept a leather-tagged weapon just because both carry "leather".
	t.Run("descriptor tag does not leak category", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		f.tpls.Add(valTpl("sr:armor-vest", "an armor vest", 90, "armor", "leather"))
		sh := newShopper("p1", 0)
		seedSaleItem(t, f, sh, "sr:whip", "a leather whip", 40, "weapon", "leather")
		cfg := ShopConfig{Sells: []string{"sr:armor-vest"}}
		res := f.svc.Sell(context.Background(), sh, "doc", cfg, "whip", nil)
		if res.Outcome != ShopItemNotAccepted {
			t.Fatalf("Outcome = %v, want ShopItemNotAccepted (leather is a descriptor, not a category)", res.Outcome)
		}
	})

	// No Buys and no categorizable Sells → fall open (prior behavior).
	t.Run("uncategorized shop falls open", func(t *testing.T) {
		f := newShopFixture(t, DefaultEconomyConfig())
		sh := newShopper("p1", 0)
		seedSaleItem(t, f, sh, "sr:gem", "a ruby", 100)
		res := f.svc.Sell(context.Background(), sh, "npc", ShopConfig{}, "ruby", nil)
		if res.Outcome != ShopOK {
			t.Fatalf("Outcome = %v, want ShopOK (ungated shop buys anything)", res.Outcome)
		}
	})
}

// TestShopBuysAnything covers the boot-audit helper: explicit Buys or a
// derivable Sells category is NOT open; a shop with neither IS.
func TestShopBuysAnything(t *testing.T) {
	tpls := item.NewTemplates()
	tpls.Add(valTpl("sr:armored-jacket", "an armored jacket", 200, "armor", "wearable"))
	tpls.Add(valTpl("sr:silver-bar", "a silver bar", 500, "valuable", "metal")) // descriptors only

	tests := []struct {
		name string
		cfg  ShopConfig
		want bool
	}{
		{name: "explicit buys is not open", cfg: ShopConfig{Buys: []string{"cyberware"}}, want: false},
		{name: "derivable sells is not open", cfg: ShopConfig{Sells: []string{"sr:armored-jacket"}}, want: false},
		{name: "descriptor-only sells falls open", cfg: ShopConfig{Sells: []string{"sr:silver-bar"}}, want: true},
		{name: "empty shop falls open", cfg: ShopConfig{}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShopBuysAnything(tpls, tt.cfg); got != tt.want {
				t.Errorf("ShopBuysAnything = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestValue_CategoryGate: `value` on a held item the shop won't buy should not
// quote a payout — it reports not-for-sale (rendered "doesn't deal in that").
func TestValue_CategoryGate(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	sh := newShopper("p1", 0)
	seedSaleItem(t, f, sh, "sr:ak-97", "an AK-97", 100, "weapon", "firearm")
	cfg := ShopConfig{Buys: []string{"cyberware"}}
	res := f.svc.Value(context.Background(), sh, cfg, "AK-97", nil)
	if res.Outcome == ShopOK {
		t.Fatalf("Value Outcome = ShopOK, want a refusal for an off-category held item")
	}
}

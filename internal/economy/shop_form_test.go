package economy

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

func TestShopForm_BuySideAffordabilityAndSkillGate(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())            // markup 1.2
	f.tpls.Add(valTpl("core:ration", "a ration", 10))         // buy 12
	f.tpls.Add(valTpl("core:plate", "a suit of plate", 1000)) // buy 1200
	f.tpls.Add(gatedTpl("core:rifle", "a rifle", 50, "guns", 5))
	shop := ShopConfig{Sells: []string{"core:ration", "core:plate", "core:rifle"}}

	buyer := newShopper("p1", 100) // affords the ration, not the plate
	// A skill check the buyer fails (proficiency 0 < the rifle's level 5).
	check := func(discipline string, level int) bool { return false }

	form := f.svc.ShopForm(buyer, shop, check, nil)
	if form.Refused {
		t.Fatalf("form unexpectedly refused")
	}
	if form.Balance != 100 {
		t.Errorf("balance = %d, want 100", form.Balance)
	}
	// The gated rifle is omitted (the buyer can't buy it); the other two show.
	if len(form.Buy) != 2 {
		t.Fatalf("buy rows = %d, want 2 (rifle gated out): %+v", len(form.Buy), form.Buy)
	}
	ration, plate := form.Buy[0], form.Buy[1]
	if ration.Name != "a ration" || ration.Price != 12 || !ration.Affordable {
		t.Errorf("ration row = %+v, want a ration / 12 / affordable", ration)
	}
	if ration.Token != "ration" {
		t.Errorf("ration token = %q, want ration (short id)", ration.Token)
	}
	if plate.Name != "a suit of plate" || plate.Price != 1200 || plate.Affordable {
		t.Errorf("plate row = %+v, want a suit of plate / 1200 / NOT affordable", plate)
	}
}

func TestShopForm_SellSideGroupsAndOmits(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig()) // sell discount 0.5
	roundTpl := kwTpl("core:round", "a caseless round", 10, "round", "ammo")
	f.tpls.Add(roundTpl)
	f.tpls.Add(valTpl("core:junk", "a worthless chit", 0))                  // zero value → omitted
	f.tpls.Add(valTpl("core:heirloom", "a bound heirloom", 100, TagNoSell)) // no-sell → omitted

	buyer := newShopper("p1", 0)
	// Two identical rounds (group into one row, qty 2), one zero-value, one no-sell.
	for i := 0; i < 2; i++ {
		inst, err := f.store.Spawn(roundTpl)
		if err != nil {
			t.Fatalf("spawn round: %v", err)
		}
		buyer.AddToInventory(inst.ID())
	}
	for _, id := range []string{"core:junk", "core:heirloom"} {
		tpl, _ := f.tpls.Get(item.TemplateID(id))
		inst, err := f.store.Spawn(tpl)
		if err != nil {
			t.Fatalf("spawn %s: %v", id, err)
		}
		buyer.AddToInventory(inst.ID())
	}

	form := f.svc.ShopForm(buyer, ShopConfig{}, nil, nil)
	if len(form.Sell) != 1 {
		t.Fatalf("sell rows = %d, want 1 (rounds grouped; junk + no-sell omitted): %+v", len(form.Sell), form.Sell)
	}
	row := form.Sell[0]
	if row.Name != "a caseless round" || row.Qty != 2 {
		t.Errorf("sell row = %+v, want a caseless round ×2", row)
	}
	if row.Price != 5 { // value 10 × 0.5 discount
		t.Errorf("sell price = %d, want 5", row.Price)
	}
	if row.Token != "round" {
		t.Errorf("sell token = %q, want round (first keyword)", row.Token)
	}
	if !row.Affordable {
		t.Errorf("sell rows are always Affordable=true")
	}
}

func TestShopForm_RefusedByFactionStandingHasNoOffers(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:ration", "a ration", 10))
	floor := -10
	shop := ShopConfig{Sells: []string{"core:ration"}, Faction: "corp", MinStanding: &floor}

	buyer := newShopper("p1", 500)
	// Standing well below the shop's access floor → refused.
	standing := func(factionID string) (int, bool) { return -50, true }

	form := f.svc.ShopForm(buyer, shop, nil, standing)
	if !form.Refused {
		t.Fatalf("hostile shopper should be refused: %+v", form)
	}
	if len(form.Buy) != 0 || len(form.Sell) != 0 {
		t.Errorf("refused form should carry no offers: buy=%d sell=%d", len(form.Buy), len(form.Sell))
	}
	if form.Balance != 500 {
		t.Errorf("balance = %d, want 500 (still reported)", form.Balance)
	}
	// The offer slices are non-nil (marshal as [], not null).
	if form.Buy == nil || form.Sell == nil {
		t.Errorf("refused form offer slices must be non-nil")
	}
}

// sanity: a lone sellable item leaves Qty at 1 (the wire omits it as single).
func TestShopForm_SingleSellItemQtyOne(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	tpl := kwTpl("core:gem", "a small gem", 40, "gem")
	f.tpls.Add(tpl)
	buyer := newShopper("p1", 0)
	inst, err := f.store.Spawn(tpl)
	if err != nil {
		t.Fatalf("spawn gem: %v", err)
	}
	buyer.AddToInventory(inst.ID())

	form := f.svc.ShopForm(buyer, ShopConfig{}, nil, nil)
	if len(form.Sell) != 1 || form.Sell[0].Qty != 1 {
		t.Fatalf("single sell item = %+v, want one row Qty 1", form.Sell)
	}
}

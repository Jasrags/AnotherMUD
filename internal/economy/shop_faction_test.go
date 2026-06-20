package economy

import (
	"context"
	"testing"
)

// standingFn returns a StandingFunc reporting a fixed standing for one faction
// (ok=false for any other id, mirroring an unresolvable faction).
func standingFn(faction string, value int) StandingFunc {
	return func(id string) (int, bool) {
		if id != faction {
			return 0, false
		}
		return value, true
	}
}

func intp(v int) *int { return &v }

func TestBuy_StandingGateRefusesHostile(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:potion", "a potion", 20))
	cfg := ShopConfig{
		Sells:       []string{"core:potion"},
		Faction:     "wot:queens-guard",
		MinStanding: intp(0), // must be neutral or better
	}
	sh := newShopper("p1", 100)

	// Hostile (-50) is refused before any charge.
	res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "potion", nil, standingFn("wot:queens-guard", -50))
	if res.Outcome != ShopStandingTooLow {
		t.Fatalf("hostile buy = %v, want ShopStandingTooLow", res.Outcome)
	}
	if res.Faction != "wot:queens-guard" || res.RequiredStanding != 0 {
		t.Errorf("gate detail = %q/%d, want wot:queens-guard/0", res.Faction, res.RequiredStanding)
	}
	if sh.gold != 100 {
		t.Errorf("gold = %d, want 100 (no charge on refusal)", sh.gold)
	}
}

func TestBuy_StandingGateAdmitsAtFloor(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:potion", "a potion", 20))
	cfg := ShopConfig{Sells: []string{"core:potion"}, Faction: "wot:queens-guard", MinStanding: intp(0)}
	sh := newShopper("p1", 100)

	res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "potion", nil, standingFn("wot:queens-guard", 0))
	if res.Outcome != ShopOK {
		t.Fatalf("at-floor buy = %v, want ShopOK", res.Outcome)
	}
}

func TestBuy_StandingGateFailsOpenWhenUnresolved(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:potion", "a potion", 20))
	cfg := ShopConfig{Sells: []string{"core:potion"}, Faction: "wot:queens-guard", MinStanding: intp(0)}
	sh := newShopper("p1", 100)

	// nil StandingFunc (no faction wired) → no gate, the purchase proceeds.
	if res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "potion", nil, nil); res.Outcome != ShopOK {
		t.Errorf("nil standing = %v, want ShopOK (fail open)", res.Outcome)
	}
	// A resolver that doesn't know this faction (ok=false) also fails open.
	sh2 := newShopper("p2", 100)
	if res := f.svc.Buy(context.Background(), sh2, "npc1", cfg, "potion", nil, standingFn("wot:other", -999)); res.Outcome != ShopOK {
		t.Errorf("unknown-faction standing = %v, want ShopOK (fail open)", res.Outcome)
	}
}

func TestBuy_AllyDiscountLowersPrice(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:potion", "a potion", 20)) // base buy = 20*1.2 = 24
	cfg := ShopConfig{
		Sells:        []string{"core:potion"},
		Faction:      "wot:queens-guard",
		AllyStanding: 100,
		AllyDiscount: 0.25,
	}
	sh := newShopper("p1", 100)

	// Below ally threshold → full price 24.
	if res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "potion", nil, standingFn("wot:queens-guard", 50)); res.Price != 24 {
		t.Errorf("non-ally price = %d, want 24", res.Price)
	}
	// Ally → 24 * (1-0.25) = 18.
	sh2 := newShopper("p2", 100)
	if res := f.svc.Buy(context.Background(), sh2, "npc1", cfg, "potion", nil, standingFn("wot:queens-guard", 250)); res.Price != 18 {
		t.Errorf("ally price = %d, want 18 (25%% off 24)", res.Price)
	}
}

func TestSell_AllyDiscountRaisesPayout(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	sh := newShopper("p1", 0)
	// Give the seller a value-40 item (base sell = 40*0.5 = 20).
	inst, _ := f.store.Spawn(valTpl("core:gem", "a gem", 40))
	sh.AddToInventory(inst.ID())
	cfg := ShopConfig{Faction: "wot:queens-guard", AllyStanding: 100, AllyDiscount: 0.25}

	// Ally → 20 * (1+0.25) = 25.
	res := f.svc.Sell(context.Background(), sh, "npc1", cfg, "gem", standingFn("wot:queens-guard", 200))
	if res.Outcome != ShopOK || res.Price != 25 {
		t.Fatalf("ally sell = %v/%d, want ShopOK/25", res.Outcome, res.Price)
	}
}

func TestSell_StandingGateRefusesHostile(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	sh := newShopper("p1", 0)
	inst, _ := f.store.Spawn(valTpl("core:gem", "a gem", 40))
	sh.AddToInventory(inst.ID())
	cfg := ShopConfig{Faction: "wot:queens-guard", MinStanding: intp(0)}

	res := f.svc.Sell(context.Background(), sh, "npc1", cfg, "gem", standingFn("wot:queens-guard", -10))
	if res.Outcome != ShopStandingTooLow {
		t.Errorf("hostile sell = %v, want ShopStandingTooLow", res.Outcome)
	}
}

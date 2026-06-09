package economy

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
)

// gatedTpl builds a stock template with a value and a §7 purchase skill
// gate (requires_skill + requires_skill_level).
func gatedTpl(id, name string, value int, skill string, level int) *item.Template {
	return &item.Template{
		ID:   item.TemplateID(id),
		Name: name,
		Type: "item",
		Properties: map[string]any{
			PropValue:              value,
			PropRequiresSkill:      skill,
			PropRequiresSkillLevel: level,
		},
	}
}

// checker returns a SkillChecker that reports a fixed proficiency for one
// discipline (0 for any other).
func checker(disc string, have int) SkillChecker {
	return func(d string, level int) bool {
		if d != disc {
			return false
		}
		return have >= level
	}
}

func TestListings_HidesGatedStockBelowSkill(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:potion", "a potion", 20))                     // ungated
	f.tpls.Add(gatedTpl("core:scroll", "a recipe scroll", 50, "smithing", 25)) // gated @25
	cfg := ShopConfig{Sells: []string{"core:potion", "core:scroll"}}

	// Below the gate: only the ungated potion lists.
	got := f.svc.Listings(cfg, checker("smithing", 10))
	if len(got) != 1 || got[0].TemplateID != "core:potion" {
		t.Fatalf("listings below skill = %+v, want only the potion", got)
	}

	// At/above the gate: both list.
	got = f.svc.Listings(cfg, checker("smithing", 25))
	if len(got) != 2 {
		t.Fatalf("listings at skill = %d rows, want 2", len(got))
	}

	// Nil checker (ungated shop / no progression): everything lists.
	got = f.svc.Listings(cfg, nil)
	if len(got) != 2 {
		t.Fatalf("listings nil-checker = %d rows, want 2 (no gating)", len(got))
	}
}

func TestBuy_SkillGateRefusedBelow(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(gatedTpl("core:scroll", "a recipe scroll", 50, "smithing", 25))
	cfg := ShopConfig{Sells: []string{"core:scroll"}}
	sh := newShopper("p1", 1000) // plenty of gold; the gate is skill, not gold

	res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "scroll", checker("smithing", 10))
	if res.Outcome != ShopSkillTooLow {
		t.Fatalf("outcome = %v, want ShopSkillTooLow", res.Outcome)
	}
	if res.RequiredSkill != "smithing" || res.RequiredLevel != 25 {
		t.Errorf("requirement = %s %d, want smithing 25", res.RequiredSkill, res.RequiredLevel)
	}
	if sh.gold != 1000 {
		t.Errorf("gold = %d, want 1000 (no charge on a refused buy)", sh.gold)
	}
	if len(sh.inv) != 0 {
		t.Errorf("inventory = %v, want empty (nothing bought)", sh.inv)
	}
	if f.sink.buys != 0 {
		t.Errorf("shop.buy events = %d, want 0 (refused before the cancellable event)", f.sink.buys)
	}
}

func TestBuy_SkillGatePassesAtThreshold(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(gatedTpl("core:scroll", "a recipe scroll", 50, "smithing", 25))
	cfg := ShopConfig{Sells: []string{"core:scroll"}}
	sh := newShopper("p1", 1000)

	res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "scroll", checker("smithing", 25))
	if res.Outcome != ShopOK {
		t.Fatalf("outcome = %v, want ShopOK at the skill threshold", res.Outcome)
	}
	if len(sh.inv) != 1 {
		t.Errorf("inventory = %v, want the bought scroll", sh.inv)
	}
}

func TestBuy_NilCheckerIgnoresGate(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(gatedTpl("core:scroll", "a recipe scroll", 50, "smithing", 25))
	cfg := ShopConfig{Sells: []string{"core:scroll"}}
	sh := newShopper("p1", 1000)

	// A nil checker means no gating — the gated scroll buys freely.
	res := f.svc.Buy(context.Background(), sh, "npc1", cfg, "scroll", nil)
	if res.Outcome != ShopOK {
		t.Fatalf("outcome = %v, want ShopOK with a nil checker", res.Outcome)
	}
}

func TestStockNamed_HidesGatedStockBelowSkill(t *testing.T) {
	f := newShopFixture(t, DefaultEconomyConfig())
	f.tpls.Add(valTpl("core:potion", "a potion", 20))
	f.tpls.Add(gatedTpl("core:scroll", "a recipe scroll", 50, "smithing", 25))
	cfg := ShopConfig{Sells: []string{"core:potion", "core:scroll"}}

	if got := f.svc.StockNamed(cfg, checker("smithing", 10)); len(got) != 1 {
		t.Errorf("completion stock below skill = %d, want 1 (gated scroll hidden)", len(got))
	}
	if got := f.svc.StockNamed(cfg, checker("smithing", 25)); len(got) != 2 {
		t.Errorf("completion stock at skill = %d, want 2", len(got))
	}
}

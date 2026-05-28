package command_test

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// shopCmdFixture wires a room with a shop mob, an item registry, and a
// ShopService into the inventory fixture so the buy/sell/value/list
// verbs can be dispatched end-to-end.
type shopCmdFixture struct {
	*invFixture
	svc *economy.ShopService
}

func merchantTpl(sells ...string) *mob.Template {
	list := make([]any, len(sells))
	for i, s := range sells {
		list[i] = s
	}
	return &mob.Template{
		ID:       "tapestry-core:merchant",
		Name:     "a merchant",
		Type:     "npc",
		Tags:     []string{"shop"},
		Keywords: []string{"merchant"},
		Properties: map[string]any{
			"shop": map[string]any{"sells": list},
		},
	}
}

func newShopCmdFixture(t *testing.T, sells ...string) *shopCmdFixture {
	t.Helper()
	inv := newInvFixture(t)
	m, err := inv.store.SpawnMob(merchantTpl(sells...))
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	inv.place.Place(m.ID(), inv.room.ID)

	tpls := item.NewTemplates()
	tpls.Add(&item.Template{
		ID: "tapestry-core:potion", Name: "a potion", Type: "item",
		Keywords: []string{"potion"}, Properties: map[string]any{"value": 20},
	})
	svc := economy.NewShopService(tpls, inv.store, economy.NewCurrencyService(nil), economy.DefaultEconomyConfig(), nil)
	return &shopCmdFixture{invFixture: inv, svc: svc}
}

func (f *shopCmdFixture) shopEnv() command.Env {
	e := f.env()
	e.Shop = f.svc
	return e
}

func TestList_ShowsStock(t *testing.T) {
	f := newShopCmdFixture(t, "tapestry-core:potion")
	a := newTestActor(f.room)
	dispatchBuiltin(t, f.shopEnv(), a, "list")
	out := a.lastLine()
	if !strings.Contains(out, "a potion") || !strings.Contains(out, "24 gold") {
		t.Errorf("list output = %q, want potion @ 24 gold", out)
	}
}

func TestBuy_VerbSuccess(t *testing.T) {
	f := newShopCmdFixture(t, "tapestry-core:potion")
	a := newTestActor(f.room)
	a.SetGold(100)

	dispatchBuiltin(t, f.shopEnv(), a, "buy potion")

	if a.Gold() != 76 {
		t.Errorf("gold = %d, want 76 (100-24)", a.Gold())
	}
	if len(a.Inventory()) != 1 {
		t.Errorf("inventory = %v, want one bought item", a.Inventory())
	}
	if !strings.Contains(a.lastLine(), "buy a potion") {
		t.Errorf("reply = %q, want buy confirmation", a.lastLine())
	}
}

func TestBuy_VerbInsufficientGold(t *testing.T) {
	f := newShopCmdFixture(t, "tapestry-core:potion")
	a := newTestActor(f.room)
	a.SetGold(5)

	dispatchBuiltin(t, f.shopEnv(), a, "buy potion")

	if a.Gold() != 5 {
		t.Errorf("gold = %d, want 5 (not charged)", a.Gold())
	}
	if !strings.Contains(a.lastLine(), "costs 24 gold") {
		t.Errorf("reply = %q, want a price hint", a.lastLine())
	}
}

func TestSell_VerbSuccess(t *testing.T) {
	f := newShopCmdFixture(t, "tapestry-core:potion")
	a := newTestActor(f.room)
	inst, _ := f.store.Spawn(&item.Template{
		ID: "tapestry-core:gem", Name: "a ruby", Type: "item",
		Properties: map[string]any{"value": 100},
	})
	a.AddToInventory(inst.ID())

	dispatchBuiltin(t, f.shopEnv(), a, "sell ruby")

	if a.Gold() != 50 {
		t.Errorf("gold = %d, want 50 (sell of 100 @ 0.5)", a.Gold())
	}
	if len(a.Inventory()) != 0 {
		t.Error("sold item should leave inventory")
	}
	if !strings.Contains(a.lastLine(), "sell a ruby") {
		t.Errorf("reply = %q, want sell confirmation", a.lastLine())
	}
}

func TestValue_VerbInventoryPrice(t *testing.T) {
	f := newShopCmdFixture(t, "tapestry-core:potion")
	a := newTestActor(f.room)
	inst, _ := f.store.Spawn(&item.Template{
		ID: "tapestry-core:gem", Name: "a ruby", Type: "item",
		Properties: map[string]any{"value": 100},
	})
	a.AddToInventory(inst.ID())

	dispatchBuiltin(t, f.shopEnv(), a, "value ruby")
	if !strings.Contains(a.lastLine(), "50 gold for a ruby") {
		t.Errorf("reply = %q, want sell-price value", a.lastLine())
	}
}

func TestShopVerb_NoShopInRoom(t *testing.T) {
	inv := newInvFixture(t) // no merchant placed
	tpls := item.NewTemplates()
	svc := economy.NewShopService(tpls, inv.store, economy.NewCurrencyService(nil), economy.DefaultEconomyConfig(), nil)
	a := newTestActor(inv.room)
	env := inv.env()
	env.Shop = svc

	dispatchBuiltin(t, env, a, "buy potion")
	if !strings.Contains(a.lastLine(), "no shop here") {
		t.Errorf("reply = %q, want 'no shop here'", a.lastLine())
	}
}

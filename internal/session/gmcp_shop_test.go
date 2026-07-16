package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// shopFormService wires an economy.ShopService over store with two stock items
// (a ration worth 10, a torch worth 5) and a nuyen currency label.
func shopFormService(t *testing.T, store *entities.Store) (*economy.ShopService, economy.CurrencyLabel) {
	t.Helper()
	tpls := item.NewTemplates()
	tpls.Add(&item.Template{ID: "sw:ration", Name: "a ration", Type: "item", Properties: map[string]any{"value": 10}})
	tpls.Add(&item.Template{ID: "sw:torch", Name: "a torch", Type: "item", Properties: map[string]any{"value": 5}})
	svc := economy.NewShopService(tpls, store, economy.NewCurrencyService(nil), economy.DefaultEconomyConfig(), nil)
	return svc, economy.CurrencyLabel{Noun: "nuyen", Suffix: "¥"}
}

// placeShopMob spawns a shop-tagged merchant selling the given template ids and
// places it in the actor's room.
func placeShopMob(t *testing.T, a *connActor, store *entities.Store, sells ...string) {
	t.Helper()
	sellsAny := make([]any, len(sells))
	for i, s := range sells {
		sellsAny[i] = s
	}
	m, err := store.SpawnMob(&mob.Template{
		ID:   "sw:merchant",
		Name: "a merchant",
		Type: "npc",
		Tags: []string{economy.TagShop},
		Properties: map[string]any{
			"shop": map[string]any{"sells": sellsAny},
		},
	})
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	a.placement.Place(m.ID(), a.Room().ID)
}

// shopFrames decodes the fake conn's Char.Shop frames.
func shopFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CharShop {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CharShop, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCharShop {
			continue
		}
		var cs gmcp.CharShop
		if err := json.Unmarshal(f.payload, &cs); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, cs)
	}
	return out
}

func TestFlushGmcpShop_NilServiceNoOp(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	fc.setActive(true)
	a.flushGmcpShop(context.Background(), nil, economy.CurrencyLabel{})
	if got := len(shopFrames(t, fc)); got != 0 {
		t.Errorf("nil shop service emitted %d frames, want 0", got)
	}
}

func TestFlushGmcpShop_ClosedWhenNoShopInRoom(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	a.placement = entities.NewPlacement()
	svc, money := shopFormService(t, store)
	fc.setActive(true)

	a.flushGmcpShop(context.Background(), svc, money)
	frames := shopFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush sent %d frames, want 1", len(frames))
	}
	if frames[0].Open {
		t.Errorf("payload should be closed (open=false) with no shop present: %+v", frames[0])
	}
	if len(frames[0].Buy) != 0 || len(frames[0].Sell) != 0 {
		t.Errorf("closed payload should carry no offers: %+v", frames[0])
	}
}

func TestFlushGmcpShop_OpenWithBuyAndSell(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	a.placement = entities.NewPlacement()
	a.SetGold(100) // affords the ration (buy 12) and the torch (buy 6)
	svc, money := shopFormService(t, store)
	placeShopMob(t, a, store, "sw:ration", "sw:torch")

	// Carry a sellable ration (value 10 → sell 5).
	ration, err := store.Spawn(&item.Template{ID: "sw:ration", Name: "a ration", Type: "item", Properties: map[string]any{"value": 10}})
	if err != nil {
		t.Fatalf("spawn ration: %v", err)
	}
	a.AddToInventory(ration.ID())
	fc.setActive(true)

	a.flushGmcpShop(context.Background(), svc, money)
	frames := shopFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush sent %d frames, want 1", len(frames))
	}
	f := frames[0]
	if !f.Open {
		t.Fatalf("payload should be open at a shop: %+v", f)
	}
	if f.Shopkeeper != "a merchant" {
		t.Errorf("shopkeeper = %q, want a merchant", f.Shopkeeper)
	}
	if f.Money != "100¥" {
		t.Errorf("money = %q, want 100¥ (currency-labelled)", f.Money)
	}
	if len(f.Buy) != 2 {
		t.Fatalf("buy rows = %d, want 2: %+v", len(f.Buy), f.Buy)
	}
	if f.Buy[0].Name != "a ration" || f.Buy[0].Cmd != "buy ration" || f.Buy[0].Price != "12¥" || !f.Buy[0].Affordable {
		t.Errorf("buy[0] = %+v, want a ration / buy ration / 12¥ / affordable", f.Buy[0])
	}
	if len(f.Sell) != 1 || f.Sell[0].Cmd != "sell ration" || f.Sell[0].Price != "5¥" {
		t.Errorf("sell rows = %+v, want one `sell ration` at 5¥", f.Sell)
	}
}

func TestFlushGmcpShop_NilVisibilityPredicateDoesNotPanic(t *testing.T) {
	// command.QuestSpawnVisible returns a NIL predicate for a viewer with no
	// player identity (and for a staff bypass) — meaning "show everything". The
	// scan must guard the nil (like roomrender.go) rather than call it, or it
	// panics. An empty playerID triggers the nil path.
	a, fc, store := newItemsGmcpActor(t, "")
	a.placement = entities.NewPlacement()
	a.SetGold(50)
	svc, money := shopFormService(t, store)
	placeShopMob(t, a, store, "sw:ration")
	fc.setActive(true)

	a.flushGmcpShop(context.Background(), svc, money) // must not panic
	frames := shopFrames(t, fc)
	if len(frames) != 1 || !frames[0].Open {
		t.Fatalf("nil-predicate viewer should still see the shop open: %+v", frames)
	}
}

func TestFlushGmcpShop_NoRedundantSendThenResendOnReset(t *testing.T) {
	a, fc, store := newItemsGmcpActor(t, "p-1")
	a.placement = entities.NewPlacement()
	a.SetGold(100)
	svc, money := shopFormService(t, store)
	placeShopMob(t, a, store, "sw:ration")
	fc.setActive(true)

	a.flushGmcpShop(context.Background(), svc, money) // baseline
	pre := len(shopFrames(t, fc))
	a.flushGmcpShop(context.Background(), svc, money)
	a.flushGmcpShop(context.Background(), svc, money)
	if got := len(shopFrames(t, fc)); got != pre {
		t.Errorf("redundant flushes added %d frames, want 0", got-pre)
	}

	a.resetGmcpItemsShadow() // clears the shop shadow too (reattach seam)
	a.flushGmcpShop(context.Background(), svc, money)
	if got := len(shopFrames(t, fc)) - pre; got != 1 {
		t.Errorf("post-reset added %d frames, want 1", got)
	}
}

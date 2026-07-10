package auction

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// fakeParty is a test seller/buyer satisfying auction.Party.
type fakeParty struct {
	id   string
	name string
	gold int
	inv  []entities.EntityID
	msgs []string
}

func (p *fakeParty) ID() string                     { return p.id }
func (p *fakeParty) Gold() int                      { return p.gold }
func (p *fakeParty) SetGold(v int)                  { p.gold = v }
func (p *fakeParty) Name() string                   { return p.name }
func (p *fakeParty) Inventory() []entities.EntityID { return p.inv }
func (p *fakeParty) AddToInventory(id entities.EntityID) {
	p.inv = append(p.inv, id)
}
func (p *fakeParty) RemoveFromInventory(id entities.EntityID) bool {
	for i, have := range p.inv {
		if have == id {
			p.inv = append(p.inv[:i], p.inv[i+1:]...)
			return true
		}
	}
	return false
}
func (p *fakeParty) Write(_ context.Context, msg string) error {
	p.msgs = append(p.msgs, msg)
	return nil
}

func (p *fakeParty) holds(id entities.EntityID) bool {
	return slices.Contains(p.inv, id)
}

// realCoin is a minimal CoinMover over the fake party's balance.
type realCoin struct{}

func (realCoin) Debit(_ context.Context, e economy.Entity, amount int, _ string) (int, bool) {
	if e.Gold() < amount {
		return e.Gold(), false
	}
	e.SetGold(e.Gold() - amount)
	return e.Gold(), true
}
func (realCoin) AddGold(_ context.Context, e economy.Entity, delta int, _ string) int {
	e.SetGold(e.Gold() + delta)
	return e.Gold()
}

// managerFixture builds a Manager over real store + entity store + templates
// with the given config, plus a seller carrying one freshly-spawned dagger.
func managerFixture(t *testing.T, cfg Config) (*Manager, *Store, *entities.Store, *fakeParty, entities.EntityID) {
	t.Helper()
	dir := t.TempDir()
	store := NewStore(dir, clock.RealClock{})
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	items := entities.NewStore()
	tpls := newTemplates(t, daggerTemplate())
	inst, err := items.Spawn(daggerTemplate())
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	seller := &fakeParty{id: "alice", name: "Alice", gold: 1000, inv: []entities.EntityID{inst.ID()}}

	m := NewManager(store, nil /*audit*/, nil /*bus*/, realCoin{}, items, tpls, clock.RealClock{}, cfg, nil /*tradable*/, economy.DefaultCurrency)
	return m, store, items, seller, inst.ID()
}

func defaultCfg() Config {
	return Config{ListingFee: 10, SaleCutPct: 5, Duration: time.Hour, MinPrice: 1, PerSellerCap: 3, PageSize: 10}
}

// TestList_StagesItemChargesFeeAndPersists is the §3 happy path: the fee is
// taken, the item leaves the bag and the live store, and a persisted active
// listing holds it.
func TestList_StagesItemChargesFeeAndPersists(t *testing.T) {
	m, store, items, seller, itemID := managerFixture(t, defaultCfg())

	inst, _ := items.GetByID(itemID)
	if err := m.List(context.Background(), seller, inst.(*entities.ItemInstance), 200); err != nil {
		t.Fatalf("list: %v", err)
	}
	if seller.gold != 990 {
		t.Errorf("gold = %d, want 990 (fee charged)", seller.gold)
	}
	if seller.holds(itemID) {
		t.Error("item still in seller bag after listing")
	}
	if _, ok := items.GetByID(itemID); ok {
		t.Error("item still a live entity after listing (should be untracked)")
	}
	active := store.ActiveListings()
	if len(active) != 1 || active[0].Price != 200 || active[0].Seller != "alice" {
		t.Fatalf("active listings wrong: %+v", active)
	}
	if active[0].Item.Template != "test:iron-dagger" {
		t.Errorf("listing item template = %q", active[0].Item.Template)
	}
}

// TestList_PriceFloor refuses a sub-minimum price and moves nothing.
func TestList_PriceFloor(t *testing.T) {
	cfg := defaultCfg()
	cfg.MinPrice = 100
	m, _, items, seller, itemID := managerFixture(t, cfg)

	inst, _ := items.GetByID(itemID)
	err := m.List(context.Background(), seller, inst.(*entities.ItemInstance), 50)
	if !errors.Is(err, ErrPriceTooLow) {
		t.Fatalf("err = %v, want ErrPriceTooLow", err)
	}
	if seller.gold != 1000 || !seller.holds(itemID) {
		t.Error("a refused listing must move nothing")
	}
}

// TestList_PerSellerCap refuses past the cap.
func TestList_PerSellerCap(t *testing.T) {
	cfg := defaultCfg()
	cfg.PerSellerCap = 1
	cfg.ListingFee = 0
	m, _, items, seller, itemID := managerFixture(t, cfg)

	// First listing fills the cap.
	inst, _ := items.GetByID(itemID)
	if err := m.List(context.Background(), seller, inst.(*entities.ItemInstance), 10); err != nil {
		t.Fatalf("first list: %v", err)
	}
	// A second item, second listing — refused.
	second, _ := items.Spawn(daggerTemplate())
	seller.AddToInventory(second.ID())
	err := m.List(context.Background(), seller, second, 10)
	if !errors.Is(err, ErrListingCap) {
		t.Fatalf("err = %v, want ErrListingCap", err)
	}
}

// TestList_CannotAffordFee refuses when the fee exceeds the seller's coin and
// moves nothing.
func TestList_CannotAffordFee(t *testing.T) {
	cfg := defaultCfg()
	cfg.ListingFee = 5000
	m, _, items, seller, itemID := managerFixture(t, cfg)

	inst, _ := items.GetByID(itemID)
	err := m.List(context.Background(), seller, inst.(*entities.ItemInstance), 200)
	if !errors.Is(err, ErrCantAfford) {
		t.Fatalf("err = %v, want ErrCantAfford", err)
	}
	if seller.gold != 1000 || !seller.holds(itemID) {
		t.Error("a refused listing must move nothing")
	}
}

// TestList_NotTradable refuses a bound item.
func TestList_NotTradable(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg())
	m.tradable = func(id entities.EntityID) bool { return false }

	inst, _ := items.GetByID(itemID)
	err := m.List(context.Background(), seller, inst.(*entities.ItemInstance), 200)
	if !errors.Is(err, ErrNotTradable) {
		t.Fatalf("err = %v, want ErrNotTradable", err)
	}
	if seller.gold != 1000 {
		t.Error("fee charged on a refused (bound) listing")
	}
}

// TestCancel_EarmarksForPickupKeepsFee — cancelling marks the listing for
// the seller's pickup (not straight back into the bag) and does not refund
// the fee.
func TestCancel_EarmarksForPickupKeepsFee(t *testing.T) {
	m, store, items, seller, itemID := managerFixture(t, defaultCfg())
	inst, _ := items.GetByID(itemID)
	_ = m.List(context.Background(), seller, inst.(*entities.ItemInstance), 200)
	goldAfterList := seller.gold

	mine := m.ListingsBySeller("alice")
	if len(mine) != 1 {
		t.Fatalf("expected 1 listing, got %d", len(mine))
	}
	if err := m.Cancel(context.Background(), seller, mine[0].ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if seller.gold != goldAfterList {
		t.Error("listing fee must not be refunded on cancel")
	}
	if seller.holds(itemID) {
		t.Error("cancel returns via pickup, not straight into the bag")
	}
	held := store.HeldForPickup("alice")
	if len(held) != 1 || held[0].Status != StatusCancelled {
		t.Fatalf("expected one cancelled pickup, got %+v", held)
	}
}

// TestCancel_NotYours refuses cancelling someone else's listing.
func TestCancel_NotYours(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg())
	inst, _ := items.GetByID(itemID)
	_ = m.List(context.Background(), seller, inst.(*entities.ItemInstance), 200)
	mine := m.ListingsBySeller("alice")

	bob := &fakeParty{id: "bob", name: "Bob", gold: 100}
	err := m.Cancel(context.Background(), bob, mine[0].ID)
	if !errors.Is(err, ErrNotYours) {
		t.Fatalf("err = %v, want ErrNotYours", err)
	}
}

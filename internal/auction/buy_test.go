package auction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// listOne lists the seller's dagger at price and returns its listing id.
func listOne(t *testing.T, m *Manager, items *entities.Store, seller *fakeParty, itemID entities.EntityID, price int) string {
	t.Helper()
	inst, _ := items.GetByID(itemID)
	if err := m.List(context.Background(), seller, inst.(*entities.ItemInstance), price); err != nil {
		t.Fatalf("list: %v", err)
	}
	mine := m.ListingsBySeller(seller.ID())
	if len(mine) == 0 {
		t.Fatal("no listing after List")
	}
	return mine[len(mine)-1].ID
}

// TestBuyout_ChargesBuyerCreditsSellerNetTakesCut is the §6/§9 happy path:
// the buyer pays the price, the seller's pending proceeds are price minus the
// cut, the cut leaves the economy, and the item is earmarked for the buyer.
func TestBuyout_ChargesBuyerCreditsSellerNetTakesCut(t *testing.T) {
	cfg := defaultCfg() // SaleCutPct 5, ListingFee 10
	m, store, items, seller, itemID := managerFixture(t, cfg)
	id := listOne(t, m, items, seller, itemID, 200)

	buyer := &fakeParty{id: "bob", name: "Bob", gold: 500}
	won, err := m.Buyout(context.Background(), buyer, id)
	if err != nil {
		t.Fatalf("buyout: %v", err)
	}
	if buyer.gold != 300 {
		t.Errorf("buyer gold = %d, want 300 (paid 200)", buyer.gold)
	}
	// Seller proceeds = 200 - 5% = 190, in the pending ledger.
	if got := store.PendingCoin("alice"); got != 190 {
		t.Errorf("seller pending = %d, want 190 (price - cut)", got)
	}
	if won.Status != StatusSold || won.Collector != "bob" {
		t.Errorf("won listing wrong: %+v", won)
	}
	// Listing is now held for the buyer to collect, not for sale.
	if len(store.ActiveListings()) != 0 {
		t.Error("sold listing still active")
	}
	if held := store.HeldForPickup("bob"); len(held) != 1 {
		t.Errorf("buyer pickup = %d, want 1", len(held))
	}
}

// TestBuyout_InsufficientCoin refuses and moves nothing.
func TestBuyout_InsufficientCoin(t *testing.T) {
	m, store, items, seller, itemID := managerFixture(t, defaultCfg())
	id := listOne(t, m, items, seller, itemID, 200)

	buyer := &fakeParty{id: "bob", name: "Bob", gold: 50}
	_, err := m.Buyout(context.Background(), buyer, id)
	if !errors.Is(err, ErrInsufficientCoin) {
		t.Fatalf("err = %v, want ErrInsufficientCoin", err)
	}
	if buyer.gold != 50 {
		t.Error("buyer charged on a failed buyout")
	}
	if len(store.ActiveListings()) != 1 {
		t.Error("listing should still be active after a failed buyout")
	}
}

// TestBuyout_GoneRefundsCleanly — buying a listing that sold a moment earlier
// fails cleanly with no value moved (§6). Simulated by selling once, then a
// second buyer racing the same id.
func TestBuyout_GoneRefundsCleanly(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg())
	id := listOne(t, m, items, seller, itemID, 200)

	first := &fakeParty{id: "bob", name: "Bob", gold: 500}
	if _, err := m.Buyout(context.Background(), first, id); err != nil {
		t.Fatalf("first buyout: %v", err)
	}
	second := &fakeParty{id: "carol", name: "Carol", gold: 500}
	_, err := m.Buyout(context.Background(), second, id)
	if !errors.Is(err, ErrNotActive) {
		t.Fatalf("err = %v, want ErrNotActive", err)
	}
	if second.gold != 500 {
		t.Errorf("losing buyer charged: gold = %d, want 500", second.gold)
	}
}

// TestBuyout_OwnListingRefused — a seller cannot buy their own listing.
func TestBuyout_OwnListingRefused(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg())
	id := listOne(t, m, items, seller, itemID, 200)
	_, err := m.Buyout(context.Background(), seller, id)
	if !errors.Is(err, ErrOwnListing) {
		t.Fatalf("err = %v, want ErrOwnListing", err)
	}
}

// TestBrowse_FilterSortPaginate covers §5: name filter, price sort, and
// paging totals.
func TestBrowse_FilterSortPaginate(t *testing.T) {
	cfg := defaultCfg()
	cfg.ListingFee = 0
	cfg.PageSize = 2
	cfg.PerSellerCap = 0 // unlimited for this fixture
	m, _, items, seller, _ := managerFixture(t, cfg)

	// List four items at varied prices. Names alternate dagger/sword.
	prices := []int{300, 100, 200, 400}
	names := []string{"dagger", "sword", "dagger", "sword"}
	for i := range prices {
		tpl := daggerTemplate()
		tpl.Name = "a " + names[i]
		inst, _ := items.Spawn(tpl)
		seller.AddToInventory(inst.ID())
		if err := m.List(context.Background(), seller, inst, prices[i]); err != nil {
			t.Fatalf("list %d: %v", i, err)
		}
	}

	now := time.Now()
	// Filter to swords, sorted by price: 100 then 400.
	page := m.Browse(now, BrowseFilter{Name: "sword", Sort: SortByPrice})
	if page.Total != 2 {
		t.Fatalf("sword total = %d, want 2", page.Total)
	}
	if page.Listings[0].Price != 100 || page.Listings[1].Price != 400 {
		t.Errorf("price sort wrong: %d, %d", page.Listings[0].Price, page.Listings[1].Price)
	}

	// No filter, page size 2 over 4 listings = 2 pages.
	p1 := m.Browse(now, BrowseFilter{Page: 1})
	if p1.TotalPages != 2 || len(p1.Listings) != 2 {
		t.Errorf("page1 = %d pages, %d items", p1.TotalPages, len(p1.Listings))
	}
	p2 := m.Browse(now, BrowseFilter{Page: 2})
	if len(p2.Listings) != 2 || p2.Page != 2 {
		t.Errorf("page2 = %d items, page %d", len(p2.Listings), p2.Page)
	}
}

// TestFindActiveByRef maps a numeric ref and full id to an active listing.
func TestFindActiveByRef(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg())
	id := listOne(t, m, items, seller, itemID, 200) // "au-1"

	num := id[len("au-"):]
	if got := m.FindActiveByRef(num); got != id {
		t.Errorf("FindActiveByRef(%q) = %q, want %q", num, got, id)
	}
	if got := m.FindActiveByRef(id); got != id {
		t.Errorf("FindActiveByRef(full) = %q, want %q", got, id)
	}
	if got := m.FindActiveByRef("999"); got != "" {
		t.Errorf("FindActiveByRef(missing) = %q, want empty", got)
	}
}

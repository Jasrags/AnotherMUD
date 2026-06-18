package auction

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// TestCollectCoin_PaysProceedsOnceThenEmpty — collecting credits the seller's
// pending proceeds, prunes the ledger, and a second collect pays nothing.
func TestCollectCoin_PaysProceedsOnceThenEmpty(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg())
	id := listOne(t, m, items, seller, itemID, 200)
	buyer := &fakeParty{id: "bob", name: "Bob", gold: 500}
	if _, err := m.Buyout(context.Background(), buyer, id); err != nil {
		t.Fatalf("buyout: %v", err)
	}

	got := m.CollectCoin(context.Background(), seller)
	if got != 190 { // 200 - 5% cut
		t.Errorf("collected = %d, want 190", got)
	}
	if seller.gold != 1180 { // started 1000, -10 fee, +190 proceeds
		t.Errorf("seller gold = %d, want 1180", seller.gold)
	}
	if again := m.CollectCoin(context.Background(), seller); again != 0 {
		t.Errorf("second collect = %d, want 0", again)
	}
}

// TestCollectItem_RehydratesAndPrunes — a buyer collects the won item: it
// rehydrates with its property bag, the pickup is pruned, and a re-collect
// finds nothing (no dupe).
func TestCollectItem_RehydratesAndPrunes(t *testing.T) {
	m, store, items, seller, itemID := managerFixture(t, defaultCfg())

	// Give the listed item a grade so we can prove the bag survives pickup.
	inst, _ := items.GetByID(itemID)
	inst.(*entities.ItemInstance).SetProperty(entities.PropGrade, "masterwork")
	id := listOne(t, m, items, seller, itemID, 200)

	buyer := &fakeParty{id: "bob", name: "Bob", gold: 500}
	if _, err := m.Buyout(context.Background(), buyer, id); err != nil {
		t.Fatalf("buyout: %v", err)
	}

	held := m.PendingPickups("bob")
	if len(held) != 1 {
		t.Fatalf("buyer pickups = %d, want 1", len(held))
	}
	live, err := m.RehydratePickup(context.Background(), held[0])
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if live.Grade() != "masterwork" {
		t.Errorf("collected item lost its grade: %q", live.Grade())
	}
	if err := m.ConfirmItemCollected(context.Background(), buyer, held[0], live.ID()); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	// Pruned: nothing waits, and the listing is gone.
	if len(m.PendingPickups("bob")) != 0 {
		t.Error("pickup not pruned after collect")
	}
	if _, ok := store.Get(id); ok {
		t.Error("collected listing not pruned")
	}
}

// TestCollect_ExpiredReturnsToSeller — an expired listing's item is held for
// the SELLER to collect, not the buyer.
func TestCollect_ExpiredReturnsToSeller(t *testing.T) {
	m, store, items, seller, itemID := managerFixture(t, defaultCfg())
	id := listOne(t, m, items, seller, itemID, 200)

	if err := store.MarkExpired(id); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if len(m.PendingPickups("alice")) != 1 {
		t.Error("expired item should wait for the seller")
	}
	held := m.PendingPickups("alice")
	live, err := m.RehydratePickup(context.Background(), held[0])
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if err := m.ConfirmItemCollected(context.Background(), seller, held[0], live.ID()); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(m.PendingPickups("alice")) != 0 {
		t.Error("seller pickup not pruned")
	}
}

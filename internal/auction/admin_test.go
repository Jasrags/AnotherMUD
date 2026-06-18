package auction

import (
	"context"
	"errors"
	"testing"
)

// TestAdminRemove_ReturnsItemToSeller — an admin removes an active listing;
// the item is earmarked for the seller and the seller is notified.
func TestAdminRemove_ReturnsItemToSeller(t *testing.T) {
	m, store, items, seller, itemID := managerFixture(t, defaultCfg())
	notif := &fakeNotifier{}
	m.SetNotifier(notif)
	id := listOne(t, m, items, seller, itemID, 200)

	admin := &fakeParty{id: "gm", name: "Gm", gold: 0}
	if _, err := m.AdminRemove(context.Background(), admin, id); err != nil {
		t.Fatalf("remove: %v", err)
	}
	l, _ := store.Get(id)
	if l.Status != StatusCancelled || l.Collector != "alice" {
		t.Errorf("removed listing wrong: %+v", l)
	}
	if len(notif.notices) != 1 {
		t.Errorf("seller notices = %d, want 1", len(notif.notices))
	}
}

// TestAdminRemove_RefusesSold — a sold listing is reversed with refund, not
// removed.
func TestAdminRemove_RefusesSold(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg())
	id := listOne(t, m, items, seller, itemID, 200)
	buyer := &fakeParty{id: "bob", name: "Bob", gold: 500}
	_, _ = m.Buyout(context.Background(), buyer, id)

	admin := &fakeParty{id: "gm", name: "Gm"}
	if _, err := m.AdminRemove(context.Background(), admin, id); !errors.Is(err, ErrNotActive) {
		t.Errorf("err = %v, want ErrNotActive", err)
	}
}

// TestAdminRefund_ReversesCleanSale — before either party collects, refund
// returns the buyer's coin (to pending) and re-earmarks the item to the
// seller, clawing back the seller's proceeds.
func TestAdminRefund_ReversesCleanSale(t *testing.T) {
	m, store, items, seller, itemID := managerFixture(t, defaultCfg())
	notif := &fakeNotifier{}
	m.SetNotifier(notif)
	id := listOne(t, m, items, seller, itemID, 200)
	buyer := &fakeParty{id: "bob", name: "Bob", gold: 500}
	if _, err := m.Buyout(context.Background(), buyer, id); err != nil {
		t.Fatalf("buyout: %v", err)
	}
	// Seller has 190 proceeds pending; buyer paid 200.
	if store.PendingCoin("alice") != 190 {
		t.Fatalf("pre-refund seller pending = %d", store.PendingCoin("alice"))
	}

	notif.notices = nil // ignore the sale notice; count only the refund's.
	admin := &fakeParty{id: "gm", name: "Gm"}
	if _, err := m.AdminRefund(context.Background(), admin, id); err != nil {
		t.Fatalf("refund: %v", err)
	}
	// Seller proceeds clawed back; buyer refunded full price to pending.
	if got := store.PendingCoin("alice"); got != 0 {
		t.Errorf("seller pending after refund = %d, want 0", got)
	}
	if got := store.PendingCoin("bob"); got != 200 {
		t.Errorf("buyer refund pending = %d, want 200", got)
	}
	// Item re-earmarked for the seller.
	l, _ := store.Get(id)
	if l.Status != StatusCancelled || l.Collector != "alice" {
		t.Errorf("refunded listing wrong: %+v", l)
	}
	if len(notif.notices) != 2 { // seller + buyer
		t.Errorf("notices = %d, want 2", len(notif.notices))
	}
}

// TestAdminRefund_RefusesAfterItemCollected — once the buyer collected the
// item the sold listing is pruned, so there is nothing left to auto-reverse
// (the audit log remains for manual operator handling).
func TestAdminRefund_RefusesAfterItemCollected(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg())
	id := listOne(t, m, items, seller, itemID, 200)
	buyer := &fakeParty{id: "bob", name: "Bob", gold: 500}
	_, _ = m.Buyout(context.Background(), buyer, id)

	// Buyer collects the item — the sold listing is pruned.
	held := m.PendingPickups("bob")
	live, _ := m.RehydratePickup(context.Background(), held[0])
	_ = m.ConfirmItemCollected(context.Background(), buyer, held[0], live.ID())

	admin := &fakeParty{id: "gm", name: "Gm"}
	if _, err := m.AdminRefund(context.Background(), admin, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound (pruned after collect)", err)
	}
}

// TestAdminRefund_RefusesAfterProceedsCollected — once the seller collected
// the proceeds, no safe clawback is possible.
func TestAdminRefund_RefusesAfterProceedsCollected(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg())
	id := listOne(t, m, items, seller, itemID, 200)
	buyer := &fakeParty{id: "bob", name: "Bob", gold: 500}
	_, _ = m.Buyout(context.Background(), buyer, id)

	m.CollectCoin(context.Background(), seller) // seller takes the 190.

	admin := &fakeParty{id: "gm", name: "Gm"}
	if _, err := m.AdminRefund(context.Background(), admin, id); !errors.Is(err, ErrCannotRefund) {
		t.Errorf("err = %v, want ErrCannotRefund", err)
	}
}

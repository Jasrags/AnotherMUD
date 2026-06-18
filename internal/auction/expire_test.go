package auction

import (
	"context"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// fakeNotifier records notices for assertions.
type fakeNotifier struct {
	notices []string
}

func (f *fakeNotifier) Notify(_ context.Context, _, _, text string) {
	f.notices = append(f.notices, text)
}

// listExpiringNow lists an item whose expiry is already in the past (the
// fixture clock is real, so a -duration listing is immediately lapsed).
func listExpiringNow(t *testing.T, m *Manager, items *entities.Store, seller *fakeParty) string {
	t.Helper()
	// Make the configured duration negative so ExpiresAt < now at List time.
	m.cfg.Duration = -time.Minute
	second, _ := items.Spawn(daggerTemplate())
	seller.AddToInventory(second.ID())
	if err := m.List(context.Background(), seller, second, 100); err != nil {
		t.Fatalf("list: %v", err)
	}
	mine := m.ListingsBySeller(seller.ID())
	return mine[len(mine)-1].ID
}

// TestSweepExpired_ReturnsItemNotifiesKeepsFee — a lapsed listing expires,
// its item is earmarked for the seller, the seller is notified, and the fee
// is not refunded (§8/§9).
func TestSweepExpired_ReturnsItemNotifiesKeepsFee(t *testing.T) {
	m, store, items, seller, _ := managerFixture(t, defaultCfg())
	notif := &fakeNotifier{}
	m.SetNotifier(notif)

	id := listExpiringNow(t, m, items, seller)
	goldAfterList := seller.gold

	n := m.SweepExpired(context.Background(), time.Now())
	if n != 1 {
		t.Fatalf("expired %d, want 1", n)
	}
	l, _ := store.Get(id)
	if l.Status != StatusExpired || l.Collector != seller.ID() {
		t.Errorf("expired listing wrong: %+v", l)
	}
	if seller.gold != goldAfterList {
		t.Error("fee must not be refunded on expiry")
	}
	if len(notif.notices) != 1 {
		t.Errorf("notices = %d, want 1", len(notif.notices))
	}
}

// TestSweepExpired_ExactlyOnce — a second sweep does not re-expire the same
// listing (idempotent; the shared boot+tick path cannot double-process).
func TestSweepExpired_ExactlyOnce(t *testing.T) {
	m, _, items, seller, _ := managerFixture(t, defaultCfg())
	listExpiringNow(t, m, items, seller)

	if n := m.SweepExpired(context.Background(), time.Now()); n != 1 {
		t.Fatalf("first sweep = %d, want 1", n)
	}
	if n := m.SweepExpired(context.Background(), time.Now()); n != 0 {
		t.Errorf("second sweep = %d, want 0", n)
	}
}

// TestSweepExpired_LeavesLiveListings — a listing still within its duration
// is not expired.
func TestSweepExpired_LeavesLiveListings(t *testing.T) {
	m, _, items, seller, itemID := managerFixture(t, defaultCfg()) // 1h duration
	_ = listOne(t, m, items, seller, itemID, 200)

	if n := m.SweepExpired(context.Background(), time.Now()); n != 0 {
		t.Errorf("sweep expired %d live listings, want 0", n)
	}
}

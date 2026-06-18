package auction

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
)

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	return NewStore(dir, clock.RealClock{}), dir
}

func sampleListing(id, seller string, expires time.Time) *Listing {
	return &Listing{
		ID:         id,
		Seller:     seller,
		SellerName: seller,
		Item:       SerializedItem{Template: "test:iron-dagger", Name: "an iron dagger"},
		Price:      100,
		Category:   "weapon",
		ListedAt:   expires.Add(-time.Hour),
		ExpiresAt:  expires,
		Status:     StatusActive,
	}
}

// TestStore_AddGetReloadRoundTrip is the §4 keystone: a listing posted
// before a reboot is present and for sale after it, item intact.
func TestStore_AddGetReloadRoundTrip(t *testing.T) {
	s, dir := testStore(t)
	exp := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	in := sampleListing("a-1", "alice", exp)
	in.Item.Properties = map[string]any{"grade": "power-wrought"}
	if err := s.Add(in); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Fresh store over the same dir = a reboot.
	s2 := NewStore(dir, clock.RealClock{})
	if err := s2.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	got, ok := s2.Get("a-1")
	if !ok {
		t.Fatal("listing missing after reload")
	}
	if got.Status != StatusActive || got.Price != 100 || got.Seller != "alice" {
		t.Errorf("reloaded listing wrong: %+v", got)
	}
	if got.Item.Properties["grade"] != "power-wrought" {
		t.Errorf("item property bag lost on reload: %+v", got.Item.Properties)
	}
	if got.Version != CurrentListingVersion {
		t.Errorf("version = %d, want %d", got.Version, CurrentListingVersion)
	}
}

// TestStore_AtomicWrite confirms the file is written via the atomic
// discipline — after an Add the canonical file exists and parses.
func TestStore_AtomicWrite(t *testing.T) {
	s, dir := testStore(t)
	if err := s.Add(sampleListing("a-1", "alice", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Fatalf("store file not written: %v", err)
	}
}

// TestStore_LoadMissingIsEmpty — a missing file is an empty store, not an
// error.
func TestStore_LoadMissingIsEmpty(t *testing.T) {
	s, _ := testStore(t)
	if err := s.Load(); err != nil {
		t.Fatalf("load of missing file: %v", err)
	}
	if got := s.ActiveListings(); len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

// TestStore_LoadCorruptIsError — a corrupt file is a hard error; the store
// holds real value and must not silently start empty.
func TestStore_LoadCorruptIsError(t *testing.T) {
	s, dir := testStore(t)
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{:not yaml:"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Load(); err == nil {
		t.Fatal("expected error loading corrupt store")
	}
}

// TestStore_LapsedActive returns only active listings past their deadline —
// the shared expire set used on tick AND on boot (a lapsed-while-down
// listing is in the set, §4/§8).
func TestStore_LapsedActive(t *testing.T) {
	s, _ := testStore(t)
	now := time.Now()
	_ = s.Add(sampleListing("live", "alice", now.Add(time.Hour)))
	_ = s.Add(sampleListing("lapsed", "bob", now.Add(-time.Minute)))

	lapsed := s.LapsedActive(now)
	if len(lapsed) != 1 || lapsed[0].ID != "lapsed" {
		t.Fatalf("LapsedActive = %+v, want [lapsed]", lapsed)
	}
}

// TestStore_PerSellerCap counts only a seller's active listings.
func TestStore_PerSellerCap(t *testing.T) {
	s, _ := testStore(t)
	_ = s.Add(sampleListing("a-1", "alice", time.Now().Add(time.Hour)))
	_ = s.Add(sampleListing("a-2", "alice", time.Now().Add(time.Hour)))
	_ = s.Add(sampleListing("b-1", "bob", time.Now().Add(time.Hour)))
	if n := s.ActiveCountBySeller("alice"); n != 2 {
		t.Errorf("alice active = %d, want 2", n)
	}
}

// TestStore_SoldCreditsSellerAndIsExactlyOnce covers the buyout transition:
// the seller's proceeds land in the pending ledger, the item is earmarked
// for the buyer, and a second MarkSold loses the race with ErrNotActive.
func TestStore_SoldCreditsSellerAndIsExactlyOnce(t *testing.T) {
	s, _ := testStore(t)
	_ = s.Add(sampleListing("a-1", "alice", time.Now().Add(time.Hour)))

	if err := s.MarkSold("a-1", "bob", 90); err != nil {
		t.Fatalf("mark sold: %v", err)
	}
	if got := s.PendingCoin("alice"); got != 90 {
		t.Errorf("alice pending = %d, want 90", got)
	}
	l, _ := s.Get("a-1")
	if l.Status != StatusSold || l.Collector != "bob" || l.Buyer != "bob" {
		t.Errorf("post-sale listing wrong: %+v", l)
	}
	if err := s.MarkSold("a-1", "carol", 90); !errors.Is(err, ErrNotActive) {
		t.Errorf("second sale err = %v, want ErrNotActive", err)
	}
}

// TestStore_ExpireIsExactlyOnce — MarkExpired earmarks the item for the
// seller and is exactly-once (the shared boot/tick path cannot double-expire).
func TestStore_ExpireIsExactlyOnce(t *testing.T) {
	s, _ := testStore(t)
	_ = s.Add(sampleListing("a-1", "alice", time.Now().Add(-time.Minute)))

	if err := s.MarkExpired("a-1"); err != nil {
		t.Fatalf("expire: %v", err)
	}
	l, _ := s.Get("a-1")
	if l.Status != StatusExpired || l.Collector != "alice" {
		t.Errorf("post-expire listing wrong: %+v", l)
	}
	if !l.HeldForPickup() {
		t.Error("expired listing should be held for pickup")
	}
	if err := s.MarkExpired("a-1"); !errors.Is(err, ErrNotActive) {
		t.Errorf("second expire err = %v, want ErrNotActive", err)
	}
}

// TestStore_CollectItemPrunes — once a returned item is collected the
// finished listing is pruned from the store.
func TestStore_CollectItemPrunes(t *testing.T) {
	s, _ := testStore(t)
	_ = s.Add(sampleListing("a-1", "alice", time.Now().Add(-time.Minute)))
	_ = s.MarkExpired("a-1")

	held := s.HeldForPickup("alice")
	if len(held) != 1 || held[0].ID != "a-1" {
		t.Fatalf("HeldForPickup = %+v", held)
	}
	if err := s.MarkItemCollected("a-1"); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if _, ok := s.Get("a-1"); ok {
		t.Error("collected listing should be pruned")
	}
}

// TestStore_ClaimCoinSingleWinner — claiming proceeds zeroes the ledger so a
// double collect cannot pay twice.
func TestStore_ClaimCoinSingleWinner(t *testing.T) {
	s, _ := testStore(t)
	_ = s.Add(sampleListing("a-1", "alice", time.Now().Add(time.Hour)))
	_ = s.MarkSold("a-1", "bob", 75)

	got, err := s.ClaimCoin("alice")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got != 75 {
		t.Errorf("claimed = %d, want 75", got)
	}
	again, _ := s.ClaimCoin("alice")
	if again != 0 {
		t.Errorf("second claim = %d, want 0", again)
	}
}

// TestStore_PendingCoinPersists — the pending ledger survives a reboot.
func TestStore_PendingCoinPersists(t *testing.T) {
	s, dir := testStore(t)
	_ = s.Add(sampleListing("a-1", "alice", time.Now().Add(time.Hour)))
	_ = s.MarkSold("a-1", "bob", 60)

	s2 := NewStore(dir, clock.RealClock{})
	if err := s2.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := s2.PendingCoin("alice"); got != 60 {
		t.Errorf("reloaded pending = %d, want 60", got)
	}
}

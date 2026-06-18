//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_AuctionHouse drives the full auction path end to end against a real
// engine: a seller lists an item at the town-square auctioneer, a second
// player browses and buys it out, then each collects their side (the buyer
// the item, the seller the proceeds) — exercising the verb → auction.Manager
// → persisted store → escrow audit → notification wiring live. Value-movement
// correctness (fee, cut, lost-race refund, expiry, admin reversal) is covered
// exhaustively by the internal/auction unit tests.
//
// Fee and cut are zeroed so the flow is deterministic regardless of a fresh
// character's starting gold; the buyer funds the purchase from the
// town-square coin pile (25 gold), so the listing price stays under that.
func TestLive_AuctionHouse(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_AUCTION_LISTING_FEE":  "0",
		"ANOTHERMUD_AUCTION_SALE_CUT_PCT": "0",
	})

	seller := telnettest.DialT(t, addr, telnettest.WithTimeout(20*time.Second))
	defer seller.Close()
	if err := createAndLogin(seller, "Auctor"); err != nil {
		t.Fatalf("login Auctor: %v", err)
	}

	// Seller picks up the leather cap and lists it.
	if err := seller.SendLine("get cap"); err != nil {
		t.Fatal(err)
	}
	if err := seller.SendLine("auction cap 20"); err != nil {
		t.Fatal(err)
	}
	if _, err := seller.ExpectString("You list"); err != nil {
		t.Fatalf("seller never saw the listing confirmation: %v", err)
	}

	// Buyer logs in, funds up from the coin pile, and browses.
	buyer := telnettest.DialT(t, addr, telnettest.WithTimeout(20*time.Second))
	defer buyer.Close()
	if err := createAndLogin(buyer, "Bidder"); err != nil {
		t.Fatalf("login Bidder: %v", err)
	}
	if err := buyer.SendLine("get coins"); err != nil {
		t.Fatal(err)
	}
	if err := buyer.SendLine("browse"); err != nil {
		t.Fatal(err)
	}
	if _, err := buyer.ExpectString("leather cap"); err != nil {
		t.Fatalf("buyer never saw the listing in browse: %v", err)
	}

	// Buy it out by its reference (the first listing is au-1 → "1").
	if err := buyer.SendLine("buyout 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := buyer.ExpectString("You win"); err != nil {
		t.Fatalf("buyer never saw the win: %v", err)
	}

	// Buyer collects the won item.
	if err := buyer.SendLine("collect"); err != nil {
		t.Fatal(err)
	}
	if _, err := buyer.ExpectString("You collect"); err != nil {
		t.Fatalf("buyer never collected the item: %v", err)
	}

	// Seller collects the proceeds.
	if err := seller.SendLine("collect"); err != nil {
		t.Fatal(err)
	}
	if _, err := seller.ExpectString("You collect 20 gold"); err != nil {
		t.Fatalf("seller never collected proceeds: %v", err)
	}
}

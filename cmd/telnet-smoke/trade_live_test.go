//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_DirectTrade drives the full direct-trade path end to end against a
// real engine: two players in the same room run the symmetric handshake,
// confirm, and complete an (empty) atomic swap — exercising the verb →
// trade.Manager → escrow → notification wiring live. Value-movement
// correctness (item/coin swap, veto make-whole, confirm-reset) is covered
// exhaustively by the internal/trade unit tests over the real escrow +
// currency services.
func TestLive_DirectTrade(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil) // starter-world, town-square

	c1 := telnettest.DialT(t, addr, telnettest.WithTimeout(20*time.Second))
	defer c1.Close()
	if err := createAndLogin(c1, "Trader"); err != nil {
		t.Fatalf("login Trader: %v", err)
	}
	c2 := telnettest.DialT(t, addr, telnettest.WithTimeout(20*time.Second))
	defer c2.Close()
	if err := createAndLogin(c2, "Buyer"); err != nil {
		t.Fatalf("login Buyer: %v", err)
	}

	// Handshake: Trader requests, Buyer is notified.
	if err := c1.SendLine("trade Buyer"); err != nil {
		t.Fatal(err)
	}
	if _, err := c2.ExpectString("wants to trade"); err != nil {
		t.Fatalf("Buyer never saw the trade request: %v", err)
	}
	// Buyer accepts (symmetric handshake) → session opens for both.
	if err := c2.SendLine("trade Trader"); err != nil {
		t.Fatal(err)
	}
	if _, err := c1.ExpectString("Trade with Buyer opened"); err != nil {
		t.Fatalf("Trader never saw the trade open: %v", err)
	}
	if _, err := c2.ExpectString("Trade with Trader opened"); err != nil {
		t.Fatalf("Buyer never saw the trade open: %v", err)
	}

	// Both confirm an unchanged (empty) pair → atomic swap fires.
	if err := c1.SendLine("confirm"); err != nil {
		t.Fatal(err)
	}
	if _, err := c1.ExpectString("Waiting on the other party"); err != nil {
		t.Fatalf("Trader confirm ack: %v", err)
	}
	if err := c2.SendLine("confirm"); err != nil {
		t.Fatal(err)
	}
	if _, err := c1.ExpectString("Trade complete"); err != nil {
		t.Fatalf("Trader never saw completion: %v", err)
	}
	if _, err := c2.ExpectString("Trade complete"); err != nil {
		t.Fatalf("Buyer never saw completion: %v", err)
	}
}

// TestLive_DirectTradeDecline checks the request/decline path: a declined
// trade ends cleanly for the initiator.
func TestLive_DirectTradeDecline(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil)

	c1 := telnettest.DialT(t, addr, telnettest.WithTimeout(20*time.Second))
	defer c1.Close()
	if err := createAndLogin(c1, "Anza"); err != nil {
		t.Fatalf("login Anza: %v", err)
	}
	c2 := telnettest.DialT(t, addr, telnettest.WithTimeout(20*time.Second))
	defer c2.Close()
	if err := createAndLogin(c2, "Bao"); err != nil {
		t.Fatalf("login Bao: %v", err)
	}

	if err := c1.SendLine("trade Bao"); err != nil {
		t.Fatal(err)
	}
	if _, err := c2.ExpectString("wants to trade"); err != nil {
		t.Fatalf("Bao never saw the request: %v", err)
	}
	if err := c2.SendLine("trade Anza"); err != nil {
		t.Fatal(err)
	}
	if _, err := c1.ExpectString("Trade with Bao opened"); err != nil {
		t.Fatalf("Anza never saw the trade open: %v", err)
	}
	// Anza declines → cancelled for both.
	if err := c1.SendLine("decline"); err != nil {
		t.Fatal(err)
	}
	if _, err := c1.ExpectString("cancelled"); err != nil {
		t.Fatalf("Anza never saw the cancel: %v", err)
	}
	if _, err := c2.ExpectString("cancelled"); err != nil {
		t.Fatalf("Bao never saw the cancel: %v", err)
	}
}

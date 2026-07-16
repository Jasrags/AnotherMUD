//go:build unix

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// shopItem / shopPayload decode the Char.Shop wire frame (web-client-plan P3
// Slice B+ — the trade form).
type shopItem struct {
	Name       string `json:"name"`
	Price      string `json:"price"`
	Qty        int    `json:"qty"`
	Cmd        string `json:"cmd"`
	Affordable bool   `json:"affordable"`
}
type shopPayload struct {
	Open       bool       `json:"open"`
	Shopkeeper string     `json:"shopkeeper"`
	Money      string     `json:"money"`
	Refused    bool       `json:"refused"`
	Buy        []shopItem `json:"buy"`
	Sell       []shopItem `json:"sell"`
}

// TestLive_GmcpShop proves the web-client P3 Slice B+ trade-form package reaches
// the wire: on login the server emits a Char.Shop GMCP frame (the additive shop
// form). At a shop-less start room it is closed (open=false, empty offers); every
// buy/sell row that appears carries a `buy <token>` / `sell <token>` submit
// command — the data a browser client renders a shop panel from.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_GmcpShop -v
func TestLive_GmcpShop(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil)

	frames := &frameLog{}
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second), telnettest.WithGMCPCapture(frames.add))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.ActivateGMCP(); err != nil {
		t.Fatalf("activate gmcp: %v", err)
	}
	if err := createAndLogin(c, "Haggler"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	c.Drain(2000 * time.Millisecond)

	f, ok := frames.lastAfter("Char.Shop", 0)
	if !ok {
		t.Fatalf("no Char.Shop frame captured on login (web-client P3 Slice B+ trade form)")
	}
	var shop shopPayload
	if err := json.Unmarshal([]byte(f.JSON), &shop); err != nil {
		t.Fatalf("unmarshal Char.Shop %q: %v", f.JSON, err)
	}

	// A closed shop (the default start room has no vendor) carries no offers; if
	// the start room DOES have a shop, every row must still be well-formed.
	if !shop.Open {
		if len(shop.Buy) != 0 || len(shop.Sell) != 0 {
			t.Errorf("closed shop carries offers: buy=%d sell=%d", len(shop.Buy), len(shop.Sell))
		}
	} else {
		for _, r := range shop.Buy {
			if !strings.HasPrefix(r.Cmd, "buy ") {
				t.Errorf("buy row %q cmd = %q, want a `buy <token>` command", r.Name, r.Cmd)
			}
		}
		for _, r := range shop.Sell {
			if !strings.HasPrefix(r.Cmd, "sell ") {
				t.Errorf("sell row %q cmd = %q, want a `sell <token>` command", r.Name, r.Cmd)
			}
		}
	}
	t.Logf("login Char.Shop: open=%v buy=%d sell=%d", shop.Open, len(shop.Buy), len(shop.Sell))
}

package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/auction"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
)

// auctionFrames decodes the fake conn's Char.Auction frames.
func auctionFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CharAuction {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CharAuction, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCharAuction {
			continue
		}
		var ca gmcp.CharAuction
		if err := json.Unmarshal(f.payload, &ca); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, ca)
	}
	return out
}

// emptyAuctionManager builds a real but empty auction manager for the closed-path
// tests (the actor is at no auctioneer, so Form is never even reached — only the
// nil guard needs a non-nil manager).
func emptyAuctionManager(t *testing.T) *auction.Manager {
	t.Helper()
	store := auction.NewStore(t.TempDir(), clock.RealClock{})
	if err := store.Load(); err != nil {
		t.Fatalf("auction store load: %v", err)
	}
	return auction.NewManager(store, nil, nil, nil, nil, nil, clock.RealClock{},
		auction.Config{PageSize: 10}, nil, economy.DefaultCurrency)
}

func TestFlushGmcpAuction_NilManagerNoOp(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	a.auction = nil
	fc.setActive(true)
	a.flushGmcpAuction(context.Background(), economy.DefaultCurrency)
	if got := len(auctionFrames(t, fc)); got != 0 {
		t.Errorf("nil auction manager emitted %d frames, want 0", got)
	}
}

func TestFlushGmcpAuction_ClosedWhenNoAuctioneer(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	a.auction = emptyAuctionManager(t) // present, but the test room is no auctioneer
	fc.setActive(true)

	a.flushGmcpAuction(context.Background(), economy.DefaultCurrency)
	frames := auctionFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush sent %d frames, want 1", len(frames))
	}
	if frames[0].Open {
		t.Errorf("payload should be closed (open=false) with no auctioneer: %+v", frames[0])
	}
	if len(frames[0].Listings) != 0 {
		t.Errorf("closed payload should carry no listings: %+v", frames[0])
	}

	// Redundant flushes add nothing; a reset (reattach) re-emits a baseline.
	a.flushGmcpAuction(context.Background(), economy.DefaultCurrency)
	if got := len(auctionFrames(t, fc)); got != 1 {
		t.Errorf("redundant flush changed frame count to %d, want 1", got)
	}
	a.resetGmcpItemsShadow()
	a.flushGmcpAuction(context.Background(), economy.DefaultCurrency)
	if got := len(auctionFrames(t, fc)); got != 2 {
		t.Errorf("post-reset frame count = %d, want 2", got)
	}
}

// TestAuctionCollect_CmdOnlyWhenSomethingWaits checks the collect affordance: the
// fixed `collect` command appears only when items or coin actually wait.
func TestAuctionCollect_CmdOnlyWhenSomethingWaits(t *testing.T) {
	money := economy.CurrencyLabel{Noun: "nuyen", Suffix: "¥"}

	empty := auctionCollect(auction.AuctionCollectible{}, money)
	if empty.Cmd != "" || empty.Coin != "" || empty.Items != 0 {
		t.Errorf("empty collect = %+v, want no cmd/coin/items", empty)
	}

	withCoin := auctionCollect(auction.AuctionCollectible{Coin: 120}, money)
	if withCoin.Cmd != "collect" || withCoin.Coin != "120¥" {
		t.Errorf("coin collect = %+v, want cmd=collect coin=120¥", withCoin)
	}

	withItems := auctionCollect(auction.AuctionCollectible{Items: 2}, money)
	if withItems.Cmd != "collect" || withItems.Items != 2 || withItems.Coin != "" {
		t.Errorf("item collect = %+v, want cmd=collect items=2 no-coin", withItems)
	}
}

func TestFormatAuctionCountdown(t *testing.T) {
	cases := []struct {
		secs int
		want string
	}{
		{0, "closing"},
		{-5, "closing"},
		{30, "<1m"},
		{90, "1m"},
		{45 * 60, "45m"},
		{2*3600 + 10*60, "2h 10m"},
		{3 * 3600, "3h"},
		{25 * 3600, "1d 1h"},
		{48 * 3600, "2d"},
	}
	for _, c := range cases {
		if got := formatAuctionCountdown(c.secs); got != c.want {
			t.Errorf("formatAuctionCountdown(%d) = %q, want %q", c.secs, got, c.want)
		}
	}
}

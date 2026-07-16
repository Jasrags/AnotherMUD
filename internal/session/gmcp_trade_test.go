package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/trade"
)

// tradeFrames decodes the fake conn's Char.Trade frames.
func tradeFrames(t *testing.T, fc *gmcpFakeConn) []gmcp.CharTrade {
	t.Helper()
	raw := fc.framesSnapshot()
	out := make([]gmcp.CharTrade, 0, len(raw))
	for _, f := range raw {
		if f.pkg != gmcp.PackageCharTrade {
			continue
		}
		var ct gmcp.CharTrade
		if err := json.Unmarshal(f.payload, &ct); err != nil {
			t.Fatalf("payload unmarshal: %v (raw %s)", err, f.payload)
		}
		out = append(out, ct)
	}
	return out
}

func TestFlushGmcpTrade_NilManagerNoOp(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	a.trades = nil
	fc.setActive(true)
	a.flushGmcpTrade(context.Background())
	if got := len(tradeFrames(t, fc)); got != 0 {
		t.Errorf("nil trade manager emitted %d frames, want 0", got)
	}
}

func TestFlushGmcpTrade_ClosedNoRedundantSendThenResendOnReset(t *testing.T) {
	a, fc, _ := newItemsGmcpActor(t, "p-1")
	// An empty manager: the actor is in no session, so every View is closed. The
	// nil deps are never touched because no session is ever opened.
	a.trades = trade.NewManager(nil, nil, nil, nil, nil, economy.DefaultCurrency)
	fc.setActive(true)

	a.flushGmcpTrade(context.Background())
	frames := tradeFrames(t, fc)
	if len(frames) != 1 {
		t.Fatalf("first flush sent %d frames, want 1", len(frames))
	}
	if frames[0].Open {
		t.Errorf("payload should be closed (open=false) with no trade: %+v", frames[0])
	}
	if len(frames[0].Mine.Items) != 0 || len(frames[0].Theirs.Items) != 0 {
		t.Errorf("closed payload should carry no items: %+v", frames[0])
	}

	// Redundant flushes add nothing.
	a.flushGmcpTrade(context.Background())
	a.flushGmcpTrade(context.Background())
	if got := len(tradeFrames(t, fc)); got != 1 {
		t.Errorf("redundant flushes changed frame count to %d, want 1", got)
	}

	// Reset (link-dead reattach) re-emits a baseline.
	a.resetGmcpItemsShadow()
	a.flushGmcpTrade(context.Background())
	if got := len(tradeFrames(t, fc)); got != 2 {
		t.Errorf("post-reset frame count = %d, want 2", got)
	}
}

// TestTradeSide_MineCarriesRescindCmd_TheirsDisplayOnly checks the viewer-side
// cmd logic directly: my staged items carry a `rescind <name>` command, the
// partner's are display-only (no cmd), and coin/confirmed pass through.
func TestTradeSide_MineCarriesRescindCmd_TheirsDisplayOnly(t *testing.T) {
	mine := tradeSide(trade.TradeOffer{
		Party:     "Alice",
		Items:     []trade.TradeItem{{ID: "i1", Name: "a steel dagger"}},
		Coin:      "50¥",
		Confirmed: true,
	}, true)
	if len(mine.Items) != 1 || mine.Items[0].Cmd != "rescind a steel dagger" {
		t.Errorf("my side item = %+v, want cmd 'rescind a steel dagger'", mine.Items)
	}
	if mine.Coin != "50¥" || !mine.Confirmed || mine.Party != "Alice" {
		t.Errorf("my side header = %+v, want Alice / 50¥ / confirmed", mine)
	}

	theirs := tradeSide(trade.TradeOffer{
		Party: "Bob",
		Items: []trade.TradeItem{{ID: "i2", Name: "a medkit"}},
	}, false)
	if len(theirs.Items) != 1 || theirs.Items[0].Cmd != "" {
		t.Errorf("their side item = %+v, want no cmd (display-only)", theirs.Items)
	}
	if theirs.Confirmed || theirs.Coin != "" {
		t.Errorf("their side header = %+v, want not-confirmed / no coin", theirs)
	}
}

package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/trade"
)

// flushGmcpTrade snapshots the actor's open direct trade into the rich Char.Trade
// form (web-client-plan P3 Slice B++) and emits a frame when it differs from the
// last-sent shadow. Rides the same gmcp-items-flush tick pass as the shop/quest
// forms with the same no-op guards: non-GMCP conn, GMCP inactive, or no trade
// manager wired. Unlike the shop/quest forms it reads the per-connActor `trades`
// manager directly (set at construction from cfg.Trades), so it needs no service
// handed down from the Manager flush snapshot.
//
// The form is CONTEXTUAL: when the actor is not trading the payload is closed
// (open=false, empty sides). Because it is rebuilt and byte-diffed every tick, an
// offer added on EITHER party's side, a coin change, or a confirm all re-emit —
// so the panel ticks live as the partner stages value. Marshaled-bytes shadow
// (like Char.Shop), guarded by gmcpItemsMu (shared with its siblings on the pass).
func (a *connActor) flushGmcpTrade(ctx context.Context) {
	if a.trades == nil {
		return
	}
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}

	data, err := json.Marshal(a.buildTradeForm())
	if err != nil {
		return
	}

	a.gmcpItemsMu.Lock()
	unchanged := a.gmcpTradeValid && string(a.gmcpTradeLast) == string(data)
	if !unchanged {
		a.gmcpTradeLast = data
		a.gmcpTradeValid = true
	}
	a.gmcpItemsMu.Unlock()
	if unchanged {
		return
	}

	if err := sender.SendGmcp(ctx, gmcp.PackageCharTrade, data); err != nil {
		logging.From(ctx).Debug("gmcp trade send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// buildTradeForm projects the actor's open trade into the Char.Trade payload via
// the trade manager's read-only View (the same offer data the `trade` verb prints,
// structured). When the actor is not trading the payload is closed (open=false) so
// the client hides the panel. Both sides' item slices are always non-nil so the
// wire carries `[]`, not `null`.
func (a *connActor) buildTradeForm() gmcp.CharTrade {
	v := a.trades.View(a.PlayerID())
	if !v.Open {
		return gmcp.CharTrade{
			Open:   false,
			Mine:   gmcp.TradeSide{Items: []gmcp.TradeGood{}},
			Theirs: gmcp.TradeSide{Items: []gmcp.TradeGood{}},
		}
	}
	return gmcp.CharTrade{
		Open:   true,
		Mine:   tradeSide(v.Mine, true),
		Theirs: tradeSide(v.Theirs, false),
	}
}

// tradeSide converts one read-only trade.TradeOffer into the wire TradeSide. Only
// the viewer's OWN items carry a submit command — `rescind <name>` pulls a staged
// item, matching how the CLI rescind matches by item-name substring (so the token
// round-trips). The partner's items are display-only (you can't rescind theirs),
// so their cmd is left empty.
func tradeSide(o trade.TradeOffer, mine bool) gmcp.TradeSide {
	goods := make([]gmcp.TradeGood, 0, len(o.Items))
	for _, it := range o.Items {
		g := gmcp.TradeGood{Name: it.Name}
		if mine {
			g.Cmd = "rescind " + it.Name
		}
		goods = append(goods, g)
	}
	return gmcp.TradeSide{
		Party:     o.Party,
		Items:     goods,
		Coin:      o.Coin,
		Confirmed: o.Confirmed,
	}
}

package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/auction"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// flushGmcpAuction snapshots the auctioneer the actor stands at into the rich
// Char.Auction form (web-client-plan P3 Slice B++) and emits a frame when it
// differs from the last-sent shadow. Rides the same gmcp-items-flush tick pass as
// the shop/trade forms with the same no-op guards: non-GMCP conn, GMCP inactive,
// or no auction manager wired. Reads the per-connActor `auction` manager directly.
//
// The form is CONTEXTUAL: when no auctioneer is present the payload is closed
// (open=false, empty listings). Because it is rebuilt and byte-diffed every tick,
// a new listing, a buyout, a spent coin, or an expiry all re-emit. Note the
// closing-time countdown means the payload changes minute-to-minute while at an
// auctioneer — that is intended (the panel ticks down), and the per-minute
// "closesIn" granularity keeps it from re-emitting every single tick.
// Marshaled-bytes shadow (like Char.Shop), guarded by gmcpItemsMu.
func (a *connActor) flushGmcpAuction(ctx context.Context, money economy.CurrencyLabel) {
	if a.auction == nil {
		return
	}
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}

	data, err := json.Marshal(a.buildAuctionForm(money))
	if err != nil {
		return
	}

	a.gmcpItemsMu.Lock()
	unchanged := a.gmcpAuctionValid && string(a.gmcpAuctionLast) == string(data)
	if !unchanged {
		a.gmcpAuctionLast = data
		a.gmcpAuctionValid = true
	}
	a.gmcpItemsMu.Unlock()
	if unchanged {
		return
	}

	if err := sender.SendGmcp(ctx, gmcp.PackageCharAuction, data); err != nil {
		logging.From(ctx).Debug("gmcp auction send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// buildAuctionForm projects the marketplace into the Char.Auction payload via the
// auction manager's read-only Form (the same Browse + pending data the CLI verbs
// show). When no auctioneer is present the payload is closed (open=false) so the
// client hides the panel. Prices are formatted here through the world's
// CurrencyLabel and affordability is judged against the viewer's balance — the
// auction package carries no currency vocabulary. The Listings slice is always
// non-nil so the wire carries `[]`, not `null`.
func (a *connActor) buildAuctionForm(money economy.CurrencyLabel) gmcp.CharAuction {
	if !a.atAuctioneer() {
		return gmcp.CharAuction{Open: false, Listings: []gmcp.AuctionItem{}}
	}
	form := a.auction.Form(a.clk.Now(), a.PlayerID())
	balance := a.Gold()
	items := make([]gmcp.AuctionItem, 0, len(form.Listings))
	for _, o := range form.Listings {
		it := gmcp.AuctionItem{
			Name:     o.Name,
			Price:    money.Format(o.Price),
			Seller:   o.Seller,
			ClosesIn: formatAuctionCountdown(o.SecondsLeft),
			Mine:     o.Mine,
		}
		if o.Mine {
			// Your own listing: no buyout (the verb refuses it); offer unlist.
			it.Affordable = true
			it.Cmd = "unlist " + o.Ref
		} else {
			it.Affordable = o.Price <= balance
			it.Cmd = "buyout " + o.Ref
		}
		items = append(items, it)
	}
	return gmcp.CharAuction{
		Open:     true,
		Money:    money.Format(balance),
		Listings: items,
		Total:    form.Total,
		Collect:  auctionCollect(form.Collectible, money),
	}
}

// auctionCollect projects the viewer's pending pickups + proceeds into the wire
// AuctionCollect: the item count, the formatted coin (omitted when none), and the
// fixed `collect` command present only when something actually waits.
func auctionCollect(c auction.AuctionCollectible, money economy.CurrencyLabel) gmcp.AuctionCollect {
	out := gmcp.AuctionCollect{Items: c.Items}
	if c.Coin > 0 {
		out.Coin = money.Format(c.Coin)
	}
	if c.Items > 0 || c.Coin > 0 {
		out.Cmd = "collect"
	}
	return out
}

// atAuctioneer reports whether the actor stands at an auction access point,
// mirroring command.atAuctioneer EXACTLY: a room tagged auction_house, or a room
// holding an auctioneer-tagged NPC the actor may interact with (a foreign quest-
// spawned auctioneer owned by another player is skipped — quest-spawns.md Phase 2
// — so the panel never dangles listings the verbs would refuse). A nil visibility
// predicate means "show everything" (staff bypass / no player identity) and is
// guarded, never called (it would panic), exactly as the room renderer does.
func (a *connActor) atAuctioneer() bool {
	room := a.Room()
	if room == nil {
		return false
	}
	if room.HasTag(auction.TagAuctionHouse) {
		return true
	}
	if a.items == nil || a.placement == nil {
		return false
	}
	visible := command.QuestSpawnVisible(a, a.adminRole)
	for _, id := range a.placement.InRoom(room.ID) {
		e, ok := a.items.GetByID(id)
		if !ok {
			continue
		}
		if visible != nil && !visible(e) {
			continue // foreign quest spawn — not interactable
		}
		mob, ok := e.(*entities.MobInstance)
		if !ok {
			continue
		}
		for _, t := range mob.Tags() {
			if strings.EqualFold(t, auction.TagAuctioneer) {
				return true
			}
		}
	}
	return false
}

// formatAuctionCountdown renders a compact time-to-expiry for the panel: "<1m",
// "12m", "2h 10m", "1d 3h". A per-minute granularity (never seconds) keeps the
// byte-diffed payload from re-emitting every tick. Zero/negative → "closing".
func formatAuctionCountdown(secs int) string {
	if secs <= 0 {
		return "closing"
	}
	d := time.Duration(secs) * time.Second
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		if m := int(d.Minutes()) - h*60; m > 0 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	default:
		days := int(d.Hours()) / 24
		if h := int(d.Hours()) - days*24; h > 0 {
			return fmt.Sprintf("%dd %dh", days, h)
		}
		return fmt.Sprintf("%dd", days)
	}
}

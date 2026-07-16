package auction

import (
	"strings"
	"time"
)

// This file holds the read-only projection of the marketplace (web-client-plan P3
// Slice B++) — the Char.Auction form. It composes the existing read-only Browse +
// pending-pickup queries into one snapshot a rich client renders into a listings
// panel and submits against with the existing buyout/unlist/collect verbs (the
// authority invariant). No mutation.

// AuctionOffer is one active listing row in an AuctionForm. Prices are RAW: the
// caller formats via the world CurrencyLabel and judges affordability against the
// viewer's balance, exactly as economy.ShopForm does — the auction package stays
// free of display vocabulary. Ref is the numeric listing reference the buyout /
// unlist verbs take (the `au-` prefix stripped, matching what `browse` displays).
type AuctionOffer struct {
	Ref         string
	Name        string
	Price       int
	Seller      string
	SecondsLeft int
	Mine        bool // the viewer's own listing (unlist, not buyout)
}

// AuctionCollectible is the viewer's pending pickups + proceeds (§7): the count of
// held items waiting to collect and the RAW coin proceeds. Either being non-zero
// enables the panel's collect affordance.
type AuctionCollectible struct {
	Items int
	Coin  int
}

// AuctionForm is the read-only projection of the marketplace for the Char.Auction
// panel: the active listings (the first browse page — soonest-closing) with the
// TOTAL active count (so the panel can say "10 of N — use browse"), plus the
// viewer's collectibles. Built without mutating.
type AuctionForm struct {
	Listings    []AuctionOffer
	Total       int
	Collectible AuctionCollectible
}

// Form snapshots the marketplace for playerID at now (web-client-plan P3 Slice
// B++). It composes the read-only Browse (default filter — page 1, soonest-
// closing) with the viewer's pending pickups + proceeds. The three reads are each
// individually locked in the store but are not one atomic snapshot, so a listing
// may shift between them — harmless for a display projection the next tick
// corrects. Prices stay raw; the caller formats + judges affordability.
func (m *Manager) Form(now time.Time, playerID string) AuctionForm {
	page := m.Browse(now, BrowseFilter{})
	offers := make([]AuctionOffer, 0, len(page.Listings))
	for i := range page.Listings {
		l := page.Listings[i]
		secs := int(l.ExpiresAt.Sub(now).Seconds())
		if secs < 0 {
			secs = 0
		}
		offers = append(offers, AuctionOffer{
			Ref:         strings.TrimPrefix(l.ID, "au-"),
			Name:        l.Item.Name,
			Price:       l.Price,
			Seller:      l.SellerName,
			SecondsLeft: secs,
			Mine:        l.Seller == playerID,
		})
	}
	return AuctionForm{
		Listings: offers,
		Total:    page.Total,
		Collectible: AuctionCollectible{
			Items: len(m.PendingPickups(playerID)),
			Coin:  m.store.PendingCoin(playerID),
		},
	}
}

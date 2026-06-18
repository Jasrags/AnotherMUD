package auction

import (
	"context"
	"errors"

	"github.com/Jasrags/AnotherMUD/internal/escrow"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// Buyout errors.
var (
	// ErrInsufficientCoin — the buyer cannot cover the buyout price.
	ErrInsufficientCoin = errors.New("auction: not enough gold")
	// ErrOwnListing — a seller cannot buy their own listing (use `unlist`).
	ErrOwnListing = errors.New("auction: that is your own listing")
	// ErrVetoed — a trade.committing subscriber refused the purchase.
	ErrVetoed = errors.New("auction: purchase refused")
)

// Buyout purchases an active listing outright at its buyout price (§6). It
// commits as one atomic unit: the cancellable trade.committing pre-event is
// the veto seam (a subscriber may refuse), the buyer's coin is charged, and
// MarkSold is the single-winner check-and-set that closes the listing —
// crediting the seller's proceeds (price minus the sale cut) to the pending
// ledger and earmarking the item for the buyer to collect (§7). The cut
// leaves the economy (the gold sink, §9).
//
// Atomicity / make-whole: the only value that physically moves here is the
// buyer's coin, so a failure after the debit needs only a refund. If MarkSold
// loses the race to another buyer (the listing sold/expired a tick earlier),
// the buyer is refunded in full and told why (§6). The item is NOT moved as
// a live entity — it stays serialized in the listing, re-owned to the buyer
// by the status flip and rehydrated lazily at collect (B3); that is why the
// purchase does not run through an escrow.Transaction (which assumes two live
// parties swapping staged legs — the seller here is an offline ledger and the
// price splits into proceeds + a sunk cut). The same veto event and the same
// audit log are used; only the leg-moving machinery differs.
//
// Returns the won listing (for the buyer-facing message) on success.
func (m *Manager) Buyout(ctx context.Context, buyer Party, listingID string) (Listing, error) {
	l, ok := m.store.Get(listingID)
	if !ok {
		return Listing{}, ErrNotFound
	}
	if l.Status != StatusActive {
		return Listing{}, ErrNotActive
	}
	if l.Seller == buyer.ID() {
		return Listing{}, ErrOwnListing
	}

	cut := l.Price * m.cfg.SaleCutPct / 100
	proceeds := l.Price - cut

	busLegs := []eventbus.TradeLeg{
		{PartyID: buyer.ID(), DestPartyID: l.Seller, Kind: eventbus.TradeLegCoin, Amount: l.Price},
		{PartyID: l.Seller, DestPartyID: buyer.ID(), Kind: eventbus.TradeLegItem, ItemID: l.Item.Template},
	}

	// Veto seam (§6): any subscriber may refuse before value moves.
	if m.bus != nil {
		if cancelled := m.bus.PublishCancellable(ctx, eventbus.NewTradeCommitting(listingID, busLegs)); cancelled {
			return Listing{}, ErrVetoed
		}
	}

	// Charge the buyer the full price. Nothing has moved yet, so a refusal
	// here leaves everyone whole.
	if _, ok := m.coin.Debit(ctx, buyer, l.Price, "auction:buyout"); !ok {
		return Listing{}, ErrInsufficientCoin
	}

	// Commit point: flip active -> sold (single-winner). Credits the seller's
	// proceeds to the pending ledger. On a lost race, refund the buyer.
	if err := m.store.MarkSold(listingID, buyer.ID(), buyer.Name(), proceeds); err != nil {
		m.coin.AddGold(ctx, buyer, l.Price, "auction:buyout-refund")
		return Listing{}, err // ErrNotActive (sold/expired a moment earlier) or ErrNotFound.
	}

	if m.bus != nil {
		m.bus.Publish(ctx, eventbus.TradeCommitted{TxnID: listingID, Legs: busLegs})
	}

	// Audit the sale: the buyer's price splits into the seller's proceeds and
	// the sunk cut, and the item moves seller -> buyer (§10).
	legs := []escrow.AuditLeg{
		coinLeg(buyer.ID(), l.Seller, proceeds),
		itemLeg(l.Seller, buyer.ID(), l.Item.Template),
	}
	if cut > 0 {
		legs = append(legs, coinLeg(buyer.ID(), "", cut)) // dest "" = gold sink
	}
	m.appendAudit(ctx, auditSale, listingID, legs)

	// Tell the seller their listing sold (offline-capable, §7). nil-safe.
	m.notifySeller(ctx, l, "Your auction of "+l.Item.Name+" sold. Collect your gold at an auctioneer.")

	l.Status = StatusSold
	l.Buyer = buyer.ID()
	l.Collector = buyer.ID()
	return l, nil
}

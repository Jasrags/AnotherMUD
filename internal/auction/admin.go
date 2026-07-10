package auction

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/escrow"
)

// AdminRemove force-removes an active listing (§11): the item is earmarked
// for the seller to collect, the seller is notified, and the action is
// audited with the acting admin. Only an active listing can be removed; a
// sold one is reversed with AdminRefund instead. Role gating is enforced by
// the command dispatcher (Admin verbs), not here.
func (m *Manager) AdminRemove(ctx context.Context, admin Party, listingID string) (Listing, error) {
	l, ok := m.store.Get(listingID)
	if !ok {
		return Listing{}, ErrNotFound
	}
	if l.Status != StatusActive {
		return Listing{}, ErrNotActive
	}
	if err := m.store.MarkCancelled(listingID); err != nil {
		return Listing{}, err
	}
	// Audit with the acting admin as the leg party (who moved the item back).
	m.appendAudit(ctx, auditRemove, listingID, []escrow.AuditLeg{
		itemLeg(admin.ID(), l.Seller, l.Item.Template),
	})
	m.notifySeller(ctx, l, "An administrator removed your auction of "+l.Item.Name+". Collect it at an auctioneer.")
	return l, nil
}

// AdminRefund reverses a sold listing (§11): the buyer's coin is returned
// (to the buyer's pending ledger — they may be offline), the seller's
// proceeds are clawed back, and the item is re-earmarked for the seller. It
// refuses (ErrCannotRefund) when the sale cannot be cleanly reversed — the
// buyer already collected the item, or the seller already collected the
// proceeds — leaving the operator to handle it manually via the audit log
// (no negative balances, no dupes). Audited with the acting admin.
func (m *Manager) AdminRefund(ctx context.Context, admin Party, listingID string) (Listing, error) {
	l, ok := m.store.Get(listingID)
	if !ok {
		return Listing{}, ErrNotFound
	}
	if l.Status != StatusSold {
		return Listing{}, ErrCannotRefund
	}
	cut := l.Price * m.cfg.SaleCutPct / 100
	proceeds := l.Price - cut

	reversed, err := m.store.RefundSale(listingID, l.Price, proceeds)
	if err != nil {
		return Listing{}, err
	}

	// Audit: buyer's price returned, item moved back to the seller.
	m.appendAudit(ctx, auditRefund, listingID, []escrow.AuditLeg{
		coinLeg(admin.ID(), l.Buyer, l.Price),
		itemLeg(admin.ID(), l.Seller, l.Item.Template),
	})

	// Notify both parties (offline-capable).
	m.notifySeller(ctx, l, "An administrator reversed the sale of "+l.Item.Name+". Collect it at an auctioneer.")
	if m.notifier != nil && l.Buyer != "" {
		m.notifier.Notify(ctx, l.Buyer, l.BuyerName, "An administrator refunded your purchase of "+l.Item.Name+". Collect your "+m.money.Name()+" at an auctioneer.")
	}
	return reversed, nil
}

package auction

import (
	"context"
	"errors"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/escrow"
)

// SweepExpired processes every active listing whose duration has elapsed as
// of now (§8): each becomes expired, its held item is earmarked for the
// seller to collect (§7), the event is audited, and the seller is notified.
// Returns the number expired.
//
// One path, two callers: the recurring expiry tick AND the boot reconcile
// (§4 — a listing whose deadline passed while the server was down is expired
// on load, not skipped). Idempotent and crash-safe: MarkExpired is a
// single-winner transition, so a listing expires exactly once even if a
// buyout raced it (a listing that sold a tick earlier is simply not in the
// lapsed set, or loses the MarkExpired race harmlessly). The listing fee is
// never refunded (§9).
func (m *Manager) SweepExpired(ctx context.Context, now time.Time) int {
	lapsed := m.store.LapsedActive(now)
	n := 0
	for _, l := range lapsed {
		if m.expireOne(ctx, l) {
			n++
		}
	}
	return n
}

// expireOne expires a single lapsed listing. Returns false (without error)
// when the listing already left the active set — it raced a buyout/cancel —
// so the sweep counts only the listings it actually expired.
func (m *Manager) expireOne(ctx context.Context, l Listing) bool {
	err := m.store.MarkExpired(l.ID)
	switch {
	case err == nil:
		// expired — fall through to audit + notify.
	case errors.Is(err, ErrNotActive), errors.Is(err, ErrNotFound):
		return false // raced a buyout/cancel; nothing to do.
	default:
		return false // persist failure already surfaced by the store; skip.
	}
	m.appendAudit(ctx, auditExpire, l.ID, []escrow.AuditLeg{
		itemLeg(l.Seller, l.Seller, l.Item.Template),
	})
	m.notifySeller(ctx, l, "Your auction of "+l.Item.Name+" expired unsold. Collect it at an auctioneer.")
	return true
}

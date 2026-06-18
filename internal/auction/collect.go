package auction

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/escrow"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// auditCollect tags a collection in the shared audit log (§10).
const auditCollect = "auction-collect"

// CollectCoin claims a player's uncollected proceeds (won-sale gold waiting
// in the pending ledger, §7), credits their balance, audits it, and returns
// the amount (0 when nothing waits). The claim is single-winner in the store,
// so a double collect cannot pay twice; the credit happens only after the
// claim durably succeeds, so a write failure leaves the coin waiting.
func (m *Manager) CollectCoin(ctx context.Context, player Party) int {
	amt, err := m.store.ClaimCoin(player.ID())
	if err != nil {
		logging.From(ctx).Error("auction: claim coin failed",
			slog.String("player", player.ID()), slog.String("err", err.Error()))
		return 0
	}
	if amt == 0 {
		return 0
	}
	m.coin.AddGold(ctx, player, amt, "auction:collect")
	m.appendAudit(ctx, auditCollect, "", []escrow.AuditLeg{coinLeg("", player.ID(), amt)})
	return amt
}

// PendingPickups returns the listings still holding an uncollected item for
// the player (won items + returned expired/cancelled items, §7).
func (m *Manager) PendingPickups(playerID string) []Listing {
	return m.store.HeldForPickup(playerID)
}

// RehydratePickup turns a held listing's serialized item back into a live
// ItemInstance (tracked in the entity store with a fresh id) so the verb can
// weigh it and hand it over. The caller MUST either deliver it (and call
// ConfirmItemCollected) or untrack it (when it won't fit) — a rehydrated
// instance that is neither delivered nor untracked leaks.
func (m *Manager) RehydratePickup(_ context.Context, l Listing) (*entities.ItemInstance, error) {
	return Rehydrate(m.items, m.tpls, l.Item)
}

// ConfirmItemCollected records that a held item was handed to the player:
// the listing is marked collected (and pruned), and the handover is audited.
// Call this AFTER a successful capacity check and BEFORE adding the live item
// to the player's inventory, so a persist failure leaves the item in escrow
// rather than duplicated. liveID is the rehydrated instance's id (recorded in
// the audit trail).
func (m *Manager) ConfirmItemCollected(ctx context.Context, player Party, l Listing, liveID entities.EntityID) error {
	if err := m.store.MarkItemCollected(l.ID); err != nil {
		return err
	}
	m.appendAudit(ctx, auditCollect, l.ID, []escrow.AuditLeg{itemLeg("", player.ID(), string(liveID))})
	return nil
}

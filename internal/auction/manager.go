package auction

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/escrow"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Sentinel errors surfaced to the verb layer for player-facing messages.
var (
	// ErrPriceTooLow — the buyout price is below the configured floor.
	ErrPriceTooLow = errors.New("auction: price below the minimum")
	// ErrNotTradable — a bound/non-tradable item cannot be listed.
	ErrNotTradable = errors.New("auction: item cannot be listed")
	// ErrListingCap — the seller is at the per-player active-listing cap.
	ErrListingCap = errors.New("auction: at your active-listing limit")
	// ErrCantAfford — the seller cannot cover the listing fee.
	ErrCantAfford = errors.New("auction: cannot afford the listing fee")
	// ErrNotYours — cancelling a listing the caller does not own.
	ErrNotYours = errors.New("auction: that is not your listing")
)

// Config holds the auction tunables (auction-house §12). All are
// operator-configurable; zero fee + zero cut disables the gold sink without
// breaking the flow.
type Config struct {
	ListingFee   int           // non-refundable fee charged at listing (gold sink)
	SaleCutPct   int           // percent of the sale price taken at purchase (gold sink)
	Duration     time.Duration // how long a listing stays active before expiry
	MinPrice     int           // floor a buyout price may be set to
	PerSellerCap int           // max simultaneous active listings per seller
	PageSize     int           // browse/search results per page
}

// Audit source tags recorded in the shared trade-audit log (§10).
const (
	auditList   = "auction-list"
	auditCancel = "auction-cancel"
	auditExpire = "auction-expire"
	auditSale   = "auction-sale"
	auditRefund = "auction-refund"
	auditRemove = "auction-remove"
)

// Manager orchestrates the auction house over the persisted Store: it
// validates and stages listings, charges fees and cuts through the currency
// seam, commits buyouts through escrow, and records every event in the
// shared audit log. It holds no lock of its own — the Store is the
// synchronization point (each of its methods is individually locked), and
// the Manager's operations are short, non-overlapping critical sequences
// driven from the serial command goroutine and the expiry tick.
type Manager struct {
	store *Store
	audit *escrow.AuditStore
	bus   escrow.Bus
	coin  CoinMover
	items *entities.Store
	tpls  *item.Templates
	clk   clock.Clock
	cfg   Config

	// tradable gates non-tradable items (nil → everything tradable).
	tradable func(entities.EntityID) bool
}

// NewManager wires a Manager. store must already be Loaded. bus drives the
// buyout's cancellable commit; audit records every event; coin is the
// currency seam; items serializes the listed instance and rehydrates it at
// collect; tpls rehydrates; tradable gates bound items.
func NewManager(store *Store, audit *escrow.AuditStore, bus escrow.Bus, coin CoinMover, items *entities.Store, tpls *item.Templates, clk clock.Clock, cfg Config, tradable func(entities.EntityID) bool) *Manager {
	return &Manager{
		store:    store,
		audit:    audit,
		bus:      bus,
		coin:     coin,
		items:    items,
		tpls:     tpls,
		clk:      clk,
		cfg:      cfg,
		tradable: tradable,
	}
}

// Config exposes the tunables (the verb layer reads PageSize for browse).
func (m *Manager) Config() Config { return m.cfg }

// List posts an item from the seller's inventory for buyout at price (§3).
// It validates the floor, the per-seller cap, and tradability; charges the
// non-refundable listing fee (refusing on insufficient coin BEFORE moving
// the item); then stages the item into persisted escrow custody — removing
// it from the seller's inventory and the live entity store, persisting it
// serialized in the listing. The item is gone from the world as a live
// entity until collected, so it cannot be duplicated.
//
// Ordering note (loss-free): the fee is charged first; if staging then fails
// the fee was already the cost of the attempt (it is non-refundable anyway).
// The item is removed from the bag only after the listing is durably
// written, so a write failure leaves the seller holding their item.
func (m *Manager) List(ctx context.Context, seller Party, inst *entities.ItemInstance, price int) error {
	if price < m.cfg.MinPrice {
		return ErrPriceTooLow
	}
	if m.tradable != nil && !m.tradable(inst.ID()) {
		return ErrNotTradable
	}
	if m.cfg.PerSellerCap > 0 && m.store.ActiveCountBySeller(seller.ID()) >= m.cfg.PerSellerCap {
		return ErrListingCap
	}

	// Charge the non-refundable fee first (the anti-spam gold sink, §9).
	if m.cfg.ListingFee > 0 {
		if _, ok := m.coin.Debit(ctx, seller, m.cfg.ListingFee, "auction:list-fee"); !ok {
			return ErrCantAfford
		}
	}

	id, err := m.store.NextID()
	if err != nil {
		m.refundFee(ctx, seller) // store unavailable — give the fee back, nothing happened.
		return fmt.Errorf("auction list: alloc id: %w", err)
	}

	now := m.now()
	listing := &Listing{
		ID:         id,
		Seller:     seller.ID(),
		SellerName: seller.Name(),
		Item:       Serialize(inst),
		Price:      price,
		Category:   inst.Type(),
		ListedAt:   now,
		ExpiresAt:  now.Add(m.cfg.Duration),
		Status:     StatusActive,
	}
	if err := m.store.Add(listing); err != nil {
		m.refundFee(ctx, seller) // listing not written — refund, leave the item in the bag.
		return fmt.Errorf("auction list: persist: %w", err)
	}

	// The listing is durably recorded — now remove the live item from the
	// world. Take it out of the bag, then untrack the entity (it lives only
	// as serialized listing data until a buyer/seller collects it).
	seller.RemoveFromInventory(inst.ID())
	if err := m.items.Untrack(inst.ID()); err != nil {
		// The item is already serialized + out of the bag; a stale live
		// entity is a leak, not value loss. Log loudly rather than fail.
		logging.From(ctx).Error("auction list: untrack failed — stale live entity",
			slog.String("listing", id), slog.String("item", string(inst.ID())),
			slog.String("err", err.Error()))
	}

	m.appendAudit(ctx, auditList, id, []escrow.AuditLeg{
		itemLeg(seller.ID(), "", listing.Item.Template),
		coinLeg(seller.ID(), "", m.cfg.ListingFee),
	})
	return nil
}

// Cancel pulls the seller's own active listing before it sells (§3). The
// item is earmarked for the seller to collect (pickup, §7) — it is not
// pushed back into the bag here — and the listing fee is NOT refunded. Only
// the owner may cancel; a non-active listing (sold/expired a tick earlier)
// reports ErrNotActive.
func (m *Manager) Cancel(ctx context.Context, seller Party, listingID string) error {
	l, ok := m.store.Get(listingID)
	if !ok {
		return ErrNotFound
	}
	if l.Seller != seller.ID() {
		return ErrNotYours
	}
	if err := m.store.MarkCancelled(listingID); err != nil {
		return err
	}
	m.appendAudit(ctx, auditCancel, listingID, []escrow.AuditLeg{
		itemLeg(seller.ID(), seller.ID(), l.Item.Template),
	})
	_ = seller.Write(ctx, fmt.Sprintf("You withdraw %s from auction. Collect it at an auctioneer.", l.Item.Name))
	return nil
}

// ListingsBySeller returns the seller's active listings (for the `auction`
// no-arg view and to map an ordinal in `unlist <n>`).
func (m *Manager) ListingsBySeller(playerID string) []Listing {
	all := m.store.ActiveListings()
	out := make([]Listing, 0, len(all))
	for _, l := range all {
		if l.Seller == playerID {
			out = append(out, l)
		}
	}
	return out
}

// refundFee returns a charged listing fee when listing fails AFTER the
// charge but BEFORE the item moved — the attempt did not happen, so the
// non-refundable rule does not apply. A no-op when there is no fee.
func (m *Manager) refundFee(ctx context.Context, seller Party) {
	if m.cfg.ListingFee > 0 {
		m.coin.AddGold(ctx, seller, m.cfg.ListingFee, "auction:list-fee-refund")
	}
}

// now reads the engine clock (F3), falling back to the zero time only when
// no clock is wired (tests that don't care about timestamps).
func (m *Manager) now() time.Time {
	if m.clk != nil {
		return m.clk.Now()
	}
	return time.Time{}
}

// appendAudit records one auction event in the shared trade-audit log (§10).
// A failed audit write is logged, never fatal — losing a bookkeeping line
// must not undo a completed action (mirrors escrow's audit-after-commit
// philosophy).
func (m *Manager) appendAudit(ctx context.Context, source, txnID string, legs []escrow.AuditLeg) {
	if m.audit == nil {
		return
	}
	rec := escrow.AuditRecord{TxnID: txnID, Source: source, Legs: legs}
	if err := m.audit.Append(rec); err != nil {
		logging.From(ctx).Error("auction: audit append failed",
			slog.String("source", source), slog.String("txn", txnID),
			slog.String("err", err.Error()))
	}
}

// itemLeg / coinLeg build audit legs. A coin leg with amount 0 is omitted by
// callers; here they are simple constructors.
func itemLeg(party, dest, item string) escrow.AuditLeg {
	return escrow.AuditLeg{Party: party, Dest: dest, Kind: "item", Item: item}
}

func coinLeg(party, dest string, amount int) escrow.AuditLeg {
	return escrow.AuditLeg{Party: party, Dest: dest, Kind: "coin", Amount: amount}
}

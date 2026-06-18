package trade

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/escrow"
)

// custodian implements escrow.Custodian for a single direct-trade session.
// All of its methods run under the escrow Transaction's mutex (escrow
// serializes every stage/withdraw/commit), so its ownerOf map needs no
// lock of its own.
//
// Item stage model: remove-at-stage. StageItem removes the item from the
// owner's inventory and records its owner, so no verb (in any package) can
// reach it while staged. DeliverItem adds it to the destination; ReturnItem
// adds it back to the owner. Coin model: debit-at-stage, so DeliverCoin is a
// credit-only (infallible) move.
type custodian struct {
	parties  map[string]Party                // playerID -> party
	coin     CoinMover                       // currency seam
	tradable func(id entities.EntityID) bool // nil → everything tradable
	ownerOf  map[entities.EntityID]string    // staged item -> owner playerID
}

func newCustodian(a, b Party, coin CoinMover, tradable func(entities.EntityID) bool) *custodian {
	return &custodian{
		parties:  map[string]Party{a.ID(): a, b.ID(): b},
		coin:     coin,
		tradable: tradable,
		ownerOf:  map[entities.EntityID]string{},
	}
}

// StageItem removes itemID from partyID's inventory (remove-at-stage) after
// checking the party holds it and it is tradable. Because the item leaves the
// inventory, a second stage of the same item (in any transaction) fails the
// holds check, and one-trade-per-player (the Manager) bounds it further.
func (c *custodian) StageItem(_ context.Context, partyID, itemID string) error {
	p := c.parties[partyID]
	if p == nil {
		return escrow.ErrItemGone
	}
	id := entities.EntityID(itemID)
	if !holds(p, id) {
		return escrow.ErrItemGone
	}
	if c.tradable != nil && !c.tradable(id) {
		return escrow.ErrItemBound
	}
	p.RemoveFromInventory(id)
	c.ownerOf[id] = partyID
	return nil
}

// ReturnItem adds a staged item back to partyID (its owner).
func (c *custodian) ReturnItem(_ context.Context, partyID, itemID string) error {
	p := c.parties[partyID]
	if p == nil {
		return escrow.ErrItemGone
	}
	p.AddToInventory(entities.EntityID(itemID))
	delete(c.ownerOf, entities.EntityID(itemID))
	return nil
}

// DeliverItem adds a staged item to destPartyID (it was removed from its
// owner at stage time).
func (c *custodian) DeliverItem(_ context.Context, destPartyID, itemID string) error {
	dest := c.parties[destPartyID]
	if dest == nil {
		return escrow.ErrItemGone
	}
	dest.AddToInventory(entities.EntityID(itemID))
	return nil
}

// TakeItem removes itemID from holderID — used only to reverse an
// already-delivered item during a mid-commit unwind.
func (c *custodian) TakeItem(_ context.Context, holderID, itemID string) error {
	h := c.parties[holderID]
	if h == nil {
		return escrow.ErrItemGone
	}
	if !h.RemoveFromInventory(entities.EntityID(itemID)) {
		return escrow.ErrItemGone
	}
	return nil
}

// ReserveCoin debits amount from partyID at stage time (debit-at-stage), so
// the coin is genuinely inert and DeliverCoin cannot fail.
func (c *custodian) ReserveCoin(ctx context.Context, partyID string, amount int) error {
	p := c.parties[partyID]
	if p == nil {
		return escrow.ErrInsufficientCoin
	}
	if _, ok := c.coin.Debit(ctx, p, amount, "trade:stage"); !ok {
		return escrow.ErrInsufficientCoin
	}
	return nil
}

// ReturnCoin credits reserved coin back to its owner (offer withdrawn /
// trade aborted).
func (c *custodian) ReturnCoin(ctx context.Context, partyID string, amount int) {
	if p := c.parties[partyID]; p != nil {
		c.coin.AddGold(ctx, p, amount, "trade:return")
	}
}

// DeliverCoin credits already-reserved coin to the destination (commit).
func (c *custodian) DeliverCoin(ctx context.Context, destPartyID string, amount int) {
	if p := c.parties[destPartyID]; p != nil {
		c.coin.AddGold(ctx, p, amount, "trade:deliver")
	}
}

// compile-time assertions.
var (
	_ escrow.Custodian = (*custodian)(nil)
	_ economy.Entity   = Party(nil)
)

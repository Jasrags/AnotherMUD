// Package trade is the synchronous, same-room player-to-player swap
// (docs/specs/direct-trade.md) — the simple, transient consumer of the
// escrow/atomic-transaction primitive (internal/escrow). Two present players
// each build an offer of items + coin, see each other's offer, and confirm;
// when both have confirmed an unchanged pair the swap commits atomically
// through escrow, or rolls both whole on any failure.
//
// It is intentionally zero-sum (no fee, no gold sink — that is the auction
// house) and never persisted: a session lives only in memory, and an
// interrupted trade simply never happened (§6).
//
// Stage semantics (the key design choice, §3): a staged item and staged coin
// are both removed-at-stage — the item leaves the owner's inventory and the
// coin is debited — so each is genuinely inert and NO other verb (drop, give,
// equip, sell, put, consume — including the cross-package ShopService) can
// reach it, with zero per-verb guarding and no dupe vector. The cost is a
// deliberate deviation from §6's "an interrupted trade leaves staged value
// with its owner": a hard crash mid-trade loses staged value, exactly the
// window the engine already tolerates for give/shop-buy. Graceful teardown
// (disconnect / link-death / room change) cancels the trade and returns
// everything, so only an abrupt kill -9 loses anything. The escrow primitive
// is agnostic to this — it is the trade.custodian that implements "remove
// from reach".
package trade

import (
	"context"
	"slices"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Party is one side of a trade. The live connActor satisfies it; a verb
// type-asserts c.Actor to Party (the command.Actor interface deliberately
// does not expose gold — currency callers type-assert economy.Entity, and so
// does trade). Staging removes an item from Inventory (remove-at-stage), so
// the Party surface needs no special "lock" method — the item is simply gone
// from the owner's reach until the trade commits or rolls back.
type Party interface {
	economy.Entity // ID() string, Gold() int, SetGold(int)

	Name() string
	Write(ctx context.Context, msg string) error

	Inventory() []entities.EntityID
	AddToInventory(id entities.EntityID)
	RemoveFromInventory(id entities.EntityID) bool
}

// holds reports whether p currently has id in its inventory.
func holds(p Party, id entities.EntityID) bool {
	return slices.Contains(p.Inventory(), id)
}

// CoinMover is the currency seam the custodian moves coin through.
// *economy.CurrencyService satisfies it.
type CoinMover interface {
	Debit(ctx context.Context, e economy.Entity, amount int, reason string) (int, bool)
	AddGold(ctx context.Context, e economy.Entity, delta int, reason string) int
}

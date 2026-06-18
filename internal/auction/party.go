package auction

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Party is a participant the auction acts on — a seller listing or
// cancelling, a buyer buying, anyone collecting. The live connActor
// satisfies it (it already satisfies the sibling trade.Party). The verb
// layer type-asserts c.Actor to Party.
//
// Unlike direct-trade, the auction never needs both parties present at once
// — a seller may be offline when a buyer buys. The interface therefore only
// describes operations on the ONE acting player; the counterparty's coin
// (proceeds) and goods (pickup) wait in the persisted store until that
// player is next at an access point.
type Party interface {
	economy.Entity // ID() string, Gold() int, SetGold(int)

	Name() string
	Write(ctx context.Context, msg string) error

	Inventory() []entities.EntityID
	AddToInventory(id entities.EntityID)
	RemoveFromInventory(id entities.EntityID) bool
}

// CoinMover is the currency seam fees + proceeds move through.
// *economy.CurrencyService satisfies it.
type CoinMover interface {
	Debit(ctx context.Context, e economy.Entity, amount int, reason string) (int, bool)
	AddGold(ctx context.Context, e economy.Entity, delta int, reason string) int
}

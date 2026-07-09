package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/trade"
)

// Direct-trade verbs (docs/specs/direct-trade.md). They are thin: each
// resolves the actor (and, for `trade`, the target) to a trade.Party and
// routes to the session Manager, which owns the session lifecycle, the
// confirm-reset rule, and all player notifications. The handlers only map a
// returned sentinel error to a player-facing line; on success the Manager
// has already written the relevant messages.
//
// The verb set is configuration per the spec; v1 uses: trade <player>
// (initiate / symmetric-accept), offer <item> / offergold <n> (add to
// offer), rescind <item> / rescindgold <n> (remove from offer), confirm,
// and decline (cancel).

// tradeParty asserts the actor onto trade.Party (the live connActor
// satisfies it; bare test stubs without a gold account do not).
func tradeParty(c *Context) (trade.Party, bool) {
	p, ok := c.Actor.(trade.Party)
	return p, ok
}

// mapTradeErr writes a player-facing line for a Manager sentinel, or nil
// when the Manager already handled messaging (err == nil).
func mapTradeErr(ctx context.Context, c *Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, trade.ErrSelf):
		return c.Actor.Write(ctx, "You can't trade with yourself.")
	case errors.Is(err, trade.ErrAlreadyTrading):
		return c.Actor.Write(ctx, "You're already in a trade. Finish or `decline` it first.")
	case errors.Is(err, trade.ErrPartnerBusy):
		return c.Actor.Write(ctx, "They're already trading with someone else.")
	case errors.Is(err, trade.ErrNotTrading):
		return c.Actor.Write(ctx, "You're not trading with anyone.")
	case errors.Is(err, trade.ErrNotInOffer):
		return c.Actor.Write(ctx, "That isn't in your offer.")
	default:
		return c.Actor.Write(ctx, "You can't do that right now.")
	}
}

// TradeHandler implements `trade <player>` — initiate a trade, or accept a
// pending request from that player (the symmetric handshake). Same-room only.
func TradeHandler(ctx context.Context, c *Context) error {
	if c.Trades == nil {
		return c.Actor.Write(ctx, "You can't trade right now.")
	}
	me, ok := tradeParty(c)
	if !ok {
		return c.Actor.Write(ctx, "You can't trade.")
	}
	room := c.Actor.Room()
	if room == nil || c.Locator == nil {
		return c.Actor.Write(ctx, "There's no one here to trade with.")
	}
	tref, _ := c.Resolved["target"].(EntityRef)
	target := c.Locator.FindInRoom(room.ID, tref.Name)
	if target == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("%q isn't here.", tref.Name))
	}
	tp, ok := target.(trade.Party)
	if !ok {
		return c.Actor.Write(ctx, "You can't trade with them.")
	}
	return mapTradeErr(ctx, c, c.Trades.Initiate(ctx, me, tp))
}

// OfferItemHandler implements `offer <item>` — stage an inventory item into
// the offer (it becomes inert).
func OfferItemHandler(ctx context.Context, c *Context) error {
	me, ok := offerActor(ctx, c)
	if !ok {
		return nil
	}
	it, ok := resolvedItemInstance(c, "item")
	if !ok {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	return mapTradeErr(ctx, c, c.Trades.OfferItem(ctx, me, it.ID()))
}

// RescindItemHandler implements `rescind <item>` — withdraw a staged item
// back from the offer. A staged item has left the inventory (remove-at-stage),
// so it is resolved by keyword against the staged offer, not the inventory.
func RescindItemHandler(ctx context.Context, c *Context) error {
	me, ok := offerActor(ctx, c)
	if !ok {
		return nil
	}
	query, _ := c.Resolved["item"].(string)
	if query == "" {
		return c.Actor.Write(ctx, "Rescind what?")
	}
	return mapTradeErr(ctx, c, c.Trades.WithdrawItemByQuery(ctx, me, query))
}

// OfferGoldHandler implements `offergold <amount>` — stage coin into the
// offer (debited immediately).
func OfferGoldHandler(ctx context.Context, c *Context) error {
	me, ok := offerActor(ctx, c)
	if !ok {
		return nil
	}
	amount, _ := c.Resolved["amount"].(int)
	if amount <= 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("Offer how much %s?", c.Money.Name()))
	}
	return mapTradeErr(ctx, c, c.Trades.OfferCoin(ctx, me, amount))
}

// RescindGoldHandler implements `rescindgold <amount>` — withdraw staged
// coin back from the offer.
func RescindGoldHandler(ctx context.Context, c *Context) error {
	me, ok := offerActor(ctx, c)
	if !ok {
		return nil
	}
	amount, _ := c.Resolved["amount"].(int)
	if amount <= 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("Take back how much %s?", c.Money.Name()))
	}
	return mapTradeErr(ctx, c, c.Trades.WithdrawCoin(ctx, me, amount))
}

// ConfirmHandler implements `confirm` — confirm the current offers; the swap
// fires only when both sides are confirmed against an unchanged pair.
func ConfirmHandler(ctx context.Context, c *Context) error {
	me, ok := offerActor(ctx, c)
	if !ok {
		return nil
	}
	return mapTradeErr(ctx, c, c.Trades.Confirm(ctx, me))
}

// DeclineHandler implements `decline` — cancel the trade (or a pending
// request), returning all staged value.
func DeclineHandler(ctx context.Context, c *Context) error {
	me, ok := offerActor(ctx, c)
	if !ok {
		return nil
	}
	return mapTradeErr(ctx, c, c.Trades.Cancel(ctx, me))
}

// offerActor is the shared guard for the offer/confirm/decline verbs: it
// reports the trade.Party, or writes the "can't trade" line and returns
// ok=false.
func offerActor(ctx context.Context, c *Context) (trade.Party, bool) {
	if c.Trades == nil {
		_ = c.Actor.Write(ctx, "You can't trade right now.")
		return nil, false
	}
	me, ok := tradeParty(c)
	if !ok {
		_ = c.Actor.Write(ctx, "You can't trade.")
		return nil, false
	}
	return me, true
}

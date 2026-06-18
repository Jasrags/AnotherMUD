package command

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/auction"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Auction-house verbs (docs/specs/auction-house.md). They are thin: each
// confirms the actor is at an access point (§2), resolves the actor to an
// auction.Party, and routes to the Manager, which owns the listing
// lifecycle, the fees, the escrow commit, and the audit log. Browse/buy/
// collect land in later slices; this file ships list / view / cancel.
//
// Verb set (configuration per the spec): `auction <item> <price>` lists,
// `auctions` views your own active listings, `unlist <ref>` cancels one.
// The buyout verb is `buyout` (B2) — distinct from the shop `buy` to avoid
// a collision at a room that is both a shop and an auctioneer.

// auctionParty asserts the actor onto auction.Party (the live connActor
// satisfies it).
func auctionParty(c *Context) (auction.Party, bool) {
	p, ok := c.Actor.(auction.Party)
	return p, ok
}

// atAuctioneer reports whether the actor stands at an access point: a room
// tagged TagAuctionHouse, or a room holding an NPC tagged TagAuctioneer
// (§2). On false it writes the "go to an auctioneer" pointer.
func atAuctioneer(ctx context.Context, c *Context) bool {
	room := c.Actor.Room()
	if room != nil {
		if room.HasTag(auction.TagAuctionHouse) {
			return true
		}
		if auctioneerInRoom(c, room.ID) {
			return true
		}
	}
	_ = c.Actor.Write(ctx, "You need to be at an auctioneer to do that.")
	return false
}

// auctioneerInRoom reports whether an auctioneer-tagged NPC stands in roomID.
func auctioneerInRoom(c *Context, roomID world.RoomID) bool {
	if c.Items == nil || c.Placement == nil {
		return false
	}
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		if mob, ok := e.(*entities.MobInstance); ok && hasTag(mob.Tags(), auction.TagAuctioneer) {
			return true
		}
	}
	return false
}

// mapAuctionErr writes a player-facing line for a Manager sentinel, or nil
// when the Manager already handled messaging (err == nil).
func mapAuctionErr(ctx context.Context, c *Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, auction.ErrPriceTooLow):
		return c.Actor.Write(ctx, fmt.Sprintf("The minimum auction price is %d gold.", c.Auction.Config().MinPrice))
	case errors.Is(err, auction.ErrNotTradable):
		return c.Actor.Write(ctx, "That item can't be auctioned.")
	case errors.Is(err, auction.ErrListingCap):
		return c.Actor.Write(ctx, "You already have as many auctions running as you're allowed.")
	case errors.Is(err, auction.ErrCantAfford):
		return c.Actor.Write(ctx, fmt.Sprintf("You can't afford the %d gold listing fee.", c.Auction.Config().ListingFee))
	case errors.Is(err, auction.ErrNotYours):
		return c.Actor.Write(ctx, "That isn't your auction.")
	case errors.Is(err, auction.ErrNotFound):
		return c.Actor.Write(ctx, "There's no such auction.")
	case errors.Is(err, auction.ErrNotActive):
		return c.Actor.Write(ctx, "That auction is no longer active.")
	default:
		return c.Actor.Write(ctx, "You can't do that right now.")
	}
}

// AuctionHandler implements `auction <item> <price>` — list an inventory
// item for buyout at an access point.
func AuctionHandler(ctx context.Context, c *Context) error {
	if c.Auction == nil {
		return c.Actor.Write(ctx, "There's no auction house here.")
	}
	me, ok := auctionParty(c)
	if !ok {
		return c.Actor.Write(ctx, "You can't use the auction house.")
	}
	if !atAuctioneer(ctx, c) {
		return nil
	}
	it, ok := resolvedItemInstance(c, "item")
	if !ok {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	price, _ := c.Resolved["price"].(int)
	if price <= 0 {
		return c.Actor.Write(ctx, "List it for how much? (auction <item> <price>)")
	}
	if err := c.Auction.List(ctx, me, it, price); err != nil {
		return mapAuctionErr(ctx, c, err)
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You list %s for %d gold.", it.Name(), price))
}

// AuctionsHandler implements `auctions` — show the caller's own active
// listings with the ordinal `unlist` accepts.
func AuctionsHandler(ctx context.Context, c *Context) error {
	if c.Auction == nil {
		return c.Actor.Write(ctx, "There's no auction house here.")
	}
	me, ok := auctionParty(c)
	if !ok {
		return c.Actor.Write(ctx, "You can't use the auction house.")
	}
	mine := c.Auction.ListingsBySeller(me.ID())
	if len(mine) == 0 {
		return c.Actor.Write(ctx, "You have no items up for auction.")
	}
	var b strings.Builder
	b.WriteString("Your auctions:\n")
	for i, l := range mine {
		fmt.Fprintf(&b, " %2d  %-28s %d gold\n", i+1, l.Item.Name, l.Price)
	}
	b.WriteString("(unlist <#> to withdraw one)")
	return c.Actor.Write(ctx, b.String())
}

// UnlistHandler implements `unlist <ref>` — cancel one of the caller's own
// active listings, named by its `auctions` ordinal or its raw id.
func UnlistHandler(ctx context.Context, c *Context) error {
	if c.Auction == nil {
		return c.Actor.Write(ctx, "There's no auction house here.")
	}
	me, ok := auctionParty(c)
	if !ok {
		return c.Actor.Write(ctx, "You can't use the auction house.")
	}
	if !atAuctioneer(ctx, c) {
		return nil
	}
	ref, _ := c.Resolved["ref"].(string)
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return c.Actor.Write(ctx, "Withdraw which auction? (see `auctions`)")
	}
	id := resolveListingRef(c.Auction.ListingsBySeller(me.ID()), ref)
	if id == "" {
		return c.Actor.Write(ctx, "You have no auction matching that.")
	}
	return mapAuctionErr(ctx, c, c.Auction.Cancel(ctx, me, id))
}

// resolveListingRef maps a player-supplied reference to a listing id: a
// 1-based ordinal into the seller's own listings, or a raw id match.
func resolveListingRef(mine []auction.Listing, ref string) string {
	if n, err := strconv.Atoi(ref); err == nil {
		if n >= 1 && n <= len(mine) {
			return mine[n-1].ID
		}
		return ""
	}
	for _, l := range mine {
		if l.ID == ref {
			return l.ID
		}
	}
	return ""
}

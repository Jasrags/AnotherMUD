package command

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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
		if c.questSpawnBlockedFrom(e) {
			continue // foreign quest spawn — not interactable (quest-spawns.md Phase 2)
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
		return c.Actor.Write(ctx, fmt.Sprintf("The minimum auction price is %s.", c.Money.Format(c.Auction.Config().MinPrice)))
	case errors.Is(err, auction.ErrNotTradable):
		return c.Actor.Write(ctx, "That item can't be auctioned.")
	case errors.Is(err, auction.ErrListingCap):
		return c.Actor.Write(ctx, "You already have as many auctions running as you're allowed.")
	case errors.Is(err, auction.ErrCantAfford):
		return c.Actor.Write(ctx, fmt.Sprintf("You can't afford the %s listing fee.", c.Money.Format(c.Auction.Config().ListingFee)))
	case errors.Is(err, auction.ErrNotYours):
		return c.Actor.Write(ctx, "That isn't your auction.")
	case errors.Is(err, auction.ErrOwnListing):
		return c.Actor.Write(ctx, "That's your own listing — use `unlist` to withdraw it.")
	case errors.Is(err, auction.ErrInsufficientCoin):
		return c.Actor.Write(ctx, "You can't afford that.")
	case errors.Is(err, auction.ErrVetoed):
		return c.Actor.Write(ctx, "The purchase was refused.")
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
	return c.Actor.Write(ctx, fmt.Sprintf("You list %s for %s.", it.Name(), c.Money.Format(price)))
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
	if !atAuctioneer(ctx, c) {
		return nil
	}
	mine := c.Auction.ListingsBySeller(me.ID())
	if len(mine) == 0 {
		return c.Actor.Write(ctx, "You have no items up for auction.")
	}
	var b strings.Builder
	b.WriteString("Your auctions:\n")
	for i, l := range mine {
		fmt.Fprintf(&b, " %2d  %-28s %s\n", i+1, l.Item.Name, c.Money.Format(l.Price))
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

// BrowseHandler implements `browse [name] [price|time] [page] [cat:<type>]`
// — the buyer's view of the global market (§5). search is an alias narrowing
// by name.
func BrowseHandler(ctx context.Context, c *Context) error {
	if c.Auction == nil {
		return c.Actor.Write(ctx, "There's no auction house here.")
	}
	if _, ok := auctionParty(c); !ok {
		return c.Actor.Write(ctx, "You can't use the auction house.")
	}
	if !atAuctioneer(ctx, c) {
		return nil
	}
	now := nowFromCtx(c)
	f := parseBrowseArgs(c.Args)
	page := c.Auction.Browse(now, f)
	return c.Actor.Write(ctx, renderBrowsePage(page, now))
}

// BuyoutHandler implements `buyout <#>` — buy a listing outright by its
// browse reference (§6). Distinct from the shop `buy` so the two coexist at
// a room that is both shop and auctioneer.
func BuyoutHandler(ctx context.Context, c *Context) error {
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
	id := c.Auction.FindActiveByRef(ref)
	if id == "" {
		return c.Actor.Write(ctx, "There's no auction with that number. (see `browse`)")
	}
	won, err := c.Auction.Buyout(ctx, me, id)
	if err != nil {
		return mapAuctionErr(ctx, c, err)
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You win %s for %s. Collect it at an auctioneer.", won.Item.Name, c.Money.Format(won.Price)))
}

// CollectHandler implements `collect` — claim everything waiting for the
// caller at an access point (§7): proceeds from sales, won items, and
// returned (expired/cancelled) items. Items pass the normal carry-weight
// gate; what doesn't fit stays in escrow for a later collect and is never
// lost.
func CollectHandler(ctx context.Context, c *Context) error {
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

	var got []string
	if coin := c.Auction.CollectCoin(ctx, me); coin > 0 {
		got = append(got, c.Money.Format(coin))
	}

	heldBack := false
	for _, l := range c.Auction.PendingPickups(me.ID()) {
		inst, err := c.Auction.RehydratePickup(ctx, l)
		if err != nil {
			// Template vanished since listing — leave it for an operator
			// (the item is still recorded in the store), keep collecting.
			continue
		}
		if c.carryWeightExceeded(inst) {
			_ = c.Items.Untrack(inst.ID()) // not delivered — drop the live copy.
			heldBack = true
			continue // this one stays waiting; keep collecting lighter items.
		}
		if err := c.Auction.ConfirmItemCollected(ctx, me, l, inst.ID()); err != nil {
			_ = c.Items.Untrack(inst.ID())
			heldBack = true
			continue
		}
		me.AddToInventory(inst.ID())
		got = append(got, inst.Name())
	}

	if len(got) == 0 {
		if heldBack {
			return c.Actor.Write(ctx, "You can't carry any more right now; your goods are still waiting.")
		}
		return c.Actor.Write(ctx, "There's nothing waiting for you here.")
	}
	msg := "You collect " + strings.Join(got, ", ") + "."
	if heldBack {
		msg += " (You couldn't carry the rest — it's still waiting.)"
	}
	return c.Actor.Write(ctx, msg)
}

// parseBrowseArgs classifies loose browse tokens: a number is the page,
// "price"/"time" the sort, "cat:<type>" the category, anything else joins
// into the name substring (§5 example: `browse sword price`).
func parseBrowseArgs(args []string) auction.BrowseFilter {
	f := auction.BrowseFilter{}
	var nameParts []string
	for _, a := range args {
		la := strings.ToLower(a)
		switch {
		case la == "price":
			f.Sort = auction.SortByPrice
		case la == "time":
			f.Sort = auction.SortByTime
		case strings.HasPrefix(la, "cat:"):
			f.Category = strings.TrimPrefix(la, "cat:")
		default:
			if n, err := strconv.Atoi(a); err == nil {
				f.Page = n
				continue
			}
			nameParts = append(nameParts, a)
		}
	}
	f.Name = strings.Join(nameParts, " ")
	return f
}

// renderBrowsePage formats one page of listings (§5): a header, then a line
// per listing with its stable reference, name, price, and time remaining.
func renderBrowsePage(p auction.BrowsePage, now time.Time) string {
	if p.Total == 0 {
		return "The auction house has nothing matching that."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Auction House — page %d/%d — %d listing(s)\n", p.Page, p.TotalPages, p.Total)
	fmt.Fprintf(&b, " %-6s %-30s %10s  %s\n", "Ref", "Item", "Price", "Closes in")
	for _, l := range p.Listings {
		ref := strings.TrimPrefix(l.ID, "au-")
		fmt.Fprintf(&b, " %-6s %-30s %10d  %s\n", ref, browseItemName(l), l.Price, closesIn(l, now))
	}
	b.WriteString("(buyout <#> to buy | browse <name> price|time <page>)")
	return b.String()
}

// browseItemName renders the listing's item name with a leading [RARITY]
// marker when the serialized item carries a rarity decoration (§5 renders
// with decorations). Falls back to the bare name.
func browseItemName(l auction.Listing) string {
	if r, ok := l.Item.Properties["rarity"].(string); ok && r != "" && !strings.EqualFold(r, "common") {
		return "[" + strings.ToUpper(r) + "] " + l.Item.Name
	}
	return l.Item.Name
}

// closesIn formats the time remaining until a listing expires (e.g. "2h 14m")
// from the stored ExpiresAt against now.
func closesIn(l auction.Listing, now time.Time) string {
	d := l.ExpiresAt.Sub(now)
	if d <= 0 {
		return "closing"
	}
	h := int(d.Hours())
	min := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, min)
	}
	return fmt.Sprintf("%dm", min)
}

// AuctionRemoveHandler implements `auctionremove <#>` (admin, §11) — force a
// listing off the market; the item returns to the seller. Role-gated by the
// dispatcher (Admin verb).
func AuctionRemoveHandler(ctx context.Context, c *Context) error {
	me, ok := adminAuctionActor(ctx, c)
	if !ok {
		return nil
	}
	ref, _ := c.Resolved["ref"].(string)
	id := normalizeListingRef(ref)
	if id == "" {
		return c.Actor.Write(ctx, "Remove which auction? (auctionremove <#>)")
	}
	l, err := c.Auction.AdminRemove(ctx, me, id)
	if err != nil {
		return mapAuctionErr(ctx, c, err)
	}
	return c.Actor.Write(ctx, fmt.Sprintf("Removed auction %s (%s); item returned to %s.", id, l.Item.Name, l.SellerName))
}

// AuctionRefundHandler implements `auctionrefund <#>` (admin, §11) — reverse
// a sale: coin back to the buyer, item back to the seller.
func AuctionRefundHandler(ctx context.Context, c *Context) error {
	me, ok := adminAuctionActor(ctx, c)
	if !ok {
		return nil
	}
	ref, _ := c.Resolved["ref"].(string)
	id := normalizeListingRef(ref)
	if id == "" {
		return c.Actor.Write(ctx, "Refund which auction? (auctionrefund <#>)")
	}
	l, err := c.Auction.AdminRefund(ctx, me, id)
	if err != nil {
		if errors.Is(err, auction.ErrCannotRefund) {
			return c.Actor.Write(ctx, "That sale can't be auto-reversed (already collected). Check the audit log.")
		}
		return mapAuctionErr(ctx, c, err)
	}
	return c.Actor.Write(ctx, fmt.Sprintf("Reversed the sale of %s; buyer refunded, item returned to %s.", l.Item.Name, l.SellerName))
}

// adminAuctionActor is the shared guard for the admin auction verbs.
func adminAuctionActor(ctx context.Context, c *Context) (auction.Party, bool) {
	if c.Auction == nil {
		_ = c.Actor.Write(ctx, "The auction house isn't available.")
		return nil, false
	}
	me, ok := auctionParty(c)
	if !ok {
		_ = c.Actor.Write(ctx, "You can't do that.")
		return nil, false
	}
	return me, true
}

// normalizeListingRef maps a numeric ref to a listing id ("5" → "au-5"), or
// passes a full id through. Unlike a buyout ref it does not require the
// listing be active (admin acts on sold/expired too); the Manager checks the
// status.
func normalizeListingRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if _, err := strconv.Atoi(ref); err == nil {
		return "au-" + ref
	}
	return ref
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

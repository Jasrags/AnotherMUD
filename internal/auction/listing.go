// Package auction is the asynchronous, persisted, fee-bearing player
// marketplace (docs/specs/auction-house.md) — the second consumer of the
// shared escrow/atomic-transaction primitive (internal/escrow), after the
// synchronous direct-trade sibling.
//
// A seller lists a real item instance at an auctioneer for a fixed buyout
// price; the item leaves their pack and is held — serialized — by the
// listing until it sells or is returned. A buyer purchases it later (the
// seller need not be online); coin moves to the seller minus a sale cut,
// and goods + proceeds wait to be collected at any access point. Unsold
// listings expire via a tick handler and return to the seller. Fees (a
// non-refundable listing fee + a sale cut) are the economy's primary gold
// sink.
//
// This file holds the durable record types. The escrowed item is stored
// SERIALIZED (template + per-instance property bag), not as a live entity:
// while held it exists only as listing data, so it is never duplicated into
// the entity store and survives a reboot intact (§4). It is rehydrated into
// a live ItemInstance lazily, at collect time (serialize.go).
package auction

import "time"

// Access-point tags (§2). An auctioneer is content: either a room carrying
// TagAuctionHouse or an NPC carrying TagAuctioneer makes that location an
// access point. The engine provides the mechanism; which rooms/NPCs are
// auctioneers is pack data. Reusing the room-tag / tagged-entity substrate
// means the auction house needs no furniture system.
const (
	TagAuctionHouse = "auction_house"
	TagAuctioneer   = "auctioneer"
)

// Status is a listing's lifecycle state (§4).
type Status string

const (
	// StatusActive — for sale; the item is held, the expiry clock runs.
	StatusActive Status = "active"
	// StatusSold — bought out; the item is earmarked for the buyer to
	// collect and the proceeds are credited to the seller's pending coin.
	StatusSold Status = "sold"
	// StatusExpired — the duration elapsed unsold; the item is earmarked
	// for the seller to collect. The listing fee is not refunded.
	StatusExpired Status = "expired"
	// StatusCancelled — the seller pulled an unsold listing; the item is
	// earmarked for the seller to collect. The listing fee is not refunded.
	StatusCancelled Status = "cancelled"
)

// SerializedItem is the durable form of an escrowed item instance: the
// template it was built from plus the per-instance property bag, which
// carries everything mutated after spawn — the quality grade (incl. a
// craft's override), decorations (rarity/essence attach as reserved
// properties), a container/light fill amount, condition, and so on (§4
// "with its property bag + decorations, intact"). The reserved keys
// (template_id / room_id) are excluded at serialize time and never overlaid
// at rehydrate (serialize.go).
//
// Name is a display cache so browse/search can render a listing without
// rehydrating a live entity; it is authoritative only for display, the
// template + properties are authoritative for the rehydrated item.
//
// v1 scope: a listed container is rehydrated EMPTY — nested contents are not
// serialized (a seller lists a single item, not a packed container). If
// container listing is wanted later, Contents recurses here the way
// player.InventoryEntry does.
type SerializedItem struct {
	Template   string         `yaml:"template"`
	Name       string         `yaml:"name,omitempty"`
	Properties map[string]any `yaml:"properties,omitempty"`
}

// Listing is one auction record — long-lived world data that MUST survive
// reboots (§4). It is versioned and migratable like a player save: the
// record carries a version and the loader migrates old records forward
// rather than dropping them (their escrowed items represent real player
// value that must not be lost).
type Listing struct {
	// Version is the on-disk schema version of this record. Bump
	// CurrentListingVersion and add a migration when the shape changes;
	// never edit a shipped record in place.
	Version int `yaml:"version"`

	ID         string `yaml:"id"`
	Seller     string `yaml:"seller"`      // seller playerID
	SellerName string `yaml:"seller_name"` // display name (seller may be offline)

	Item     SerializedItem `yaml:"item"`
	Price    int            `yaml:"price"`              // buyout price in gold
	Category string         `yaml:"category,omitempty"` // item type, for browse filtering

	ListedAt  time.Time `yaml:"listed_at"`
	ExpiresAt time.Time `yaml:"expires_at"`

	Status Status `yaml:"status"`

	// Collector is the playerID entitled to collect the held item: the
	// seller for an expired/cancelled listing, the buyer for a sold one.
	// Empty while active.
	Collector string `yaml:"collector,omitempty"`
	// Buyer is recorded on a sold listing for the audit trail; Collector
	// equals it for a sale.
	Buyer string `yaml:"buyer,omitempty"`
	// ItemCollected is set once the held item has been claimed; a fully
	// collected non-active listing is prunable.
	ItemCollected bool `yaml:"item_collected,omitempty"`
}

// IsActive reports whether the listing is for sale.
func (l *Listing) IsActive() bool { return l.Status == StatusActive }

// HeldForPickup reports whether the listing still holds an uncollected item
// waiting for its Collector (a terminal status whose item has not been
// claimed yet).
func (l *Listing) HeldForPickup() bool {
	return l.Status != StatusActive && !l.ItemCollected
}

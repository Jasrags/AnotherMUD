# Auction House — Feature Specification

**Status:** Draft · **Scope:** An asynchronous player marketplace — a seller
lists an item now and a buyer purchases it later, with no requirement that
the two be online together; the persisted listing store; the access point;
browse/search; buyout purchase; tick-driven expiry with returns; fees as the
economy's gold sink; and admin moderation · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the auction house must do, not *how* to
implement it. All tunables live in the configuration-surface table at §12.

The auction house is the **asynchronous, persisted** consumer of the shared
escrow/transaction primitive in `trade-escrow.md`; it does **not** redefine
that primitive — a listing holds its item in that escrow, and a purchase
commits through it. Where `direct-trade.md` is synchronous, transient, and
zero-sum, the auction house is asynchronous, persisted, and **fee-bearing**:
its fees are the **primary gold sink** that plugs into the existing economy
(`economy-survival.md`).

**v1 shape (each an open question below, defaulted here so the spec isn't
blocked):** a single **global** market; **buyout-only** purchase (no
bidding); **pickup at the auctioneer** for delivering goods and proceeds
(the notification queue carries the *notice*, not the goods).

---

## 1. Overview

A seller takes an item to an **access point** (an auctioneer), pays a
non-refundable **listing fee**, and posts it for a fixed **buyout price** for
a fixed **duration**. The item goes into persisted escrow. Other players
**browse/search** the listings and **buy** one outright; the buyer's coin
moves to the seller minus a **sale cut**, and the goods and proceeds are
**collected at the access point**. If a listing's duration elapses unsold, it
**expires** and the item returns to the seller. At every step a
**notification** tells the affected offline-or-online player what happened.

### 1.1 What the auction house is *not* (v1)

- **No anti-sniping.** No bid extensions, soft-close, or last-second
  protection. (Buyout-only v1 has no bidding, so this is moot now and stays
  excluded if bidding lands later.)
- **No bot / RMT detection.** The audit log (`trade-escrow.md` §5) makes
  investigation *possible*; the auction house does not itself detect or judge
  automated trading or real-money trade.
- **No market-manipulation defenses.** No price floors/ceilings, no
  wash-trade detection, no anti-corner logic. The market is what players make
  it; abuse is an operator/moderation concern (§11 admin moderation), not an engine
  feature in v1.
- **Not direct trade.** No co-presence, no negotiation, no confirm/counter.
  That is `direct-trade.md`.
- **Not an NPC vendor.** Prices and stock come from players, not from a shop
  definition (`economy-survival.md` shops); the auctioneer brokers player
  goods, it does not sell its own.
- **Not mail.** It does not push goods to a player's inbox; v1 delivers by
  pickup (§7). (Push delivery shares a substrate with mail-with-attachments —
  a deferred dependency — see Open questions.)

---

## 2. The access point

The market is reached through an **access point** placed by content — an
**auctioneer**: either an NPC carrying an auctioneer tag, or a room carrying
an auction-house tag. This reuses the **temporary-entity + room-tag**
substrate (the same trick crafting stations use), so the auction house needs
**no furniture system** and no new placement machinery.

- Listing, browsing, buying, and **collecting** (§7) happen at an access
  point. A player not at one is told where to go, not allowed to act
  remotely (v1).
- Whether the market is one global pool reachable from any access point, or
  per-location pools, is an Open question; **v1 is global** — every access point shows
  the same listings.
- The concrete auctioneers (which NPC, which rooms) are **content**; the
  engine provides the "this is an access point" mechanism.

**Acceptance — access point**

- [ ] Listing/browsing/buying/collecting require being at an access point;
      acting from elsewhere is refused with a pointer to one.
- [ ] An access point is content (a tagged NPC or room); no furniture system
      is required.
- [ ] In v1, all access points present the same (global) set of listings.

---

## 3. Listing an item

- A seller at an access point lists a **real item instance** from their
  inventory at a **buyout price**, for a configured **duration**. Listing
  **stages the item into persisted escrow** (`trade-escrow.md` §2): it leaves
  the seller's inventory immediately and is held by the listing until sold or
  returned. A non-tradable/bound item is refused at listing time.
- The seller pays a **non-refundable listing fee** (§9) at listing time — a
  gold sink. Insufficient coin refuses the listing.
- A seller may hold at most a configured **per-player listing cap** of active
  listings at once.
- A seller may **cancel their own** active listing before it sells; the item
  returns (via pickup, §7) and the listing fee is **not** refunded (it was
  the cost of the attempt — and refunding it would make listing-spam free).

**Acceptance — listing**

- [ ] Listing stages the item into persisted escrow and removes it from the
      seller's inventory; a bound/non-tradable item is refused.
- [ ] Listing charges the non-refundable listing fee; insufficient coin
      refuses the listing.
- [ ] A seller cannot exceed the per-player listing cap.
- [ ] A seller can cancel their own unsold listing; the item is returned and
      the listing fee is not refunded.

---

## 4. The persisted listing store

Auction listings are the **one genuinely new storage concern** here. They are
neither player-save state nor transient in-world state — they are
**long-lived world data that MUST survive reboots**: a listing posted before
a restart is still for sale after it.

- Listings live in a **new persisted store**, written with the engine's
  existing atomic discipline (`persistence.md`: tmp→bak→rename), so a crash
  mid-write never corrupts the store.
- A listing record holds: the listing id, the seller, the **escrowed item
  instance** (with its property bag + decorations, intact), the buyout price,
  the listing + expiry timestamps, and status (active / sold / expired /
  cancelled).
- The store is **versioned and migratable**, like player saves — listings can
  outlive a schema change, so the record carries a version and the loader
  migrates old records forward rather than dropping them (their escrowed
  items represent real player value that must not be lost).
- On **restart**, the store is loaded and reconciled: active listings resume;
  the escrowed items they hold are accounted for (never duplicated into both
  the store and an inventory); any listing whose expiry passed while the
  server was down is processed as an expiry (§8) on load.

**Acceptance — listing store**

- [ ] A listing posted before a reboot is present and for sale after it, item
      intact.
- [ ] The store is written atomically; a crash mid-write leaves the prior
      good store, never a corrupt one.
- [ ] The store is versioned; an older-version record loads via migration,
      not by being dropped.
- [ ] On restart, escrowed items are accounted to exactly one place (the
      listing), never duplicated; lapsed-while-down listings expire on load.

---

## 5. Browse and search

Listings are discoverable through text verbs at an access point
(`commands-and-dispatch.md`).

- **Browse/search** supports filtering by item **category** and **name**
  (substring, using the existing keyword conventions), **sorting** by price
  or by time-remaining, and **pagination** (a page at a time, with a way to
  step pages).
- Each result line shows enough to decide: the item (rendered with its
  decorations / rarity, `item-decorations.md`), the buyout price, the
  time remaining, and a per-listing reference (an id or ordinal) the buy verb
  takes. Example shape (illustrative, not literal):

  ```
  Auction House — page 1/4 — 37 listings (sword, by price)
   #  Item                         Price    Closes in
   1  a fine steel longsword       420g     2h 14m
   2  a short sword                 85g       46m
   3  [RARE] a Trolloc-forged axe  1,900g    5h 02m
  (browse next | browse sword price | buy 3)
  ```

**Acceptance — browse/search**

- [ ] A player can filter listings by category and by name substring.
- [ ] A player can sort by price and by time-remaining, and page through
      results.
- [ ] Each result shows the decorated item, price, time remaining, and a
      stable reference the buy verb accepts.

---

## 6. Buying (buyout)

- A buyer at an access point buys a listing outright at its **buyout price**.
  The purchase **commits through `trade-escrow.md` §3** as one atomic unit:
  the buyer's coin is staged and validated in the cancellable pre-event
  (sufficient coin; the listing still active — it is rejected if it sold or
  expired a tick earlier), then on commit the **coin moves to the seller's
  proceeds minus the sale cut** (§9) and the **item moves to the buyer**
  (delivered by §7). The fact event fires and the sale is audit-logged.
- A purchase that fails the veto (insufficient coin, listing gone) changes
  nothing — buyer and seller are made whole (`trade-escrow.md` §4) and the
  buyer is told why.
- The listing's status becomes **sold**; it leaves the active set.

**Acceptance — buying**

- [ ] A buyout charges the buyer the price atomically, credits the seller the
      price minus the sale cut, and transfers the item — all-or-nothing.
- [ ] Buying a listing that sold/expired a moment earlier fails cleanly with
      no value moved.
- [ ] A completed sale is audit-logged and the listing is marked sold.

---

## 7. Delivery (pickup, v1)

Goods and proceeds reach their owner by **pickup at the access point** in
v1 — not by push delivery (which needs the greenfield mail-with-attachments
substrate — see Open questions).

- A **sold item** is held in escrow for the **buyer** to collect at any
  access point; the **seller's proceeds** are held for the seller to collect.
  An **expired/cancelled** item is held for the **seller** to collect.
- The **notification queue** (`notifications.md`) carries the *notice*
  regardless of co-presence: "your listing sold — collect your coin",
  "your listing expired — collect your item", delivered immediately if
  online and on next login if offline. The queue carries **text only**; the
  goods themselves wait in escrow for pickup.
- A **collect** verb at an access point claims everything waiting for that
  player (won items, proceeds, returned items), moving each from escrow to
  the player through the normal capacity/weight checks — and if the player
  cannot hold it (full pack), the uncollected remainder stays in escrow for a
  later collect (never dropped, never lost).

**Acceptance — delivery**

- [ ] A sold item waits in escrow for the buyer; proceeds wait for the
      seller; an expired item waits for the seller.
- [ ] The notification queue delivers the sold/expired/cancelled notice
      online-immediately or on next login; it carries text, not goods.
- [ ] `collect` claims waiting items/coin subject to capacity; what doesn't
      fit stays in escrow for a later collect and is never lost.

---

## 8. Expiry and returns

Listing lifetime is driven by a **tick handler** (`time-and-clock.md` — the
tick loop), never a wall-clock timer or a poll.

- A recurring expiry handler finds listings whose **duration has elapsed**
  and processes each as an **expiry**: the listing becomes `expired`, its
  escrowed item is moved to the seller's pickup queue (§7), and a notice is
  sent. The listing fee is **not** refunded (§9).
- Expiry is **idempotent and crash-safe**: a listing expires exactly once;
  one whose expiry lapsed while the server was down is expired on load (§4),
  not skipped.
- (When bidding lands later, the same handler returns **outbid funds** to the
  losing bidder via the pickup/notification path. v1 buyout has no held
  bids, so there are no outbid returns yet.)

**Acceptance — expiry**

- [ ] A listing past its duration is expired by the tick handler; its item
      goes to the seller's pickup queue with a notice; the fee is not
      refunded.
- [ ] A listing expires exactly once, including one whose deadline passed
      during downtime (expired on load).

---

## 9. Fees: the gold sink

The auction house's fees are the **primary gold sink** of this system and
plug into the existing economy (`economy-survival.md`), removing gold that
quests and loot inject — the economic counterweight that keeps prices
meaningful.

- A **non-refundable listing fee** is charged at listing time (§3). It is the
  cost of taking up market space and the main anti-spam pressure; it is not
  returned on sale, cancel, or expiry.
- A **sale cut** is taken from the sale price at purchase (§6): the seller
  receives price minus the cut. Both the fee and the cut are **configuration**
  (§12) so an operator tunes the sink's strength.
- This is the deliberate contrast with `direct-trade.md`, which is **zero-sum
  by design** (no fee, value only moves between two players). The auction
  house *consumes* gold; direct trade only *moves* it.

**Acceptance — fees**

- [ ] A non-refundable listing fee is removed from the economy at listing and
      never returned.
- [ ] A sale takes the configured cut; the seller receives price minus cut;
      the cut leaves the economy.
- [ ] Fee and cut are configuration; setting them to zero disables the sink
      without breaking the flow.

---

## 10. Audit

Every listing, sale, cancel, expiry, and collection is recorded through the
shared audit log (`trade-escrow.md` §5): which player, which item instance,
which coin amounts, which outcome, when. This is what lets an operator
reverse a mistaken sale and lets an analyst trace an item's custody for
dupe/RMT investigation.

**Acceptance — audit**

- [ ] Each list / sale / cancel / expiry / collect appends an audit record
      with player, item instance, coin, outcome, and time.

---

## 11. Admin moderation

An operator can intervene — **cancel or remove** a listing and **refund** a
sale — gated by role.

- Moderation verbs require an administrative role
  (`roles-and-permissions.md` / `admin-verbs.md`). **Build dependency:** both
  are currently **spec-only** (no implementation yet); auction moderation
  cannot enforce its gate until roles ship, so v1 moderation is built behind
  that dependency (or ships ungated-but-disabled until roles land — a plan
  decision, see `docs/plans/trade-plan.md`).
- A removed listing returns the item to the seller (pickup, §7) and is
  audit-logged with the acting admin; a refunded sale reverses the coin/item
  through the escrow primitive and is audit-logged.

**Acceptance — moderation**

- [ ] An admin (role-gated) can remove a listing (item returned, audited) and
      refund a sale (reversed through escrow, audited).
- [ ] Moderation verbs are refused to non-admins once role gating is live.

---

## 12. Configuration surface

| Setting | Description |
|---|---|
| Listing duration | How long a listing stays active before expiry (§3, §8). |
| Minimum buyout price | The floor a listing's price may be set to (§3). |
| Per-player listing cap | Max simultaneous active listings per seller (§3). |
| Listing fee | The non-refundable fee charged at listing (gold sink) (§9). |
| Sale cut | The fraction of the sale price taken at purchase (gold sink) (§9). |
| Page size | Browse/search results per page (§5). |
| Market scope | Global vs. per-location (v1 global; see Open questions). |
| Access-point tag(s) | The NPC/room tag(s) that mark an auctioneer (§2). |

No item names, prices, or place names are fixed by behavior — listings are
player data and access points are content.

---

## Open questions

- **Global vs. location-scoped markets.** Recommend **global for v1**:
  one world pool reachable from any access point. With a small or growing
  population, a single pool keeps **liquidity** high (listings actually sell);
  fragmenting into per-location markets thins each pool. **Later evolution:**
  per-location markets with arbitrage between them — richer *trader gameplay*
  (buy low in one region, sell high in another) and a travel reward, but only
  once population supports it. **Rejected for v1:** location-scoped from the
  start (kills liquidity while the playerbase is small). Locations, if added,
  are content.
- **Goods delivery (the cross-system dependency gating the plan).**
  Recommend **pickup-at-the-auctioneer for v1**: goods and proceeds wait in
  escrow to be collected (§7); the notification queue carries only the text
  notice. This needs **no new substrate**. **Deferred alternative:** push
  delivery via **mail-with-attachments** — which is *greenfield* (the
  notification queue delivers text only, not items/coin). That attachment-
  delivery layer is the **same substrate the mail backlog item needs**; build
  it once, and both auction push-delivery and player mail consume it.
  **Rejected for v1:** building attachment-delivery first (it gates the whole
  auction house on a separate greenfield system; pickup ships value sooner).
- **Bid vs. buyout for v1.** Recommend **buyout-only for v1**: a fixed
  price, first buyer with the coin wins. It minimizes escrow states (only the
  seller's item is held; the buyer's coin is charged at purchase, not held
  across a contest) and **sidesteps anti-sniping entirely** (§1.1).
  **Deferred:** auction **bidding** — held bidder coin, outbid returns,
  and the sniping/soft-close questions that v1 explicitly excludes; a
  well-scoped later phase. **Rejected for v1:** bidding-first or both-at-once
  (carries the most escrow complexity and the excluded sniping concerns into
  v1).
- **Multi-currency (out of scope, not an open question).** Gold +
  auto-conversion already exists (`economy-survival.md`); the auction house
  uses gold. Multiple currencies are not a v1 question; if ever revisited it
  is a content/economy concern, not an auction-house mechanism.

---

## Cross-references

- `trade-escrow.md` — the escrow/atomic-transaction primitive this consumes;
  listings hold items in its escrow and purchases commit through it; the
  audit log lives there. **Built once, consumed twice** (with
  `direct-trade.md`).
- `direct-trade.md` — the synchronous sibling consumer; contrast async/
  persisted/fee-bearing (here) vs. sync/transient/zero-sum (there).
- `economy-survival.md` — the gold currency, and the fee/cut **gold sink**
  that balances the economy.
- `notifications.md` — the offline-capable queue that carries sold/expired/
  outbid **notices** (text; goods wait for pickup).
- `commands-and-dispatch.md` — the list/browse/search/buy/collect verbs and
  typed argument resolvers.
- `item-decorations.md` — how listed items render (rarity/essence).
- `persistence.md` — the atomic, versioned-and-migrated discipline the new
  listing store and the audit log follow.
- `time-and-clock.md` — the tick loop that drives expiry (§8), not a
  wall-clock timer.
- `roles-and-permissions.md` / `admin-verbs.md` — moderation gating (both
  **spec-only build dependencies** today, §11).
- mail-with-attachments (greenfield, `BACKLOG.md`) — the deferred push-
  delivery substrate this would share (see Open questions).

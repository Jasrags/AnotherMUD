# Player Trade — Implementation Plan (Direct Trade + Auction House)

**Specs:** `docs/specs/trade-escrow.md` (the shared primitive),
`docs/specs/direct-trade.md`, `docs/specs/auction-house.md`. **Status:**
Planning — no code yet. **Audience:** the build sequence to review before
implementation.

A phased, dependency-ordered plan covering **both** trade systems. The specs
are the timeless contracts; this is the sequence. The guiding fact: both
systems consume **one** primitive (`trade-escrow.md`) — **built once,
consumed twice** — so the primitive is Phase 0 and everything else hangs off
it. Most later phases are *new* surface (listings, the persisted store,
verbs), but they all commit through the same escrow guarantee.

---

## Dependency map (lean on vs. new)

| Need | Source | Status |
|---|---|---|
| Cancellable pre-event + fact event for the commit | the typed event bus | built |
| Coin staging / movement | gold currency + auto-conversion (`economy-survival`) | built |
| Item instances, stacking, tradability, capacity/weight | `inventory-equipment-items` | built |
| Offline "sold/expired/outbid" notice | notification queue (`notifications`) | built |
| Expiry timing | tick loop (`time-and-clock`) | built |
| Verbs + typed args (player/item/ordinal/bulk) | `commands-and-dispatch` | built |
| Listing item render | rarity/essence (`item-decorations`) | **spec-only** |
| Auction access point | temporary-entity + room-tag (no furniture system) | built (reused) |
| Atomic file writes | `persistence` (tmp→bak→rename) | built |
| Admin moderation gate | roles + admin-verbs | **built** (shipped + enforcing) |
| **Escrow / atomic-transaction primitive** | `trade-escrow` | **build (Phase 0)** |
| **Persisted listing store** | new, versioned, atomic | **build** |
| **Goods push-delivery (mail attachments)** | greenfield, shared w/ mail | **deferred** |

---

## Phases (dependency-ordered)

### Phase 0 — Escrow / atomic-transaction primitive *(the foundation)*
- **Build:** `trade-escrow` — escrow staging (items + coin go inert, stay
  owned), the **cancellable `trade.committing`** pre-event (validation seam:
  capacity/weight/tradability/coin), the all-or-nothing commit + the
  non-cancellable `trade.committed` fact, and **make-whole rollback** on any
  veto/withdraw/cancel/failure. N-party-capable, two-party-used.
- **Leans on:** event bus, currency, item instances.
- **Open Q:** escrow placement — **resolved (standalone spec)**.
- **Done when:** a test transaction stages two parties' value, commits
  atomically on no-veto, and rolls everyone whole on a veto.

### Phase 1 — Audit log *(with the primitive)*
- **Build:** the persisted, append-only, tamper-evident audit record written
  on every commit (parties, item instances, coin, source, time); atomic file
  discipline (`persistence`). Underpins support rollback + dupe/RMT tracing.
- **Leans on:** persistence.
- **Done when:** every commit appends an immutable record sufficient to
  reconstruct and reverse it.

### Phase 2 — Direct trade *(the simple consumer — proves the primitive)*
- **Build:** the same-room trade session (transient), offers (add/remove
  items+coin → stage/withdraw in escrow), the **confirm + total-reset rule**
  (any change clears both confirmations), the atomic swap via Phase 0, and
  graceful teardown (cancel/disconnect/link-death/separation/restart → full
  restore, nothing persisted). Verbs via the command registry.
- **Leans on:** Phase 0/1, commands, inventory, currency.
- **Open Q:** confirm-reset UX (warn on change) — **resolved (hard reset +
  clear message)**.
- **Done when:** two players swap items+coin atomically; any mid-trade change
  resets confirmations; every abort path returns all value.

> ### ⎯⎯ this is a complete shippable feature on its own ⎯⎯
> Phases 0–2 deliver **working player-to-player trade**. The auction house
> (below) reuses the same Phase 0/1 foundation.

### Phase 3 — Delivery decision: pickup *(the cross-system gate)*
- **Build:** the escrow-holds-until-collected delivery + a `collect` verb at
  an access point, with the **notification queue** carrying the text notice
  (online now / on next login). **v1 = pickup**, which needs **no new
  substrate**.
- **Open Q:** push-delivery (mail attachments) vs pickup — **resolved
  (pickup v1)**; push delivery deferred (Phase 10), shared with mail.
- **Done when:** goods/coin wait in escrow, a notice is sent, and `collect`
  claims them subject to capacity (remainder stays escrowed, never lost).

### Phase 4 — Auction listing + persisted store
- **Build:** the **list** verb at an access point (stage item into persisted
  escrow, set buyout price + duration, charge the non-refundable listing fee,
  enforce per-player cap, allow self-cancel); the **new persisted listing
  store** — versioned + migratable + atomic — with restart reconciliation
  (escrowed items accounted to exactly one place; lapsed-while-down handled).
- **Leans on:** Phase 0/1/3, persistence, the access-point tag (reused
  substrate), currency (fee).
- **Open Q:** store versioning — **decided (versioned like player saves)**.
- **Done when:** a listing survives a reboot item-intact; the store never
  corrupts on a mid-write crash.

### Phase 5 — Browse / search verbs
- **Build:** browse/search with category + name filters, price/time sort,
  pagination, and a stable per-listing reference the buy verb takes; results
  render items with decorations.
- **Leans on:** commands, `item-decorations` (spec-only — render via whatever
  decoration support exists when this lands).
- **Done when:** a player filters, sorts, and pages listings and gets a
  reference to buy.

### Phase 6 — Buyout purchase
- **Build:** the **buy** verb — atomic purchase through Phase 0 (stage buyer
  coin → veto checks → coin to seller minus the **sale cut**, item to buyer's
  pickup), marks the listing sold, notice sent.
- **Open Q:** buyout vs bidding — **resolved (buyout-only v1)**; bidding
  deferred (Phase 9).
- **Done when:** a buyout charges the buyer, credits the seller net of cut,
  and delivers the item atomically; a just-sold/expired listing fails clean.

### Phase 7 — Tick expiry + returns
- **Build:** the recurring expiry **tick handler** — expire past-duration
  listings, return the item to the seller's pickup queue with a notice,
  idempotent + crash-safe (expire-on-load for lapsed-while-down). No fee
  refund.
- **Leans on:** tick loop, Phase 3 (pickup), notifications.
- **Done when:** unsold listings expire exactly once and return to the seller.

### Phase 8 — Fees as the gold sink *(tuning)*
- **Build:** wire the non-refundable listing fee (Phase 4) and the sale cut
  (Phase 6) as **configuration**; document the sink's economic role. Mostly
  config + balance, lands alongside Phases 4/6 but tuned here.
- **Done when:** fee + cut remove gold; setting them to zero disables the sink
  without breaking flow.

> ### ⎯⎯ MVP CUT LINE ⎯⎯
> Phases 0–8 deliver the **full v1 trade economy**: atomic escrow + audit,
> working direct trade, and a global buyout auction house with a persisted
> store, browse/search, pickup delivery, tick expiry, and a fee gold sink.
> Everything below is depth.

---

## Deferred (post-MVP)

### Phase 9 — Auction bidding
Held bidder coin in escrow, outbid returns (via the same expiry/pickup/notice
path), and the **anti-sniping / soft-close** questions v1 explicitly excludes.
A well-scoped later phase; blocked on deciding the sniping policy.

### Phase 10 — Push delivery (mail-with-attachments)
The greenfield attachment-delivery substrate that lets goods/coin be *pushed*
to a player's inbox instead of collected. **Shared with the mail backlog
item** — build once, consumed by both auction delivery and player mail. Until
then, pickup (Phase 3) stands.

### Phase 11 — Location-scoped markets
Per-location auction pools with arbitrage between them (trader gameplay +
travel reward). Deferred until the population supports fragmenting liquidity;
v1 is global.

### Admin moderation *(dependency now built)*
Cancel/remove a listing + refund a sale, **role-gated**. The gate is no
longer blocked: `roles-and-permissions` + `admin-verbs` are shipped and
enforcing (admin commands gate on `HasRole`, with live `grant`/`revoke`
verbs), so moderation can gate directly when built. Not on the MVP
critical path.

---

## Open questions blocking phases

| Question | Blocks | Status |
|---|---|---|
| Escrow placement | Phase 0 | **DECIDED — standalone `trade-escrow.md`.** |
| Confirm-reset UX | Phase 2 | **DECIDED — hard reset + clear message.** |
| Delivery: pickup vs push | Phase 3 | **DECIDED — pickup v1; push deferred (Phase 10).** |
| Listing-store versioning | Phase 4 | **DECIDED — versioned/migratable like player saves.** |
| Buyout vs bidding | Phase 6 | **DECIDED — buyout-only v1; bidding deferred (Phase 9).** |
| Global vs location market | Phase 11 | **DECIDED — global v1; location-scoped deferred.** |
| Bidding anti-sniping policy | Phase 9 (deferred) | open — decide when bidding is scheduled. |
| Attachment-delivery substrate | Phase 10 (deferred) | greenfield; design jointly with mail. |
| Roles/admin built | Admin moderation | **RESOLVED — roles + admin-verbs shipped + enforcing.** |

---

## What review should confirm before Phase 0

The v1 shape and all MVP-path decisions are **made** (standalone escrow;
pickup delivery; buyout-only; global market; versioned store). Remaining
confirmations are non-blocking and surface later:

1. **Fee/cut first-pass numbers** (Phase 8) — propose-and-tune, not a design
   gate.
2. **Admin moderation timing** — build now-but-disabled vs. wait for roles.
3. **Bidding** (Phase 9) and **push delivery** (Phase 10) are deferred; their
   open questions only matter when scheduled.

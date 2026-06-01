# Direct Trade — Feature Specification

**Status:** Draft · **Scope:** A synchronous, same-room trade session
between two present players — each stakes items and coin, sees the other's
current offer, and both confirm; an atomic swap fires only when both have
confirmed an unchanged pair of offers. Sessions are transient and graceful
cancel/restore on disconnect, link-death, or separation · **Audience:**
Anyone reimplementing or porting this feature in any language.

This document describes *what* direct trade must do, not *how* to implement
it. All tunables live in the configuration-surface table at §7.

Direct trade is the **simpler, first consumer** of the shared escrow/
transaction primitive defined in `trade-escrow.md`. It does **not** redefine
that primitive — it stages two offers into escrow and commits them through
it. (The primitive lives in its own spec rather than here; the reasoning is
in `trade-escrow.md` §Overview/§8.) Direct trade is intentionally
**zero-sum**: value moves between the two players and nothing is removed
from the economy — there is **no fee and no gold sink**, in deliberate
contrast to the auction house (`auction-house.md`), whose fees *are* a sink.

---

## 1. Overview

Two players in the same place agree to swap. Each builds an **offer** of
items and/or coin; each sees the other's current offer; each **confirms**.
When both have confirmed the *same, unchanged* pair of offers, the swap
commits atomically through `trade-escrow.md`. Until then, either may change
their offer or walk away, and everything is returned.

Two rules carry the design:

1. **Atomic swap** — the exchange either completes fully or not at all,
   committed through the escrow primitive's cancellable pre-event (so
   capacity/weight/tradability are validated before anything moves) and
   rolled back making both players whole on any failure.
2. **Confirm-then-tamper is impossible** — **any** change to **either**
   side's offer immediately **resets both confirmations**. A swap can never
   fire on a stale confirmation. This single rule kills the classic
   bait-and-switch ("show a good offer, get the confirm, swap in a worse one
   before committing").

### 1.1 What direct trade is *not*

- Not asynchronous. Both players must be present together; there is no
  list-now-collect-later. That is the auction house (`auction-house.md`).
- Not `give`. `give` is a one-way gift with no reciprocation or confirmation;
  direct trade is a confirmed two-way swap.
- Not persisted. A trade session is transient in-world state, like a
  connection or the weather — it is **never** written to disk. An
  interrupted trade simply never happened (§6).
- Not a market. There is no price, no fee, no listing, no search. It is a
  private agreement between two named players.

---

## 2. The trade session

- A player **initiates** a trade against another player **in the same room**
  (resolved through the player argument type, `commands-and-dispatch.md`).
  The target must be present, a real player, and not already in a trade. The
  target is asked to accept; on accept, a **trade session** opens between the
  two.
- A session binds exactly **two** players and tracks each one's current
  offer. It is **transient**: it exists only in memory for the life of the
  negotiation and is destroyed on commit, cancel, or any teardown (§6).
- A player is in **at most one** trade session at a time.

**Acceptance — session**

- [ ] A trade opens only against a present, real player in the same room who
      is not already trading; the target accepts before the session starts.
- [ ] A player can be in at most one trade session at a time.
- [ ] The session holds no persisted state; nothing about it is written to
      disk.

---

## 3. Offers

- Each player builds an **offer** by adding items and/or an amount of coin,
  and may remove them again. **Adding an item to the offer stages it into
  escrow** (`trade-escrow.md` §2): it becomes inert immediately (cannot be
  dropped, equipped, given, sold, or staged elsewhere) while it sits in the
  offer. Removing it from the offer withdraws it from escrow back to the
  player. Coin is staged the same way.
- Each player can **see the other's current offer** at all times — the exact
  items (rendered with their decorations, `item-decorations.md`) and coin —
  so a swap is never blind.
- Non-tradable/bound items are refused at add time (`trade-escrow.md` §6);
  they never appear in an offer.

**Acceptance — offers**

- [ ] Adding an item/coin to an offer stages it into escrow (inert);
      removing it returns it to the player.
- [ ] Each player sees the other's current offer (items + coin) accurately,
      updated as it changes.
- [ ] A bound/non-tradable item cannot be added to an offer.

---

## 4. Confirmation and the reset rule

- Either player may **confirm** their readiness once they are satisfied with
  *both* offers as shown.
- **Any change to either offer resets both confirmations.** The moment a
  player adds, removes, or alters any item or coin on *either* side, both
  players' confirmations are cleared and must be given again against the new
  state. There is no window in which a confirmation outlives the offer it
  was given against.
- A swap (§5) fires **only** when both players are confirmed **and** neither
  offer has changed since those confirmations. Confirming an already-stable,
  both-confirmed pair is what triggers the commit.
- Each player can always **see** the other's confirmation state, so "I
  confirmed, waiting on you" is legible.

**Acceptance — confirmation**

- [ ] A swap fires only when both players are confirmed against an unchanged
      pair of offers.
- [ ] Any add/remove/change on either side immediately clears both
      confirmations.
- [ ] No swap can fire on a confirmation given against a since-changed offer.

---

## 5. The atomic swap

- When both confirmations stand against an unchanged pair, the session
  **commits** the two staged offers through `trade-escrow.md` §3: the
  primitive publishes its cancellable pre-event (a subscriber may veto for
  the recipient's **capacity/weight**, a late **tradability** change, or
  other eligibility), and on no veto moves both offers as **one unit** —
  player A's offer to B and B's to A — then fires the fact event and writes
  the audit record.
- On a **veto or any failure**, nothing moves: the escrow rolls back and
  **both players are made whole** (`trade-escrow.md` §4). The session
  surfaces the reason (e.g. "their pack is full") and returns to the
  unconfirmed state so the players can adjust and retry.
- On success, the session closes; each player is told what they received.

**Acceptance — swap**

- [ ] A successful swap moves both offers atomically (never a partial state)
      and is audit-logged.
- [ ] A veto (full pack, over weight, became-bound) aborts the swap, makes
      both whole, surfaces the reason, and leaves the session open to retry.
- [ ] The swap is zero-sum — no coin or item is removed from the economy by
      the trade itself (no fee).

---

## 6. Cancel and teardown

A trade ends cleanly in every path, with all staged value returned unless it
committed.

- **Explicit cancel/decline** by either player ends the session; escrow
  returns every staged item and coin to its owner.
- **Disconnect / link-death** of either player ends the session the same
  way: full restore. (Because the session is transient, a reconnecting player
  is simply not in a trade anymore.)
- **Separation** — either player leaving the room — ends the session with
  full restore (a same-room invariant; you cannot trade with someone who
  walked off).
- **Process restart** mid-trade: the session was never persisted, so on
  restart it does not exist; because nothing committed, the staged items and
  coin are simply the players' own (they were never transferred). An
  interrupted trade **never happened**.

**Acceptance — teardown**

- [ ] Cancel, decline, disconnect, link-death, and either player leaving the
      room each end the session and return all staged value to its owner.
- [ ] No teardown path transfers value; only a completed §5 swap does.
- [ ] A restart mid-trade leaves both players holding exactly what they
      staged; no trade is resumed and none is committed.

---

## 7. Verbs and configuration

The player surface uses the command registry + typed arguments
(`commands-and-dispatch.md`): a verb to **initiate** a trade with a present
player; verbs to **add/remove** items (item + ordinal + bulk resolvers) and
**coin** to one's offer; a verb to **confirm**; and a verb to
**cancel/decline**. Exact verb names are configuration.

| Setting | Description |
|---|---|
| Trade verbs | The initiate / add / remove / coin / confirm / cancel verb names. |
| Max offer size | Cap on item count (and/or weight) per offer, if any (§3). |
| Trade range | The co-location requirement (same room in v1) (§2). |
| Warn on coin change | Whether changing the coin amount on a confirmed offer emits an extra warning before the reset (§8). |

---

## 8. Open questions

- **Confirm-reset friction (UX).** The reset rule is non-negotiable
  (security), but a player who confirms and then *adds more coin as a
  bonus* still trips the reset and may not understand why. Recommend: keep
  the hard reset, and on any post-confirm change emit a clear line to both
  players ("offer changed — confirmations reset"). Optionally warn before
  applying a coin change to an already-confirmed offer (`Warn on coin
  change`). **Rejected:** softening the reset (e.g. "additions don't reset,
  only removals") — additions are exactly the bait-and-switch vector
  (swap a junk item's stack size, slip in a cursed item), so the reset must
  be total.
- **Escrow placement (informational).** The shared primitive lives in
  `trade-escrow.md`, not here, even though direct trade is its first/simpler
  consumer — see that spec's §8. Recorded here so a reader of direct trade
  knows where the swap guarantee is defined.
- **Trade range.** v1 requires the same room. Recommend keeping it
  (presence makes the social contract legible and the offer-view trustworthy).
  **Rejected for v1:** cross-room trade (it blurs the line with the auction
  house and invites "trade" as a teleporting item-courier).

---

## Cross-references

- `trade-escrow.md` — the escrow/atomic-transaction primitive this consumes;
  the swap's atomicity, veto, rollback, and audit all live there.
- `auction-house.md` — the asynchronous sibling consumer; shares the same
  primitive (`trade-escrow.md`); contrast: auction is async, persisted, and
  fee-bearing where direct trade is sync, transient, and zero-sum.
- `commands-and-dispatch.md` — the verbs and typed argument resolvers
  (player, item, ordinal, bulk).
- `inventory-equipment-items.md` — item instances, stacking, capacity/weight
  the swap veto checks.
- `economy-survival.md` — the gold currency staged as coin.
- `item-decorations.md` — how offered items render.

# Trade Escrow & Atomic Transactions — Feature Specification

**Status:** Draft · **Scope:** The shared primitive both player-trade
systems consume — *escrow* (staging items and coin from one or more
parties in a pending, inert state) and the *atomic transaction* that
commits all staged value as one unit through a cancellable bus event, or
rolls it back making every party whole; plus the tamper-evident audit log
every transaction writes · **Audience:** Anyone reimplementing or porting
this feature in any language.

This document describes *what* the primitive must do, not *how* to
implement it. All tunables live in the configuration-surface table at §7.

**Built once, consumed twice.** This primitive is the single authoritative
home of player-trade atomicity. `direct-trade.md` (a synchronous, transient
consumer) and `auction-house.md` (an asynchronous, persisted consumer) both
**cite** it and neither redefines it. It is specified as its own spec —
rather than inside its first consumer — deliberately: the two consumers
hold escrow for very different lifetimes (a direct trade holds it for
seconds in-session; an auction holds a seller's item and bidders' coin for
the listing's whole duration, persisted across reboots, with multiple
parties). A neutral home keeps the commit/rollback/audit guarantees
identical for both and makes "neither consumer owns the primitive"
structural, not merely conventional. (The rejected alternative — folding it
into `direct-trade.md` — is recorded in §8.)

---

## 1. Overview

A **transaction** moves value between parties so that it either happens
completely or not at all. To get there, value is first placed in **escrow**:
removed from its owner's normal reach (cannot be spent, dropped, traded, or
consumed) but still *theirs* until the transaction commits. The commit is
gated by a **cancellable bus event** so any system may veto it before it
fires; the actual move happens as one indivisible unit; and a
**non-cancellable fact event** announces it afterward. Anything that goes
wrong returns every escrowed thing to its owner — no value is ever
duplicated or lost. Every transaction is written to an **audit log**.

The spine, in one line:

> **Every transaction is atomic, logged, and tamper-evident** — staged in
> escrow, vetoable before commit, all-or-nothing at commit, and fully
> reversible on any failure.

### 1.1 What this is *not*

- Not a trade *system*. It is the substrate; the player-facing flows
  (offer/confirm/swap, list/bid/buy) live in `direct-trade.md` and
  `auction-house.md`. This spec defines the guarantee, not the verbs.
- Not a currency or item system. It *moves* coin and item instances that
  already exist (`economy-survival.md`, `inventory-equipment-items.md`); it
  does not mint, destroy, or define them.
- Not a persistence policy. Whether escrowed value survives a reboot is the
  *consumer's* decision (transient for direct trade, persisted for the
  auction house). This spec defines what "made whole" means in each case,
  not where the bytes live.
- Not a fraud engine. It produces the audit trail that makes dupe/RMT
  investigation and support rollback *possible*; it does not itself detect
  or judge abuse.

---

## 2. Escrow: staging value

An **escrow** is a holding area for value a party has committed to a pending
transaction.

- A party may stage **item instances** (real instances, honoring tradability
  — §6) and **coin** (an amount of the gold currency,
  `economy-survival.md`). The staged item leaves normal availability: while
  escrowed it cannot be equipped, dropped, given, sold, consumed, or staged
  into a second transaction. Staged coin is likewise reserved — it cannot be
  spent or staged elsewhere.
- Staged value remains **owned by the staging party** until commit. Escrow
  is a hold, not a transfer. A party can **withdraw** their own staged value
  back to themselves at any time before commit (the consumer decides when
  withdrawal is allowed — e.g. direct trade resets confirmations on it,
  §consumer).
- An escrow references value by **identity** (the specific item instance,
  the specific coin amount from a specific party), so a commit moves exactly
  what was staged and a rollback returns exactly what was staged.

**Acceptance — escrow**

- [ ] Staged items and coin are removed from the owner's normal reach
      (cannot be spent/dropped/equipped/consumed/double-staged) while held.
- [ ] Staged value remains the staging party's until commit; withdrawal
      before commit returns it intact to that party.
- [ ] The same item instance or coin cannot be staged into two transactions
      at once.

---

## 3. The atomic commit

A transaction commits all staged legs as one unit, gated by the cancellable
bus.

- When a consumer requests commit, the primitive publishes a **cancellable
  pre-event** (`trade.committing`) carrying the full transaction: every
  party, every staged item and coin amount, and the destination each leg
  moves to. Any subscriber may **veto** it by flipping the cancel flag —
  this is the validation seam, not a separate validation pass. Canonical
  veto reasons: a recipient lacks inventory **capacity** or **weight**
  headroom for an incoming item; an item is **not tradable** (§6); a payer
  has **insufficient coin** at commit time; a party is **no longer eligible**
  (gone, dead, disconnected).
- If no subscriber vetoes, the transaction **commits as one indivisible
  unit**: every leg moves to its destination together. There is no
  observable state in which some legs moved and others did not.
- On commit, the primitive publishes the **non-cancellable fact event**
  (`trade.committed`) and writes the audit record (§5). Subscribers that
  react to a completed trade (quests, stats, GMCP) listen here.
- The pre-event fires **after** all value is staged and **before** any leg
  moves, so a veto costs nothing — the rollback (§4) simply returns staged
  value.

**Acceptance — commit**

- [ ] Commit publishes a cancellable `trade.committing` carrying all parties
      and legs before any value moves.
- [ ] A veto (capacity/weight/tradability/insufficient-coin/ineligible)
      aborts the commit; no leg moves; every party is made whole (§4).
- [ ] On no veto, all legs move as one unit — never a partial state — and a
      non-cancellable `trade.committed` fires with the audit record written.

---

## 4. Make-whole rollback

A transaction that does not commit must leave the world exactly as it was
before staging.

- **Rollback returns every escrowed thing to the party that staged it** —
  the exact item instances and coin amounts. No value is created
  (duplication) or destroyed (loss). After rollback, each party holds
  precisely what they held before staging.
- Rollback is triggered by: a **veto** (§3); a party **withdrawing** before
  commit; the consumer **cancelling** the transaction (an abandoned direct
  trade, an expired auction listing); or a **failure mid-commit**
  (a party disconnects, the process restarts).
- **Crash/reboot safety** is the consumer's persistence decision, but the
  guarantee is uniform: a transaction interrupted before its commit fact
  event is **not committed**, and its staged value is owned by the original
  parties. A transient consumer (direct trade) achieves this by simply not
  persisting in-flight escrow — an interrupted trade never happened. A
  persisted consumer (auction house) achieves this by recording staged value
  durably and reconciling on restart so escrowed goods are returned or the
  listing resumed, never duplicated.

**Acceptance — rollback**

- [ ] After any non-commit (veto, withdraw, cancel, mid-commit failure),
      every party holds exactly what they held before staging.
- [ ] No rollback path can duplicate or destroy an item instance or coin.
- [ ] A transaction interrupted before its `trade.committed` fact event is
      treated as not committed; its staged value belongs to the original
      owners after recovery.

---

## 5. The audit log

Every transaction is recorded for support and abuse investigation.

- On each commit, the primitive appends one **audit record** sufficient to
  reconstruct what happened: the parties, every item instance and coin
  amount moved, the source consumer (direct trade / auction sale / auction
  return), and a timestamp. Vetoed/rolled-back attempts MAY also be recorded
  at a lower level (decided in §8).
- The log is **append-only and tamper-evident**: records are added, never
  edited or deleted in place, so a later reading reflects the true history.
  It is sufficient for **support rollback** (an operator can see exactly what
  moved and reverse it) and for **dupe/RMT detection** (an analyst can trace
  an item instance's chain of custody and spot value flowing one way for
  nothing).
- The audit log is **persisted** (it is long-lived world data, like the
  auction store), written with the engine's atomic file discipline
  (`persistence.md`). Its retention/rotation is configuration (§7).

**Acceptance — audit**

- [ ] Every commit appends an audit record naming the parties, the exact
      items + coin moved, the source, and the time.
- [ ] Audit records are append-only; an existing record is never mutated.
- [ ] The log lets an operator reconstruct and reverse a transaction and
      trace an item instance's custody chain.

---

## 6. Tradability, instances, and stacking

The primitive moves real items and honors their flags.

- Only **real item instances** stage — with their property bags, decorations
  (`item-decorations.md`), and stack identity intact. A staged stack moves
  as the quantity staged (stacking rules per `inventory-equipment-items.md`);
  the rest of the stack stays with the owner.
- An item carrying a **non-tradable / bound** flag cannot be staged; an
  attempt is refused at staging time (not at commit), so a party never sees
  a bound item in escrow.
- Coin is fungible: staged coin is an amount, drawn from and returned to the
  party's gold via the existing currency surface (`economy-survival.md`),
  including its auto-conversion behavior.

**Acceptance — tradability**

- [ ] A non-tradable/bound item is refused at staging; it never enters
      escrow.
- [ ] A staged partial stack moves the staged quantity and leaves the
      remainder with the owner; instance identity and decorations survive
      the move.

---

## 7. Configuration surface

| Setting | Description |
|---|---|
| `trade.committing` / `trade.committed` event names | The cancellable pre-event and non-cancellable fact event (§3). |
| Audit retention | How long / how many audit records are kept before rotation (§5). |
| Record rolled-back attempts? | Whether vetoed/withdrawn attempts are audited, and at what level (§5/§8). |
| Coin currency | Which currency stages as "coin" (gold today) (§2/§6). |

No party count, item count, or value limit is fixed here — those are the
*consumers'* caps (a direct trade's offer size, an auction's listing cap).

---

## 8. Open questions

- **Placement (resolved — standalone).** Chosen: this primitive is its own
  spec, cited by both consumers. **Rejected:** folding it into
  `direct-trade.md` (the brief's first-listed option) — it would force the
  transient consumer to also describe the auction's persisted, multi-party,
  long-lived escrow, blurring the guarantee and making the second consumer
  read the first to understand its own substrate. Revisit only if the
  primitive ever shrinks to something direct-trade-specific.
- **Auditing rolled-back attempts.** Recommend: record vetoes/withdrawals at
  a low (debug/analytics) level, not the full audit tier — they're useful
  for spotting probing but are noise in the support-rollback view. Default:
  commit-only at the audit tier, attempts at debug. **Rejected:** auditing
  every attempt at full tier (drowns real transactions); auditing nothing
  but commits (loses the probing signal).
- **Multi-party (N>2) transactions.** The primitive is written for any
  number of parties, but v1 consumers use exactly two (a direct trade has
  two; an auction sale is seller↔buyer). Recommend keeping the N-party
  generality in the contract (it costs nothing) while shipping two-party
  consumers. **Rejected:** hard-coding two parties — it would block a future
  three-way trade or a consignment split for no gain.

---

## Cross-references

- `direct-trade.md` — the synchronous, transient consumer (offer/confirm/
  swap); stages two offers and commits them through this primitive.
- `auction-house.md` — the asynchronous, persisted consumer (list/buy);
  holds a seller's item and a buyer's coin in this primitive's escrow.
- `economy-survival.md` — the gold currency that stages as coin, and
  (for the auction) the fee gold sink.
- `inventory-equipment-items.md` — item instances, stacking, tradability
  flags, capacity/weight that the commit veto checks.
- `item-decorations.md` — how escrowed/listed items render (rarity/essence).
- `persistence.md` — the atomic file discipline the audit log (and the
  auction store) write with.
- the engine event bus — the cancellable pre-event / fact event mechanism
  the commit rides (see `docs/specs/README.md` cancellable-events table).

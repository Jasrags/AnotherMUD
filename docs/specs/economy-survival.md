# Economy, Sustenance, Rest, and Consumables — Feature Specification

**Status:** Draft · **Scope:** The currency property and auto-
conversion hook; shop NPCs and their buy / sell / value
operations; the sustenance pool with tier-driven regen
modulation; the rest-state machine (awake / resting / sleeping)
with combat-driven wake; and the consumable-item pipeline that
ties food / drink / potions back into the sustenance and effect
features · **Audience:** Anyone reimplementing or porting this
feature in any language.

This document describes *what* the survival-and-economy subsystem
must do, not *how* to implement it. Specific multipliers,
thresholds, regen formulas, and the exact list of consumable
properties are policy that lives in configuration or content.

---

## 1. Overview

Four small, loosely-coupled subsystems live in this spec because
they share the same shape — small services backed by entity
properties, integrating with each other through events and
property reads:

1. **Economy** — a single integer **gold** property on entities,
   plus shop NPCs that exchange items for gold.
2. **Sustenance** — a hunger-like pool from 0 to 100 with named
   tiers and a regen-multiplier hook so other systems can scale
   their regen based on it.
3. **Rest** — a three-state machine (awake / resting / sleeping)
   that scales regen and is interrupted by combat.
4. **Consumables** — items that, when consumed, raise sustenance,
   queue effects, and tick charges down.

They do NOT own:

- Regen itself. Regen is the responsibility of other features
  (heartbeat / world tick). Sustenance and rest expose
  multipliers; the regen feature consults them.
- The actual effect application from a consumable's `effect_id`.
  Consumables emit an event with the effect parameters; the
  effects feature consumes it.
- The commands `buy` / `sell` / `value` / `eat` / `drink` /
  `sleep` / `rest`. Those are commands feature handlers; this
  spec defines the operations they call.
- Crafting, brewing, or item creation. Items come from content
  templates.

---

## 2. Currency

### 2.1 The gold property

Every entity (player or NPC) carries an integer **gold**
property. Missing entries are treated as zero. The property
persists with the entity.

There is no separate "wallet" entity; gold lives directly on
the holder. Coins picked up from the world are auto-converted
into the gold property via the auto-convert hook (§2.3) — they
do not stack as items in inventory.

### 2.2 Add / set / read

- **AddGold(entity, delta, reason).** Reads the current value,
  computes `max(0, current + delta)`, writes it back, emits an
  event:
  - `currency.credited` when delta ≥ 0.
  - `currency.debited` when delta < 0.
  The event carries actor id, absolute amount, source, and
  reason. Returns the new total.

  Gold is **floored at zero** — debits that would go negative
  silently clamp. A debit caller MUST verify funds itself
  (e.g. via a separate gate) if "insufficient funds" needs to
  be reported as a distinct outcome.

- **SetGold(entity, amount, reason).** Forces gold to the
  given value. Throws on negative input. Always emits
  `currency.credited` regardless of direction.

- **Read.** Read the property; default to zero.

### 2.3 Pickup auto-conversion

Inventory pickup and give operations consult a hook
`TryAutoConvert(destination, item)` (referenced in
`docs/specs/inventory-equipment-items.md` §4.1):

1. Skip if the destination is not a player.
2. Skip if the item does not carry the `currency` tag.
3. Read the item's `value` property (integer, accepting `int`,
   `long`, `double`, or numeric string). If zero or missing,
   skip.
4. Remove the item from the destination's contents.
5. Untrack the item from the world (the coins become abstract
   gold, the item entity is gone).
6. Credit the destination with the value via AddGold, with
   reason `pickup:<templateId>`.
7. Return `true` — inventory operation reports success without
   emitting its own pickup event.

NPCs (including mobs that loot the player) do NOT trigger
auto-conversion. Only players have gold accounts in practice.

### 2.4 Quest reward integration

The currency service implements the quest feature's
`IQuestCurrencyService` interface (`AddGold(entity, delta,
reason)`). Quest rewards call this with reason
`quest_reward`. See `docs/specs/quests.md` §5.

**Acceptance criteria**

- [ ] Gold is floored at zero on every mutation.
- [ ] Add fires `currency.credited` on non-negative deltas and
      `currency.debited` on negative deltas.
- [ ] Set rejects negative input.
- [ ] Auto-convert applies only to player destinations, only
      to items tagged `currency` with a positive value.
- [ ] Auto-converted coins do not stay in inventory as items.
- [ ] Auto-convert suppresses the inventory feature's pickup
      event (currency feature emits its own).

---

## 3. Shops

### 3.1 Shop NPCs

A shop is an NPC carrying the `shop` tag and a `ShopConfig`
record. The config holds:

- A **sells** list of item template ids the shop offers.
- A per-shop **buy markup** (multiplier applied to base item
  value when computing buy price).
- A per-shop **sell discount** (multiplier applied when
  computing sell price).

Per-shop multipliers fall back to the global economy config
defaults when zero / unset. The global defaults today are
`1.2` markup and `0.5` discount; both are configurable.

### 3.2 Locating a shop

- **FindShopInRoom(player).** Returns the first NPC in the
  player's current room carrying the shop tag, or none.
- **IsShop(npc).** Tag check.

A room may contain multiple shop NPCs; only the first is
returned by the locator. The command layer chooses the
disambiguation (e.g. require an explicit shop target when
multiple exist).

### 3.3 Pricing

For an item with base value `V`:

- **Buy price** = `max(1, round(V × markup))`.
- **Sell price** = `max(1, round(V × discount))`.

Both prices are computed as `long` to tolerate very expensive
items, and floored at 1 so a shop never gives away or buys
for free.

The shop's per-config multipliers take precedence over the
global defaults when set to a positive value. Zero or unset
falls through to the global config.

### 3.4 Listings

`GetListings(npc)` returns a list of `(templateId, name,
buyPrice)` records, one per entry in the shop's sells list
where:

- The template id resolves in the item registry.
- The template's `value` property is positive.

Entries failing either check are silently dropped. Used by
shop-list rendering.

### 3.5 Buy

**Buy(player, npc, itemQuery):**

1. Read the player's gold.
2. **Resolve stock item by query** (§3.7). If no match or
   ambiguous, return `ItemNotForSale`.
3. Compute the buy price.
4. If player gold < price, return `InsufficientGold` with the
   computed price (so the caller can report "this costs X,
   you have Y").
5. Publish a **cancellable** `shop.buy` event carrying actor,
   shop, template id, amount. If cancelled, return
   `ItemNotForSale` with the computed price.
6. Debit the player by the price via AddGold (negative
   delta).
7. Create an item instance from the template; if creation
   fails, return `ItemNotForSale`. (Note: at this point the
   player has already been charged. The current implementation
   does NOT refund — see open questions.)
8. Track the new item in the world; add it to the player's
   contents.
9. Return `Ok` with the new item id, price, and the player's
   updated gold.

### 3.6 Sell

**Sell(player, npc, itemQuery):**

1. **Resolve inventory item by query** (§3.8). If no match,
   return `ItemNotInInventory`.
2. If the item carries the `no_sell` tag, return `ItemIsNoSell`.
3. Read the item's value. If zero or missing, return
   `ItemValueZero`.
4. Compute the sell price.
5. Publish a **cancellable** `shop.sell` event. If cancelled,
   return `ItemNotForSale` (sic — the event-cancel result
   uses the same reason as the no-shop-match path).
6. If the item is currently equipped, auto-unequip silently
   so the player isn't left wearing what they sold.
7. Remove the item from the player's contents; untrack it.
8. Credit the player with the sell price.
9. Return `Ok` with item name, price, updated gold.

### 3.7 Stock resolution

Stock queries resolve through the **shared keyword rules**
(`inventory-equipment-items` §6.1 — exact keyword, then
keyword prefix, then name substring), so a sells-list item
answers to the same words it does for look / get / wear. Each
entry's short id (the segment after the last `:`) participates
as a synthetic keyword, in both its hyphenated and spaced
forms, so a bare template id still resolves even when it
differs from the display name.

The result wins UNLESS the query matches more than one
sells-list entry — then the result is **ambiguous** and the
resolver returns "none". Callers report ambiguity as "the
shop doesn't carry that exactly" via the same `ItemNotForSale`
reason today.

### 3.8 Inventory resolution

Inventory queries resolve through the same shared keyword
rules (§6.1) against each held item, carried items before
equipped. The first match wins. Ambiguity is NOT detected on
the inventory side — the first matching item is sold.

### 3.9 Value query

**Value(player, npc, itemQuery):** Used to answer "how much
would you give me for this?" or "how much does X cost?".

1. Try to resolve the query as an inventory item (§3.8). If
   present, return the **sell** price (what the shop would
   pay), tagged with scope = inventory.
2. Otherwise try the stock list (§3.7). If present, return
   the **buy** price (what the player would pay), tagged
   with scope = stock.
3. Otherwise return `ItemNotForSale`.

Inventory-first ordering means a player asking about an item
they hold sees the price they'd receive, not the price the
shop sells it for.

### 3.10 Currency events on shop transactions

Shop buy / sell credit / debit the player via the currency
service, which emits its own `currency.credited` /
`currency.debited` events. The shop service ALSO emits its
own `shop.buy` / `shop.sell` events (as cancellable pre-
events). The two streams are independent and downstream
observers (audit log, achievements, quest tracking) can hook
either.

**Acceptance criteria**

- [ ] Shop config multipliers override global defaults only
      when positive.
- [ ] Prices are floored at 1.
- [ ] Buy resolves stock by keyword, partial name, or id;
      ambiguity prevents the sale.
- [ ] Buy fires the cancellable pre-event before charging.
- [ ] Sell auto-unequips silently before transferring.
- [ ] `no_sell` items reject sale.
- [ ] Value returns inventory price first, then stock price.

---

## 4. Sustenance

### 4.1 The pool

Every player (and conceptually NPCs, though content drives
this) carries an integer `sustenance` property in the range
`[0, 100]`. The pool starts at 100 (full) on character
creation (see `docs/specs/world-rooms-movement.md` and the
character-created subscriber that seeds it). The pool persists.

### 4.2 Tiers

Three tiers are derived from the current value via configured
thresholds:

- **Full** — value ≥ `TierFullMin` (default 67).
- **Hungry** — value ≥ `TierHungryMin` (default 34) and
  below the full threshold.
- **Famished** — below the hungry threshold.

Tier names are content-visible (used in render strings) and
the boundary values are configurable.

### 4.3 Regen multiplier

The sustenance config exposes a multiplier per tier:

- **Full** → 1.0 (baseline regen).
- **Hungry** → 0.5 (regen halved).
- **Famished** → 0.0 (no regen).

Regen-driving features (heartbeat tick, room healing,
abilities) consult `GetRegenMultiplier(currentSustenance)`
when computing the per-tick regen amount for an entity.
Sustenance itself does NOT modify HP / resource / movement
directly — it only scales the multiplier the consuming
feature applies.

### 4.4 Drain

The drain pipeline (the engine ticker that decrements
sustenance over time) is not implemented in this feature
file. The config carries the parameters used by the drain
subscriber:

- **DrainAmount** (default 1) — points decremented per drain
  tick.
- **DrainCadence** (default 300 ticks) — drain interval.
- **ReminderIntervalTicks** (default 3000) — minimum interval
  between hunger reminder messages.

These values are exposed for any feature that implements the
drain (typically a world-tick subscriber).

### 4.5 Replenishment

Sustenance is replenished by consuming food / drink (§6) and
by quest rewards or admin commands setting the property
directly. Replenishment is clamped to the 0..100 range; any
value above 100 is silently capped.

The 100 ceiling is an engine constant in the consume path,
not part of the config. Content cannot raise the cap.

**Acceptance criteria**

- [ ] Tiers honor configured thresholds (full / hungry /
      famished).
- [ ] Regen multiplier is 1.0 / 0.5 / 0.0 by default.
- [ ] Consume replenishment clamps at 100.
- [ ] Sustenance does not modify vitals directly; only the
      regen multiplier is exposed.

---

## 5. Rest

### 5.1 State machine

Each entity carries an optional **rest state** transient
property whose value is one of:

- `awake` — default state (also when the property is unset).
- `resting` — seated / lying on furniture.
- `sleeping` — deep rest.

Rest state is transient: it does not persist across save /
load. A disconnect that lands the entity in resting / sleeping
state restores as awake on the next login.

### 5.2 Auxiliary state

A resting / sleeping entity may carry:

- **rest target** — the entity id of the furniture being
  rested on. Cleared when state returns to awake.
- **rest bonus** — an integer bonus regen scaled into the
  multiplier (content-driven, optional).
- **sleep start tick** — the engine tick at which sleeping
  began. Used to award "well-rested" credit.

All three are transient.

### 5.3 Set state

`SetRestState(entityId, newState, furnitureId?)`:

1. Look up the entity. Return `(false, "entity_not_found")`
   on miss.
2. Read the current state (default `awake`). If equal to the
   requested new state, return `(false, "already_in_state")`.
3. Publish a **cancellable** `entity.rest_state.changed`
   event carrying old / new state. If cancelled, return
   `(false, "cancelled")`.
4. Write the new state.
5. Apply auxiliary changes:
   - Transition to `awake` clears the rest target.
   - Transition to `resting` or `sleeping` with a furniture id
     sets the rest target.
   - Transition to `sleeping` records the current tick as
     sleep start.
6. Return `(true, null)`.

### 5.4 Combat wake

A subscriber listening to `combat.engage` events checks the
target's rest state. When resting or sleeping, the target is
forcibly set to awake (clearing the rest target) and a
secondary `entity.rest_state.changed` event is emitted with
reason `combat`. This lives in the world event module — see
`docs/specs/world-rooms-movement.md` § integration notes.

Combat wake bypasses the cancellable change check. You don't
get to *refuse* to wake up when something attacks you.

### 5.5 Regen multipliers

The rest config exposes per-state multipliers:

- `resting` → 2.0 (default).
- `sleeping` → 3.0 (default).
- Everything else → 1.0.

The regen-driving feature consults `GetRestMultiplier(state)`
and composes it with the sustenance multiplier (§4.3),
typically by multiplying them.

### 5.6 Well-rested

The config carries a `MinSleepTicksForWellRested` value
(default 120). Content / regen features may consult the
recorded sleep-start tick to award a separate "well-rested"
bonus when waking after at least that many ticks. The rest
feature itself stores only the timestamp; the bonus logic
lives wherever it's applied.

### 5.7 Room healing rate

A room property `healing_rate` (integer, registered for
entity type `room`) provides an additive room-level bonus
that regen features may apply to entities resting in that
room. Inns, infirmaries, and shrines use this.

**Acceptance criteria**

- [ ] Rest state defaults to `awake` when the property is
      unset.
- [ ] SetRestState fails on same-state requests and on
      cancelled events.
- [ ] Transition to `sleeping` records the start tick.
- [ ] Transition to `awake` clears the rest target.
- [ ] Combat-engage forces sleeping / resting targets back
      to awake and emits an `entity.rest_state.changed`
      event with reason `combat`.
- [ ] Multipliers are 2.0 (resting) and 3.0 (sleeping) by
      default.

---

## 6. Consumables

### 6.1 Consumable item shape

A consumable item declares:

- **`consume_method`** (string) — `eat`, `drink`, or similar.
  Drives which dedicated command can consume it; carried in
  events for observers. The `eat` and `drink` verbs are strict
  (they consume only their own method); the `use` verb is a
  **generic fallback** that consumes any item declaring a
  consume_method, whatever its value (see §6.2). An item with
  no consume_method is not a consumable and no verb — not even
  `use` — will consume it.
- **`sustenance_value`** (int) — amount added to the
  consumer's sustenance on consume (clamped at 100, §4.5).
- **`effect_id`** (string) — name of an effect to apply (e.g.
  `bless`, `poison`). The consumable feature does not apply
  the effect itself; it emits an event carrying the id, and
  the effects feature subscribes.
- **`effect_duration`** (int) — pulses for the effect to
  last.
- **`effect_data`** (map of int) — transient parameters for
  the effect handler.
- **`charges`** (int) — current remaining charges.
- **`max_charges`** (int) — maximum charges (used by fill —
  see `docs/specs/inventory-equipment-items.md` §4.6).
- **`destroy_on_empty`** (bool, defaults to true) — whether
  to remove the item from the world when charges reach zero.

Any subset of these fields may be present. The item's
identity comes from its template; the consumable feature
treats them all as content-driven knobs.

### 6.2 Consume

`Consume(entityId, itemId)`:

1. Resolve the entity. If missing, return `ItemNotFound`.
2. Find the item in the entity's contents (top level only —
   nested-in-container items are not directly consumable).
   If missing, return `ItemNotFound`.
3. **Charge gate.** If the item declares `charges`:
   - Read the current charge value. If at or below zero,
     return `NoCharges`.
4. **Method gate.** Read the item's `consume_method`. A
   strict verb (`eat`/`drink`) consumes only items whose
   method matches; mismatch returns `WrongMethod`. The generic
   `use` verb consumes any item with a non-empty consume_method
   but returns `WrongMethod` for an item that declares none (so
   `use <sword>` never destroys gear). The method is also
   carried in the event.
5. Publish a **cancellable** `item.consuming` event carrying
   actor, item, consume method. If cancelled, return
   `Cancelled` (no charge spent, no destruction).
6. Read the effect parameters (`effect_id`, duration, data),
   sustenance value, and item name.
7. **Decrement / destroy.**
   - If charges are present, decrement by 1.
   - If new charges ≤ 0 AND `destroy_on_empty` is true (or
     unset, defaulting to true), mark the item for
     destruction.
   - If charges are NOT present at all, mark the item for
     destruction (single-use).
8. **Apply sustenance.** If `sustenance_value > 0`, read the
   entity's current sustenance (defaulting to 100), add the
   value, clamp at 100, write back.
9. **Emit `item.consumed`** with actor, item, item name,
   consume method, effect id / duration / data, sustenance
   value. The event MUST fire while the item still exists in
   memory so subscribers (the effects feature) can read its
   tags and properties.
10. **Destroy if marked.** Remove the item from the entity's
    contents and untrack it from the world.
11. Return `Success` with item id, name, consume method,
    sustenance value, and effect fields.

### 6.3 Effect application

The consumable feature DOES NOT call into the effect manager
directly. It emits `item.consumed` with the effect fields;
the effects feature subscribes and constructs an active
effect from the parameters. Decoupling lets content add new
effect types without modifying the consume path.

### 6.4 Charge management at fill time

The fill operation (see `docs/specs/inventory-equipment-
items.md` §4.6) tops a consumable's `charges` to its
`max_charges`. The consume operation only decrements. Other
recharge mechanics (recipes, scripted bonuses) act on the
property directly.

### 6.5 Nested-container restriction

`Consume` resolves the item from the entity's direct contents
only. An item inside a container in the entity's inventory
(e.g. a potion inside a satchel) is NOT directly consumable.
The player must take it out first. This is intentional — it
keeps consume O(1) on the player's top-level list — but it
may surprise content authors.

**Acceptance criteria**

- [ ] Consume requires the item to be a direct (top-level)
      child of the consumer's contents.
- [ ] Items with charges at zero fail with `NoCharges`
      without firing the pre-event.
- [ ] Pre-event cancel returns `Cancelled` without consuming
      a charge or destroying the item.
- [ ] Single-use items (no charge property) destroy on
      consume.
- [ ] `destroy_on_empty = false` keeps an empty item alive
      for refilling.
- [ ] Sustenance increase clamps at 100.
- [ ] The `item.consumed` event fires before the item is
      destroyed so subscribers can read its state.

---

## 7. Observable events

| Event | When |
|---|---|
| currency.credited | gold added or set positive (§2.2) |
| currency.debited | gold subtracted (§2.2) |
| shop.buy (cancellable) | before a buy transaction (§3.5) |
| shop.sell (cancellable) | before a sell transaction (§3.6) |
| entity.rest_state.changed (cancellable) | before a rest state change (§5.3); also after combat-driven wake (§5.4) |
| item.consuming (cancellable) | before a consume operation (§6.2) |
| item.consumed | after a successful consume, before destruction (§6.2) |

The sustenance feature emits no engine events; it is a value
plus tier-derivation helpers.

**Acceptance criteria**

- [ ] Every state transition in §2 - §6 emits exactly the
      listed event with the documented payload.
- [ ] `shop.buy`, `shop.sell`, `entity.rest_state.changed`,
      and `item.consuming` are the four cancellable events
      in this feature set.

---

## 8. Configuration surface

The following are externally configurable and not fixed by
this spec.

| Policy | Where it applies |
|---|---|
| Global shop buy markup and sell discount | §3.1 |
| Per-shop markup / discount overrides | §3.1 |
| Sustenance tier thresholds (full / hungry mins) | §4.2 |
| Sustenance tier regen multipliers | §4.3 |
| Sustenance drain amount, cadence, reminder interval | §4.4 |
| Rest multipliers (resting, sleeping) | §5.5 |
| Min sleep ticks for well-rested | §5.6 |
| `currency` and `no_sell` and `shop` tag names | §2.3, §3 |
| Default consumable `destroy_on_empty` behavior | §6.1 |
| The 100-point sustenance cap | §4.5 (hardcoded today) |

---

## 9. Open questions / future work

- **Buy refund on instance-creation failure.** The buy
  pipeline charges the player *before* creating the item
  instance. If item creation fails (corrupt template, missing
  data), the player loses gold with nothing to show. A two-
  phase commit (create item, then charge on success) would be
  safer.
- **Cancelled-shop-event reason is generic.** Both "no match"
  and "event cancelled" return `ItemNotForSale` to the
  caller. A distinct `Cancelled` reason would let scripts
  emit more informative messages.
- **Ambiguous shop sells silently miss.** When a player's
  query matches two stock items, the resolver returns no
  match (same outcome as "doesn't carry that"). A distinct
  `Ambiguous` outcome surfacing in the result would help
  hint the player.
- **Inventory resolution ignores ambiguity.** Unlike stock
  resolution, the inventory matcher returns the first match
  without checking for ambiguity. A player selling
  `dagger` with two daggers in inventory sells whichever is
  first.
- **Sustenance drain is not in this feature.** The drain
  loop is implemented by a separate world-tick subscriber.
  The config knobs live here, but the implementation is
  elsewhere — flagged because it makes the spec incomplete.
- **Sustenance cap of 100 is hardcoded.** The clamp in the
  consume path is a magic number. Externalize for content
  that wants a different scale.
- **Rest state is transient.** A player who disconnects
  while sleeping wakes up at next login. The well-rested
  timer also resets. Whether this is intentional or worth
  persisting is open.
- **No "currency type" abstraction.** Gold is a single
  integer; there is no copper / silver / platinum stack or
  multi-currency support. Content that wants regional
  currencies would need to extend the auto-convert path.
- **No-sell tag vs no-drop tag.** `no_sell` is enforced for
  shops; `no_drop` is not enforced anywhere (the inventory
  spec flags this). Symmetric enforcement would help quest
  designers.
- **Cancellable rest-state change has no reason field.**
  Listeners cancel by setting `cancel = true` but cannot
  attach a reason for why. Callers see only "cancelled".
- **`consume_method` is opaque to the engine.** The engine
  accepts any string and forwards it on the event. Commands
  `eat` / `drink` / `use` enforce the convention by
  filtering on the method; this is content discipline, not
  engine guarantee.

---

<!-- Generated: 2026-05-21 · Scope: CurrencyService + CurrencyProperties + EconomyConfig + ShopConfig + ShopProperties + ShopResults + ShopService + SustenanceConfig + SustenanceProperties + RestConfig + RestProperties + RestService + ConsumableProperties + ConsumableResult + ConsumableService · Spec style: narrative + acceptance criteria · Detail level: behavior only -->

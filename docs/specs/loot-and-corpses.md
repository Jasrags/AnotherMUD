# Loot and Corpses — Feature Specification

**Status:** Draft · **Scope:** Turning a mob death into lootable
drops — corpse creation, coin drops, looting rights, the loot/get
verbs, autoloot, and corpse decay · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the loot subsystem must do, not *how*
to implement it. Specific durations (ownership window, corpse
lifetime), coin amounts, and capacity ceilings are policy and live
in the configuration-surface table, not in the narrative.

---

## 1. Overview

When a mob dies, the items it was carrying must reach the players who
killed it. This feature owns the path from the **mob-killed** signal
to loot in a player's hands.

Loot *generation* is **not** owned here. A mob's drop list is rolled
at spawn time into the mob's contents (see
[mobs-ai-spawning](mobs-ai-spawning.md) §6.3), so a live mob already
carries the items it will drop. Combat decides *that* a mob died and
emits the canonical **mob-killed** event (see [combat](combat.md)
§6.3); this feature subscribes to that event and owns everything
after it.

### Core concepts

- **Corpse.** A transient container entity created in the room where
  a mob died. It holds the dead mob's contents (the spawn-time loot)
  plus any coins rolled on death. A corpse is an item whose entity
  type is `container` (see
  [inventory-equipment-items](inventory-equipment-items.md) §2.5), so
  the existing container-access machinery (look-in, get-from) applies
  to it.
- **Coin drop.** An optional currency amount a loot table declares,
  rolled when the corpse is created and deposited into the corpse as
  a coin pile.
- **Looting rights.** A short post-death **ownership window** during
  which only the killer (and, when grouping lands, the killer's
  group) may take from the corpse. After the window the corpse is
  open to anyone in the room.
- **Autoloot.** A per-player preference that, when enabled, transfers
  a freshly-created corpse's contents to the killer automatically.
- **Corpse decay.** A timed sweep that removes corpses (and their
  unlooted contents) after a configurable lifetime.

### Goals

1. Create a corpse holding the dead mob's loot when a mob-killed
   event fires for a mob that carried contents and/or a coin drop.
2. Roll an optional coin drop into the corpse from the mob's loot
   table.
3. Restrict looting to the killer for an ownership window, then open
   the corpse to all — without disclosing the gate to non-owners in a
   way that leaks ownership.
4. Provide the `loot` and `get … from <corpse>` verbs, honoring carry
   capacity and looting rights.
5. Provide a persisted per-player autoloot toggle that loots the
   killer's own kills automatically.
6. Decay corpses on a timed sweep so unlooted loot does not
   accumulate forever.

### Non-goals

- **Loot generation / loot tables' item rolls.** Owned by
  [mobs-ai-spawning](mobs-ai-spawning.md) §6.3. This spec only adds
  the *coin* block to a loot table and consumes the already-generated
  item contents.
- **The death decision, the cancellable death check, and killer
  attribution.** Owned by [combat](combat.md) §6. This feature
  consumes the resolved killer id that combat publishes.
- **Player corpses, gear loss, and respawn.** Combat §6.4 defers
  player-death recovery to a separate feature; today player death
  heals to a floor and relocates the player (no corpse). This spec is
  **mob loot only**.
- **Group loot *distribution rules*** (round-robin, need/greed, even
  coin split). Grouping is a separate, deferred system. This spec
  defines only the **rights seam** the group system plugs into (§4);
  today a "group" is just the killer.
- **Pickpocket / give-from-a-living-mob.** The other paths
  mobs-ai-spawning §6.3 leaves open. Out of scope; they may later
  share the loot-table substrate.

---

## 2. The corpse

### 2.1 Corpse creation on death

This feature subscribes to the **mob-killed** event ([combat](combat.md)
§6.3), which carries the mob template id, mob name, killer id, killer
name, and room id. On that event, before the mob is removed from the
world, the system MUST:

1. Publish a **cancellable** corpse-creating event (§8) carrying the
   victim mob, killer id, and room id. If any listener cancels (e.g.
   a summoned/illusory mob that should leave no body, a scripted boss
   with bespoke rewards), the system MUST NOT create a corpse and MUST
   skip the remaining steps. It remains the mob death-cleanup path's
   responsibility to remove the mob.
2. Create a corpse container entity in the victim's room. Its display
   name derives from the mob name via a configurable template (e.g.
   "the corpse of a goblin").
3. Transfer the dead mob's tracked contents into the corpse — a move,
   not a copy: each item leaves the mob and is re-placed inside the
   corpse, preserving instance identity and per-instance properties.
4. Roll the loot table's coin drop, if any (§3), and deposit the
   resulting coins into the corpse as a coin pile.
5. Record on the corpse: the killer id, the creation tick (for the
   ownership window and decay deadline), and — as the seam for
   grouping — the looting-rights owner set (today just the killer id).
6. Emit the corpse-created event (§8).

A corpse MUST be created even when the mob carried no items, **if**
the coin roll produced coins. A mob that carried no items and rolled
no coins produces **no** corpse (nothing to loot); the system MAY
still let content suppress an empty body via the cancellable event.

Corpse creation orders **after** combat's death disengagement and the
mob-killed emission, and **before** the mob is untracked/removed —
the contents move out of the live mob into the corpse as part of the
same death-cleanup sequence, so the items are never double-tracked
and never orphaned.

**Acceptance criteria**

- [ ] A mob-killed event for a mob carrying contents creates a corpse
      in the mob's room holding exactly those contents.
- [ ] The contents are *moved* (each item leaves the mob and is
      tracked inside the corpse); no item is duplicated or left on the
      removed mob.
- [ ] A cancelled corpse-creating event suppresses the corpse; the
      mob is still removed by the death-cleanup path.
- [ ] A mob with neither items nor a coin drop produces no corpse.
- [ ] A corpse records the killer id and its creation tick.

### 2.2 The corpse as a container

A corpse is a container item, so reads and takes flow through the
existing container access path
([inventory-equipment-items](inventory-equipment-items.md) §4):

- Looking in / at a corpse lists its contents (items and, if present,
  the coin amount), subject to the same room-presence rules as any
  container.
- Taking from a corpse uses the `get … from` path (§5.2), gated by
  looting rights (§4).
- A corpse does **not** accept `put` — it is a loot source, not
  storage. Attempts to put into a corpse are refused.
- A corpse is a no-get fixture for the corpse *item itself*: a player
  cannot pick up the corpse and carry it; only its contents leave it.

**Acceptance criteria**

- [ ] Looking in a corpse lists its items and coin amount.
- [ ] A corpse refuses `put`.
- [ ] The corpse entity itself cannot be picked up; only its contents
      can be taken.

---

## 3. Coin drops

A loot table MAY declare an optional **coin block** alongside its item
pools (the item pools are resolved at spawn per
[mobs-ai-spawning](mobs-ai-spawning.md) §6.3; the coin block is
resolved here, at corpse creation). The coin block declares a coin
amount as a range or dice expression in the world's base currency
(see [economy-survival](economy-survival.md) §2.1).

At corpse creation (§2.1 step 4) the system rolls the coin block once
and deposits the result as a coin pile in the corpse. A zero roll (or
no coin block) deposits no coins. Coins in a corpse are looted like
any other content (§5) but credit the looter's **currency balance**,
not their item inventory.

**Acceptance criteria**

- [ ] A loot table with a coin block rolls a coin amount into the
      corpse at creation.
- [ ] A loot table with no coin block (or a zero roll) deposits no
      coins.
- [ ] Looting coins increases the looter's currency balance, not
      their carried item list.

---

## 4. Looting rights

A corpse records its **owner set** at creation (§2.1 step 5). For a
configurable **ownership window** measured from the creation tick,
only members of the owner set may take from the corpse. After the
window elapses the corpse is **open** — anyone in the room may loot
it.

The owner set is the **grouping seam**: today it is exactly the
killer id. When grouping lands, the killer's group members join the
owner set at creation; the rights check is unchanged. This feature
defines the check (`may a given actor loot this corpse?`) and the
window; it does **not** define group membership.

Rights are enforced on every take (§5) — the `loot` verb and
`get … from <corpse>`. A take attempted by a non-owner during the
ownership window is refused with a message that does **not** name the
owner (no ownership disclosure beyond "someone else's kill"). An
absent killer attribution (combat §6.2 allows an empty killer) yields
an **immediately open** corpse — there is no owner to reserve it for.

**Acceptance criteria**

- [ ] During the ownership window, only an owner-set member may take
      from the corpse.
- [ ] After the window, any actor in the room may take from the
      corpse.
- [ ] A corpse created with no killer attribution is open
      immediately.
- [ ] A non-owner take during the window is refused without naming
      the owner.

---

## 5. Looting verbs

### 5.1 `loot`

`loot <corpse>` takes everything the actor is allowed to take from the
named corpse: all items that fit the actor's carry capacity, plus any
coins. `loot` with no argument targets a corpse in the room by a
configurable default rule (e.g. the most recently created corpse the
actor may loot); ambiguity falls back to requiring an explicit
target.

Looting is **partial on capacity**: items that fit are taken and the
rest remain in the corpse, with a message naming what was left.
Coins always transfer (currency has no carry-weight). After a `loot`
that empties the corpse, the corpse is removed and a corpse-looted
event fires (§8); a corpse emptied only of coins but still holding
unfittable items is **not** removed.

Looting is gated by §4 rights and refused (without side effects) when
the actor is not allowed to loot.

**Acceptance criteria**

- [ ] `loot <corpse>` transfers all fitting items plus all coins to
      the actor.
- [ ] Items that exceed carry capacity remain in the corpse; the
      actor is told what was left.
- [ ] A corpse emptied by looting is removed and emits corpse-looted.
- [ ] `loot` with no target resolves a default corpse or asks for an
      explicit one.

### 5.2 `get … from <corpse>`

Taking a single item (or a stack) from a corpse uses the existing
container `get … from` path
([inventory-equipment-items](inventory-equipment-items.md) §4),
additionally gated by §4 looting rights. Coins are taken with the
same verb by a reserved coin keyword (e.g. `get coins from corpse`),
crediting the currency balance.

**Acceptance criteria**

- [ ] `get <item> from <corpse>` moves one resolved item, gated by
      looting rights and carry capacity.
- [ ] Taking the corpse's coins via the verb credits the currency
      balance.

---

## 6. Autoloot

Each character carries a persisted **autoloot** preference, **off by
default**, toggled by an `autoloot on|off` verb (and reported by
`autoloot` with no argument). The preference persists on the player
save (see Save/load below).

When a corpse is created (§2.1) for a kill whose killer is a player
with autoloot **on** and who is present in the corpse's room, the
system immediately performs a `loot` (§5.1) on that corpse on the
killer's behalf, as the killer (so §4 rights are trivially
satisfied — the killer owns their own fresh kill). Capacity rules
apply: items that do not fit stay in the corpse for a manual `loot`.

Autoloot is **scoped to the killer's own kills** in v1 and loots
**everything** (items + coins). Narrower scopes — coins-only, or a
rarity-tier floor (see [item-decorations](item-decorations.md)) — are
future refinements (§10), not v1.

**Acceptance criteria**

- [ ] With autoloot off (the default), a kill leaves a full corpse.
- [ ] With autoloot on, the killer's own fresh kill is looted
      automatically at corpse creation.
- [ ] Autoloot honors carry capacity: unfittable items remain in the
      corpse.
- [ ] The autoloot preference persists across logout/login.

---

## 7. Corpse decay

Corpses are removed on a timed sweep — the **corpse-decay** tick
handler (reserved in [time-and-clock](time-and-clock.md) §3's handler
table). Each corpse carries a decay deadline derived from its creation
tick plus a configurable **corpse lifetime**. On each sweep, every
corpse past its deadline is removed along with **all of its remaining
contents** — unlooted items and coins are destroyed with the corpse.

Decay removal emits a corpse-decayed event (§8). Destroying unlooted
loot (rather than spilling it to the room) is the v1 rule; it bounds
world growth and rewards timely looting. (Spill-to-room is noted as an
alternative in §10.)

Corpses are **not persisted** (see Save/load): a server restart
removes all corpses and their unlooted loot, like mob spawn tracking
and temporary exits.

**Acceptance criteria**

- [ ] A corpse past its lifetime is removed by the decay sweep.
- [ ] Decay destroys the corpse's remaining contents (items and
      coins).
- [ ] Decay removal emits a corpse-decayed event.

---

## 8. Observable events

The feature MUST publish events on the engine bus for every state
transition observers care about:

| Event | When | Cancellable |
|---|---|---|
| `corpse.creating` | before a corpse is created on mob death (§2.1) | **yes** |
| `corpse.created` | a corpse was created (§2.1) | no |
| `corpse.looted` | a corpse was emptied by looting and removed (§5.1) | no |
| `corpse.decayed` | a corpse was removed by the decay sweep (§7) | no |

Each event carries enough for listeners to act without further
queries: the room id, the source mob template id and name, the killer
id (where known), and — on creation — the item count and coin amount.
A `corpse.creating` listener that cancels suppresses the corpse
entirely (no items move, no coins roll); it is the listener's job to
handle the loot it suppressed (e.g. a scripted boss that grants
bespoke rewards instead).

**Acceptance criteria**

- [ ] `corpse.creating` is published before any contents move and is
      cancellable.
- [ ] Cancelling `corpse.creating` leaves the mob's contents on the
      (about-to-be-removed) mob and creates no corpse.
- [ ] `corpse.created`, `corpse.looted`, and `corpse.decayed` fire on
      their respective transitions with the documented payload.

---

## 9. Configuration surface

| Setting | Meaning |
|---|---|
| Ownership window | How long after creation a corpse is reserved for its owner set (§4). |
| Corpse lifetime | How long a corpse persists before the decay sweep removes it (§7). |
| Corpse-decay sweep cadence | Tick interval of the decay handler (§7; time-and-clock handler table). |
| Autoloot default | The preference value for a new character (off) (§6). |
| Corpse display-name template | How a corpse's name derives from the mob name (§2.1). |
| Corpse container capacity | Capacity ceiling for a corpse, if any (§2.2) — a corpse must hold whatever the mob carried. |
| Loot-table coin block | Per-loot-table coin amount (range / dice) (§3) — content. |

All values are policy; none appear in the narrative.

---

## 10. Open questions / future work

- **Coins rolled at death vs at spawn.** Item loot is rolled at spawn
  and inspectable on the live mob (mobs-ai-spawning §6.3); coins are
  rolled here, at corpse creation, so a live mob's eventual coin drop
  is *not* inspectable. Rolling coins at spawn (stored as a mob
  property) would restore symmetry at the cost of extending §6.3.
- **Decay: destroy vs spill.** v1 destroys unlooted loot on decay.
  Spilling it to the room floor instead would preserve loot but
  reintroduce the room-clutter and re-decay problem.
- **Group loot distribution.** This spec defines only the rights seam
  (§4). Round-robin / need-greed / even coin split are owned by the
  deferred grouping system; the owner set is where it attaches.
- **Autoloot scope.** v1 loots everything from the killer's own kills.
  Coins-only and rarity-tier-floor filters (item-decorations) are
  future toggles.
- **Player corpses.** Still deferred to a player-death feature (combat
  §6.4); if player corpses land, decide whether they share this
  corpse-as-container substrate or need gear-loss/soulbound rules of
  their own.
- **Multiple corpses, `loot` ambiguity.** The no-argument `loot`
  default rule (most-recent? nearest?) needs a concrete choice when a
  room holds several lootable corpses.
- **Corpse persistence.** Corpses are transient (lost on restart). A
  persistent-world variant would need them saved like the auction
  listing store.
- **Pickpocket / give-from-mob.** The other mobs-ai-spawning §6.3
  deferrals; could share the loot-table + corpse substrate later.

---

## Save / load surface

- **Player file** — gains the **autoloot preference** (a single
  boolean field/property; §6). No other loot state is per-player.
- **NOT persisted** — corpses and their contents are transient world
  objects (like mob spawn tracking, temporary exits, and weather); a
  restart removes them and any unlooted loot.

---

## Cross-references

- [combat](combat.md) §6.3 — the **mob-killed** event this feature
  subscribes to; §6.2 killer attribution (may be empty → open
  corpse).
- [mobs-ai-spawning](mobs-ai-spawning.md) §6.3 — loot **generation**
  at spawn into the mob's contents (the items this feature moves to
  the corpse); this spec adds the coin block to a loot table.
- [inventory-equipment-items](inventory-equipment-items.md) §2.5,
  §4 — container items and the get/look-in access path the corpse
  reuses.
- [economy-survival](economy-survival.md) §2.1 — the currency a coin
  drop credits.
- [time-and-clock](time-and-clock.md) §3 — the `corpse-decay` tick
  handler.
- [item-decorations](item-decorations.md) — rarity tiers a future
  autoloot filter would key on.

---

<!-- Draft: 2026-06-02 · Scope: corpse creation on mob death, coin drops, looting rights/window, loot + get-from verbs, autoloot toggle, corpse decay · Spec style: narrative + acceptance criteria · Detail level: behavior only. Build pending. -->

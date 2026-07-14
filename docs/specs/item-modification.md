# Item Modification (capacity · installed mods)

Greenfield engine primitive. *Spec ahead of code — build pending.* Carved out of
`inventory-equipment-items` (which lists "item modification" as an explicit
non-goal, §"Non-goals") as its own feature. Layers on
`inventory-equipment-items` (items, the equip modifier pipeline),
`item-decorations §1.1` (the "power is independent of presentation" discipline),
and reuses the item registry (`internal/item`) — a modification **is an item**.
The Shadowrun pack is the reference consumer (armor + armor mods); the primitive
is ruleset-agnostic.

## 1. Overview

Some gear can be **modified** — a host item accepts installed modification-items
that persist on it and, while it is equipped/used, contribute their own effects.
This spec defines that shared **substrate** (a mod is an item; install/remove;
per-instance persistence; equip-time aggregation; presentation) **plus one
admission rule** — the **capacity budget** — that governs what fits.

In Shadowrun the capacity rule is **Capacity**: an armor vest with capacity 9
accepts mods whose capacity costs sum to at most 9; installing Shock Frills
(cost 2) leaves 7. **Cyberware clusters** (cybereyes, cyberlimbs) work the same
way — a capacity budget their enhancements consume — so they are a later *host
domain* of this same rule, not a new mechanic.

**Weapons use a *different* admission rule** — a fixed set of named mount points,
each holding one accessory — specified separately in `weapon-accessories.md`,
layered on **this spec's substrate**. The two admission rules (capacity budget /
named mounts) share everything except *what may be installed*; the substrate here
is written generically over the admission rule so the weapon spec bolts on without
reworking it.

The engine has no such concept today. Every "depth" field an item carries
(grade, armor bonus, resistances, essence cost) attaches to *one* item; nothing
composes a child item **into** a parent. The one existing exception is narrow and
instructive: an ammunition **holder inserted into a weapon** already persists as
instance state on the host (`ammo-and-reloading`; `entities.ItemInstance`
inserted-holder properties). This feature generalizes that item-on-item
composition and adds the missing piece — a **capacity budget** that governs what
may be installed.

Two things stay **independent**, mirroring `item-decorations §1.1`:

- **The mod is mechanical.** Its effect rides the **existing equip modifier
  pipeline** — an installed mod contributes its stat modifiers / properties
  through the same seam a worn item's own modifiers use (`inventory-equipment-
  items §2.3` step 6). A mod never invents a new resolution path; a Fire
  Resistance mod contributes mitigation exactly as intrinsic armor resistance
  does (`armor-depth §4`).
- **Capacity is bookkeeping.** The budget gates *what can be installed*; it is
  not itself a combat or resolution value.

**Goals.** Give a host item a **capacity budget**; let **modification items** be
installed into a compatible host, consuming capacity and validated against the
remaining budget; **aggregate** installed-mod effects into the host's
contribution while the host is equipped/used; persist installed mods as durable
instance state; expose an install/remove flow.

**Non-goals (this slice).**

- **Core source only.** Only host items and modifications drawn from the
  **`Core`** source book (`docs/shadowrun/ARMOR.md`) are in scope. Mods from
  R&G / Hard Targets / other books are deferred, and with them two data shapes
  they introduce: the **compound capacity form** (`[4]+[2]`, RIG) and the
  **no-capacity** `—` mod (YNT Softweave). v1 models a single non-negative
  capacity cost per mod.
- **Weapon and cyberware hosts.** This slice authors and accepts only **armor**
  hosts + **armor** mods. **Weapon** modification uses a *different admission
  rule* (named mount slots) specified separately in `weapon-accessories.md`,
  layered on this spec's substrate — not "the same machinery," only the same
  substrate. **Cyberware cluster** modification (enhancements into a
  cybereye/cyberlimb) reuses *this* spec's capacity rule as another host domain,
  but is deferred and interacts with Essence (SR-M4, shipped) and ware grades,
  which are their own features. No weapon/cyberware content here.
- **Crafting / installation skill checks / an Armorer facility.** Whether
  installation requires an NPC (an armorer, like the ripperdoc gates cyberware),
  a skill test, time, or tools is out of scope — v1 is a direct player command
  (§5). The facility gate is a content concern layered on later.
- **Availability / cost / legality (the source's Avail/¥ columns).** Economy and
  black-market gating are the shop/economy feature's concern, not this spec.

## 2. The host and its capacity

A **modifiable host** is an ordinary item (`inventory-equipment-items`) that
declares a **capacity budget** — the total modification space it offers. An item
that declares no budget is unmodifiable and behaves exactly as today.

- The budget is a single non-negative amount, item metadata on the host template
  (an armor vest offers more than an armor jacket; the magnitudes are content,
  §10). Zero or absent ⇒ unmodifiable.
- **Used capacity** is the sum of the capacity costs of the mods currently
  installed in the host. **Free capacity** is `budget − used`, never negative
  (the install rule §4 enforces it).
- The budget is a property of the **host kind** (its template), but *which* mods
  are installed is **per-instance** durable state (§7) — two armor vests off the
  same template can carry different mods.

**Acceptance criteria**

- [ ] An item declaring a capacity budget is modifiable; one declaring none (or
      zero) is unmodifiable and unchanged from today.
- [ ] Used capacity equals the sum of installed mods' costs; free capacity is
      budget minus used and is never negative.
- [ ] The budget is template metadata; the installed-mod set is per-instance.

## 3. The modification

A **modification** is itself an **item** (a template in the item registry, so it
can be bought, carried, looted, and traded like any other) that declares:

- A **capacity cost** — how much of a host's budget it consumes (§4). A cost may
  be **rating-scaled**: several Core mods (Chemical Protection, Fire Resistance,
  Insulation, Nonconductivity, Thermal Damping) cost `[Rating]`, so the mod
  carries a **rating** and its effective capacity cost is derived from it. A
  flat-cost mod (Shock Frills, Chemical Seal) has no rating. (The compound
  `[X]+[Y]` form is out of scope, §1.)
- A **host-compatibility key** — the class of host it fits (this slice: armor).
  A mod may only be installed into a host whose kind matches (§4). An armor mod
  cannot be installed into a weapon.
- Its **effect** — the stat modifiers / properties it contributes to the host's
  equipped contribution (§6), authored exactly like an ordinary item's
  modifiers. A mod whose effect is a resistance contributes through
  `armor-depth §4`; a mod granting an AC term through `armor-depth §3`; and so
  on. The mod declares *what it does* the same way any equippable does.
- Its **rating**, when rating-scaled — a small positive integer that scales both
  the capacity cost and (typically) the effect magnitude. **v1 may model each
  rating as a distinct template** with its cost and effect baked in as flat values
  (the way the source lists Cybereyes R1–R4 and the codebase authors them — one
  row per rating), so **no runtime rating-choice mechanic is required** and every
  Core mod reduces to a *flat* capacity cost. A single-template-carrying-an-
  instance-rating form is a later refinement (§11).

A modification, while **uninstalled**, is an inert item in inventory — it grants
nothing until installed into a host and the host is equipped/used.

**Acceptance criteria**

- [ ] A modification is an ordinary registry item (buyable / carriable / lootable).
- [ ] A mod declares a capacity cost (flat, or derived from a rating for the
      `[Rating]` Core mods) and a host-compatibility key.
- [ ] An uninstalled mod grants no effect; its effect applies only via §6.

## 4. Installing a modification

**v1 scope (Slice A).** Both the host and the mod are resolved from the actor's
**inventory**, and the host must be **carried (unequipped)** to be modified — a
bench action. This keeps effect aggregation (§6) computed fresh on the next equip
and avoids any reverse-while-worn recompute; the "reverse a mod on an *equipped*
host" convenience of §5 is deferred. Modifying worn gear tells the player to take
it off first.

Installing moves a mod from the actor's inventory **into a host** the actor
holds, subject to validation:

1. **Compatibility** — the mod's host-compatibility key matches the host's kind;
   otherwise the install is refused with a clear reason.
2. **Capacity** — the mod's (rating-derived) capacity cost is `≤` the host's
   **free capacity**; otherwise refused, naming the shortfall.
3. On success the mod item is **consumed from inventory and recorded on the host
   instance** (§7); the host's used capacity rises by the cost.

Installation is validated at the boundary and **fails fast** with a
user-facing message (`coding-style`: validate input, clear errors) — over-
capacity, wrong host type, no such mod, no such host all produce distinct cues.

A host may hold **multiple** mods so long as their costs fit the budget. Whether
the **same** mod may be installed twice (two identical mods stacking) is host/mod
policy; the default is that duplicate installation is allowed only if the source
does not forbid it — a per-mod open question (§11), not a v1 blocker (Core mods
are singular in practice).

**Acceptance criteria**

- [ ] Installing an incompatible mod (wrong host kind) is refused with a reason.
- [ ] Installing a mod whose cost exceeds free capacity is refused, naming the
      shortfall; a mod that fits is installed and free capacity decreases.
- [ ] A successful install consumes the mod item from inventory and records it on
      the host instance.
- [ ] Multiple mods coexist in one host while their summed cost fits the budget.

## 5. Removing a modification

Removing reverses an install: the mod is taken **out of the host** and returned
to the actor's inventory as an item, and the host's free capacity is restored.

- The removed mod becomes an ordinary inventory item again (re-installable
  elsewhere), unless a per-mod policy marks it **destroyed-on-removal** (some
  mods are damaged when pried out); v1 default is **removable and recovered**,
  with destroy-on-removal a recorded per-mod flag reserved for later (§11).
- If the host is currently **equipped**, removal also reverses that mod's
  contribution to the wearer (§6) — the aggregate modifiers are recomputed.

**Acceptance criteria**

- [ ] Removing an installed mod returns it to inventory and restores the host's
      free capacity.
- [ ] Removing a mod from an equipped host reverses that mod's contribution to
      the wearer.
- [ ] A mod marked destroy-on-removal (reserved) is consumed rather than
      returned; the v1 default recovers it.

## 6. Effect aggregation (equipped hosts)

Installed mods take effect **only while the host is equipped/used**, and they do
so through the **existing equip modifier pipeline** — this is the load-bearing
seam of the feature.

- When a host is equipped, its contribution to the wearer is the **union** of
  the host's own modifiers/properties **and** the modifiers/properties of every
  installed mod. The wearer's `Stats()` (and any equip-derived value: AC,
  mitigation, resistances) reads that union.
- **Aggregation covers the typed depth fields, not only the generic modifier
  list.** The equip step snapshots armor's *typed* fields (armor bonus, max-Dex
  cap, per-type resistances — `armor-depth §3–4`) separately from the generic
  `[]Modifier` list. A Fire Resistance mod's resistance merges into the host's
  **effective resistance map**; a Chemical Seal mod's property into the host's
  properties. An implementation that folds only the generic modifiers would
  silently drop any mod whose effect rides a typed field — the load-bearing
  detail of this seam.
- Contributions are **tagged by source** so they reverse cleanly on unequip or
  on mod removal (`inventory-equipment-items §2.3` step 6 tags modifiers by
  source key). Each installed mod is a distinct source; removing it or unequipping
  the host reverses exactly its terms.
- Aggregation is **additive across distinct sources**, consistent with
  `armor-depth §3`–§4 (armor bonus / per-type mitigation stack across sources).
  A Fire Resistance mod adds to the host's fire mitigation; a Chemical Seal mod
  adds a chemical-protection property; etc.
- A mod installed in an **unequipped** host contributes nothing (it is latent
  instance state until the host is worn).

**Acceptance criteria**

- [ ] Equipping a modded host applies the host's own modifiers plus every
      installed mod's modifiers; unequipping reverses all of them.
- [ ] Each installed mod's contribution is tagged by a distinct source and
      reverses independently on removal.
- [ ] Mod contributions stack additively with the host's own and with other
      worn sources, consistent with `armor-depth`.
- [ ] A mod in an unequipped host contributes nothing.

## 7. Persistence

Installed mods are **durable instance state** on the host item — a modded vest
stays modded across save/load, in inventory or equipped.

- The host `ItemInstance` records its installed mods (the mod template id, and
  each mod's per-instance data — its rating, and any grade/decoration the mod
  item itself carries), following the precedent of the inserted-holder instance
  properties (`ammo-and-reloading`; `entities.ItemInstance`).
- Because this adds durable player-reachable item state, it is a **player save
  version bump** with an append-only migration (`internal/player`,
  `CurrentVersion`) — a pre-migration save simply has no modded items; the
  migration is additive and lossless.
- Free/used capacity is **derived** from the installed-mod set at load, not
  separately persisted (single source of truth — mirrors how the essence pool's
  current is derived from installed cyberware, SR-M4).

**Acceptance criteria**

- [ ] A host's installed mods (with each mod's rating and own decorations)
      survive save/load whether the host is carried or equipped.
- [ ] The save version is bumped with an additive migration; pre-migration saves
      load cleanly with no modded items.
- [ ] Used/free capacity is recomputed from the installed set on load, not
      persisted independently.

## 8. Presentation

Modification state is visible to the player.

- `look <host>` (and/or `examine`) shows the host's installed mods and its
  **used / free capacity** (`ui-rendering-help` appearance lens).
- Installing / removing emits a confirmation cue naming the mod, the host, and
  the resulting free capacity.
- A refused install names *why* (incompatible host, or the capacity shortfall).

**Acceptance criteria**

- [ ] Examining a modifiable host lists its installed mods and its used/free
      capacity.
- [ ] Install/remove and refusal each emit a clear, specific cue.

## 9. Interaction with existing systems

- **Item stacking** (`inventory-equipment-items`, M21): an installed mod is
  per-instance state (§7), so a modified host **cannot stack** with an
  unmodified one (or a differently-modded one) — it is effectively unique.
  Modifying a member of a stack splits it out; a host with no mods stacks
  exactly as today.
- **Action economy / combat** (`action-economy`, `armor-depth §7`): installing
  and removing a mod is a deliberate bench action, not a combat move. Install and
  remove follow the same **combat/busy gate** as don/doff — barred while the actor
  is in combat (or busy) — so a host cannot be re-modded mid-fight. This reuses
  the don/doff machinery rather than inventing a new timer.
- **Trade / auction** (`direct-trade`, `trade-escrow`, `auction-house`): a
  modified host travels with its installed mods intact (they are instance state,
  §7) — trading the host trades its mods. An installed mod is not independently
  tradable while installed; remove it first (§5).
- **Masterwork / decorations** (`masterwork`, `item-decorations`): a mod may
  itself carry a grade or rarity (its own instance data, §7), independent of the
  host's grade/decoration; the two do not derive from each other (mirroring
  `item-decorations §1.1`).
- **Equipment slots** (`inventory-equipment-items §3`): modification never
  changes which slot the host occupies — a modded vest is still one body-armor
  slot. Capacity is internal to the item.

## 10. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Host capacity budgets | Per-host-template capacity (§2). | per-item content metadata; absent ⇒ unmodifiable |
| Mod capacity costs | Per-mod flat or rating-derived cost (§3, §4). | per-mod content metadata |
| Host-compatibility keys | The classes of host a mod fits (§3). | this slice: armor only |
| Duplicate-install policy | Whether the same mod may be installed twice (§4). | allowed unless the mod forbids it |
| Destroy-on-removal | Per-mod flag: consumed rather than recovered on removal (§5). | recovered (removable) |
| Installation gate | Whether install requires an NPC / skill / tools / time (§1 non-goal). | none (direct command) in v1 |

All numeric magnitudes (budgets, costs, ratings) live in content; the prose names
behaviors, not values (spec convention).

## 11. Open questions

- **Admission rules and host domains.** Two admission rules ride this substrate:
  the **capacity budget** (this spec — armor, and later cyberware clusters) and
  **named mount slots** (`weapon-accessories.md` — weapons). Confirm the shared
  substrate (install/remove §4–5, instance persistence §7, equip aggregation §6,
  presentation §8) is expressed generically over the admission rule so the weapon
  spec bolts on without reworking this one. **Cyberware clusters** add a wrinkle —
  the mod-host is itself a character-slot implant (SR-M4) with an Essence/ware-
  grade interaction — folded in when that slice lands, not here.
- **Compound and no-capacity mods** — the `[X]+[Y]` compound form (RIG) and the
  `—` no-capacity mod (YNT Softweave) arrive with the R&G source book; the data
  model must grow a way to express "consumes two pools" and "consumes none" then.
  Deferred with the non-Core content.
- **Rating as capacity vs. effect, and template vs. instance rating** — for
  `[Rating]` mods the rating scales both the capacity cost and the effect; confirm
  one rating drives both (the SR reading). And decide whether v1 **bakes each
  rating into its own template** (flat cost, no runtime rating — simpler, matches
  how rated cyberware is authored, §3) or carries a rating on one template. The
  former is the recommended v1 default and collapses the `[Rating]` handling to
  ordinary flat costs.
- **Duplicate installation** — whether two of the same mod stack (effect and
  cost both double) or are forbidden is per-mod; Core mods are singular so this
  is non-blocking, but the flag shape should be decided when authored.
- **Destroy-on-removal** — the reserved per-mod flag (§5); no Core armor mod
  needs it, so it ships inert (recorded, no consumer) until a mod that does.
- **Installation gate** — an Armorer facility / skill test / time cost is the
  natural SR-faithful gate (parallels the ripperdoc gating cyberware). Left to a
  content/economy slice; v1 is a direct command so the mechanic can be exercised
  first.

---

<!-- Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: greenfield, spec ahead of code (build pending). Scope: Core-source armor hosts + armor mods only; weapon/cyberware modification, non-Core mods, the compound/no-capacity capacity forms, and the installation facility gate are deferred. Carved out of inventory-equipment-items' "item modification" non-goal. Reuses the equip modifier pipeline (§6) + the inserted-holder instance-state precedent (§7); player save-version bump required. -->

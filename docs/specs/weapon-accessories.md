# Weapon Accessories (mount slots)

Greenfield engine slice. *Spec ahead of code — build pending.* The **second
admission rule** of item modification: where armor/cyberware use a **capacity
budget** (`item-modification.md`), a weapon exposes a fixed set of **named mount
points**, each holding at most one accessory. This spec owns **only** that
admission rule; it **reuses the shared substrate** of `item-modification.md` (a
mod is an item; install/remove flow; per-instance persistence; equip-time
effect aggregation; presentation) rather than restating it. Layers on
`item-modification` (substrate), `inventory-equipment-items` (items, equip
pipeline), `weapon-identity` / `ranged-combat` / `ammo-and-reloading` (the
weapons being modified). Shadowrun firearms are the reference consumer.

## 1. Overview

A firearm has a fixed physical geometry — a barrel, an under-barrel rail, sides,
a top rail, a stock, and internal space. Accessories bolt onto those specific
**mount points**, and each mount holds one thing: a barrel wears a silencer **or**
a longbarrel, not both. This is **slot occupancy**, not a fungible budget — there
is no number to sum. An accessory declares **which mount(s) it can occupy**; a
weapon declares **which mounts it exposes**; attachment succeeds when a compatible
mount is free.

This differs from `item-modification`'s capacity rule only in the **admission
test** (a free compatible mount vs. `Σcost ≤ budget`) and the host/mod data shape
(a mount **set** vs. a capacity **number**). Everything downstream — the accessory
*is* an item, attaching moves it onto the host instance, it persists, and while
the weapon is wielded its effects fold into the weapon's contribution through the
equip pipeline — is the shared substrate (`item-modification §4–8`), referenced
here, not redefined.

**Goals.** Give weapons the source's mount-point geometry; let **accessory items**
attach to a compatible free mount and detach cleanly; validate attachment by mount
compatibility and occupancy; ride the shared modification substrate for effect,
persistence, and presentation.

**Non-goals (this slice).**

- **Core source only.** Only weapons and accessories drawn from the **`Core`**
  source book (`docs/shadowrun/WEAPONS.md`) are in scope — Silencer/Suppressor,
  Smartgun System (External / Internal), Gas-Vent System, Laser Sight, Bipod,
  Tripod, Gyro Mount, Imaging Scope, Periscope, Airburst Link, Smart Firing
  Platform, Shock Pad, Hidden Arm Slide, the holster/clip/loader accessories, etc.
  R&G / Hard Targets / other-book accessories are deferred.
- **The capacity admission rule.** Owned by `item-modification.md`; a weapon uses
  mounts, not a budget. (A pack that wanted a *capacity-modded* weapon would use
  that rule instead — not modeled here.)
- **New weapon mechanics for the accessories' effects.** An accessory's effect
  rides existing seams (a recoil-reduction modifier, a to-hit modifier, a
  `ranged-combat` range term, a smartlink flag). Inventing new combat resolution
  for an accessory is out of scope; an accessory whose effect has no live seam yet
  attaches and records inertly (the `weapon-identity` "record now, light up later"
  pattern).
- **The smartgun ↔ smartlink pairing mechanic.** A Smartgun System's benefit
  requires a matching smartlink (cybereye or goggles) on the wielder. This spec
  lets the smartgun *attach* and record that it is present; the *paired bonus*
  (and its interaction with cyberware, `item-modification §"cyberware"`) is a
  follow-on — flagged in §10, not built here.
- **Installation gate.** Whether attaching needs an armorer / tools / time is out
  of scope (as in `item-modification §1`); v1 is a direct command.

## 2. Mount points (the weapon's geometry)

A **modifiable weapon** declares the set of **mount points** it exposes — a
weapon that declares none accepts no accessories and is unchanged from today. Each
mount is a named position drawn from a **pack-declared mount vocabulary**; the
Shadowrun reference set is **barrel · under-barrel · side · top · stock ·
internal** (`WEAPONS.md` accessory columns).

- A mount point holds **at most one** accessory (occupancy 1). A weapon has one
  barrel; a barrel accessory excludes another barrel accessory.
- A weapon may expose the **same mount kind** more than once only if it declares
  so (the default is one of each it lists); occupancy is per declared mount, not
  per mount *kind*.
- The mount vocabulary is content policy (§9); a pack may name a different set
  (a bow's mounts, a melee weapon's) without engine change.

**Acceptance criteria**

- [ ] A weapon declaring mount points is modifiable; one declaring none accepts
      no accessories and is unchanged from today.
- [ ] Each declared mount holds at most one accessory.
- [ ] Mount names come from a pack-declared vocabulary (the SR reference set:
      barrel / under-barrel / side / top / stock / internal).

## 3. The accessory

An **accessory** is an item (`item-modification §3` — a registry item, so it is
buyable / carriable / lootable / tradable) that declares the set of **mount points
it can occupy**. Most accessories fit exactly one kind (a Silencer → barrel; a
Bipod → under-barrel); some fit **several** (a Laser Sight → under-barrel *or*
top; a Guncam → almost any), and the accessory lists all it accepts. Attachment
chooses one free compatible mount (§4).

- An accessory whose compatible-mount set is empty is not weapon-mountable (it is
  the wrong domain — e.g. an armor mod, gated by `item-modification`'s
  host-compatibility key).
- A **multi-mount** accessory — one the source marks as occupying *both* of a
  paired mount (the reference set's `both`) or spanning two mounts — declares the
  **set of mounts it consumes on attach**, and attachment requires **all** of them
  free (§4). v1 Core has few of these; the field exists so they are expressible.
- The accessory's **effect** (its modifiers / properties / flags) is authored
  exactly as `item-modification §3` prescribes and aggregates via `§6`.

**Acceptance criteria**

- [ ] An accessory declares the mount point(s) it can occupy; a single-mount
      accessory fits one kind, a multi-fit accessory lists several alternatives.
- [ ] An accessory with no compatible weapon mounts is not weapon-mountable.
- [ ] A multi-mount accessory declares every mount it consumes; all must be free
      to attach.

## 4. Attaching (the admission rule)

Attaching moves an accessory from the wielder's inventory **onto a weapon** they
hold, subject to the **mount-slot admission test** (this is the only place the
rule differs from the capacity model):

1. **Compatibility** — the accessory's compatible-mount set intersects the
   weapon's exposed mounts; otherwise refused with a clear reason (wrong weapon /
   no such mount).
2. **Occupancy** — there is a **free** mount in that intersection (for a
   multi-mount accessory, **all** the mounts it consumes are free); otherwise
   refused, naming the occupied mount and what holds it ("the barrel already
   mounts a suppressor").
3. On success the accessory is **consumed from inventory and recorded on the
   weapon instance** as occupying the chosen mount(s) (`item-modification §7`
   persistence).

Where an accessory can go on more than one free mount, the choice is
deterministic policy (e.g. the wielder names the mount, or the first free
compatible mount in the vocabulary order); the resolver picks unambiguously and
the cue states which mount was used.

Validation **fails fast** with a user-facing reason (`item-modification §4`), each
refusal distinct: incompatible weapon, mount occupied, no such accessory, no such
weapon.

**Acceptance criteria**

- [ ] Attaching an accessory whose mounts the weapon does not expose is refused
      with a reason.
- [ ] Attaching to an occupied mount is refused, naming the mount and its current
      occupant; a free compatible mount succeeds.
- [ ] A multi-mount accessory attaches only when all its mounts are free.
- [ ] A successful attach consumes the accessory from inventory and records it on
      the weapon instance at a named mount.

## 5. Detaching

Detaching reverses an attach (`item-modification §5`): the accessory returns to
the wielder's inventory as an item, its mount(s) free again, and — if the weapon
is currently wielded — its contribution to the wielder is reversed (§6). The
destroy-on-removal per-mod policy of `item-modification §5` applies identically
(v1 default: recovered).

**Acceptance criteria**

- [ ] Detaching returns the accessory to inventory and frees its mount(s).
- [ ] Detaching from a wielded weapon reverses that accessory's contribution.

## 6. Effect aggregation, persistence, presentation (shared substrate)

These are **not redefined here** — they are `item-modification §6 (aggregation),
§7 (persistence), §8 (presentation)` applied with the weapon as host:

- **Aggregation (§6).** While the weapon is **wielded**, its contribution is the
  union of the weapon's own modifiers and every attached accessory's, tagged by
  source so each reverses cleanly on detach or unwield. An accessory in a
  sheathed/unwielded weapon contributes nothing.
- **Persistence (§7).** Attached accessories (mount + each accessory's own
  grade/decoration) are durable weapon-instance state. If this slice ships
  **with** `item-modification`, one save-version bump + migration covers both
  admission rules; if it ships **after**, it carries **its own** additive bump
  (the migration chain is append-only either way). Do not assume a single shared
  bump unless the two co-ship.
- **Presentation (§8).** `look`/`examine <weapon>` lists its mounts, which are
  occupied and by what, and which are free. Attach/detach and refusals emit
  specific cues.

**Acceptance criteria**

- [ ] Wielding a modded weapon applies its own plus every attached accessory's
      modifiers; unwielding reverses them; a sheathed weapon's accessories are
      inert. *(via `item-modification §6`)*
- [ ] Attached accessories survive save/load carried or wielded, under this
      slice's save-version bump (shared with `item-modification` only if
      co-shipped). *(via `item-modification §7`)*
- [ ] Examining a weapon lists occupied/free mounts and their occupants. *(via
      `item-modification §8`)*

## 7. Interaction with existing systems

- **Item modification** (`item-modification`): this spec is its second admission
  rule; substrate (§4–8 there) is reused wholesale, **including its cross-system
  rules** (`item-modification §9`): a modded weapon is non-stackable, attach/
  detach follows the don/doff combat gate, and accessories travel with the weapon
  in trade.
- **Weapon identity** (`weapon-identity`): accessories modifying to-hit, damage,
  or crit ride the same seams the weapon's intrinsic stats do.
- **Ranged combat / ammo** (`ranged-combat`, `ammo-and-reloading`): a barrel or
  under-barrel accessory that changes range, recoil, or feeding contributes
  through those features' existing terms; an accessory whose seam is not built yet
  attaches inertly.
- **Cyberware / smartlink** (`item-modification` cyberware host): a Smartgun
  System's paired bonus depends on a smartlink on the wielder — the cross-feature
  pairing is a follow-on (§1 non-goal, §8 open).
- **Equipment** (`inventory-equipment-items`): the weapon is an ordinary wielded
  item; accessories never change which slot it occupies.

## 8. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Mount vocabulary | The named mount points weapons may expose (§2). | the SR reference set (barrel / under-barrel / side / top / stock / internal) |
| Per-weapon mounts | Which mounts a given weapon exposes (§2). | per-weapon content metadata; absent ⇒ unmodifiable |
| Per-accessory mounts | Which mount(s) an accessory can occupy / consumes (§3). | per-accessory content metadata |
| Multi-mount resolution | How a free mount is chosen when several fit (§4). | wielder-named, else first free in vocabulary order |
| Destroy-on-removal / installation gate | Inherited from `item-modification §10`. | recovered; direct command |

Numeric effect magnitudes live on the accessory items (shared substrate); this
spec's own surface is the mount vocabulary and per-item mount declarations.

## 9. Open questions

- **Smartgun ↔ smartlink pairing.** The Smartgun System (External/Internal) is
  Core and central to a street samurai, but its bonus needs a matching smartlink
  (cybereye/goggles). Deciding where the paired bonus is computed — and how it
  reads the wielder's cyberware — is the main follow-on; v1 attaches the smartgun
  and records its presence so the pairing can light up later without re-authoring.
- **Multi-mount / `both` occupancy.** The reference set's `both` (e.g. a mounted
  under-barrel weapon) and cross-mount accessories need the "consumes a set of
  mounts" shape (§3). Few Core accessories use it; confirm the data shape before
  a book that leans on it.
- **Internal-mount accessories vs. capacity.** Some `internal` accessories (an
  internal smartgun, guncam) resemble capacity consumers more than external
  mounts. Confirm they model cleanly as a single-occupancy `internal` mount here
  rather than needing the capacity rule on weapons.
- **Mount-kind duplication.** Whether any weapon exposes two of the same mount
  kind (§2) — the default is one of each; revisit if content needs otherwise.
- **Shared substrate inheritance.** This spec assumes `item-modification §6–8`
  is authored generically over the admission rule; if that generalization slips,
  the shared save migration (§6) and aggregation must be reconciled before both
  ship.

---

<!-- Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: greenfield, spec ahead of code (build pending). The SECOND admission rule of item-modification (named mount slots vs. capacity budget); reuses item-modification's substrate (§4–8 there) for effect/persistence/presentation — its own additive save-version bump, shared with item-modification only if co-shipped. Scope: Core-source weapons + accessories only; the smartgun↔smartlink pairing, multi-mount `both` occupancy, and the installation gate are deferred. -->

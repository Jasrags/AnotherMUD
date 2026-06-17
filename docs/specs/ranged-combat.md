# Ranged Combat (thrown · projectile · range bands)

EPIC sub-epic **S1** — increment **G** of the WoT Combat & Equipment Depth
program (`docs/themes/wot-mechanics-epic.md`,
`docs/proposals/combat-equipment-depth.md`). Governed by EPIC **Decision 0**
(translate onto the existing tick model; no d20 rewrite). *Shipped — Slice A
(same-room ranged: thrown/projectile + ammo + Strength rules + masterwork ammo)
and Slice B (the far→near→melee range bands + auto-close + advance/withdraw
kiting). Cross-room targeting (Model C, §9) remains deferred.* Layers on `combat`
(the round loop, engage/disengage),
`weapon-identity` (weapon categories/proficiency), `inventory-equipment-items`
(ammo as items), and `masterwork` (masterwork ammo, now in scope).

## 1. Overview

The engine is melee-only: a weapon is a damage expression resolved when attacker
and target **share a room** (`combat §4.1` disengages a combatant whose target
leaves the room). Ranged weapons need two things the melee model lacks — **ammo**
and **distance** — and this slice adds both **without leaving the same-room
model**.

The resolved design (the §7 fork) is **abstract per-engagement range bands**: a
fight has a **range** state (e.g. *far → near → melee*) that lives on the
**engagement between two combatants**, not on the room. Both combatants are still
in the same room (the `combat §4.1` invariant holds); the band abstracts the
distance *between them*. An archer opens at range and gets shots while a
melee opponent **closes** band by band; once at the melee band it is a knife
fight. This delivers the "out-ranges, then it's a knife fight" feel that the WoT
longbow wants, entirely inside `internal/combat`.

It ships in **two slices**:

- **Slice A — same-room ranged mechanics (small, shippable alone).** A weapon
  may be **ranged**; ranged attacks consume **ammo**; thrown vs projectile
  **Strength** rules apply; a flat range-related to-hit adjustment is allowed.
  No band state yet — ranged weapons resolve on the existing same-room path. This
  is a complete, useful slice (bows that need arrows, throwing knives) on its own.
- **Slice B — range bands (the distance model).** Adds the per-engagement band
  state, opening-at-range, closing over rounds, and the advance/withdraw
  (kiting) play. Layers on A.

**Goals.** Give ranged weapons ammo + Strength rules + the out-ranges-then-closes
feel, on one room, inside combat; ship A as its own slice and B on top.

**Non-goals.** **Cross-room / adjacent-room targeting** (shooting into the next
room) — that is a separate, larger theme (proposal Model C): it would invert the
same-room invariant and add line-of-sight, off-room targeting, and two-room
render. It is explicitly **not** built here; range bands stay within one room.
No initiative or action economy (Decision 0). A bespoke quiver slot (§3 uses
ordinary consumables).

## 2. Ranged weapons (the data) — Slice A

A weapon MAY declare itself **ranged** and carry ranged metadata, additive to the
weapon-identity attributes (`weapon-identity §2`); a weapon that declares none is
melee, exactly as today.

- A **ranged flag / class** — the weapon is *thrown* (the weapon itself is
  hurled — a knife, a spear) or *projectile* (the weapon launches separate
  ammunition — a bow, a crossbow, a sling).
- A **range increment** — the distance unit over which accuracy falls off (used
  by the to-hit adjustment, §5.3). Policy magnitude.
- For projectile weapons, the **ammunition kind** they consume (§3).

A ranged weapon is still a weapon for every other purpose: it has a category,
proficiency tier, damage type, crit profile (`weapon-identity`), and may be
masterwork (`masterwork`). Proficiency gating applies unchanged — a non-proficient
bow takes the non-proficient to-hit penalty.

**Acceptance criteria**

- [ ] A weapon may declare a ranged class (thrown or projectile), a range
      increment, and (for projectile) an ammunition kind; a weapon declaring none
      is melee and unchanged.
- [ ] A ranged weapon participates in weapon-identity (category/tier/type/crit)
      and proficiency gating exactly as a melee weapon does.

## 3. Ammunition — Slice A

- A **projectile** weapon consumes **ammunition** — ordinary consumable items
  (`inventory-equipment-items`), not a bespoke quiver slot. Each shot consumes one
  matching unit; with none available the attack fails with a clear reason
  ("out of arrows") rather than firing. The weapon and the ammo declare matching
  **ammunition kinds** (a bow needs arrows, a crossbow bolts, a sling bullets);
  mismatched ammo does not fire.
- A **thrown** weapon **is** its own ammunition: throwing it removes it from hand
  and it **lands in the room** (recoverable by picking it up), rather than
  decrementing a separate stack.
- **Masterwork ammunition** (deferred to here by `masterwork §1`): a graded ammo
  item adds its grade's to-hit bonus, **stacks** with a masterwork launcher, and
  is **destroyed on use** (it does not land recoverable even if a thrown-style
  rule would otherwise recover it).

**Acceptance criteria**

- [ ] A projectile weapon consumes one matching ammunition unit per shot; with no
      matching ammo the attack fails with a reason and fires nothing.
- [ ] A thrown weapon leaves the hand and lands in the room (recoverable), rather
      than consuming a separate stack.
- [ ] Masterwork ammo adds its to-hit bonus (stacking with a masterwork launcher)
      and is destroyed on use.

## 4. Strength rules — Slice A

Ranged damage scales with Strength differently from melee (`combat §4.5`), per
the source:

- **Thrown** weapons add the **full** Strength damage bonus (you put your body
  into the throw), exactly like a melee weapon.
- **Projectile** weapons add **no positive** Strength bonus by default (a
  bowstring does the work) — but a **negative** Strength modifier still applies
  (too weak to draw it cleanly). Content MAY declare a **Strength-rated**
  projectile weapon (a composite bow built to a draw) that adds a positive
  Strength bonus **capped** at its rating.

This rides the existing `damage_bonus` shape (`combat §4.5`): the Strength
contribution is computed per the rule above for the wielded weapon's ranged class
before it is added to the rolled dice.

**Acceptance criteria**

- [ ] A thrown weapon adds the full Strength damage bonus.
- [ ] A plain projectile weapon adds no positive Strength bonus but still applies
      a negative Strength modifier.
- [ ] A Strength-rated projectile weapon adds a positive Strength bonus capped at
      its rating.

## 5. Range bands — Slice B

A fight gains a **range** state. It is **per engagement** (a property of the
attacker↔target pairing), not of the room — both combatants remain in the same
room (`combat §4.1`).

### 5.1 The band

The band is drawn from an ordered, content-defined vocabulary from farthest to
closest, ending at a **melee** band (e.g. *far → near → melee*). The melee band
is the colocated state today's combat always assumes; the farther bands are the
new distance the ranged model adds.

### 5.2 Opening and closing

- **Opening band** depends on how the engagement starts. A fight begun by a
  **ranged** attack (a `shoot`/`throw` at a not-yet-engaged target) opens at a
  **far** band; a fight begun by a **melee** engage (`kill`) opens at the
  **melee** band, exactly as today.
- **Closing.** On each round, a combatant who wants to melee **advances** one band
  toward melee (§5.4). A purely-melee combatant therefore spends the opening
  rounds closing the distance while a ranged opponent shoots — the archer's
  opening volley. Once at the melee band, melee resolves normally.

### 5.3 Effectiveness by band

- A **melee** weapon can only strike at the **melee** band. At any farther band a
  melee combatant has no valid attack and must close first.
- A **ranged** weapon can strike at any band; accuracy falls off with distance via
  a range-increment to-hit adjustment (§2). Using a ranged weapon **at the melee
  band** is allowed but takes a configurable point-blank penalty (awkward up
  close) — so a switch to a melee sidearm is the natural play once closed.

### 5.4 Advance and withdraw (kiting)

- **Advance** closes one band toward melee.
- **Withdraw** opens one band away from melee — staying in the room, increasing
  distance. A ranged combatant who withdraws while a melee opponent advances is
  **kiting** (keeping the distance open to keep shooting).
- Withdraw is distinct from **flee** (`combat §5`): flee leaves the room entirely;
  withdraw only opens the band within the room. They compose — withdraw to open
  distance, flee to escape.

**Acceptance criteria**

- [ ] The band is engagement state (per attacker↔target pair), not room state;
      both combatants stay in the same room.
- [ ] A ranged-initiated fight opens at a far band; a melee-initiated fight opens
      at the melee band.
- [ ] A melee combatant closes one band per round and can only strike at the melee
      band; a ranged combatant can strike at any band with a distance to-hit
      falloff and a point-blank penalty at the melee band.
- [ ] Advance closes a band and withdraw opens one; withdraw is distinct from flee
      (in-room vs leaving the room) and the two compose.

## 6. Phasing: A first, then B

Slice **A** (§2–§4) is shippable on its own and adds concrete, useful weapons —
bows that need arrows, throwing knives, the Strength rules — on the existing
same-room path, with at most a flat range to-hit adjustment and no band state. It
is the recommended first deliverable.

Slice **B** (§5) layers the band state, opening-at-range, closing, and kiting on
top of A. Until B lands, ranged weapons function at the melee band's terms (A's
flat adjustment) — correct, just without the distance play.

**Acceptance criteria**

- [ ] Slice A functions with no band state (ranged weapons resolve same-room with
      ammo + Strength rules + a flat adjustment).
- [ ] Slice B adds bands without changing A's weapon/ammo/Strength behavior.

## 7. Interaction with existing systems

- **Combat** (`combat §4`–§5): the band is per-engagement state mutated in the
  round loop; the same-room engage/disengage invariant (`§4.1`) is **unchanged**
  (bands are within a room). Withdraw is a new in-room retreat distinct from flee.
- **Weapon identity** (`weapon-identity`): ranged weapons carry the same
  category/tier/type/crit/proficiency attributes; the non-proficient penalty
  applies to bows.
- **Masterwork** (`masterwork`): ranged weapons may be graded; **masterwork ammo**
  (deferred to here) adds a to-hit bonus, stacks with a masterwork launcher, and
  is destroyed on use (§3).
- **Inventory** (`inventory-equipment-items`): ammunition is ordinary consumable
  items; no bespoke quiver slot. Thrown weapons return to the room on use.
- **Size & wielding** (`size-and-wielding`): a thrown weapon's full Strength
  bonus composes with the size Strength rules; a two-handed bow follows the size
  footprint as any two-handed weapon.
- **Light & darkness** (`light-and-darkness`): because bands stay within one room,
  the cross-room line-of-sight concern does not arise here; within-room darkness
  to-hit penalties apply to ranged attacks as to melee.

## 8. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| Range-band vocabulary | The ordered far→melee band names (§5.1). | the WoT pack bands (e.g. far / near / melee) |
| Range-increment falloff | The to-hit penalty per range increment of distance (§5.3). | policy magnitude |
| Point-blank penalty | The to-hit penalty for a ranged weapon used at the melee band (§5.3). | a mild penalty |
| Close/advance cadence | How many bands a combatant may advance/withdraw per round (§5.2, §5.4). | one band per round |
| Strength rules | Full on thrown / none-positive on projectile / capped on Strength-rated (§4). | the source rules |
| Out-of-ammo behavior | What happens when a projectile weapon has no matching ammo (§3). | the attack fails with a reason |

All numeric magnitudes live here; the prose names behaviors, not values.

## 9. Decisions and open questions

**Decided (resolves the proposal §7 ranged pre-decisions):**

- **Range = abstract per-engagement bands, within one room.** Not a flat
  same-room to-hit only (A alone leaves the bow unable to out-range), and **not**
  cross-room targeting (Model C — a separate theme, deferred). Bands keep the
  same-room invariant while delivering the distance feel.
- **Build A first, then B.** A (same-room ranged mechanics) is a shippable slice;
  B (bands) layers on it.
- **Ammo = ordinary consumables**, not a bespoke quiver slot; thrown weapons land
  recoverable, projectiles consume matching ammo, masterwork ammo is destroyed on
  use.
- **Strength rules per source** — full on thrown, none-positive on plain
  projectile (negative still applies), capped on a Strength-rated bow.

**Still open (non-blocking):**

- **Cross-room targeting (Model C)** — sniping into adjacent rooms remains a
  future theme if ever wanted; it inverts the same-room invariant and adds
  line-of-sight and two-room render. Recorded, not scheduled.
- **Mixed-band parties / multiple opponents** — bands track per attacker↔target
  pair (the shipped model), but the round-loop auto-close/kite acts only against
  the PRIMARY target each round; how a combatant relates to several foes at
  different bands at once is a refinement, not yet built.
- **A bespoke quiver/ammo slot** — if ammo management via ordinary inventory
  proves clumsy, a dedicated slot is a later refinement.

**Shipped since (mob AI):** a bow-wielding mob is a real ranged attacker — its
equipped weapon's ranged class flows into combat.Stats, so it opens at far and
shoots with the band falloff/point-blank, and a **kiting AI** opens the distance
(probabilistically, so a closing foe still net-advances) instead of shooting
when a foe gets inside far. Players kite manually with the withdraw verb.

---

<!-- Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: SHIPPED — EPIC S1 increment G · Slice A (thrown/projectile + ammo + Strength + masterwork ammo) and Slice B (per-engagement far→near→melee bands within one room, auto-close, advance/withdraw kiting). Cross-room (Model C) deferred. -->

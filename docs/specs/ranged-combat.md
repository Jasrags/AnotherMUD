# Ranged Combat (thrown · projectile · range bands)

EPIC sub-epic **S1** — increment **G** of the WoT Combat & Equipment Depth
program (`docs/themes/wot-mechanics-epic.md`,
`docs/proposals/combat-equipment-depth.md`). Governed by EPIC **Decision 0**
(translate onto the existing tick model; no d20 rewrite). *Shipped — Slice A
(same-room ranged: thrown/projectile + ammo + Strength rules + masterwork ammo)
and Slice B (the far→near→melee range bands + auto-close + advance/withdraw
kiting). Cross-room targeting (Model C, §10) ships as an opportunistic
adjacent-room action — slice 1 (the `shoot` verb) + slice 2 (a shot mob paths to
its attacker and engages) are in; sustained cross-room combat stays deferred.*
Layers on `combat`
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
- **Holder-fed weapons** (a firearm's clip, an autocannon's belt) extend this
  loose-round model: rounds live in a removable **ammunition holder** that is
  loaded into the weapon, firing draws from the inserted holder, and reloading
  swaps holders (ejecting the spent one). The loose-round model here is the
  internally-fed / feed-the-holder base; see `ammo-and-reloading` for holders,
  the unified `reload` verb, ejection/decay, and grade-through-holder.

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
- **Vision magnification** (an attacker capability — magnifying optics, e.g. a
  cybereye grant) reduces the distance falloff by treating the target as a
  configurable number of bands **closer** (floored at the melee band, i.e. no
  falloff — never a bonus). It applies only to the **range** falloff, never the
  point-blank penalty (optics do not help up close). Sourced like the other
  vision modes — an attacker property from equipped gear or a racial trait.

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

### 5.5 Firing modes

A firearm may support several **firing modes** — single, burst, full-auto. A mode
trades **ammunition and accuracy for damage**:

- A weapon declares the modes it supports (content); **single is always
  available** regardless. Melee/thrown and single-shot firearms have no extra
  modes.
- The attacker selects the active mode (the `firemode` verb). The selection is
  **transient** — a tactical choice, not a persisted preference — and is
  **clamped** to the wielded weapon at resolve time (a mode the current weapon
  can't fire resolves to single).
- Each mode has a configurable effect (§8): the **rounds consumed** per attack, a
  **damage bonus** (more lead on target), and a **recoil to-hit penalty** (the
  climb of an uncompensated burst). Single is the identity (one round, no bonus,
  no penalty). The recoil penalty is what **recoil compensation** (a later slice)
  offsets.
- Only **projectiles** consult the mode. The per-attack round cost is spent
  through the same ammunition path as a single shot (§3); a burst that outruns the
  remaining ammo fires the rounds it has rather than a dry click.

**Acceptance criteria**

- [ ] A weapon fires single by default; `firemode` reports and sets the mode, and
      refuses a mode the wielded weapon does not support (single always allowed).
- [ ] Burst/full-auto apply a damage bonus and a recoil to-hit penalty to a
      projectile attack; single leaves the attack unchanged.
- [ ] Burst/full-auto consume the configured rounds per attack; melee/thrown and
      single-fire are unaffected.
- [ ] The selection is clamped to the wielded weapon (switching to a weapon that
      lacks the mode falls back to single) and is not persisted across sessions.

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
| Vision-magnification bands | How many bands closer a magnifying attacker treats the target for the range falloff (§5.3). | one band |
| Firing-mode effects | Per mode (single/burst/auto): rounds consumed, damage bonus, recoil to-hit penalty (§5.5). | single 1/0/0 · burst 3/+2/−2 · auto 6/+4/−4 |
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

- **Cross-room targeting (Model C)** — **now in progress** (§10). Resolved as an
  opportunistic, adjacent-room **action** (not a sustained cross-room
  engagement), so the same-room round-loop invariant is preserved. Slice 1 (the
  `shoot` verb) and slice 2 (a shot mob paths to its attacker and engages) have
  shipped. Remaining Model C work (sustained cross-room combat, multi-room LoS
  and pursuit) stays deferred.
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

## 10. Cross-room targeting (Model C) — slices 1 & 2 shipped

The §1 non-goal becomes scope here, but on the **least invasive reading of the
proposal's Model C**. Two forks were resolved before any code:

- **Engagement model — opportunistic action, NOT sustained engagement.** A
  cross-room shot is a discrete `shoot` action; it does **not** open a fight that
  persists across the room boundary. The same-room round-loop invariant (§1,
  `combat §4.1`: a combatant whose target left the room disengages) is left
  **intact** — Model C does not invert it. You snipe; to keep fighting you close
  the distance (or the target comes to you). This is the lower-risk reading: a
  ranged *verb*, not an engine-wide round-loop change.
- **Range depth — adjacent room only.** Line of sight reaches through **one** open
  exit. Multi-room line-of-sight (shooting N rooms down a corridor via room
  coordinates) is a later increment, not built.

**Line of sight = "what you could walk through."** A shot to a direction requires
that the exit exists and is **visible** to the shooter (an undiscovered hidden
exit reads exactly like a wall — `hidden-exits §4.1`), that its **door is open**
(a closed door blocks the shot the way it blocks a step), and that the target
room is **not pitch-black** to the shooter (the per-viewer `light` level gates
aiming, mirroring within-room darkness §5.3). Fine-grained per-observer
concealment of the target (a hidden/sneaking mob in the next room) is deferred to
a later refinement; v1 gates on exit visibility + door + darkness.

**Targeting.** `shoot <target> <direction>` (alias `fire`): the last token is the
direction, the rest is the target keyword, resolved against the **adjacent**
room — mobs by keyword, players by exact name (the same two channels and
mob-wins-ties rule as same-room targeting).

**Ammo, Strength, weapon profile.** Identical to the same-room projectile path:
the shot consumes one matching ammo unit (out of ammo ⇒ a *click* and no shot),
and the wielded bow's damage/crit/Strength rules ride its `combat.Stats`. (One
recorded slice-1 gap: a consumed unit's **masterwork to-hit bonus** is not yet
folded into this one-shot path, which reads the stable `Stats().HitMod` only.)

**Two-room render — no event-struct change.** The swing's events are stamped with
the **target's** room, so the third-person hit/miss/death announce lands where
the target is, and the second-person tells route by player id to each participant
regardless of room. The verb adds the directional flavor on each side: an
*outbound* line in the shooter's room (`looses a shot to the north`) and an
*inbound* line in the target's room (`a shot streaks in from the south`).

**Acceptance criteria (slice 1 — shipped).**

- [x] `shoot <target> <direction>` looses one projectile at a target in the
      adjacent room through an open, visible exit.
- [x] An absent, undiscovered-hidden, or closed-door exit refuses the shot; an
      undiscovered hidden exit is indistinguishable from no exit.
- [x] A target room that is black to the shooter refuses the shot.
- [x] A projectile weapon must be wielded; one matching ammo unit is spent;
      out-of-ammo refuses with a *click* and fires nothing.
- [x] The hit/miss/death narration appears in the **target's** room; the
      shooter's room sees only the outbound flavor; each participant gets their
      own second-person line.
- [x] No combat engagement persists across the boundary — the round loop is
      untouched.

**Slice 2 — retaliation pathing (shipped).** A **living mob** that is shot bears a
**grudge**: on the AI tick it **paths toward** the shooter (adjacent-only,
matching C1 — one step through the exit toward the shooter's room) and then
**engages**, forced via the existing aggro→engage wiring **regardless of its base
disposition** (being shot makes the fight personal, even for a neutral mob). The
grudge **preempts** the mob's normal behavior, so a stationary or behaviour-less
mob still comes after you. A closed door between them holds the grudge (the mob
retries until a timeout); an unreachable or vanished shooter drops it. So the
snipe **provokes a response** rather than dealing free, riskless damage.

**Acceptance criteria (slice 2 — shipped).**

- [x] A surviving shot mob paths one step into the shooter's room and engages on
      the AI tick; a mob already co-located engages without moving.
- [x] The engage is forced even for a non-hostile mob (the shot is the trigger).
- [x] Retaliation preempts the mob's normal behavior (stationary/wander/none).
- [x] A closed door keeps the grudge (retry until a timeout); an unreachable
      shooter (not adjacent — multi-room pursuit is deferred) drops it.
- [x] A **killed** mob and a **player** target bear no automatic grudge (a player
      chooses their own response).
- [x] A mob already in combat has its lingering grudge cleared (it won't
      re-pursue after that fight ends).

**Still deferred.** Sustained cross-room engagement (Model C full, the round-loop
inversion); **multi-room** retaliation pathing (slice 2 pursues only an adjacent
shooter); multi-room line-of-sight; per-observer target concealment across the
exit; thrown weapons across a boundary (the `throw` verb stays same-room).

---

<!-- Spec style: narrative + acceptance criteria · Detail level: behavior only · Status: SHIPPED — EPIC S1 increment G · Slice A (thrown/projectile + ammo + Strength + masterwork ammo) and Slice B (per-engagement far→near→melee bands within one room, auto-close, advance/withdraw kiting). Cross-room (Model C, §10) — slice 1 (opportunistic adjacent-room `shoot` verb) + slice 2 (a shot mob paths to its attacker and engages) shipped; sustained cross-room combat + multi-room LoS/pursuit deferred. -->

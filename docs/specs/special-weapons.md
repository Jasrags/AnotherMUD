# Special Weapons (reach · trip · disarm)

EPIC sub-epic **S1** — increment **J** of the WoT Combat & Equipment Depth
program (`docs/themes/wot-mechanics-epic.md`,
`docs/proposals/combat-equipment-depth.md`). Governed by EPIC **Decision 0**
(translate WoT onto the existing tick/chance model; no d20 rewrite). It layers
on the shipped **weapon-identity** (`weapon-identity.md`, the weapon-metadata
seam), **ranged-combat** (`ranged-combat.md`, the range-band system reach
extends), **conditions** (`conditions.md`, the `prone` condition trip applies),
and **saves** (`saves.md`, the resolved-check primitive). *Spec ahead of code —
build pending; the increment is sliced so each maneuver ships independently.*

## 1. Overview

`docs/proposals/combat-equipment-depth.md` calls increment **J** "bottomless" —
the source material lists a dozen special-weapon behaviors (reach, set-vs-charge,
trip, disarm, net/entangle, whip, swordbreaker, double weapons, …), each a
bespoke combat-pipeline switch added one at a time. This spec does **not** try to
build all of them. It picks a deliberate **starter set of three** — **reach**,
**trip**, and **disarm** — that gives the *existing* weapon roster tactical
identity beyond its damage dice, and establishes the **weapon `special:` tag**
seam every later J behavior will reuse.

The engine today: a weapon is its dice + identity metadata (category, tier, crit,
damage type, size, ranged class — `weapon-identity.md`, `ranged-combat.md`,
`size-and-wielding.md`). Two combat *maneuvers* already exist as generic,
weapon-agnostic abilities (`conditions.md` §6): **`trip`** (Reflex save → `prone`)
and **`bash`** (Fortitude save → `stunned`). Nothing about a weapon changes how
those maneuvers play, and there is **no disarm** at all.

This increment makes weapons **matter** to maneuvers:

- A **reach** weapon (a polearm) strikes at the **near** range band, not just
  melee — it lands blows on a closing foe a hand-weapon cannot yet reach.
- A **trip** weapon (a bill, a poleaxe) makes the existing trip maneuver
  **land harder** — the wielder trips more reliably than a bare hand.
- A **disarm** weapon (a swordbreaker, a boarspear) wields a **new** maneuver:
  knock the target's weapon out of its hands to the ground.

**Goals.** Honor the source's special-weapon flavor
(`docs/wot/equipment.md` — Pike/Bill/Poleaxe *reach*, Bill/Poleaxe *trip*,
Boarspear/Swordbreaker *disarm bonus*) in spirit; reuse the **range-band** gate
(reach), the **existing `trip` ability + save DC** (trip), and the **equipment
unequip + room-drop** path (disarm); add the **`special:` weapon tag** as the one
new piece of metadata, validated at load like every other weapon field. Keep
every weapon that declares no `special:` tags behaving exactly as today.

**Non-goals.** The rest of J — set-vs-charge, net/entangle, whip subdual+range,
swordbreaker weapon-breaking, double weapons, the "drop your weapon to dodge a
counter-trip" nuance — stay deferred, each its own later slice on this same seam.
No new range geometry; reach rides the bands `ranged-combat.md` already ships.

**Slices.** (1) the maneuver-tag + reach metadata substrate (load + validate +
accessor, recorded-only) — SHIPPED; (2) **reach** (the band-gate extension;
reach modeled as a numeric cross-ruleset stat per §3) — SHIPPED; (3) **trip**
weapon-awareness (the DC bonus, via a per-caster `SaveDCBonusFunc` on the
resolver) — SHIPPED; (4) **disarm** (the new maneuver — a save-gated `disarmed`
to-hit-penalty condition, the trip/bash sibling; physical-drop variant deferred)
— SHIPPED. The J starter set is complete; the bottomless tail (set-vs-charge,
net, whip, swordbreaker-breaking, …) stays deferred on the `special:` seam.

## 2. The metadata: maneuver tags + the numeric reach stat

Special-weapon data splits into two kinds:

- **Maneuver tags** — a `special:` set naming the *maneuvers* a weapon enables,
  from a fixed engine vocabulary. This slice's vocabulary is `trip`, `disarm`;
  later J slices extend it (`set`, `entangle`, …). Each carries an optional
  magnitude scalar — **`trip_bonus`** / **`disarm_bonus`** — so the tag says
  *whether* and the scalar says *how much* (the source rates a boarspear +2
  disarm, a swordbreaker +3).
- **`reach`** — a **numeric weapon stat**, NOT a maneuver tag. Reach is a rating
  (`0` = an ordinary close weapon, `1`, `2`, …) that sits alongside crit / size /
  range-increment, because it is a **cross-ruleset** property each ruleset reads
  differently (§3): WoT thresholds it (`reach > 0` → the near-band strike); a
  Shadowrun pack diffs it (net reach → a defense-roll modifier). Modeling reach
  as one integer lets a WoT polearm (`reach: 1`) and a Shadowrun staff (`reach:
  2`) share a field, with the *interpretation* living in each ruleset's combat
  layer — the engine's standing "one substrate, many rulesets" posture.

### Acceptance criteria

- A weapon template may carry `special: [trip, disarm]` (any subset) and a
  numeric `reach: N`; absent/zero means an ordinary weapon (every weapon today).
- Each `special:` tag is validated against the engine vocabulary at **pack load**
  — an unknown tag fails the pack by file name (mirrors `damage_types` /
  `ranged_class`), never silently ignored; tags are normalized + deduplicated.
- `reach`, `trip_bonus`, `disarm_bonus` are non-negative integers, validated at
  load; a bonus with no corresponding tag is an authoring error (load fails) so a
  typo cannot ship an inert magnitude. (`reach` needs no tag — it IS the stat.)
- A built weapon instance exposes its maneuver tags, bonuses, and reach rating to
  the combat path.
- Metadata is recorded-only until the consuming slice (reach/trip/disarm) wires
  it — a weapon-identity-style "data ahead of consumer" landing is permitted.

## 3. Reach

Reach is a **numeric weapon stat** (`Template.Reach`, §2) read **per ruleset**:

- **WoT (this increment):** a weapon with `reach > 0` engages one range band
  further out than an ordinary melee weapon. `ranged-combat.md` §5 models the
  distance between two combatants as bands `melee → near → far`; a melee weapon
  may swing only at the **melee** band and otherwise **closes one band** per round
  (the auto-close), while a projectile fires from range. A reach weapon's
  **effective striking band includes `near`**: it swings at both `melee` and
  `near`. WoT reads reach as a **threshold** (any positive rating grants the
  near-band strike); the magnitude beyond 1 is not yet consumed (a `reach: 2`
  WoT weapon plays as `reach: 1` until a future slice gives the bands more depth).
- **Shadowrun (a future pack):** reach is a **relative** modifier — the *net*
  reach (attacker reach − defender reach) adjusts the defense roll (the source's
  "±1 defense per point of net Reach"). No band system; the same integer, a
  different consumer. Out of scope here; recorded so the field's shape is right.

The rest of this section specifies the **WoT** near-band behavior.

Consequences, all falling out of the existing band loop:

- Against a foe closing from range (an archer, a charger), the reach wielder lands
  a round of blows at `near` **before** a hand-weapon foe — who must still close
  to melee — can answer. This is the polearm's opening advantage.
- A reach wielder does **not** auto-close from `near` to `melee` (it is already in
  range); it strikes instead. It still auto-closes from `far`.
- Reach changes *only* the band at which the weapon may swing — not its dice,
  crit, to-hit, or the round cadence.

### Acceptance criteria

- A `reach` weapon swings at the `near` band; a non-reach melee weapon at `near`
  closes a band instead (today's behavior, unchanged).
- A `reach` weapon at the `far` band still auto-closes (reach is one band, not
  unlimited) — it does not snipe across the whole engagement.
- Reach is inert at the `melee` band (a reach weapon fights a melee-band foe
  exactly as any weapon).
- A projectile weapon is unaffected (reach is a melee property; `ranged_class`
  governs projectiles).
- Removing/swapping to a non-reach weapon mid-fight reverts to melee-only on the
  next round (reach is read from the live wielded weapon, not latched).

## 4. Trip (weapon-aware)

The generic `trip` ability (`conditions.md` §6 — a Reflex save vs a fixed DC;
on a failure the target is knocked `prone`) stays available to **every**
combatant. A **trip** weapon makes it **land harder**: while the wielder holds a
trip weapon, the trip maneuver resolves at an **elevated save DC** (the target
must beat the base DC **plus** the weapon's `trip_bonus`). A bare hand or a
non-trip weapon trips at the base DC, exactly as today.

This is the minimal weapon-awareness that gives a bill/poleaxe its identity
without restricting the maneuver or adding a second code path: the maneuver is
unchanged; only its DC reads the wielder's weapon.

### Acceptance criteria

- A wielder holding a `trip` weapon resolves `trip` at `base DC + trip_bonus`;
  the prone outcome and the existing save axis (Reflex) are otherwise unchanged.
- A wielder with no trip weapon (bare hand, sword, …) trips at the base DC —
  the existing behavior, byte-for-byte.
- The weapon read is live (the wielded weapon at resolution time), so swapping to
  or from a trip weapon changes the next trip's DC.
- Inert outside content that authors a `trip` weapon — no weapon today carries the
  tag, so trip behaves exactly as it does pre-increment.

### Deferred

- The source's "a trip-weapon wielder may **drop the weapon** to avoid being
  tripped in return on a failed attempt" — a counter-trip nuance — is **not** in
  this slice (the engine has no counter-trip on a failed maneuver to begin with).

## 5. Disarm (new maneuver)

A new combat maneuver: **`disarm`** — the natural sibling of `trip` (→ prone) and
`bash` (→ stunned) in the `conditions.md` §6 save-gated family. The target rolls a
save (Reflex — agility to keep its grip); on a **failure** it is afflicted with a
**`disarmed` condition**: a to-hit penalty for a few rounds, the combatant
fumbling and off-balance without a settled weapon. On a made save the maneuver is
resisted (no condition). This translates the source's "the weapon is knocked
away, you fight at a disadvantage until you recover it" into the engine's
condition vocabulary — the same way trip translates "knocked down" into the
`prone` condition rather than simulating a physics fall.

A **disarm** weapon resolves the maneuver at an **elevated DC** (`base DC +
disarm_bonus`) — the boarspear's +2, the swordbreaker's +3 (wired via the
per-caster `SaveDCBonusFunc`, §4). A wielder with no disarm weapon may still
attempt a disarm at the base DC (a generic maneuver, like trip), so the verb is
universally available and the weapon is an amplifier.

Because the outcome is a **condition**, it applies uniformly to a **player or a
mob** target through the thread-safe effect manager — no weapon-item manipulation,
so player-disarms-mob (the common case) and mob-disarms-player both work.

### Acceptance criteria

- A successful `disarm` (target fails the save) afflicts the target with the
  `disarmed` condition (a to-hit penalty) for its duration; the target's swings
  land less often while disarmed.
- A failed maneuver (target makes the save) applies nothing — the maneuver is
  resisted, exactly like a resisted trip.
- A `disarm` weapon raises the DC by its `disarm_bonus`; a non-disarm weapon
  disarms at the base DC.
- The maneuver applies to both player and mob targets (it is an effect, not an
  item move).
- The maneuver is available to players via a `disarm <target>` verb (every ability
  auto-registers a verb) and grantable to a class (the core fighter, like `trip`),
  and authorable on mobs as a proficiency.

### Deferred — the physical-drop variant

The richer "the weapon physically flies to the room floor and is retrievable"
disarm is **deferred**. It needs an **unequip-to-room** path for players (the
current `Unequip` returns the item to inventory, not the floor) and, for mobs, a
**slot→item link plus thread-safe weapon mutation** the engine does not have today
(a mob's weapon is write-once dice with no retained item reference). The v1
condition translation delivers the *mechanical* disarm — the target fights worse
without its weapon — without those engine extensions; the physical drop is a later
refinement on top. Also deferred: **swordbreaker weapon-breaking** (destroying the
weapon) and **off-hand disarm without the two-weapon penalty**.

## 6. Configuration surface

| Setting | Meaning | Default |
|---|---|---|
| `ANOTHERMUD_DISARM_BASE_DC` | Base save DC a disarm must beat (before a disarm weapon's bonus). | (engine default, ~13 — matches `trip`/`bash`) |
| `ANOTHERMUD_DISARM_COST` | Resource cost of a disarm attempt (movement, like `trip`/`bash`). | (engine default) |
| `ANOTHERMUD_DISARM_PULSE_DELAY` | Cooldown pulses after a disarm attempt. | (engine default) |
| `reach` near-band striking | Whether reach grants the near-band swing. | on (the increment) |
| weapon `trip_bonus` default | DC bonus when a `trip` weapon omits an explicit value. | content / engine default |
| weapon `disarm_bonus` default | DC bonus when a `disarm` weapon omits an explicit value. | content / engine default |

The trip/bash maneuvers' own DC/cost knobs (`conditions.md` §6) are unchanged;
disarm reuses that ability shape, so its numeric surface mirrors theirs and most
values come from the ability YAML rather than env where the existing maneuvers do.

## 7. Open questions

- **Disarm save axis.** Reflex (keep your grip by agility) vs a Strength contest
  (raw grip strength) vs the attacker's to-hit. **Resolved: Reflex** (v1), for
  consistency with `trip`; revisit if a grip-strength feel is wanted.
- **Physical drop vs condition.** v1 ships the **condition** translation (a
  `disarmed` to-hit penalty), uniform across players and mobs. The physical
  weapon-drop (knocked to the floor, retrievable) is deferred — see §5 Deferred.
  Open: when built, should it *replace* the condition for player targets (you're
  physically unarmed, not penalized) or *stack* with it?
- **Reach vs two reach weapons.** When *both* combatants wield reach, do they
  simply both strike at `near` (the natural reading), or does reach-vs-reach
  collapse to a melee-like exchange? Lean: both strike at `near`, no special case.
- **Mob disarm AI cadence.** When mobs gain re-equip AI, does a disarmed mob
  prioritize retrieving its own weapon over closing/attacking? Out of scope here;
  recorded for the mob-AI slice.
- **Trip/disarm as weapon-gated vs universal.** This spec keeps both maneuvers
  universal (any wielder may attempt; the weapon amplifies). The stricter source
  reading gates trip/disarm to weapons that have the property. Lean universal —
  it preserves the shipped generic `trip` and avoids a "you can't even try" wall.

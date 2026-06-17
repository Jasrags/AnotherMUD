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

**Slices.** (1) the `special:` weapon-tag metadata substrate (load + validate +
accessor, recorded-only); (2) **reach** (the band-gate extension); (3) **trip**
weapon-awareness (the DC bonus); (4) **disarm** (the new maneuver). Each is its
own commit + review in the project rhythm.

## 2. The `special:` weapon tag

A weapon template MAY declare a set of **special tags** naming the behaviors it
unlocks, from a fixed engine vocabulary. This slice's vocabulary is
`reach`, `trip`, `disarm`; later J slices extend it (`set`, `entangle`, …).

A separate scalar, **`disarm_bonus`**, carries the *magnitude* of a disarm
weapon's advantage (the source rates them +2 boarspear / +3 swordbreaker), so the
tag says *whether* and the scalar says *how much*. `trip` likewise reads a
**`trip_bonus`** scalar (default applies when the tag is present without one).

### Acceptance criteria

- A weapon template may carry `special: [reach, trip, disarm]` (any subset);
  absent means an ordinary weapon (every weapon today).
- Each tag is validated against the engine vocabulary at **pack load** — an
  unknown tag fails the pack by file name (mirrors `damage_types` / `ranged_class`
  validation), never silently ignored.
- `disarm_bonus` / `trip_bonus` are non-negative integers, validated at load; a
  bonus with no corresponding tag is an authoring error (load fails) so a typo
  cannot ship an inert magnitude.
- A built weapon instance exposes its special tags + bonuses to the combat path.
- Tags are recorded-only until the consuming slice (reach/trip/disarm) wires
  them — a weapon-identity-style "data ahead of consumer" landing is permitted.

## 3. Reach

A **reach** weapon engages one range band further out than an ordinary melee
weapon. `ranged-combat.md` §5 models the distance between two combatants as bands
`melee → near → far`; a melee weapon may swing only at the **melee** band and
otherwise **closes one band** per round (the auto-close), while a projectile
fires from range. A reach weapon's **effective striking band includes `near`**:
it swings at both `melee` and `near`.

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

A new combat maneuver: **`disarm`** attempts to knock a target's **main wielded
weapon** out of its hands. It resolves as a save-gated maneuver in the same shape
as `trip`/`bash` (`conditions.md` §6): the target rolls a save (Reflex — agility
to keep its grip); on a **failure** its main weapon is **unequipped and dropped to
the room floor**, where it lies until someone `get`s it. The disarmed combatant
fights **unarmed** (the engine's unarmed default) until it re-`equip`s a weapon.

A **disarm** weapon resolves the maneuver at an **elevated DC** (`base DC +
disarm_bonus`) — the boarspear's +2, the swordbreaker's +3. A wielder with no
disarm weapon may still attempt a disarm at the base DC (a generic maneuver, like
trip), so the verb is universally available and the weapon is an amplifier.

Edge rules:

- A target with **no weapon wielded** (already unarmed, or fighting with natural
  weapons) cannot be disarmed — the maneuver reports "nothing to disarm" and
  spends nothing (or its cost, per the config — see Open Questions).
- A target wielding a **two-handed** weapon is disarmed normally (the whole weapon
  drops); off-hand-only edge cases follow "main wielded weapon" (the `wield`
  slot), leaving an off-hand weapon in place.
- The dropped weapon enters the room via the existing unequip → room-placement
  path (the same machinery `drop` and corpse-spill use); ownership/decay is
  whatever that path already does (it does not vanish).

### Acceptance criteria

- A successful `disarm` unequips the target's `wield`-slot weapon and places it in
  the current room; the target's combat profile reverts to unarmed next round.
- A failed save leaves the target armed (the maneuver is resisted).
- A `disarm` weapon raises the DC by its `disarm_bonus`; a non-disarm weapon
  disarms at the base DC.
- Disarming an unarmed target is a no-op with a clear message (no weapon drops).
- A **player** target may `get` + `equip` the dropped weapon to re-arm; a
  **mob** target fights unarmed after a disarm in v1 (mob re-equip AI is deferred
  — see Open Questions).
- The maneuver is available to players via a `disarm <target>` verb and to mobs
  as an authorable ability (a mob may carry `disarm` in its proficiencies, like
  `trip`/`bash` today).

### Deferred

- **Mob re-equip AI** (a disarmed mob picking its weapon back up) — v1 leaves the
  mob unarmed; the disarm is a real, lasting tempo swing. Revisit when mob
  pick-up/equip AI is wanted.
- **Swordbreaker weapon-breaking** (destroying the weapon instead of dropping it)
  and **off-hand disarm without the two-weapon penalty** — later J slices.

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

- **Disarm-an-unarmed-target cost.** Should a disarm whiffed against an
  already-unarmed target cost the attempt's resource/cooldown (it was a real
  action), or refund as a no-op (the player mis-targeted)? Lean: spend nothing
  and message, matching how an unresolvable cast is handled.
- **Disarm save axis.** Reflex (keep your grip by agility) vs a Strength contest
  (raw grip strength) vs the attacker's to-hit. Lean Reflex for consistency with
  `trip`; revisit if a grip-strength feel is wanted.
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

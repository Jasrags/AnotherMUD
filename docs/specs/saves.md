# Saving Throws (Fortitude · Reflex · Will)

EPIC sub-epic **S6** — the saves primitive of the WoT Mechanics program
(`docs/themes/wot-mechanics-epic.md`, row S6). Governed by EPIC **Decision 0**
(translate WoT onto the existing tick/chance model; no d20 rewrite) and resolves
the EPIC's S6 open decision in favor of **real resolved d20 checks** (§7).

## 1. Overview

The WoT RPG resists harmful effects with three **saving throws** — Fortitude,
Reflex, Will — each `1d20 + base-save bonus + ability modifier` against a
difficulty class (`docs/wot/combat.md`, *Saving Throws*). AnotherMUD's combat
resolver already rolls a visible `d20 + modifier ≥ target` to hit (`combat §4.4`)
and already uses the d20 ability-modifier convention `(score − 10) / 2`
(`progression`). Saves therefore translate in **without new resolution
machinery**: the same roll shape, against a DC instead of an AC.

This slice adds:

- **A — three derived save values.** Every creature has a Fortitude, Reflex, and
  Will save, each derived from a **governing ability** (Constitution / Dexterity /
  Wisdom) plus a **class-granted base-save bonus**.
- **B — the resolve-check primitive.** A pure `Resolve(saveType, dc) → outcome`
  that rolls `d20 + saveBonus ≥ DC`, with the natural-1 / natural-20 auto rules,
  emitting a save event so observers (combat log, GMCP, future weave/condition
  systems) can react.
- **C — the first consumer.** The WoT **massive-damage Fortitude save**: a single
  hit at or above a configured threshold forces a Fortitude save or the victim
  suffers the lethal consequence the engine already has. Saves are also shown on
  the `score` sheet.

**Goals.** Land the small cross-cutting primitive that S2 (weaves), S5
(conditions), and S7 (poison / fear / disease) all consume; make it observable
in-game today through one real combat consumer; reuse the existing d20 roll and
the `(score − 10) / 2` modifier convention rather than inventing a parallel
resolver.

**Non-goals (this slice).** Weaves, conditions, poison, disease, fear, traps,
and area effects — the systems that will be the *bulk* of save consumers — are
later sub-epics; this slice ships only the primitive plus the massive-damage
consumer. No save-vs-check distinction, no "evasion / improved evasion" damage
halving, no take-10/take-20 (tabletop procedure, Decision 0 / EPIC §3). Reflex
and Will have **no engine consumer yet** this slice — they are derived and
queryable (and shown on `score`), waiting on their first triggering system.

## 2. The three saves and their derivation (A)

Every creature (player and mob) exposes three save values:

| Save | Governing ability | Resists (WoT) |
|---|---|---|
| **Fortitude** | Constitution | poison, disease, paralysis, raw physical trauma |
| **Reflex** | Dexterity | area effects, traps, falling, dodgeable harm |
| **Will** | Wisdom | compulsion, illusion, fear, mental influence |

Each save value is:

```
save = base-save bonus (class-granted) + ability modifier (governing stat)
```

- **Ability modifier** is the existing `(score − 10) / 2` convention read off the
  governing canonical stat (`con` / `dex` / `wis`) on the creature's stat block.
  It moves automatically with buffs/equipment that modify those stats — saves are
  a *derived read*, never a stored number that can drift.
- **Base-save bonus** is **granted by class** (like weapon proficiency,
  weapon-identity §3 / character-model D2), not use-gained. A class declares each
  of its three saves as **strong** or **weak**; the bonus is derived from the
  character's level along a strong or weak progression (the WoT "good save" vs
  "poor save" tracks). Composed across a multiclass character by taking, per
  axis, the **best** contributing class (matching d20 multiclass save stacking's
  intent without porting the additive-fractional-BAB bookkeeping — see §7).
- **Mobs** without class content declare base saves directly on their template
  (an engine default of zero when absent), plus their stat-derived ability
  modifier. A mob that declares nothing still has working saves (modifier only).

Because both inputs are derived (ability modifier from the live stat block,
base-save bonus from the character's classes + level), **no new save field is
persisted and no save-version bump is required** — the values are recomputed on
demand, exactly as the weapon-proficiency set is (weapon-identity §7).

**Acceptance criteria**

- [ ] Every player and mob exposes a Fortitude, Reflex, and Will value.
- [ ] Each value = class-granted base-save bonus + the `(score − 10) / 2`
      modifier of its governing ability (`con` / `dex` / `wis`).
- [ ] A buff or item that raises the governing ability raises the matching save
      on the next read, with no separate save-modifier plumbing.
- [ ] A class declares each save axis as strong or weak; the base bonus follows
      the level-scaled strong/weak progression (§6).
- [ ] A multiclass character takes, per axis, the strongest contributing class's
      base bonus.
- [ ] A mob with no class uses template-declared base saves (default zero) plus
      its stat-derived modifier; a mob that declares none still saves on the
      modifier alone.
- [ ] Save values are **derived** — nothing new is written to the player or mob
      save, and the player-save version is unchanged.

## 3. The resolve-check primitive (B)

A single resolution function the engine calls wherever something is "saved
against":

```
Resolve(creature, saveType, dc) → { success, roll, total, natural1, natural20 }
```

- Rolls a fresh `d20`, computes `total = roll + creature.Save(saveType)`, and
  reports **success** when `total ≥ dc`.
- **Natural 1 always fails; natural 20 always succeeds**, regardless of bonus or
  DC — mirroring the resolver's existing natural-1-fumble / max-roll-auto rules
  (`combat §4.4`) so saves and to-hit share the same edge semantics.
- Returns the roll detail (not just the boolean) so callers can render the math
  and so a future "evasion"-style consumer can inspect the margin.

Every resolved save emits a **save event** carrying the creature, save type, DC,
roll, total, and outcome. This is the seam future systems hook (a weave that
should be resisted, a condition that grants a re-save each round, a GMCP
save-result feed); this slice emits it and the combat log consumes it. The event
is **informational, not cancellable** this slice — saves report what happened;
they do not themselves veto an action (the *consumer* decides what a failure
does).

The primitive is a **pure function over an injected roller** (the same `Roller`
seam combat uses), so it is deterministic under a seeded roller in tests and
carries no global state.

**Acceptance criteria**

- [ ] `Resolve` rolls `d20 + Save(type)` and succeeds when the total is at least
      the DC.
- [ ] A natural 1 fails even when bonus − DC would otherwise pass; a natural 20
      succeeds even when it would otherwise fail.
- [ ] The result exposes the roll, the total, and the natural-1 / natural-20
      flags, not only the success boolean.
- [ ] Each resolved save emits one save event with creature, type, DC, roll,
      total, and outcome.
- [ ] The check is deterministic under a seeded roller (no hidden randomness or
      global state).
- [ ] The save event is informational only this slice — it does not cancel or
      alter the triggering action by itself.

## 4. First consumer: the massive-damage Fortitude save (C)

Straight from `docs/wot/combat.md` (*Massive damage: any single hit dealing ≥ 50
HP forces a Fort DC 15 or die outright*), translated to the engine's existing
damage and death path:

- When a **single hit** applies damage at or above a configured **massive-damage
  threshold**, the victim immediately resolves a **Fortitude save** against a
  configured **massive-damage DC** (both in §6).
- **On a failed save**, the victim suffers the engine's existing lethal
  consequence — the same `VitalDepleted` → death path combat already drives
  (`combat §4.5`, `combat §10`); today that routes to the non-punishing
  heal-to-1-HP-and-teleport recovery (m7-5), so massive damage becomes the first
  *save-gated* death trigger without introducing a new death penalty.
- **On a success**, the hit's normal damage still applies — the save only
  prevents the *extra* lethal consequence, never the rolled damage.
- The save fires **after** the hit's normal damage is applied and only if that
  damage did not already deplete the victim (a hit that already killed needs no
  massive-damage save).
- Both combatants see a save-resolution line (the victim resisting or
  succumbing), so the primitive is visible in ordinary play.

This consumer is **content-gated by the threshold**: with the engine default
threshold set high enough that ordinary low-level swings never reach it, the rule
is inert until weapons/abilities hit hard enough — it switches on naturally as
the WoT damage curve grows, and a pack may lower the threshold to make it bite
sooner.

Saves are additionally surfaced on the **`score`** sheet (a small Fort / Reflex /
Will row), so a player can read their defenses even before a consumer triggers.

**Acceptance criteria**

- [ ] A single hit at or above the massive-damage threshold triggers a Fortitude
      save against the massive-damage DC.
- [ ] A failed massive-damage save applies the existing lethal consequence; a
      successful one does not (the normal rolled damage applies regardless).
- [ ] The massive-damage save fires only after normal damage is applied and only
      when that damage did not already kill the victim.
- [ ] A hit below the threshold triggers no save and behaves exactly as before
      this slice.
- [ ] Both combatants see the save resolution in the combat log.
- [ ] `score` shows the viewer's Fortitude, Reflex, and Will values.

## 5. Interaction with existing systems

- **Combat resolver** (`combat §4.4`–`§4.5`, `combat §10`): the to-hit roll,
  damage, and death path are unchanged; the massive-damage save is an additional
  post-damage step on the existing hit pipeline, reusing the same `Roller` and
  the existing `VitalDepleted` death seam. No new combat phase, no action economy.
- **Progression / classes** (`progression`, character-model D1/D2): a class
  grants per-axis strong/weak save progressions as a class feature; multiclass
  composes them (best per axis), exactly the pattern weapon-identity uses for
  weapon proficiency. Save values are a derived read off the stat block.
- **Stats** (`progression` StatBlock): saves read `con` / `dex` / `wis` through
  the existing `Effective` cache, so any modifier source (equipment, effects,
  training) that moves those stats moves the saves for free.
- **Effects / abilities** (`abilities-and-effects`): not consumed this slice, but
  the save event and `Resolve` primitive are the seam S5 (conditions) and S2
  (weaves) will call — a weave that "allows a Will save" calls `Resolve`; a
  condition that grants a recurring save re-calls it each tick.
- **Light & darkness / weapon identity**: orthogonal; saves add no to-hit
  contributor and consume none.

## 6. Configuration surface

| Setting | Meaning | Default (engine) |
|---|---|---|
| Massive-damage threshold | The single-hit damage at or above which a Fortitude save is forced (§4). | high enough that ordinary low-level hits never reach it (the WoT pack tunes it) |
| Massive-damage DC | The Fortitude DC for the massive-damage save (§4). | the WoT pack value (the source's fixed DC) |
| Strong-save progression | Level → base-save bonus for an axis a class declares **strong** (§2). | the WoT "good save" curve |
| Weak-save progression | Level → base-save bonus for an axis a class declares **weak** (§2). | the WoT "poor save" curve |
| Default mob base saves | Base Fort / Reflex / Will for a mob template that declares none (§2). | zero (modifier-only saves) |

All numeric magnitudes live here per spec convention; the prose names behaviors,
not values.

## 7. Decisions (resolved at slice start)

- **Resolution model — real resolved d20 checks** (the EPIC S6 open decision).
  The engine already surfaces a `d20` for combat hits and uses `(score − 10) / 2`
  modifiers, so a `d20 + saveBonus ≥ DC` save is the *idiomatic* shape; folding
  saves into a hidden probability would be less consistent, not more. Saves get
  the same natural-1 / natural-20 edges as the to-hit roll.
- **Base-save source — class-granted, derived (no save field).** Like weapon
  proficiency (weapon-identity §7): the base bonus is read from the character's
  classes + level at runtime; ability modifier is read from the live stat block.
  Nothing new persists; no version bump.
- **Multiclass composition — best-per-axis.** A multiclass character takes the
  strongest contributing class's base bonus per save axis. This keeps the
  "strong save from any good-save class" intent of d20 multiclass without porting
  the additive fractional bookkeeping (Decision 0: translate, don't port).
- **Save event — informational, not cancellable.** This slice's event reports a
  resolved save; it does not veto the triggering action. The *consumer* owns the
  consequence of a pass/fail. (A cancellable variant can be added if a future
  system needs to gate an action *on* the save rather than reacting to it.)
- **First consumer — massive-damage Fortitude only.** Reflex and Will are derived
  and shown but have no triggering system until S5/S7 land. Shipping one consumer
  keeps the slice small while making the primitive observable.

### Still open (non-blocking)

- **Save-DC authorship convention** for future consumers (weaves, conditions):
  whether DCs are flat per-effect or scale with a caster/level input. Resolve
  when S2/S5 author their first save-gated effect — not needed for the
  fixed-DC massive-damage consumer.
- **Evasion-style outcomes** (Reflex save halves instead of negates) — a Reflex
  consumer concern; deferred with the first area-effect system.
- **Whether mobs should get class-style strong/weak progressions** rather than
  flat template base saves, once mob "classes" exist (S9/S11).

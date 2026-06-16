# Combat — Feature Specification

**Status:** Draft · **Scope:** Engagement, per-round resolution, fleeing,
and death handling for entities fighting in a shared room ·
**Audience:** Anyone reimplementing or porting this feature in any
language.

This document describes *what* combat must do, not *how* to implement
it. Specific dice expressions, modifiers, scaling formulas, cadence
counts, and cooldown lengths are policy and live outside this spec.

---

## 1. Overview

Combat is the system that lets entities (players and mobs) damage each
other over time once an attacker engages a target. It runs on a fixed
cadence within the engine's tick loop, resolves each combatant's
ongoing actions, applies time-based status effects, and lets entities
flee or die.

It does **not** decide when fights start (other systems — player
commands, mob aggression, scripted hooks — call into combat to engage
or disengage) and it does **not** decide what happens after a death
(loot, corpses, experience, quest credit, respawn — all of those live
in other features that subscribe to combat events).

### Core concepts

- **Combatant** — an entity with at least one opponent in its
  combat list.
- **Combat list** — the per-entity ordered list of opponents
  currently fighting that entity. Order is meaningful: the head of
  the list is the *primary target*.
- **Engagement** — adding two entities to each other's combat lists.
- **Disengagement** — removing two entities from each other's combat
  lists. Symmetric.
- **Combat round** — a periodic pulse during which each combatant's
  auto-attacks, queued abilities, and status effects resolve.
- **Auto-attack** — the default per-round attack a combatant makes
  against its primary target with no explicit input.
- **Wimpy threshold** — a per-entity HP percentage below which the
  entity attempts to flee at the end of a round.
- **Flee cooldown** — a per-entity window after a successful flee
  during which the entity cannot re-engage.

### Goals

1. Provide an engage/disengage API that other features use to start
   and stop fights.
2. Resolve each combatant's round on a fixed cadence: pending
   abilities first, then auto-attacks, then time-based effects, then
   flee checks.
3. Make hit/miss/damage outcomes deterministic given the random
   stream and current entity state, so combat is testable.
4. Emit observable events on engagement, hits, misses, evades,
   kills, flees, and disengagement so other features (UI, quests,
   alignment, persistence, AI) can react.
5. Refuse combat in protected contexts (safe rooms, "no-kill"
   targets, flee-cooldown attackers).
6. Hand off death to a cancellable check so other features can
   intercept (e.g. resurrection skills) before disengagement runs.

### Non-goals

- Loot drops, corpses, experience awards, quest credit, alignment
  shifts beyond a generic kill notification — handled by features
  that subscribe to combat events.
- Initial target acquisition (e.g. mob aggression, player `kill`
  command parsing) — callers resolve targets and then call engage.
- Respawn and re-tracking of dead entities — owned by the world and
  spawn features.
- Persistent durability or weapon decay.
- PvP policy beyond the safe-room and no-kill tag checks.

---

## 2. Engage / disengage

### 2.1 Engage

A caller asks combat to engage two entities (an attacker and a
target). The system MUST refuse the engagement when any of the
following holds:

- The target carries a "no-kill" marker.
- The attacker is currently in a room marked "safe".
- The attacker is currently under a flee cooldown (§5.3).
- The attacker is already engaged with the target (the call is a
  no-op — not an error, but not a fresh engagement either).

If none of these refusals apply:

1. The target is appended to the attacker's combat list (if not
   already present).
2. The attacker is appended to the target's combat list (if not
   already present).
3. An "engagement" event is emitted carrying both entities' ids and
   names and the room id.

Engagement is symmetric: a successful engage MUST leave both
entities in each other's combat lists.

### 2.2 Disengage (pair)

A pairwise disengage removes each entity from the other's combat
list. After removal, for each side whose combat list became empty,
the system MUST emit a "combat ended" event for that entity.

### 2.3 Disengage (all)

An entity may be removed from combat entirely — e.g. on death or
flee. The system MUST:

1. Snapshot the entity's current opponent list.
2. For each opponent, remove the entity from the opponent's list and
   emit "combat ended" for the opponent if their list becomes empty.
3. Remove the entity's own combat list entry entirely.
4. Emit "combat ended" for the entity itself.

### 2.4 Primary target

The head of an entity's combat list is its primary target. The
system MUST expose:

- A query for "primary target of entity X" returning either an
  entity id or none.
- An operation to **promote** an existing opponent to primary
  target. The opponent MUST already be in the entity's list (no
  silent insertion); if so, it is moved to the head. Used by taunt,
  rescue, threat, etc. Not symmetric: promoting on one side does not
  change the other side's ordering.

### 2.5 Combat-state queries

The system MUST expose:

- "Is entity X in combat?" — true iff their combat list is
  non-empty.
- "Combat list of entity X" — a snapshot copy (not a live
  reference), so callers may iterate without affecting state.
- "All current combatants" — iteration over every entity currently
  in combat. Order is unspecified to callers but MUST be stable
  during a single tick.

**Acceptance criteria**

- [ ] Engage is refused for safe-room attackers, no-kill targets,
      flee-cooldown attackers, and already-engaged pairs.
- [ ] Engage is symmetric and idempotent.
- [ ] Pairwise disengage emits "combat ended" only when a side's
      list becomes empty.
- [ ] Disengage-all cleans both directions and emits exactly the
      right "combat ended" events.
- [ ] Primary-target promotion fails when the opponent is not
      already in the list.
- [ ] Combat-list snapshots returned to callers are immutable
      relative to internal state.

---

## 3. The combat round

Combat resolution does not run every tick. It runs on a fixed
**combat cadence** — a configured number of base ticks between
rounds. On each round, the system MUST execute three phases in this
fixed order:

1. **Ability resolution.** Each combatant's queued ability actions
   are advanced. At most one valid ability resolves per combatant
   per round; invalid entries fail with a fizzle event and are
   discarded. Detailed semantics of the ability queue and validation
   are out of scope here.
2. **Auto-attack resolution.** §4.
3. **Status-effect resolution.** Time-based effects (damage-over-
   time, heal-over-time, ticking debuffs) advance one step. Effect
   expiration MAY drop the effect from the entity but MUST NOT
   disengage combat by itself.
4. **Wimpy check.** §5.

Phases MUST execute in deterministic priority order. Adding a new
phase MUST NOT silently reorder existing phases.

Within a single round the system MUST tolerate combatants being
removed mid-iteration (deaths during ability resolution must not
crash auto-attack resolution). Iteration MUST be performed over a
snapshot of the combatant set, with per-step liveness re-checked.

**Acceptance criteria**

- [ ] Combat resolution runs on a fixed cadence, not every tick.
- [ ] The three core phases (ability, auto-attack, status effects,
      wimpy) run in a stable, prioritized order.
- [ ] Iteration over combatants is safe against mid-round removal.
- [ ] A single combatant resolves at most one queued ability per
      round.

---

## 4. Auto-attack resolution

For each combatant with a primary target, the system runs one or
more **swings** against that target.

### 4.1 Pre-flight per combatant

Before any swing:

- If the combatant has no primary target, skip them.
- If the primary target is missing, dead (HP ≤ 0), or in a different
  room from the attacker, pairwise-disengage that opponent and skip
  them. This is the mechanism that ends a fight after a kill or a
  successful flee.

Player combatants SHOULD be ordered before mob combatants so that
ties (e.g. mutual lethal swings on the same round) resolve in the
player's favor. The exact policy is configurable but the default
prefers players.

### 4.2 Swing count

Each combatant takes at least one swing per round. Passive abilities
MAY grant additional swings; the swing count for a round is
`1 + extra-attack count`. Extra-attack determination is a passive-
abilities concern and out of scope here, but the auto-attack phase
MUST consult it once per combatant per round.

### 4.3 Per-swing sequence

For each swing:

1. **Live check.** If the target is already at HP ≤ 0, stop further
   swings for this attacker this round.
2. **Defensive passive check.** Passive abilities on the *defender*
   may pre-empt the swing with an evade. If so, emit an "evade"
   event identifying defender, attacker, and the evading ability,
   and skip to the next swing.
3. **Hit roll.** §4.4.
4. **On hit:** roll damage (§4.5), subtract from target HP, emit a
   "hit" event carrying damage amount, damage type, weapon's combat
   name (or a default unarmed name), attacker/target names, and a
   critical flag. If HP reached zero, emit a vital-depleted event
   for the target with vital="hp" and the attacker id; then stop
   further swings.
5. **On miss:** emit a "miss" event carrying weapon combat name,
   attacker/target names, and a fumble flag.

### 4.4 Hit roll

The hit roll uses a single d20 with attacker- and defender-derived
modifiers:

- A natural 1 is a **fumble** — automatic miss regardless of
  modifiers.
- A natural 20 is a **critical** — automatic hit regardless of
  modifiers.
- Otherwise the total is `d20 + attacker hit modifier` compared
  against the defender's armor class for the weapon's damage type.

The attacker hit modifier and defender armor class are derived from
ability scores, equipment, and per-damage-type AC tables. The exact
derivation is policy; the spec requires only that the same inputs
produce the same outputs and that critical/fumble override the
modifier comparison.

The derivation is supplied by a content-defined **channel map** rather
than hardcoded: a ruleset declares a named **derived channel** (`attack`,
`defense`, `damage_bonus`, `mitigation`, …) as a formula over attribute
names, and the resolver reads the channel by name, never the attributes
directly. This is what lets different rulesets feed the same resolution
kernel without engine changes. A pack that declares no formula for a
channel inherits the engine baseline (which reproduces the pre-channel
behavior); a later pack overrides a single channel's formula without
touching the others. The channel name set is the contract; the formulas
behind each name are policy.

### 4.5 Damage roll

Damage uses a dice expression (notation: `<count>d<sides>[+/-mod]`)
from the wielded weapon or a default unarmed expression when no
weapon is wielded. The rolled value is scaled by an attacker stat
(strength) modifier. Final damage MUST be at least 1 on a hit.

A critical hit's effect on damage (e.g. doubled dice, fixed bonus,
or "no extra effect, just guaranteed hit") is a policy decision and
is signaled to listeners via the `isCritical` flag on the hit event.

The applied damage is then adjusted by the channel layer (§4.4): the
attacker's `damage_bonus` is added (it is *not* multiplied by a
critical) and the defender's `mitigation` (soak) is subtracted, with the
min-1-on-hit floor applied last. The strength scaling above is the
baseline `damage_bonus`; `mitigation` is a **single** scalar today
(baseline maps it to zero). The **armor-depth** slice (`armor-depth.md`
§4, *spec; build pending*) makes `mitigation` **per damage type** — the
incoming damage's type selects the resistance — without otherwise
changing this step.

**Acceptance criteria**

- [ ] An attacker with no primary target or a dead/missing/distant
      target is disengaged from that opponent at round start.
- [ ] Swing count is `1 + extra-attack count`, evaluated per round.
- [ ] Defensive passives can pre-empt a swing and emit an evade.
- [ ] Natural 1 and natural 20 override the modifier comparison.
- [ ] Hits emit a hit event with damage, type, weapon name, names,
      and critical flag; misses emit a miss event with fumble flag.
- [ ] Reaching HP ≤ 0 emits a vital-depleted event with the
      attacker id and stops further swings from that attacker.
- [ ] Final damage on a hit is at least 1.

---

## 5. Flee and wimpy

### 5.1 Wimpy check

After auto-attacks and status effects resolve, the system iterates
over surviving combatants. If an entity carries a wimpy-threshold
property and its current HP as a percentage of max HP is at or
below that threshold, the system attempts to flee on that entity's
behalf (§5.2).

The wimpy check MUST skip entities at HP ≤ 0 (handled by death
flow, not flee).

### 5.2 Flee attempt

A flee attempt against an entity in a room with available exits:

1. If the entity carries a "no-flee" marker, the attempt fails
   immediately and a "flee prevented" event is emitted.
2. If the entity is not in a known room or the room has no
   available exits, the attempt fails and a "flee failed" event is
   emitted.
3. Otherwise the system selects an available exit uniformly at
   random, disengages the entity from all combat (§2.3), moves the
   entity through the selected exit, starts a flee cooldown on the
   entity (§5.3), and emits a "flee" event carrying entity name,
   the from-room id, the to-room id, and the direction.

The move itself goes through the world's movement subsystem and is
subject to its normal checks (doors, traversal cost, etc.). If the
move fails, the spec does not define the outcome and the world's
movement rules apply.

### 5.3 Flee cooldown

After a successful flee, the entity MUST NOT be allowed to engage a
new target until a configured cooldown has expired. The cooldown is
measured in ticks against the engine's tick counter. The cooldown
applies to engagement only — it does not interfere with being
attacked.

Callers that initiate attacks SHOULD check flee cooldown before
calling engage and SHOULD report it to the user (when the caller is
a player) with a clear message rather than a silent failure.

**Acceptance criteria**

- [ ] Wimpy fires only for entities with the threshold property and
      a non-zero threshold.
- [ ] Wimpy compares current HP percentage against the threshold,
      not absolute HP.
- [ ] "no-flee" entities never flee and emit the prevented event.
- [ ] No-exit and unknown-room conditions emit the failed event.
- [ ] On a successful flee the entity is disengaged from every
      opponent, moved, cooldown is set, and the flee event carries
      the from-room and to-room.
- [ ] A flee cooldown blocks subsequent engagement but does not
      block being engaged.

---

## 6. Death

When an entity's HP reaches zero (typically from a hit during
auto-attack resolution, but possibly from an ability or a damage-
over-time effect), a vital-depleted event with vital="hp" is
emitted. The combat feature itself does not consume that event —
another feature does — and then calls back into combat to handle
the death.

### 6.1 Cancellable death check

Before combat performs death-related disengagement, the system MUST
publish a **cancellable** death check event carrying the victim id,
the resolved killer id (if known), and the room id. Listeners (e.g.
a resurrection skill, a phylactery item, a soulbound effect) MAY
flip a cancel flag on the event.

If any listener cancels the check, combat MUST NOT proceed with the
disengagement-for-death path. It is the canceller's responsibility
to restore the victim to a non-dead state (e.g. heal to 1 HP).

### 6.2 Killer attribution

When death is not cancelled, combat resolves the killer in the
following order:

1. An explicit attacker id supplied with the vital-depleted event
   (used for ability one-shots, where the queued ability knows the
   actor even if combat had not yet engaged).
2. The victim's current primary target (auto-attack kills).

The attribution MAY be empty (e.g. environmental damage). Listeners
MUST tolerate the absence of a killer.

### 6.3 Kill emission

When death is not cancelled, combat:

1. Emits a kill event with the killer id, victim id, room id, and
   names. Other features (alignment, quest progress, achievements)
   subscribe here.
2. If the victim is a mob, emits a separate mob-killed event with
   the mob template id, mob name, killer id, killer name, and room
   id. This is the canonical signal for loot, XP, and quest credit
   keyed on mob templates.
3. Calls disengage-all on the victim, which emits "combat ended"
   for the victim and for any opponents who become idle.

### 6.4 Players vs mobs

Combat does not distinguish player and mob deaths beyond emitting
the mob-killed event for mobs. Player death recovery (corpse,
respawn, gear loss) is owned by another feature subscribing to the
vital-depleted or death events.

**Acceptance criteria**

- [ ] Death-related disengagement runs only after a cancellable
      death check that was not cancelled.
- [ ] An explicit attacker id on the vital-depleted event wins
      attribution over the victim's primary target.
- [ ] Attribution may be absent and downstream listeners tolerate
      that.
- [ ] Mob deaths emit a mob-killed event in addition to the generic
      kill event.
- [ ] Disengage-all on death cleans both directions and emits the
      correct "combat ended" events.

---

## 7. Observable events

The combat feature MUST publish events on the engine event bus for
every state transition that affects observers. The set, at minimum:

| Event | When |
|---|---|
| engagement | both sides added to each other's combat list (§2.1) |
| hit | a swing connected and damage was applied (§4.3) |
| miss | a swing did not connect (§4.3) |
| evade | a defensive passive pre-empted a swing (§4.3) |
| vital-depleted (hp) | target HP reached zero from any source (§4.3, §6) |
| death check (cancellable) | before death disengagement (§6.1) |
| kill | a death was not cancelled (§6.3) |
| mob killed | a mob died (§6.3) |
| flee | a successful flee (§5.2) |
| flee prevented | a no-flee entity attempted to flee (§5.2) |
| flee failed | no available exits (§5.2) |
| combat ended | an entity's combat list became empty (§2.2, §2.3) |
| ability fizzled | a queued ability failed validation (§3) |

Each event MUST carry enough data for listeners to act without
querying further state in the common case: entity ids, names where
relevant, room id, weapon combat name on hit/miss, damage and type
on hit, direction and rooms on flee. Names are included as a
convenience; the canonical identity is the id.

The combat feature itself MUST NOT depend on its own events; it
mutates state directly and emits events as a side channel.

**Acceptance criteria**

- [ ] Every state transition in §2-§6 emits exactly the right event
      with the documented payload.
- [ ] Combat correctness does not depend on event delivery (events
      may be sampled, dropped by observers, etc., without changing
      gameplay).
- [ ] The death check is the only combat event listeners can
      cancel.

---

## 8. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| Combat cadence (ticks per round) | §3 |
| Phase set and priority | §3 |
| Player-vs-mob iteration order on ties | §4.1 |
| Default unarmed damage dice and damage type | §4.5 |
| Critical / fumble effect on damage | §4.5 |
| Hit-modifier and AC derivation formulas (via the channel map) | §4.4 |
| Derived-channel names and their per-pack formulas | §4.4 |
| Damage scaling formula (e.g. stat-based multiplier) | §4.5 |
| Flee cooldown length | §5.3 |
| Wimpy threshold property name | §5.1 |
| Safe-room tag name and no-kill / no-flee tag names | §2.1, §5.2 |

---

## 9. Open questions / future work

- **Hardcoded flee cooldown.** The current cooldown length is a
  constant in the combat manager. Treat it as configuration.
- **Player-first ordering.** Auto-attack iteration prefers players
  to mobs; whether this is desirable for PvP (player vs player)
  rounds where both sides should be symmetric is unclear.
- **Damage type vs AC matrix.** Armor class is per damage type, but
  the policy for missing entries (treat as 0? as base?) is
  implementation detail today.
- **Cancellable damage.** The death check is cancellable; an
  earlier "incoming damage" hook (so shields/wards can reduce or
  absorb before HP is touched) is not specified.
- **Threat / aggro modelling.** Primary target is just list-head;
  there is no numeric threat score, no taunt duration, no falloff.
- **Multi-target / area effects.** Auto-attacks are single-target.
  AOE damage is presumably resolved inside abilities, but the
  shared damage-application path is not specified here.
- **Combat-end cleanup.** Pulse-delay state is cleared on flee and
  combat-end via a subscriber, not directly inside combat.
  Whether that belongs *in* combat is a design question.

---

<!-- Generated: 2026-05-21 · Scope: CombatManager + Heartbeat phases (auto-attacks, status effects, wimpy) + CombatEventModule death flow · Spec style: narrative + acceptance criteria · Detail level: behavior only -->

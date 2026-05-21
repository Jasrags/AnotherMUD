# Abilities and Effects — Feature Specification

**Status:** Draft · **Scope:** Definition, learning, queueing, validation,
resolution of abilities; lifecycle of active effects produced by them ·
**Audience:** Anyone reimplementing or porting this feature in any
language.

This document describes *what* the abilities-and-effects substrate must
do, not *how* to implement it. Specific dice expressions, scaling
constants, default caps, chance percentages, and hook names are policy
and live outside this spec.

---

## 1. Overview

The abilities feature is the substrate for everything an entity can
*do* beyond plain movement and auto-attacks: skills (kick, rescue,
backstab) and spells (heal, fireball, bless). It pairs with the
**effects** feature, which holds the time-bounded consequences an
ability can leave behind (stat buffs, debuffs, status flags, damage-
over-time).

Abilities are content: they are declared by packs, registered by id,
and resolved by a fixed engine pipeline on a fixed cadence. Effects
are runtime instances of those declarations, attached to entities.

### Core concepts

- **Ability definition** — content-defined declaration of an ability:
  id, name, type, category, cost, cooldown, target rules, optional
  effect template, metadata, handler reference.
- **Ability type** — *active* (queueable, resolves on a pulse) or
  *passive* (always-on, triggered by hooks during other systems'
  resolution).
- **Ability category** — *skill* or *spell*. Drives which resource
  the ability draws from and the default offensive classification.
- **Proficiency** — a per-entity, per-ability integer (bounded by a
  per-ability cap) representing how well the entity performs the
  ability. Both the chance to succeed and the chance to gain
  proficiency depend on the current value.
- **Cap** — a per-entity, per-ability ceiling on proficiency. Lets
  content gate eventual mastery without restricting initial learning.
- **Action queue** — a per-entity list of queued ability invocations
  consumed by the ability resolution phase, at most one valid
  invocation per pulse.
- **Pulse delay** — a per-entity, per-ability cooldown measured in
  pulses, recorded only in memory (not persisted).
- **Effect template** — the immutable description on an ability of
  the effect it produces.
- **Active effect** — a live instance of an effect template attached
  to a target entity, with a remaining-pulse counter, stat
  modifiers, and flags.

### Goals

1. Provide a registry of ability definitions identifiable by stable
   ids, overridable by pack priority.
2. Track per-entity proficiency and caps; persist them as part of the
   entity.
3. Accept queued ability invocations from any source (player command,
   AI, scripted hook) and resolve them in a fixed, ordered pipeline.
4. Reject invalid invocations with a structured "fizzle" reason
   without partially mutating state.
5. Apply effects to targets, accumulate their stat and flag changes,
   tick them down, and remove them cleanly.
6. Emit observable events for every meaningful transition.
7. Support passive abilities as hook-driven probability rolls without
   queueing.

### Non-goals

- The command surface that lets a player invoke an ability (covered
  by the commands feature). Abilities only specify what an
  invocation must look like once it lands in the queue.
- Specific ability content: skills, spells, healing formulas, damage
  formulas, effect catalogs. All policy.
- The combat round itself (covered in `docs/specs/combat.md`); this
  spec defines the ability-resolution phase that runs *within* it.
- Persistence of active effects across save/load. Effects are
  ephemeral; stat modifiers they applied persist with the entity by
  separate mechanism.

---

## 2. Ability definitions

### 2.1 Registration

Abilities are registered into a single ability registry keyed by id.
When two registrations share the same id, **the higher-priority
registration wins**. Equal priorities resolve in registration order.
This lets a pack override a baseline ability without renaming.

### 2.2 Required and optional fields

A registration MUST carry:

- A stable id.
- A display name.
- A type (active or passive).
- A category (skill or spell).

A registration MAY carry:

- A resource cost.
- A pulse-delay cooldown in pulses.
- An "initiate-only" flag (cannot be used while already in combat —
  intended for combat-opening moves like ambush).
- A target-type list (e.g. `enemy`, `self`, `ally`).
- An effect template (id, duration in pulses, stat modifiers, flag
  set).
- An equipment-slot requirement (slot id, optional required tag on
  the equipped item).
- An alignment range restricting who may use it.
- A variance percentage controlling how proficiency translates into
  success chance.
- A max-chance ceiling on success.
- A proficiency-gain base chance, a failure-gain multiplier, and an
  optional gain-stat with scale (which player stat helps you learn
  faster).
- A short name and command name (used by the command surface to
  generate aliases).
- A handler reference (an opaque content-defined token the resolver
  forwards as event data so packs can attach side effects on hit).
- Arbitrary metadata used by passive hooks and by the offensive
  classifier (§4.6).

### 2.3 Identity vs content

Two abilities differ if their ids differ. Two registrations with the
same id are treated as a single ability with override semantics. An
ability's display name, costs, and metadata MAY change across
overrides; its semantic role (type, category) SHOULD NOT, since
proficiency entries on existing entities are keyed by id.

**Acceptance criteria**

- [ ] Registration is by stable id.
- [ ] Higher-priority registration wins on duplicate id; lower
      priority is silently superseded.
- [ ] Lookups (`get`, `has`, `get-by-type`, `get-by-category`) are
      stable and case-insensitive on id.
- [ ] An ability with no effect template can still be used; it just
      produces no active effect.

---

## 3. Proficiency

### 3.1 Per-entity storage

Each entity carries two id-keyed integer maps as properties: a
proficiency map and a cap map. Both persist with the entity. An
entity *has* an ability when its proficiency map contains an entry
for that id, regardless of value (the value is bounded below at 1).

### 3.2 Learning and forgetting

Learning an ability sets the proficiency for that id (clamped to the
1..100 range). If no cap exists for the ability on this entity, a
default cap MUST be established at learn time; the default cap is
configurable.

Forgetting an ability removes its proficiency entry; the cap entry
SHOULD be left as-is so a re-learn preserves cap progress (cap is
treated as character progression, not as a skill memory).

### 3.3 Querying

The system MUST expose:

- "Does entity X have ability Y?"
- "Proficiency of entity X in Y" (returns none if not known).
- "Cap of entity X in Y" (returns 100 if not known).
- "All learned abilities of entity X" as a snapshot list of
  `(id, proficiency)` pairs.

### 3.4 Bounds

Proficiency values MUST be clamped to `[1, min(cap, 100)]` on every
mutation, never written outside the range.

### 3.5 Gain on use

After each invocation (regardless of hit or miss), the system rolls
for a proficiency gain. The gain chance:

1. Starts from the ability's configured base chance.
2. Is multiplied by `1 - currentProficiency/100` (so gains slow as
   skill rises and stop at 100).
3. Is multiplied by a stat-based factor when the ability declares a
   gain-stat (so e.g. an int-tagged spell rewards high-int casters
   faster). Stat is read from the entity's current stat values.
4. Is multiplied by the configured failure multiplier when the
   invocation was a miss.

If the roll succeeds the proficiency is incremented by one, capped
by `min(cap, 100)`. No gain is granted when the entity is already at
its effective cap.

**Acceptance criteria**

- [ ] Proficiency and cap maps are persisted as entity properties.
- [ ] Learning establishes a default cap when none exists.
- [ ] Forgetting clears proficiency without clearing the cap.
- [ ] Gain probability collapses to zero at the effective cap.
- [ ] Miss gains use the per-ability failure multiplier.
- [ ] A gain roll never exceeds the effective cap.

---

## 4. Active ability resolution

Active abilities resolve in the **ability resolution phase**, which
runs at the top of each combat round (see `docs/specs/combat.md` §3).
The phase iterates every entity that holds a non-empty action queue
and processes that entity's queue with the rules below.

### 4.1 The action queue

The action queue is a per-entity, ordered list of queued invocations.
Each entry MUST carry at minimum:

- The ability id.
- Optionally, an explicit target entity id.

Entries MAY carry additional content-defined data the handler uses.

The queue is exposed as an entity property so any system (commands,
AI, scripted hooks) can enqueue actions consistently and so it is
naturally snapshotted by save/load. Empty queues SHOULD be elided
from the property bag.

### 4.2 Per-pulse processing

For each entity with a queue, the phase repeatedly inspects the
front entry until one valid execution occurs:

1. If the entry's ability id is missing or unknown, emit a fizzle
   with reason `unknown ability`, drop the entry, continue.
2. Otherwise run validation (§4.3). If validation fails, emit a
   fizzle carrying the reason as a lower-case keyword, drop the
   entry, continue.
3. Otherwise resolve the ability (§4.5), drop the entry, **stop**
   processing this entity's queue for this pulse.

That is: invalid entries are skipped without consuming the pulse,
but **at most one valid execution occurs per entity per pulse**. If
the queue ends up empty the property is cleared.

### 4.3 Validation

Validation MUST be performed in a stable order so that the first
reason found is the reason reported. The order is:

1. **Rest-state check.** Sleeping or resting entities cannot use
   abilities. Result: `asleep`.
2. **Alignment range check.** When the ability declares an
   alignment range, the entity's current alignment must fall inside
   it. Result: `alignment_restricted`.
3. **Proficiency check.** The entity must have an entry for the
   ability in its proficiency map. Result: `no_proficiency`.
4. **Equipment-slot check.** When the ability declares a required
   slot, the entity must have an item equipped in that slot. When a
   required tag is also declared, the equipped item must carry that
   tag. Result: `equipment_required`.
5. **Initiate-only check.** When the ability is initiate-only, the
   entity must not already be in combat. Result: `initiate_only`.
6. **Target validity check (offensive abilities only).** A target
   must be resolvable (§4.4) and the entity must currently be in
   combat. Result: `invalid_target` or `not_in_combat`.
7. **Effect-present check.** When the ability produces an effect,
   the source entity must not already carry an active effect with
   that effect id. (Models "you cannot stack bless on yourself" in
   the source-cast case.) Result: `effect_present`.
8. **Pulse-delay check.** When the ability has a per-entity cooldown
   and the entity has an unexpired delay recorded for this ability,
   reject. Result: `pulse_delay`.
9. **Resource check.** Compute the resource cost adjusted by the
   entity's race modifier (if any) and verify the entity has at
   least that much of the appropriate resource (§4.7). Result:
   `insufficient_resources`.

If all checks pass the validation result is `ok` and resolution
begins.

### 4.4 Target resolution

Resolution determines a target entity:

1. If the queue entry carries an explicit `targetEntityId`, look it
   up. If the lookup succeeds, that entity is the target. If the id
   was supplied but cannot be resolved, the resolution yields no
   target (offensive abilities then fail validation as
   `invalid_target`).
2. Otherwise, for offensive abilities, fall back to the entity's
   current primary combat target.
3. Otherwise (self-targeted or buff abilities), the target is the
   source entity itself.

Target resolution MUST happen during validation for the offensive
target check and again during resolution. The two calls MUST return
the same target unless world state changed mid-pulse.

### 4.5 Resolution

Resolution proceeds in this order:

1. **Deduct resource.** Subtract the race-adjusted cost from the
   appropriate resource (§4.7). Skipped when cost is zero.
2. **Record last-used.** Set the entity's "last ability used"
   property to the ability id.
3. **Record pulse delay.** If the ability declares a pulse delay,
   record the next-ready pulse as `currentPulse + delay + 1`.
4. **Roll hit/miss.** When the ability declares variance zero, the
   invocation always hits. Otherwise the hit chance is
   `proficiency × (variance/100) + luck × luckScale`, rolled
   against a uniform 1..100. The roll is hit when ≤ chance.
5. **Resolve target** (§4.4).
6. **On miss:** emit a "missed" event carrying ability id, ability
   name, target name (if any), and the source entity. Roll a
   proficiency gain with the failure multiplier. Stop.
7. **On hit, with effect:** build an active effect from the
   template (§5.1) and apply it to the target.
8. **On hit, always:** emit an "ability used" event carrying
   ability id, ability name, category, target name (if any), and
   the handler token. Roll a proficiency gain with the success
   multiplier.
9. **Post-hit death check.** If the target's HP reached zero as a
   result of the ability's side effects, emit a vital-depleted
   event with `vital = hp` and `killerId = source entity id`. The
   ability resolution phase does *not* itself read damage from the
   ability definition; damage application is the handler's
   responsibility, and the phase only emits the death signal so
   combat can run the cancellable death check (`docs/specs/
   combat.md` §6.1).

### 4.6 Offensive classification

Several validation and resolution decisions depend on whether an
ability is "offensive". An ability is offensive when:

- Its category is **skill**; OR
- Its category is **spell** AND it has no effect template AND its
  metadata declares damage dice. Spells with effect templates or
  with heal-dice metadata are NOT offensive even if they target an
  enemy.

This rule is engine-defined so packs do not have to mark every
ability explicitly. Packs can still influence the outcome by
choosing category and metadata.

### 4.7 Resource selection

- **Skills** draw from the entity's movement pool.
- **Spells** draw from the entity's primary resource pool (mana).
- The cost is modified by a race-defined multiplier before both the
  validation comparison and the deduction. Validation and deduction
  MUST use the same adjusted cost.

### 4.8 Fizzle reasons

Fizzles MUST emit the validation result as the `reason` field, in
lower-case keyword form. The set of reasons is fixed by §4.3 plus
`unknown_ability`. Adding a new reason is a content/engine
coordination event; clients SHOULD treat unknown reasons as opaque
strings rather than failing.

**Acceptance criteria**

- [ ] At most one valid execution per entity per pulse.
- [ ] Invalid entries are dropped without consuming the pulse's
      single execution slot.
- [ ] Validation order is exactly as in §4.3; the first failing
      check is the reported reason.
- [ ] Offensive classification is engine-derived from category +
      metadata; packs do not set "offensive" directly.
- [ ] Resource deduction uses the race-adjusted cost.
- [ ] Pulse delay is recorded on success, not on miss or fizzle.
- [ ] Hit chance collapses to "always" when variance is zero.
- [ ] Proficiency gain is rolled on both hit and miss.
- [ ] A post-resolution vital-depleted event is emitted only when
      the target reached HP ≤ 0 *during* this resolution.

---

## 5. Active effects

### 5.1 Building an effect

When a hit ability has an effect template, the engine constructs an
active effect with:

- The effect id from the template.
- The source ability id.
- The source and target entity ids.
- A remaining-pulse counter set from the template's duration. A
  negative duration means **permanent** (never decremented).
- Stat modifiers from the template, each tagged with a source key
  derived from the effect id, so the modifier set can be removed
  cleanly later.
- A copy of the template's flag list.

### 5.2 Application

Applying an effect to a target:

1. Look up the per-target effect list (a per-entity in-memory
   collection).
2. **Single-instance rule.** If the target already carries an
   active effect with the same id, the application MUST be refused
   and report failure to the caller (no event, no mutation).
3. Otherwise add the effect to the list and:
   - Add each stat modifier to the target's stat block. Modifiers
     MUST be deduplicated by `(source, stat)` to be safe against
     re-application after save/load.
   - Add each flag to the target as a tag.
4. Emit an "effect applied" event carrying effect id, source
   ability id, target, and duration.

### 5.3 Removal

Removal can be triggered by id, by flag, by expiration, or by
external systems (dispel, cleanse). In every case:

1. Reverse the stat modifiers by source key.
2. Reverse the flags as tags.
3. Remove the effect from the per-target list.
4. Emit an "effect removed" event (or, on expiration, an "effect
   expired" event).

Removal by flag removes **every** effect whose flag list contains
that flag, in one batch. Removal by unknown id or absent flag is a
silent no-op.

### 5.4 Tick

Once per combat round (as part of the status-effects phase, see
`docs/specs/combat.md` §3), the engine ticks every active effect:

1. For each active effect with a non-negative remaining count,
   decrement by one.
2. Any effect whose remaining count is now ≤ 0 is collected for
   expiration.
3. After iteration, each collected effect is removed from its
   target as in §5.3 with an "expired" event.

Permanent effects (remaining count negative) are not ticked.

The tick MUST NOT mutate the active-effect list during iteration;
expirations are batched and applied afterward.

### 5.5 Effects vs entity state

Active effects are ephemeral: their *state* (which effects are on
which entities, with what remaining counts) is not persisted across
save/load. However, the stat modifiers they applied to the
entity's stat block ARE persisted with the entity. To prevent
duplicate modifiers after a reload, modifier application is
deduplicated by source key (§5.2).

Content systems that need persistent buffs SHOULD either use
permanent effects (negative duration) and accept that the effect-
list state is not durable, or store the durable buff as a direct
property modifier outside the effect system.

### 5.6 Queries

The system MUST expose:

- "Does entity X carry effect Y?"
- "All active effects on entity X" as a snapshot list.

Snapshots MUST be copies, not references to internal state.

**Acceptance criteria**

- [ ] Applying an effect already present on the target fails
      cleanly with no mutation and no event.
- [ ] Stat modifiers are deduplicated by source key on re-apply.
- [ ] Removal by flag removes every matching effect in one batch.
- [ ] Permanent effects (negative duration) survive every tick
      until externally removed.
- [ ] Tick iteration is safe against mid-tick modifications of the
      effect list (expirations are batched).
- [ ] "Applied", "removed", and "expired" are distinct events.
- [ ] Snapshot queries do not expose mutable internal lists.

---

## 6. Passive abilities

Passive abilities are not queued. They are evaluated by other
subsystems at well-defined hooks (auto-attack swings, defensive
checks, etc.). The abilities feature provides three building
blocks:

### 6.1 Binary check

A binary check rolls success against the entity's proficiency in
the passive ability, modulated by the ability's variance and
max-chance settings:

- Effective chance is `proficiency × (variance / 100)` when
  variance is below 100; otherwise `proficiency × (max-chance /
  100)`.
- Roll uniformly in `1..100`. Success when roll ≤ effective chance.

Callers use this for "does the passive fire on this opportunity?"

### 6.2 Scaling bonus

A scaling bonus returns `max_bonus × proficiency / 100`, with
`max_bonus` declared in the ability's metadata. Used for passives
that contribute additive numeric bonuses (extra hit, extra damage,
extra crit chance) proportional to skill.

### 6.3 Hook-based discovery

Passive abilities are discovered by their metadata. The engine
exposes "all passive abilities tagged with hook H" so subsystems
can iterate the passives that apply to a particular event. The
canonical hooks used today include extra-attack chance and
defensive-check pre-emption, but the hook set is content-defined.

When a hook-driven passive fires, the engine MUST roll a
proficiency gain for it (the entity doesn't actively choose to use
it, but their using-it-in-context still trains them).

**Acceptance criteria**

- [ ] Binary check uses proficiency, variance, and max-chance as
      specified in §6.1.
- [ ] Scaling bonus uses `max_bonus` from metadata, scaled by
      proficiency.
- [ ] Hook iteration matches passives by metadata hook key, not by
      hardcoded ability id.
- [ ] Successful passive activations roll proficiency gain.

---

## 7. Observable events

The features publish at least these events. Each event carries
enough payload that observers do not need to query state for the
common case.

| Event | When |
|---|---|
| ability used | a queued ability resolved as a hit (§4.5) |
| ability missed | a queued ability resolved as a miss (§4.5) |
| ability fizzled | a queued ability failed validation (§4.3, §4.8) |
| effect applied | an active effect was added to a target (§5.2) |
| effect removed | an active effect was removed by id or by flag (§5.3) |
| effect expired | an active effect's remaining count hit zero (§5.4) |
| vital-depleted (hp) | a target reached HP ≤ 0 during resolution (§4.5) |

The handler token on an ability definition (§2.2) is forwarded
verbatim in the "ability used" payload so packs can attach
side-effect side channels without modifying the engine.

**Acceptance criteria**

- [ ] Every state transition in §3-§6 emits exactly the listed
      event with the documented payload.
- [ ] The handler token is forwarded on hit but never on miss or
      fizzle.

---

## 8. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| Default initial proficiency cap | §3.2 |
| Default proficiency cap when none set | §3.3 |
| Per-ability gain base chance, failure multiplier, gain stat & scale | §3.5, §2.2 |
| Luck scaling factor in hit chance | §4.5 |
| Race cost-adjustment formula | §4.7 |
| Resource pool used by skill vs spell | §4.7 |
| Default variance and max-chance when ability omits them | §2.2, §6.1 |
| Passive hook keys (e.g. "extra_attack", "defensive_check") | §6.3 |
| Reserved metadata keys (e.g. damage_dice, heal_dice, max_bonus) | §4.6, §6.2 |

---

## 9. Open questions / future work

- **Stacking refinement.** The single-instance rule per effect id
  is global to the target. Some games distinguish stacks by source
  caster (so two clerics' blesses don't collide). Whether to key
  the single-instance check on `(effect id, source id)` instead is
  a design question.
- **Refresh on re-cast.** Today a re-cast onto a target that
  already carries the effect is refused. A common alternative is
  "refresh duration if same id". Should be a per-effect-template
  policy.
- **Permanent-effect persistence.** Permanent effects do not
  survive a server restart because the active-effect list is
  ephemeral. If content depends on this, it needs an alternate
  storage path.
- **Damage attribution inside abilities.** The resolution phase
  emits the vital-depleted event but doesn't itself apply damage —
  damage is handler-defined. There is no central "incoming damage"
  hook to mediate wards or absorbs (also flagged in the combat
  spec).
- **Queue depth bound.** Nothing caps how many actions a player
  may have queued. A misbehaving client could push large lists.
- **Pulse-delay durability.** Pulse-delay state is ephemeral. A
  short-duration cooldown survives a reconnect; a long one does
  not. Whether long cooldowns should persist is a policy choice.
- **Magic strings.** Property names (`heal_dice`, `damage_dice`,
  hook names) are stringly-typed. A typed shape would prevent
  silent misregistration.
- **Initiate-only semantics.** Initiate-only fizzles when in
  combat; it does not engage combat on success. Whether such moves
  should *also* be the engagement step (so the user does not need
  a separate `kill` first) is unclear.

---

<!-- Generated: 2026-05-21 · Scope: AbilityRegistry + AbilityResolutionPhase + EffectManager + ProficiencyManager + PassiveAbilityProcessor · Spec style: narrative + acceptance criteria · Detail level: behavior only -->

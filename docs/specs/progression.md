# Progression — Feature Specification

**Status:** Draft · **Scope:** The stat block (attributes, vitals,
modifiers); race and class definitions and registries; the
progression-track system (levels, XP, level-up events); alignment
(integer value, buckets, cancellable shifts, history); training
(stat training and practice via in-room trainers); and the
class-driven side effects on level up (path entries, stat growth,
trains granted) · **Audience:** Anyone reimplementing or porting
this feature in any language.

This document describes *what* the progression substrate must do,
not *how* to implement it. The exact attribute set, dice formulas,
threshold values, cap tier ladder, and bucket boundaries are
policy and live outside this spec.

---

## 1. Overview

"Progression" in this feature set is the collection of mechanisms
that let an entity get stronger over time. Six distinct
substrates compose to produce that:

1. **Stat block** — the cached, modifier-aware container holding
   an entity's attributes and vital pools.
2. **Races** — content-defined origin records: stat caps,
   spell-cost modifier, racial flag set, category, starting
   alignment.
3. **Classes** — content-defined role records: per-level
   ability path, stat-growth dice, growth-stat bonuses, bound
   progression track, allowed race categories and genders,
   trains-granted per level, starting alignment.
4. **Progression tracks** — independently-leveled XP ladders.
   An entity has a `(level, xp)` pair for every track it has
   ever earned on. Classes bind to one track; other features
   (crafting, exploration) MAY define their own tracks.
5. **Alignment** — a bounded integer with named buckets, a
   cancellable shift event, mirrored tags on the entity, and a
   bounded history list.
6. **Training** — both ability-cap practice via in-room
   trainers and base-stat training spending a per-level "trains
   available" pool.

The pieces interlock:

- Level-up on a class's bound track fires events that the class
  path processor and stat-growth subscriber act on.
- Class growth-bonus tables refer to other attributes on the
  entity for cross-stat scaling.
- Training consults the race's stat caps and the proficiency
  feature's per-ability cap.
- Disposition rules (in the mobs feature) read alignment value
  and bucket; combat reads alignment too.
- Abilities (in the abilities feature) honor an optional
  alignment range gate.

### Non-goals

- The actual catalog of races, classes, and tracks (content).
- The user-facing commands `score`, `train`, `practice`, `who`
  (handled by the commands feature; this spec defines the
  operations they call).
- Persistence of any of this beyond noting which properties
  must persist with the entity (covered by
  `docs/specs/persistence.md`).
- The specific cap-tier values or threshold integers — the
  *structure* of caps and brackets is in scope, the numbers
  are policy.
- The death-penalty mechanic. The track definition carries the
  field; this spec acknowledges it but does not specify the
  penalty pipeline.

---

## 2. Stat block

### 2.1 Shape

Every combatant entity carries a stat block with three
sections:

- **Base attributes.** A fixed set of integer attributes (the
  six classic ones — strength, intelligence, wisdom, dexterity,
  constitution, luck — plus three vital-pool maxima: max-hp,
  max-resource, max-movement).
- **Current vitals.** Three integers (hp, resource, movement)
  that track present pool values.
- **Modifiers.** A list of `(source, stat, value)` records.
  Source is a free-form string used for keyed removal; stat is
  one of the StatType enum members; value is the integer delta.

### 2.2 Effective values

Reads of any attribute or vital maximum return `base + sum of
modifiers on that stat`. The block caches this computation
behind a dirty flag:

- The cache is invalidated by every base-attribute setter and
  by every modifier add/remove.
- Reads check the flag and recompute lazily when set.

The implementation MUST NOT expose stale cached values: any
mutation that could affect effective values MUST invalidate.

The block exposes a manual `Invalidate()` for callers (e.g.
training) that touch the block through alternate paths that
need to force a recompute before subsequent reads.

### 2.3 Vital clamping

Current vitals (hp / resource / movement) are stored
internally as integers and clamped on every write to the range
`[0, effective max]`. The cache recompute MUST re-clamp current
vitals after recalculating maxima: a buff that drops would
otherwise leave hp above the new max.

Writes to current vitals MAY be made through the public setter
(which clamps against the cached max) or via direct combat
mutation (`stats.Hp -= damage`); both paths clamp.

### 2.4 Modifiers

A modifier carries a source key (string), a target stat, an
integer value, and a modifier-type discriminator. The current
implementation supports only **flat** (additive) modifiers; the
type discriminator exists so percentage / multiplicative
modifiers can be added without breaking the existing surface.

**Removal by source** is the canonical lifecycle pattern:

- Equipment adds modifiers tagged `equipment:<item entity id>`;
  unequipping removes by that exact source key.
- Effects add modifiers tagged `effect:<effect id>`; expiration
  removes by that source key.
- Race / class / training touch base attributes directly, not
  through modifiers.

Multiple modifiers MAY share a source key; remove-by-source
removes them all in one call. The reverse — modifying by
`(source, stat)` tuple — is NOT exposed; consumers either
remove the whole source group or call `Invalidate()` and
mutate the list manually.

### 2.5 Display names

A separate `StatDisplayNames` registry maps lowercase stat names
to display strings. It carries a fixed default set
(`strength → Strength`, `hp → HP`, `resource → Mana`, etc.) and
allows per-name overrides registered at startup. Lookups fall
through default → unmapped (return the raw name).

This is the only point at which the feature exposes the
"is this resource called mana or essence?" decision. Content
configures it once; renderers and help generators consume it
everywhere.

### 2.6 Resource pools

The three current vitals (§2.1) are the engine's canonical pools, but
they are special cases of a general **resource pool** model. Each entity
carries a set of pools keyed by a **pool kind** (a content-friendly
string; `hp`, the primary `resource`, and `movement` are always
present). A ruleset MAY declare additional pools — a second magic
resource, a stamina or stun track — without engine changes.

Each pool holds a current value, a maximum, and a **floor** the current
clamps to (zero for the canonical pools; a ruleset MAY set a different
floor for a track that bottoms out elsewhere). A pool MAY carry optional
rules a ruleset attaches: routing the overflow past its floor into
another named pool, or capping a derived value's maximum by this pool's
current. These are reserved for rulesets that need them; the canonical
pools carry neither.

Pool maxima are derived from base attributes (§2.1) — max-resource,
max-movement, and so on. A pool's ceiling tracks its backing maximum:
raising the maximum (level-up, a buff) does NOT auto-fill the pool, but
lowering it clamps the current down (the §2.3 re-clamp rule, generalized
to every pool).

**Spending and depletion:**

- **Conditional spend** deducts only if the pool would not drop below
  its floor, atomically, and reports whether it spent. Used for an
  up-front affordability check that must not partially drain a pool.
- **Unconditional deduct** subtracts a pre-validated amount, clamping at
  the floor. Used on the spend-on-success path
  (`docs/specs/abilities-and-effects.md` §4.5) once validation has
  already proven affordability.
- A pool does not itself emit events. When a deduction or damage
  application drives a pool to its floor, the **owner** (combat /
  session) emits the vital-depleted signal exactly once, on the
  transition into the floor — not on subsequent hits to an
  already-floored pool.

**Regeneration:** pools regenerate by the owner restoring an amount on a
periodic tick; a pool has no clock of its own and never reads time
(honoring the F3 clock convention). Restore caps at the maximum.

The durable form of pools is covered by the save surface
(`docs/specs/saves.md`): only pools below full are persisted, and maxima
are re-derived from attributes at load, so rebalancing a pool's maximum
never requires a save migration.

**Acceptance criteria**

- [ ] Effective reads return `base + sum(modifiers)` for every
      stat.
- [ ] Base setters and modifier mutations invalidate the cache.
- [ ] Current vitals are clamped on every write and re-clamped
      after a max-affecting recompute.
- [ ] Remove-by-source removes every modifier with the given
      source key in one operation.
- [ ] Display-name lookup falls through overrides → defaults →
      raw name.
- [ ] Current vitals are entries in a per-entity pool set; the
      canonical hp / resource / movement pools are always present,
      and rulesets may add pools without engine changes.
- [ ] Conditional spend deducts only when the result stays at or
      above the floor and reports whether it spent; unconditional
      deduct clamps at the floor.
- [ ] A pool emits no events; the owner emits vital-depleted exactly
      once on the transition to the floor.
- [ ] Raising a pool maximum does not refill it; lowering it clamps
      the current down.
- [ ] Pool regeneration is owner-driven; a pool never reads a clock.

---

## 3. Races

### 3.1 Definition shape

A race definition carries:

- A stable id and a display name.
- A **stat-caps map** of `StatType → int`. Used by training to
  limit how high a base attribute can be trained on this race.
- A **cast-cost modifier** integer. Added to every ability's
  base resource cost (clamped at zero, never negative). Used by
  the abilities feature when validating and deducting resource
  cost.
- A **racial-flag list** of strings, applied as tags at
  instantiation (the mob spawn and the new-player flow both
  apply these).
- A pack name and priority (used by registry override
  semantics).
- Optional presentation fields: tagline, description, category
  (a string the class system uses for allowed-category
  filtering), starting alignment integer.

### 3.2 Registry

The race registry is keyed by stable id. Higher-priority
registrations replace lower-priority ones with the same id;
equal priorities retain the existing registration. (This
mirrors the class and ability registries — see §4.2.)

The registry exposes get-by-id, get-all, and has-id queries
only. It does NOT expose filtering by category — race-eligible-
for-class filtering happens on the class side (§4.4).

### 3.3 Cost calculator

A small helper `AdjustCost(baseCost, race)` returns
`max(0, baseCost + race.CastCostModifier)`. Used by the
abilities feature in both validation and deduction (§4.7 of
`docs/specs/abilities-and-effects.md`). A null race yields the
base cost unchanged.

### 3.4 Default-race fallback

A character record (player save, mob template) MAY carry an
empty race id — legacy saves written before races existed, fresh
characters that have not yet been through a character-creation
flow, or content that deliberately omits the field. The runtime
resolves an empty saved id against a host-configured **default
race** (e.g. `human`). If the resolved id is not in the registry
(race renamed or removed between restarts), the entity stays
**raceless**: empty race id, no RacialFlags applied, no
CastCostModifier, no StatCaps consulted by training.

Raceless is a fail-soft state, not an error condition. A pack
that removes a race in a content update does not break
characters that referenced it — those characters simply do not
benefit from any race contributions until they're explicitly
re-raced (a future admin verb, or a M12 character-creation
re-run).

The resolved id is mirrored back into the save so the chosen
default persists across logout. On a future load with a
different configured default, the previously-recorded id wins.

**Acceptance criteria**

- [ ] Race lookups are case-insensitive on id.
- [ ] Higher-priority registrations replace lower; equal-priority
      duplicates are no-ops.
- [ ] Adjusted cost is never negative.
- [ ] A null race yields the unmodified base cost.
- [ ] Empty saved race id resolves against the configured default
      race; unknown ids leave the entity raceless without
      erroring.

---

## 4. Classes

### 4.1 Definition shape

A class definition carries:

- A stable id and a display name.
- A **stat-growth map** of `StatType → dice expression string`.
  On every level-up of the bound track, each entry rolls the
  dice and adds the result to the entity's base stat.
- An optional **growth-bonuses map** of `StatType →
  source StatType`. For each entry, the level-up handler ALSO
  adds a bonus of `max(0, (sourceStatValue - 10) / 2)` (a
  D&D-style modifier derived from the source attribute's
  current effective value) to the rolled growth. This lets a
  caster's INT bump max-resource growth, or a fighter's CON
  bump max-hp growth.
- A **bound track name**. Level-ups on other tracks do NOT
  apply this class's growth or path. When unset, the class has
  no growth-on-level behavior.
- A **path list** of `ClassPathEntry(level, abilityId,
  unlockedVia)` entries. Each entry declares an ability to
  grant at a specific level on the bound track. The
  `unlockedVia` field marks entries owned by other systems
  (e.g. quest unlock) and excludes them from the level-up
  grant path (§4.5).
- A **trains-per-level** integer (default 5) granted to the
  entity via the training feature on every level-up.
- An **allowed-categories** list. The class is selectable
  during character creation only by races whose `RaceCategory`
  matches one of these. Empty list = available to every race.
- An **allowed-genders** list with the same semantics.
- A pack name and priority.
- Optional presentation fields: tagline, description, level-up
  flavor text, starting alignment.

### 4.2 Registry

The class registry mirrors the race registry: keyed by id,
priority-based overrides, get/get-all/has. It adds one query:

- **Get-eligible-classes(raceCategory, gender)** filters by
  the allowed-categories and allowed-genders lists. Empty
  list = unrestricted (the class is eligible regardless of
  the input). Used by character creation.

### 4.3 Eligibility semantics

A class is eligible iff:

- `allowedCategories.Count == 0` OR the race's category is in
  the list (case-insensitive).
- AND `allowedGenders.Count == 0` OR the gender is in the list
  (case-insensitive).

Eligibility is a creation-time concern. After commit, a class
swap (e.g. via quest reward, admin command) MAY bypass
eligibility — content's choice.

### 4.4 Level-up subscriptions

Two engine services subscribe to `progression.level.up` events
to apply class effects. The order in which their handlers run
on the bus is not specified — both are idempotent on the same
event.

### 4.5 Class path processor

Listens to `progression.level.up` and `character.created`. On
each:

1. Resolve the entity, its class id, and its class definition.
   If any is missing or the class declares no track, return.
2. For level-up events, require the event's `track` field to
   equal the class's bound track (case-insensitive).
3. For character-created events, treat the level as 1.
4. Iterate the class path. For every entry whose `level`
   matches the relevant level AND whose `unlockedVia` is empty:
   - Resolve the ability id. If missing from the ability
     registry, log a warning and skip.
   - Teach the ability via the proficiency feature at initial
     proficiency 1.
   - Enqueue a player-visible notification ("You have learned
     &lt;ability name&gt;!") via the notification queue.

Path entries with `unlockedVia` set are owned by another
subsystem (quest reward, scripted hook); the path processor
treats them as not-its-problem.

### 4.6 Stat growth on level up

Listens to `progression.level.up`. On each event:

1. Resolve the entity and class. Return on missing.
2. **No track gate.** This handler runs for every level-up,
   regardless of which track triggered it. (Whether to gate on
   track is open — see §10.)
3. For each `(stat → dice)` entry in the growth map:
   - Roll the dice (using the dice notation parser from the
     combat feature).
   - If the growth-bonuses map declares a source stat for this
     entry, add `max(0, (sourceStatValue - 10) / 2)` to the
     roll.
   - Apply the resulting delta to the entity's base stat
     (which invalidates the stat block).
4. Grant `trainsPerLevel` trains via the training feature.

**Acceptance criteria**

- [ ] Class lookups are case-insensitive on id.
- [ ] Eligibility honors allowedCategories and allowedGenders
      with empty-list-means-unrestricted.
- [ ] Path processor grants only entries whose `unlockedVia`
      is unset.
- [ ] Path processor runs at level 1 on character creation.
- [ ] Path processor logs and skips unknown ability ids
      without raising.
- [ ] Stat-growth handler rolls each declared dice entry per
      level-up.
- [ ] Growth-bonus formula uses the *effective* (post-modifier)
      value of the source attribute.
- [ ] Trains-per-level are credited on every level-up.

---

## 5. Progression tracks

### 5.1 Track definitions

A track definition carries:

- A stable name.
- A maximum level.
- EITHER an **XP table** array indexed by level (`xpTable[L]`
  is the total XP required to reach level L) OR an **XP
  formula** function `L → totalXp`. Table takes priority when
  both are set.
- An optional `OnLevelUp(entityId, trackName, newLevel)`
  callback fired by the manager (in addition to the bus
  event).
- An optional death-penalty fraction (acknowledged but not
  driven by this feature).

The manager queries the track for `GetXpForLevel(L)`. If
neither table nor formula is set, it returns -1 (sentinel for
"undefined"). Negative thresholds short-circuit level-up
checks (no further progress beyond an undefined threshold).

### 5.2 Per-entity state

Each entity carries two map properties keyed by track name:

- A `level` map of `track → current level`.
- An `xp` map of `track → total XP earned`.

Both persist with the entity. An entity that has never earned
on a given track has no entry in either map; lookups treat
this as `(level=0, xp=0)` until the first interaction
initializes the track (§5.3).

### 5.3 Lazy initialization

The first call to `GetLevel`, `GetTrackInfo`, or
`GrantExperience` for a track on an entity initializes the
entry to `(level=1, xp=0)`. Subsequent reads see level 1.
Initialization is implicit; callers do not need to
pre-register a player on every track.

### 5.4 Granting XP

`GrantExperience(entityId, amount, trackName, source)`:

1. Look up the track. If unknown, return silently.
2. Look up the entity. If unknown, return silently.
3. Initialize the entity's track entry if absent (§5.3).
4. Add `amount` to the entity's XP on this track.
5. Emit `progression.xp.gained` with track, amount, source,
   new total.
6. Loop: while the entity is below `MaxLevel` AND the next
   level's threshold is defined AND the new total has reached
   it, increment the entity's level, fire the track's
   `OnLevelUp` callback, and emit `progression.level.up` with
   old / new level, track, and entity id.

The loop allows a single XP grant to cascade through multiple
level-ups when a big award (e.g. a quest reward) crosses
several thresholds.

XP that pushes the entity past max level is NOT clamped — the
XP keeps accumulating, but no further level-ups fire. The
TrackInfo query (§5.6) reports the excess as `overflow`.

### 5.5 Deducting XP

`DeductExperience(entityId, amount, trackName)` removes XP
without de-leveling the entity:

1. Look up the track and entity.
2. The XP floor is the current level's threshold (or 0 for
   level 1). XP cannot drop below this floor.
3. Compute `newXp = max(floor, currentXp - amount)`. The
   actual loss is `currentXp - newXp` (may be less than the
   caller asked for).
4. If actual loss > 0, emit `progression.xp.lost` with track,
   amount, and new total.

This means death penalties can erase progress within the
current level but cannot strip a level once earned. Whether
this is the intended invariant is open (§10).

### 5.6 Track info query

`GetTrackInfo(entityId, trackName)` returns a structured view
suitable for the score panel and GMCP packages:

- `Xp`: total XP on this track.
- `Level`: current level (after lazy init).
- `XpToNext`: `max(0, nextThreshold - xp)`; zero at max level.
- `CurrentLevelThreshold`: `GetXpForLevel(level)` clamped to
  ≥ 0 (level 1 has threshold 0).
- `MaxLevel`: the track's max level.
- `Overflow`: at max level, `xp - currentLevelThreshold`;
  zero below max.

Used by renderers to draw progress bars without each renderer
re-implementing the threshold math.

### 5.7 Reset

`ResetTrack(entityId, trackName)` sets level to 1 and XP to 0
and emits `progression.track.reset`. No automatic level-up
fires from the reset (the level reset is downward, not a
fresh start that crosses thresholds). Used by admin tooling.

**Acceptance criteria**

- [ ] Tracks are looked up case-sensitively by name.
- [ ] Lazy initialization happens once per entity per track on
      first interaction.
- [ ] XP grants emit `progression.xp.gained` exactly once per
      call.
- [ ] A single grant can cascade through multiple level-ups in
      one call.
- [ ] XP cannot drop below the current level's threshold via
      deduct.
- [ ] Overflow at max level is reported via TrackInfo, not
      clamped.
- [ ] Reset emits its event and does not re-enter the level-up
      loop.

---

## 6. Alignment

### 6.1 Model

Alignment is a single signed integer per entity, persisted on
the entity as a property. The integer is bounded by configured
minimum and maximum values (today -1000 / +1000). Three named
**buckets** partition the range:

- **Evil** — at or below the configured evil threshold.
- **Good** — at or above the configured good threshold.
- **Neutral** — strictly between.

Thresholds are configurable at startup. The default
(non-configured) entity alignment is zero.

### 6.2 Bucket tags

Whenever the bucket changes (and on every Bucket / Set / Shift
call), the manager mirrors the bucket as a tag on the entity:
`alignment_evil`, `alignment_neutral`, or `alignment_good`.
Exactly one of these tags is present at a time; setting the
new tag also removes the other two.

This mirroring lets the world's tag index (see
`docs/specs/world-rooms-movement.md` §3.4) drive "all good
players" queries efficiently and lets disposition rules match
on tags as well as numeric ranges.

### 6.3 History

Every successful shift appends an entry to a per-entity
`alignment_history` property — a bounded list of records
carrying timestamp, delta, reason, and resulting value. The
list is capped at a fixed capacity (today 20); older entries
are dropped from the front when capacity is exceeded.

Set operations (the admin path, §6.4) do NOT append history.

### 6.4 Operations

**Get** returns the current integer (defaulting to zero for
missing entities or missing property).

**Bucket** returns the current bucket name AND ensures the
tag mirror is in sync (it's idempotent if already in sync).

**Set(entityId, value, reason)** is the **admin / scripted
override**: it clamps the value to the configured range,
writes it, updates tags, and emits **no events**. Used by
admin commands, character creation seeding, and tests.

**Shift(entityId, delta, reason, context?)** is the **gameplay
path**:

1. Resolve the entity. Return on missing.
2. **Admin bypass.** If the entity carries the `admin` role,
   return immediately. Admin characters are alignment-immune.
3. Build an `alignment.shift.check` event with `actorId`,
   `reason`, `suggestedDelta`, `cancel: false`, and any
   additional fields the caller passed via `context`.
4. Publish the event. Listeners may set `cancel: true` to
   abort, or may rewrite `suggestedDelta` to a different
   integer (e.g. an item that halves negative alignment
   shifts).
5. If cancelled, return.
6. Resolve the post-event delta (treating numeric coercion
   leniently — int or double both accepted).
7. If the resolved delta is zero, return without an event.
8. Apply the shift (§6.5).

### 6.5 Applying a shift

1. Read the old value (default 0).
2. Compute the new value clamped to `[min, max]`.
3. Compute `actualDelta = newValue - oldValue`. If zero,
   return.
4. Write the new value, update the bucket tag, and append a
   history entry.
5. Emit `alignment.shifted` with old / new / actualDelta /
   reason and a `bucketChanged` boolean.
6. If the bucket changed, ALSO emit `alignment.bucket.changed`
   with old / new bucket names.

### 6.6 Bucket-set resolution for content

A helper `ResolveBuckets(buckets)` translates a set of bucket
names (used in disposition rules and ability gates) into a
`(min, max)` range:

- `{evil}` → `(null, evilThreshold)`
- `{good}` → `(goodThreshold, null)`
- `{neutral}` → `(evilThreshold+1, goodThreshold-1)`
- `{evil, neutral}` → `(null, goodThreshold-1)`
- `{good, neutral}` → `(evilThreshold+1, null)`
- `{evil, good}` → `(null, null)` (degenerate: matches
  everything, since the gap between them is "neutral")
- empty / all three → `(null, null)`

Resolution uses the threshold values at the moment of the
call. Content that calls this at registration time bakes in
the thresholds as numbers, so changing thresholds at runtime
does not retroactively change registered rules.

**Acceptance criteria**

- [ ] Alignment is bounded by min/max on every write.
- [ ] The alignment bucket tag is present and unique on every
      entity that has been touched by the manager.
- [ ] History is bounded at a fixed capacity.
- [ ] Set does not emit events and does not append history.
- [ ] Shift is a no-op for admin entities.
- [ ] The shift check is cancellable and the delta is
      rewritable by listeners.
- [ ] `alignment.shifted` fires whenever the value actually
      changes; `alignment.bucket.changed` ALSO fires when the
      bucket changes.

---

## 7. Training

The training feature exposes two distinct operations:
**practice** (raising the cap on a learned ability) and **stat
training** (spending a train to bump a base attribute).

### 7.1 Trains-available pool

Every player carries a `trains_available` integer property.
Classes credit it on level-up via the stat-growth subscriber
(§4.6). Stat training (§7.4) decrements it by one per success.
Practice (§7.3) does NOT consume trains.

### 7.2 Cap tier ladder

Ability proficiency is bounded above by a per-ability cap on
the entity (see `docs/specs/abilities-and-effects.md` §3).
Trainers raise this cap up an ordered ladder of **cap tiers**.
The current ladder has four tiers — Novice, Apprentice,
Journeyman, Master — at proficiency values 25, 50, 75, 100.

The ladder is fixed by the engine: tier values are not
content-configurable. A trainer registered at a given tier can
ONLY teach a player whose current cap is exactly the tier
below — no skipping. (A Journeyman trainer cannot teach a
Novice; the player must first find an Apprentice trainer.)

### 7.3 Practice via in-room trainer

**Find trainer in room.** Iterates the entities in the
player's current room and returns the first entity carrying
the `skill_trainer` tag AND a non-null `TrainerConfig` (which
declares the trainer's tier and the ability ids it teaches).

**Try practice(entityId, abilityId).** Resolves the practice
attempt and returns a structured result:

- **NotLearned** — the player has no proficiency entry for
  this ability.
- **NoTrainer** — no matching trainer in the room.
- **CannotTeach** — the trainer's ability list does not
  include this ability.
- **AlreadyAtOrAboveTier** — the trainer's tier is at or below
  the player's current cap. (You have surpassed what they can
  teach.)
- **TierSkip** — the trainer's tier is not exactly the next
  step on the ladder above the current cap. (You must master
  the basics first.)
- **Success** — set the cap to the trainer's tier value.
  Apply a **catch-up boost** (§7.5).

The result carries a user-facing message string. Each result
kind has a canonical message; content controls only the
trainer names embedded in the messages.

### 7.4 Stat training

**Try train(entityId, stat):**

1. Resolve the entity. Fail with NotTrainable if missing.
2. **Safe-room check** when configured. If the global config
   `RequireSafeRoomForStats` is true, the entity's current
   room must carry the `safe` tag. Otherwise fail with
   UnsafeRoom.
3. **Trainable-stat check.** The stat name must be in the
   global config's trainable list. Otherwise fail with
   NotTrainable. (The default list is the six attributes;
   content may add or remove.)
4. **Trains-available check.** The player must have at least
   one train. Otherwise fail with NoTrains.
5. **Race cap check.** The entity's current effective value
   for the stat must be below the race's cap for that stat
   (default 25 when the race doesn't declare one, default
   when the race itself is unknown). Otherwise fail with
   AtRaceCap.
6. Spend one train (`trains_available -= 1`).
7. Increment the base attribute by 1.
8. Invalidate the stat block.
9. Return Success with the new effective value.

### 7.5 Catch-up boost

When `TryPractice` raises the cap from value V to value V+25
(or whatever the next tier is), the player's *current*
proficiency may be anywhere from 1 to V. If proficiency is
strictly below V, the practice operation also bumps proficiency
toward V by a configured **catch-up boost** amount (default 5),
clamped at V.

This rewards players who get their cap raised early — they
don't have to grind every point of the prior tier to make
progress at the new one. If proficiency is already at V (the
prior cap), no boost is applied; subsequent gains come from
ability use.

### 7.6 Configuration

The training configuration carries:

- A `RequireSafeRoomForStats` boolean.
- A trainable-stats list (default: the six attributes by
  lowercase name).
- A catch-up boost integer (default 5).
- A runtime setter `SetTrainable(stat, enabled)` for
  admin / scripted toggles.

**Acceptance criteria**

- [ ] Trains-available defaults to zero and increases only via
      class level-up credits.
- [ ] Practice requires both a learned ability and a matching
      in-room trainer.
- [ ] Practice cannot skip cap tiers; the trainer's tier must
      equal `nextTier(currentCap)`.
- [ ] Practice does not consume a train.
- [ ] Stat training enforces the safe-room rule only when the
      config flag is set.
- [ ] Stat training honors the per-race stat cap.
- [ ] Catch-up boost bumps proficiency toward the prior cap, not
      the new cap.

---

## 8. Observable events

The features publish at least these events. Each event carries
enough data for observers to act without re-querying state in
the common case.

| Event | When |
|---|---|
| progression.xp.gained | XP was added to a track (§5.4) |
| progression.xp.lost | XP was deducted (§5.5) |
| progression.level.up | A track level increased (§5.4) |
| progression.track.reset | A track was reset to level 1 / 0 XP (§5.7) |
| alignment.shift.check (cancellable) | before an alignment shift is applied (§6.4) |
| alignment.shifted | an alignment shift was applied (§6.5) |
| alignment.bucket.changed | the alignment bucket changed (§6.5) |

The training feature emits no events directly; its callers
(commands, scripts) produce user-visible feedback from the
returned result records.

Stat-block mutations also do not emit events. Stat changes are
high-frequency and observers that care (e.g. GMCP `Char.Vitals`)
read snapshots on tick boundaries, not on every mutation.

**Acceptance criteria**

- [ ] Every transition in §5 and §6 emits exactly the listed
      event with the documented payload.
- [ ] `alignment.shift.check` is the only event in this feature
      set that listeners may cancel.

---

## 9. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| The attribute set and vital-pool maxima identities | §2.1 |
| Stat display names and per-name overrides | §2.5 |
| Race catalog (definitions, caps, modifiers, flags, categories) | §3 |
| Class catalog (definitions, growth, paths, allowed sets) | §4 |
| Track catalog (max level, XP table or formula, on-level-up) | §5 |
| Alignment min/max bounds | §6.1 |
| Alignment evil / good threshold values | §6.1 |
| Alignment history capacity | §6.3 |
| The cap-tier ladder values (today 25/50/75/100) | §7.2 |
| Trainable-stats list | §7.4 |
| RequireSafeRoomForStats flag | §7.4 |
| Catch-up boost amount | §7.5 |

---

## 10. Open questions / future work

- **Modifier types are flat-only.** The `ModifierType` enum
  has only `Flat`. Percent / multiplicative / "set to" modifier
  kinds would unlock content (e.g. a curse that halves
  strength) but require a layered application order.
- **Stat-growth handler runs on every track's level-up.**
  Today the stat-growth subscriber does not gate on the class's
  bound track, so leveling on a side track (crafting, etc.)
  would still apply class growth. Whether that is intentional
  ("everything you do makes you stronger") or a bug ("only the
  main track gives stat growth") needs a decision.
- **XP overflow at max level is uncapped.** Total XP keeps
  accumulating forever after max level. Consumers that read
  the raw XP property must handle large integers; consumers
  that read TrackInfo see `overflow` cleanly.
- **DeductExperience can't strip a level.** Death penalties or
  scripted XP loss never drop the entity below the current
  level's threshold. Whether that's the design or a bug worth
  fixing (so big penalties can de-level) is open.
- **Alignment Set is silent.** Admin / scripted Set operations
  emit no events. Quest hooks that watch alignment.shifted
  miss admin overrides. A `silent` flag on Shift would let
  callers opt out instead of going through the no-event
  variant.
- **Cap tier ladder is hardcoded.** The Novice/Apprentice/
  Journeyman/Master values 25/50/75/100 are baked into both
  the enum and the `NextTierValue` function. Externalizing the
  ladder would let content with a different progression
  structure (e.g. five tiers, asymmetric spacing) reuse the
  practice mechanism.
- **Race stat-cap default is hardcoded.** A race that doesn't
  declare a cap for a stat uses 25 as the default. Treating
  this as a per-feature config value would make the boundary
  visible.
- **No event on training success.** Practice and stat training
  return result records but emit no events. Observers (quest
  watchers wanting "train your strength 5 times") need a
  custom integration. A `training.complete` event would be
  symmetric with the rest of the feature set.
- **Two level-up subscribers, undefined order.** The class
  path processor and the stat-growth subscriber both react to
  `progression.level.up`. They commute today (path grants
  abilities, growth bumps stats; neither reads the other's
  output), but a future subscriber that reads stats during
  level-up may need ordering guarantees.
- **Class swap consequences.** The spec describes the create-
  time eligibility check but says nothing about what happens
  when an existing character's class property changes (e.g.
  via quest unlock). Path grants for the new class at lower
  levels do not retroactively fire, so the player misses
  those abilities silently.

---

<!-- Generated: 2026-05-21 · Scope: StatBlock + StatType + StatModifier + StatDisplayNames + RaceDefinition + RaceRegistry + RaceCostCalculator + ClassDefinition + ClassRegistry + ClassPathProcessor + StatGrowthOnLevelUp + TrackDefinition + ProgressionManager + ProgressionProperties + AlignmentManager + AlignmentConfig + AlignmentRange + TrainingManager + TrainingConfig + TrainingProperties + CapTier + TrainerConfig · Spec style: narrative + acceptance criteria · Detail level: behavior only -->

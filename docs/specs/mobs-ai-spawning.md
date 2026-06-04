# Mobs, AI, and Spawning — Feature Specification

**Status:** Draft · **Scope:** Mob templates, area-driven spawn/respawn,
behavior dispatch, disposition reactions, command issuance, and loot
generation · **Audience:** Anyone reimplementing or porting this
feature in any language.

This document describes *what* the mob subsystem must do, not *how* to
implement it. Specific behavior implementations (wanderer, patrol,
shopkeeper), default intervals, chance values, and stat-growth
formulas are policy and live outside this spec.

---

## 1. Overview

Mobs (mobile non-player characters) are entities the world spawns,
moves, attacks with, and despawns on death. The system has four
loosely-coupled responsibilities:

1. **Templates** — content-defined descriptions of a mob type.
2. **Spawning** — turning a template into a live entity placed in a
   specific room, on an area-driven schedule.
3. **AI** — running per-mob behavior handlers each AI tick, plus
   flee, disposition reactions, and mob-issued commands.
4. **Loot** — resolving an item drop list when a mob is created
   (loot is generated at spawn time, not on death — see §6.3).

Mobs share the entity model with players. A mob is just an entity
whose type is `npc` carrying a stable template id.

### Core concepts

- **Mob template** — a content-defined record holding everything
  needed to instantiate one mob: name, tags, keywords, base stats,
  optional class/race/level, behavior name, equipment list, loot
  table id, disposition rules, idle and battle command scripts,
  abilities, patrol route, trainer or shop config, free-form
  properties.
- **Spawn rule** — a placement instruction inside an area: which
  template, which room, how many copies, optional rare-swap config,
  rule-level tags.
- **Area spawn config** — a per-area bundle of spawn rules plus a
  reset interval (in ticks).
- **Behavior** — a string name (e.g. `stationary`, `wanderer`,
  `patroller`, `shopkeeper`) bound to a behavior handler at startup.
  Each mob carries one behavior name as a property.
- **Disposition** — a mob's reaction policy toward a player given
  the player's alignment, alignment bucket, and tags.
- **Mob command** — a string the mob issues into the world (a `say`,
  an `emote`, a movement, an ability) routed through the command
  system as if the mob typed it.

### Goals

1. Let content declare what mobs exist (templates, loot tables,
   spawn rules) without engine changes.
2. Drive an area-tick clock that periodically repopulates each
   area's spawn rules to their declared counts.
3. Run behavior, flee, and disposition logic only for mobs in
   *active* areas (areas with at least one player nearby).
4. Provide a dispatch layer for both immediate mob commands and
   delay-queued commands, so AI handlers can compose sequences
   without blocking the tick loop.
5. Emit observable events at every meaningful transition.

### Non-goals

- Player-vs-environment combat resolution (covered in
  `docs/specs/combat.md`).
- Ability validation and resolution (covered in
  `docs/specs/abilities-and-effects.md`).
- The world's room model, movement, and area registry. This spec
  references area ids; the world feature owns them.
- Persistent mob state across server restart. Mobs are
  re-spawned by the area-tick mechanism on the first qualifying
  reset after restart; nothing about an individual mob is
  durable.
- Player respawn / corpse / loot pickup. Mob loot is generated at
  spawn into the mob's contents; how the player retrieves it is
  the inventory feature's concern.

---

## 2. Templates and content

### 2.1 Template registration

Mob templates are registered into a single template registry keyed
by stable id. The system MUST expose:

- Register a template.
- Look up a template by id.
- Enumerate all templates.

Templates may be authored in any content format; the engine
consumes a normalized in-memory record.

### 2.2 Template content

A template MUST carry:

- A stable id.
- A display name.
- A base entity type (default `npc`).
- A base disposition value.
- A behavior name.

A template MAY carry:

- Tags and keywords applied at instantiation.
- Base stat block (six attributes, three vital pools).
- A class id and a level (drives stat derivation; see §3.2).
- A race id (drives racial flags and resource cost modifiers).
- An equipment list of item-template ids equipped at spawn.
- A loot-table id used at spawn (§6).
- Free-form property bag merged into the entity's properties.
- Disposition rules (§5).
- Idle-command set with chance and interval; battle-command set
  with chance and interval; optional ability list with per-entry
  proficiency overrides; default proficiency for unspecified
  abilities.
- Patrol route (list of room ids).
- Trainer / shop configuration.
- A script reference for ad-hoc per-mob logic.

### 2.3 Instantiation

Creating an entity from a template MUST:

1. Build a fresh entity of the configured type with the configured
   name.
2. Apply tags and keywords. The tag that matches the entity's own
   type MUST NOT be re-applied (it is implicit).
3. Copy base stats and set current vitals (HP, resource, movement)
   to their maxima.
4. Set the template id property to the source template id (so loot
   listeners and AI can identify the template later).
5. Set the behavior property to the configured behavior name.
6. Copy patrol route, idle/battle command sets, shop sells list,
   chances and intervals into properties.
7. Copy disposition rules onto the entity if declared.
8. Copy script reference, trainer config, and shop config if
   declared.

After instantiation, **stat derivation** (§3.2) may further adjust
base stats. Instantiation itself MUST be deterministic given the
template inputs.

**Acceptance criteria**

- [ ] Templates register by stable id and are retrievable by id.
- [ ] Instantiation produces a fresh entity carrying the template
      id property.
- [ ] Tags, keywords, and properties from the template appear on
      the entity.
- [ ] HP/resource/movement start at their max after instantiation.

---

## 3. Spawning

### 3.1 Spawn placement

The system MUST expose a primitive: *spawn this template id into
this room id*. The placement steps are:

1. Resolve the template; if missing, fail silently and return.
2. Resolve the room; if missing, fail silently and return.
3. Instantiate the entity from the template (§2.3).
4. Apply stat derivation (§3.2).
5. Set the entity's location to the room and add it to the room.
6. Track the entity in the world's live-entity set.
7. Instantiate and equip items from the template's equipment list
   (§3.3).
8. If a loot table is declared, generate loot into the mob's
   contents (§6).
9. If abilities are declared, register their proficiencies on the
   entity using the proficiency feature (§3.4).
10. Emit a "mob spawned" event carrying the entity id, room id,
    and template id.

The primitive returns the new entity reference (or none on
failure).

### 3.2 Stat derivation from class and race

If the template declares a class and a non-zero level, the system
MUST grow the entity's base stats by `averageDice(growth) × level`
for each stat in the class's growth table, using **integer
averaging** of the dice expression. After growth, current vitals
are reset to max so the level-applied HP gain is immediately
available.

If the template declares a race, the system MUST set a race
property on the entity and apply the race's flag list as tags.

Class and race are independent: a template may carry one, both, or
neither.

### 3.3 Equipment and stat modifiers

For each equipment template id on the mob template:

1. Create an item instance through the item registry; skip if not
   found.
2. If the item declares a slot, equip it into that slot.
3. If the item declares stat modifiers, add each modifier to the
   entity's stat block with a source key derived from the item id,
   so the modifier set can be cleanly reversed later.
4. Track the item in the world's live-entity set.

### 3.4 Ability proficiencies

For each ability entry in the template, the system MUST teach the
ability to the entity via the proficiency feature using either the
per-entry proficiency, the template-level default, or an engine-
level default (in that order of precedence).

This is how mob casters and skill-users gain their kit. The
proficiency feature owns the actual storage; spawn just calls into
it.

### 3.5 Area spawn config and the reset cycle

Each area carries a spawn config:

- An area id.
- An ordered list of spawn rules. Each rule names a room, a mob
  template id, a target count, an optional rare-swap (alternate
  template with a per-roll chance), an optional per-rule reset-
  interval override, and a rule tag list.
- A reset interval in ticks (the default cadence at which the area
  refills).

The system tracks every spawned entity against the `(area, rule
index)` pair that produced it. This tracking is purely runtime
state: it does not persist across restart.

### 3.6 Area reset

An area reset is triggered by an "area tick" event. On reset for
area A:

1. **Purge dead tracking.** Remove tracking entries whose entity
   no longer exists in the world.
2. **For each spawn rule** in registration order:
   - Resolve the room; skip the rule if the room does not exist.
   - Count the currently-tracked living instances of this rule.
   - Read the **persistent** flag from the rule's tag list.
   - If persistent and the count is at or above the target, skip
     this rule.
   - Otherwise compute `missing = target − living`.
   - For each missing slot:
     - Choose a template id: by default the rule's template, but
       if the rule has a rare-swap and the rare chance roll
       succeeds, use the rare template instead.
     - Spawn the chosen template into the rule's room (§3.1).
     - Record tracking for the new entity against this rule.

The spec does NOT require any particular handling of overpopulation
(more living instances than target). The system MAY leave them
alone.

### 3.7 Area-tick clock

A separate area-tick service emits "area tick" events at a per-
area cadence. The cadence is configured as `(baseInterval ×
occupiedModifier)` where the modifier applies only when the area
has at least one player present. A modifier above one slows resets
in occupied areas; a modifier below one speeds them up. The
modifier MAY be overridden at runtime per area.

Each "area tick" event carries the area id, a monotonic tick
count, and the current player count. The spawn manager subscribes
and treats the area id as the cue to call §3.6.

**Acceptance criteria**

- [ ] Spawn placement is idempotent on missing template or missing
      room.
- [ ] The template id is set on the entity after spawn.
- [ ] Stat derivation grows stats by `average(dice) × level` using
      integer dice averaging.
- [x] Equipment instantiation is skipped silently when an item
      template is missing.
- [x] Stat modifiers from equipment are tagged with a source key so
      they can be reversed.
- [ ] Ability proficiencies are taught using the configured
      precedence (entry → template default → engine default).
- [ ] Area resets purge dead tracking before counting.
- [ ] `persistent` rules respect the target count as a ceiling.
- [ ] Rare-swap rolls are independent per missing slot.
- [ ] Area-tick cadence is `base × occupiedModifier`; the modifier
      is configurable globally and overridable per area.
- [ ] Every spawn emits a "mob spawned" event with template id.

---

## 4. AI tick and behavior dispatch

### 4.1 Active vs inactive areas

To avoid simulating empty areas, the AI tick MUST iterate only
mobs whose room belongs to an **active** area. An area is active
while its player count is above zero. Player count is maintained
by `playerEnteredRoom` / `playerLeftRoom` callbacks invoked by the
world's movement subsystem. An area MAY also be activated
explicitly (e.g. for tests or scripted simulation) and remains
active until deactivated or all players leave.

The area id of a room is derived from a stable convention (e.g.
the prefix before a colon in a `area:room` id). The world owns
that convention; this spec consumes it.

### 4.2 Behavior registration

Behavior handlers are registered by name at startup. The mob AI
manager exposes:

- Register a `(name, handler)` pair.
- Query whether a behavior name is registered.

Behaviors are content-defined — examples include `stationary`,
`wanderer`, `patroller`, `shopkeeper`, `trainer` — but the engine
itself does not assume any specific name.

### 4.3 Per-tick processing

On each AI tick the system MUST:

1. Increment the AI tick counter.
2. Clear the disposition evaluator's per-tick dedup cache (§5.2).
3. For each NPC entity with a non-null location whose area is
   active:
   - Try a flee attempt (§4.5). If the flee succeeded, skip the
     rest of this mob's tick.
   - Look up the registered behavior handler by name. If present,
     invoke it with a context carrying the entity id, name, room
     id, and behavior name. Exceptions from handlers MUST be
     caught and logged without aborting the tick.
   - Emit a "mob AI tick" event for the mob even if no handler is
     registered. The event signals that this mob was considered
     this tick — useful for tests, telemetry, and content that
     hooks the event directly.
   - If the mob carries disposition rules, evaluate them against
     every player in the mob's current room (§5.3).

### 4.4 Last-action tracking

The AI manager exposes a `record-action(entityId)` and a
`ticks-since-last-action(entityId)` so behavior handlers can rate-
limit their own actions without each handler maintaining its own
clock. Behaviors that have not recorded any action return a
sentinel "very large" value, so the first call always passes a
"has it been long enough?" gate.

### 4.5 Flee gate

Before invoking a behavior handler, the AI manager checks whether
the mob should flee:

1. If the mob does not carry a flee-threshold property, no flee.
2. If the threshold cannot be interpreted as a positive number, no
   flee.
3. If the mob is not currently in combat, no flee.
4. If the mob's current HP percentage is at or above the
   threshold, no flee.
5. Otherwise call into the combat feature's flee mechanism with a
   pulse context built from the AI tick. If flee succeeds, the
   mob's tick stops here (no behavior handler runs).

This is the mob analogue of player wimpy from the combat spec.

### 4.6 Room-entry hooks

The AI feature provides three room-entry hooks:

- **Player entered room (immediate).** Called inside movement so
  it runs *before* the player sees the room description. The
  evaluator runs in "aggro-only" mode: only hostile reactions
  dispatch; non-hostile ones are intentionally deferred.
- **Player entered room (deferred).** Called after the room
  description is rendered. Runs the full evaluator (non-hostile
  reactions included). This split keeps friendly-greeting messages
  from appearing before the room description.
- **Mob entered room.** When a mob moves into a room, evaluate
  against every player present. No ordering concern (no room
  description involved), so all reactions fire immediately.

**Acceptance criteria**

- [ ] AI tick iterates only mobs whose area is active.
- [ ] A successful flee suppresses the behavior handler for that
      tick.
- [ ] Behavior handler exceptions are caught and logged.
- [ ] A "mob AI tick" event fires for every mob considered, even
      when no handler is registered.
- [ ] The immediate room-entry hook dispatches hostile reactions
      only; non-hostile reactions arrive via the deferred hook.
- [ ] Last-action tracking exposes a query and a recorder usable
      by any behavior.

---

## 5. Disposition reactions

### 5.1 Reaction model

A reaction is a string keyword. The canonical set is `hostile`,
`wary`, `friendly`, `neutral`, but the set is extensible — content
defines reactions and the engine treats unknown reactions as
no-op (no event dispatched).

A mob carries either:

- A static `base disposition` (the legacy path: a fixed
  disposition value the mob uses regardless of player state); or
- A `disposition definition` consisting of a default reaction and
  an ordered list of conditional rules; or
- Both. Static `hostile` always wins (§5.3).

### 5.2 Per-tick dedup and per-room reaction state

The disposition evaluator maintains two caches:

- A **per-tick dedup cache**, cleared at the top of every AI tick.
  Prevents re-evaluating the same `(mob, player)` pair multiple
  times within a single tick.
- A **per-room reaction state**, cleared for a player when they
  leave a room (via subscription to a `player.moved` event). Used
  to suppress spamming the same non-hostile reaction repeatedly.
  Hostile reactions bypass the state check — they are emitted on
  every evaluation, relying on combat's engagement idempotency to
  collapse duplicates.

### 5.3 Evaluation

To evaluate one mob against one player:

1. If the pair is already in the per-tick dedup cache, do nothing.
2. If the mob has no disposition rules, do nothing.
3. If the mob's static base disposition is `hostile`, the reaction
   is unconditionally `hostile`.
4. Otherwise, iterate the rule list in order. The first rule whose
   condition matches sets the reaction. If no rule matches, the
   reaction is the default.

Rules match against player **alignment value**, **alignment
bucket**, and **tags**:

- `min alignment` / `max alignment` — inclusive bounds against the
  player's current alignment integer.
- `buckets` — set membership against the player's current bucket
  (the alignment feature owns the bucketing).
- `has tag` — the player must carry the named tag.

A rule with all conditions present requires all of them. A rule
with no conditions matches anything (typically the last entry in
the list).

### 5.4 Aggro-only mode

In aggro-only mode (used by the immediate room-entry hook):

- Hostile reactions ARE evaluated, cached, dispatched.
- Any other reaction is NOT cached, NOT dispatched. (Leaving it
  uncached preserves the option for the deferred call to evaluate
  and dispatch it normally.)

### 5.5 Dispatch

On a fresh hostile reaction the system emits a `mob aggro` event
carrying attacker id, target id, mob's location, and mob's name.
The combat feature's engagement listener subscribes and calls
engage; engagement's own duplicate guard makes repeated hostile
emissions idempotent.

On a fresh `wary` or `friendly` reaction the system emits a
correspondingly-named disposition event with the mob id, player
id, and location. Unknown reaction strings emit nothing.

Reaction state is only updated when a fresh reaction is being
dispatched. This means a `friendly` mob does not re-greet the same
player every tick.

**Acceptance criteria**

- [ ] Disposition rules evaluate in declaration order; first match
      wins; default applies when no rule matches.
- [ ] A mob with static base disposition `hostile` is always
      hostile regardless of rule outcomes.
- [ ] The per-tick dedup cache prevents repeat evaluations within
      a single tick.
- [ ] The per-room reaction state suppresses repeat non-hostile
      dispatches.
- [ ] Leaving a room clears the reaction state for the moving
      player.
- [ ] Aggro-only mode only dispatches hostile reactions and does
      not cache anything else.
- [ ] Hostile dispatch emits a `mob aggro` event the combat
      feature can consume.

---

## 6. Mob commands and loot

### 6.1 Mob command verbs

Behavior handlers and scripts express mob actions as **command
strings**: a verb plus arguments, exactly as a player would type
them. The mob command registry lets the engine and packs register
verb handlers keyed by lower-cased verb. A registration carries:

- A handler that receives a mob context and the argument text.
- An optional GMCP channel name; when present, the registry emits
  a `communication message` event after the handler runs, carrying
  the channel, sender name, sender id, source kind (`mob`), the
  argument text (optionally prepended with the sender's name), and
  the mob's location.
- A `prepend sender` flag controlling whether the GMCP text is
  `text` or `senderName text`.

Dispatch resolves the verb from the front of the command string,
looks up the registration, and invokes the handler inside a
try/catch. Handler exceptions are logged and do NOT raise the GMCP
event. Unknown verbs are logged at debug level and otherwise
ignored — mob commands are not a hard contract.

### 6.2 Delay-queued commands

Behavior handlers may need to issue a sequence (e.g. "say hello,
wait 2 seconds, say world"). The mob command queue:

- Accepts `(entityId, commandStr, delaySeconds)` and schedules the
  command for a future tick.
- **Chains delays per mob.** If the mob already has a pending
  command scheduled in the future, the new command's fire tick is
  computed from that future tick, not from now. This makes
  `enqueue(say A, 0); enqueue(say B, 2)` always produce A
  immediately followed by B 2 seconds later, regardless of how
  quickly the second enqueue happens.
- Processes the queue once per tick: every entry whose fire tick
  is ≤ current tick is routed through the command router on
  behalf of the mob and removed from the queue.
- Drops scheduled commands silently if the mob has been removed
  before fire time (no router call, no error).
- Catches and logs router exceptions per entry so one broken
  command does not affect others scheduled for the same tick.

### 6.3 Loot generation at spawn

Loot resolution runs during spawn (§3.1 step 8), not on death.
This means the mob carries its loot in its contents from the
moment it appears, and any feature interested in what a mob is
*about* to drop can inspect the contents before the kill.

Resolution against a loot table id produces a list of item-
template ids:

1. **Guaranteed pool.** For each guaranteed entry, append the
   item id `count` times.
2. **Weighted pool.** Roll `poolRolls` independent weighted
   selections from the pool. Each selection picks an entry with
   probability proportional to its weight.
3. **Rare bonus.** If a rare-bonus block exists, roll one chance
   roll; on success, take one additional weighted selection from
   the rare-bonus pool.

The spawn step then instantiates each item id, adds it to the
mob's contents, and tracks each item.

After loot generation the system MUST emit a "mob loot generated"
event with the source mob, room id, template id, and count.

How the items leave the mob (drop-on-death, pickpocket, give) is
not specified here.

**Acceptance criteria**

- [ ] Mob command dispatch resolves verb case-insensitively.
- [ ] Unknown verbs do not raise errors and do not emit
      communication events.
- [ ] Handler exceptions suppress the communication event.
- [ ] Delay-queued commands chain from any future scheduled tick
      for the same mob.
- [ ] Removed mobs' scheduled commands are dropped silently.
- [ ] Loot is generated at spawn time and lives in the mob's
      contents.
- [ ] Guaranteed entries are emitted before pool rolls; rare-bonus
      runs last.
- [ ] Pool rolls and rare-bonus rolls are independent.
- [ ] A "mob loot generated" event is emitted with template id and
      count after generation.

---

## 7. Observable events

The features publish at least these events.

| Event | When |
|---|---|
| mob spawned | a new mob entity was placed in a room (§3.1) |
| mob loot generated | loot was rolled and placed in a mob's contents (§6.3) |
| area tick | an area's reset cadence fired (§3.7) |
| mob AI tick | a mob was considered in an AI tick (§4.3) |
| mob aggro | a fresh hostile disposition was dispatched (§5.5) |
| mob disposition wary | a fresh wary reaction was dispatched (§5.5) |
| mob disposition friendly | a fresh friendly reaction was dispatched (§5.5) |
| communication message | a mob command issued chat-like output (§6.1) |

The features themselves do not consume these events; observers
(combat, quests, telemetry, UI) do.

**Acceptance criteria**

- [ ] Every state transition in §3-§6 emits exactly the listed
      event with the documented payload.
- [ ] Spawn-time events never reference an entity before it has
      been tracked by the world.

---

## 8. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| Set of registered behavior names and handlers | §4.2 |
| Default idle / battle chance and intervals | §2.2 |
| Default ability proficiency for templates that don't override | §3.4 |
| Default and per-area reset interval | §3.5, §3.7 |
| Occupied-area modifier (and per-area override) | §3.7 |
| Convention for deriving area id from room id | §4.1 |
| Reaction keywords and their corresponding event names | §5.1, §5.5 |
| Loot table content | §6.3 |
| Mob command verb registrations | §6.1 |

---

## 9. Open questions / future work

- **Ability proficiency engine-default.** Spawn falls back to a
  hardcoded numeric default when neither per-ability nor template-
  default proficiency is set. Move it to config.
- **Tracking durability across restart.** Spawn tracking is
  in-memory only; after a restart, the first area reset will
  oversee an empty world and spawn the full count even if mobs
  "should" persist. Persistent worlds need a different design.
- **Overpopulation handling.** If admin commands or scripted
  spawns push a rule's count above its target, the area reset
  ignores the excess. Whether to despawn the extras is a policy
  question.
- **Hardcoded area-id convention.** Today the AI manager parses
  the area id from a room id by splitting on a colon. Treat this
  as a property of the room (`room.area`) and remove the parsing,
  so room ids are not stringly-typed.
- **Reaction extensibility.** The dispatcher hardcodes the event
  name for each canonical reaction. A registration-driven mapping
  would let packs define new reactions without engine changes.
- **Behavior handler isolation.** Handler exceptions are caught
  but the exception does not feed back to the AI manager beyond
  logging. A circuit-breaker (disable a misbehaving handler after
  N exceptions) would be a useful operability primitive.
- **Per-mob disposition state vs per-player.** Reaction state is
  keyed `(mob, player)`. When a mob is despawned, its entries
  linger until the player moves. A despawn-cleanup hook would be
  tidier.
- **Rare-swap composition.** Rare-swap is a single alternate
  template at a single chance. A weighted list of alternates would
  let packs author rarer cascades.

---

<!-- Generated: 2026-05-21 · Scope: MobTemplate + SpawnManager + AreaTickService + MobAIManager + DispositionEvaluator + MobCommandRegistry/Queue + LootTableResolver + MobStatDerivation · Spec style: narrative + acceptance criteria · Detail level: behavior only -->

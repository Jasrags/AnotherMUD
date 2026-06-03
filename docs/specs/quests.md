# Quests — Feature Specification

**Status:** Draft · **Scope:** Quest definitions, acceptance, stage and
objective progression, rewards on completion, abandonment,
persistence, auto-tracking against world events, and quest markers
for renderers · **Audience:** Anyone reimplementing or porting this
feature in any language.

This document describes *what* the quests feature must do, not *how*
to implement it. Specific quest content, reward amounts, banner
layout, persistence file format, and the default active-cap value
are policy and live outside this spec.

---

## 1. Overview

The quests feature tracks content-defined goals that players accept,
work on by acting in the world, and complete to receive rewards. It
maintains per-player quest state, observes world events to advance
objectives automatically, runs optional content scripts at lifecycle
hooks, dispatches rewards through replaceable services, and persists
state per player.

Quests are pure content. The engine has no built-in quests; the
registry is empty until content packs populate it. Every system the
quest feature depends on for rewards (XP, gold, items, abilities) is
expressed as a small interface so it can be replaced or stubbed:
quests work without those services, they just grant nothing.

### Core concepts

- **Quest definition** — content-defined record carrying a stable
  id, display name, classification (e.g. main / side), an optional
  giver template id, repeatability and abandonability flags, a
  prerequisite block, an ordered list of stages, a reward block, a
  secret flag, an optional script reference, and an optional pack
  directory (used by scripts to resolve resources).
- **Stage** — an ordered milestone within a quest, carrying a
  description, an optional hint, and a list of objectives. A quest
  advances stage-by-stage; objectives within a stage are completed
  in parallel.
- **Objective** — a single trackable goal: an id, a type keyword,
  a target string (mob template id, item template id, room id),
  an optional npc target (for deliver), a required count (default
  1), and a description.
- **Active quest** — a runtime record of a quest the player is
  working on: quest id, current stage index, and per-objective
  progress.
- **Quest state** — a per-player record carrying the list of
  active quests and the set of completed quest ids.
- **Marker** — a renderer-facing flag indicating an entity in the
  player's view is relevant to one of the player's active quests
  (a quest giver, a delivery target, a collect target).

### Goals

1. Register quest definitions, normalize their objective ids on
   load, and expose lookup by id.
2. Accept quests against per-player state with a documented set of
   failure modes and prerequisite checks.
3. Advance objectives by id with progress clamping and emit a
   structured event on every change.
4. Advance a stage automatically when all its objectives complete;
   complete a quest when no further stages exist.
5. Dispatch a quest's rewards on completion through replaceable
   services so the feature has no hard dependency on XP, currency,
   abilities, or items existing.
6. Observe a fixed set of world events and auto-advance objectives
   of matching type, so most content does not need to call into
   the quest service explicitly.
7. Persist per-player state on every change, load it on player
   login, and filter quests that no longer exist on the next load.
8. Expose marker queries so renderers can highlight quest-relevant
   entities without crawling per-player state.

### Non-goals

- Quest journals, UI rendering, or the `quests` / `accept` /
  `abandon` commands themselves — those belong to the commands
  feature, which calls into the operations defined here.
- Branching quest narratives, choices that mutate other quests, or
  cross-character quest state. The model is a linear stage list
  per quest, per player.
- Currency, XP, item, or ability subsystems themselves. The reward
  dispatcher delegates to them through interfaces.
- The script runtime that executes the optional lifecycle hooks
  (covered by the scripting / pack feature).
- Time-based or random quest expirations.

---

## 2. Quest definitions

### 2.1 Registration

Quest definitions register into a single registry keyed by stable
quest id. The registry MUST expose:

- Register or replace a definition by id.
- Look up by id.
- Enumerate all definitions.
- Load a directory tree of content files. (Optional convenience:
  the registry may walk pack directories for quest files and
  deserialize them. The exact format is content's choice; this
  spec only requires that the registry can be populated.)

### 2.2 Objective id normalization

Objective ids are content-provided but MAY be missing. On
registration (or on the file-load convenience), the registry MUST
ensure every objective carries an id, generating one from the
stage id, the objective type, and the objective's position in the
stage when absent. Generated ids MUST be stable across reloads of
the same content.

This guarantees that progress records — which key on objective id
— survive content edits that don't change semantics (e.g.
reordering descriptions, fixing typos).

### 2.3 Required fields

A definition MUST carry:

- A stable id.
- A non-empty list of stages, each with at least one objective.
- A reward block (may be empty — see §5).

A definition MAY carry:

- A display name and a classification keyword (e.g. main / side /
  daily).
- A giver template id (used by markers, by the giver-interaction
  surface — see §3.5 — and by certain content flows).
- An offer string: the giver's pitch, surfaced when a player
  interacts with the giver to discover the quest (§3.5). Optional;
  a renderer may fall back to the first stage's description.
- A turn-in flag. When set, completing the final objective does
  NOT grant the reward; instead the quest parks awaiting turn-in
  and the reward is claimed by returning to the giver (§4.3).
  Default: not set (rewards auto-grant on completion).
- Repeatable, abandonable, and secret flags. Defaults: not
  repeatable, abandonable, not secret.
- A prerequisite block (§3.2). Defaults: no prerequisites.
- A script reference and a pack directory pointer (used by the
  scripting feature to resolve hook handlers).

**Acceptance criteria**

- [ ] Definitions register by id; later registrations replace
      earlier ones.
- [ ] Objective ids are generated when absent and remain stable
      across reloads of the same content.
- [ ] Missing reward, prereq, repeatable, abandonable, and secret
      values resolve to sensible defaults without error.

---

## 3. Accepting a quest

### 3.1 The acceptance operation

Accepting a quest takes a player entity, a quest id, and an
optional `silent` flag. It returns a structured result enumerating
the possible outcomes:

- **Accepted** — the quest is now active on the player.
- **Not found** — no definition exists for the quest id.
- **Already active** — the player has this quest active.
- **Already completed** — the player has completed this quest and
  it is not repeatable.
- **Prerequisite not met** — the player fails one or more
  prerequisite checks (§3.2).
- **Cap reached** — the player is at the active-quest cap and the
  quest is abandonable (§3.3).

On Accepted the system MUST:

1. Build an active-quest record for the first stage, initializing
   every objective to `(current=0, required=objective.count)`.
2. Append the record to the player's active list.
3. Build a banner (§3.4) for player-visible feedback unless the
   quest is secret or the caller passed silent (and the optional
   script `on_granted` hook did not suppress).
4. Emit a `quest started` event carrying the quest id and, when
   present, the banner text.
5. Persist the player's quest state.

The first-stage objectives are seeded from the stage definition;
progress starts at zero for each.

### 3.2 Prerequisite checks

A quest's prerequisite block defines four optional gates:

- **Minimum level.** The player's level on the `main` progression
  track is read from the entity's level map (default 1 when
  absent). The player's level MUST be at least the minimum.
- **Class.** When set, the player's class property MUST equal the
  required class string.
- **Quests completed.** Every named quest id MUST be present in
  the player's completed set.
- **Quests not completed.** None of the named quest ids may be
  present in the player's completed set.

All present gates must pass for the prerequisite check to succeed.
Absent gates are no-ops. The check operates on the snapshot of
state at acceptance time; concurrent state changes between check
and append are not specified here.

### 3.3 Active-quest cap

The system enforces a configured cap on the player's count of
*abandonable* active quests. Non-abandonable quests do not count
toward the cap, and the cap is not checked when the quest being
accepted is itself non-abandonable. This lets content force-grant
plot-critical quests without filling the player's slate.

Counting MUST inspect each currently-active quest's *definition*
to determine abandonability. An active quest whose definition is
missing (e.g. content removed since last save) counts as
abandonable for the purposes of cap math.

### 3.4 Banner

The banner is a player-visible block of text generated server-
side. The feature MUST produce a banner when all of:

- The quest is not secret.
- The caller did not pass silent.
- The optional script hook `on granted` (when present) returned
  false / did not suppress.

When suppressed, the banner is not generated and is not included
in the event payload. The script hook MUST be called whether or
not a banner is generated, so content that relies on the hook for
side effects (sending custom messages, starting an effect) works
regardless of banner suppression rules.

The banner contents — title, stage description, objective list
with progress, footer — are content-formatted; this spec requires
only that:

- The banner reflects the *initial* state of the active quest
  (stage 0, all objectives at zero progress).
- The banner identifies the quest by display name and
  classification.
- The banner is included verbatim in the `quest started` event
  payload when generated.

**Acceptance criteria**

- [ ] All six acceptance outcomes (Accepted, NotFound,
      AlreadyActive, AlreadyCompleted, PrereqNotMet, CapReached)
      are distinguishable to callers.
- [ ] Acceptance constructs an active record at stage 0 with
      every objective at zero progress.
- [ ] Prerequisite checks evaluate min level, class, quests
      completed, quests not completed.
- [ ] Cap check counts only abandonable active quests and is
      bypassed for non-abandonable quests being accepted.
- [ ] Banner suppression honors secret, silent, and script hook.
- [ ] `quest started` is emitted exactly once on a successful
      acceptance.
- [ ] State is persisted after acceptance.

### 3.5 Giver interaction (discovery and turn-in)

Markers (§8) hint that a giver is relevant, but a player also needs
a way to *learn what a giver offers* without already knowing the
quest's name, and to *claim* a turn-in quest's reward. A single
giver-interaction operation serves both, keyed on a giver the
player is co-located with:

- **Offers.** Query the non-secret quests the giver can offer this
  player right now — those the player is eligible to accept (not
  already active, not completed unless repeatable, prerequisites
  met). The active-quest cap is NOT applied to the offer list; an
  over-cap player still sees the offer and the cap is enforced when
  they accept. Each offer carries the quest's display name and its
  offer pitch (§2.3), so the surface can present it and tell the
  player how to accept.
- **Turn-in.** For each of the player's awaiting-turn-in quests
  whose giver is this NPC, run turn-in (§4.3a) — the completion
  banner and reward dispatch follow the normal completion path.

How this is surfaced to the player (a `talk`/`ask` verb, dialogue,
etc.) is presentation policy and outside this spec; the required
behavior is that interacting with a giver both reveals its eligible
offers and claims any of the player's turn-ins due at that giver.

---

## 4. Progression

### 4.1 Advance one objective

`Advance objective(playerId, questId, objectiveId, amount)` is the
primitive for moving progress forward on a single objective.

1. Load the player's quest state. If absent, no-op.
2. Find the matching active quest. If absent, no-op.
3. Find the matching objective entry. If absent or already
   complete, no-op.
4. Clamp `current = min(current + amount, required)`.
5. Call the script hook `on objective advanced` if registered.
6. Emit `quest objective advanced` with quest id, objective id,
   new current, and required.
7. If any objective in the stage is still incomplete, persist and
   return.
8. Otherwise, if a next stage exists, advance the stage (§4.2)
   and persist.
9. Otherwise complete the quest (§4.3).

Incrementing an already-complete objective is a no-op even if the
caller passed a non-zero amount.

### 4.2 Advance stage

To advance from a completed stage to the next:

1. Set the active-quest's stage index to the next index.
2. Build fresh objective entries from the next stage's
   definitions, each at zero progress.
3. Call the script hook `on stage advanced`.
4. Emit `quest stage advanced` with quest id and the new stage
   index.

Stage advancement does NOT emit per-objective `advanced` events
for the new objectives (they start at zero, which is the implied
initial state).

### 4.3 Complete a quest

When the final stage's objectives all complete, the definition's
**turn-in flag** decides what happens:

- **Auto-grant (default, flag unset):** the quest completes
  immediately — proceed with the completion steps below.
- **Turn-in (flag set):** the quest does NOT complete yet. Instead
  it stays on the active list, marked **awaiting turn-in**, with no
  reward dispatched. The system emits `quest ready to turn in`
  (carrying the quest id and the giver template id) and persists.
  The reward is granted later by the **turn-in operation** (§4.3a),
  triggered when the player returns to the giver. A quest whose
  definition can't be resolved cannot declare turn-in, so it
  auto-grants.

On completion (auto-grant, or turn-in once claimed):

1. Remove the active record from the player's active list.
2. Append the quest id to the player's completed set. Completion
   is idempotent against the completed set — re-completing a
   repeatable quest may produce duplicate entries, and callers
   that care about "did this player complete X" should use a
   set-style lookup, not a list count.
3. Resolve the player entity from the internal player cache; if
   present, hand the quest's reward block to the reward
   dispatcher (§5).
4. Call the script hook `on completed`.
5. Emit `quest completed` carrying the quest id, the reward
   amounts (XP, gold), and the reward lists (items, abilities,
   class unlock).
6. Persist state.

The player cache is populated implicitly when the player accepts
a quest (so the cache key matches the entity id used during
play). If the cache miss happens at completion time (e.g. quest
completed by a service event before the player ever accepted
anything from this process), reward dispatch is silently skipped
but the event is still emitted.

### 4.3a Turn-in

`Turn in(player, questId)` claims an awaiting-turn-in quest's
reward and completes it. It MUST:

1. Fail (not found) if the quest definition is unknown.
2. Fail (not active) if the player has no active record for it.
3. Fail (not ready) if the record is not marked awaiting turn-in
   (objectives still outstanding, or it is an auto-grant quest that
   never parks here).
4. Otherwise run the completion steps of §4.3 (dispatch reward,
   record completed, emit `quest completed`, persist).

The operation is **room-agnostic**: it does NOT itself verify the
player is standing with the giver. The interaction surface (§3.5)
performs that check — it only calls turn-in for a quest whose giver
the player is currently talking to. This keeps the quest service
free of world/room knowledge.

### 4.4 Advance by predicate

`Advance matching objectives(playerId, type, predicate)` is the
collective form used by the watcher (§7):

1. Load state. No-op if absent.
2. For each active quest:
   - Skip if the definition is missing.
   - For each objective in the current stage whose `type` matches
     AND whose data satisfies `predicate(objectiveDefinition)`,
     call `advance objective` with amount 1.

The function iterates a snapshot of the active list so that side
effects of advancement (stage advance, quest completion) do not
disturb iteration.

### 4.5 Abandonment

A player may abandon an active quest. The system MUST:

1. Look up the definition. Fail silently if missing or if the
   quest is not abandonable.
2. Load the player's state. Fail silently if absent.
3. Remove every active entry for this quest id.
4. Emit `quest abandoned` with the quest id.
5. Persist.

Abandonment does NOT roll back any partial side effects (e.g.
items the player collected for a collect objective remain in the
inventory). Re-accepting the quest later starts fresh from stage
0; partial progress is lost.

**Acceptance criteria**

- [ ] Advance is a no-op when state, quest, or objective is
      absent; or when the objective is already complete.
- [ ] Progress is clamped at `required`.
- [ ] Stage advancement seeds the new stage's objectives at zero.
- [ ] Quest completion only fires when all objectives in the
      final stage are complete.
- [ ] Reward dispatch is skipped silently on cache miss but the
      `quest completed` event still fires.
- [ ] Abandonment is silently rejected for non-abandonable
      quests.
- [ ] Persistence is invoked on every state-mutating path.

---

## 5. Rewards

### 5.1 Reward block

A reward block carries:

- An XP amount.
- A gold amount.
- A list of item template ids.
- A list of ability ids to teach.
- An optional class unlock id.
- An optional race unlock id.

Any field may be absent or zero / empty.

### 5.2 Dispatch pipeline

The reward dispatcher applies the reward block to the player in
this order, each step independent:

1. If XP > 0, grant experience on the `main` progression track
   with source `quest`.
2. If gold > 0, add gold with reason `quest reward`.
3. For each ability id, teach the player the ability at initial
   proficiency 1.
4. If class unlock is set, set the player's class property.
5. If race unlock is set, set the player's race property.
6. For each item template id, create an item and pick it up
   silently into the player's inventory. Missing templates are
   skipped silently.

Steps that target missing or no-op services (see §5.3) succeed
silently without effect.

### 5.3 Replaceable service interfaces

The dispatcher does not call into the XP, currency, ability, or
item subsystems directly. It calls four small interfaces:

- **Progression service** — `GrantExperience(entityId, amount,
  trackName, source)`.
- **Currency service** — `AddGold(entity, delta, reason)`.
- **Proficiency service** — `Learn(entityId, abilityId,
  initialProficiency)`.
- **Item registry / inventory service** — `CreateItem(templateId)`
  and `PickUp(entity, item, silent)`.

Each interface MUST have a no-op default registered when no real
implementation is wired up. This is what lets the quests feature
ship without forcing the rest of the engine to exist — for tests,
for embedding, and for content that runs without progression.

### 5.4 Class / race unlock

Class and race unlocks are SETTERS, not predicates. Setting a
class unlock changes the player's current class property
unconditionally. There is no "you unlocked class X, would you
like to switch" intermediate flow — that, if desired, lives in
content (an `on completed` script hook can route the unlock
through a class-change command instead).

**Acceptance criteria**

- [ ] Reward dispatch handles missing fields and empty lists.
- [ ] Each step is independent and silently succeeds when its
      service is the null implementation.
- [ ] Item rewards are picked up silently (no per-item event
      noise on completion).
- [ ] Class/race unlock writes the entity property directly.

---

## 6. Persistence

### 6.1 Per-player file

Each player's quest state is persisted as a per-player file under
the configured save path, conventionally under `players/<lowercase
name>/quests.<format>`. The exact format is implementation
choice; YAML with snake_case fields is the current implementation.

### 6.2 Save

Every state-mutating operation (§3.1 step 5, §4.1 step 7, §4.2
implicit, §4.3 step 6, §4.5 step 5) persists by writing the
player's full state file. The save is synchronous and overwrites
any prior file.

### 6.3 Load on login

The persistence service subscribes to a `player login` event
carrying the player's id and name. On the event:

1. Read the player's state file. Return without effect if missing
   or unreadable (errors are logged, not propagated).
2. Apply orphan filtering (§6.4).
3. Set the loaded state into the per-player state repository,
   keyed by the player's id from the event.

This means the player's quest state is hydrated AFTER login
completes (the login flow does not gate on quest load) and is
available to subsequent commands and watcher events. Operations
issued in the brief gap between login and load see the player
with no state.

### 6.4 Orphan filtering

A loaded state file may reference quest ids no longer present in
the registry (content removed or renamed since the file was
written). On load, the persistence service MUST filter the
loaded state to remove orphans:

- Active entries whose quest id is unknown are dropped.
- Completed entries whose quest id is unknown are dropped.

**Exception:** when the registry is empty (no content loaded
yet), orphan filtering MUST be skipped. An empty registry is
typically a startup-order issue, not a content removal, and
filtering against it would wipe every player's history.

**Acceptance criteria**

- [ ] Every state-mutating operation writes the file.
- [ ] Load is triggered by the `player login` event and is
      side-effect-only on error (no propagation).
- [ ] Orphan filter applies when the registry has entries.
- [ ] Orphan filter is skipped when the registry is empty.

---

## 7. Auto-tracking watcher

The objective watcher subscribes to a fixed set of world events
and translates each into `advance matching objectives` calls
against the player it identifies as the source. The watcher MUST
NOT modify quest state directly; it goes through the quest
service so events emit correctly.

### 7.1 Subscribed events and mapping

| World event | Objective type | Match predicate |
|---|---|---|
| mob killed | `kill` | objective target equals the killed mob's template id |
| item picked up | `collect` | objective target equals the picked-up item's template id |
| item given | `deliver` | objective target equals the item template id AND objective npc equals the recipient's template id |
| player moved | `visit` | objective target equals the destination room id |

The four canonical objective types — `kill`, `collect`, `deliver`,
`visit` — are the engine-recognized auto-tracked types. Content
MAY use additional type strings, but those must be advanced
explicitly via the service's `advance objective` call (typically
from a script hook); the watcher will ignore them.

### 7.2 Side-channel advancement

The item-pickup event handler ALSO honors two side channels for
explicit, content-driven advancement and grant:

- **`quest_grant` on the item template.** If the picked-up item's
  template carries a `quest_grant` property whose value is a
  quest id, the watcher MUST attempt to accept that quest for
  the picker. Acceptance failures (already active, already
  completed, prereq, cap) are silent.
- **`quest_advance` on the event payload.** If the event data
  carries a `quest_advance` string of the form
  `<packId>:<questId>:<objectiveId>`, the watcher MUST parse it
  and advance the named objective by 1. Malformed strings are
  silently ignored.

### 7.3 Room grant on entry

The player-moved handler ALSO checks the destination room for a
`quest_grant` property. When set, the watcher MUST attempt to
accept the named quest, with the same silent-failure semantics
as §7.2.

### 7.4 Source / target identity

For each event, the source player is taken from the event's
source entity id. Events without a source entity id are ignored.
Watcher logic MUST tolerate world entities going missing between
event emission and event handling (e.g. a kill event for a mob
the world has already cleaned up); missing data in the event
payload is silently ignored.

**Acceptance criteria**

- [ ] The watcher subscribes to exactly the four world events
      listed in §7.1 and translates them per the table.
- [ ] Custom (non-canonical) objective types are advanced only
      by explicit calls, not by the watcher.
- [ ] Item-pickup honors `quest_grant` on the template and
      `quest_advance` on the event payload.
- [ ] Player-moved honors `quest_grant` on the destination
      room.
- [ ] Missing payload fields and missing world entities do not
      raise from watcher handlers.

---

## 8. Markers

### 8.1 Marker queries

Renderers may ask:

- "Does the player have an active quest marker for template id
  T?" — used by entity-name decorators (e.g. `!` next to a quest
  giver).
- "For these entities in the room, which carry a marker?" — used
  by room renderers to highlight relevant entities in bulk.

### 8.2 Marker eligibility

An entity carries a marker iff some active quest definition (not
the active record alone) treats the entity's template id as
relevant. Specifically:

- The definition's giver template id matches. Even when the quest
  is already accepted, the giver remains marker-eligible (so the
  player can find the giver to follow up).
- An objective in the player's CURRENT stage is `deliver` and the
  objective's npc target matches the entity's template id.
- An objective in the player's current stage is `collect` and the
  objective's item target matches the entity's template id.

Kill objectives MUST NOT produce markers. Marking every aggro mob
with a quest icon was found to be too noisy.

### 8.3 Secret quests

Secret quests MUST NOT contribute any markers. A secret quest's
giver, deliver targets, and collect targets are invisible to the
marker query.

**Acceptance criteria**

- [ ] Marker eligibility is per-definition (giver) and per
      current-stage (deliver / collect), not per completed
      history.
- [ ] Kill objectives are excluded from markers.
- [ ] Secret quests contribute no markers.
- [ ] Bulk room query returns at most one marker per entity (the
      first matching active quest wins).

---

## 9. Observable events

The feature publishes at least these events.

| Event | When |
|---|---|
| quest started | a quest was accepted (§3.1) |
| quest objective advanced | an objective's progress changed (§4.1) |
| quest stage advanced | the active stage moved forward (§4.2) |
| quest ready to turn in | a turn-in quest's final objectives completed; reward pending return to the giver (§4.3) |
| quest completed | the quest completed and its reward dispatched — immediately for auto-grant quests, on turn-in for turn-in quests (§4.3 / §4.3a) |
| quest abandoned | a player abandoned a quest (§4.5) |

The quest watcher consumes external events (see §7.1) but does
not emit its own — it routes through the service so the service's
events are the canonical signal.

**Acceptance criteria**

- [ ] Each transition in §3-§4 emits exactly the listed event
      with the documented payload.
- [ ] Watcher-driven advancement produces the same events as
      explicit advancement.

---

## 10. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| Active-quest cap (abandonable only) | §3.3 |
| Quest content | §2 |
| Banner text and formatting | §3.4 |
| Persistence path | §6.1 |
| Persistence format (YAML, JSON, binary, …) | §6.1 |
| Identity of the `main` progression track | §3.2, §5.2 |
| Watcher event names | §7.1 |
| Canonical auto-tracked objective types | §7.1 |
| Reserved property keys (`quest_grant`, `quest_advance`) | §7.2, §7.3 |
| Class / race property keys | §3.2, §5.4 |

---

## 11. Open questions / future work

- **Duplicate completion entries on repeatable quests.** A
  repeatable quest, completed twice, appends to the completed
  list twice. Switching the completed set to a true set (or
  guarding the append) would normalize this — but might also
  break content that uses the list count as "how many times have
  I done X".
- **No reward rollback on abandonment.** Items collected for a
  collect objective stay with the player after abandon. Content
  that wants strict semantics needs to handle this in scripts.
- **Hardcoded `main` track for quest XP.** The dispatcher grants
  XP only on the `main` track. Quests targeting a sub-track
  (e.g. a crafting track) cannot express it through the reward
  block.
- **No multi-objective predicates.** Objectives match by type +
  target + npc. Compound conditions (e.g. "kill mob X with a
  fire weapon") need a custom type and an explicit script
  advance.
- **No per-quest objective ordering.** All objectives in a stage
  advance independently. Sequential within a stage requires
  splitting into multiple stages.
- **Pack id format leak.** The `quest_advance` side channel
  parses an id of the form `<packId>:<questId>:<objectiveId>`,
  baking the pack-id convention into a property value. If the
  pack id convention changes, this string also has to.
- **Login load race.** Quest state is loaded on a login event
  that fires after the login flow completes. Commands issued in
  the gap see empty state. A synchronous load before the player
  becomes interactive would close the window.
- **Cap math counts abandonable-by-definition.** A quest's
  definition can change `abandonable` between save and load. The
  cap math reads the current definition, which is the right
  default but means a quest the player accepted as
  non-abandonable starts counting toward the cap if the content
  flips the flag.
- **Marker key on giver vs current target.** Givers stay marker-
  eligible after acceptance. That helps the player retrace, but
  conflates "go talk to him" (turn-in) and "you have a quest from
  him" (followup-available). A two-tier marker system would let
  renderers distinguish.
- **No event for orphan filter.** Loading a save and silently
  dropping orphan quest entries is invisible to players. An
  audit/log path or a one-off notification on next login would
  be friendlier.

---

<!-- Generated: 2026-05-21 · Scope: QuestRegistry + QuestService + QuestStateRepository + QuestObjectiveWatcher + QuestRewardDispatcher + QuestMarkerService + QuestPersistenceService + IQuest* service interfaces · Spec style: narrative + acceptance criteria · Detail level: behavior only -->

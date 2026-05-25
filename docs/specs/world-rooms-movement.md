# World, Rooms, and Movement — Feature Specification

**Status:** Draft · **Scope:** Room graph, area registry, the entity
tracking layer (including tag indexing), entity movement between
rooms, doors, temporary keyword exits ("portals"), and area-level
ambient effects (weather, time messaging) · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the world feature must do, not *how*
to implement it. Specific direction lists, terrain types, weather
states, transition weights, and reset cadences are policy and live
outside this spec.

---

## 1. Overview

The world feature is the spatial substrate every other feature
stands on. It owns:

- The directed graph of **rooms** and the **areas** that group them.
- The mapping from runtime entity ids to live entity references,
  with secondary indices by tag and by type.
- Entity placement: which room an entity is in, and the
  atomic move operation that takes an entity from one room to
  another along a directional exit.
- **Doors**: per-exit state with paired reverse-side synchronization.
- **Temporary keyword exits** ("portals"): dynamically-created
  exits with bounded lifetimes that auto-expire on the area's tick
  clock.
- **Weather and time ambience**: area-scoped state changes
  delivered as room messages, gated by terrain.

### Core concepts

- **Room** — a node in the world graph. Identified by a stable
  string id. Carries name, description, an exit map keyed by
  cardinal/vertical direction, a separate keyword-exit map, an
  entity list, tags, free-form properties, alignment access
  policy, an area assignment, and weather/time exposure flags.
- **Exit** — a directed edge from one room toward a target room
  id, optionally guarded by a door, optionally carrying conditions
  and a display name.
- **Door** — per-exit gate with name (keywords), closed/locked
  flags, optional key id, pickability, and reset defaults.
- **Area** — a content-defined group of rooms identified by a
  stable id. Carries level range, reset interval, occupied
  modifier, an optional weather zone, flags, and per-state
  override tables.
- **Direction** — a fixed enumeration of compass + vertical
  movement directions (cardinal north/south/east/west plus
  up/down), each with a canonical short form and a known opposite.
- **Entity tracking** — the world's live index of entities by id.
  An entity is "tracked" when it appears in the index regardless
  of whether it is currently placed in a room.
- **Tag index** — a per-tag set of tracked entities, maintained
  by the world by subscribing to entity tag-change notifications.
  Queries against the tag index are consistent across a tick (see
  §3.4).
- **Temporary exit (portal)** — a runtime-only keyword exit
  registered on a room with an expiry tick count. Stored
  out-of-band so it can be expired or cancelled in bulk.

### Goals

1. Provide a single graph the engine can read for "where is X" and
   "what's in room R" without ambiguity.
2. Make entity movement a single, observable operation that other
   features (mob AI, disposition, combat, quests) hook with
   confidence.
3. Make door state, including reverse-side state, automatically
   consistent.
4. Let content create transient keyword exits with cleanup
   guarantees.
5. Support area-wide ambience (weather, time) without coupling
   the world feature to the time-of-day or weather-roll
   subsystems — those are independent producers and the world
   subscribes.
6. Keep tag-indexed reads stable within a tick, even while other
   systems are mutating tags.

### Non-goals

- The Direction enum's exact membership. Cardinal + vertical is
  the engine's default but content/UI layers may extend it; this
  spec treats Direction as opaque except where it explicitly says
  otherwise.
- Movement *cost* (movement points / stamina). Cost gating lives
  outside this feature; the move primitive here is unconditional
  on resource availability.
- Pathfinding or routing. The world exposes the graph; navigation
  is a caller concern.
- Persistence of rooms / world state across restart. Room and
  exit definitions are reloaded from content; runtime mutations
  beyond entity placement and door state are not durable.
- Player input parsing (`north`, `n`, `open door`). The commands
  feature owns parsing; this feature defines the operations
  commands invoke.
- The clock that produces time-of-day events (hour change, period
  change). This feature subscribes to those events; the time
  feature owns them.

---

## 2. Rooms and areas

### 2.1 Room registry

The world maintains a single mutable registry of rooms keyed by
stable string id. Add/get/remove/iterate are the only required
operations. Adding a room with an existing id replaces the prior
entry.

Room ids are content-defined; this feature treats them as opaque
strings. (Other features impose conventions on top — e.g. the AI
manager derives an area name from a room id by splitting on a
colon. Such conventions belong to those features, not to this
one. See §10 for the standing question.)

### 2.2 Room shape

A room MUST carry:

- A stable string id.
- A display name and a description (both mutable, both rendered
  to players).
- An exit map keyed by **Direction**, mapping to an Exit value.
- A keyword-exit map keyed by string (case-insensitive),
  separate from the direction map. Used by temporary exits and
  any content that needs non-cardinal movement keywords (e.g.
  `gate`, `portal`, `down the well`).
- An ordered list of entities currently placed in the room.
  Order is preservation-of-arrival; it is observable and stable
  for renderers but carries no other semantics.
- A string tag set and a free-form property bag (string-keyed,
  typed read).
- An optional alignment access range with optional block message
  (used by movement gates layered on top — see §3.5).
- An optional area assignment (string area id).
- Two booleans: weather-exposed and time-exposed (see §6).
- A weather-messages map and a time-messages map (per-room
  overrides — see §6.3).

A room MAY additionally declare:

- An ordered list of **boot-placed item template ids**. The pack
  loader spawns one instance per list entry (duplicate entries
  produce duplicate instances) and places each in the room during
  a post-pass that runs after every pack has been read, so
  cross-pack template references resolve regardless of load order.
  Ids follow the same qualification rule as exit targets: bare ids
  resolve against the current pack namespace, qualified ids
  (`other-pack:foo`) cross packs. An unknown template id is a
  fatal load error.

Boot-placed items are static fixtures from the world's perspective:
they have no respawn cadence and are not re-spawned if removed
during play (e.g. `get`'d, despite the typical `no_get` tag).
Mob spawning and area-reset cadence — when they land — own the
respawn surface; this list is the simple "place X at boot" path.

### 2.3 Entity placement

The world provides two primitive operations on a room:

- **Add entity to room.** MUST first detach the entity from any
  current container (item-in-container case), then set the
  entity's location room id to this room, then append to the
  entity list if not already present. Duplicate-add is a no-op
  on the entity list.
- **Remove entity from room.** MUST clear the entity's location
  room id on success. Removal of an entity not in the room is a
  no-op.

Both operations are synchronous and do not emit events here;
movement (§3) is the higher-level operation that emits events
and may compose these primitives.

### 2.4 Area registry

Areas register into a single registry keyed by stable area id.
An area definition carries:

- A stable id.
- A display name.
- A level-range pair (inclusive low and high) used by content
  for matchmaking and difficulty advice.
- A reset interval in ticks.
- An occupied-area modifier (a multiplier applied to reset
  interval when at least one player is present in the area —
  values above 1 slow resets in occupied areas, below 1 speed
  them up).
- An optional weather-zone id (§6).
- A string flag list.
- Per-state weather override tables and a per-period time
  override table (§6.3).

Areas are content; the world does not synthesize them. A room
may declare an area assignment that does not correspond to any
registered area without error — features that depend on area
metadata MUST tolerate that case.

**Acceptance criteria**

- [ ] Room ids are opaque to this feature; add/get/remove/iterate
      treat them as case-sensitive keys.
- [ ] A room exposes its entity list as a stable read-only view.
- [ ] Add-to-room detaches from any prior container first.
- [ ] Remove-from-room clears the entity's location room id.
- [ ] Area lookups by unknown id return none, not error.

---

## 3. Movement

### 3.1 The Direction enumeration

The engine recognizes a fixed Direction enumeration with at least
the cardinal directions and vertical up/down. Every Direction
value MUST have:

- A canonical full-word name (e.g. `north`).
- A short-form alias (e.g. `n`).
- A unique opposite (e.g. `north` ↔ `south`).
- A short single-character render form for compact display
  (e.g. `N`).

The engine MUST provide case-insensitive parsing from arbitrary
input to Direction (returning failure for non-matches) and the
opposite operation. Adding directions (e.g. diagonals) requires
content/engine coordination and is out of scope here.

### 3.2 Exits

An exit declares a target room id, an optional door, an optional
display name (used by temporary exits and content), and an
optional condition bag (content-defined gates; this feature does
not interpret them). A room may have at most one exit per
Direction. Multiple keyword exits per room are allowed (keyword
exits are tracked separately from direction exits).

### 3.3 The move primitive

The world exposes a single move primitive: *move entity E in
Direction D*. The operation MUST:

1. Read the entity's current location room id. If none, fail.
2. Look up the current room. If not registered, fail.
3. Look up the exit for Direction D. If none, fail.
4. If the exit has a door and the door is closed, emit a
   "door blocked" event with the source entity id, current room
   id, direction (short form), and door name, then fail.
5. Look up the target room. If not registered, fail.
6. Remove the entity from the current room.
7. Add the entity to the target room.
8. Return success.

Door-blocked is the ONLY failure mode in this list that emits an
event; all other failures are silent. Higher-level callers may
emit their own events on success (e.g. a "player moved" event
emitted by the command layer).

The move primitive is unconditional with respect to:

- Movement cost (movement points / stamina). Callers check and
  deduct.
- Door locks beyond the closed check (a closed-but-locked door
  is still just "closed" for movement purposes).
- Alignment access (§3.5).

This keeps the primitive composable. Multiple callers (player
commands, mob AI, scripted teleports, flee) use the same move
primitive on top of their own preconditions.

### 3.4 Tag-indexed reads during movement

While movement is in progress, other systems may query the world
for "entities with tag X" (mobs filtering by `aggro`, abilities
finding all `cleric` casters in a zone, etc.). The world MUST
keep these reads consistent within a tick.

Implementation contract:

- The world maintains two tag-index maps: a **read index** seen
  by queries, and a **write index** that absorbs mutations.
- When a tag is mutated for the first time in a tick, the write
  index entry for that tag is created by cloning the read
  entry, and the tag is marked dirty.
- Subsequent mutations within the same tick write to the
  (already-dirty) write entry.
- A swap operation, invoked by the engine at a well-defined
  boundary (e.g. top of every tick), promotes the write index
  to read and rebuilds the write index from the new read.
  Untouched tags remain stable across the swap.
- Removes that empty a tag's set MUST remove the entry from the
  write index but MUST NOT clear the dirty flag, so a
  re-add later in the same tick does not pull a stale read
  entry back.

The world reports the number of dirty tags and the total tag
count at each swap, so the engine can observe pressure on the
index.

### 3.5 Alignment access (cross-cutting)

A room may carry an alignment access range with an optional
custom block message. Movement itself does NOT check this; it is
checked by the command layer that invokes the move primitive.
This spec only requires that rooms expose the range and message
so callers can enforce it consistently.

**Acceptance criteria**

- [ ] The move primitive is the single point at which "entity
      changes room" happens.
- [ ] Door-blocked is the only emitter on a failed move.
- [ ] Door-blocked carries the actor id, current room, direction
      short form, and door name.
- [ ] Movement does not deduct cost, check locks beyond closed,
      or enforce alignment access.
- [ ] Tag-index queries within a tick see a consistent snapshot;
      mutations land in a side buffer.
- [ ] The swap operation makes mutations from the prior tick
      visible to the next tick's queries.

---

## 4. Entity tracking

The world maintains a runtime index of all entities by id.

### 4.1 Track / untrack

- **Track entity.** Add the entity to the id index; register the
  world as a tag observer on the entity; for each tag the entity
  currently carries, insert it into the write side of the tag
  index.
- **Untrack entity.** Remove from the id index; unregister as a
  tag observer; remove from the write side of the tag index for
  every tag the entity currently carries.

Tracking is orthogonal to room placement. An item carried inside
a container is tracked but is not in any room. A pending player
mid-character-creation may be tracked (or, in the current
implementation, available through a side index — see §4.2).

### 4.2 Get-by-id

A lookup by id MUST resolve in this order:

1. The tracked id index.
2. (Fallback) Scan all rooms for an entity with that id. If
   found, opportunistically add it to the tracked index and
   return.
3. (Fallback) A pending-player side index used during character
   creation. Mid-creation entities are not yet tracked but must
   still be resolvable by id.

The first fallback is a safety net for entities created without
explicit tracking; the second is required so character creation
can resolve `self` references before commit.

### 4.3 Get-by-tag and get-by-type

- **By tag.** Returns the read side of the tag index for the
  given tag (an empty set if absent). Sets returned MUST be
  read-only relative to internal state.
- **By type.** Filters the tracked id index by the entity's type
  string (case-insensitive). No secondary index is required.
- **In room.** Returns the room's entity list directly. (Rooms
  manage their own ordered list; the tag index does not
  duplicate it.)

**Acceptance criteria**

- [ ] Tracking sets up the tag observation; untracking tears it
      down.
- [ ] Get-by-id resolves in the order tracked → room scan →
      pending players; first hit wins.
- [ ] Get-by-tag returns the consistent read-side snapshot.
- [ ] Get-by-type filters on the entity's own type string.

---

## 5. Doors and temporary exits

### 5.1 Door state

A door carries a name (whose space-split tokens are also its
match keywords), a closed flag, a locked flag, an optional key
id, a pickable flag, a pick-difficulty number, and the default
closed/locked values used by reset (§5.4).

### 5.2 Operations

The world exposes Open, Close, Unlock, and Lock operations,
each taking the actor entity, the current room, and a direction.
Each operation:

1. Resolves the exit and its door. Fails silently if either is
   absent.
2. Checks the precondition appropriate to the operation:
   - Open: door is currently closed.
   - Close: door is currently open.
   - Unlock: door is currently locked.
   - Lock: door is closed AND not already locked.
   Fails silently if not met.
3. Mutates the local door state.
4. **Synchronizes the reverse side.** Looks up the target room's
   exit in the opposite direction; if that exit has a door,
   applies the same mutation there. Reverse-side absence is not
   an error (one-way doors are allowed).
5. Emits the corresponding event (`door opened`, `door closed`,
   `door unlocked`, `door locked`) with the room id, direction
   (short form), actor id, door name; the unlock/lock events
   also carry the door's key id.

The implementation MUST NOT itself check whether the actor holds
the key. Key-holder check is a query exposed to the command
layer (§5.3); whether a command requires a key for a given
operation is policy.

### 5.3 Queries

- **Can pass.** True iff the exit exists AND (it has no door OR
  the door is not closed). This is the boolean the move
  primitive uses indirectly via the closed check.
- **Get door.** Returns the door state for a (room, direction)
  pair, or none.
- **Has key.** Given an actor and a key id, returns true iff the
  actor's contents contain an item whose template id equals the
  key id (case-insensitive).

### 5.4 Reset

A reset restores a door to its `default closed` and `default
locked` flags. Per-area reset applies to every door in every
room whose id matches an area-prefix convention (`<area>:...`
or equals `<area>` for a singleton room). Reset MUST also sync
the reverse side.

The area-prefix convention is a known leak from the room id
convention (see §10) and is the operative one here.

### 5.5 Target resolution from text

Door operations accept a string input from the command layer
that may be a direction or a door keyword (with optional
ordinal). The world resolves it as follows:

1. **Direction parse.** If the input parses to a Direction and
   the current room has an exit in that direction, the
   resolution is that direction.
2. **Ordinal split.** If the input is `<integer>.<keyword>`,
   split into ordinal and keyword. Otherwise the whole input is
   the keyword and the ordinal is zero.
3. **Collect candidates.** Iterate exits in the current room and
   collect every direction whose exit has a door whose keyword
   set contains the input keyword (case-insensitive).
4. **Disambiguate.**
   - Zero candidates → none.
   - Ordinal zero AND multiple candidates → none (the input is
     ambiguous; the command layer reports it).
   - Ordinal zero AND one candidate → that direction.
   - Ordinal in range → the matching candidate (1-indexed).

This resolver MUST be the canonical way the command layer
translates "open gate", "close 2.door" into a direction.

### 5.6 Temporary keyword exits

The world exposes a temporary-exit service that creates runtime
keyword exits with a bounded lifetime measured against the area
tick clock.

**Create single-direction exit.**

- Inputs: source room id, keyword, target room id, tick
  duration, optional display name.
- Refuses if the source room is missing or already has the
  keyword exit.
- Determines the current area tick count from the area-tick
  service (using the room id's area prefix); the expiry tick is
  current + duration.
- Registers the keyword exit on the source room and records the
  exit metadata internally under a fresh id.
- Emits a "portal opened" event with the source room id, target
  room id, keyword, display name, and tick duration.
- Returns the new exit's record id, or an empty string on
  refusal.

**Create paired exit.**

- Same as above but registers symmetric keyword exits on both
  rooms with mutually cross-referenced ids. Refuses if either
  side is missing or either keyword is already taken. A single
  "portal opened" event is emitted for the source side.

**Remove exit.**

- Removes the exit by id; if the exit is paired, the partner is
  also removed. Each removal emits a "portal closed" event.
- Concurrent removals are guarded by a mutex so that paired-
  exit removal is atomic.

**Auto-expiry.**

- The service subscribes to the area tick event. On each tick,
  it collects every record whose source room belongs to the
  ticking area AND whose expiry tick is at or below the
  current tick, removes them (and their paired partners
  regardless of partner's area), emits "portal closed" for each
  primary side. Paired-partner removal is silent on the event
  side to avoid duplicate "closed" pairs for the same portal.

The service MUST tolerate the area-tick event carrying either
an `areaId` or an older `areaPrefix` key for backwards
compatibility.

**Acceptance criteria**

- [ ] Door operations sync the reverse-side door state when a
      reverse exit and door exist.
- [ ] Lock requires the door to be closed AND not already
      locked; Open / Close / Unlock check single-sided
      preconditions.
- [ ] Each successful door operation emits its corresponding
      event; locked/unlocked include the key id.
- [ ] Has-key matches by template id of items in the actor's
      contents.
- [ ] Target resolution prefers a Direction parse, then ordinal,
      then disambiguates by candidate count.
- [ ] Temporary-exit create refuses on keyword conflict and on
      missing rooms.
- [ ] Paired exits remove together and emit a single "portal
      closed" per pair side.
- [ ] Auto-expiry runs on area-tick events and removes paired
      partners regardless of which area they live in.
- [ ] Concurrent create/remove operations on temporary exits are
      atomic (no orphaned half-pair).

---

## 6. Weather and time ambience

### 6.1 Roles

The world subscribes to two external events:

- **Hour change** — the time feature emits this once per
  in-game hour, carrying the new hour as data.
- **Period change** — the time feature emits this when the
  named period (e.g. dawn, midday, dusk, midnight) changes,
  carrying the new period name as data.

The world's weather service does NOT decide *when* time changes;
it only reacts to them.

### 6.2 Weather roll on hour change

For each area whose definition declares a weather zone:

1. If the current hour is not on the configured weather-roll
   interval (e.g. every 3 in-game hours), skip the area.
2. Look up the area's current weather state (default `clear`).
3. Roll the next state from the zone's transition table — a
   map of `currentState → {nextState → weight}` — via a
   weighted random pick. If the table has no entry for the
   current state, keep the current state.
4. If the rolled state is identical to the current state, skip.
5. Update the area's current weather state.
6. Emit a `weather change` event with the area id, new state,
   and previous state.
7. For each room in the area that is **eligible** (§6.4), send
   the *end* message for the previous state followed by the
   *start* message for the new state. Messages may be absent at
   any layer (§6.3); absent messages are simply not sent.

### 6.3 Message resolution cascade

Both weather and time messages cascade from most specific to
most general:

1. **Room override.** The room's weather-messages or time-
   messages map (keyed by state or period).
2. **Area override.** The area definition's analogous map.
3. **Zone default.** For weather, the zone's terrain-keyed
   messages map (e.g. different message for `outdoors`,
   `forest`, `mountain`). For time, the zone's terrain-keyed
   period messages.

The first layer to produce a message wins. Time messages are
strings; weather messages are `(start, ongoing, end)` triples
with each field optional.

### 6.4 Eligibility (terrain gating)

A room is eligible to receive weather or time ambience messages
according to its **terrain** property. The default terrain (when
no property is set) is `outdoors`. Rooms whose terrain is
`indoors` or `underground` are **shielded** and do NOT receive
messages by default; they receive them only when the room's
weather-exposed or time-exposed flag is set respectively.

Other terrain values are always eligible. The two specific
shielding terrains are engine-known; new shielding terrains
require a configuration change.

### 6.5 Time-of-day delivery

On a period-change event:

- For each area, for each room belonging to that area, if the
  room is eligible (§6.4), resolve the period's message via
  the cascade and send it to every session in that room as a
  one-shot message. Missing messages are silently skipped.

Unlike weather, time-period changes do NOT emit a separate
engine event from this feature; the time feature already
emitted the period change, and the world's job is only to
deliver messages.

### 6.6 Querying current weather

The world exposes a per-area "current weather" read returning
the area's current state string (defaulting to `clear` for any
area not yet rolled). It also exposes a setter used by content
that needs to force weather (e.g. an in-game ritual that summons
rain).

**Acceptance criteria**

- [ ] Weather rolls only on hours that match the configured
      interval.
- [ ] Identical-state rolls are no-ops (no event, no messages).
- [ ] The roll uses a weighted random pick from the zone's
      transition table for the current state.
- [ ] `weather change` events carry both the new and the
      previous state.
- [ ] Room → area → zone+terrain cascade applies to both
      weather and time messages.
- [ ] `indoors` and `underground` rooms are shielded by default;
      exposure flags override.
- [ ] Period-change delivery does not emit a separate engine
      event from this feature.

---

## 7. Visibility

The world exposes a visibility filter primitive used by renderers
that need to ask "what can this observer see in this room?"

Today the implementation is permissive: every entity in the room
is visible to every observer, and the per-pair `can see` check
returns true unconditionally.

The contract is intentionally extensible: a future implementation
may consult tags (`hidden`, `sneaking`), entity properties (light
sources), or room properties (darkness, fog). Callers MUST go
through the filter rather than reading the room's entity list
directly, so a later change is a single integration point.

**Acceptance criteria**

- [ ] Renderers obtain the visible-entity list through the
      filter, not by direct access to the room's entity list.
- [ ] The filter's defaults are permissive but the call sites
      are wired so a stricter policy is a single-file change.

---

## 8. Observable events

The world feature publishes at least these events.

| Event | When |
|---|---|
| door blocked | a closed door prevented a move (§3.3) |
| door opened | a door operation opened a door (§5.2) |
| door closed | a door operation closed a door (§5.2) |
| door unlocked | a door operation unlocked a door (§5.2) |
| door locked | a door operation locked a door (§5.2) |
| portal opened | a temporary exit was created (§5.6) |
| portal closed | a temporary exit was removed or expired (§5.6) |
| weather change | an area's weather state transitioned (§6.2) |

Player-movement events (`player.moved`, `player.entered`,
`player.left`) are not emitted by *this* feature — they belong
to the command layer that wraps the move primitive. The mob-AI
feature already subscribes to a `player.moved` event for
disposition-state reset (see `docs/specs/mobs-ai-spawning.md`
§5.2); whichever layer emits that event is the canonical
source.

**Acceptance criteria**

- [ ] Each event in the table is emitted with the documented
      payload.
- [ ] No movement-event emission happens inside the move
      primitive (other than door-blocked).

---

## 9. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| Direction enumeration content (cardinal + vertical default) | §3.1 |
| Default and per-area reset interval; occupied modifier | §2.4 |
| Weather zones, transition tables, and per-terrain message defaults | §6 |
| Weather-roll interval in in-game hours | §6.2 |
| Terrain values that shield rooms by default | §6.4 |
| Area-prefix convention used for `reset-area` and portal expiry | §5.4, §5.6 |
| Tag swap cadence (top-of-tick is the engine default) | §3.4 |

---

## 10. Open questions / future work

- **Room-id-as-stringly-typed area pointer.** Multiple
  subsystems derive the area from a room id by splitting on a
  colon: temporary exits, door reset by area, mob AI. Treating
  area as a first-class property on the Room (`room.area`,
  already present but not consistently used) and dropping the
  string parsing would let room ids be opaque again.
- **Two MoveEntity overloads.** The world exposes a door-aware
  and a door-unaware move primitive. The latter exists for
  early callers that pre-date the door feature; consolidating
  on the door-aware form would be a clarity win.
- **Visibility filter is a stub.** The interface is in place but
  the implementation is permissive. A real implementation
  needs darkness, hide/sneak, and possibly per-tag occlusion.
- **Tag-index back pressure.** The world reports dirty-tag
  count per swap but takes no defensive action when the count
  is large. A per-tick budget would prevent pathological cases
  from blowing up the swap cost.
- **Door reset semantics.** Per-area reset restores defaults but
  does not coordinate with the area-tick clock. Whether area
  reset should also reset doors automatically is a design call;
  today it's a separate operation.
- **Temporary exits and persistence.** Temporary exits live in
  memory only; a server restart drops them silently. Content
  that relies on long-running portals (a 24-hour event gate)
  needs a different storage path.
- **Weather forcing vs natural rolls.** Setters override the
  current state but do not affect the transition table, so the
  next roll may bounce right back. A "lock weather to X for N
  hours" primitive would be more useful.
- **Time messages emit no event.** Time-period delivery sends
  text directly to sessions and does not emit an engine event
  for observability or testability. Mirroring the weather-
  change event for time would even the surface.
- **Keyword-exit collision policy.** Two systems may both want
  to register the same keyword on the same room. Today the
  second one fails silently. A first-class collision policy
  (overwrite vs reject vs error) would help authoring.

---

<!-- Generated: 2026-05-21 · Scope: World + Room + AreaRegistry + AreaDefinition + Exit + DoorState + DoorService + TemporaryExitService + WeatherService + WeatherZoneRegistry + VisibilityFilter + AreaTickService + Direction · Spec style: narrative + acceptance criteria · Detail level: behavior only -->

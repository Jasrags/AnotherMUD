# Room Coordinates — Feature Specification

**Status:** Draft · **Scope:** Area-local integer (x, y, z)
coordinates **derived from the exit graph** at load, with an optional
**per-room authored override** (pin) for the spaces derivation can't
place; the derivation walk and its determinism guarantee, the
collision / non-square-loop / unplaced-room conflict policy, load-time
validation warnings, and the `Room.Info` GMCP exposure that lets a
mapper lay rooms out faithfully · **Audience:** Anyone reimplementing
or porting this feature in any language.

This document describes *what* the coordinate surface must do, not
*how*. The direction→delta convention, anchor-selection rule, and
warning wording are policy and live in the configuration-surface
table at §9.

Room coordinates are a **spatial projection of the room graph**
already defined by [world-rooms-movement](world-rooms-movement.md)
§3.2 (exits). Their only authored input is an optional per-room
override for the deliberately non-Euclidean spaces derivation places
poorly (§3.5); they change no movement behavior — they exist so a
client mapper (Mudlet's bundled mapper consuming `Room.Info`) and a
future in-game ASCII `map` verb can render a geographically faithful
layout instead of a topology-only auto-placement. The exit graph
(plus those overrides) remains the single source of truth;
coordinates are recomputed from it at every load.

---

## 1. Overview

Every **placed** room carries an integer coordinate triple
`(x, y, z)` that is **local to its area**. For most rooms the
coordinate is *derived* at load — not stored in content — by walking
the area's intra-area cardinal/vertical exits outward from an
**anchor** room at the origin `(0, 0, 0)`, assigning each newly
reached room the source coordinate plus the direction's delta. A room
may instead **pin** itself with an authored `coord:` (§3.5); pinned
rooms are placed at their declared coordinate and the walk derives
around them.

Because derived coordinates are a pure function of the exit graph +
the pins + the derivation policy, the bulk of the world maps itself
with **no authoring** — adding a room or exit automatically re-places
the neighborhood — while authors hand-fix only the rooms that
genuinely derive wrong (a stacked tower, a twisting cave). Derived
coordinates never drift from exits the way fully hand-placed
coordinates would; pins are the deliberate exception, scoped to the
rooms that need them.

A coordinate is **stable**: a room's `(x, y, z)` depends only on its
area's graph + pins, never on who is looking or where they stand.
Centering a map on the player is a *render-time translation*
(subtract the viewer's room coordinate), not a re-derivation — so the
same room reports the same coordinate to every client and to Mudlet's
persistent mapper across every move (§6).

### Core concepts

- **Coordinate** — an integer `(x, y, z)` triple. `x` is the
  east(+)/west(−) axis, `y` is the north(+)/south(−) axis, `z`
  is the up(+)/down(−) axis. Integers, not floats — one exit =
  one unit step.
- **Area-local** — coordinates are scoped to a single area
  ([world-rooms-movement](world-rooms-movement.md) §2.4). Each
  area has its own origin and its own grid; two rooms in
  different areas may share a coordinate with no relationship.
  There is no global world-space.
- **Anchor** — the room per area placed at `(0, 0, 0)`; the
  seed of that area's derivation walk (a pin may serve as it, §3.1).
- **Pin (authored override)** — a room that declares its own
  area-local `coord:` in content. The loader places it at exactly
  that coordinate and the walk derives around it, never over it
  (§3.5). The escape hatch for spaces derivation can't embed.
- **Placed / unplaced** — a room is *placed* when it is pinned or
  the derivation walk reaches it from a seed via intra-area
  cardinal/vertical exits. A room that is neither pinned nor reached
  (e.g. portal- or cross-area-only) is *unplaced* and carries no
  coordinate.
- **Direction delta** — the fixed per-direction unit vector
  applied when stepping across an exit (north `(0,+1,0)`, etc.;
  full table at §2.3).

### Goals

- Give every placed room a deterministic, **stable** area-local
  coordinate with **near-zero authoring burden** — derivation covers
  the world by default; authors add YAML only to pin the rooms that
  derive wrong.
- Expose coordinates over GMCP so a client mapper renders a
  faithful layout.
- Provide the coordinate substrate a future telnet `map` verb
  consumes, without specifying that verb here.
- Surface content topology problems (overlaps, non-square loops,
  orphaned rooms) as **load-time warnings**, never crashes.

### Non-goals

- **Hand-placing every room.** The model is derive-by-default with a
  *targeted* per-room override (PD-1), not authored-first. Authors
  are never required to place a room; pins are the exception, not the
  baseline. (Contrast Option A in the proposal — full authored
  layout — which is explicitly rejected.)
- **A global world map.** Coordinates are area-local (PD-2); this
  spec does not align areas into one space.
- **The `map` verb / any rendering.** This spec is substrate. The
  ASCII map renderer and its formatting are a separate
  presentation slice ([ui-rendering-help](ui-rendering-help.md)
  is the layer it would join).
- **A movement change.** Coordinates are descriptive. The move
  primitive ([world-rooms-movement](world-rooms-movement.md) §3.3)
  stays exit-driven and never consults a coordinate (PD-3).
- **Diagonal directions.** The engine's direction set is
  cardinal + vertical only; when diagonals land, the delta table
  (§2.3) extends. No diagonal handling is specified now.
- **Persistence.** Coordinates are derived content state, not
  player state; nothing new is saved (§8).

### 1.1 Pre-decisions

| ID | Decision | Status |
|---|---|---|
| PD-1 | **Derive-by-default with a per-room override (hybrid).** Derivation places every room from the exit graph; a room MAY carry an authored `coord:` that pins it as a fixed point the walk treats as ground truth and never overwrites (§3.5). Authors override only where derivation visibly breaks — full coverage on day one, hand-fixes where needed. (Chosen over pure-derived in v1: the override is the only recourse for deliberately non-Euclidean spaces, and cheap to add.) | Decided |
| PD-2 | **Area-local, not global.** Each area is its own grid with its own origin; no inter-area alignment. Mudlet already groups rooms by area, so a per-area grid is the native shape. | Decided |
| PD-3 | **Descriptive, never gating.** Coordinates never affect movement, targeting, or any rule — the move primitive stays exit-driven. A wrong coordinate degrades the map, nothing else. | Decided |
| PD-4 | **Non-fatal derivation.** Collisions, non-square loops, and unplaced rooms emit load-time warnings and degrade the map locally; they never abort the load. A strict fail-mode is an open question (§10). | Decided |
| PD-5 | **Fixed direction→delta convention** (§2.3), matching the mapper's expected axes (north = +y, east = +x, up = +z). | Decided |
| PD-6 | **Deterministic anchor + walk.** Same content always yields the same coordinates. A pin (lexicographically-smallest pinned room id) seeds the area when present; otherwise the default anchor = the lexicographically-smallest room id in the area. Walk order is deterministic (§3.2). | Decided |
| PD-7 | **Stable coordinates.** A room's coordinate depends only on its area's graph + pins, never on the viewer or their position. Player-centering is a render-time translation, not a re-derivation — required so Mudlet's persistent mapper sees one fixed coordinate per room (§6). | Decided |

### 1.2 What room coordinates are *not*

- **Not authored-by-default.** Most rooms declare no position; the
  loader derives it. A room MAY pin itself (§3.5), but that is the
  exception. (Contrast: `terrain`, `tags`, `healing_rate` are always
  authored.)
- **Not viewer-relative.** A room's coordinate is the same for every
  player and every client (PD-7); a map centered on the player is a
  render-time recenter, not a different coordinate.
- **Not a constraint on exits.** A non-square loop (go E, N, W,
  S and not return to start) is *legal* — it just produces a
  collision warning and a locally-overlapping map (which a pin can
  then fix). The spec does not reject non-Euclidean topology; it
  tolerates it.
- **Not persisted or migrated.** Recomputed every boot; no save
  field, no `player.CurrentVersion` bump.
- **Not cross-area.** The walk stops at an area boundary; a
  neighbor in another area belongs to that area's grid.

---

## 2. The coordinate model

### 2.1 Shape

A coordinate is an integer triple `(x, y, z)`. A placed room has
exactly one. An unplaced room (§4.3) has none.

### 2.2 Area-local scope

The origin `(0, 0, 0)` is per area. A room's coordinate is only
meaningful relative to other rooms **in the same area**. Comparing
coordinates across areas is undefined and no consumer may do it.

### 2.2.1 Stability (viewer-independent)

A room's coordinate is a property of the room, not of the request:
it is identical for every viewer and every client (PD-7). Consumers
that want a player-centered view (the ASCII `map` verb) **translate**
the area's coordinates by the viewer's room coordinate at render
time; they never ask the substrate for a different, relative set.
This is what lets Mudlet's persistent mapper store one fixed
coordinate per room and update incrementally as the player moves
(§6).

### 2.3 Direction deltas

Stepping a placed room's coordinate across an exit in direction
`d` adds the fixed delta:

| Direction | Delta (x, y, z) |
|---|---|
| north | (0, +1, 0) |
| south | (0, −1, 0) |
| east | (+1, 0, 0) |
| west | (−1, 0, 0) |
| up | (0, 0, +1) |
| down | (0, 0, −1) |

Keyword exits (portals) have no direction and therefore no delta —
they are never stepped (§3.3).

### 2.4 Acceptance — model

- [ ] Every placed room has exactly one integer `(x, y, z)`.
- [ ] Coordinates are area-local: a pin-free area's origin is its
      anchor at `(0, 0, 0)`; a pinned area's origin is wherever the
      author placed the pin (§3.1).
- [ ] A room's coordinate is the same regardless of viewer (PD-7).
- [ ] The six engine directions map to the §2.3 deltas exactly.
- [ ] No consumer compares coordinates of rooms in different
      areas.

---

## 3. Derivation

Derivation runs **once per load**, after the room graph and area
assignments are fully assembled and before the server accepts
connections. It is a pure function of (rooms, exits, areas,
policy).

### 3.1 Seeds and anchor selection

Each area's derivation starts from one or more **seeds** — rooms
whose coordinates are fixed before the walk:

- **Pinned rooms** (§3.5) are always seeds, placed at their
  authored `coord:`.
- If the area has **no pin**, the loader picks a default anchor: the
  area's room with the lexicographically-smallest id, placed at
  `(0, 0, 0)`. If the area **has** pins, no synthetic anchor is
  added — the pins are the seeds, and the origin is wherever the
  author put them (a pin at `(0,0,0)` is the conventional centre).

The anchor rule is configurable (§9). Pinning a room subsumes the
"authored anchor" need: to recentre an area, pin its centre room at
`(0,0,0)`.

### 3.2 The walk

From the seeds, perform a deterministic breadth-first traversal
over **intra-area cardinal/vertical exits**:

1. Place every seed (pins at their authored coordinate; the default
   anchor, if any, at `(0, 0, 0)`).
2. Process placed rooms in a deterministic order (seeds first, by
   ascending room id); for each, visit its direction exits in
   canonical direction order (north, south, east, west, up, down).
3. For an exit in direction `d` to a target room `T` **in the
   same area** that is not yet placed: place `T` at
   `source + delta(d)`.
4. A target already placed — by a pin or an earlier step — is
   **not** re-placed (pin wins, else first placement wins — §4.1,
   §4.4).
5. Continue until no unplaced, reachable intra-area room remains.

Determinism: the seed order, traversal order, and per-room direction
order are all fixed, so the same content (graph + pins) always
yields byte-identical coordinates.

### 3.3 What the walk does *not* follow

- **Cross-area exits.** An exit whose target is in a different
  area is not stepped; the target is placed (if at all) by its
  own area's walk.
- **Keyword exits / portals.** Non-directional, non-spatial; never
  stepped. A portal-only destination is unplaced (§4.3).
- **Hidden exits** ([hidden-exits](hidden-exits.md)) are followed
  *as normal directional exits* for derivation — concealment is a
  per-observer runtime concern, orthogonal to the static map. A
  secret passage still occupies a grid cell.

### 3.4 Acceptance — derivation

- [ ] A pin-free area's default anchor is placed at `(0, 0, 0)` per
      the §3.1 rule.
- [ ] An area's pins are placed at their authored coordinates and
      seed the walk; no synthetic anchor is added when a pin exists.
- [ ] A target reached across a same-area direction exit `d` is
      placed at `source + delta(d)`.
- [ ] Re-deriving the same content (graph + pins) produces identical
      coordinates (determinism).
- [ ] Cross-area and keyword exits are never stepped.
- [ ] Hidden directional exits are stepped like ordinary exits.

### 3.5 Pinned rooms (authored override)

A room may carry an authored area-local `coord:` (the YAML key shape
is policy, §9). A pinned room is the override escape hatch for the
spaces derivation places poorly — a vertical shaft, a portal
arrival, a non-Euclidean cave that won't embed in a grid.

- A pin is an **area-local** `(x, y, z)`, same coordinate space as
  derived rooms (§2.2). The author chooses the area's origin by
  where they place the pins.
- A pinned room is placed at exactly its `coord:` and seeds the walk
  (§3.1). Its neighbors derive outward from it by the normal deltas.
- A pin is **ground truth**: the walk never overwrites it, and a
  derived placement that would land another room on a pinned cell
  loses to the pin (§4.4) — the pin is the author saying "this room
  is *here*, derive around it".
- Pins are **content**, loaded with the room; they are not a save
  field and add no migration (§8). Editing a pin and reloading
  re-derives the neighborhood.
- A malformed pin (non-integer, missing axis) is a **content
  authoring error** surfaced as a load warning; the room falls back
  to derived placement (or unplaced) rather than aborting the load.

### 3.6 Acceptance — pinned rooms

- [ ] A room with an authored `coord:` is placed at exactly that
      coordinate.
- [ ] A pinned room seeds the walk; its same-area neighbors derive
      outward from the pin.
- [ ] A pin is never overwritten by derivation; a derived room that
      would collide with a pinned cell loses to the pin (§4.4).
- [ ] A malformed pin warns and falls back; it does not abort the
      load.
- [ ] A pin is content, not a save field; no migration is added.

---

## 4. Conflict and validation policy

Derivation tolerates non-grid topology. Three situations are
expected in real content and each has a defined, non-fatal
outcome (PD-4).

### 4.1 Collision — two rooms, one cell

Two same-area rooms derive to the same `(x, y, z)` (a folded or
non-square layout). **First placement wins**: the room placed
first keeps the cell; the later room keeps the coordinate its own
first placement assigned (it is not re-placed). The map may
overlap at that cell. The loader emits a **collision warning**
naming both room ids and the coordinate.

### 4.2 Non-square loop — inconsistent re-reach

A room is reached again by a different path implying a *different*
coordinate (e.g. `A →E→ B →N→ C` vs `A →N→ D →E→ C` landing C at
two absolute cells because the loop is not square). **First
assignment wins** (§3.2 step 4); the contradicting edge is
recorded as an **inconsistent-edge warning** (from-room, to-room,
direction, expected vs existing coordinate). No coordinate is
changed.

### 4.3 Unplaced room — unreachable from any seed

A room in the area that is neither pinned nor reached by the walk
via intra-area cardinal/vertical exits (reachable only by portal,
cross-area exit, or not at all) is **unplaced**: it has no
coordinate, and GMCP omits its coordinate fields (§5). The loader
emits an **unplaced-room warning** naming the room and its area.
(Pinning such a room is the author's fix.)

### 4.4 Pin vs derived — the override wins

When derivation would place a room on a cell already held by a
**pinned** room, or would assign a coordinate to a room that is
itself pinned, the **pin wins** and the derived placement is
discarded — this is the intended override, not an error, so it is
**silent** (no warning). Two situations differ:

- *Derived room lands on a pinned cell* — the derived room keeps the
  coordinate its own first placement gave it (it is not the pinned
  room); only the *pinned* room owns that cell. If they still
  overlap visually that is an ordinary §4.1 collision between the
  derived room and the pin's cell, warned as such.
- *Two pins share a cell* — an authoring mistake (the author placed
  two rooms at one coordinate). The loader emits a **pin-collision
  warning** naming both rooms and the coordinate; first-by-id wins
  the cell, the load continues.

### 4.5 Acceptance — conflict policy

- [ ] A collision keeps the first room's cell and warns with both
      ids + the coordinate; the load still succeeds.
- [ ] An inconsistent re-reach keeps the first assignment and
      warns with the conflicting edge; no coordinate is mutated.
- [ ] An unreachable, unpinned room is left unplaced and warned, not
      crashed or defaulted to `(0,0,0)`.
- [ ] A derived room never displaces a pin (§4.4); a pin override is
      silent, not warned.
- [ ] Two pins on one cell warn (pin-collision) and the load still
      succeeds.
- [ ] No conflict condition aborts the load (PD-4).

---

## 5. GMCP exposure

The `Room.Info` package
([networking-protocols](networking-protocols.md) §7) gains optional
area-local coordinate fields so a mapper places the room directly
rather than inferring position from exit topology.

- A placed room's `Room.Info` carries its area-local integer
  coordinate.
- An unplaced room (§4.3) **omits** the coordinate (the mapper falls
  back to its own relative placement for that room).
- The existing `area` field is the mapper's zone key; the vertical
  axis is its layer. No other `Room.Info` field changes.
- Coordinates are emitted on the same cadence as the rest of
  `Room.Info` (on room change); they are stable per room (§2.2.1),
  so no extra diffing is needed.

> **Wire-shape caveat (validate against a live client).** This spec
> commits to *area-local integer coordinates on `Room.Info`*, **not**
> to a precise field layout. The current `Room.Info` uses flat
> fields, but Mudlet's bundled mapper has its own convention (an
> area id + a `coords`-style grouping and a specific update flow).
> The exact JSON shape — flat `x`/`y`/`z` vs. a `coords` array vs.
> whatever Mudlet's generic mapper script reads — MUST be pinned
> against a real Mudlet client before the GMCP slice ships, and may
> differ from the placeholder used while building the ASCII renderer.
> The substrate (a stable area-local integer triple per room) is
> fixed; its serialization is an integration detail.

### 5.1 Acceptance — GMCP

- [ ] A placed room's `Room.Info` carries its area-local integer
      coordinate.
- [ ] An unplaced room omits the coordinate entirely (not zero).
- [ ] No existing `Room.Info` field changes shape.
- [ ] The emitted shape is validated against a live Mudlet mapper
      before the GMCP slice is called done.

---

## 6. Consumers (informative)

This spec is **substrate**; it defines the coordinate and its GMCP
surface, not the things that read them. Known intended consumers,
each its own future slice:

- **Client mapper (Mudlet).** Consumes the `Room.Info` coordinate
  for faithful layout; relies on **stability** (§2.2.1) to store one
  fixed cell per room and update incrementally. Client-owned, no
  server work beyond §5.
- **Telnet `map` verb.** A future presentation slice
  ([ui-rendering-help](ui-rendering-help.md)) that renders an
  ASCII minimap of the current area from coordinates. It centers on
  the player by **translating** the area's stable coordinates at
  render time (subtract the viewer's room coordinate) — it does not
  ask the substrate for viewer-relative coordinates. Out of scope
  here; this spec only guarantees the data it would read.

**Visibility note (both renderers).** Coordinates are computed
omnisciently (the static map includes secret passages and rooms the
player has not visited). Today this leaks nothing because `CanSee`
is effectively permissive, but **both** the ASCII map and the GMCP
feed are information-leak vectors once [visibility](visibility.md)
and any exploration model land. The consuming map spec — not this
substrate — must state that rendered/emitted rooms respect
visibility and exploration, and decide whether v1 reveals the whole
area or only visited rooms (§10).

No consumer behavior is normative in this document.

---

## 7. Interaction with existing world features

- **Doors / locked exits** ([world-rooms-movement](world-rooms-movement.md)
  §5). A door is a property of an exit, not a barrier to
  derivation — the walk steps a doored exit normally. A closed
  door does not make its target unplaced.
- **Portals** (temporary keyword exits). Runtime-created and
  non-directional; never affect coordinates. A room reachable
  only through a portal is unplaced (§4.3).
- **Terrain / biomes** ([biomes](biomes.md)). Orthogonal; a
  coordinate says *where*, terrain says *what*. Both ride along
  in `Room.Info`.
- **Hidden exits** ([hidden-exits](hidden-exits.md)). Stepped
  during derivation as ordinary directional exits (§3.3); the
  static map includes secret passages even though a given player
  has not discovered them. (Whether the *client* should be told
  about an undiscovered exit's neighbor is a visibility concern,
  not a coordinate one — see §10.)

### 7.1 Acceptance — interaction

- [ ] A doored same-area exit is stepped; its target is placed.
- [ ] A portal-only destination is unplaced.
- [ ] A hidden directional exit's target is placed.

---

## 8. Persistence

Nothing new is persisted as **state**. Coordinates are derived
content state, recomputed at every load from the exit graph + pins.

- No field is added to the player or account save.
- `player.CurrentVersion` is **not** bumped; no migration.
- The per-room **pin** (§3.5) persists only **as content** — it
  lives in the room YAML and is loaded with the room, exactly like
  `terrain` or `tags`. It is not a save field and introduces no
  migration.
- Re-deriving after a content change (rooms/exits/pins edited)
  yields the new coordinates with no save-format concern.

### 8.1 Acceptance — persistence

- [ ] No save shape changes and no migration is introduced.
- [ ] A pin round-trips as content (room YAML), not as save state.
- [ ] Editing content and reloading re-derives coordinates with
      no persisted residue from the prior layout.

---

## 9. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| Direction → delta | §2.3 table | The per-direction unit step. Fixed convention; listed for interoperability with the mapper's axes. |
| Per-room pin (`coord:`) | unset | Authored area-local override placing a room at a fixed coordinate the walk derives around (§3.5). The YAML key name is policy. |
| Anchor selection (pin-free area) | lexicographically-smallest room id in the area | Which room is placed at `(0,0,0)` when an area has no pin (§3.1). |
| Cross-area exits followed | no | Whether the walk steps an exit into another area (§3.3). v1: no. |
| Collision handling | first-wins + warn | Behavior when two rooms derive to one cell (§4.1). |
| Inconsistent-edge handling | first-wins + warn | Behavior on a non-square re-reach (§4.2). |
| Pin override | pin wins, silent | A pin beats a derived placement for its cell with no warning (§4.4). |
| Pin collision (two pins, one cell) | first-by-id + warn | Authoring-error handling when two pins share a coordinate (§4.4). |
| Unplaced-room handling | omit coordinate + warn | Behavior for a room neither pinned nor reachable from a seed (§4.3). |
| Strict mode (fail load on conflict) | off | Whether any conflict warning is promoted to a fatal load error (§10). v1: off. |
| Warning log level | warn | Severity of the derivation warnings (F2 structured logging). |

---

## 10. Open questions

*(Resolved since the first draft: the **per-room override** and the
**authored-anchor** questions are now decided — the override is in
v1 as the pin, §3.5, and pinning a room subsumes the anchor flag.)*

- **Secondary anchors for disconnected sub-graphs.** Instead of
  leaving a portal-only cluster unplaced (§4.3) and asking the
  author to pin it, should the loader auto-seed a *second* anchor
  for each disconnected component in an area so the cluster gets its
  own local grid (offset to avoid overlap)? Trades a cleaner
  out-of-the-box map for more policy; pinning already covers it
  manually.
- **Strict mode.** Should there be a build/CI mode where any
  collision or inconsistent-edge warning fails the load, to keep
  authored areas grid-clean (and force a pin)? (Config stub at §9;
  behavior TBD.)
- **Diagonal directions.** When/if the direction set gains
  NE/NW/SE/SW, the delta table extends to combined steps
  (NE = (+1,+1,0)). No work now; flagged so the table is known to
  be extensible.
- **Visibility / exploration leak (both renderers).** The static
  coordinate set is omniscient — it includes secret passages and
  unvisited rooms. A renderer or GMCP feed that surfaces them leaks
  information the player should not have (§6). Does coordinate
  *emission* filter by per-observer visibility/exploration, or stay
  omniscient with the *consuming map spec* doing the filtering? This
  substrate leans "stay omniscient; the map spec filters", but the
  decision (and the v1 reveal-whole-area vs. visited-only fork) is
  owned by the player-maps slice, coordinated with
  [visibility](visibility.md).

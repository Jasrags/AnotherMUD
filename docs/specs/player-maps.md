# Player Maps — Feature Specification

**Status:** Draft · **Scope:** Two ASCII map surfaces — an
**active, toggleable minimap** (a small local window auto-appended to
the room view) and a **`map` verb** (the full discovered map of the
current area) — plus a **persisted fog of war** (only rooms the
character has entered are ever drawn) and the **Mudlet GMCP** map
surface, all sitting on one shared **local-window query** over the
[room-coordinates](room-coordinates.md) substrate · **Audience:**
Anyone reimplementing or porting this feature in any language.

This document describes *what* the map surfaces must do, not *how*.
Glyphs, colors, the default minimap radius, and the prompt placement
are policy and live in the configuration-surface table at §10.

Player maps are a **rendering** of the room graph already defined by
[world-rooms-movement](world-rooms-movement.md) §3 (exits) and the
stable coordinates from [room-coordinates](room-coordinates.md). The
maps change no movement behavior; they read coordinates and the
character's exploration history and draw them. The exit graph + the
coordinate substrate remain the single source of truth for geometry;
fog of war is the only new state this feature owns.

---

## 1. Overview

Three things sit on top of the coordinate substrate:

1. A **local-window query** — the shared seam. Given a room and a
   radius, it returns the nearby *placed, same-area* rooms (a bounded
   walk over intra-area directional exits) carrying their stable
   area-local coordinates and the exits among them. It reads
   coordinates; it never computes or re-centers them.
2. A **fog-of-war filter** — the character's persisted set of
   **visited** rooms (those they have entered at least once),
   intersected with the window before anything is drawn. A room the
   character has not entered is never rendered.
3. Two **renderers** over the filtered window:
   - the **active minimap** — a small player-centered grid
     auto-appended to the room view, toggled by a persisted
     per-character preference, that updates as the player moves;
   - the **`map` verb** — an explicit, on-demand render of the full
     discovered map of the **current area** (every visited room in
     it).
   Plus the **GMCP map surface**: the `Room.Info` coordinate fields
   ([room-coordinates](room-coordinates.md) §5) emitted current-room-
   only as the player moves, which Mudlet's native mapper consumes.

### Core concepts

- **Window query** — a bounded breadth-first walk from an origin room
  over intra-area cardinal/vertical exits, returning placed rooms
  within a radius and the exits among them. Radius is the step bound;
  the minimap passes a small radius, the `map` verb passes "the whole
  area".
- **Visited set** — the per-character set of namespaced room ids the
  character has entered. Persisted; the fog-of-war authority. Empty for
  a fresh character.
- **Fog of war** — the rule that only visited rooms are drawn. An
  **unvisited neighbor** of a visited room is never drawn as a room;
  the exit toward it is drawn as a **stub** (a connector into the dark)
  so the player sees that an exit exists without seeing what is beyond
  it (PD-3).
- **Active minimap** — a small local map appended to the room view on
  every room render (look + each arrival) when the per-character
  `minimap` toggle is on. "Active" = it redraws as the player moves.
- **`map` verb** — an explicit command rendering the full discovered
  map of the current area.
- **Player-centering** — the minimap places the viewer's room at grid
  center by **translating** the area's stable coordinates at draw time
  (subtract the viewer's coordinate). Not a re-derivation
  ([room-coordinates](room-coordinates.md) §2.2.1).

### Goals

- An **active, toggleable minimap** of the player's surroundings,
  centered on their room, drawn from discovered rooms only, redrawn as
  they move — usable on raw telnet with zero client cooperation.
- A **`map` verb** rendering the full discovered map of the current
  area on demand.
- **Persisted fog of war** — the maps reflect only rooms the character
  has explored, remembered across sessions.
- Structured map data over **GMCP** that Mudlet's native mapper
  consumes, drawing from the same coordinate substrate, with no
  bulk-reveal of unexplored rooms.
- Both ASCII surfaces and the GMCP surface drawing from **one shared
  window query**, not parallel graph walks.

### Non-goals

- **Pathfinding / speedwalk** in v1 (Mudlet gets click-to-walk for
  free once it has a map; a server-side speedwalk is a later
  increment).
- **A player-annotatable map** — Mudlet annotates client-side; the
  server does not duplicate it.
- **An out-of-game web atlas** or a stylized overland "world map" — this
  is room-graph mapping only.
- **The coordinate substrate** — that is
  [room-coordinates](room-coordinates.md); this feature consumes it.
- **A movement change** — maps are descriptive; the move primitive
  stays exit-driven ([room-coordinates](room-coordinates.md) PD-3).

### 1.1 Pre-decisions

| ID | Decision | Status |
|---|---|---|
| PD-1 | **Two ASCII surfaces, one engine.** An *active, toggleable minimap* (local window auto-appended to the room view) and a *`map` verb* (full current-area map) both consume the same window query + fog filter; the minimap passes a small radius, `map` passes the whole area. | Decided |
| PD-2 | **Fog of war is persisted and in v1.** Only rooms the character has *entered* are ever drawn; the visited set is per-character and survives sessions. This is the one piece of new save state (§4, §8). | Decided |
| PD-3 | **Fog edge = stub the exit, hide the room.** A visited room shows connectors toward its exits even when the neighbor is unvisited, but the unvisited room itself is never drawn — the player sees a door from where they stand, not the layout beyond it. | Decided |
| PD-4 | **Geometry is the room-coordinates substrate** (stable, area-local, derived; [room-coordinates](room-coordinates.md)). Player-centering is render-time translation, never re-derivation (PD-7 there). Not re-decided here. | Decided |
| PD-5 | **Exploration rides the room-change chokepoint.** A room is marked visited at the single seam every arrival passes through (move, recall, teleport, login, link-dead reattach) — entering is entering, so an admin teleport counts. | Decided |
| PD-6 | **Phasing.** Fog/visited substrate → shared window query → ASCII (minimap + `map`) → Mudlet GMCP surface. The ASCII path validates geometry *and* fog visibly on every client before the client-specific GMCP work. | Decided |
| PD-7 | **Z is single-plane for ASCII.** The ASCII surfaces draw the viewer's z-level and mark vertical exits with up/down indicators; Mudlet handles z natively. | Decided |
| PD-8 | **Non-cardinal / keyword exits are annotated, not gridded.** Portals and keyword exits are *unplaced* in the substrate; the renderers note their presence (an annotation) rather than drawing a grid connector. | Decided |
| PD-9 | **GMCP wire shape is a placeholder pending a live client.** The flat `x`/`y`/`z` on `Room.Info` is validated against a real Mudlet mapper before the GMCP slice is called done ([room-coordinates](room-coordinates.md) §5 caveat). | Decided |
| PD-10 | **Visited set is pruned against live content.** Ids for rooms no longer in the loaded world are dropped (on load and/or harmlessly at render time — a stale id is simply never drawn). The set never blocks the load. | Decided |

---

## 2. The local-window query

The shared seam both ASCII renderers and the area query stand on.

- Given an **origin room** and a **radius**, the query performs a
  bounded breadth-first walk from the origin over **intra-area
  cardinal/vertical exits**, collecting every reachable **placed**
  room (one with a coordinate) within the radius, plus the directional
  exits among the collected rooms.
- The query **reads** coordinates from the substrate; it never
  computes or re-centers them. It returns absolute area-local
  coordinates and lets each renderer translate.
- It does **not** apply fog of war — that is a separate filter (§4)
  the callers apply to the result, so the same query serves the
  omniscient GMCP path and the fog-filtered ASCII path.
- It stops at the **area boundary** (cross-area exits are not walked)
  and does not follow **keyword/portal** exits (non-directional, hence
  unplaced) — those are surfaced as annotations (§6.5), not as window
  members.
- A "whole area" request (the `map` verb) is the same query with an
  unbounded radius, equivalent to every placed room in the origin's
  area.

### 2.1 Acceptance — window query

- [ ] The query returns only placed, same-area rooms within the
      radius of the origin, plus the directional exits among them.
- [ ] The query reads stable coordinates; it returns absolute area
      coordinates and re-centers nothing.
- [ ] The query applies no fog filter; callers filter the result.
- [ ] Cross-area and keyword exits are not walked.
- [ ] An unbounded radius yields every placed room in the area.

---

## 3. Fog of war — the visited set

The maps draw only what the character has explored.

- Each character carries a persisted **visited set**: the namespaced
  ids of rooms they have entered at least once. It is **authoritative**
  for what the ASCII surfaces may draw and what the server is willing
  to reveal.
- A room is added to the set the moment the character **enters** it,
  at the single room-change seam every arrival passes through (PD-5) —
  ordinary movement, recall, teleport, login spawn, and link-dead
  reattach all count. Re-entering an already-visited room is a no-op.
- The set is **persisted** with the character save (§8) and seeded from
  it on login; a fresh character starts empty (the spec MAY back-fill
  the spawn room on first entry, which the entry hook does naturally).
- Both ASCII renderers **intersect** the window-query result with the
  visited set before drawing. A window member the character has not
  visited is dropped as a *room* but its inbound exit from a visited
  room is still drawn as a **stub** (PD-3, §6.4).
- The set is **pruned against live content** (PD-10): an id for a room
  no longer present in the loaded world contributes nothing (it is
  dropped on load, or simply never matches a live room at render
  time). A stale id never blocks the load or crashes a render.

### 3.1 Acceptance — fog of war

- [ ] Entering a room adds it to the character's visited set; a repeat
      entry changes nothing.
- [ ] Every arrival path (move, recall, teleport, login, reattach)
      marks the destination visited.
- [ ] The visited set round-trips across logout/login.
- [ ] A fresh character's set is empty until they enter a room.
- [ ] An unvisited room is never drawn; only its inbound exit-stub from
      a visited room appears.
- [ ] A visited id with no matching live room is ignored, not crashed.

---

## 4. The active minimap

A small local map that travels with the player.

- The minimap is **appended to the room view** — the same render the
  player sees from `look` and on every arrival — when the character's
  **`minimap` preference** is on. Because it redraws on each room
  change, it tracks the player as they move ("active").
- It is a **per-character preference**, toggled by a `minimap` command
  (`minimap on` / `minimap off`, bare `minimap` reports or flips —
  policy, §10) and **persisted** with the save so it survives logout.
  It is a normal player preference, not gated by any role.
- It renders the **local window** (§2) at a small radius (§10) around
  the player's current room, **fog-filtered** (§3) and
  **player-centered** by render-time translation (the viewer's room at
  grid center).
- It is appended **outside** any light gate that suppresses the room
  body — but a renderer MAY choose to omit or blank the minimap when
  the viewer cannot see at all (a §10 policy aligned with
  [light-and-darkness](light-and-darkness.md)); v1 leaves the minimap
  visible since it is memory, not sight.
- When the player's current room is **unplaced** (no coordinate), the
  minimap cannot center; it degrades to an empty/placeholder frame
  rather than erroring.

### 4.1 Acceptance — active minimap

- [ ] With the toggle on, the room view (look + each arrival) carries a
      player-centered local minimap; with it off, it does not.
- [ ] The toggle persists across logout/login.
- [ ] The minimap draws only visited rooms within the configured
      radius, centered on the player.
- [ ] The minimap redraws to follow the player on each room change.
- [ ] An unplaced current room degrades to a placeholder, not an error.

---

## 5. The `map` verb

The full discovered map, on demand.

- The `map` verb renders the **full discovered map of the current
  area** — every **visited** room in the area (§2 unbounded radius,
  §3 fog-filtered), player-centered by translation, drawn with the
  same glyph/terrain model as the minimap (§6).
- It is always available (no toggle); it is an explicit command, so a
  large explored area MAY exceed the viewport and wrap/scroll — that
  is acceptable for an on-demand render. (A bounded `map <radius>`
  form and a cross-area "world map" mode are out of scope for v1, §11.)
- When the player's current room is unplaced, `map` reports that the
  area cannot be mapped from here rather than drawing an un-centered
  grid.

### 5.1 Acceptance — `map` verb

- [ ] `map` renders every visited room in the current area, centered
      on the player.
- [ ] `map` applies the same fog filter and glyph model as the
      minimap.
- [ ] `map` from an unplaced room reports it cannot map, not a broken
      grid.

---

## 6. Rendering model (both ASCII surfaces)

The minimap and `map` share one drawing model; they differ only in
radius and framing.

### 6.1 The grid

- Rooms are placed on a character grid by their **translated**
  coordinates (viewer at center). One room occupies one cell; the gap
  between adjacent cells carries the connector for the exit between
  them.
- The **viewer's room** is marked distinctly (a "you are here" glyph,
  §10).
- Empty grid cells (no placed/visited room) render as blank.

### 6.2 Terrain glyphs and color

- A room's cell glyph and color derive from its **terrain**
  ([world-rooms-movement](world-rooms-movement.md) §6.4) via the
  theme renderer ([ui-rendering-help](ui-rendering-help.md) §3) —
  semantic map tags resolved per color tier, degrading to plain text
  for non-color clients. Terrain→glyph/tag mapping is policy (§10).

### 6.3 Connectors

- A directional exit between two drawn rooms renders as a connector in
  the gap between their cells (horizontal, vertical, matching the
  direction). A **door** on that exit MAY be marked on the connector
  (open/closed/locked — a §10 policy), reusing the door state already
  on the exit.

### 6.4 Fog edge — exit stubs

- An exit from a **visited** room to an **unvisited** room renders as a
  **stub**: a connector that points toward the neighbor's direction but
  terminates in blank space, with no room cell drawn beyond it (PD-3).
  The player sees that an exit exists without seeing the room it leads
  to.

### 6.5 Non-cardinal and vertical exits

- **Keyword/portal exits** are unplaced (no grid direction); they are
  surfaced as an **annotation** beside the map (e.g. a noted portal),
  never as a grid connector (PD-8).
- **Up/down exits** are not drawn as grid cells (the grid is one
  z-level, PD-7); the viewer's room (and any drawn room with a vertical
  exit) carries an **up/down indicator** (§10). Mudlet renders z
  natively from the GMCP coordinate.

### 6.6 Visibility interaction

- A **hidden/undiscovered exit** ([hidden-exits](hidden-exits.md)) in
  an already-visited room MUST NOT be drawn (neither connector nor
  stub) until that observer has discovered it — concealment is
  per-observer, so the omniscient window must be filtered by the
  observer's discovery state before drawing. (Until a discovery model
  lands, the engine's exits are all visible; this clause is the
  forward contract so the map does not become a hidden-exit oracle.)

### 6.7 Acceptance — rendering

- [ ] Drawn rooms sit at their translated coordinates with the viewer
      at center; one room per cell.
- [ ] A room's glyph/color comes from its terrain via the theme,
      degrading to plain text without color.
- [ ] A directional exit between two drawn rooms draws a connector;
      door state MAY be marked.
- [ ] An exit to an unvisited room draws a stub with no room beyond it.
- [ ] Keyword exits are annotated, not gridded; vertical exits show an
      up/down indicator, not a cell.
- [ ] An undiscovered hidden exit in a visited room is not drawn.

---

## 7. GMCP / Mudlet surface

- The `Room.Info` package already carries the room's stable area-local
  coordinate ([room-coordinates](room-coordinates.md) §5); this feature
  adds **no new bulk map package**. Mudlet's mapper builds its graph
  from the per-move `Room.Info` frames.
- Emission stays **current-room-only**, on each room change — the
  existing cadence. The server never bulk-sends an unexplored area, so
  Mudlet's client-side map fills in exactly as the player walks, which
  keeps fog of war honest **without** a second server-side persistence
  layer for the Mudlet map.
- The **wire shape** of the coordinate (flat `x`/`y`/`z` vs. Mudlet's
  `coords` convention) is validated against a **live Mudlet client**
  before the GMCP slice is called done (PD-9).

### 7.1 Acceptance — GMCP

- [ ] Map data rides the existing `Room.Info` coordinate fields; no
      new bulk package is added.
- [ ] `Room.Info` is emitted current-room-only on room change; no
      unexplored area is bulk-revealed.
- [ ] The coordinate wire shape is validated against a live Mudlet
      mapper before the GMCP slice is done.

---

## 8. Persistence

This feature adds **one** new piece of per-character save state: the
visited set.

- The character save gains a **visited-room set** (namespaced room
  ids). It is the only new save field; the coordinate substrate stays
  derived and save-free ([room-coordinates](room-coordinates.md) §8).
- Adding the field bumps the player save version with an **append-only
  migration**; a legacy save with no set migrates to an **empty** set
  (the character re-explores to rebuild their map — the intended
  fog-of-war behavior, not a value to back-fill).
- The set is updated on the room-entry seam (§3) and the save marked
  dirty so the normal autosave pipeline persists it.
- The **`minimap` preference** is also persisted (a boolean), following
  the same convention as other per-character display preferences.
- The visited set is **pruned against live content** (PD-10); nothing
  about a removed room persists in a way that breaks a later load.

### 8.1 Acceptance — persistence

- [ ] The visited set persists with the character save and seeds on
      login.
- [ ] A legacy save migrates to an empty set via an append-only
      migration; no old migration is edited.
- [ ] The `minimap` toggle persists across sessions.
- [ ] A removed room's id in a persisted set never breaks a load.

---

## 9. Interaction with existing features

- **Room coordinates** ([room-coordinates](room-coordinates.md)) — the
  geometry source; the maps read `Coord` and translate. An unplaced
  room is undrawable (§4/§5 degrade).
- **Doors / locked exits** ([world-rooms-movement](world-rooms-movement.md)
  §5) — door state MAY be marked on a connector (§6.3); a door never
  changes whether a room is drawn.
- **Portals** (temporary keyword exits) — unplaced; annotated, not
  gridded (§6.5).
- **Terrain / biomes** ([biomes](biomes.md)) — drives glyph/color
  (§6.2).
- **Light and darkness** ([light-and-darkness](light-and-darkness.md))
  — the map is memory, not sight; v1 draws it regardless of current
  light, with an optional darkness-blank policy (§4, §10).
- **Hidden exits / visibility**
  ([hidden-exits](hidden-exits.md), [visibility](visibility.md)) — an
  undiscovered exit is not drawn (§6.6).
- **Roles** ([roles-and-permissions](roles-and-permissions.md)) — the
  `minimap`/`map` surfaces are **not** role-gated; they are ordinary
  player features. (Contrast the admin `roomdata` block.)

---

## 10. Configuration surface

| Setting | Default | Meaning |
|---|---|---|
| Minimap radius | small (terminal-sized) | The step/cell radius of the active minimap window around the player (§4). |
| Minimap placement | appended below the room view | Where the active minimap renders relative to the room body (§4). |
| Minimap toggle default | off | Whether a fresh character starts with the minimap on (§4). |
| `minimap` verb form | `minimap [on\|off]` | The toggle command surface (§4). |
| `map` extent | whole current area | What the `map` verb draws (§5); a bounded `map <radius>` form is a §11 follow-on. |
| Viewer glyph | policy | The "you are here" marker (§6.1). |
| Terrain → glyph/color tag | policy | Per-terrain cell glyph + semantic map tag, resolved by the theme (§6.2). |
| Door marking on connectors | policy | Whether/how open/closed/locked doors annotate connectors (§6.3). |
| Exit-stub rendering | connector into blank | How an exit to an unvisited room is drawn (§6.4). |
| Vertical-exit indicator | policy | The up/down marker on a room with a vertical exit (§6.5). |
| Keyword-exit annotation | policy | How a portal/keyword exit is noted beside the map (§6.5). |
| Darkness blanks the minimap | no | Whether zero-light suppresses the minimap (§4, light-and-darkness). |
| Visited-set pruning | prune against live rooms | When/whether stale ids are dropped (PD-10, §8). |

---

## 11. Open questions

- **`map` overflow on huge explored areas.** v1 lets `map` wrap/scroll
  when the explored area exceeds the viewport. A bounded `map <radius>`
  form, paging, or a "too large — try the minimap" cap are follow-ons;
  pick a default cap behavior if scrolling proves unusable.
- **Cross-area / world map.** v1 is single-area. Stitching areas into
  one overland view (or a stylized content world map) is a separate,
  later artifact (non-goal §1).
- **Speedwalk / click-to-walk.** Mudlet gets click-to-walk once it has
  a map; a server-side `walk <room>` (pathfind over the visited graph)
  is a natural follow-on but out of v1.
- **Darkness vs. the minimap.** Is the minimap "memory" (always shown)
  or "sight" (blanked in the dark)? v1 leans memory; a darkness-blank
  policy is stubbed (§10).
- **Exit-stub leak.** Stubbing an exit to an unvisited room reveals a
  neighbor *exists* (a mild leak, accepted in PD-3). If even that is
  unwanted for some content, a per-area "hide stubs" policy could
  follow.
- **GMCP wire shape.** Flat `x/y/z` vs. Mudlet's `coords` convention —
  resolved against a live client at GMCP-slice time (PD-9, carried from
  [room-coordinates](room-coordinates.md) §5).

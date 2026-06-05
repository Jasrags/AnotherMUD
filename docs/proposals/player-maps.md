# Proposal: Player Maps (ASCII + Mudlet/GMCP)

**Status:** Draft / for discussion · **Type:** Feature proposal (pre-spec) · **Audience:** engine
**Feeds into:** a future `player-maps.md` spec + plan
**Builds on:** [`docs/specs/room-coordinates.md`](../specs/room-coordinates.md) — the coordinate **substrate**, already specced. This proposal is the *feature* (the local-window query + the two renderers) that consumes it.

## Decisions taken so far (steering this draft)

Settled in review; the spec will inherit them. The biggest shift from the original proposal: **"where do coordinates come from" is no longer an open fork — it's resolved in the `room-coordinates` spec**, so this document's center of gravity moves up to the renderers and the windowing query.

- **Geometry source is settled (was §4 here).** Coordinates are **area-local integers derived from the exit graph at load, with a per-room `coord:` override/pin** for non-Euclidean spaces — the derive-with-override "Option C", scoped per area. See `room-coordinates.md` §3 (PD-1). This proposal does **not** re-open that; §4 below is now a pointer.
- **Coordinates are stable, not viewer-relative** (`room-coordinates.md` PD-7). A room's coordinate never depends on who is looking. This corrects the original proposal's "walk outward from the player's room" framing, which would have made coordinates change every step and broken Mudlet's persistent mapper. Player-centering is a **render-time translation**, not a re-derivation.
- **The "geometry layer" splits in two.** (1) *Coordinate assignment* — stable, per-area, once at load = the `room-coordinates` substrate. (2) *Local-window extraction* — "give me the rooms within radius N of room X, with their stable coordinates" = a per-request shared helper this feature owns, consumed by both renderers. The original "build once, render twice" instinct is right; the "build once" is the load-time coordinate pass, and the per-request window sits between it and the two renderers.
- **Non-fatal geometry.** Collisions / non-square loops / unplaced rooms are load-time warnings, never crashes, and a pin is the author's fix (`room-coordinates.md` §4).
- **Fog of war is IN v1, persisted (§7, decided).** The map shows only rooms the character has **entered**, and the explored set is **remembered across sessions**. This is the one part of the feature that adds **new per-character save state** — a persisted visited-room set + a player-save version bump + an exploration-tracking hook on room entry. The coordinate substrate stays omniscient and save-free (`room-coordinates.md` §8); fog of war is a *filter layered on top* of it, owned entirely by this feature. This reverses the original proposal's recommendation (which was reveal-all in v1) per an explicit decision.

## 1. Problem / motivation

The world is a graph of rooms, and right now the player holds that graph entirely in their head (or a notebook, or a hand-built Mudlet map). New players get lost; everyone navigates by memorized room names. A map is the standard remedy, and AnotherMUD now has the raw material *and the hard part solved*: rooms, areas, exits, doors, a `terrain` property, and — once `room-coordinates` ships — a stable area-local coordinate per room. Two natural delivery surfaces sit on top: a server-rendered ASCII minimap for any client, and structured map data for Mudlet's built-in graphical mapper over GMCP. The missing piece is no longer the geometry; it is the **windowing query** that pulls "the rooms near me" out of the coordinate set, and the **two renderers** on top of it.

## 2. Goals & non-goals

**Goals.** A `map` verb that renders an ASCII minimap of the player's surroundings, centered on their current room, usable on raw telnet with zero client cooperation. Structured map data over GMCP that Mudlet's native mapper consumes to draw and auto-update a graphical map as the player moves. Terrain-aware presentation (terrain drives glyph/color via the existing theme renderer). **Persisted fog of war** — the map reflects only rooms the character has explored, remembered across sessions (§7). Both surfaces drawing from **one shared local-window query** over the stable coordinate substrate, not two parallel graph walks.

**Non-goals (rule out now).** Not pathfinding or speedwalk in v1 — though Mudlet gets click-to-walk essentially for free *once it has a map*, so this is a near-term follow-on, not a permanent exclusion. Not a player-annotatable map (Mudlet already does client-side annotation; the server shouldn't duplicate it). Not an out-of-game web atlas. Not overland/zoomed-out "world map" cartography as a distinct mode — this is room-graph mapping; a stylized world map is a separate content artifact. Not the coordinate substrate itself — that is `room-coordinates.md`, already specced. *(Fog of war is **in** v1 — §7 — and was the prior draft's main deferral; it is now scoped in, with the per-character save state that implies.)*

## 3. Proposed approach (the shape)

**One windowing query, two renderers**, sitting on the stable coordinate substrate.

The **local-window query** is the shared seam. Given the player's current room and a radius, it returns the nearby same-area rooms (BFS to depth N over intra-area exits, intersected with *placed* rooms) carrying their **stable** area-local coordinates and the exits between them. It does not compute coordinates — it reads them. It does not center on anyone — it returns absolute area coordinates and lets each renderer recenter. This is the "build once" both renderers share.

The two renderers then diverge cleanly, because they serve different clients:

**ASCII minimap** is server-rendered text. It takes the window query's output, **translates** it so the player's room sits at grid-center (subtract the player's coordinate — this is how a viewer-independent coordinate becomes a player-centered view), draws a small grid (current room marked, exits as connectors, terrain as colored glyphs via the theme renderer), and sends it as ordinary output in response to the `map` verb. Works for *every* client including raw `telnet`/`nc`, because it's just text.

**Mudlet map data** is structured GMCP. It extends `Room.Info` (`room-coordinates.md` §5) with the room's **stable** coordinate, area, and exits, updated as the player moves so the map follows them. Because the coordinate is stable, Mudlet stores one fixed cell per room and updates incrementally — *this is exactly why the substrate forbids viewer-relative coordinates*. Mudlet draws natively; the server sends data, not pixels. Fits the existing "server knows state, GMCP surfaces it, client renders" pattern.

Both consume the same window query. **Fog of war is a filter on that query's output** (§7, decided): the window returns nearby placed rooms, then the feature intersects them with the character's persisted **visited set** before either renderer draws. A room the character has not entered is dropped (or shown as an unexplored exit-stub — a v1 sub-decision, §7). The visited set is updated on the room-entry path (the same `player.moved` / `SetRoom` seam the questwatch + AI-reset hooks already use) and persisted on the player save. That's the architecture; §6 names the risks.

The two memories worth distinguishing: the **server's visited set** (persisted, authoritative, governs the ASCII map and what the server is willing to reveal) and **Mudlet's own client-side map** (Mudlet remembers every `Room.Info` frame it receives). They stay aligned naturally if the server only emits `Room.Info` for the **current** room as the player moves — which is the existing cadence — so the server never bulk-reveals an unexplored area and Mudlet's map fills in exactly as the player walks. Fog of war therefore lands mostly in the ASCII renderer + the "don't bulk-send" GMCP policy, not as a second persistence layer for Mudlet.

## 4. The coordinate substrate (settled — see `room-coordinates.md`)

The original proposal's §4 was the geometry fork: authored vs. derived vs. hybrid, and where coordinates come from. **That is resolved** and lives in [`room-coordinates.md`](../specs/room-coordinates.md):

- **Derive-by-default with a per-room `coord:` override** (hybrid / Option C), scoped **per area** (§3, PD-1). Full coverage of existing content on day one; authors pin only the rooms that derive wrong.
- **Stable, viewer-independent** coordinates (PD-7) — the property this feature's Mudlet renderer depends on.
- **Recomputed/transient**, never persisted as state; the pin is content, not a save field (§8).
- **Non-fatal** collision / loop / unplaced policy (§4); a pin is the author's escape hatch.

This proposal consumes that substrate and does not re-decide it. The only substrate item still open that touches this feature is the **wire shape** of the GMCP coordinate (flat `x/y/z` vs. Mudlet's `coords` convention) — pinned against a live client at GMCP-slice time (`room-coordinates.md` §5 caveat, and §7 below).

## 5. Alternatives considered & rejected

- **A single unified renderer** (one code path emitting both ASCII and Mudlet data) — rejected: the targets are too different (server-drawn text for arbitrary clients vs. structured data drawn by an external program); forcing them together compromises both. Splitting at the **window query** keeps the shared logic shared and the divergent rendering divergent.
- **Client-side ASCII generation** (server sends only the graph, the client draws the grid) — rejected for the ASCII target specifically: raw telnet clients can't compute anything, and server-rendered text is the only thing that reaches every client, which is the whole point of an ASCII option alongside the Mudlet one.
- **Viewer-relative coordinates** (compute "outward from the player") — rejected at the substrate level (`room-coordinates.md` PD-7): it breaks Mudlet's persistent mapper. Player-centering is achieved by render-time translation instead, which costs nothing and keeps coordinates stable.
- **Full authored layout** (place every room by hand) — rejected as the baseline (it ships blank until content is back-filled); survives only as the *targeted* per-room pin.

## 6. Dependencies & risks

Enabling substrate already exists or is specced: the room graph (rooms/areas/exits/doors), the **coordinate substrate** (`room-coordinates.md`), the `terrain` property (glyph/color source), the theme renderer with semantic color tags (ASCII coloring), the command registry + typed args (the `map` verb), and the GMCP package layer with `Room.Info` to extend. No greenfield system is strictly required for v1 beyond `room-coordinates` shipping first.

Risks worth naming now (geometry-source risk is **gone** — it's resolved in the substrate):

- **Non-cardinal / keyword exits (rendering).** `room-coordinates` leaves portal/keyword-exit targets *unplaced* (no cell). The **renderer** still knows the current room has such an exit (it's in the room's keyword-exit list) and must decide whether to draw it as a special connector, an annotation ("a shimmering portal"), or omit it. A rendering decision, not a geometry one; interacts with the temporary-portal system.
- **Visibility / information leak (both renderers).** The coordinate set is omniscient — it includes secret passages and unvisited rooms (`room-coordinates.md` §6 visibility note). The v1 **fog-of-war filter** (§7) already closes most of this: an unexplored room is never drawn, so the map can't reveal layout the player hasn't earned. Two residual leaks remain for the spec to address: (a) a **secret exit inside an already-visited room** — discovered-or-not concealment is per-observer ([visibility](../specs/visibility.md)/[hidden-exits](../specs/hidden-exits.md)), so the map must not draw an undiscovered hidden exit's connector even when both its rooms are visited; and (b) **exit-stubs to unvisited rooms** — if v1 shows "there's an exit north to somewhere you haven't been", that still leaks the *existence* of a neighbor (a milder leak, and a §7 sub-decision).
- **Z-levels (rendering asymmetry).** Up/down make the local map 3D. The two renderers handle it *asymmetrically*: ASCII likely shows one floor with up/down indicators (`<` / `>` glyphs or a layer toggle); Mudlet handles z natively. Worth settling explicitly in the spec.
- **Mudlet protocol specifics.** The exact GMCP schema Mudlet's mapper expects (field layout, area-id contract, update flow) must be **pinned against a live Mudlet client**, not guessed — the substrate commits to "area-local integer coordinate on `Room.Info`", not the wire shape (`room-coordinates.md` §5 caveat).

## 7. Open questions (for sign-off before the spec)

- **Fog of war — DECIDED: persisted.** v1 shows only rooms the
  character has **entered**, remembered across sessions. This is the
  one part of the feature that adds **new per-character save state**.
  Settled scope:
  - A persisted **visited-room set** on the player save (namespaced
    room ids), `player.CurrentVersion` bump + append-only migration
    (existing characters start with an empty set; the spec may
    optionally back-fill the current room on first load).
  - An **exploration hook** on room entry (`player.moved`/`SetRoom`)
    that adds the room to the set + marks the save dirty.
  - Both renderers **filter** the window query against the set
    (§3); Mudlet stays in sync via the current-room-only emission
    cadence (§3), so no separate Mudlet persistence is built.
  - **Sub-decisions still open:** (a) do **unvisited neighbors of a
    visited room** show as exit-stubs ("an exit leads north,
    unexplored") or are they fully hidden until entered? — leaning
    *stub the exit, hide the room* (you can see a door from inside a
    room you're standing in); (b) does an admin/wizinvis or a
    `teleport` arrival count as "visited"? — leaning yes, entering is
    entering; (c) is the set ever **prunable** (areas removed from
    content) or does it grow unbounded? — leaning prune-on-load
    against the live room set.
- **ASCII map extent.** A fixed local radius around the player, or the whole current area? *Recommendation: a configurable local radius for the `map` verb* (keeps output terminal-sized), with a possible separate "area map" mode as a follow-on. (Fog of war bounds this further — even an "area map" only shows visited rooms.)
- **Phasing.** Ship ASCII and Mudlet together or sequentially? *Recommendation: substrate (`room-coordinates`) → fog-of-war visited-set + exploration hook → ASCII renderer (validates geometry **and** fog visibly, serves every client) → Mudlet GMCP surface on the same window query.* Confirm the ordering.
- **GMCP wire shape.** Resolve the flat `x/y/z` vs. Mudlet `coords`-array question against a live client before the Mudlet slice is called done (carried from `room-coordinates.md` §5).
- **Non-cardinal exit rendering.** Connector, annotation, or omit (see §6) — a v1 default to pick.

## 8. Rough sizing

The substrate (`room-coordinates`) is the bulk of the *thinking* and is already specced. On top of it: the **local-window query** is small and self-contained (a bounded BFS over placed rooms returning their coordinates). **Fog of war** is the one piece now scoped into v1 that touches persistence — a visited-set field + version-bump migration + a room-entry hook + the query filter; mechanically modest, but it's the part that needs care (save migration is append-only, the hook rides an existing seam). The **ASCII renderer** is modest (a grid draw + render-time recenter over the theme renderer, with the fog filter applied). The **Mudlet surface** is mostly extending existing `Room.Info` GMCP plus getting the schema right against a real client (and the "current-room-only" emission policy that keeps fog of war honest). Area-wide maps and speedwalk remain separable later increments the v1 architecture leaves room for without building.

---

*Acceptance criteria and the configuration-surface table are deliberately omitted — those are spec-level. The two forks that dominated earlier drafts are both now settled: the **geometry source** is decided in [`room-coordinates.md`](../specs/room-coordinates.md) (derive-with-override), and **fog of war** is decided here (§7: persisted, in v1). What remains for the spec are the §7 sub-decisions (exit-stubs, teleport-counts-as-visited, set pruning) and the cross-cutting visibility/hidden-exit interaction (§6) — all bounded. The feature is ready to become a `docs/specs/player-maps.md` once `room-coordinates` ships.*

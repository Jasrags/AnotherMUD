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

## 1. Problem / motivation

The world is a graph of rooms, and right now the player holds that graph entirely in their head (or a notebook, or a hand-built Mudlet map). New players get lost; everyone navigates by memorized room names. A map is the standard remedy, and AnotherMUD now has the raw material *and the hard part solved*: rooms, areas, exits, doors, a `terrain` property, and — once `room-coordinates` ships — a stable area-local coordinate per room. Two natural delivery surfaces sit on top: a server-rendered ASCII minimap for any client, and structured map data for Mudlet's built-in graphical mapper over GMCP. The missing piece is no longer the geometry; it is the **windowing query** that pulls "the rooms near me" out of the coordinate set, and the **two renderers** on top of it.

## 2. Goals & non-goals

**Goals.** A `map` verb that renders an ASCII minimap of the player's surroundings, centered on their current room, usable on raw telnet with zero client cooperation. Structured map data over GMCP that Mudlet's native mapper consumes to draw and auto-update a graphical map as the player moves. Terrain-aware presentation (terrain drives glyph/color via the existing theme renderer). Both surfaces drawing from **one shared local-window query** over the stable coordinate substrate, not two parallel graph walks.

**Non-goals (rule out now).** Not pathfinding or speedwalk in v1 — though Mudlet gets click-to-walk essentially for free *once it has a map*, so this is a near-term follow-on, not a permanent exclusion. Not a player-annotatable map (Mudlet already does client-side annotation; the server shouldn't duplicate it). Not an out-of-game web atlas. Not overland/zoomed-out "world map" cartography as a distinct mode — this is room-graph mapping; a stylized world map is a separate content artifact. And explicitly **not** a fog-of-war exploration system in v1 unless §7 decides otherwise (it adds new persisted per-character state). Not the coordinate substrate itself — that is `room-coordinates.md`, already specced.

## 3. Proposed approach (the shape)

**One windowing query, two renderers**, sitting on the stable coordinate substrate.

The **local-window query** is the shared seam. Given the player's current room and a radius, it returns the nearby same-area rooms (BFS to depth N over intra-area exits, intersected with *placed* rooms) carrying their **stable** area-local coordinates and the exits between them. It does not compute coordinates — it reads them. It does not center on anyone — it returns absolute area coordinates and lets each renderer recenter. This is the "build once" both renderers share.

The two renderers then diverge cleanly, because they serve different clients:

**ASCII minimap** is server-rendered text. It takes the window query's output, **translates** it so the player's room sits at grid-center (subtract the player's coordinate — this is how a viewer-independent coordinate becomes a player-centered view), draws a small grid (current room marked, exits as connectors, terrain as colored glyphs via the theme renderer), and sends it as ordinary output in response to the `map` verb. Works for *every* client including raw `telnet`/`nc`, because it's just text.

**Mudlet map data** is structured GMCP. It extends `Room.Info` (`room-coordinates.md` §5) with the room's **stable** coordinate, area, and exits, updated as the player moves so the map follows them. Because the coordinate is stable, Mudlet stores one fixed cell per room and updates incrementally — *this is exactly why the substrate forbids viewer-relative coordinates*. Mudlet draws natively; the server sends data, not pixels. Fits the existing "server knows state, GMCP surfaces it, client renders" pattern.

Both consume the same window query. That's the architecture; §6 names the risks.

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
- **Visibility / information leak (both renderers).** The coordinate set is omniscient — it includes secret passages and unvisited rooms (`room-coordinates.md` §6 visibility note). Today `CanSee` is permissive so nothing leaks *yet*, but **both** the ASCII map and the GMCP feed are textbook leak vectors once [visibility](../specs/visibility.md) and any exploration model land. The spec must state that rendered/emitted rooms respect visibility and exploration, and decide the v1 reveal-whole-area vs. visited-only question (§7).
- **Z-levels (rendering asymmetry).** Up/down make the local map 3D. The two renderers handle it *asymmetrically*: ASCII likely shows one floor with up/down indicators (`<` / `>` glyphs or a layer toggle); Mudlet handles z natively. Worth settling explicitly in the spec.
- **Mudlet protocol specifics.** The exact GMCP schema Mudlet's mapper expects (field layout, area-id contract, update flow) must be **pinned against a live Mudlet client**, not guessed — the substrate commits to "area-local integer coordinate on `Room.Info`", not the wire shape (`room-coordinates.md` §5 caveat).

## 7. Open questions (for sign-off before the spec)

- **Fog of war.** Does v1 reveal the **full area** around the player, or only **rooms they've visited**? *Recommendation: reveal the local area in v1.* Fog of war needs new per-character persisted "visited" state and an exploration model — a meaningful addition better made deliberately later than smuggled into v1. (Note: this is the one piece that would introduce new save state; the coordinate substrate deliberately adds none.)
- **ASCII map extent.** A fixed local radius around the player, or the whole current area? *Recommendation: a configurable local radius for the `map` verb* (keeps output terminal-sized), with a possible separate "area map" mode as a follow-on.
- **Phasing.** Ship ASCII and Mudlet together or sequentially? *Recommendation: substrate (`room-coordinates`) → ASCII renderer (validates the geometry visibly, serves every client) → Mudlet GMCP surface on the same window query.* Confirm the ordering.
- **GMCP wire shape.** Resolve the flat `x/y/z` vs. Mudlet `coords`-array question against a live client before the Mudlet slice is called done (carried from `room-coordinates.md` §5).
- **Non-cardinal exit rendering.** Connector, annotation, or omit (see §6) — a v1 default to pick.

## 8. Rough sizing

The substrate (`room-coordinates`) is the bulk of the *thinking* and is already specced. On top of it: the **local-window query** is small and self-contained (a bounded BFS over placed rooms returning their coordinates). The **ASCII renderer** is modest (a grid draw + render-time recenter over the theme renderer). The **Mudlet surface** is mostly extending existing `Room.Info` GMCP plus getting the schema right against a real client. Fog of war, area-wide maps, and speedwalk are each separable later increments the v1 architecture should leave room for without building.

---

*Acceptance criteria and the configuration-surface table are deliberately omitted — those are spec-level once §7's fog-of-war and extent questions are settled. The geometry fork that dominated the first draft is no longer here; it is decided in [`room-coordinates.md`](../specs/room-coordinates.md). The decision with the most downstream weight remaining is **fog of war** (§7), since "only visited rooms" quietly introduces new persisted per-character state.*

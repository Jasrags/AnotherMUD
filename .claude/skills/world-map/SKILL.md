---
name: world-map
description: Render the AnotherMUD world content (rooms, areas, regions, exits, NPCs) into a single self-contained interactive HTML map and open it. Use when the user wants to SEE the world from a map viewpoint — visualize the layout, check how areas connect, eyeball a region, or review newly-authored geography. Triggers include "show me the world map", "visualize the world/content", "what does the map look like", "render the map", after authoring rooms/areas, or any request to see the geography as a map.
user-invocable: true
---

# World Map

Generate and open an interactive HTML map of the AnotherMUD world content. The
map is produced by the in-repo Go tool `cmd/worldmap`, which parses a pack's
`areas/`, `rooms/`, and `mobs/` YAML directly (no server boot) and lays every
room out with a BFS over the exit graph — mirroring the engine's own coordinate
derivation (`internal/world/coords.go`: north = +y, east = +x, up = +z). Output
is a dependency-free `docs/maps/world.html`.

## When to use

- The user asks to *see* / *visualize* / *map* the world or a region.
- After authoring or editing rooms/areas — to eyeball connectivity and layout.
- To sanity-check a new region, road, or cross-area seam visually.

## How to run

From the repo root:

```bash
make worldmap                 # renders docs/maps/world.html for the wot pack
# or directly, with options:
go run ./cmd/worldmap -pack wot -start the-green -out docs/maps/world.html
```

Flags: `-pack` (default `wot`), `-start` (BFS seed / spawn marker, default
`the-green`), `-content` (default `./content`), `-out` (default
`docs/maps/world.html`).

Then open it for the user:

```bash
open docs/maps/world.html     # macOS
```

(On another platform, give the path; the file is fully self-contained and opens
in any browser.)

## What the map shows

- **Every room** as a region-tinted card, positioned by compass geography
  (north up). Spawn ◉, shop ⛁, trainer ⚒, items ▪, and stairs ⇡⇣ badges; a
  terrain dot.
- **Exits** as lines between rooms; **cross-area** roads are dashed gold.
- **Regions** color-coded (Two Rivers, Andor, …) with a clickable legend filter.
- **Interactions:** drag to pan, wheel to zoom, click a room for a detail panel
  (area, region, terrain, NPCs with shop/trainer roles, every exit — click an
  exit to jump), a search box (name or id), z-level toggles (ground / upstairs /
  diggings), and "Recenter".

## Procedure

1. Confirm you are in the repo root (the tool reads `./content`).
2. Run `make worldmap` (or the `go run` form for a different pack/start/output).
3. Report the room/area counts from the tool's stdout.
4. Open the HTML (`open docs/maps/world.html` on macOS) or give the path.

## Notes

- The map is **static content** — it reflects the YAML on disk, not a running
  server. Re-run after authoring to refresh.
- Layout is a best-effort grid from the exit graph; where the world graph folds
  back on itself the BFS spreads colliding rooms to the nearest free cell, so a
  few rooms may sit one cell off true compass position. Exits are always drawn,
  so connectivity stays correct.
- `docs/maps/world.html` is a committed generated artifact — regenerate via this
  skill / `make worldmap` rather than hand-editing it.

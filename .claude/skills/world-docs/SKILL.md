---
name: world-docs
description: Generate documentation for the AnotherMUD world content (rooms, areas, regions, exits, NPCs, mobs, items, recipes, factions, quests) from the content packs — an interactive self-contained HTML map, a region→area→room gazetteer, and content catalogs per world pack, plus a cross-pack index (a health report and a player guide land in later phases). Use when the user wants to SEE or document the world — visualize the layout, check how areas connect, eyeball a region, review newly-authored geography, list what mobs/items/recipes/quests exist, or regenerate the world docs. Triggers include "show me the world map", "visualize the world/content", "what does the map look like", "render the map", "generate world docs", "catalog the items/mobs/quests", after authoring rooms/areas, or any request to see the geography as a map.
user-invocable: true
---

# World Docs

Generate the AnotherMUD world documentation from the content packs. The docs are
produced by the in-repo Go tool `cmd/worlddoc`, which parses a pack's `areas/`,
`rooms/`, `mobs/`, and `quests/` YAML directly (no server boot) and lays every
room out with a BFS over the exit graph — mirroring the engine's own coordinate
derivation (`internal/world/coords.go`: north = +y, east = +x, up = +z).

The output is a small **styled HTML site** — every page shares a left-sidebar nav
(section links + a pack switcher) and a common parchment theme lifted from the
map. It lives under `docs/world/`:

- `docs/world/index.html` — the cross-pack landing (a card per world pack; written
  on a full run).
- `docs/world/<pack>/index.html` — the pack's **Overview**: summary, regions, and
  cards linking to each section.
- `docs/world/<pack>/map.html` — the dependency-free interactive map (with a
  "◀ Docs" back link).
- `docs/world/<pack>/gazetteer.html` — a region→area→room reference (exits with
  door/locked/hidden markers, resident NPCs with roles, per-room notes).
- `docs/world/<pack>/catalogs.html` — reference tables of what the pack ships
  (mobs with room placement + roles, items with stats, recipes with inputs→output,
  factions, quests with reward summaries).
- `docs/world/<pack>/health.html` — an authoring-gap audit (report only, never
  fails): unreachable/orphan rooms, dangling exit targets, one-way exits,
  undescribed rooms, empty areas, unknown mob refs, dangling quest givers/reward
  factions.
- `docs/world/<pack>/guide.html` — a player-facing orientation assembled from the
  world itself: where you start, a region→area tour, and where to find services.

The tool is built as a shared parse feeding a registry of **emitters** (`overview`,
`map`, `gazetteer`, `catalogs`, `health`, `guide`), each rendered into the shared
page shell (`html/template`, so content is auto-escaped). See
`docs/plans/world-docs-plan.md` for the design.

## When to use

- The user asks to *see* / *visualize* / *map* / *document* the world or a region.
- After authoring or editing rooms/areas — to eyeball connectivity and layout.
- To sanity-check a new region, road, or cross-area seam visually.

## How to run

From the repo root:

```bash
make worlddoc                         # every world pack → docs/world/ (full site)
# or directly:
go run ./cmd/worlddoc -pack all       # all kind:world packs + docs/world/index.html
go run ./cmd/worlddoc -pack wot -start the-green -emit map   # one pack, map only
```

Flags: `-pack` (`wot` default, or `all` for every kind:world pack), `-start`
(BFS seed / spawn marker, default `the-green`; ignored for `-pack all`, which
seeds each pack from a built-in default), `-content` (default `./content`),
`-emit` (`all` default, or a single emitter — `overview`, `map`, `gazetteer`,
`catalogs`, `health`, or `guide`), `-outdir` (default `docs/world`).

Then open the map for the user:

```bash
open docs/world/index.html            # macOS — the site landing (or wot/map.html for the map)
```

(On another platform, give the path; the file is fully self-contained and opens
in any browser.)

## What the map shows

- **Every room** as a region-tinted card, positioned by compass geography
  (north up), with a terrain dot and **feature badges** drawn from the room's
  content:
  - ◉ spawn · ⛁ shop · ⚒ trainer · ⚙ craft station · ▪ items · ⇡⇣ stairs
  - 🐎 stable/mount · ⚔ hire (recruiter/hireling) · ❗ quest giver · ⚑ faction NPC
    · ☠ hostile mob
  - 🔒 locked door · 🔍 hidden exit · 🌙 dark room
- **Exits** as lines: normal tan, **cross-area** dashed gold, **locked** dashed
  red, **hidden** faint red dotted (toggle with the "🔍 hidden" button).
- **Regions** color-coded (Two Rivers, Andor, …) with a clickable legend filter.
- **Feature filter:** a chip per feature present in the pack. Toggle chips to
  keep matching rooms bright and **dim the rest** (multiple chips = OR — any
  selected feature). E.g. click 🐎 + 🔒 to spotlight stables and locked caches.
- **Search** matches room name/id, **NPC names, feature keywords, terrain, and
  weather zone** — so `recruiter`, `stable`, `guard`, `locked`, `hidden`, or a
  mob name all find rooms (and jump to the first match, switching z-level if
  needed).
- **Detail panel** (click a room): area, region, terrain, **light level**,
  **weather zone**, the room's feature tags, NPCs with role glyphs, and every
  exit (marked 🔒 locked / 🔍 hidden) — click an exit to jump.
- **Legend:** a "Legend" button opens a panel explaining every badge glyph, the
  four exit/door line styles, terrain-dot colors, and region tints — built from
  the same data the map renders, so it stays in sync.
- **Controls:** drag to pan, wheel to zoom, z-level toggles (ground / upstairs /
  diggings), the hidden-exit toggle, and "Recenter".

### Feature detection (what maps to each badge)

The renderer reads these YAML fields directly — add content in those shapes and
it shows up on the next `make worlddoc`:

| Badge | Source |
|-------|--------|
| ⛁ shop / ⚒ trainer | mob `properties.shop`/`shop` tag · `trainer:`/`skill_trainer` tag |
| 🐎 stable | mob `properties.stable` or `stable` tag |
| ⚔ hire | mob `hireling:` or `recruiter:` block |
| ❗ quest | mob is a quest `giver:` (from `quests/*.yaml`) |
| ⚑ faction | mob `faction:` field |
| ☠ hostile | mob `disposition_rules.default: hostile` |
| 🔒 locked / 🔍 hidden | room `doors.<dir>.locked` · `hidden_exits:` |
| ⚙ craft / 🌙 dark | room `properties.craft_stations` · `properties.light: black` |
| weather (panel) | area `weather_zone:` |

## Procedure

1. Confirm you are in the repo root (the tool reads `./content`).
2. Run `make worlddoc` (all world packs), or `go run ./cmd/worlddoc` with
   `-pack`/`-start`/`-emit` for a single pack/emitter.
3. Report the room/area counts from the tool's stdout.
4. Open the site (`open docs/world/index.html` on macOS, or `<pack>/map.html` for
   the map) or give the path.

## Notes

- The docs are **static content** — they reflect the YAML on disk, not a running
  server. Re-run after authoring to refresh.
- Layout is a best-effort grid from the exit graph; where the world graph folds
  back on itself the BFS spreads colliding rooms to the nearest free cell, so a
  few rooms may sit one cell off true compass position. Exits are always drawn,
  so connectivity stays correct.
- Everything under `docs/world/` is a committed **generated** artifact —
  regenerate via this skill / `make worlddoc` rather than hand-editing it.

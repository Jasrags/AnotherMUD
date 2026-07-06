# World Documentation — Implementation Plan

**Status:** Shipped — all six phases landed (`cmd/worlddoc`, emitters map /
gazetteer / catalogs / health / guide, cross-pack index, `world-docs` skill).
This doc is kept as the design record; the open questions below remain live.
**Audience:** the build sequence. **Superseded:** the standalone `cmd/worldmap` +
`world-map` skill, now one emitter of the larger system.

A phased plan to promote the interactive world map from a one-off view into a
**content-documentation system**: the pack YAML is the single source of truth,
parsed once, and rendered into several coordinated, audience-layered views —
the map being one of them. Everything is **deterministic and regenerable**
(derived output, like `docs/CODEMAPS/` — regenerate, never hand-edit). No
engine boot; no LLM in the loop.

---

## Guiding decisions (locked)

| Decision | Choice |
|---|---|
| **Artifacts** | Map (exists) + gazetteer + content catalogs + world health report + player guide |
| **Audience** | All three, layered — one generator, output tuned per section |
| **Mechanism** | Restructure `docs/` first, then extend `cmd/worldmap` → `cmd/worlddoc` |
| **Packs** | All active packs at once, plus a cross-pack index |
| **Guide prose** | Fully deterministic — assembled from YAML fields, always in sync |
| **Health report** | Report only — lists gaps, never fails a build (no exit code) |
| **Format** | Markdown for text artifacts (diffable/greppable); HTML only for the map |

**Why this shape:** the map is one *rendering* of the content. The natural
umbrella is a generator that emits many renderings from the same parse. Keeping
the guide deterministic (no hand-authored prose, no LLM) means the whole system
stays regenerable with zero drift — the same property that makes CODEMAPS
trustworthy.

**Scope boundary:** this is **content/world documentation** (derived from
packs). It is a distinct axis from **code documentation** (`docs/CODEMAPS/`,
`/update-codemaps`) and **spec documentation** (`docs/specs/`, behavioral
source of truth). This plan does not fold those in.

---

## What the parse already has (lean on vs. new)

The existing `cmd/worldmap/main.go` already parses the fields every emitter
needs — this is mostly *new emitters over an existing parse*, not new parsing.

| Need | Source | Status |
|---|---|---|
| Areas: id, name, region, weather_zone | `areaYAML` | parsed today |
| Rooms: id, area, name, terrain, exits, mobs, items, properties, doors, hidden_exits | `roomYAML` | parsed today |
| Mobs: id, name, tags, trainer/mount/hireling/recruiter, faction, disposition | `mobYAML` | parsed today |
| Quests: giver link | `questYAML` | parsed today (giver only) |
| Coordinate derivation (BFS over exit graph) | `worldmap` layout walk | built |
| Feature classification (shop/trainer/craft/hostile/…) | `roomJSON.Features` | built |
| Items: id, name, rarity, slots, stats | items/*.yaml | **new parse** (map only flags presence) |
| Recipes: inputs, outputs, station | recipes/*.yaml | **new parse** |
| Factions: id, name, axes | core/wot faction content | **new parse** |
| Quests: full (name, steps, rewards, prereqs) | quests/*.yaml | **extend parse** (today: giver only) |

New parsing is additive — the map's shapes are deliberately partial (only what
badges need); the catalog emitters read the fuller shape.

---

## Target layout (Phase 1 output)

```
docs/world/
  index.md                         # cross-pack index — every active pack, linked
  <pack>/                          # e.g. starter-world/, wot/
    map.html                       # interactive (moved from docs/maps/world.html)
    gazetteer.md                   # region → area → room, exits, NPCs
    catalogs/
      mobs.md  items.md  recipes.md  factions.md  quests.md
    health.md                      # authoring-gap audit (report only)
    guide.md                       # player-facing, deterministic prose
```

`docs/maps/world.html` is retired in favor of `docs/world/<pack>/map.html`.

---

## Phases (dependency-ordered)

### Phase 1 — Restructure `docs/`
- Create `docs/world/` as the generated-docs home.
- Move `cmd/worldmap`'s default output to `docs/world/<pack>/map.html`.
- Update the `-out` default + any references (README, skill).
- No behavior change yet — this is the home the emitters write into.
- **Acceptance:** `go run ./cmd/worldmap -pack starter-world` writes to the new
  path; the old `docs/maps/world.html` is removed; nothing else references it.

### Phase 2 — Rename & multi-emitter scaffold
- `cmd/worldmap` → `cmd/worlddoc`; map generation becomes one emitter behind a
  shared parse + a per-pack driver.
- Add an `-emit` selector (`all` default; `map`, `gazetteer`, `catalogs`,
  `health`, `guide`) and `-pack all` looping over active packs.
- Emit `docs/world/index.md` linking every pack's artifacts.
- The `world-map` skill becomes `world-docs` (map = a subset it can still target).
- **Acceptance:** `go run ./cmd/worlddoc -pack all` regenerates the map for
  every pack + an index; `-emit map` reproduces today's behavior byte-for-byte.

### Phase 3 — Gazetteer emitter
- `gazetteer.md`: region → area → room hierarchy. Per room: name, terrain,
  exits (with door/locked/hidden markers), resident NPCs, item/spawn/station
  presence. Deterministic ordering (region, then BFS/id).
- **Acceptance:** every room in the pack appears exactly once under its area;
  exits render direction + destination; cross-area exits marked.

### Phase 4 — Content catalogs
- `catalogs/{mobs,items,recipes,factions,quests}.md` as reference tables.
- Cross-reference placement: each mob lists rooms it spawns in; each quest lists
  its giver + reward; each item lists sources (loot/recipe/shop) where derivable.
- **Acceptance:** counts match the pack's YAML; every referenced id resolves or
  is flagged (feeds the health report).

### Phase 5 — World health report
- `health.md`, **report only** (no exit code). Checks:
  - Orphan rooms (no inbound exits) and unreachable rooms (not in the BFS from start).
  - One-way exits (A→B with no B→A) — informational, not always a bug.
  - Rooms with empty/missing description.
  - Dangling referenced ids (mob/quest/item/faction/door-key that doesn't exist).
  - Areas with no rooms; quest givers that aren't placed.
- **Acceptance:** a deliberately broken fixture surfaces each class of gap;
  a clean pack reports zero.

### Phase 6 — Player guide (deterministic)
- `guide.md`: player-facing, assembled straight from YAML — region intros from
  area descriptions, notable locations (shops/trainers/quest givers), a
  getting-started orientation from the start room outward. No hand-authored
  prose; regenerates in sync.
- **Acceptance:** guide regenerates identically on repeat runs; every region
  with content appears; no room/mob referenced that doesn't exist.

---

## Audience layering (how sections map)

| Audience | Primary artifacts |
|---|---|
| World authors / designers | gazetteer, catalogs, health |
| Engine developers | health (gaps), catalogs (cross-refs) |
| Players | guide, map |

---

## Regeneration discipline

- Output is **derived** — regenerate, never hand-edit (mirror the CODEMAPS rule).
- The `world-docs` skill wraps `cmd/worlddoc` and opens the map/index.
- The **health report** is the artifact with ongoing value: run it after
  authoring to catch gaps. (Report-only now; a `-strict` exit-code mode is a
  possible later addition if it ever gates CI — explicitly out of scope here.)

---

## Open questions (preserve, don't delete)

- **Item source derivation** (Phase 4): "where does this item come from" needs
  loot-table + recipe + shop cross-referencing; how deep to trace before it's
  just noise?
- **One-way exits** (Phase 5): common and often intentional (chutes, portals).
  List informationally vs. suppress by a room/exit property?
- **Cross-pack index depth**: flat links per pack, or a rolled-up summary
  (room/area/mob counts) across packs?
- **Guide start point**: derive orientation from `ANOTHERMUD_START_ROOM` per
  pack, or the pack's declared start?

# Crafting & Cooking — Implementation Plan

**Source:** `docs/specs/crafting-and-cooking.md` (the spec) + the original
design brief. **Status:** Planning — no code yet. **Audience:** the build
sequence to review before implementation begins.

This is a phased, dependency-ordered build plan shaped for AnotherMUD.
Unlike a from-scratch system, most of crafting is **wiring to systems that
already exist** — the spec deliberately leans on rarity, proficiency,
effects, sustenance, shops, quests, loot, terrain, weather, and the
Roller. Only two pieces are new substrate (stations, gathering), and both
sit **past the MVP cut**.

---

## Dependency map (what we lean on vs. what's new)

| Need | Source | Status |
|---|---|---|
| Output quality tiers | rarity tiers — `item-decorations.md` | specced |
| Crafting skill + skillup | proficiency + crafting track — `progression.md` §3/§3.5 | built |
| Quality-roll RNG | `progression.Roller` | built |
| Well-fed buff | EffectTemplate + consumable effect pipeline (M14.2) | built |
| Hunger clear | sustenance pool — `economy-survival.md` §3 | built |
| Item instances + atomic consume | `inventory-equipment-items.md` | built |
| Terrain/weather station gating | `world-rooms-movement.md` §6 | built |
| Common recipes (gold sink) | shops — `economy-survival.md` §3 | built |
| Uncommon recipes | quest rewards — `quests.md` | built |
| Rare recipes / ingredients | mob loot — `mobs-ai-spawning.md` §6.3 | built |
| **Recipe registry + schema** | new (mirrors ability/item registries) | **build** |
| **Crafting resolution + quality roll** | new | **build** |
| Tier 2 station | room tag/property — M14.5 (**decided**) | **build (light wiring)** |
| Tier 1 campfire | temporary placed entity — M15.2 decay pattern (**decided**) | **build (reuses substrate, in MVP)** |
| **Gathering / resource nodes** | new — overlaps biomes | **deferred** |

---

## Phases (dependency-ordered)

### Phase 0 — Recipe substrate *(data model)*
- **Build:** a recipe registry + schema (pack content, mirroring the
  ability/item registries) — inputs (kind + qty + min quality), output,
  discipline, skill floor, station-tier floor, tool, time, acquisition
  tier. Per-character **known-recipes** state + persistence (a list on the
  player save, like proficiencies/abilities; save-version bump).
- **Leans on:** pack loader, player save, progression proficiency.
- **Blockers:** none. Pure substrate.
- **Done when:** a content recipe loads; a character can know/persist
  recipe ids; unknown ids load cleanly.

### Phase 1 — Crafting skills *(mostly wiring)*
- **Build:** a crafting **discipline = a proficiency on a crafting track**
  (the track is already anticipated in `progression.md` §3). Grant
  baseline recipes on acquiring the discipline. Reuse §3.5 use-based gain
  for skillup — no new XP path.
- **Leans on:** progression track + proficiency + gain (built).
- **Blockers:** none.
- **Done when:** acquiring a discipline grants baseline recipes and a craft
  rolls a proficiency gain.

### Phase 2 — Tier 0 crafting + the quality roll *(MVP spine)*
- **Build:** the `craft <recipe>` verb; atomic input-reserve → consume →
  produce-output (the M5 two-actor/atomic discipline); the **quality roll**
  (§5): weighted score from skill + tool quality + ingredient quality →
  RNG band via `Roller` → clamp to the **Tier 0 ceiling** → stamp the
  output with a **rarity-tier instance property**.
- **Leans on:** items/instances, rarity tiers, Roller, proficiency.
- **Blockers:** the quality-roll **weights** need first-pass numbers
  (config surface §10) — tune-able, not a design block.
- **Done when:** a player crafts a known Tier-0 recipe anywhere, gets a
  varying-but-capped quality output that renders its tier.

### Phase 3 — Cooking + sustenance / well-fed *(MVP cooking)*
- **Build:** cooked-food outputs are consumables; eating clears sustenance
  (existing) **and** applies a **well-fed `EffectTemplate`** whose tier is
  selected by the food's crafted quality. Content: starter cooking recipes
  + well-fed effect templates per tier. Cold-ration (Tier 0) = weak/no
  effect.
- **Leans on:** sustenance pool, consumable effect pipeline (M14.2),
  EffectTemplate registry.
- **Blockers:** none (the effect pipeline exists).
- **Done when:** a cooked meal clears hunger and grants a quality-scaled
  well-fed buff that refreshes (not stacks) on re-eat.

### Phase 4 — Tier 2 fixed stations *(MVP town crafting)*
- **Build:** **decided model — a room tag/property** marks a room as a
  Tier 2 station for one or more disciplines (a `forge` / `kitchen` room).
  The craft reads the room's station tag to set the quality ceiling. Place
  Tier 2 station tags in `core`-pack town rooms. Portable-tool item raises
  the field ceiling one tier (a property read at craft time).
- **Leans on:** room tags/properties (M14.5), content placement, the
  ceiling clamp from Phase 2.
- **Blockers:** none — model decided (room-tag).
- **Done when:** crafting in a town `forge` room reaches higher quality
  than the same recipe in the field; a portable tool narrows that gap.

### Phase 5 — Tier 1 improvised stations *(MVP field crafting)*
- **Build:** a `build`-style action creates a **campfire** as a temporary
  placed entity, **reusing the M15.2 temporary-exit/portal decay pattern**
  (TTL + cleanup tick) — no new furniture system — that applies a station
  tag to the room while it lives. Consumes fuel/material + time;
  **refused by terrain/weather** (read room terrain + weather, M15); decays
  on TTL. Test content (campfire buildable + fuel item) in the `core` pack.
- **Leans on:** the entity store + placement, the M15.2 decay/cleanup
  pattern, terrain + weather (M15), the station-tag read from Phase 4.
- **Blockers:** none — reuses existing substrate (this is why it's in MVP).
- **Done when:** a player builds a campfire in an eligible room, cooks at
  Tier 1 quality there, and the campfire decays; building is refused in
  water/storm.

> ### ⎯⎯ MVP CUT LINE ⎯⎯
> Phases 0–5 deliver a **complete playable loop**: learn a discipline →
> gather ingredients (mob loot + authored placement) → craft anywhere at
> Tier 0 → build a campfire for Tier 1 field cooking → craft best at a town
> Tier 2 station → cook meals that buff. Quality renders through rarity;
> skill grows through use. All MVP test content lives in the `core` pack
> (a real content pack follows once features lock). Everything below is
> breadth and depth, not the core loop.

---

## Deferred (post-MVP)

### Phase 6 — Recipe-acquisition breadth
- **Build/content:** common recipes in **shop** stock (availability by
  skill — the gold sink); uncommon recipes as **quest** rewards; rare
  recipes from **mob loot** / containers. Mostly content + thin wiring on
  built systems. (Baseline already lands in Phase 1.)

### Phase 7 — Regional recipe sets + guided discovery
- **Build/content:** "learnable only from region/source X" recipe gating
  (engine mechanism) + the **region content** that fills it, and
  discovery-hint items (partial pages, NPC clues).
- **Blocker:** the **setting/geography content** — no geography reference
  in this repo yet; author the regions (the `mud-world-builder` skill)
  before regional content. Engine mechanism can land first.

### Phase 8 — Gathering / resource nodes
- **Build:** the non-vendor ingredient source (forage/harvest nodes),
  replacing the MVP's mob-loot/authored sourcing. **Greenfield; design
  together with Biomes** (BACKLOG §2) — they share the resource-in-the-
  world concept.

---

## Open questions blocking phases

| Question | Blocks | Status / default |
|---|---|---|
| Quality-roll weights (numbers) | Phase 2 | First-pass in config; tune in playtest. Not a design block. |
| Tier 2 station model | Phase 4 | **DECIDED — room tag/property** (M14.5). |
| Tier 1 campfire model | Phase 5 | **DECIDED — temporary placed entity on the M15.2 decay pattern; in MVP.** |
| Test content location | all MVP phases | **DECIDED — `core` pack** (real content pack after features lock). |
| Geography/region content | Phase 7 | Author via `mud-world-builder` skill; engine mechanism lands first. |
| Gathering nodes vs. biomes | Phase 8 | Design jointly with Biomes (shared substrate). |
| Load-bearing degree (spec §11) | scope-wide | Cooking semi-load-bearing now; crafting grows with Tier 2 content. |

---

## What review should confirm before Phase 0

The MVP shape and station models are **decided** (room-tag Tier 2,
temporary-entity Tier 1, both in MVP; test content in `core`). Remaining
confirmations are non-blocking and surface later:

1. **Quality-roll first-pass numbers** (Phase 2) — propose-and-tune, not a
   design gate.
2. **Setting** — spec stays setting-agnostic with regions as content
   (recommended); only matters at Phase 7 (regional recipes).
3. **Gathering vs. biomes** — design jointly; only matters at Phase 8.

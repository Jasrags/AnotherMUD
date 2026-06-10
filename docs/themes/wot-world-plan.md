# Wheel of Time world — implementation plan

**Status:** M0 shipped; M1 (Emond's Field) next. **Setting source:**
`docs/wot/wot_geography_mud.md` (the in-repo Westlands gazetteer the `wot` pack
is authored from) + `wot-reference/` (symlink → WheelMUD RPG-sourcebook extracts;
`the-westlands.md` for kingdom/culture detail). **Engine specs (source
of truth, unchanged):** the specs stay setting-agnostic per `docs/PRIMER.md` —
this plan adds *content packs*, not spec changes.

This plan commits the playable world to the **Wheel of Time** setting and
sequences it into buildable milestones. It is a personal, non-commercial fan
project (a long-standing MUD tradition); all room/NPC prose authored here is
**original writing set in the licensed WoT world** — we use factual geography
and place names, never reproduced book text.

## Why this exists

The engine and the crafting/gathering arc are feature-complete, but the world
is a placeholder (the village of "Hearthwick", namespace `tapestry-core`). The
only remaining crafting work — **Phase 7 regional recipes** — is blocked on a
committed geography (`crafting-and-cooking §11` "setting content prerequisite").
This plan provides that geography and, with it, a real place to play.

## Decisions (locked)

- **D1 — Setting = Wheel of Time, anchored at the Two Rivers / Emond's Field.**
  The canonical low-level starter: an isolated, effectively self-governing
  district in the far west of Andor, "present day" ≈ 998 NE. It maps almost
  one-to-one onto the existing seed (village green/inn ≈ town-square/market,
  smithy ≈ forge, the Westwood ≈ forest, the Sand Hills + Mountains of Mist ≈
  foothills/cave/mine; the wild boar already belongs in the Two Rivers wilds).
- **D2 — Build WoT as its own pack, leave the demo alone.** Per
  [[wot-setting-plan]]: split `content/core` into `core` (engine baseline) +
  `starter-world` (the Hearthwick demo, deactivatable); a new `wot` pack
  depends on `core` and supplies its own world. Boot selects the active world
  pack. We do **not** reframe Hearthwick in place — `starter-world` stays as
  the engine demo/reference.
- **D3 — Living-world backwater, not the epic plot.** The world is the Two
  Rivers as a place to live and adventure, circa the books' opening — *before*
  it becomes a warfront. NPCs are villagers, craftspeople, brigands, and
  (later, at the edges) Shadowspawn. We do **not** script the main-character
  arc; protagonists are avoided as NPCs (see Open questions on naming).
- **D4 — Region identity drives the §8/Phase-7 economy.** Each region gets a
  stable identity (climate, materials, signature goods) so "learnable only
  here" recipes and region-specific trade goods are meaningful. The Two Rivers'
  signature is the **longbow**, plus **tabac** and **wool**.

## Setting frame

- **Era:** ~998 NE (Farede calendar). Store world dates in NE; AB/FY conversion
  is flavor (`the-westlands.md` calendars) and out of scope for v1.
- **Scope boundary:** v1 is the **Two Rivers region** entire (four villages +
  their wilds + connecting roads), then a **second region** beyond the River
  Taren (Baerlon / the road toward Caemlyn) to give Phase 7 regional recipes a
  contrast. The wider Westlands (the fourteen kingdoms, Tar Valon, the Aiel
  Waste, Seanchan, etc. from `the-westlands.md`) are a long-horizon map, not v1.
- **Tone:** rural, grounded, a little uncanny at the edges. Trickle lore (one
  beat per room per the world-builder house style); the Wheel's weight comes
  from accumulation, not info-dumps.

## Region 1 — The Two Rivers (the starter region)

Far-west Andor, hedged by the **Mountains of Mist** (west), the **River Taren**
(north), the **Waterwood / the Mire** and the **White River** (south & east).
Effectively self-governing: a Mayor + Village Council and a Wisdom + Women's
Circle in each village, no lord. Climate temperate; the existing `temperate`
weather zone carries over.

### Settlements (each an `area`, some with building sub-areas)

| Settlement | Role | Maps from / notes |
|---|---|---|
| **Emond's Field** | Starter village (spawn). The Green at center; the **Winespring Inn**; the **smithy**; the **Wisdom's house** (healer/herbalist + cooking); a few houses; the Winespring itself. | Reframes the demo town-square/forge/market shape; spawn = `wot:emonds-field-green`. |
| **Watch Hill** | Smaller village on a rise to the north, on the North Road. | New; a second trainer/shop hub. |
| **Deven Ride** | Village to the south, sheep country. | New; wool/weaving flavor. |
| **Taren Ferry** | Northern edge, on the River Taren — the ferry **out** of the Two Rivers. | New; the seam to Region 2. |

### Wilderness zones (each an `area`, biome-tagged)

| Zone | Biome | Gathering / notes | Maps from |
|---|---|---|---|
| **The Westwood** | forest | timber, herbs, mushrooms; boar, wolves | existing forest zone |
| **The Sand Hills** | mountain | stone outcrops; rises toward the Mountains of Mist | existing foothills |
| **Mountains of Mist diggings** | cave | iron veins (the smithing chain) | existing cave-mouth/old-mine |
| **The Waterwood (the Mire)** | **swamp (new biome)** | reeds/fenroot/marsh forage; leeches, marsh-things | new |
| **The pastures / commons** | grassland | open-field forage | existing meadow |

### Roads (live in the *region/area*, not the settlement — world-builder rule)

- **The North Road** — Emond's Field → Watch Hill → Taren Ferry.
- **The Quarry Road** — Emond's Field west toward the old stone quarry under
  the Mountains of Mist (through the Westwood / Sand Hills).
- **The Old Road** — Emond's Field south to Deven Ride.
- Each road is a short sequence of transitional rooms (1–2 sentence house
  style), varied to avoid repetition.

### Signature economy (D4 → Phase 7 hooks)

- **Two Rivers longbow** — the famous regional craft: a bowyer/fletcher chain
  (stave from a Westwood node → bow). A **regional recipe** learnable only in
  the Two Rivers.
- **Two Rivers tabac** & **wool** — luxury trade goods (sell high elsewhere;
  a future trade-route economy hook).

## Region 2 — Beyond the Taren (future; unlocks Phase 7 contrast)

Across the ferry: the road north toward **Baerlon** (an Andoran mining town
under the Mountains of Mist) and east on the **Caemlyn Road**. Baerlon's
identity — a walled mining/trade town with Whitecloak and Queen's-Guard
presence — contrasts the Two Rivers' rural self-governance, giving regional
recipes and goods a real second pole. v1 builds **only the seam** (Taren Ferry
→ a first Baerlon-road room); the town itself is its own milestone.

## Engine mapping (how WoT content rides the existing systems)

- **Areas & regions.** A settlement or wilderness zone = one `area`. "Region"
  is a grouping above areas. v1 represents region identity as an **area-level
  `region` property** (`region: two-rivers`) — cheap, queryable, and enough for
  regional-recipe placement. A first-class region registry (à la the
  `the-westlands.md` kingdom registry) is deferred until a second region needs
  shared metadata (laws, currency, customs).
- **Regional recipes (Phase 7).** Mostly **placement, not a new engine gate**:
  a recipe is "regional" when its only trainer/scroll source exists in that
  region. The `recipe.AcqRegional` tier already exists as metadata. An optional
  soft gate ("you can only learn this in the Two Rivers") can read the area
  `region` property if we want enforcement beyond placement — decide when the
  bowyer lands.
- **Biomes.** Reuse grassland/forest/mountain/cave; add **swamp** (one new
  biome YAML — outdoors, its own forage/ambience; no engine change, biomes are
  content). Light/weather shielding flags as the existing biomes use them.
- **Currency.** Reskin the demo coin to **Andoran coinage** (crowns/marks/
  pennies across gold/silver/copper) — content only; the `mk/sp/cp/gc`
  abstraction already supports it.
- **Mobs.** Brigands (the demo bandit reframes), wolves, boar (kept), marsh
  creatures; at the long-horizon edges, Shadowspawn (Trollocs/Fades) for
  higher-level Blight-adjacent content — **not** in the peaceful Two Rivers v1.
  Reuse the loot/corpse/AI systems unchanged.
- **Crafting.** The Milestone-C smithing (forge) and cooking (inn/Wisdom)
  chains reframe directly. New regional chain: the longbow (Westwood stave node
  → fletcher).
- **NPC roles.** Innkeeper, Wisdom, smith, Mayor, fletcher, ferryman — the
  trainer+shop dual-role mob pattern (Brandr/Marta) carries over.

## Milestones

Each milestone is independently committable, keeps `go test -race ./...`
green, gets a code review before "done", and commits on `main` (no branches).

### M0 — Enabling refactor (engine, before any room authoring) — ✅ SHIPPED

The [[wot-setting-plan]] prerequisite. Make settings boot-selectable. All five
sub-slices shipped (453af1a split → 3c09172 wot boot), each code-reviewed:

- **M0.1** Split `content/core` → `core` (baseline: races/classes/tracks/slots/
  effects/abilities/rarity/essence/theme/biomes/weather/help) + `starter-world`
  (the demo village: areas/rooms/mobs/quests/items). Fix namespaces, default
  start room, and tests. `starter-world` depends on `core`.
- **M0.2** Wire an `ANOTHERMUD_PACKS` allowlist env (+ honor manifest
  `active:false`) into `pack.Load` (today `main.go` passes `nil` = all active).
- **M0.3** Move baseline **channels + emotes** from `main.go` into `core` pack
  YAML (already flagged M13.6b/M13.7b). Keep slots/biomes baselines **locked**
  for v1 (WoT lives with them).
- **M0.4** ✅ Centralize the engine tag/reserved-property **vocabulary** into one
  documented reference (`docs/ENGINE-VOCABULARY.md`) so `wot` authors know the
  contract — reserved room/item properties (room props are registry-validated,
  item props free-form), engine tags, and the namespaced-vs-bare-global id
  collision rules. Linked from the PRIMER.
- **M0.5** ✅ `content/wot` (manifest, `dependencies:{tapestry-core}`) + the
  Emond's Field area + a placeholder Green room. `ANOTHERMUD_PACKS=wot
  ANOTHERMUD_START_ROOM=wot:emonds-field-green` boots `{tapestry-core, wot}`
  (closure pulls the baseline; starter-world excluded) and spawns a character
  on the Green. Test + live-verified.
- Acceptance: ✅ default boot = demo unchanged; `ANOTHERMUD_PACKS=wot` boots the
  WoT room with the baseline via closure; channels/emotes load from YAML; the
  vocabulary contract is documented. **M0 complete — M1 (Emond's Field) next.**

### M1 — Emond's Field (the starter village)

- Author the village: the Green (spawn), Winespring Inn (+ interior sub-area if
  3+ rooms), the smithy, the Wisdom's house, the Winespring, a handful of
  houses/lanes. Exits radiate from the Green; road stubs out (Quarry/North/Old)
  wired to M2.
- Port the Milestone-C smithing + cooking content into `wot` namespace (smith,
  Wisdom-as-cook, recipes, stations). Andoran coin reskin.
- Acceptance: a new character spawns on the Green, can learn smithing/cooking,
  and run the gather→craft loop in-region. Live-verified.

### M2 — The Two Rivers wilds + roads

- The Westwood (forest), Sand Hills (mountain), Mountains-of-Mist diggings
  (cave), the pastures (grassland), and the **Waterwood** (new swamp biome).
- The three roads as transitional-room sequences, wiring Emond's Field to the
  wild zones and to the outlying-village stubs.
- Gathering nodes + forage tables per biome (reuse Milestone-C infra); brigand
  + boar + wolf spawns.
- Acceptance: the full Two Rivers gather economy works; coordinates derive
  clean; no hanging exits.

### M3 — The outlying villages

- Watch Hill, Deven Ride, Taren Ferry — each a small settlement off its road,
  with a trainer/shop or two and local flavor (Deven Ride wool, Watch Hill
  hub, Taren Ferry the ferry seam).
- Acceptance: the Two Rivers is a connected, explorable region (loop + spurs);
  `who`/maps/quests behave across areas.

### M4 — The Two Rivers longbow + region metadata (Phase 7 lands here)

- The bowyer/fletcher chain: a Westwood **stave node** → longbow recipe,
  placed *only* in the Two Rivers — the first **regional recipe**.
- Add the area `region: two-rivers` property + (optional) the soft learn-gate.
- Acceptance: the longbow is craftable in the Two Rivers and unobtainable
  elsewhere; the `recipe.AcqRegional` tier is exercised end-to-end.

### M5 — Beyond the Taren (Region 2 seam → Baerlon)

- Taren Ferry → first Baerlon-road rooms → a starter cut of Baerlon, with a
  contrasting signature good/recipe so "regional" has a second pole.
- Acceptance: two regions with distinct identities; a Baerlon-only and a
  Two-Rivers-only recipe each gated to its region.

## Risks

- **Scope creep into the wider Westlands.** `the-westlands.md` is a whole
  continent; v1 must stay disciplined to the Two Rivers (+ a Baerlon seam).
  Everything beyond is a named long-horizon backlog, not M1–M5.
- **M0 refactor touches boot + tests.** The core/starter-world split rewrites
  namespaces and the default start room; lean on the existing pack/loader tests
  as the no-regression proof and split M0 into small reviewed commits.
- **New swamp biome.** Low risk (biomes are content), but it's the first biome
  authored without a Milestone-C precedent — verify forage/ambience wiring.
- **NPC naming vs canon.** Lore-accuracy rule cuts both ways: canonical minor
  NPCs (e.g. the village smith) ground the world, but protagonist names entangle
  the plot. Default to generic roles; see Open questions.
- **Regional-recipe mechanism.** Placement-only is simplest; if we add a soft
  region gate, it must not violate crafting §1.1 (friction lowers quality/
  availability of *learning*, never refuses a *field craft* once known).

## Open questions (deferred, not blocking)

- **NPC names.** Use canonical Emond's Field figures (smith, innkeeper/Mayor,
  Wisdom) by name, or generic roles to avoid plot entanglement? Recommend
  generic-with-canon-flavor for v1; revisit per-NPC.
- **Region as property vs registry.** v1 uses an area `region` property; a
  first-class region/kingdom registry (laws, currency, customs from
  `the-westlands.md`) waits until ≥2 regions share metadata.
- **The One Power / channeling.** `wot-reference/the-one-power.md` is a large
  system (weaves, ter'angreal, Aes Sedai). Entirely out of v1 world scope; a
  future mechanics theme, not geography.
- **Andoran coinage depth.** Single reskinned coin now; per-kingdom mints /
  exchange (`the-westlands.md` notes) is a future economy lever.
- **Sectors/biomes breadth.** v1 adds swamp; blight/waste/stedding (the
  long-horizon WoT sectors) come with the regions that need them.

## Sequencing

M0 (engine refactor) → M1 (Emond's Field) → M2 (wilds + roads) → M3 (outlying
villages) → M4 (longbow + region metadata = Phase 7) → M5 (Baerlon seam). M0 is
the only engine work; M1–M5 are content authored with the `mud-world-builder`
skill into `content/wot`. Related: [[wot-setting-plan]] (architecture + boundary
audit), [[crafting-deferred-fixes]] (Phase 7 was the geography-blocked remainder
this unblocks).

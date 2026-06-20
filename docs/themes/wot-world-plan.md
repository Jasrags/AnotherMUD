# Wheel of Time world — implementation plan

**Status:** M0–M5 shipped — the Two Rivers region is complete (Emond's Field,
the wilds + roads, Watch Hill / Taren Ferry / Deven Ride, the longbow + region
metadata), plus an off-roadmap westward arc (the Westwood, the Mountains of Mist,
and Stedding Chinden), and **M5 (Beyond the Taren → Baerlon)** opens Region 2:
the ferry crosses north onto the Baerlon road into Andor proper, ending in a
starter cut of Baerlon (the walled mining town) with its own signature regional
craft (silversmithing). **Setting source:**
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
| **The Waterwood (the Mire)** | **swamp** ✅ | reeds/fenroot/marsh forage; leeches, marsh-adder. SHIPPED — `the-waterwood` area east of Deven Ride (6 rooms: fen edge → reed-beds → black pools → drowned wood → mire heart → White River bank), the `swamp` biome + `mire-forage`, a fen-trapper anchor + neutral/hostile fauna. | new |
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

## Region 2 — Beyond the Taren (✅ Baerlon + the Whitebridge corridor shipped, M5–M6)

Across the ferry: the road north toward **Baerlon** (an Andoran mining town
under the Mountains of Mist) and east on the **Caemlyn Road**. Baerlon's
identity — a walled mining/trade town with Whitecloak and Queen's-Guard
presence — contrasts the Two Rivers' rural self-governance, giving regional
recipes and goods a real second pole. **M5 built the seam *and* a Baerlon
starter cut** (the Baerlon road + 6 town rooms + silversmithing as the second
regional pole). **M6 added the Whitebridge corridor** (see the M6 milestone
below): a crossroads ~2/3 up the Baerlon road branches east onto the **Caemlyn
Road**, which runs down the Arinelle vale to the **White Bridge** — the
Age-of-Legends glass span — and into **Whitebridge** town on the far bank.
**M7–M8 pushed the Caemlyn Road east** (see those milestones below): the
map-correct spine now runs **Whitebridge → Arien → Four Kings → Market Sheran**,
with the order set by the project's Andor wall-maps (which override book
travel-order — Arien sits *between* Whitebridge and Four Kings). Still future:
a fuller Baerlon (the Queen's Guard, a Whitecloak garrison once S8 reputation
lands); the **Caemlyn Road east** of Market Sheran (Carysford → Caemlyn — the
Sheran East Road is the stub); the map's **northern arm** (Braem Wood, New
Braem) and **southern arm** (the Black Tower, Harlon Bridge, Damelien, …); and
the **Arinelle river route** south toward Illian (the Whitebridge docks are the seam).

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

### M1 — Emond's Field (the starter village) — ✅ SHIPPED (M1a 93b140a, M1b 86ba722)

- **M1a** the walkable village: 10 rooms in the `emonds-field` area (the Green
  spawn, the Winespring Inn common-room/kitchen/guest-room, the smithy, the
  Wisdom's cottage, the Winespring, the Quarry/North/Old road stubs out).
  Cardinal-only layout (engine has no diagonals). Five named villagers — Bran
  al'Vere (innkeeper+Mayor), Marin al'Vere (cook), Haral Luhhan (smith),
  Nynaeve al'Meara (the Wisdom), Cenn Buie (thatcher). Real character names,
  original prose.
- **M1b** both crafts playable gold-free: cooking is a complete forage→cook
  loop (the Wisdom's herb garden = a `herb-garden` forage biome → wild-herb +
  garden-greens; Marin teaches cooking; inn kitchen = Tier-2 station;
  `cook-pottage` baseline). Smithing = learn at Haral + forge Tier-2 station +
  `forge-iron-dagger` baseline, fed by two **practice ingots** on the forge
  floor (a teaching aid removed in M2). Both NPCs are trainer+shop. Abilities/
  effects reused from `tapestry-core`.
- **Decision (gold/economy):** practice-stock so both crafts complete in M1
  without coin; the ore-gather + smelt + §8 closure (and the practice-stock
  removal) land in M2. Andoran-coin reskin deferred — no gold source in the
  village yet (M2 hunting/loot).
- Acceptance: ✅ new character spawns on the Green, learns smithing + cooking,
  runs forage→cook→eat and learn→forge end to end. Live-verified.

### M2 — The Two Rivers wilds + roads — ✅ SHIPPED (M2a cc8af7a, M2b 270bea7)

- **M2a** the west chain off the forge: the Westwood (forest), the Sand Hills
  (mountain), and the Mountains-of-Mist diggings (cave) — 5 rooms, 2 areas, 3
  wot biomes with Westwood forage. Surfaced + resolved the **one-world-per-boot**
  model (two world packs share bare-global biome ids → can't co-load; the boot
  defaults to the demo, `ANOTHERMUD_PACKS=wot` selects WoT; `make run-wot`).
- **M2b** gathering + the §8 closure: an iron vein (mining) in the diggings and
  a timber stand (woodcutting) in the Westwood ride the spawn scheduler; tools
  in the wilds + Haral's shop; a neutral huntable wild-boar drops raw meat.
  `smelt-iron-ingot` (ore→ingot) makes smithing bottom out in **gathered ore**,
  and the **M1 forge practice ingots are removed**; `cook-hearty-meal` uses the
  boar meat + foraged herb. Both recipes pass the §8 economy guardrail.
- **Deferred from this plan's M2:** the North/Old road stubs stay stubs (Watch
  Hill/Deven Ride are M3); the pastures (grassland) and the Waterwood (swamp)
  weren't needed for the gather economy — fold into M3 or a later pass.
- Acceptance: ✅ full gather→refine→craft economy works in-region (mine→smelt→
  forge; hunt→meat; forage→cook); coords derive clean; no hanging exits.
  Live-verified end to end.

### M3 — The outlying villages — ✅ SHIPPED (M3a 946c238, M3b 16b8e36, M3c f8e0286)

- **M3a Watch Hill** (North Road): the road climbs through a sheep pasture (new
  wot `grassland` biome + pasture-forage) to Watch Hill's green, the Goose and
  Crown inn, and the beacon lookout. NPCs: innkeeper, old watchman.
- **M3b Deven Ride** (Old Road): south through pasture to the hedge-bound green
  and the sheepfold. NPCs: a shepherd, an old weaver (wool flavor — no new
  craft profession this milestone).
- **M3c Taren Ferry** (North Road on): the pile-built village on the Taren's
  south bank + the rope-and-barge ferry landing. The crossing north is the seam
  onto Andor proper (Baerlon/Region 2) — stubbed for M5. NPCs: the ferryman, a
  light-fingered villager (neutral, per the village's shady reputation).
- **Decisions:** generic-role NPCs for the outlying villages (canon names stay
  in Emond's Field); no new craft profession (cooking/smithing stay central);
  the grassland pastures landed here, the Waterwood/swamp stays deferred.
- Acceptance: ✅ the Two Rivers is one connected, explorable district (Emond's
  Field + the western wilds + Watch Hill/Taren Ferry north + Deven Ride south);
  6 areas, 25 rooms, coords derive clean, no hanging exits; walked end to end.

### M4 — The Two Rivers longbow + region metadata (Phase 7 lands here)

- The bowyer/fletcher chain: a Westwood **stave node** → longbow recipe,
  placed *only* in the Two Rivers — the first **regional recipe**.
- Add the area `region: two-rivers` property + (optional) the soft learn-gate.
- Acceptance: the longbow is craftable in the Two Rivers and unobtainable
  elsewhere; the `recipe.AcqRegional` tier is exercised end-to-end.

### M5 — Beyond the Taren (Region 2 seam → Baerlon) — ✅ SHIPPED

- **The Baerlon road** (new area `baerlon-road`, `region: andor`): the ferry
  landing's north stub now crosses the Taren onto the north bank → the Andor
  road → the approach under the Mountains of Mist (3 rooms; grassland → mountain,
  a deliberate hard-country contrast leaving the cozy Two Rivers).
- **Baerlon** (new area `baerlon`, `region: andor`): a starter cut of the walled
  mining town — the Watch Gate (suspicious Andoran guard), Market Street (the
  hub, with a Child of the Light walking it — a Whitecloak flavor + S8-reputation
  seed), the Stag and Lion (Master Fitch, canon innkeeper), the Silverworks, the
  Market Square (a general dealer), and the Mining Quarter under the peaks
  (6 rooms).
- **The second region pole:** `silversmithing` — a new craft discipline taught
  *only* by the Baerlon silversmith (region-exclusive by trainer placement, the
  §8/Phase-7 economy). `learn silversmithing` → buy a silver bar → `work-silver`
  at her tier-2 bench → a piece of Baerlon silverwork (a valuable trade good).
  She's a **journeyman** trainer, so Baerlon also lets a Two Rivers smith raise a
  craft cap past the apprentice ceiling at home.
- **Acceptance met:** two regions with distinct identities (`two-rivers` /
  `andor`); the Baerlon-only silversmithing recipe is gated to Baerlon by
  placement. **One honest gap:** the `recipe.AcqRegional` tier is *not* literally
  exercised — the recipe is tagged `baseline` because the engine has no
  teach-a-single-recipe path, so a `regional`-tagged recipe would be ungrantable.
  Region-exclusivity is achieved by the discipline's trainer placement instead.
  Exercising `AcqRegional` end-to-end needs a crafting-system follow-on (a
  trainer-teaches-named-recipe or region-grant path), not geography.
- Verified: WoT pack boots clean — areas 7→9, rooms 34→43, mobs +6,
  abilities +1, items +2; all 84 room exits resolve; pack/crafting/economy
  tests green under `-race`.
- **Remaining Region-2 pole (deferred):** the *Two-Rivers-only* recipe (the M4
  bowyer/fletcher longbow chain — stave node → bowyer discipline → recipe) is a
  crafting milestone, not geography; the longbow exists as an item/shop good but
  has no craft recipe yet.

### M6 — The Whitebridge corridor (Caemlyn Road west + the Arinelle) — ✅ SHIPPED

- **The crossroads:** a new `the-caemlyn-crossing` room inserted into the Baerlon
  road ~2/3 of the way up (between `the-andor-road` and `baerlon-approach`) — a
  signposted junction: north to Baerlon, south to the Taren/Two Rivers, **east on
  the Caemlyn Road**. (The road sequence rewired cleanly; all exits stay
  reciprocal.)
- **The Caemlyn Road corridor** (new area `caemlyn-road`, `region: andor`): 3
  rooms east off the crossing — the highway proper → the Arinelle vale (the land
  falling toward the river) → the west bank, where the **White Bridge** first
  comes into view.
- **Whitebridge** (new area `whitebridge`, `region: andor`): the **White Bridge**
  itself (the seamless Age-of-Legends glass span over the Arinelle), the bridge
  foot (a Queen's Guardsman — Andor's law, contrasting Baerlon's Whitecloaks), the
  market square (a dealer), the **Wayfarers' Rest** (Master Bartim, canon
  innkeeper), and the river docks (a riverman — the seam for a future Arinelle
  river route south to Illian). 5 rooms.
- Verified: WoT pack boots clean — areas 10→12, rooms 49→58 (+9), mobs +4; all
  114 room exits resolve and are reciprocal; pack tests green under `-race`.
- **Onward seams (deferred):** the Caemlyn Road *east* of Whitebridge (Four Kings
  → Market Sheran → Carysford → Arien → Caemlyn) and the Arinelle river route
  south. No regional craft good for Whitebridge (a trade/crossing town, not a
  craft-signature one) — left for a later economy pass.

### M7 — The Caemlyn Road east → Four Kings — ✅ SHIPPED

- **The road east** (added to the `caemlyn-road` area, `region: andor`): Whitebridge's
  market square exits east onto road rooms toward Four Kings. (**Corrected in M8:**
  the Andor wall-maps put **Arien** between Whitebridge and Four Kings, so M7's
  Four-Kings-directly-east placement was off by one town — Arien was inserted into
  this corridor in M8. See the map-canon note below.)
- **Four Kings** (new area, `region: andor`): a starter cut of the lawless
  crossroads wagon-town — the west end (a hard-faced tough loitering), the
  Crossroads that names the town (the mine-road meets the Caemlyn Road), the
  **Dancing Cartman** (Saml Hake, canon innkeeper — a cheap, grasping house), the
  wagon yard (a too-smooth merchant — a **Darkfriend seed**, the road-going twin of
  Baerlon's Whitecloak), and the East End (the stub onward to Market Sheran).
  5 rooms. A deliberate tonal drop from Whitebridge's Queen's-law respectability to
  a town that "belongs to nobody."
- Verified: WoT pack boots clean — areas 12→13, rooms 58→66 (+8), mobs +3; all
  130 room exits resolve and are reciprocal; pack tests green under `-race`.
- **Onward stub:** the East End's east exit picks up the next leg (built in M8).

### M8 — Map reconciliation: Arien + Market Sheran — ✅ SHIPPED

**Geography source-of-truth decision (2026-06-19):** the WoT pack follows the
project's **Andor wall-maps**, which **override book travel-order**. The
map-correct Caemlyn Road order is **Whitebridge → Arien → Four Kings → Market
Sheran → Carysford → Caemlyn** — reversing Arien/Four Kings vs the books. The
geography doc (`docs/wot/wot_geography_mud.md`) was corrected to match, and the
decision is recorded in memory `wot-geography-map-canon`.

- **Arien** (new area `arien`, `region: andor`) — inserted **between Whitebridge
  and Four Kings** to fix the M7 order. A deliberately small **waystop** (3 rooms:
  the road-green, the Wagoner's Rest inn, the wainwright's yard) living off the
  wagon trade. The M7 corridor was re-threaded (`the-eastward-road` → Arien →
  `the-wagon-road`) so the spine now reads Whitebridge → Arien → Four Kings.
- **Market Sheran** (new area `market-sheran`, `region: andor`) — the next town
  **east of Four Kings**: a prosperous, orderly Andoran **farming village** (4
  rooms: the green + well, the market that names it, the Harvest Home inn, the
  east road) — a wholesome counterweight to Four Kings' squalor, its folk civil
  but wary in uneasy times. Reached via one new road room (`the-sheran-road`).
- Verified: WoT pack boots clean — areas 13→15, rooms 66→74 (+8), mobs +4; the
  full W→E spine traces Whitebridge → Arien → Four Kings → Market Sheran; all 146
  room exits resolve and are reciprocal; pack tests green under `-race`.
- **Onward stub:** Market Sheran's East Road is the seam — built out in M9.

### M9 — The road's end: Carysford → Caemlyn — ✅ SHIPPED

The Caemlyn Road reaches the capital. The full west→east spine is now walkable
end to end from the Two Rivers: **Emond's Field → the Taren → the Baerlon
crossroads → the White Bridge → Whitebridge → Arien → Four Kings → Market
Sheran → Carysford → Caemlyn**.

- **Carysford** (new area `carysford`, `region: andor`): a pretty 4-room
  ford-village where the road crosses the little **River Cary** — the green +
  the Leaping Fish inn, the paved **ford** itself (a fisherman, the watermill),
  and the east bank with the first distant gleam of Caemlyn's walls. The last
  quiet stop before the city.
- **Caemlyn** (new area `caemlyn`, `region: andor`): the capital, **expanded to
  be map-faithful** against Adam Whitehead's Caemlyn map (atlasoficeandfire) — a
  14-room city, not a linear corridor. The **New City** ring carries all five
  named outer gates, each a **stub seam for a future road arm**: the **Whitebridge
  Gate** (W — our road in), the **Tar Valon Gate** (N → Tar Valon), the **Sunrise
  Gate** (E → Cairhien), the **Far Madding Gate** (S), and the **Lugard Gate**
  (SW → Murandy) — plus the **Great Square**, the **Queen's Blessing** inn
  (**Basel Gill**, canon), and **Full Moon Street** dividing New City from Inner.
  Through the **Mondel Gate** in the Ogier-built silver-streaked wall lies the
  **Inner City**: the **Queen's Plaza**, the gated **Royal Palace** (the Queen's
  leave required — interior is future depth), and — a gift of a hook — the
  **Waygate** (a shut Ogier Waygate into the Ways; ties to the steddings, a future
  travel system). Reached via one road room (`the-caemlyn-approach`).
- Verified: WoT pack boots clean — rooms now 94 (Caemlyn 14); the full W→E spine
  traces Whitebridge → … → the Royal Palace gates; all 186 room exits resolve and
  are reciprocal; pack tests green under `-race`. (The M9 starter cut was 8 rooms;
  the map-faithful gate/Inner-City/Waygate expansion brought it to 14.)
- **Caemlyn is the hub for what comes next.** The five gate-stubs are the seams
  for the roads radiating out: the **Tar Valon Gate** (N → Tar Valon / the White
  Tower), the **Sunrise Gate** (E → Cairhien, over the Erinin), the **Far Madding
  Gate** (S), and the **Lugard Gate** (SW → Murandy). Other future depth: the
  Royal Palace interior; the Waygate → **the Ways** (a greenfield travel system);
  more Inner/New-City districts and the named inns (Crown & Lion, Culain's Hound,
  …); the **northern arm** (Braem Wood: New Braem, Kore Springs), the **southern
  arm** (Harlon Bridge, Forel Market, Buryhill, Damelien, Cullen's Crossing), and
  the hill ranges (Splintered, Tunaighan) — all from the Andor map. The **Black
  Tower** (the Asha'man fortress, on the southern arm) is **deferred by request
  (2026-06-19)** — not to be built next until the deferral is lifted.

### M10 — Reroute the western spine through Baerlon (canon-map correction)

New canon maps (Whitebridge, Baerlon, and the route maps from the same set as
the Andor/Caemlyn maps) showed the Caemlyn Road runs **through Baerlon**, not
past it on a bypass. The original M5/M6 had a `the-caemlyn-crossing` branching
east to Whitebridge *before* Baerlon (per an earlier instruction); that's now
**corrected** — the bypass is deleted and the road runs **Two Rivers → Baerlon
(in the Mountain Gate, out the Caemlyn Gate) → White Bridge → Whitebridge**.

- **Baerlon reworked** to its city map: the west gate is the **Mountain Gate**
  (renamed from "Watch Gate"; outside it the roads fork — **south to the Two
  Rivers**, **north to Comfrey & the Mountains of Mist**), the main street runs
  east through a new **Town Square** past the new **Governor's Hall** (a new
  *Governor of Andor's-appointee* NPC — Baerlon is administered, not self-ruled)
  to the new **Caemlyn Gate** (east, onto the road to Whitebridge).
- **Comfrey route** (the NW fork, new `comfrey` area): the Comfrey road climbs
  the skirts of the Mountains of Mist to the upland hamlet of **Comfrey** (the
  Miner's Rest inn) and a stub on into the high passes — the user-requested
  route to Comfrey & the Mountains of Mist. The **Two Rivers** fork (south) is
  the existing ferry road, now wired to the Mountain Gate approach.
- Verified: rooms now 101; the full W→E spine traces Two Rivers → **through
  Baerlon** → Whitebridge → … → Caemlyn; all 200 exits resolve and are
  reciprocal; pack tests green under `-race`. (Whitebridge's own N/S arms — to
  Shadar Logoth/Maradon and to Remen/Illian down the Arinelle — remain future.)

## Risks

- **Scope creep into the wider Westlands.** `the-westlands.md` is a whole
  continent; v1 must stay disciplined to the Two Rivers (+ a Baerlon seam).
  Everything beyond is a named long-horizon backlog, not M1–M5.
- **M0 refactor touches boot + tests.** The core/starter-world split rewrites
  namespaces and the default start room; lean on the existing pack/loader tests
  as the no-regression proof and split M0 into small reviewed commits.
- **New swamp biome.** ✅ Resolved — shipped with the Waterwood (the `swamp`
  biome + `mire-forage` table load and wire clean; confirmed on a `wot` boot:
  biomes 5→6, forage_tables 5→6). Low risk as predicted; biomes are content.
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
villages) → M4 (longbow + region metadata = Phase 7) → M5 (Baerlon seam +
starter cut) — **all shipped.** M0 was the only engine work; M1–M5 are content
authored with the `mud-world-builder` skill into `content/wot`. Next geography
candidates: a fuller Baerlon, the Caemlyn Road east, or the Two-Rivers longbow
craft chain (the deferred second recipe pole — a crafting slice, not geography). Related: [[wot-setting-plan]] (architecture + boundary
audit), [[crafting-deferred-fixes]] (Phase 7 was the geography-blocked remainder
this unblocks).

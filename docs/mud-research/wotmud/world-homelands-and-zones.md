# WoTMUD — World, Homelands, and Zones

Source:
- https://wotmud.info/homelands/
- https://wotmud.info/zones/
- https://wotmud.info/wheel-of-time-interactive-world-map/
- Backing data: https://wotmud.fandom.com/wiki/Homeland and https://wotmud.fandom.com/wiki/Zone

Fetched: 2026-06-18

> Sourcing note: the wotmud.info homelands/zones pages are JS-rendered and embed
> the WoTMUD Fandom wiki (`data-resource=".../wiki/Homeland"`, `.../wiki/Zone`).
> The zone table and homeland stat tables below come from the wiki via the
> MediaWiki API (templates expanded). Racial *traits* live in `races.md`; this
> file holds geography + the per-homeland stat machinery.

---

## 1. The interactive world map

`wotmud.info/wheel-of-time-interactive-world-map/` is **an interactive world map
that bridges in-game zones and lore**. Clickable areas (highlighted in red) load
the corresponding **zone maps from the wiki**. Map image by *Joystikx*; a separate
overview map (`Wheel-of-Time-Map.jpg`, 2019) is embedded on the homelands page.

Cities labeled on the interactive map:
**Caemlyn, Whitebridge, Far Madding, Tear, Amador, Maradon, Falme, Illian, Lugard,
Tar Valon, Fal Dara.**

---

## 2. Homelands ("Stocks" for Trollocs) and what they do

A **Homeland** is a player character's birthplace (for Trollocs, an animal
**stock**). Homelands once mattered at character creation/stat-rolling; **now they
only impact the "rerolling" process** because all characters enter with pre-rolled
race+class base stats. They also serve roleplay (generally no mechanical RP
advantage/disadvantage).

Notes:
- Some "homeland-class" combinations are flagged **obsolete** (Immortal-evaluated);
  affected characters can have their homeland changed free.
- As of **Apr 27, 2025**, humans may change homeland at the **Four Kings
  Magistrate** for **2000 gold crowns**.
- **Minimum statsum for humans is 70** (lower rolls are tossed).

### Lightside homelands — base stats + D100 reroll modifier table

Base stats are STR INT WIL DEX CON. The D100 columns are the reroll-modifier
distribution (chance of each ±N bump):

| Homeland | Base (STR INT WIL DEX CON) | +4 | +3 | +2 | +1 | 0 | -1 | -2 | -3 | -4 |
|----------|----------------------------|----|----|----|----|---|----|----|----|----|
| Altara | 14 11 12 15 13 | 10 | 10 | 10 | 10 | 45 | 10 | 5 | 0 | 0 |
| Amadicia | 14 13 12 12 16 | 1 | 4 | 9 | 11 | 50 | 8 | 12 | 4 | 1 |
| Andor | 15 14 13 14 14 | 1 | 4 | 12 | 12 | 42 | 12 | 12 | 4 | 1 |
| Arad Doman | 15 11 11 14 12 | 2 | 8 | 13 | 13 | 34 | 10 | 10 | 8 | 2 |
| Arafel | 16 12 12 14 15 | 1 | 4 | 12 | 12 | 42 | 12 | 12 | 4 | 1 |
| Cairhien | 14 15 15 13 13 | 4 | 8 | 13 | 13 | 30 | 12 | 8 | 7 | 5 |
| Ghealdan | 13 14 14 13 13 | 3 | 13 | 13 | 13 | 40 | 8 | 4 | 3 | 3 |
| Illian | 13 14 14 15 14 | 5 | 10 | 10 | 10 | 45 | 8 | 4 | 4 | 4 |
| Kandor | 16 12 12 14 15 | 1 | 4 | 12 | 12 | 42 | 12 | 12 | 4 | 1 |
| Malkieri Diaspora | 16 12 12 14 15 | 1 | 4 | 12 | 12 | 42 | 12 | 12 | 4 | 1 |
| Mayene | 12 15 13 14 13 | 8 | 10 | 15 | 15 | 27 | 10 | 8 | 5 | 2 |
| Murandy | 14 11 12 15 13 | 10 | 10 | 10 | 10 | 45 | 15 | 0 | 0 | 0 |
| Saldaea | 16 12 12 14 15 | 1 | 4 | 12 | 12 | 42 | 12 | 12 | 4 | 1 |
| Shienar | 16 12 12 14 15 | 1 | 4 | 12 | 12 | 42 | 12 | 12 | 4 | 1 |
| Tarabon | 14 11 12 15 13 | 10 | 10 | 10 | 10 | 45 | 15 | 0 | 0 | 0 |
| Tear | 15 14 14 12 14 | 1 | 4 | 12 | 12 | 42 | 12 | 12 | 4 | 1 |
| Two Rivers | 15 12 14 12 15 | 5 | 10 | 10 | 10 | 30 | 10 | 11 | 10 | 4 |

### Seanchan homelands — base stats + D100 reroll modifier table

All six Seanchan homelands share the same modifier distribution (6/8/12/12/37/15/4/4/2):

| Homeland | Base (STR INT WIL DEX CON) |
|----------|----------------------------|
| Kirendad | 15 12 12 14 15 |
| Noren M'Shar | 16 11 11 13 15 |
| Rampore | 16 13 13 14 14 |
| Seandar | 16 12 12 14 15 |
| Shon Kifar | 15 12 11 13 14 |
| Tzura | 15 12 12 14 14 |

(No obsolete Seanchan homeland combinations currently exist.)

### Trolloc stocks — base stats + D100 modifier table

| Stock | Base (STR INT WIL DEX CON) | +4 | +3 | +2 | +1 | 0 | -1 | -2 | -3 | -4 |
|-------|----------------------------|----|----|----|----|---|----|----|----|----|
| Beaked | 15 9 7 14 15 | 2 | 5 | 15 | 20 | 16 | 20 | 15 | 5 | 2 |
| Bearish | 16 5 6 10 14 | 2 | 5 | 15 | 20 | 16 | 20 | 15 | 5 | 2 |
| Boarish | 16 8 7 12 16 | 2 | 5 | 15 | 20 | 16 | 20 | 15 | 5 | 2 |
| Ramshorned | 14 10 7 13 15 | 2 | 5 | 15 | 20 | 16 | 20 | 15 | 5 | 2 |
| Wolfish | 15 9 7 14 16 | 2 | 5 | 15 | 20 | 16 | 20 | 15 | 5 | 2 |

(Trolloc stocks all share the same reroll distribution; the difference is base
stats + max caps. See `races.md` §6 for per-stock playstyle notes.)

---

## 3. Zones — the map's building blocks

A **zone** is the basic map unit: **up to 100 rooms**, connecting to each other and
to other zones via **N/S/E/W/Up/Down** exits.

Counts (from the wiki, as of fetch):
- **236 zones** currently accessible in-game
- **33 zones** known to have been removed
- **269 zone pages** total (the "All Zones" superset)

Each zone has a **Grid** coordinate (e.g. Caemlyn region `(5,0)`, Tar Valon `(6,5)`),
a **Repop** timer (mostly blank/varies), and belongs to one or more **Regions**.

### Regions (the higher-level geography)

The major regions zones are grouped under:

**Andor, Tar Valon, Cairhien, Shienar, Kandor, Saldaea, Arafel, Arad Doman,
Amadicia, Ghealdan, Murandy, Altara, Tear, Illian, Mayene, Far Madding, The Two
Rivers, The Black Hills, The Caralain Grass, The Spine of the World, The Mountains
of Mist, The Aiel Waste, The Plains of Maredo, The Hills of Kintara, The Haddon
Mirk, The Great Forest, Almoth Plain, Toman Head, Tremalking, Jafar, The Shadow
Coast, The Isle of Mad Men, Seanchan Empire / Seandar, The Blight, The Blasted
Lands, Plain of Lances, the Wilderness.**

### Notable zone clusters (verbatim names, abbreviated by region)

- **Andor (largest hub):** Inner/Outer Caemlyn, Caemlyn Road East/West of Baerlon
  and Whitebridge, Baerlon, Four Kings, Aringill, North of Aringill, Braem Wood,
  Forgotten Braem, Deadly Braem East/West, In the Forest/Mountains, Tarendrelle at
  the Eldar, North Road, Whitebridge, Outskirts of Whitebridge, Tar Valon Road
  North of Caemlyn.
- **Tar Valon:** Tar Valon (zone), White Tower (zone), Upper Floors of the White
  Tower, the Ajah Quarters (Blue and White / Brown / Green and Yellow / Red and
  Gray), Fal Dara Road, Tar Valon Forest, NE/SE Shores of Tar Valon, roads to
  Cairhien.
- **The Two Rivers:** Emond's Field, Deven Ride, Watch Hill, Taren Ferry, Quarry
  Road, Old Road, The Waterwood, Westwood by the Mountains, The Volcano.
- **Shienar / Borderlands:** Fal Dara, Fal Dara Outskirts, Borderlands North/West
  of Fal Dara, Tarwin's Gap, Long/Northern Fal Dara Road.
- **The Blight / Blasted Lands (Dark territory):** Shayol Ghul, Northern Shayol
  Ghul, Thakan'dar, Dark Fortress, The Ruined Keep, Endless/Ruined/Volcanic
  Blight, Deep Within the Blasted Lands, Tunnels and Caverns Beneath the Blight,
  Beneath the Desolate Mountains, City in the Blight, Lockshear.
- **Cairhien:** Cairhien (zone), Cairhien Province, The Sun Palace, Maerone,
  Kinslayer's Dagger, The Cairhien Hills, roads to Tar Valon.
- **Tear / South:** The Stone of Tear, Tear (zone), Tear Road, The Tear to Illian
  Road, The Coastal Roadway, The Sea of Storms.
- **Far Madding & the Plains of Maredo:** New Far Madding North/South, The
  Lakeside / Countryside of Far Madding, Far Madding – Tear Road, Glancor,
  Eastern/Western Plains of Maredo.
- **Seanchan Empire / Seandar:** City of Seandar, Court of Nine Moons and Tower of
  Ravens, Seanchan Shore, Flood Plains of Seandar, Grasslands North of Seandar,
  Isle of Madmen North/South, Horse Thief Zone, Foothills of Tamika.
- **Special:** Shadar Logoth I/II, The Ways, The Eye of the World (removed),
  stedding (Shangtai, Tsochan, Yandar), Valan Luca's Menagerie.

(The full 236-zone list with grid coords lives on the wiki Zone page; the above is
a region-organized digest. The wiki also tracks 33 removed zones with creator +
removal dates, e.g. the original Garen's Wall, multiple Altaran countryside zones,
several Seanchan zones removed 2003–2011.)

---

## 4. Takeaways for AnotherMUD

- **Zone = up to 100 rooms, N/S/E/W/U/D + zone-to-zone exits, with a grid coord.**
  Maps directly onto our `world` package (rooms/areas/exits + derived coordinates,
  M23). Their per-zone `(x,y)` grid is essentially our area-local coordinate
  derivation.
- **Geography is dense and lore-anchored** — ~236 live zones tiling a recognizable
  WoT map (Andor as the central hub, the Blight as Dark territory, Seandar as the
  Seanchan continent). A useful scale reference for the WoT content track
  (`wot-world-plan.md`), which currently starts at Two Rivers / Emond's Field.
- **Homeland-as-reroll-modifier, not as starting stats** is a clean separation:
  birthplace shapes the *distribution* of what you can roll into, not your initial
  character. Worth noting against our character-creation wizard.
- **Removed-zone tracking with creator + date** is good content-governance hygiene
  if AnotherMUD's WoT world grows enough to retire areas.

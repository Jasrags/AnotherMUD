# Shadowrun 5th Edition — Complete Geography Reference
## AnotherMUD world-building gazetteer · Setting: ~2075
*Compiled from SR5 core, Sixth World Almanac, Seattle Sprawl sourcebook, and setting knowledge.*

This is the geography reference for the **`shadowrun` content pack** — the Sixth World
gazetteer (nations, sprawls, districts, corporate enclaves, barrens, Matrix zones) that
`content/shadowrun/` is authored from. It pairs with two companions: the build sequence +
engine mapping live in `docs/themes/sr-world-plan.md`, and the content↔engine contract
(tags, properties, ids) in `docs/ENGINE-VOCABULARY.md`.

The **Builder Conventions** below map this gazetteer onto AnotherMUD's content model (the
`shadowrun` pack; areas, rooms, biomes); everything after is lore.

---

## BUILDER CONVENTIONS

These rules map this gazetteer onto AnotherMUD's content model. Builders read this section
first; the rest of the doc is the lore gazetteer.

### Where it lives

All Shadowrun world content is the **`shadowrun` content pack** under `content/shadowrun/`,
namespaced `sr:<id>` (a bare id in a YAML file is qualified to `sr:` at load; cross-pack
references use the full form). The pack depends on the `tapestry-core` engine baseline
(races, classes, slots, effects, abilities, channels, emotes). Boot it with
`ANOTHERMUD_PACKS=shadowrun` and an `ANOTHERMUD_START_ROOM` inside the pack.

### Areas, rooms, regions

- A **room** (`content/shadowrun/rooms/*.yaml`) declares its `area:`, a `terrain:`
  (→ biome), `exits:`, and optional `items:`. Coordinates are auto-derived from the exit
  graph — no manual placement; pin a room with `coord:` only to anchor a sub-grid.
- An **area** (`content/shadowrun/areas/*.yaml`) groups rooms — one per district zone or
  interior location (Redmond Barrens, Dante's Inferno, Renraku Arcology lobby level). A
  building with 3+ interior rooms gets its own area; smaller buildings are single rooms
  inside their district's area.
- A **region** is the sprawl or nation level above areas, carried as an area-level
  `region:` property (e.g. `region: seattle-redmond`). This drives crafting economy, corp
  presence, and security-zone rules.

### The security zone system (critical — replaces WoT's danger-band concept)

Security zones replace level-bands as the primary area-danger signal in this setting. They
are not a mob-level system — they are a **consequence and patrol density system**. A runner
in a AAA zone faces fast response and overwhelming force; a runner in a Z-Zone faces no law
enforcement but constant environmental and gang threat.

| Security Zone | Level Band | Response Time | Description |
|---|---|---|---|
| `AAA` | Endgame | 1d6 minutes | Corp HQs, military installations, luxury arcologies. Proactive patrols, astral security, drone coverage, no tolerance for crime. |
| `AA` | High | 1d6+4 minutes | High-lifestyle residential, major corp campuses. Constant coverage; specialists on call. |
| `A` | Mid-High | 2d6+3 minutes | Most of the metroplex. Middle lifestyle. Regular patrols, call-and-response. |
| `B` | Mid | 1d6×5 minutes | Mid-level residential, industrial. Patrols thin, response slow. |
| `C` | Low | 1d6×10 minutes | Low-end residential, storage. Minimal patrol, mostly absent. |
| `Z` | Barrens | 2d6 hours | Redmond Barrens, containment zones. Law doesn't try; gangs rule; survival of the fittest. |

### Build order

Build **Seattle-first, radiating outward**: the Downtown core → Redmond/Puyallup barrens
→ Bellevue corporate → Renton industrial → Snohomish rural → other UCAS sprawls → global
sprawls at the high end. The nations below double as the long-horizon map; populate each
as a milestone reaches it — gaps are fine.

### Level-band / Security / Biome master table

`level_range` is a planning hint for mob tuning. `security` maps to the zone system above.
**Default biome** is the `terrain:` a room uses when it doesn't set its own.

| Region | Band | level_range | Security Zone | Default biome |
|---|---|---|---|---|
| Seattle Downtown (core) | Starter | 1–10 | AA/A | urban |
| Seattle Bellevue | Corporate | 5–15 | AA | corp-enclave |
| Seattle Renton | Industrial | 5–15 | B/C | industrial |
| Seattle Snohomish | Rural | 5–15 | B/C | rural |
| Seattle Everett | Industrial-port | 5–15 | B/C | industrial |
| Seattle Tacoma | Port/industrial | 5–15 | B/C | industrial |
| Seattle Auburn | Low-income | 10–20 | C/Z | barrens |
| Seattle Redmond Barrens | Barrens | 15–30 | Z | barrens |
| Seattle Puyallup Barrens | Barrens | 15–30 | Z | barrens |
| Fort Lewis | Military | 10–20 | AAA | military |
| Council Island | Neutral/Tribal | 10–20 | A | urban |
| Ork Underground | Barrens-sub | 15–25 | Z | underground |
| Tír Tairngire (Oregon) | Contested | 20–30 | varies | forest |
| Salish-Shidhe Council | Wilderness | 20–30 | varies | forest |
| UCAS (other cities) | Settled | 5–20 | A/B | urban |
| CAS (southern states) | Settled | 5–20 | A/B | urban |
| Aztlan (Mexico+Central Am.) | Contested | 15–30 | varies | urban/jungle |
| Amazonia | Wilderness | 20–35 | varies | jungle |
| Allied German States | Settled | 10–20 | A/AA | urban |
| Rhine-Ruhr Megaplex | Corporate | 15–25 | AA/AAA | urban |
| Japan / Chiba | Corporate | 15–25 | AA/AAA | urban |
| Hong Kong FEZ | Contested | 15–30 | varies | urban |
| Chicago Containment Zone | Endgame | 30+ | Z | ruins |
| Glow City (Redmond) | Endgame | 25+ | Z | toxic |
| Zurich-Orbital | Special | 40+ | AAA | vacuum |

### Biomes

A biome is content — a YAML file in `content/shadowrun/biomes/*.yaml` keyed by the
`terrain:` string a room sets. Adding a new biome is just authoring a file; no engine enum
or migration.

| Biome | Where | Notes |
|---|---|---|
| `urban` | Most sprawl districts | Default for streets, buildings, shops. |
| `corp-enclave` | Bellevue, arcologies, corp campuses | High security; clean; SIN-checked entrances. |
| `barrens` | Redmond, Puyallup, Auburn Z-zones | Urban decay; no utilities; gangs; ambient hazard. |
| `industrial` | Renton, Everett, Tacoma docks | Warehouses, factories, freight yards. |
| `underground` | Ork Underground, sewers, sub-levels | No natural light; complex 3D navigation. |
| `military` | Fort Lewis, corporate military sites | Restricted; armed perimeter; AAA response. |
| `forest` | Snohomish, SSC lands, Tír Tairngire | Reclaimed nature; paracritters; awakened hazards. |
| `jungle` | Amazonia, Yucatán, Aztlan south | Dense vegetation; paracritters; blood magic sites. |
| `toxic` | Glow City, contaminated zones | Radiation/toxin hazard; mutant life; no normals. |
| `ruins` | Chicago CCZ, ghost towns | Structural collapse; insect spirits; no law. |
| `astral` | The Astral Plane | Magic-only access; spirits; mana ebbs and flows. |
| `matrix` | The Matrix (virtual) | Wireless overlay everywhere; hosts as rooms. |
| `vacuum` | Zurich-Orbital, space stations | Sealed environment; pressure hazard; elite security. |
| `rural` | Snohomish farms, NAN land | Low-density; paracritter encounters; quiet. |

### Climate / weather

Unlike WoT's fantasy climates, weather in SR is grounded in real Pacific Northwest / global
geography. Seattle's climate (`pacific-northwest`) drives fog, rain, overcast defaults.
Each sprawl has a `weather_zone:` set to its real-world climate baseline, modified by
pollution and magical mana flows where relevant (Awakened areas may have unusual phenomena).

### Off-world / off-network realms

- **The Astral Plane** — accessible only to Awakened characters (magicians, adepts
  partially). A parallel magical reality overlaying the physical. Spirits, wards, mana
  barriers exist here. Design as an overlay area, not a separate world per se.
- **The Matrix** — wireless virtual reality overlay accessible via commlink or direct
  neural link (DNI). Hosts are structured virtual spaces (corp intranets, arcologies, data
  vaults). Design hosts as instanced areas with IC (Intrusion Countermeasures) as mobs.
- **Metaplanes** — deep astral reaches accessed via Astral Quest; highly magical, spirit-
  ruled planes. Endgame content; design deferred.

---

## SIXTH WORLD KEY CONCEPTS (Builder Primer)

Before building, internalize these setting-defining facts that shape every room description.

### The Awakening & Magic
Magic returned to the world on 2011-12-24 — Goblinization, dragon emergence, and the
return of active spellcasting. By 2075 magic is normalized but still feared and regulated.
Mana levels vary by location (mana ebbs in industrial zones; mana flows in wilderness).
Dual-natured creatures (both physical and astral presence) are common paracritters.

### Goblinization & Metatypes
- **Humans** — still the majority
- **Elves** — long-lived, often in Tír Tairngire or corporate elite roles
- **Dwarves** — stocky, often technical/engineering roles
- **Orks** — discriminated against; concentrated in barrens (Ork Underground, Redmond)
- **Trolls** — rare; often in barrens or heavy labor
- **Other metatypes** — changelings (post-Halley's Comet 2061 mutations), etc.

### SINs & SINless
A System Identification Number (SIN) is your legal identity. Without one you are SINless —
no legal employment, no official housing, no rights. Runners often carry multiple fake SINs.
Security zones check SINs at entry points; Z-zones don't bother. SINless people are
invisible to the legal system, which is both freedom and vulnerability.

### The Matrix (2075 version)
The current Matrix is wireless, Augmented Reality (AR) overlaid on the physical world, and
immersive Virtual Reality (VR) for direct connection. It replaced the old wired grid after
Crash 2.0 (2064). Corporate grids (G-zones) offer better speeds but corporate monitoring.
The public grid is slower and open. Rogue AI and dangerous IC inhabit deep hosts.

### Nuyen (¥)
The global currency. Corp scrip also exists but is corp-specific. Gold, barter, and
commodity exchange dominate the Z-zones where nuyen has less meaning.

---

## THE BIG TEN: MEGACORPORATIONS (2075)

The real power in the Sixth World. All ten hold extraterritoriality — their property is
effectively a sovereign nation. Corp security is not "cops" — it's a private army.

| Corp | Court Rank | HQ | CEO | Industries | Security Arm |
|---|---|---|---|---|---|
| Saeder-Krupp | #1 | Essen, AGS | Lofwyr (Great Dragon) | Heavy industry, chemicals, finance, aerospace, BMW | S-K Security |
| NeoNET | #2 | Boston, UCAS | Richard Villiers | Matrix infrastructure, cyberware, electronics, biotech | NeoNET Security |
| Ares Macrotechnology | #3 | Detroit, UCAS | Damien Knight | Military hardware, arms, aerospace, automotive, entertainment | Knight Errant |
| Aztechnology | #4 | Tenochtitlán, Aztlan | Flavia de la Rosa | Consumer goods, food (Stuffer Shack), chemicals, magic, military | Securitech |
| Renraku Computer Systems | #5 | Chiba, JIS | Inazo Aneki | Data storage, archives, Matrix grids, arms | Red Samurai |
| Evo (fmr. Yamatetsu) | #6 | Vladivostok, Russia | Yuri Shibanokuji | Bioware, cybernetics, anti-aging, transhumanism | Evo Security |
| Horizon | #7 | Los Angeles, PCC | Gary Cline | Media, PR, social manipulation, entertainment | Horizon Security |
| Shiawase | #8 | Osaka, JIS | Korin Yamana | Nuclear power, environmental, biotech, consumer goods | Shiawase Security |
| Wuxing | #9 | Hong Kong FEZ | Wu Lung-Wei | Magical services, agriculture, engineering, chemicals | Wuxing Security |
| Mitsuhama Computer Technologies | #10 | Osaka, JIS | (rotating) | Heavy industry, robotics, cybertechnology, entertainment | Zero Zone |

**Slogan reference (flavor for rooms/NPCs):**
- S-K: "One Step Ahead" | NeoNET: "Tomorrow Runs on NeoNET" | Ares: "Making the World a Safer Place"
- Aztechnology: "The Way to a Better Tomorrow" | Renraku: "Today's Solutions to Today's Problems"
- Evo: "Changing Life" | Horizon: "We Know What You Think" | Shiawase: "Advancing Life"
- Wuxing: "We're Behind Everything You Do" | MCT: (ZZ security, no marketing needed)

### Key Security Force Notes
- **Knight Errant** (Ares) — Seattle's primary law enforcement contractor post-2075;
  replaced Lone Star after Lone Star's embarrassments. Professional, militarized.
- **Red Samurai** (Renraku) — elite corporate forces; highly disciplined, lethal.
- **Zero Zone** (MCT) — MCT facilities are legendarily unrunnable; kill-on-sight policy.
- **Lone Star** — lost the Seattle contract but still operates in other UCAS cities.
  Infamous for corruption in their Seattle days.

---

## NORTH AMERICA (2075)

The former USA and Canada no longer exist as unified nations. The continent fragmented.

### Nations

| Nation | Capital / Center | Notes |
|---|---|---|
| United Canadian and American States (UCAS) | Washington DC | Successor state to most of USA + Canada. Seattle is an exclave. |
| Confederation of American States (CAS) | Atlanta | Southern successor state. More conservative, horse culture in some regions. |
| Native American Nations (NAN) | (distributed) | Coalition of tribal nations claiming western North America. See sub-nations below. |
| California Free State (CFS) | Sacramento | Independent California. San Francisco is a major corp hub. |
| Pueblo Corporate Council (PCC) | Albuquerque | Corporate-dominated former Four Corners region. |
| Tír Tairngire | Portland, OR | Elven isolationist nation-state in Oregon. Secretive, high magic. |
| Tír na nÓg | Dublin, Ireland | Irish elven nation. |
| Quebec | Quebec City | French-Canadian separatist republic. |
| Trans-polar Aleut Nation | Alaska | Native Alaskan nation. |
| Aztlan | Tenochtitlán (Mexico City) | Aztec-revivalist megastate; essentially Aztechnology with a flag. Aggressive expansionism. |

### Native American Nations (NAN sub-nations)

| NAN Nation | Territory | Notes |
|---|---|---|
| Salish-Shidhe Council (SSC) | Pacific Northwest | Surrounds Seattle. Access to Puget Sound. |
| Sioux Nation | Great Plains | Military power; Wildcats spec ops are famous. |
| Pueblo Corporate Council | Four Corners | Corporate-dominated; PCC runs like a business. |
| Athabaskan Council | Northern Canada/Alaska | Large territory, low population. |
| Algonkian-Manitou Council | Eastern Canada | Forest and tundra territory. |

### Denver Front Range Free Zone

The neutral zone city where all major powers have a quarter: UCAS, CAS, NAN (Sioux), Aztlan,
Pueblo. A natural setting for intrigue, faction play, and smuggling. The city is divided
into sectors by treaty.

---

## SEATTLE METROPLEX (Primary Setting)

**Status:** UCAS exclave; geographically surrounded by Salish-Shidhe Council territory
**Population:** ~3.5 million (2075)
**Law Enforcement:** Knight Errant (primary), Lone Star (secondary/legacy contracts)
**Governor (2075):** Josephine Doohan
**Rep:** The shadowrunning capital of the world. All AAA corps present. Multiple
competing interests; no single power dominates. Constant shadow war under a veneer of civility.

Seattle is divided into **12 districts**, each with its own character, security rating,
and dominant power.

---

### DOWNTOWN SEATTLE

**Security:** AA / A (varies by neighborhood)
**Character:** Corporate towers, government buildings, upscale nightlife, the political class.

| Neighborhood / Location | Notes |
|---|---|
| Arcology Commercial and Housing Enclave (ACHE) | Former Renraku Arcology — scene of 2059 AI disaster (Deus). Now reclaimed, renamed. Mixed security. |
| Ballard | Northwest neighborhood; fishing heritage; working class with corp overlay. |
| Capitol Hill | Bohemian, arts, counterculture; relatively tolerant of metahumans. |
| Council Island | Salish-Shidhe Council sovereign territory in Puget Sound. Neutral ground. |
| Elven District | High concentration of elves; upscale; subtle Tír Tairngire influence. |
| International District | Asian cultural hub; Yakuza presence; restaurants, imports. |
| Ork Underground | Literal underground city beneath Downtown; ork/troll community; C/Z security. |
| Queen Anne Hill | Wealthy residential; corp elite homes; AA security. |
| Interbay and Magnolia Bluff | Mixed industrial/residential; port access. |
| Seattle Center | Former 1962 World's Fair site; converted to entertainment/corp venue. |
| University District | UW campus; student population; moderate security; tech and magical research. |

**Notable locations:**
- **Dante's Inferno** — famous (infamous) nightclub; nine levels themed after Hell; neutral shadow ground; Mr. Johnsons meet runners here
- **The Big Rhino** — ork-run bar; major social hub for metatypes
- **Penumbra** — upscale nightclub; where corp types go
- **Alabaster Maiden** — high-end escort/social club; major info brokerage location

---

### BELLEVUE

**Security:** AA
**Character:** Wealthy corporate residential. Walled, manicured, gated. The other side of the wall from Redmond.

| Location | Notes |
|---|---|
| Bellevue proper | Upscale malls, corp housing, private schools. SIN-checked entry. |
| Hunt's Point | Ultra-wealthy enclaves; private security augmenting KE. |
| Medina | Old-money corp-exec neighborhood; AAA-equivalent private security. |
| Factoria | Industrial/commercial zone at Bellevue's edge; more accessible. |

The Bellevue/Redmond border is walled and guarded — the literal line between the haves
and the have-nots of Seattle.

---

### REDMOND

**Security:** Z (Barrens) / C (Touristville border only)
**Character:** The Redmond Barrens. Urban decay and lawlessness. A partial nuclear meltdown
at the Trojan-Satsop plant in 2013 created **Glow City** — a radioactive wasteland in
southeastern Redmond. Three-quarters of residents are SINless.

| Area | Notes |
|---|---|
| Glow City | Radioactive contamination zone; mutant paracritters; treasure hunters die here. Biome: `toxic`. |
| Touristville | The relatively safe (C security) enclave near the Bellevue border. Bars, chop shops, fixers. |
| Sophocles | Neighborhood with decent community organization; one of the less wretched Barrens zones. |
| Avondale | Gang territory; Rusted Stilettos turf. |
| Hollywood (Redmond) | Entertainment ghetto; illegal BTL parlors; Halloweeners fringe presence. |
| Brain Heaven | BTL den district; dangerous. |
| Kingsgate | Mixed gang / squatter territory. |
| Plastic Jungles | Industrial ruins turned gang playground. |
| Purity | Zone fighting for some semblance of order; vigilante groups active. |
| Woodinville | Northern Redmond; slightly more stable; border with Snohomish. |

**Major gangs:**
| Gang | Type | Notes |
|---|---|---|
| Rusted Stilettos | Ork/Troll gang | Major territory holders; anti-human violence common. |
| 162s | Ghoul gang | Glow City and surrounds; very dangerous. |
| Brain Eaters | Thrill gang | Violent, random; named for their hobby. |
| Red Hot Nukes | Dwarf gang | Glow City adjacent; radiation-resistant by mutation/cybernetics. |
| Spiders | Mixed | Fanatical hatred of Insect Spirits; surprisingly organized. |
| Crimson Crush | Mixed | East Redmond along 228th Ave; large and territorial. |
| Halloweeners | Mixed | Halloween-themed violence; presence here and Downtown fringe. |

---

### PUYALLUP

**Security:** Z / C (at its best)
**Character:** The other major Barrens. Built over volcanic geography; Mount Rainier looms.
Partially burned in gang wars; toxic zones near the volcano's ash fields. Strong criminal
underworld; smuggling hub.

| Area | Notes |
|---|---|
| Carbonado | Mining remnant; independent; violent. |
| Loveland | Brothel and entertainment district; rough but functional economy. |
| Tarislar | Elven refugee camp-turned-permanent settlement; poverty; Ancients gang presence. |
| Hell's Kitchen | Restaurant and entertainment district; surprisingly alive. |
| The Ash Flat | Near Mt. Rainier ash deposits; toxic/volcanic biome. |
| Puyallup City | District center; C security at best; corrupt local officials. |

**Major gangs:**
| Gang | Notes |
|---|---|
| Ancients | Elven go-gang; national org; strong Puyallup presence. Tír Tairngire sympathies. |
| Razor Heads | Local thrill gang. |
| Scatterbrains | Clown-themed; warehouse district. |

---

### RENTON

**Security:** B / C
**Character:** Working class, industrial, anti-metahuman sentiment. Humanis Policlub HQ is
here. More gangs than any district except Redmond. The district has "definitely seen better
days."

| Area | Notes |
|---|---|
| Lake Youngs housing | Nicest part of Renton; AA security micro-zone. |
| Renton industrial corridor | Warehouses, small manufacturing; B security. |
| Humanis HQ | Anti-metahuman political organization's Seattle base. |

**Major gangs:** Blood Mountain Boys (go-gang), Night Hunters (thrill gang).

**Notable locations:**
- **Auburn General Hospital** — some research on mystic cyberware compatibility (Mafia-funded).
- **Greasy Ben's** — diner and illegal body shop; classic runner resource.
- Mafia and Yakuza are both very active here.

---

### SNOHOMISH

**Security:** B / C
**Character:** Rural. Farmland, forests, small towns. Most open land in the Metroplex.
Paracritters from SSC land wander in. Quieter, slower, but still has its darkness.

| Area | Notes |
|---|---|
| Snohomish Town | Small city center; B security; agricultural economy. |
| Rural farmland | Extensive; some corporate agri-operations; some independent. |
| Monroe | Small town; county seat feel; corrections facilities. |
| Wilderness fringe | Borders SSC land; occasional spirit and paracritter incursion. |

---

### EVERETT

**Security:** B / C
**Character:** Port and industrial. Federated Boeing's primary Seattle facility is here —
a quasi-AAA enclave within a B-security district. Waterfront is active; smugglers use
it extensively.

| Area | Notes |
|---|---|
| Federated Boeing campus | AAA security island within B district; huge aerospace footprint. |
| Everett port | Active commercial and smuggling port. |
| Marysville | Northern Everett; residential working class. |

---

### TACOMA

**Security:** B / C
**Character:** Port city. Heavy industrial, chemical plants, organized crime (Finnigan Family).
The smell of the Tacoma Narrows is infamous. Chemical spills are not uncommon.

| Area | Notes |
|---|---|
| Tacoma port | Major Pacific Northwest freight hub; Finnigan crime family territory. |
| Dome District | Arena and entertainment district around the old Tacoma Dome. |
| Hilltop | Low-income residential; C security; gang presence. |
| Federal Way | South Tacoma; border zone; light industrial. |

**Organized crime:** The Finnigan Family (Irish Mafia) dominate the port and much of Tacoma.

---

### AUBURN

**Security:** C / Z
**Character:** Low-income, declining. Bridges the gap between Renton's semi-order and
Puyallup's chaos. Significant SINless population.

---

### FORT LEWIS

**Security:** AAA (military)
**Character:** UCAS military installation. Joint base for multiple military branches.
Not a runner destination — it's a target or a heavily avoided zone. Trespassing means
military response, not civilian law enforcement.

---

### COUNCIL ISLAND

**Security:** A (SSC sovereign territory)
**Character:** Salish-Shidhe Council sovereign ground in Puget Sound. Native American
community; different laws than Seattle proper; no corp extraterritoriality applies here.
A genuine neutral zone for certain negotiations.

---

### ORK UNDERGROUND

**Security:** Z / C
**Character:** A subterranean city beneath Downtown, built by the ork and troll community
that was pushed out of surface Seattle. Has its own economy, culture, and governance.
Growing; increasingly organized; politically assertive. Biome: `underground`.

Notable groups: Skraacha (ork gang turned community defenders — not strictly criminal).

---

## SEATTLE FACTION ROSTER

One-line entries to anchor mob/NPC builders. Expand as factions are mechanically wired.

### Corporate Presence in Seattle (all AAA corps have significant footprint)
- **Ares / Knight Errant** — primary law enforcement contract; extensive presence
- **Aztechnology** — consumer goods, Stuffer Shack franchises everywhere
- **Horizon** — media, entertainment; significant PR/surveillance presence
- **MCT** — discrete; their facilities are notoriously secure
- **NeoNET** — Matrix infrastructure; grid ownership in Seattle
- **Renraku** — rebuilding after ACHE disaster; data and archive services
- **Saeder-Krupp** — heavy industry and finance interests; BMW dealerships
- **Shiawase** — environmental and energy; power infrastructure
- **Wuxing** — magical services; mystical shops; discreet influence
- **Evo** — metahuman-positive; clinics in metahuman-heavy districts

### Law Enforcement
- **Knight Errant** — primary Seattle law enforcement contractor (replaced Lone Star ~2073)
- **Lone Star** — lost main contract; still operates some subsidiary/supplemental contracts
- **Star** (slang) — generic term runners use for whichever corp holds the law contract

### Organized Crime
- **Finnigan Family** — Irish Mafia; Tacoma and port operations; old money
- **Yakuza (Watada-rengo)** — Japanese organized crime; significant Seattle presence
- **Mafia** — various families; Renton, Downtown operations
- **Triad** — International District and Asian community influence
- **Vory v Zakone** — Russian organized crime; smaller presence but growing

### Shadowrunner Community
- **Fixers** — critical NPCs; connect runners to jobs (Mr. Johnson contacts)
- **Mr. Johnson** — generic name for corporate/criminal contact hiring runners; can be any gender
- **Street docs** — unlicensed medical; runner essential; found near barrens borders
- **Talismongers** — magical supply shops; Awakened community

### Political Organizations
- **Humanis Policlub** — anti-metahuman political org; legal but violent-adjacent; Renton HQ
- **Mothers of Metahumans (MOM)** — metahuman civil rights; counter to Humanis
- **Tír Tairngire government** — political influence through the Elven District
- **SSC tribal representatives** — Council Island presence; SSC political interests

### Gangs (see district sections above for specifics)
Major cross-district gangs:
- **Ancients** — elven go-gang; national org; Tír sympathies
- **Halloweeners** — holiday-themed violence; Downtown/Redmond fringe
- **Spikes** — Interstate 5 corridor; go-gang

---

## NORTH AMERICAN SPRAWLS (Beyond Seattle)

### Boston, UCAS (NeoNET HQ)
The Matrix capital of North America. NeoNET dominates. High-tech, corp-dense. Scene of the
2075 "Lockdown" event (Boston Lockdown — NeoNET blamed for disaster; leads to corp's eventual
dismantling in 2079). For 2075 play: the Lockdown has just happened or is ongoing.

### Detroit, UCAS (Ares HQ)
Ares Macrotechnology's home city. Knight Errant originated here. Heavy industry;
manufacturing; car culture. Gritty corporate city with a Midwestern edge.

### Chicago, UCAS (Containment Zone)
The **Chicago Containment Zone (CCZ)** is a walled-off disaster area after the 2055 Insect
Spirit infestation and Bug City incident. The core of the city is sealed; Bug City is an
endgame Z-zone. The suburbs function but live under the shadow of the CCZ. Insect Spirits
(ant, wasp, roach spirits) still active inside.

### Atlanta, CAS
Capital of the Confederation of American States. More politically conservative; corp
presence but with stronger state governance than UCAS. CAS military is capable.

### Denver, Front Range Free Zone
Multi-national shared city; UCAS, CAS, NAN (Sioux), Aztlan, and Pueblo each hold a
sector. The Denver Accord governs the arrangement. A natural neutral-ground setting for
pan-faction intrigue.

### Los Angeles, PCC (Horizon HQ)
Hollywood is still Hollywood, except it's now corpo-entertainment through Horizon. Aztlan
influence from the south; PCC governance; Spanish heavily spoken. Metahuman entertainment
industry is large.

---

## EUROPE (2075)

### Allied German States (AGS)
Germany reformed as a loose federation after the Euro-Wars. Saeder-Krupp dominates from
Essen; the Rhine-Ruhr Megaplex is the largest European sprawl.

**Rhine-Ruhr Megaplex:** Essen, Dortmund, Cologne, Düsseldorf merged into one industrial
megalopolis. Saeder-Krupp HQ is here. The great dragon Lofwyr effectively runs the region.
AA/AAA security in corp zones; B/C in working-class industrial areas.

### Berlin, AGS
Divided city. East Berlin retains some of its old character; West Berlin is heavily
corp-developed. A major shadowrunning city — diverse factions, multiple power players, squat
culture in abandoned areas. The Wall (a different one) separates corp-zone from squat zones.

**Notable:** Berlin's anarchist squat districts are runner-friendly; Matrix infrastructure
is contested between corps and hackers; Troll Kingdom in the southern industrial districts.

### United Kingdom (Treaty of Portsmouth)
The UK exists but is diminished; Scotland has some autonomy; Wales is partly elven territory.
London remains a major financial hub and corp center.

**London:** Finance capital. City of London is AAA corp territory. East End barrens. Significant
Wuxing and Saeder-Krupp presence. Underground (the Tube) has shadowrunner uses.

### Tír na nÓg (Ireland)
Irish elven nation-state. Similar to Tír Tairngire but older establishment. Magic-rich;
isolationist; beautiful but controlled. Highly magical environment.

### France (Paris)
Paris survived the Euro-Wars and remained French. Major cultural and corp presence.
Azzies (Aztechnology) have significant influence here despite European resistance. The
Paris catacombs have acquired new significance with Awakening.

---

## ASIA (2075)

### Japanese Imperial State (JIS)
Japan reconstituted as an Imperial State with genuine imperial authority restored. Militaristic;
corporate power (Renraku, Shiawase, MCT, Evo all HQ'd in Japan). Highly technological;
strict SIN enforcement; metahuman discrimination (orks/trolls heavily marginalized).

**Tokyo:** Megacity. Population density extreme. MCT, Shiawase, Renraku all have major
facilities. Yakuza is woven into every layer. Security varies wildly by ward: Shinjuku
(entertainment/crime), Shibuya (corp/youth), Akihabara (tech/Matrix), Roppongi (foreign/corp).

**Chiba:** Purpose-built corporate city; Renraku HQ. Highest concentration of tech in the
world. Called the "city of chrome."

**Osaka:** Shiawase HQ. Industrial and financial. Less flashy than Tokyo; more work, less play.

### Hong Kong Free Enterprise Zone
China collapsed into ~10 successor states; Hong Kong was left without a national government
and became a Free Enterprise Zone. No national oversight = maximum corp and criminal freedom.
Wuxing HQ. Kowloon Walled City is its own dark mystery. Runner heaven; everything is for sale.

**Kowloon Walled City:** An entirely autonomous district even within Hong Kong FEZ. Triads,
spirits, dark magic, poverty, crime, and freedom all compressed into dense urban space.
Z-equivalent security despite being technically inside a city; its own rules apply.

### China (fragmented)
Multiple successor states. The largest are Manchuria (north), Canton Confederation (south),
and the Wuhan Triarchy (central). Political and military instability; corp exploitation.

### Russia (various)
Russia fractured. Evo moved its HQ to **Vladivostok** which is now effectively an Evo company
town. Moscow is the remnant Russian state capital. Vory v Zakone (Russian mob) fills gaps
in state authority across the former Russian territories.

---

## CENTRAL & SOUTH AMERICA (2075)

### Aztlan
Mexico and Central America unified under Aztechnology's guidance, rebrand, and genuine
ideological embrace of Aztec cultural revival. Aztlan is effectively a corporate nation-state.
Blood magic is a state secret but widely suspected. The government and Aztechnology are
inseparable. Expansionist — has taken former US territories in the southwest.

**Tenochtitlán (Mexico City):** Aztlan capital and Aztechnology HQ. Rebuilt in Aztec-revival
architectural style. Beautiful, prosperous surface; dark secrets underneath.

### Amazonia
The great dragon **Hualpa** rules Amazonia — most of South America's rain forest, now
a dragon's personal domain. The most biodiverse region on Earth; paracritters everywhere.
Corp presence suppressed; Amazonia is militantly anti-corp exploitation. Called a
"dragon-run eco-paradise with a dictator." The sprawl of **Metrópole** is its showcase city.

### Metrópole (Amazonia)
Hualpa's model city; 100+ km of coastline. Green architecture; corporate restrictions;
environmental showcase. Unusual for the Sixth World: a major city where corps take a back seat
to government — because the government is a great dragon.

---

## SPECIAL ZONES (Cross-Reference)

### The Matrix (Virtual)
The wireless Matrix is an overlay on the physical world accessible from almost anywhere via
commlink. For MUD purposes, model Matrix hosts as instanced areas:

| Host Type | Equivalent | Notes |
|---|---|---|
| Corporate host (local) | A-AA security area | Internal data; IC (Intrusion Countermeasures) as mobs. |
| Corporate host (major) | AA-AAA security area | Red IC; Black IC; Spider (security hacker) NPCs. |
| Public grid node | B security | Open; monitored; slow. |
| Rogue/pirate host | Z security | No rules; Resonance wells; AI encounters. |
| Resonance Realm | Special/Endgame | Pure Technomancer territory; living Matrix space. |

**Matrix biome:** `matrix`. All Matrix rooms are indoors/virtual; no weather; time feels
compressed. Sprites (Matrix spirits) are Matrix equivalents of astral spirits.

### The Astral Plane
Accessible only to Awakened characters. Overlays the physical world but shows the magical
"true" nature of things. Corp buildings appear as grey dead zones; forests glow with life;
people with strong emotions blaze astrally. Spirits are native here. Wards appear as walls.
Mana ebbs (low magic areas) and mana flows (high magic areas) shape the landscape.

| Astral Zone Type | Physical Equivalent | Notes |
|---|---|---|
| Astral corp enclave | Corp host in Matrix | Grey, lifeless, warded heavily. |
| Astral barrens | Toxic/industrial | Polluted astral; background count; spirit sickness. |
| Astral Awakened forest | Wilderness/forest | Brilliant mana flow; spirit activity; dangerous but beautiful. |
| Metaplane | Endgame special | Deep astral; spirit domains; Astral Quest destinations. |

### Chicago Containment Zone (CCZ)
The most dangerous location in North America for non-endgame characters. Walled off by
UCAS military after the 2055 Insect Spirit infestation. Inside: structural collapse, Insect
Spirit hives, feral metahumans, desperate survivors. The city is effectively a ruin.
Biome: `ruins`. Security: Z (but the threat is monsters, not gangers).

Insect Spirit types found here: Ant, Wasp, Roach, Fly, Termite queens and their drones.

### Glow City (Redmond, Seattle)
Southeastern Redmond around the Trojan-Satsop contamination zone. Radioactive; mutant
paracritters; "glow ghouls" and other radiation-adapted life. Biome: `toxic`.
Even gangers avoid it; BTL addicts and desperate treasure hunters brave it.

### Zurich-Orbital
The Corporate Court's home — a space station in Earth orbit. The Zurich-Orbital Gemeinschaft
Bank (ZOG) is here; the wealthiest entities in the world have accounts. AAA security; vacuum
environment. Not a runner destination — it's where Corporate Court deliberations happen and
where the ultra-elite live and scheme.

---

## AWAKENED GEOGRAPHY (Mana Flows & Special Sites)

Unlike the WoT setting where magic is systemic, Shadowrun magic is geographically uneven.
Mana concentrations create sites of special significance.

| Site Type | Effect | Example Locations |
|---|---|---|
| **Power site** | High mana; magic enhanced; spirits powerful | Ancient ruins, sacred native sites, untouched wilderness |
| **Mana ebb** | Low mana; magic suppressed; spirits avoid | Heavy industrial zones, polluted areas, CCZ |
| **Mana flow** | Ley line; magic enhanced along a path | Rivers of mana connecting power sites |
| **Background count** | Residual emotional/violent imprint; disrupts magic | Battlefields, mass death sites, toxic zones |
| **Astral rift** | Tear between physical and astral | Post-disaster sites; Deep Resonance anomalies |

**Seattle-specific awakened notes:**
- Puget Sound has significant water spirit activity (SSC's spiritual relationships)
- Snohomish forests hold mana flows from SSC sacred sites
- Glow City has severe background count from radiation and death
- Council Island is a power site (SSC protected)
- The Ork Underground has a surprisingly stable low-level mana flow (community spiritual life)

---

## PARACRITTERS (Builder Reference)

Paracritters are the Shadowrun equivalent of fantasy monsters — real-world animals mutated
or Awakened by the return of magic.

| Paracritter | Biome | Danger | Notes |
|---|---|---|---|
| Devil Rat | Urban/Barrens | Low | Giant rats; pack predators; everywhere in Z-zones. |
| Hellhound | Urban/Barrens | Medium | Fire-breathing dog; guard animal / wild predator. |
| Barghest | Barrens/Forest | Medium | Wolf-like; shadow-stepping; hunts at night. |
| Toxic spirits | Toxic/Industrial | High | Spirits of polluted areas; poison/disease aura. |
| Insect spirits | CCZ / urban | Very High | Ant/Wasp/Roach spirits; possess metahumans; CCZ endemic. |
| Wyvern | Mountain/Forest | High | Dragon-adjacent flying reptile; less intelligent than true dragons. |
| Piasma | Northern Forest/NAN | High | Great bear variant; very large; territorial. |
| Sasquatch | Forest (NAN) | Low-Med | Intelligent metahuman-adjacent; shy; NAN protected. |
| Blood spirit | Aztlan | Very High | Aztechnology blood magic construct; extremely dangerous. |
| Free spirit | Anywhere | Varies | Spirits with own agendas; can be ally or deadly enemy. |
| Great Dragon | Global | Endgame | Lofwyr, Hualpa, Dunkelzahn (deceased), Ghostwalker, etc. |

---

## NOTABLE RUNNER LOCATIONS (Key Rooms / Areas)

The equivalent of WoT's famous inns and keeps — places every runner knows.

| Location | District | Notes |
|---|---|---|
| Dante's Inferno | Downtown | Nine-level nightclub; neutral shadow ground; iconic Mr. Johnson meet location. |
| The Big Rhino | Downtown | Ork-run bar; metahuman-friendly; rough crowd; trustworthy. |
| Penumbra | Downtown | Upscale club; corp exec clientele; info for those with manners. |
| Alabaster Maiden | Downtown | High-end escort/social club; critical info brokerage. |
| Touristville bars | Redmond | Various; runner-friendly; cheap; dangerous adjacent. |
| Novelty Hill Sleep & Eat | Redmond | No-frills cubicle hotel; cheap; used by those in a hurry. |
| Downfall | Redmond | Bar; safe-ish for Redmond; neutral runner ground. |
| Jackal's Lantern | Redmond | Halloweener hangout; barbed wire decor; avoid unless Halloweener contact needed. |
| Greasy Ben's | Renton | Diner + illegal body shop; runner staple. |
| Auburn General Hospital | Renton | Legitimate hospital with shadow-side research; street doc adjacent. |
| Renraku Arcology (ACHE) | Downtown | Former catastrophe; now reclaimed mixed-use zone; still has dark corners. |

---

## WEAPONS, CYBERWARE & GEAR (Builder Notes)

Unlike WoT's swords and saddles, SR gear is the character system. Rooms and vendors
should reflect the tech level.

### Tech tiers (maps to item rarity in content)
| Tier | Availability | Examples |
|---|---|---|
| Street | Openly sold; no license | Knives, basic firearms, low-end commlinks |
| Restricted (R) | License required; black market common | Silenced weapons, light armor, basic cyberware |
| Forbidden (F) | Illegal; shadow market only | Military weapons, heavy cyberware, BTL chips |
| Military (M) | Corp/military only; extremely rare | Tank weapons, full-body conversion cyberware |

### Gear categories for content
- **Firearms** — Pistols, SMGs, Rifles, Shotguns, Heavy weapons; ammo types matter (APDS, hollow point, etc.)
- **Melee** — Monofilament wire, vibroblade, shock gloves, combat axe
- **Armor** — Lined coat, armor jacket, full body armor, riot gear
- **Cyberware** — Reflexes (wired reflexes, synaptic booster), muscle replacement, datajack, smartlink, cybereyes
- **Bioware** — Muscle toner, synaptic booster, adrenal pump, orthoskin
- **Drones** — Fly-spy, Steel Lynx (combat), Rotodrone; Rigger-operated
- **Commlinks** — Personal computers/phones; Matrix interface; cheap to expensive
- **Credsticks** — Cash equivalent; registered (traceable) vs. anonymous (clean)
- **BTL chips** — Better-Than-Life; illegal wireheading; addictive; everywhere in barrens

---

*Geography compiled from SR5 core rulebook, Sixth World Almanac, Seattle Sprawl sourcebook,
Run & Gun, and setting knowledge as of ~2075 (SR5 era). The AnotherMUD `shadowrun` content
pack is authored from this gazetteer; see `docs/themes/sr-world-plan.md` for the build
sequence and `docs/ENGINE-VOCABULARY.md` for the content↔engine contract.*

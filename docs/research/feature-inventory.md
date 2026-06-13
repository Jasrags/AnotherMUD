# MUD Research — Cross-Game Feature Inventory

Compiled: 2026-06-09
Sources: Aardwolf, Achaea, BatMUD, Discworld MUD (search snippets), Evennia, Ranvier

This document is the master cross-reference. Each section is a feature category with notes
on how different MUDs have implemented it. Use as inspiration and comparison when writing
feature proposals and specs for AnotherMUD.

---

## Directory

```
mud-research/
├── aardwolf/
│   ├── features-index.md
│   ├── auction-system.md
│   ├── quests-gquests-campaigns.md
│   ├── clans.md
│   └── warfare-manors-remort-channels.md
├── achaea/
│   ├── features-overview.md
│   ├── crafting-tradeskills.md
│   ├── player-economy.md
│   └── housing.md
├── batmud/
│   └── features-and-guilds.md
├── discworld/
│   └── features-notes.md
└── feature-inventory.md  ← this file
```

---

## Combat & Progression

| Feature | Aardwolf | Achaea | BatMUD | Discworld |
|---|---|---|---|---|
| Levels | 100 + superhero | levels to 99 + Dragonhood | 100 levels | use-based |
| Remort/Rebirth | Yes (7 classes) | No direct equivalent | Yes (reincarnation) | No |
| Tiers | Yes (post-7-remort) | — | — | — |
| Use-based skills | — | — | — | Yes |
| Class system | 7 classes | 21 classes | 5 backgrounds + 30+ guilds | classless (guild-structured) |
| Races | 7 races | 14 races | 44 races | many |
| Stat system | extensive | stats | stats | skill tree |
| PvP | clan-based + wars | opt-in + city conflict | partying + raiding | guild PvP |
| Affliction/curing | — | deep (tunable curing) | — | — |

---

## Grouping

| Feature | Aardwolf | Achaea | BatMUD |
|---|---|---|---|
| Party/group | Yes | Yes | Yes (partying) |
| Raid groups | — | — | Yes (party linking) |
| War grouping | Yes (any level during wars) | — | — |
| XP sharing | Yes | Yes | Yes |
| Combat stats tracking | Yes | Yes | Yes |

---

## Quests & Content

| Feature | Aardwolf | Achaea | BatMUD | Discworld |
|---|---|---|---|---|
| Standard quests | Yes (quest master) | Yes | Yes | Yes |
| Global/competitive quests | Yes (GQuest) | — | — | — |
| Campaigns (kill-list timed) | Yes | — | — | — |
| Area quests/goals | — | Yes | Yes | Yes |
| Exploration rewards | ranking system | Fellowship of Explorers | — | — |
| Quest currency | Quest Points | — | — | — |
| Trivia system | Yes | — | — | — |

---

## Economy & Trade

| Feature | Aardwolf | Achaea | BatMUD |
|---|---|---|---|
| Player shops | — | Yes (limited/scarce) | Yes |
| Auction house | Yes (dual-currency) | — | — |
| Direct player trade | — | — | Yes |
| Mining | — | Yes (Legions system) | — |
| Commodity market | — | Yes (Delos) | — |
| Trade guilds/carts | — | — | Yes |
| Lottery | Yes | — | — |
| Multiple currencies | Yes (gold + QP) | — | — |

---

## Crafting

| Feature | Aardwolf | Achaea | BatMUD | Discworld |
|---|---|---|---|---|
| Number of crafting skills | — | 16 tradeskills | — | many (undocumented) |
| Armour/weapon crafting | — | Yes | — | — |
| Food/cooking | — | Yes (Cooking) | — | Yes |
| Clothing/tailoring | — | Yes (Tailoring) | — | Yes (weaving) |
| Jewellery | — | Yes | — | — |
| Poisons | — | Yes (Toxicology) | — | — |
| Furniture | — | Yes (Furnishing) | — | — |
| Alchemy | — | — | Yes (guild) | — |
| Ship components | — | Yes (Shipfitting) | Yes (shipwrights) | — |

---

## Social & Communication

| Feature | Aardwolf | Achaea | BatMUD |
|---|---|---|---|
| Global channels | Yes | Yes | Yes |
| Race/class channels | Yes | — | — |
| Clan/guild channel | Yes | Yes | Yes |
| Friends list channel | Yes (unique) | — | — |
| In-game boards/notes | Yes | Yes | — |
| Marriages | Yes | — | — |
| Socials/emotes | Yes (extended) | Yes | Yes |
| Tells + offline inbox | Yes | Yes | Yes |
| InterMUD communication | — | — | Yes |

---

## Guilds / Clans / Factions

| Feature | Aardwolf | Achaea | BatMUD |
|---|---|---|---|
| Player organizations | Clans | Cities + Great Houses + Divine Orders | Guilds |
| Player governance | — | Elections, city leadership | — |
| Exclusive skills | Yes | Yes | Yes |
| Physical halls/buildings | Yes (clan halls) | Yes (player cities) | Yes (player cities) |
| PK/raiding | Yes (hall raids) | Yes (mine raids, wars) | Yes |
| Bank accounts | Yes | — | — |
| Racial guilds | — | — | Yes |

---

## Housing & World Building

| Feature | Aardwolf | Achaea | BatMUD |
|---|---|---|---|
| Personal housing | Yes (Manors) | Yes (room-by-room) | Yes (castles) |
| Player cities | — | Yes | Yes |
| Custom room descriptions | Yes | Yes | Yes |
| Servants/NPCs in housing | Yes (pet mobs) | Yes (customizable servants) | — |
| Furniture (player-crafted) | Yes | Yes | — |
| Hidden exits | — | Yes | — |
| Cost model | QP + gold | Commodities + gold | — |

---

## Navigation & World

| Feature | Aardwolf | Achaea | BatMUD | Ranvier |
|---|---|---|---|---|
| World map | — | Two continents + islands | Outerworld overland map | Coordinate rooms |
| Mounts | — | — | Yes (ridable) | — |
| Ships / seafaring | — | Yes (5 oceans) | Yes | — |
| Coordinate room system | — | — | — | Yes (optional) |
| Planes of existence | — | Yes | — | — |

---

## Unique / Noteworthy Features by MUD

### Aardwolf
- Friends channel (private group channel to chosen list)
- Dual-currency auction (gold + quest points)
- Remort auction (triggered by player remorti, QP currency)
- Wars with 4 types (genocide/class/race/clan)
- Manor waiting room + bell system (player-designed visitor management)
- Campaign timer is real-world, not game-world

### Achaea
- 16 named tradeskills with clear gathering→production chains
- Crafting directly feeds combat (curatives/venoms/weapons by players)
- Housing costs paid in commodities (not just gold) — economy integration
- Mining with Legions = economic PvP (raid/defend mines)
- Shop scarcity as status marker
- Dragonhood as level 99 prestige transformation (6 elements)
- 5 named oceans with multiple ship types for different purposes

### BatMUD
- 44 races including player-run races (invitation only)
- Racial guilds separate from background guilds
- Master Merchants and Navigator guilds spanning multiple backgrounds
- Combat statistics tracking (party/raid stats)
- Entire world on a single overland map

### Discworld
- Undocumented skills by design (discovery is content)
- 7 lives permadeath variant
- Non-persistent world, persistent inventory/vaults (hybrid)
- Player-elected magistrates with real power over city areas
- Hierarchical skill tree (classless in intent)

---

## Feature Gaps (common across MUDs, not in AnotherMUD yet per primer)

Based on cross-game survey, features that appear in multiple top MUDs but are not yet in
AnotherMUD:

- **Mounts** — ridable animals; BatMUD has, Achaea implies
- **Player housing** — Aardwolf (Manors), Achaea (room-by-room), BatMUD (castles)
- **Clan/guild system** — distinct from grouping; all three major MUDs have it
- **Board/note system** — in-game persistent forums (Aardwolf, Achaea)
- **Marriages** — Aardwolf
- **Fame/ranking/leaderboards** — Aardwolf (ranking system, plaques)
- **Global/competitive quests** — Aardwolf (GQuest)
- **Campaigns** (kill-list timed quest format) — Aardwolf
- **Seafaring** — Achaea, BatMUD
- **Player-built cities** — Achaea, BatMUD
- **Pets** (non-combat companions) — BatMUD (customizable)
- **Player governance** (elections, city leadership) — Achaea, Discworld
- **Racial guilds** — BatMUD

---

## URLs for Further Research

- Aardwolf: https://www.aardwolf.com / https://wiki.aardwolf.com
- Achaea: https://www.achaea.com/game-features / https://wiki.achaea.com
- BatMUD: https://www.bat.org/help
- Discworld: https://discworld.imaginary.com/lpc/playing/documentation.c (robots blocked)
- Evennia: https://www.evennia.com
- Ranvier: https://ranviermud.com

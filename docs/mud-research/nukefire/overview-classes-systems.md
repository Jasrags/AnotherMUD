# NukeFire MUD — Overview, Classes & Systems

Source: https://nukefire.org
Fetched: 2026-06-18
Successor to: TDome MUD (Thunderdome of Missoula, MT)
Theme: Sci-Fi post-apocalyptic wasteland — "Terminator meets Lord of the Rings meets Mad Max"
Engine: Circle lineage (heavily modified)

---

## Quick Facts

- 23,000+ rooms, 200+ zones
- 13 base classes, 9 prestige classes
- Endless remort system
- No permanent gear loss, no rent
- Active immortal team, frequent updates
- Live feed on website (gossip, auctions, events via GMCP)
- Screen reader accessibility support (sr command)
- GPS navigation system (Skynet GPS)
- Web client, TinTin++ web client, or standard MUD client (port 4000)

---

## Base Classes (13)

| Class | Description | Combat Role |
|---|---|---|
| Assassin | High-stakes targets, surgical precision, secretive | Stealth DPS |
| Barbarian | Mighty fighters, relies on Curists for healing | Melee tank/DPS |
| Curist | Divine caster, heals/buffs allies | Healer/support |
| Cyborg | Tech-enhanced human with cybernetic implants | Hybrid melee/tech |
| Fanatic | Gun-toting zealot, battle-prayers + close-quarters firepower | Gun/caster hybrid |
| Infiltrator | Covert operative, stealth + precision strikes | Stealth/gun |
| Knight | Holy warrior, combat + crucial support skills | Melee/support |
| Mutant | Radiation-hardened survivor, wretched and powerful | Melee bruiser |
| Pirate | Drunken swashbuckler, close-range combat | Melee DPS |
| Ranger | Wilderness guardian, tracker, terrain master | Utility/ranged |
| Samurai | Disciplined bushido warrior, body+soul as weapon | Melee DPS |
| Slinger | Potent spellcaster, devastating in groups | Spellcaster |
| Vagrant | Knife combat + dirty fighting, reads enemy movements | Rogue/DPS |

---

## Prestige Classes (9)
*Unlocked by accumulating class remorts in specific base classes*

| Prestige Class | Requirement | Description |
|---|---|---|
| Headhunter | 25 remorts in Cyborg + Barbarian | Cybernetic bounty hunter, gadgets + brutality |
| Ninja | 25 remorts in Assassin + Samurai | Decoys, illusion, swordplay, shadow strikes |
| Heretic | 25 remorts in Curist + Knight | Forbidden blessings/curses, occult renegade |
| Gypsy | 25 remorts in Pirate + Vagrant | Mysticism, deception, precision |
| Wolfman | 50 remorts in Ranger | Primal hunter, beast-mode combat |
| Kaiju | 50 remorts in Mutant | Radiation titan, crushing unstoppable power |
| Voidstriker | 50 remorts in Slinger | Shadow/space manipulation, hit-and-vanish |
| Occultist | 50 remorts in Fanatic | Outer Dark prayers, twists luck/flesh/bullets |
| Outlander | 50 remorts in Infiltrator | Gun-wielding, calm under fire, field medic |

---

## Progression System

### Remorts
- Remort = restart at level 1 in a new class while carrying forward accumulated power
- Each remort adds: damage, armor, movement, resistances, regeneration
- Remort count tracked globally and per class (class remorts)
- Prestige classes require high class remorts in specific base classes
- Endless — no cap mentioned

### Class Legacy (new system, May 2026)
- Available to prestige characters far enough into remorting
- Spend current remorts + class remorts to burn lasting **Remort Runes** into character
- Runes are permanent progression upgrades
- Milestones: 100-remort marks are protected — rune purchases can't drop below last 100-remort mark
- Example rune: Rune of the Candlekeeper's Claim → `candletrack <inscription>` to seek lost inscribed equipment
- Example rune: Rune of the Keymaster's Writ → `keycopy <key>` to remember a key pattern for session

---

## Equipment & Item Systems

### Item Types (from CSV)
- WEAPON
- ARMOR
- WORN (non-armor wearable: rings, accessories, etc.)
- IMPLANT
- TATTOO
- KEY

### Worn Locations (from CSV data)
Body slot examples seen:
- HEAD, FACE, JAW, SHOULDER
- BODY, LEGS, WAIST
- FINGER, WRIST, ACCESSORY
- WIELD, DUAL_WIELD
- IMP_FINGER, IMP_BACK, IMP_CHEST, IMP_SKULL, IMP_JAW (implant slots)
- TAT_FACE, TAT_JAW, TAT_FINGER, TAT_FOOT, TAT_WRIST, TAT_CHEST, TAT_ARM (tattoo slots)
- OVERSHIELD (special shield slot)
- TAKE (carryable)

### Item Flags (from CSV)
- SOCKET_OBJECT — has gem sockets
- VARIANT — variant/unstable crafted item
- GLOW, HUM — cosmetic/detection flags
- CONCEALABLE — can be hidden
- SHOP_BOUGHT — purchased from shop

### Item Stats / Applies (from CSV)
Combat: DAMROLL, HITROLL, FIGHTSPEED, DAM_REDUCTION, SPIKE_GUARD, DODGEROLL, BULLET_IMPACT_DAMAGE
Survivability: MAXHIT, MAXMANA, MAXMOVE, ARMOR, AC_APPLY
Regeneration: HIT_REGEN, MANA_REGEN, MOVE_REGEN
Magic: SPELLPOWER
Resources: GOLD (bonus gold carrying capacity or drop)
Stats: STR (strength)
Special: LEECH (life drain on hit)

### Item Quality Tiers
- ✪ MYTHIC ✪ — highest visible tier (seen on Festival Lederhosen, Stinger Tail Graft)
- VARIANT — crafted items from the instability system (may be warped, unstable, perfected)
- Standard — no prefix

### Rarity/Min Remorts
Items have a `min_remorts` field — you need at least N total remorts to use the item.
Range seen: 0 (anyone) to 60+ (high-end gear)

### Sockets & Jewels
Items with SOCKET_OBJECT can have socket jewels inserted. Socket jewels seen mentioned in news:
- "new socket jewels" in Mega-City One and high-remort areas
- Separate from implant modules

### Crafting System
- Material-based crafting (addmat command, saved material storage)
- Area-specific crafting recipes (Mutagenic Bonefoundry, Warlock's Sea-Den, Teklords)
- Foundry instability system: crafted items may emerge volatile/warped/unstable/perfected
- Unstable Foundry Assayer can rework Bonefoundry items using matching materials
- Crafting materials include: mutant bone, reactor slag, graftwire, marrow resin, living nerve parts, drowned things
- NukeBlasted Scrapyard is a beginner area that teaches crafting

### Soul-Bound Gear
At higher tiers, gear can be **bound to your soul** as permanent upgrades that grow with you.

---

## World & Navigation

### GPS System (Skynet GPS)
- `gps status` — current GPS status
- `gps route` — show current route
- `gps nearest` — find nearest destination
- Area searching tied into GPS network
- GMCP Char.GPS data for supported clients
- Supports route to specific zones

### Zone Scouting Tools (new June 2026)
- `zinfo` — check a zone's rough mob health, damage, damroll, remorts required, boss flags, XP value
- `zcompare <zone> <zone>` — compare two hunting areas side by side for risk vs. reward

### Item Discovery Tools
- `foundlist` — in approved zones, see which loadable objects have been found and how many remain
- `dbid <vnum>` — pull stored identify info for a found object without holding it

### Map System
- `bigmap` — continent overview
- `map memory on/off/reset` — control map memory
- `map legend` — tune displayed legend info

---

## Economy & Shops

### Modern Shop System (April 2026)
- Shop lists show upgrade markers (compare logic checks class, remorts, equipment, item rules)
- `list upgrades` — filter shop list for likely upgrades
- `identify <number>` — inspect a numbered shop item before buying

### Casino (Lost Wages)
- Progressive jackpot system across: Petrograd Bones, Slots, Blackjack, Roulette, Video Poker, Craps
- Each game tracks its own growing prize pool
- Skynet announces major jackpot wins
- Rare bonus item wins on qualifying wagers

---

## Social / Communication Channels

From the live feed on the website:
- **Gossip** — general chat
- **Auction** — item auction announcements
- **Skynet** — system/event announcements (jackpots, new players, area events)

Sources: Player, NPC, Immortal

---

## Accessibility

- **Screen reader mode** (`sr` command): sr setup, sr all, sr room, sr exits, sr afx, sr danger
- `afx sr` — plain affect summary for screen readers
- **Compact combat** — end-of-round summaries instead of individual combat lines
- Heavy gunfire/repeated actions resolve into summaries (shots, hits, misses, raw damage)
- Compact output, brief room descriptions, prompt none available

---

## Recent Major Systems (from news feed)

| Date | Feature |
|---|---|
| Jun 2026 | zinfo/zcompare zone scouting tools |
| Jun 2026 | Mutagenic Bonefoundry (crafting area with instability system) |
| Jun 2026 | Warlock's Sea-Den (crafting-focused area) |
| Jun 2026 | Rune of the Keymaster's Writ (Class Legacy rune) |
| May 2026 | Class Legacy system (Remort Runes, permanent progression) |
| May 2026 | Major skill scaling overhaul (scales with class remorts) |
| May 2026 | Screen reader and compact combat accessibility pass |
| Apr 2026 | Modern shop system with upgrade markers |
| Apr 2026 | Skynet GPS major upgrade (cleaner routes, GMCP data) |

---

## Key Resources

- Combat mechanics: https://nukefire.org/docs/combat-mechanics
- Item database (CSV): https://nukefire.org/api/items/csv
- Wiki (bot-blocked): https://nukefire.org/wiki
- Discord: https://discord.gg/B4pzagYaqR
- Interview: https://writing-games.org/nukefire-mud/

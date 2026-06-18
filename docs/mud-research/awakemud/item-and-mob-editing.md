# AwakeMUD — Item, Mob, Shop & Quest Editing

Source:
- https://github.com/luciensadi/AwakeMUD/wiki/Item-Editing
- https://github.com/luciensadi/AwakeMUD/wiki/Item-Extra-Flags
- https://github.com/luciensadi/AwakeMUD/wiki/NPC---Mobile-Editing
- https://github.com/luciensadi/AwakeMUD/wiki/Mob-Flags
- https://github.com/luciensadi/AwakeMUD/wiki/Shop-Editing
- https://github.com/luciensadi/AwakeMUD/wiki/Quest-Editing
Fetched: 2026-06-18

---

## Item editing (`iedit`)

Syntax: `iedit <vnum>` — edits a game item by its virtual number.

> Before creating an item, search existing GitHub code for similar items (especially
> cyberware) to prevent duplication and "item bloat."

### Main menu fields

| # | Field | Meaning |
|---|---|---|
| 1 | **Item keywords** | Aliases for referencing the item; should be verbose (e.g. `powerful black handgun ares predator iii 3`) |
| 2 | **Item name** | Inventory display name; starts lowercase, no ending punctuation |
| 3 | **Room description** | Text shown when dropped; a full sentence with capital + punctuation |
| 4 | **Look description** | Text displayed when examining the item |
| 5 | **Item type** | Classification (weapon, armor, drink, container, etc.) |
| 6 | **Item extra flags** | Behavioral modifiers (see Item Extra Flags below) |
| 7 | **Item affection flags** | Effects applied to the wearer/wielder (see `flags-reference.md`) |
| 8 | **Item wear flags** | Where the item can be equipped; removing the take flag prevents pickup |
| 9 | **Item weight** | Mass in kilograms, up to two decimal places |
| 10 | **Item cost** | Price in nuyen |
| 11 | **Item availability** | TN and delay settings, with Street Index calculation |
| 12 | **Item timer** | Usually left 0; code-controlled |
| 13 | **Item Material** | Used for breakage tests |
| 14 | **Item Barrier Rating** | Durability for breakage mechanics |
| 15 | **Item Legality** | Three-step legality configuration |
| 16 | **Item values** | Type-specific data, varies by item classification |
| 17 | **Item applies** | Bonuses/penalties on equip (requires head-immortal approval) |
| 18 | **Item extra descriptions** | Keyword-specific examine text, with optional color coding |

---

## Item extra flags

| Flag | Description |
|---|---|
| `GLOW` | Item has `^W(glowing)` appended to its short description. |
| `HUM` | Item has `^c(humming)` appended to its short description. |
| `!RENT` | Copies of this item in the world cannot be saved — lost on quit, won't persist in housing/storage. |
| `!DONATE` | Cannot be donated to the Chargen Salvation Army; junked instead. |
| `!INVIS` | Legacy flag with no effect. |
| `INVISIBLE` | Item is invisible as if affected by Improved Invisibility. If visible to the viewer, has `^B(invisible)` appended. |
| `MAGIC` | Has `^Y(magic aura)` appended to its short desc if the viewer is astral or perceiving. |
| `!DROP` | Prevents dropping the object: "You can't drop , it must be CURSED!" |
| `FORMFIT` | Armor value applied directly to ballistic/impact ratings without encumbrance effects; matched-set penalties apply if unqualified. |
| `!SELL` | Item cannot be sold to shopkeepers. |
| `CORPSE` | Container behaves like a corpse (restricts actions; may invoke auto-decay). |
| `GODONLY` | Usable only by immortals/staff. Mortal PCs cannot touch it. |
| `TWOHANDS` | Item must be wielded in both hands. |
| `COMPLEXBURST` | Not implemented; no effect. |
| `VOLATILE` | Set by code on objects not from connected zones. No effect. |
| `WIZLOAD` | Set by code on objects loaded by staff. Anti-cheat / tracking flag. |
| `NOTROLL` / `NOELF` / `NODWARF` / `NOORK` / `NOHUMAN` | Prevents the specified race from using the object. |
| `SNIPER` | Using this weapon at close range (same room) gives +6 to TNs. |
| `IMMLOAD` | Appears to replicate WIZLOAD; purpose unclear. |

---

## NPC / mobile editing (`medit`)

Syntax: `medit <vnum>`

### Main menu fields

| # | Field | Meaning |
|---|---|---|
| 1 | **Keywords** | Targeting aliases; include several relevant terms |
| 2 | **Name** | Display name; starts lowercase (unless a proper name), no ending punctuation |
| 3 | **Room Description** | Printed when viewing the room; starts capital, ends with punctuation |
| 4 | **Look Description** | Detailed text when examining the NPC directly |
| 5 | **Mob Flags** | Behavioral settings (see Mob Flags below) |
| 6 | **Affected Flags** | Special abilities / state modifiers (see `flags-reference.md`) |
| 8 | **Average Nuyen / Credstick Value** | Loose currency + credstick on death; 0 for none |
| 9 | **Bonus Karma Points** | Added to calculated karma awarded on kill |
| a | **Attributes** | Submenu for base attributes |
| b | **Level** | Prevents lower-level staff from forcing/manipulating the NPC |
| c–d | **Ballistic / Impact** | Inherent armor values without equipped gear |
| e–f | **Max Physical / Mental Points** | Health and mental maximums (default 10/10) |
| g–h | **Position / Default Position** | Spawn posture and displayed position; avoid Mortally Wounded/Stunned |
| i–k | **Gender / Weight / Height** | Physical characteristics |
| l | **Race** | Species; exotic races need justification |
| m | **Attack Type** | Unarmed combat style; avoid explosive types (splash damage) |
| n | **Skill Menu** | Up to six combat/magic skills |
| o–p | **Arrive / Leave Text** | Movement descriptions mimicking player movement text |

> **Skill note:** match weapons to skills — FIREARMS for projectiles, ARMED COMBAT for
> melee, BRAWLING for all NPCs.

---

## Mob flags

| Flag | Description |
|---|---|
| `!BLIND` | Legacy flag with no effect. |
| `!EXPLODE` | Explosive attacks do not damage this NPC. |
| `!KILL` | Attacks do not damage this NPC. |
| `!RAM` | It is not possible to ram this NPC. |
| `AGGR` | NPC attacks PCs and all vehicles in the room. |
| `AGGR_DWARF` | Targets Dwarf, Gnome, Menehune, Koborokuru PCs and vehicles. |
| `AGGR_ELF` | Targets Elf, Wakyambi, Night-One, Dryad PCs and vehicles. |
| `AGGR_HUMAN` | Attacks Human PCs and all vehicles in the room. |
| `AGGR_ORK` | Targets Ork, Hobgoblin, Ogre, Satyr, Oni PCs and vehicles. |
| `AGGR_TROLL` | Targets Troll, Cyclops, Fomori, Giant, Minotaur PCs and vehicles. |
| `ASTRAL` | The NPC is an astral form such as a spirit or elemental. |
| `AWARE` | Legacy flag with no effect. |
| `AZTECHNOLOGY` | Legacy flag with no effect. |
| `DUAL` | Dual-natured; has presence on the astral plane. |
| `FLAMEAURA` | Strengthens engulfing fire attacks; modifies room descriptions. |
| `GUARD` | Attacks gear violators, nearby shooters, and certain vehicles. |
| `HELPER` | Jumps to the aid of NPCs fighting characters. |
| `INANIMATE` | NPC is an inanimate object; used for vending machines. |
| `ISNPC` | Leave this set. Flags an NPC as being an NPC. |
| `MEMORY` | Remembers attackers and attacks them next time. |
| `PRIVATE` | Stops the NPC's stats from being investigated by Immortals below level 4. |
| `SCAVENGER` | Picks up the most expensive item it can carry every action round. |
| `SENTINEL` | Stands still and won't leave its post under normal conditions. |
| `SNIPER` | Performs duties at range with a sniper rifle; uses vision and weapon range. |
| `SPEC` | Has a special function hardcoded in `spec_assign.cpp`. |
| `SPIRITGUARD` | Set by code for spirits/elementals performing services. Do not set. |
| `SPIRITSORCERY` | Set by code for spirits/elementals performing services. Do not set. |
| `SPIRITSTUDY` | Set by code for spirits/elementals performing services. Do not set. |
| `STAY-ZONE` | Only moves to rooms within the zone of its current room. |
| `TOTALINVIS` | Completely invisible to mortals. |
| `TRACK` | Legacy flag with no effect. |
| `WIMPY` | Flees if its health gets too low. |

---

## Shop editing (`sedit`)

Syntax: `sedit <vnum>`

### Main menu fields

| # | Field | Meaning |
|---|---|---|
| 1 | **Keeper** | NPC vnum acting as shopkeeper. All NPCs with this vnum respond to shop commands; a unique NPC is recommended. |
| 2 | **Shop Type** | Unimplemented. Intended white-market (credsticks only) / grey-market (both) / black-market (cash only). |
| 3 | **Cost Multiplier (Buying)** | Markup when players buy. Must be ≥ 1.0, typically 1.1+. |
| 4 | **Cost Multiplier (Selling)** | Discount when players sell. Default 0.1; higher values need special authorization. |
| 5 | **Price Variance** | Percentage price fluctuation, typically capped at 10%. |
| 6 | **Opens / Closes** | Business hours (currently disabled — poor player experience). |
| 7 | **Etiquette for Availability** | Skill governing special-order persuasion rolls (default: Street Etiquette). |
| 8 | **Doesn't Trade With** | "Racism" menu restricting transactions with specific racial categories. |
| 9 | **Flags** | `Doctor`, `!NEGOTIATE` (fixed prices), `!RESELL` (no item restocking), `Grey` (TBD). |

### Submenus

| Key | Submenu | Purpose |
|---|---|---|
| a | **Buytype Menu** | Item categories the shop will purchase (optional) |
| b | **Text Menu** | Customize shopkeeper dialogue for various interactions |
| c | **Selling Menu** | Item vnums for sale (items need nonzero value to display) |

---

## Quest editing (`qedit`)

Syntax: `qedit <vnum>`

### Main menu fields

| # | Field | Meaning |
|---|---|---|
| 1 | **Quest Giver** | NPC vnum who distributes the quest (all instances hand it out) |
| 2 | **Time limit** | Minutes to complete |
| 3 | **Reputation range** | Min–max reputation for eligibility |
| 4 | **Nuyen bonus** | Flat nuyen payout |
| 5 | **Karma bonus** | Flat karma payout |
| 6 | **Item objective menu** | Tasks involving item manipulation |
| 7 | **Mobile objective menu** | Tasks involving NPC interactions |
| 8 | **Opening pitch** | Text when the player requests a job |
| 9 | **Decline response** | Text when the player refuses |
| a | **Quit message** | When the player abandons an accepted quest |
| b | **Completion acknowledgment** | On successful turn-in |
| c | **Quest description / recap** | Detailed quest text |
| d | **Johnson's availability hours** | Typically "Always" |
| e | **Start-giving message** | When the Johnson starts giving quests |
| f | **Stop-giving message** | When the Johnson stops giving quests |
| g | **Already-completed message** | When the player has already done this quest |
| h | **Item reward vnum** | The item given as reward |

All text fields support block-entry editing. Default values display as `(null)` or
numeric placeholders until customized.

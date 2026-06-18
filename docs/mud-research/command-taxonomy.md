# Cross-MUD Command Taxonomy

Source: Aardwolf (wiki.aardwolf.com), Achaea (achaea.com/game-help), BatMUD (bat.org/help)
Compiled: 2026-06-09

This document identifies the universal command surface that appears across MUDs,
useful for building a comprehensive command set for AnotherMUD.

---

## Universal Core Commands (appear in virtually every MUD)

### Movement
| Command | Usage | Description |
|---|---|---|
| n/s/e/w/ne/nw/se/sw/u/d | `n` | Move in cardinal direction |
| go | `go north` | Explicit movement verb |
| enter | `enter portal` | Enter a named object/portal |
| exits | `exits` | Show available exits from current room |
| look / l | `look`, `look sword`, `look north` | Examine room, object, or direction |
| recall | `recall` | Return to bind/safe point |
| run | `run nne2s` | Speedwalk via direction string |

### Information & Status
| Command | Usage | Description |
|---|---|---|
| score / sc | `score` | Display character stats and status |
| inventory / inv / i | `inv` | List carried items |
| equipment / eq | `eq` | List equipped items |
| affects / aff | `affects` | Show active buffs/debuffs |
| skills | `skills` | List known skills |
| consider / con | `consider goblin` | Gauge relative difficulty of target |
| who | `who` | List online players |
| where | `where` | Show location info / nearby players |
| time | `time` | Show in-game time/date |
| weather | `weather` | Show current weather |
| exits | `exits` | Show available exits |
| scan | `scan` | See mobs in adjacent rooms |

### Item Interaction
| Command | Usage | Description |
|---|---|---|
| get / take | `get sword`, `get all` | Pick up item(s) |
| drop | `drop sword` | Drop item |
| put | `put sword bag` | Put item into container |
| give | `give sword bob` | Give item to another player or mob |
| wear / wield | `wear helm`, `wield sword` | Equip item |
| remove | `remove helm` | Unequip item |
| examine / ex | `examine chest` | Detailed look at an object |
| open / close | `open door` | Open/close doors or containers |
| lock / unlock | `lock door` | Lock/unlock with a key |
| eat | `eat bread` | Consume food |
| drink | `drink flask` | Drink from a container |
| use | `use potion` | Use a consumable item |
| hold | `hold torch` | Hold an item in off-hand |

### Combat
| Command | Usage | Description |
|---|---|---|
| kill / k / attack | `kill goblin`, `k goblin` | Initiate combat |
| flee | `flee` | Attempt to escape combat |
| bash | `bash goblin` | Knockdown attack |
| kick | `kick goblin` | Kick attack |
| cast | `cast 'fireball' goblin` | Cast a spell |
| backstab / bs | `backstab goblin` | Thief sneak attack |
| disarm | `disarm goblin` | Attempt to disarm opponent |

### Communication
| Command | Usage | Description |
|---|---|---|
| say / ' | `say hello`, `'hello` | Speak to room |
| tell / t | `tell bob hello` | Private message to player |
| shout | `shout help!` | Heard across a wide area |
| yell | `yell help!` | Heard in adjacent rooms |
| emote / : | `emote waves`, `:waves` | Perform a custom action |
| whisper | `whisper bob hello` | Quiet tell (room-local) |
| reply / r | `reply thanks` | Reply to last tell |

### Social & Group
| Command | Usage | Description |
|---|---|---|
| socials | `socials` | List available social commands |
| wave / smile / bow | `wave`, `smile bob` | Pre-defined social emotes |
| group | `group` | Show group status |
| gtell / gt | `gt let's go north` | Talk to group only |
| follow | `follow bob` | Follow another player |
| nofollow | `nofollow` | Stop others from following you |

### Utility & Config
| Command | Usage | Description |
|---|---|---|
| help / h | `help combat`, `help 2.4` | Look up help on a topic |
| alias | `alias bs backstab` | Create a command shortcut |
| config | `config` | View/change settings |
| quit | `quit` | Leave the game |
| save | `save` | Save character (many MUDs auto-save) |
| title | `title the Brave` | Set your character title |
| description | `description I am tall...` | Set your character description |
| password | `password` | Change password |
| prompt | `prompt %h/%H hp` | Customize your status prompt |
| ignore | `ignore bob` | Ignore a player's communications |
| afk | `afk` | Toggle away-from-keyboard status |
| brief | `brief` | Toggle short room descriptions |
| color | `color on/off` | Toggle ANSI color |
| cls / clear | `cls` | Clear screen |
| commands | `commands` | List available commands |

---

## Extended Commands (appear in many MUDs, not all)

### Economy
| Command | Usage | Description |
|---|---|---|
| buy | `buy sword` | Buy from a shop |
| sell | `sell sword` | Sell to a shop |
| list | `list` | See shop inventory |
| value | `value sword` | Check sell value of item |
| gold / coins | `gold` | Check how much money you have |
| deposit | `deposit 1000` | Deposit gold at a bank |
| withdraw | `withdraw 1000` | Withdraw gold from bank |
| balance | `balance` | Check bank balance |
| auction | `auction sword 1000` | Put item on auction for starting bid |
| bid | `bid 1500` | Bid on an auctioned item |

### Navigation & Mapping
| Command | Usage | Description |
|---|---|---|
| map | `map` | Display an ASCII map of surroundings |
| areas | `areas` | List available areas |
| hunt | `hunt goblin` | Track a target to its room |
| goto (admin) | `goto bob` | Teleport to a player (admin) |
| summon | `summon bob` | Summon a player to you |
| gate | `gate bob` | Teleport to a player (spell) |
| portal | `portal nexus` | Create a portal to a location |

### Character Advancement
| Command | Usage | Description |
|---|---|---|
| train | `train str` | Increase a stat with train points |
| practice / prac | `practice kick` | Practice a skill to improve it |
| learn | `learn kick from master` | Learn a skill from a trainer |
| level | `level` | Gain a level (some MUDs manual) |
| gain | `gain` | Gain levels/skills at guild |
| spells | `spells` | List known spells |
| abilities | `abilities` | List special abilities |

### Questing
| Command | Usage | Description |
|---|---|---|
| quest | `quest` | Request a quest from questmaster |
| quests | `quests` | List active quests |
| quest info | `quest info` | Show current quest details |
| quest complete | `quest complete` | Turn in a completed quest |
| campaign | `campaign` | Request a campaign |

### Groups & Parties
| Command | Usage | Description |
|---|---|---|
| group | `group` | View group status |
| group add | `group add bob` | Invite someone to group |
| group kick | `group kick bob` | Remove someone from group |
| group leader | `group leader bob` | Transfer group leadership |
| group loot | `group loot round` | Set loot distribution mode |
| split | `split 1000` | Split gold among group members |

### Crafting (where implemented)
| Command | Usage | Description |
|---|---|---|
| craft | `craft sword` | Craft an item from recipe |
| combine | `combine iron ore` | Combine ingredients |
| recipe | `recipe list` | List known recipes |
| forge | `forge sword` | Use a forge station |
| brew | `brew potion` | Brew a potion |
| cook | `cook meat` | Cook food |

### Housing (where implemented)
| Command | Usage | Description |
|---|---|---|
| build | `build north` | Add a room in a direction |
| describe | `describe room ...` | Set room description |
| home | `home` | Teleport to your home |
| invite | `invite bob` | Let someone into your home |
| evict | `evict bob` | Remove someone from your home |
| ring | `ring` | Ring doorbell of a manor |
| furnish | `furnish table here` | Place furniture in a room |

---

## MUD-Specific Command Innovations Worth Noting

### Aardwolf
- `RUNTO <area>` — navigate automatically to a known area
- `FIND <target>` — pathfind within current area
- `SPEEDWALKS` — built-in speedwalk with pause/resume
- `WAYFIND` — see portal destinations without entering
- `DTRACK` — live damage tracker display
- `SPELLUP` — one-command buff sequence
- `ALLSPELLS <class>` — preview all abilities for a class before choosing it
- `TOPRANK` — leaderboard for various metrics
- `EXPLORED` — percentage of world you have visited
- `WANGRP` — announce you want a group (dedicate channel)
- `REPLAY` — replay recent tells you may have missed
- `LASTKILLS` — see what mobs you killed recently

### Achaea
- `LANDMARKS <name>` — auto-path to a named landmark
- `REFLEXES` — in-game alias/keybind system more powerful than simple aliases
- `SLIT WRIST` — draw blood for use in rituals (context-specific)
- `IDENTIFIERS` — flexible item shortname system (2.sword, etc.)

### BatMUD
- `PARTY LINK` — party linking for raid groups
- Full outerworld navigation system

---

## Command Taxonomy for AnotherMUD Reference

When auditing AnotherMUD's existing command set, these are the categories to check:

1. **Movement** — directions, go, enter, exits, recall, speedwalk/run
2. **Information** — look, score, inv, eq, affects, skills, consider, who, where
3. **Item interaction** — get/drop/put/give, wear/wield/remove, open/close/lock, eat/drink/use
4. **Combat** — kill/attack, flee, combat skills, flee direction
5. **Communication** — say/tell/shout/yell/emote, channels, reply, whisper, ignore, afk
6. **Social** — socials library, group communication (gtell), follow
7. **Economy** — buy/sell/list/value, gold, bank, auction/bid
8. **Navigation extras** — map, areas, hunt/track, portal/gate/summon
9. **Advancement** — train, practice/learn, level, skills/spells/abilities
10. **Questing** — quest, campaign, quest info/complete
11. **Grouping** — group management, loot modes, split
12. **Crafting** — craft/combine/recipe, station-specific (forge/brew/cook)
13. **Housing** — build, describe, home, invite/evict, furnish
14. **Config/utility** — alias, config, help, quit, title, description, prompt, password, brief, color

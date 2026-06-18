# Aardwolf — Warfare, Manors, Remort, Channels

Source: multiple pages on aardwolf.com
Fetched: 2026-06-09

---

## Warfare

Source: https://www.aardwolf.com/mud/warfare.html

A feature in which players fight each other in a special warzone area. No death penalties
during war — slain players are returned to Midgaard with no XP loss, no equipment loss, and
restored HP/mana.

### War Types

| Type | Description |
|------|-------------|
| Genocide War | Free for all — last player alive wins |
| Class War | Grouped by class — last class standing wins |
| Race War | Grouped by race — last race standing wins |
| Clan War | Grouped by clan — last clan standing wins |

### Rules

- Kill or be killed
- Grouping with any level is allowed during a war
- Using healing potions/pills during combat in the warzone = cheating
- Eating/using healing between fights (not in active combat) is allowed
- War records stored per player, viewable via `WARINFO`
- `WARSTATUS` shows current status at any time
- `WARSITUATION` shows live fight progress and % health during an active war

---

## Manors (Player Housing)

Source: https://www.aardwolf.com/mud/manor.html

Players may purchase private homes called Manors.

### Cost

- 40 trivia points + 5 million gold (standard)
- OR 50 trivia points (no gold required, for lower-level players)

### Features

- `HOME` command — functions like recall but returns you to your manor
- Married players can share costs and benefits (one player is registered owner)
- **Two-room minimum**: main room + a waiting/anteroom
  - Owner can `INVITE {player}` from waiting room into main room
  - Owner can `LOOK OUTSIDE` to see who is in the waiting room
  - Visitor in waiting room can `RING BELL` to alert the owner
- Built-in fireplace and fountain
- Room is flagged as safe (no combat)
- Can be sold to other players (including transfer of Quest and Trivia points)
- Can purchase pet mobs and objects for the manor

---

## Remorting

Source: https://www.aardwolf.com/mud/remort.html

When a character reaches SUPERHERO level, they can choose to remort — reborn into another
class, restarting at level 1 but carrying forward skills/spells.

### Mechanics

- 7 classes available → player can remort up to 7 times
- After 7 classes: TIER option available for further progression
- Skills/spells kept but reset to 1%; re-practiced at correct levels
- Gold, trivia points, quest points are kept
- Quest weapons can be sold back to Questor; other quest items sold via remort auction
- XP per level increases each remort: 2k (2nd time), 3k (3rd), etc.
- Best benefit of a skill/spell shared if multiple classes have it
  - e.g. warrior remorti into mage gets second extra attack from haste

---

## Dynamic Channels

Source: https://www.aardwolf.com/mud/channels.html

Aardwolf supports typical channel styles plus some unique forms.

### Channel Types

- **Race channel** — talk to members of your race
- **Class channel** — talk to members of your class
- **Clan channel** — talk to clan members
- **Level-range channel** — talk to players near your level
- **Friends list channel** — private channel to a specific list of chosen friends
  (unique feature: reduces global channel traffic because people use targeted channels)

### Design Note

The friends channel system is cited as a reason why global channels are not as busy on
Aardwolf as on other muds despite the large player count.

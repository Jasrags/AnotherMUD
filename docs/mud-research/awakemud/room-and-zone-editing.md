# AwakeMUD — Room & Zone Editing

Source:
- https://github.com/luciensadi/AwakeMUD/wiki/Room-Editing
- https://github.com/luciensadi/AwakeMUD/wiki/Room-Flags-(redit)
- https://github.com/luciensadi/AwakeMUD/wiki/Zone-Editing
- https://github.com/luciensadi/AwakeMUD/wiki/All-About-Zone-Commands
- https://github.com/luciensadi/AwakeMUD/wiki/ZRESET
- https://github.com/luciensadi/AwakeMUD/wiki/Elevators
Fetched: 2026-06-18

---

## Room editing (`redit`)

Two forms:
- `redit` — edits your current room.
- `redit <room_number>` — edits a specific room by vnum (creates it if it doesn't exist).

### Main menu fields

| # | Field | Meaning |
|---|---|---|
| 1 | **Room Name** | Title in sentence case (e.g. "A room with a view") |
| 2 | **Room Desc** | Primary `LOOK` description (recommended 3–6 lines); overridden by night desc when present |
| 3 | **Night Desc** | Optional alternative description active at night |
| 4 | **Room Flags** | Toggleable behavioral flags (see table below) |
| 5 | **Domain Type** | Associated Shamanic domain for the room |
| 6–f | **Exits** | Connected room vnums; selectable to open the exit submenu |
| g | **Edit Jackpoint** | Matrix connectivity (see below) |
| h | **Light Level** | Ambient illumination setting |
| i | **Smoke Level** | Visibility obstruction; affects vision and TNs |
| j | **Combat Options** | Crowd / Cover / Room Type / X,Y,Z dimensions (see below) |
| k | **Background Count** | Modifies spellcasting TNs (ref: Magic in the Shadows, p. 83) |
| l | **Extra Descriptions** | Hidden keywords giving extra detail when examined |
| m | **Current / Fall Rating** | Swimming/falling difficulty for water/FALL-flagged rooms |
| n | **Staff Level Required** | Access restriction (mainly administrative areas) |

### Exit submenu

| # | Field | Meaning |
|---|---|---|
| 1 | **Exit To** | Target vnum (a nonexistent vnum leads to "A Bright Light" at vnum 0) |
| 2 | **Description** | Text shown when looking that direction |
| 3 | **Door Names** | Exit aliases; first name appears in interaction messages |
| 4 | **Key vnum** | Item vnum that serves as the key (0 = no lock; 998 = bypassable, no player key) |
| 5 | **Door Flags** | Standard door, Pickproof, Warded, Glass Window, Barred Window, No Shooting Through, Strict Key Requirement |
| 6 | **Lock Level** | Target number required to bypass the door |
| 7 | **Material Type** | Door material for breakage calculations |
| 8 | **Barrier Rating** | Used in breakage tests |
| 9 | **Hidden Rating** | TN to notice the exit (0 = not hidden) |
| a–c | **Custom Messages** | 2nd-person leaving, 3rd-person leaving, and entering messages (support variable substitution) |
| x | **Purge Exit** | Destroys one side of the exit |

### Jackpoint (Matrix connectivity) — field `g`

- **To Host** (vnum)
- **I/O Speed** (0 = unlimited; -1 = capped at MPCP × 50)
- **Commlink Number** (caller-ID display)
- **Physical Address** (location string)

### Combat options — field `j`

- **Crowd** — affects Puma shaman TNs if > 4
- **Cover** — display only
- **Room Type** — display only
- **X, Y, Z dimensions** — Z affects slouch penalties

### Creating exits

Use the `dig` command: `dig <direction> <vnum>` automatically creates the reciprocal
connection. Doors require zone commands to start closed.

---

## Room flags (`redit`)

| Flag | Description |
|---|---|
| `!BIKE` | Blocks travel of motorbikes through the room. |
| `!DROP` | Prevents dropping anything here. Don't set unless Lucien/Vile tells you to. |
| `!GRID` | Blocks gridguide from, to, and through this room. Can separate locations into distinct grid networks. |
| `!MAGIC` | Prevents casting spells in this room. **Legacy** — should no longer be set; use higher background counts instead. |
| `!MOB` | Blocks NPCs from entering of their own volition. NPCs can still be dragged or baited by gunfire. |
| `!QUIT` | Blocks players from quitting in the room. Requires express permission from the game owner. |
| `!RADIO` | Makes radio transmissions spotty/hard to understand. Typically for underground spaces. |
| `!TRAFFIC` | Blocks display of traffic environmental messages. Works with the ROAD flag; useful for driveways. |
| `!WALK` | Marks the room as a highway, preventing on-foot players from walking through it. |
| `AIRCRAFT_CRASH_OK` | Aircraft can crash here. Restrict to contiguous sprawl areas (Seattle, Tacoma, etc.). |
| `AIRCRAFT_ROAD` | Aircraft can taxi through here. |
| `ALL_VEH_ACCESS` | ANY vehicle can enter this room. Used for marinas where vehicles access the water's edge. |
| `ARENA` | Allows players to fight here regardless of PK status. |
| `FALL` | Players fall through the down exit. The `m` field sets the athletics TN. |
| `GARAGE` | Allows storage of vehicles over crash/copyover. Implicitly sets `!GRID` and `ROAD`. |
| `HELIPAD` | Place that helicopters and blimps can fly to. Requires explicit permission. |
| `INDOORS` | Blocks traffic and weather effects in the room. |
| `PEACEFUL` | Prevents aggression within the room and prevents attacks being directed into it. |
| `RADIOACTIVE` | Radiation damage (not yet implemented). Don't set. |
| `ROAD` | Allows cars and trucks to travel through the room. |
| `RUNWAY` | Allows planes to fly here. Requires permission to set. |
| `SMALL_DRONE_ONLY` | Characters can't enter, only small drones. Planned for future drone-manipulation features. |
| `SOCIALIZE!` | Makes a social room, displaying characters on the WHERE list. Requires public accessibility and a location in the room name. |
| `SOUNDPROOF` | Entirely blocks shouting and radio usage; also muffles gunshots. Use sparingly. |
| `STAFF_ONLY` | Prevents non-staff characters from entering. |
| `STERILE` | Enhances medical effects and makes surgery easier. Restrict to plausibly sterile surgical clinics. |
| `STORAGE` | Items dropped in the room save over MUD interruptions. Requires permission to set. |
| `STREETLIGHTS` | Streetlights turn on at night, preventing the room from going dark. |
| `TUNNEL` | Only two characters/NPCs can be in the room at a time. Pair with `!MOB` if needed. |

---

## Zone editing (`zedit`)

Core commands:
- `ZSWITCH <zone number>` — switch to a zone for editing.
- `SHOW ZONES` — list all existing zones.
- `ZEDIT` — edit the current zone (creates or modifies).

### ZEDIT menu fields

| # | Field | Meaning |
|---|---|---|
| 1 | **Name** | Internal zone identifier (not visible to players) |
| 2 | **Top of zone** | Highest vnum the zone covers. Zones start at `zone number × 100` (zone 74 → 7400–7699) |
| 3 | **Lifespan** | Reset interval in real-world minutes |
| 4 | **Reset mode** | Don't reset / Always reset / Reset only if no PCs are in the zone (idle players don't block) |
| 5 | **Security level (1–15)** | Guard sensitivity; higher = more vigilant |
| 6 | **Jurisdiction** | Where corpses and vehicles go on expiration (affects Docwagon and parking locations) |
| 7 | **Editors** | Five ID numbers separated by spaces (use 0 for blanks), e.g. `123 88 0 0 0` |
| 8 | **Connected** | Whether the zone connects to the game world (connected zones require higher edit privileges) |
| 9 | **Is PGHQ** | Toggles player-group-headquarters status |
| L | **Secret Squirrel** | Locks the zone from non-editor visibility (for unreleased content) |

---

## Zone commands (resets)

Zone commands drive resets: NPC placement, door states, vehicle/item spawning on a
schedule. Access via `zlist`; add a new one with `zedit add`, edit existing with
`zedit <index>` (after `zswitch` to the target zone).

### Command types

| Code | Type | Function |
|---|---|---|
| 1 | MOB | Load an NPC at a location |
| 2 | OBJECT | Load an object at a location |
| 3 | PUT | Place object inside the previously loaded object |
| 4 | EQUIP | Equip object to the previously loaded NPC |
| 5 | GIVE | Add object to the previous NPC's inventory |
| 6 | REMOVE | Remove object from a room |
| 7 | DOOR | Set door state (open/closed/locked) |
| 8 | GIVE NUMBER | GIVE with quantity specification |
| 9 | CYBER/BIOWARE | Install cyberware on the previous NPC |
| 0 | VEHICLE | Load vehicle at location |
| A | DRIVER/PASSENGER | Load NPC into the previous vehicle |
| B | UPGRADE | Install upgrade into the previous vehicle |
| C | CARRIED | Place object in vehicle cargo |
| D | HOST | Load object into a Matrix host |
| n | NOTHING | No-op; deleted at boot |

### Conditionals (execution modes)

- **ALWAYS** — executes every zone reset.
- **IF LAST** — executes only if the previous command succeeded (avoid pairing with count flags).

### Quantity / spawn-count modes

1. **Global Count** (positive, e.g. 1–5) — counts instances across the *entire game*; refuses if the threshold is already met.
2. **Room Count** (negative formula) — counts within the target room only; use `-1 minus desired quantity` (e.g. `-2` for a count of 1).
3. **Load Once** (0) — spawns once at startup; never respawns until reboot.
4. **Unlimited** (-1) — respawns every reset; use sparingly.

---

## ZRESET — force a reset

Syntax: `zreset <zone | * | .>`

Forces an immediate zone reset:
- a specific zone number resets that zone,
- `*` resets all zones simultaneously,
- `.` resets only the zone where the issuer is currently located.

Example: `zreset 355` (resets zone 17, "Dante's Inferno").

---

## Elevators

Elevators are processed at startup from `lib/etc/elevator`. The file begins with a
number giving the total count of elevator blocks.

### Elevator (header) line

Four numbers:
1. The elevator **car room vnum**.
2. The number of **columns** on the elevator button panel (described as "probably").
3. The number of **floors**.
4. The **lowest** elevator floor available.

Floor numbering starts at the lowest value and increments up (`0` = ground, `-1` =
basement, positive = upper floors).

### Floor lines

Listed from **highest to lowest** floor. Each floor needs three values:

1. **Landing room vnum** — the destination the elevator opens into.
2. **Shaft vnum** — corresponds to this floor level.
3. **Direction code** — which way the door opens (0–7, clockwise from north).

Orient the elevator with the highest floor at the top and the lowest at the bottom.

### Direction codes

```
NORTH = 0      NORTHEAST = 1    EAST = 2     SOUTHEAST = 3
SOUTH = 4      SOUTHWEST = 5    WEST = 6     NORTHWEST = 7
```

### Example

A 2-floor elevator (car vnum 60531), 1 button column, lowest floor 4:
- Floor 5: landing 60530, shaft 60621, south exit
- Floor 4: landing 60532, shaft 60622, south exit

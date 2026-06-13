# Mudlet client — GMCP mapper

AnotherMud emits map data over the GMCP `Room.Info` package
([room-coordinates](../../docs/specs/room-coordinates.md) §5,
[player-maps](../../docs/specs/player-maps.md) §7): each room's id, name,
area, exits, terrain, and a stable area-local `x`/`y`/`z` coordinate.

Mudlet has **no universal built-in GMCP auto-mapper** — a generic mapper
script doesn't know AnotherMud's schema and will lay rooms out by exit
direction (a stacked, non-geographic map). `AnotherMud-Mapper.lua` teaches
Mudlet our exact `Room.Info` shape so it places each room at its true
coordinate.

## Install

1. **Settings (gear icon) → Scripts → Add Item.**
2. Name it `AnotherMud Mapper`, paste the contents of
   [`AnotherMud-Mapper.lua`](./AnotherMud-Mapper.lua), click **Save Item**.
3. **Disable any other generic GMCP mapper script** — two mappers fighting
   over `Room.Info` produce a garbled map.
4. Reset any old/bad map once — paste into Mudlet's command line:
   ```
   lua local rs=getRooms() or {}; for id in pairs(rs) do deleteRoom(id) end; updateMap()
   ```
5. Walk around. Each room you enter is placed at its coordinate; exits to
   places you haven't visited show as **stubs** (fog of war). The server
   sends `Room.Info` only for your current room, so the map fills in as you
   explore — no area is bulk-revealed.

## How it works

- Room ids are namespaced strings (`tapestry-core:town-square`); the script
  maps each onto a Mudlet integer room id via Mudlet's room-hash table
  (`setRoomIDbyHash`/`getRoomIDbyHash`).
- `x`/`y`/`z` → `setRoomCoordinates`; `area` → a Mudlet area;
  `terrain` → a Mudlet environment color.
- An exit to an already-mapped room becomes a real connection; an exit to an
  unvisited room becomes a stub, so you can see a passage exists without
  seeing what's beyond it.

The server's ASCII surfaces (`minimap` toggle, `map` verb) need no client
help and work on any terminal; this script is the richer Mudlet path on top
of the same coordinate data.

-- AnotherMud — Mudlet GMCP mapper
-- =================================
-- Builds a coordinate-accurate map from the server's Room.Info GMCP
-- frames (see docs/specs/room-coordinates.md §5 + docs/specs/player-maps.md
-- §7). Mudlet ships no universal GMCP mapper; this script teaches Mudlet
-- AnotherMud's exact Room.Info schema so it positions rooms by the
-- server's stable area-local (x,y,z) instead of guessing from exits.
--
-- INSTALL
--   1. Mudlet > Settings (gear) > Scripts > "Add Item".
--   2. Name it "AnotherMud Mapper", paste this whole file into the editor,
--      click "Save Item".
--   3. DISABLE any other generic GMCP mapper script (it will fight this one
--      and produce a stacked/garbled map).
--   4. Reset the old bad map once (paste into the command line):
--        lua local rs=getRooms() or {}; for id in pairs(rs) do deleteRoom(id) end; updateMap()
--   5. Walk around. Each room you enter is placed at its true coordinate;
--      exits to places you haven't been show as stubs (fog of war).
--
-- The server emits Room.Info only for your CURRENT room as you move, so the
-- map fills in exactly as you explore — no area is bulk-revealed.

AnotherMud = AnotherMud or {}

-- Server exit codes (Direction.Short) -> Mudlet direction names.
local DIRS = {
  n = "north", s = "south", e = "east", w = "west",
  u = "up", d = "down",
  ne = "northeast", nw = "northwest", se = "southeast", sw = "southwest",
}

-- terrain -> Mudlet environment color id (>=16 so engine defaults are safe).
-- Covers the full starter-world terrain vocabulary; extend as new packs add
-- terrain classifiers.
local ENV = {
  outdoors = 200, indoors = 201, underground = 202, water = 203,
  road = 204, forest = 205, mountain = 206, cave = 207, grassland = 208,
}

-- Fallback: a room with no recognized terrain is coloured by its light level
-- (light-and-darkness §8) so nothing draws in Mudlet's flat default. Any room
-- with a known terrain uses ENV above and never reaches this.
local LIGHT_ENV = { lit = 210, dim = 211, gloom = 212, black = 213 }

local function ensureEnvColors()
  if AnotherMud._envReady then return end
  setCustomEnvColor(200, 120, 160, 90, 255)  -- outdoors
  setCustomEnvColor(201, 165, 140, 95, 255)  -- indoors
  setCustomEnvColor(202, 95, 85, 75, 255)    -- underground
  setCustomEnvColor(203, 70, 120, 200, 255)  -- water
  setCustomEnvColor(204, 155, 145, 120, 255) -- road
  setCustomEnvColor(205, 45, 120, 65, 255)   -- forest
  setCustomEnvColor(206, 115, 105, 100, 255) -- mountain
  setCustomEnvColor(207, 80, 72, 66, 255)    -- cave
  setCustomEnvColor(208, 120, 175, 95, 255)  -- grassland
  setCustomEnvColor(210, 205, 200, 175, 255) -- light fallback: lit
  setCustomEnvColor(211, 150, 145, 130, 255) -- light fallback: dim
  setCustomEnvColor(212, 100, 100, 110, 255) -- light fallback: gloom
  setCustomEnvColor(213, 60, 60, 72, 255)    -- light fallback: black
  AnotherMud._envReady = true
end

-- Map an AnotherMud namespaced room id (a string) onto a Mudlet integer
-- room id via Mudlet's room-hash table. Creates the room on first sight.
local function roomFor(hash)
  local id = getRoomIDbyHash(hash)
  if id == -1 then
    id = createRoomID()
    addRoom(id)
    setRoomIDbyHash(id, hash)
  end
  return id
end

local function areaFor(name)
  name = name or "Unknown"
  local areas = getAreaTable() or {}
  return areas[name] or addAreaName(name)
end

function AnotherMud.onRoomInfo()
  local info = gmcp and gmcp.Room and gmcp.Room.Info
  if not info or not info.num then return end
  ensureEnvColors()

  local id = roomFor(info.num)
  setRoomName(id, info.name or info.num)
  setRoomArea(id, areaFor(info.area))

  -- Stable area-local coordinate from the server (omitted for unplaced
  -- rooms, which Mudlet then lays out on its own).
  if info.x ~= nil and info.y ~= nil and info.z ~= nil then
    setRoomCoordinates(id, info.x, info.y, info.z)
  end

  -- Colour by terrain when the room carries one; otherwise fall back to the
  -- room's light level so town streets and other terrain-less rooms still
  -- render with a distinct colour instead of Mudlet's flat default.
  local env = ENV[string.lower(info.terrain or "")]
    or LIGHT_ENV[string.lower(info.light or "")]
  if env then setRoomEnv(id, env) end

  -- Exits: link to a room we've already mapped, else leave a stub pointing
  -- into the unexplored dark (fog of war — player-maps §6.4).
  if info.exits then
    for code, target in pairs(info.exits) do
      local dir = DIRS[code] or code
      local neighbour = getRoomIDbyHash(target)
      if neighbour ~= -1 then
        setExit(id, neighbour, dir)
      else
        setExitStub(id, dir, true)
      end
    end
  end

  centerview(id)
  updateMap()
end

registerAnonymousEventHandler("gmcp.Room.Info", "AnotherMud.onRoomInfo")
echo("[AnotherMud] mapper script loaded — walk to build the map.\n")

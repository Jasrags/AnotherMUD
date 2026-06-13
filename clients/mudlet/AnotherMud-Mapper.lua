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
local ENV = {
  outdoors = 200, indoors = 201, underground = 202, water = 203,
  road = 204, forest = 205, mountain = 206,
}

local function ensureEnvColors()
  if AnotherMud._envReady then return end
  setCustomEnvColor(200, 120, 160, 90, 255)  -- outdoors
  setCustomEnvColor(201, 165, 140, 95, 255)  -- indoors
  setCustomEnvColor(202, 95, 85, 75, 255)    -- underground
  setCustomEnvColor(203, 70, 120, 200, 255)  -- water
  setCustomEnvColor(204, 155, 145, 120, 255) -- road
  setCustomEnvColor(205, 45, 120, 65, 255)   -- forest
  setCustomEnvColor(206, 115, 105, 100, 255) -- mountain
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

  local env = ENV[string.lower(info.terrain or "")]
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

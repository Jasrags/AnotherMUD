package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// newRoomGmcpActor returns a connActor parked in `room` with a
// GMCP-aware fake conn. The Vitals/Sustenance plumbing is
// inherited from the gmcpFakeConn helpers in gmcp_vitals_test.go.
func newRoomGmcpActor(playerID string, room *world.Room) (*connActor, *gmcpFakeConn) {
	fc := &gmcpFakeConn{fakeConn: fakeConn{id: "test-" + playerID}}
	a := &connActor{
		id:       fc.id,
		conn:     fc,
		playerID: playerID,
		room:     room,
		vitals:   combat.NewVitalsAt(50, 100),
		save:     &player.Save{ID: playerID, Name: playerID, Sustenance: 100},
	}
	a.sustenance = 100
	return a, fc
}

func TestBuildRoomInfoPayload_FlattensExits(t *testing.T) {
	// Exits map keys map through Direction.Short() (n/s/e/w/u/d).
	room := &world.Room{
		ID:     "town:square",
		AreaID: "town",
		Name:   "Town Square",
		Exits: map[world.Direction]world.Exit{
			world.DirNorth: {Target: "town:forge"},
			world.DirEast:  {Target: "town:market"},
		},
		Description: "A worn cobblestone plaza.",
		Terrain:     "outdoors",
	}
	got := buildRoomInfoPayload(room)

	if got.Num != "town:square" {
		t.Errorf("Num = %q", got.Num)
	}
	if got.Name != "Town Square" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Area != "town" {
		t.Errorf("Area = %q", got.Area)
	}
	if got.Terrain != "outdoors" {
		t.Errorf("Terrain = %q", got.Terrain)
	}
	if got.Exits["n"] != "town:forge" {
		t.Errorf("Exits[n] = %q, want town:forge", got.Exits["n"])
	}
	if got.Exits["e"] != "town:market" {
		t.Errorf("Exits[e] = %q, want town:market", got.Exits["e"])
	}
	if len(got.Exits) != 2 {
		t.Errorf("exits map has %d entries, want 2: %+v", len(got.Exits), got.Exits)
	}
}

func TestBuildRoomInfoPayload_StripsColorMarkup(t *testing.T) {
	// Room names/descriptions carry brace-shorthand colour markup (e.g.
	// "{Y}Hearthwick Forge{x}"). GMCP is structured data — a graphical mapper
	// renders Name as its room label, so the raw markup must be stripped or it
	// shows up literally in the client. See clients/mudlet.
	room := &world.Room{
		ID:     "town:forge",
		AreaID: "town",
		Name:   "{Y}Hearthwick Forge{x}",
		// A stray '<' ("blade <2ft") is content, not markup — it must survive
		// while brace + well-formed angle markup are stripped (gmcpPlain uses the
		// lenient tag strip so the description isn't truncated at the '<').
		Description: "A {r}glowing{x} forge and <b>anvil</b>, blade <2ft.",
		Exits:       map[world.Direction]world.Exit{},
	}
	got := buildRoomInfoPayload(room)

	if got.Name != "Hearthwick Forge" {
		t.Errorf("Name = %q, want %q (brace markup must be stripped)", got.Name, "Hearthwick Forge")
	}
	if got.Details != "A glowing forge and anvil, blade <2ft." {
		t.Errorf("Details = %q, want %q (brace + angle markup stripped, stray < kept)", got.Details, "A glowing forge and anvil, blade <2ft.")
	}
}

func TestBuildRoomInfoPayload_FlattensKeywordExits(t *testing.T) {
	// Portal-style keyword exits land in the Keywords map.
	room := &world.Room{
		ID:           "town:square",
		Name:         "Town Square",
		Exits:        map[world.Direction]world.Exit{},
		KeywordExits: map[string]world.Exit{"portal": {Target: "tower:top"}},
	}
	got := buildRoomInfoPayload(room)
	if got.Keywords["portal"] != "tower:top" {
		t.Errorf("Keywords[portal] = %q, want tower:top", got.Keywords["portal"])
	}
}

// A placed room carries its area-local coordinate on Room.Info, copied
// into fresh ints (player-maps §7 / room-coordinates §5). NOTE: the flat
// x/y/z layout is a PLACEHOLDER pending validation against a live Mudlet
// mapper (PD-9) — see the room-coordinates-gmcp-wireshape memory.
func TestBuildRoomInfoPayload_CarriesCoordinatesWhenPlaced(t *testing.T) {
	room := &world.Room{
		ID: "town:square", AreaID: "town", Name: "Town Square",
		Exits: map[world.Direction]world.Exit{},
		Coord: &world.Coord{X: 3, Y: -2, Z: 1},
	}
	got := buildRoomInfoPayload(room)
	if got.X == nil || got.Y == nil || got.Z == nil {
		t.Fatalf("coordinate fields nil for a placed room: %+v", got)
	}
	if *got.X != 3 || *got.Y != -2 || *got.Z != 1 {
		t.Errorf("coord = (%d,%d,%d), want (3,-2,1)", *got.X, *got.Y, *got.Z)
	}
	// Payload ints are fresh, not aliasing the shared Room.Coord.
	room.Coord.X = 99
	if *got.X != 3 {
		t.Error("payload coordinate aliases the shared Room.Coord")
	}
}

// An unplaced room omits the coordinate ENTIRELY (not x:0) so a mapper
// falls back to its own relative placement (room-coordinates §5.1).
func TestBuildRoomInfoPayload_OmitsCoordinatesWhenUnplaced(t *testing.T) {
	room := &world.Room{ID: "town:void", AreaID: "town", Name: "Void", Exits: map[world.Direction]world.Exit{}}
	got := buildRoomInfoPayload(room)
	if got.X != nil || got.Y != nil || got.Z != nil {
		t.Errorf("unplaced room should leave coordinate fields nil, got %+v", got)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"x"`, `"y"`, `"z"`} {
		if strings.Contains(string(data), key) {
			t.Errorf("unplaced payload must omit %s, got: %s", key, data)
		}
	}
}

func TestSendGmcpRoomInfo_NoSendBeforeActivation(t *testing.T) {
	room := &world.Room{ID: "x", Name: "X", Exits: map[world.Direction]world.Exit{}}
	a, fc := newRoomGmcpActor("p-1", room)
	a.sendGmcpRoomInfo(context.Background(), room)
	if len(fc.framesSnapshot()) != 0 {
		t.Errorf("pre-activation send emitted frames")
	}
}

func TestSendGmcpRoomInfo_EmitsAfterActivation(t *testing.T) {
	room := &world.Room{
		ID:     "town:square",
		AreaID: "town",
		Name:   "Town Square",
		Exits:  map[world.Direction]world.Exit{world.DirNorth: {Target: "town:forge"}},
	}
	a, fc := newRoomGmcpActor("p-1", room)
	fc.setActive(true)

	a.sendGmcpRoomInfo(context.Background(), room)

	frames := fc.framesSnapshot()
	if len(frames) != 1 {
		t.Fatalf("active send emitted %d frames, want 1", len(frames))
	}
	if frames[0].pkg != gmcp.PackageRoomInfo {
		t.Errorf("pkg = %q, want %q", frames[0].pkg, gmcp.PackageRoomInfo)
	}
	var got gmcp.RoomInfo
	if err := json.Unmarshal(frames[0].payload, &got); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if got.Num != "town:square" || got.Name != "Town Square" {
		t.Errorf("payload = %+v", got)
	}
}

func TestSendGmcpRoomInfo_NilRoomIsSafe(t *testing.T) {
	a, fc := newRoomGmcpActor("p-1", &world.Room{ID: "x", Exits: map[world.Direction]world.Exit{}})
	fc.setActive(true)
	a.sendGmcpRoomInfo(context.Background(), nil)
	if len(fc.framesSnapshot()) != 0 {
		t.Errorf("nil room emitted %d frames", len(fc.framesSnapshot()))
	}
}

func TestSetRoom_EmitsRoomInfoOnTransition(t *testing.T) {
	// SetRoom is the canonical movement seam. After it commits a
	// real transition (oldID != newID), the actor's GMCP-active
	// peer should receive one Room.Info frame for the destination.
	roomA := &world.Room{ID: "A", Name: "Room A", Exits: map[world.Direction]world.Exit{}}
	roomB := &world.Room{ID: "B", Name: "Room B", Exits: map[world.Direction]world.Exit{}}
	a, fc := newRoomGmcpActor("p-1", roomA)
	fc.setActive(true)

	a.SetRoom(roomB)

	frames := fc.framesSnapshot()
	if len(frames) != 1 {
		t.Fatalf("SetRoom emitted %d frames, want 1", len(frames))
	}
	var got gmcp.RoomInfo
	_ = json.Unmarshal(frames[0].payload, &got)
	if got.Num != "B" {
		t.Errorf("destination = %q, want B", got.Num)
	}
}

func TestSetRoom_SameRoomEmitsAnyway(t *testing.T) {
	// Even a SetRoom to the current room produces a Room.Info
	// frame. Movement loops (e.g. some content scripts) may
	// re-enter the same room and clients can use the redundant
	// frame as a "you're stationary" signal. Cheap and avoids
	// drift between the moveRoom side-effect (which DOES check
	// oldID != newID) and the GMCP emission.
	room := &world.Room{ID: "A", Name: "Room A", Exits: map[world.Direction]world.Exit{}}
	a, fc := newRoomGmcpActor("p-1", room)
	fc.setActive(true)

	a.SetRoom(room)

	if len(fc.framesSnapshot()) != 1 {
		t.Errorf("same-room SetRoom emitted %d frames, want 1", len(fc.framesSnapshot()))
	}
}

func TestSetRoom_NonGmcpConnIsSilent(t *testing.T) {
	room := &world.Room{ID: "A", Name: "Room A", Exits: map[world.Direction]world.Exit{}}
	a := &connActor{
		id:       "x",
		conn:     &fakeConn{id: "x"},
		playerID: "p-x",
		room:     room,
		vitals:   combat.NewVitalsAt(50, 100),
		save:     &player.Save{ID: "p-x", Sustenance: 100},
	}
	a.sustenance = 100
	// Must not panic, must not deadlock.
	a.SetRoom(&world.Room{ID: "B", Name: "B", Exits: map[world.Direction]world.Exit{}})
}

func TestSendGmcpRoomInfo_LandsOnSwappedConnAfterReattach(t *testing.T) {
	// Pins the M16.4b reconnect contract: after the link-dead
	// reattach swaps a.conn for a fresh peer, the next
	// sendGmcpRoomInfo call lands on the NEW conn, giving the
	// reconnected client's mapper panel its baseline frame.
	// (The reconnect function calls sendGmcpRoomInfo directly
	// after reattach succeeds; this test exercises the seam
	// without dragging in the full pump/teardown harness.)
	room := &world.Room{ID: "A", Name: "Room A", Exits: map[world.Direction]world.Exit{}}
	a, oldFC := newRoomGmcpActor("p-1", room)
	oldFC.setActive(true)

	// Simulate the conn swap that connActor.reattach performs.
	newFC := &gmcpFakeConn{fakeConn: fakeConn{id: "test-new"}}
	newFC.setActive(true)
	a.mu.Lock()
	a.conn = newFC
	a.mu.Unlock()

	a.sendGmcpRoomInfo(context.Background(), room)

	if got := len(oldFC.framesSnapshot()); got != 0 {
		t.Errorf("old conn received %d frames after swap, want 0", got)
	}
	if got := len(newFC.framesSnapshot()); got != 1 {
		t.Errorf("new conn received %d frames after swap, want 1", got)
	}
}

func TestSendGmcpRoomInfo_CarriesEffectiveLight(t *testing.T) {
	// Underground room with no light → the per-viewer level is black,
	// and the Room.Info frame carries it (light-and-darkness §8).
	room := &world.Room{ID: "x:cave", AreaID: "x", Name: "Cave", Terrain: world.TerrainUnderground}
	a, fc := newRoomGmcpActor("p-1", room)
	fc.setActive(true)
	a.items = entities.NewStore()
	a.placement = entities.NewPlacement()
	a.light = light.NewResolver(light.DefaultConfig(), fixedClockPeriod(gameclock.PeriodDay))

	a.sendGmcpRoomInfo(context.Background(), room)

	frames := fc.framesSnapshot()
	if len(frames) != 1 {
		t.Fatalf("emitted %d frames, want 1", len(frames))
	}
	var got gmcp.RoomInfo
	if err := json.Unmarshal(frames[0].payload, &got); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if got.Light != "black" {
		t.Fatalf("Room.Info light = %q, want black", got.Light)
	}
}

func TestSendGmcpRoomInfo_OmitsLightWhenUnwired(t *testing.T) {
	room := &world.Room{ID: "x:road", AreaID: "x", Name: "Road"}
	a, fc := newRoomGmcpActor("p-1", room)
	fc.setActive(true)
	// a.light left nil → field omitted.
	a.sendGmcpRoomInfo(context.Background(), room)
	frames := fc.framesSnapshot()
	if len(frames) != 1 {
		t.Fatalf("emitted %d frames, want 1", len(frames))
	}
	if string(frames[0].payload) == "" {
		t.Fatal("empty payload")
	}
	var got gmcp.RoomInfo
	if err := json.Unmarshal(frames[0].payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Light != "" {
		t.Fatalf("light should be omitted when unwired, got %q", got.Light)
	}
}

func TestBuildRoomMapPayload_NodesExitsVisited(t *testing.T) {
	mk := func(id, name string, x, y, z int, exits map[world.Direction]world.RoomID) world.WindowRoom {
		ex := make(map[world.Direction]world.Exit, len(exits))
		for d, target := range exits {
			ex[d] = world.Exit{Target: target}
		}
		return world.WindowRoom{
			Room:  &world.Room{ID: world.RoomID(id), Name: name, Exits: ex},
			Coord: world.Coord{X: x, Y: y, Z: z},
		}
	}
	win := world.Window{
		Origin: "town:square",
		Area:   "town",
		Rooms: []world.WindowRoom{
			mk("town:road", "{Y}North Road{x}", 0, 1, 0, map[world.Direction]world.RoomID{world.DirSouth: "town:square"}),
			mk("town:square", "Town Square", 0, 0, 0, map[world.Direction]world.RoomID{world.DirNorth: "town:road"}),
		},
	}
	// Nothing in the persisted fog set — proves the center is forced visited (you
	// stand in it) while the seen-but-unentered road stays unvisited (fog).
	visited := map[string]bool{}

	got := buildRoomMapPayload(win, "town:square", 3, func(id string) bool { return visited[id] })

	if got.Center != "town:square" || got.Radius != 3 {
		t.Fatalf("center/radius = %q/%d, want town:square/3", got.Center, got.Radius)
	}
	if len(got.Rooms) != 2 {
		t.Fatalf("nodes = %d, want 2", len(got.Rooms))
	}
	byID := map[string]gmcp.RoomMapNode{}
	for _, n := range got.Rooms {
		byID[n.Num] = n
	}
	sq := byID["town:square"]
	if !sq.Visited || sq.Exits["n"] != "town:road" || sq.X != 0 || sq.Y != 0 {
		t.Errorf("square node = %+v (want visited, exits[n]=town:road, 0,0)", sq)
	}
	road := byID["town:road"]
	if road.Visited { // fog: seen on the map but not entered
		t.Errorf("road should be unvisited (fog), got visited")
	}
	if road.Name != "North Road" { // colour markup stripped
		t.Errorf("road name = %q, want stripped 'North Road'", road.Name)
	}
	if road.Exits["s"] != "town:square" || road.Y != 1 {
		t.Errorf("road node = %+v (want exits[s]=town:square, y=1)", road)
	}
}

type fixedClockPeriod string

func (f fixedClockPeriod) CurrentPeriod() string { return string(f) }

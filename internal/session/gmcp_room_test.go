package session

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
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

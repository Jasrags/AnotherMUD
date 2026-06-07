package world_test

import (
	"errors"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// placedRoom builds a room at an explicit coordinate with directional
// exits (reuses roomWithExits from coords_test.go, then pins the coord).
func placedRoom(id world.RoomID, area world.AreaID, x, y, z int, exits map[world.Direction]world.RoomID) *world.Room {
	r := roomWithExits(id, area, exits)
	r.Coord = &world.Coord{X: x, Y: y, Z: z}
	return r
}

func ids(win world.Window) []world.RoomID {
	out := make([]world.RoomID, len(win.Rooms))
	for i, wr := range win.Rooms {
		out[i] = wr.Room.ID
	}
	return out
}

func eqIDs(a []world.RoomID, b ...world.RoomID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A chain a-b-c-d-e (bidirectional east/west) windows by step radius.
func chainWorld() *world.World {
	w := world.New()
	w.AddRoom(placedRoom("a", "ar", 0, 0, 0, map[world.Direction]world.RoomID{world.DirEast: "b"}))
	w.AddRoom(placedRoom("b", "ar", 1, 0, 0, map[world.Direction]world.RoomID{world.DirWest: "a", world.DirEast: "c"}))
	w.AddRoom(placedRoom("c", "ar", 2, 0, 0, map[world.Direction]world.RoomID{world.DirWest: "b", world.DirEast: "d"}))
	w.AddRoom(placedRoom("d", "ar", 3, 0, 0, map[world.Direction]world.RoomID{world.DirWest: "c", world.DirEast: "e"}))
	w.AddRoom(placedRoom("e", "ar", 4, 0, 0, map[world.Direction]world.RoomID{world.DirWest: "d"}))
	return w
}

func TestLocalWindow_StepRadius(t *testing.T) {
	w := chainWorld()
	cases := []struct {
		radius int
		want   []world.RoomID
	}{
		{0, []world.RoomID{"c"}},
		{1, []world.RoomID{"b", "c", "d"}},
		{2, []world.RoomID{"a", "b", "c", "d", "e"}},
		{-1, []world.RoomID{"a", "b", "c", "d", "e"}}, // unbounded = whole area
	}
	for _, tc := range cases {
		win, err := w.LocalWindow("c", tc.radius)
		if err != nil {
			t.Fatalf("radius %d: %v", tc.radius, err)
		}
		if got := ids(win); !eqIDs(got, tc.want...) {
			t.Errorf("radius %d: rooms = %v, want %v", tc.radius, got, tc.want)
		}
	}
}

func TestLocalWindow_CoordsCopiedNotAliased(t *testing.T) {
	w := chainWorld()
	win, _ := w.LocalWindow("c", 0)
	if len(win.Rooms) != 1 {
		t.Fatalf("rooms = %v", ids(win))
	}
	if win.Rooms[0].Coord != (world.Coord{X: 2, Y: 0, Z: 0}) {
		t.Errorf("coord = %+v, want (2,0,0)", win.Rooms[0].Coord)
	}
	got, ok := win.OriginCoord()
	if !ok || got != (world.Coord{X: 2, Y: 0, Z: 0}) {
		t.Errorf("OriginCoord = %+v,%v want (2,0,0),true", got, ok)
	}
}

// Cross-area exits are not walked; the window stays inside the origin area.
func TestLocalWindow_StopsAtAreaBoundary(t *testing.T) {
	w := world.New()
	w.AddRoom(placedRoom("a", "ar", 0, 0, 0, map[world.Direction]world.RoomID{world.DirEast: "x"}))
	w.AddRoom(placedRoom("x", "other", 0, 0, 0, map[world.Direction]world.RoomID{world.DirWest: "a"}))

	win, _ := w.LocalWindow("a", 5)
	if got := ids(win); !eqIDs(got, "a") {
		t.Errorf("rooms = %v, want [a] (cross-area x excluded)", got)
	}
	if win.Contains("x") {
		t.Error("Contains(x) = true, want false (different area)")
	}
	if !win.Contains("a") {
		t.Error("Contains(a) = false, want true (origin is a member)")
	}
}

// Keyword/portal exits are non-directional and never traversed.
func TestLocalWindow_IgnoresKeywordExits(t *testing.T) {
	w := world.New()
	a := placedRoom("a", "ar", 0, 0, 0, nil)
	a.KeywordExits = map[string]world.Exit{"portal": {Target: "b"}}
	w.AddRoom(a)
	w.AddRoom(placedRoom("b", "ar", 9, 9, 9, nil))

	win, _ := w.LocalWindow("a", 5)
	if got := ids(win); !eqIDs(got, "a") {
		t.Errorf("rooms = %v, want [a] (keyword target b not walked)", got)
	}
}

// A doored exit is crossed normally — a door never changes mapping.
func TestLocalWindow_CrossesDoors(t *testing.T) {
	w := world.New()
	a := placedRoom("a", "ar", 0, 0, 0, nil)
	a.Exits = map[world.Direction]world.Exit{
		world.DirEast: {Target: "b", Door: &world.DoorState{Closed: true, Locked: true}},
	}
	w.AddRoom(a)
	w.AddRoom(placedRoom("b", "ar", 1, 0, 0, map[world.Direction]world.RoomID{world.DirWest: "a"}))

	win, _ := w.LocalWindow("a", 1)
	if got := ids(win); !eqIDs(got, "a", "b") {
		t.Errorf("rooms = %v, want [a b] (locked door still mapped)", got)
	}
}

// An unplaced origin surfaces its placed neighbours but is itself absent,
// and OriginCoord reports it cannot center.
func TestLocalWindow_UnplacedOrigin(t *testing.T) {
	w := world.New()
	u := roomWithExits("u", "ar", map[world.Direction]world.RoomID{world.DirEast: "a"})
	// u has no Coord (unplaced) and no inbound exit.
	w.AddRoom(u)
	w.AddRoom(placedRoom("a", "ar", 0, 0, 0, map[world.Direction]world.RoomID{world.DirEast: "b"}))
	w.AddRoom(placedRoom("b", "ar", 1, 0, 0, nil))

	win, err := w.LocalWindow("u", 2)
	if err != nil {
		t.Fatalf("LocalWindow: %v", err)
	}
	if got := ids(win); !eqIDs(got, "a", "b") {
		t.Errorf("rooms = %v, want [a b] (unplaced origin u excluded, neighbours surfaced)", got)
	}
	if _, ok := win.OriginCoord(); ok {
		t.Error("OriginCoord ok = true, want false for an unplaced origin")
	}
}

func TestLocalWindow_OriginNotFound(t *testing.T) {
	w := world.New()
	if _, err := w.LocalWindow("nope", 1); !errors.Is(err, world.ErrRoomNotFound) {
		t.Errorf("err = %v, want ErrRoomNotFound", err)
	}
}

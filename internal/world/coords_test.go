package world_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// --- helpers ---

func roomWithExits(id world.RoomID, area world.AreaID, exits map[world.Direction]world.RoomID) *world.Room {
	ex := make(map[world.Direction]world.Exit, len(exits))
	for d, t := range exits {
		ex[d] = world.Exit{Target: t}
	}
	return &world.Room{ID: id, AreaID: area, Exits: ex}
}

func coordOf(t *testing.T, w *world.World, id world.RoomID) *world.Coord {
	t.Helper()
	r, err := w.Room(id)
	if err != nil {
		t.Fatalf("room %q: %v", id, err)
	}
	return r.Coord
}

func wantCoord(t *testing.T, w *world.World, id world.RoomID, x, y, z int) {
	t.Helper()
	c := coordOf(t, w, id)
	if c == nil {
		t.Fatalf("room %q unplaced, want (%d,%d,%d)", id, x, y, z)
	}
	if *c != (world.Coord{X: x, Y: y, Z: z}) {
		t.Errorf("room %q = %+v, want (%d,%d,%d)", id, *c, x, y, z)
	}
}

func wantUnplaced(t *testing.T, w *world.World, id world.RoomID) {
	t.Helper()
	if c := coordOf(t, w, id); c != nil {
		t.Errorf("room %q placed at %+v, want unplaced (nil)", id, *c)
	}
}

func findWarning(ws []world.CoordWarning, kind world.CoordWarningKind, room world.RoomID) (world.CoordWarning, bool) {
	for _, w := range ws {
		if w.Kind == kind && (room == "" || w.Room == room || w.Other == room) {
			return w, true
		}
	}
	return world.CoordWarning{}, false
}

// --- §2.3 direction deltas ---

func TestDelta(t *testing.T) {
	t.Parallel()
	cases := []struct {
		dir     world.Direction
		x, y, z int
	}{
		{world.DirNorth, 0, 1, 0},
		{world.DirSouth, 0, -1, 0},
		{world.DirEast, 1, 0, 0},
		{world.DirWest, -1, 0, 0},
		{world.DirUp, 0, 0, 1},
		{world.DirDown, 0, 0, -1},
		{world.DirInvalid, 0, 0, 0},
	}
	for _, c := range cases {
		if got := world.Delta(c.dir); got != (world.Coord{X: c.x, Y: c.y, Z: c.z}) {
			t.Errorf("Delta(%s) = %+v, want (%d,%d,%d)", c.dir, got, c.x, c.y, c.z)
		}
	}
}

// --- §3.1/§3.2 default anchor + walk ---

func TestDeriveDefaultAnchorAndWalk(t *testing.T) {
	t.Parallel()
	w := world.New()
	w.AddArea(&world.Area{ID: "a"})
	// b is the lexicographically-smallest id → default anchor at origin.
	w.AddRoom(roomWithExits("b", "a", map[world.Direction]world.RoomID{
		world.DirNorth: "c", world.DirEast: "d", world.DirUp: "e",
	}))
	w.AddRoom(roomWithExits("c", "a", nil))
	w.AddRoom(roomWithExits("d", "a", nil))
	w.AddRoom(roomWithExits("e", "a", nil))

	if ws := w.DeriveCoordinates(); len(ws) != 0 {
		t.Fatalf("unexpected warnings: %+v", ws)
	}
	wantCoord(t, w, "b", 0, 0, 0) // origin is a valid placed coordinate
	wantCoord(t, w, "c", 0, 1, 0)
	wantCoord(t, w, "d", 1, 0, 0)
	wantCoord(t, w, "e", 0, 0, 1)
}

// --- §2.4 origin is placed, not unplaced (the (0,0,0) trap) ---

func TestOriginIsPlaced(t *testing.T) {
	t.Parallel()
	w := world.New()
	w.AddRoom(roomWithExits("only", "a", nil))
	w.DeriveCoordinates()
	wantCoord(t, w, "only", 0, 0, 0)
}

// --- §3.5 pin seeds the area; no synthetic anchor ---

func TestDerivePinSeedsArea(t *testing.T) {
	t.Parallel()
	w := world.New()
	p := roomWithExits("p", "a", map[world.Direction]world.RoomID{world.DirNorth: "q"})
	p.Pin = &world.Coord{X: 5, Y: 5, Z: 5}
	w.AddRoom(p)
	w.AddRoom(roomWithExits("q", "a", nil))
	// "aaa" sorts before "p" but is disconnected: with a pin present no
	// synthetic anchor is added, so it stays unplaced (§3.1).
	w.AddRoom(roomWithExits("aaa", "a", nil))

	ws := w.DeriveCoordinates()
	wantCoord(t, w, "p", 5, 5, 5) // origin is wherever the author pinned
	wantCoord(t, w, "q", 5, 6, 5)
	wantUnplaced(t, w, "aaa")
	if _, ok := findWarning(ws, world.CoordWarnUnplaced, "aaa"); !ok {
		t.Errorf("want unplaced-room warning for aaa; got %+v", ws)
	}
}

// --- §3.6/§4.4 a pin is never overwritten; re-reaching it is silent ---

func TestDerivePinNotOverwritten(t *testing.T) {
	t.Parallel()
	w := world.New()
	p := roomWithExits("p", "a", map[world.Direction]world.RoomID{world.DirEast: "q"})
	p.Pin = &world.Coord{X: 0, Y: 0, Z: 0}
	w.AddRoom(p)
	// q loops back west to p, implying p at (0,0,0) — consistent here, but
	// even an inconsistent implication must not warn for a pin (§4.4).
	w.AddRoom(roomWithExits("q", "a", map[world.Direction]world.RoomID{world.DirWest: "p"}))

	ws := w.DeriveCoordinates()
	wantCoord(t, w, "p", 0, 0, 0)
	wantCoord(t, w, "q", 1, 0, 0)
	if _, ok := findWarning(ws, world.CoordWarnInconsistent, "p"); ok {
		t.Errorf("re-reaching a pin must be silent; got %+v", ws)
	}
}

// --- §4.1 collision: two rooms, one cell ---

func TestCollisionFirstWins(t *testing.T) {
	t.Parallel()
	w := world.New()
	// a(0,0,0) →N→ b(0,1,0); b →S→ d, which lands back on a's cell (0,0,0).
	w.AddRoom(roomWithExits("a", "z", map[world.Direction]world.RoomID{world.DirNorth: "b"}))
	w.AddRoom(roomWithExits("b", "z", map[world.Direction]world.RoomID{world.DirSouth: "d"}))
	w.AddRoom(roomWithExits("d", "z", nil))

	ws := w.DeriveCoordinates()
	wantCoord(t, w, "a", 0, 0, 0) // first placement keeps the cell
	wantCoord(t, w, "b", 0, 1, 0)
	wantCoord(t, w, "d", 0, 0, 0) // later room keeps its own first placement
	cw, ok := findWarning(ws, world.CoordWarnCollision, "d")
	if !ok {
		t.Fatalf("want collision warning naming d; got %+v", ws)
	}
	if cw.At != (world.Coord{X: 0, Y: 0, Z: 0}) {
		t.Errorf("collision At = %+v, want (0,0,0)", cw.At)
	}
}

// --- §4.2 non-square loop: inconsistent re-reach ---

func TestInconsistentEdge(t *testing.T) {
	t.Parallel()
	w := world.New()
	// a(0,0,0): N→c places c at (0,1,0); E→b places b at (1,0,0).
	// b →N→ c implies c at (1,1,0), contradicting (0,1,0).
	w.AddRoom(roomWithExits("a", "z", map[world.Direction]world.RoomID{
		world.DirNorth: "c", world.DirEast: "b",
	}))
	w.AddRoom(roomWithExits("b", "z", map[world.Direction]world.RoomID{world.DirNorth: "c"}))
	w.AddRoom(roomWithExits("c", "z", nil))

	ws := w.DeriveCoordinates()
	wantCoord(t, w, "c", 0, 1, 0) // first assignment wins; not mutated
	cw, ok := findWarning(ws, world.CoordWarnInconsistent, "c")
	if !ok {
		t.Fatalf("want inconsistent-edge warning for c; got %+v", ws)
	}
	if cw.At != (world.Coord{X: 0, Y: 1, Z: 0}) || cw.Expect != (world.Coord{X: 1, Y: 1, Z: 0}) {
		t.Errorf("inconsistent edge: At=%+v Expect=%+v, want existing (0,1,0) expected (1,1,0)", cw.At, cw.Expect)
	}
	if cw.Dir != world.DirNorth {
		t.Errorf("inconsistent edge Dir = %s, want north", cw.Dir)
	}
}

// --- §3.3/§4.3 unplaced room: portal-only / disconnected ---

func TestUnplacedRoomKeywordExitNotStepped(t *testing.T) {
	t.Parallel()
	w := world.New()
	a := roomWithExits("a", "z", nil)
	// A keyword exit (portal) must NOT place its target (§3.3).
	a.KeywordExits = map[string]world.Exit{"portal": {Target: "b"}}
	w.AddRoom(a)
	w.AddRoom(roomWithExits("b", "z", nil))

	ws := w.DeriveCoordinates()
	wantCoord(t, w, "a", 0, 0, 0)
	wantUnplaced(t, w, "b")
	if _, ok := findWarning(ws, world.CoordWarnUnplaced, "b"); !ok {
		t.Errorf("want unplaced-room warning for b; got %+v", ws)
	}
}

// --- §3.3 cross-area exits are not stepped; coordinates are area-local ---

func TestCrossAreaNotStepped(t *testing.T) {
	t.Parallel()
	w := world.New()
	// a (area1) has an east exit into x (area2). The walk must not step it;
	// x is placed by area2's own walk (its own anchor at origin).
	w.AddRoom(roomWithExits("a", "area1", map[world.Direction]world.RoomID{world.DirEast: "x"}))
	w.AddRoom(roomWithExits("x", "area2", nil))

	if ws := w.DeriveCoordinates(); len(ws) != 0 {
		t.Fatalf("unexpected warnings: %+v", ws)
	}
	wantCoord(t, w, "a", 0, 0, 0)
	wantCoord(t, w, "x", 0, 0, 0) // its own area's origin, not (1,0,0)
}

// --- §4.4 two pins, one cell ---

func TestPinCollision(t *testing.T) {
	t.Parallel()
	w := world.New()
	p1 := roomWithExits("p1", "a", nil)
	p1.Pin = &world.Coord{X: 2, Y: 2, Z: 0}
	p2 := roomWithExits("p2", "a", nil)
	p2.Pin = &world.Coord{X: 2, Y: 2, Z: 0}
	w.AddRoom(p1)
	w.AddRoom(p2)

	ws := w.DeriveCoordinates()
	cw, ok := findWarning(ws, world.CoordWarnPinCollision, "")
	if !ok {
		t.Fatalf("want pin-collision warning; got %+v", ws)
	}
	if cw.Room != "p1" || cw.Other != "p2" {
		t.Errorf("pin-collision rooms = %q/%q, want p1 (first-by-id) / p2", cw.Room, cw.Other)
	}
	// Both still carry their authored coordinate (overlap tolerated).
	wantCoord(t, w, "p1", 2, 2, 0)
	wantCoord(t, w, "p2", 2, 2, 0)
}

// --- §4.4 derived room landing on a pinned cell warns as a collision ---

func TestDerivedRoomOnPinnedCell(t *testing.T) {
	t.Parallel()
	w := world.New()
	// p pinned at (1,0,0); p →W→ a at (0,0,0); a →E→ b lands on p's cell.
	p := roomWithExits("p", "a", map[world.Direction]world.RoomID{world.DirWest: "a"})
	p.Pin = &world.Coord{X: 1, Y: 0, Z: 0}
	w.AddRoom(p)
	w.AddRoom(roomWithExits("a", "a", map[world.Direction]world.RoomID{world.DirEast: "b"}))
	w.AddRoom(roomWithExits("b", "a", nil))

	ws := w.DeriveCoordinates()
	wantCoord(t, w, "p", 1, 0, 0)
	wantCoord(t, w, "a", 0, 0, 0)
	wantCoord(t, w, "b", 1, 0, 0) // derived room keeps its own placement
	if _, ok := findWarning(ws, world.CoordWarnCollision, "b"); !ok {
		t.Errorf("want collision warning for b landing on p's pinned cell; got %+v", ws)
	}
}

// --- §3.4 determinism: re-deriving identical content is byte-identical ---

func TestDeriveDeterministic(t *testing.T) {
	t.Parallel()
	build := func() *world.World {
		w := world.New()
		w.AddRoom(roomWithExits("b", "a", map[world.Direction]world.RoomID{
			world.DirNorth: "c", world.DirEast: "d",
		}))
		w.AddRoom(roomWithExits("c", "a", map[world.Direction]world.RoomID{world.DirEast: "e"}))
		w.AddRoom(roomWithExits("d", "a", map[world.Direction]world.RoomID{world.DirNorth: "e"}))
		w.AddRoom(roomWithExits("e", "a", nil))
		return w
	}
	w := build()
	w.DeriveCoordinates()
	snap := map[world.RoomID]world.Coord{}
	for _, id := range []world.RoomID{"b", "c", "d", "e"} {
		snap[id] = *coordOf(t, w, id)
	}
	// Re-run on the same world: idempotent.
	w.DeriveCoordinates()
	for _, id := range []world.RoomID{"b", "c", "d", "e"} {
		if *coordOf(t, w, id) != snap[id] {
			t.Errorf("re-run room %q = %+v, want %+v (not idempotent)", id, *coordOf(t, w, id), snap[id])
		}
	}
	// Fresh world, same content: same coordinates.
	w2 := build()
	w2.DeriveCoordinates()
	for _, id := range []world.RoomID{"b", "c", "d", "e"} {
		if *coordOf(t, w2, id) != snap[id] {
			t.Errorf("fresh world room %q = %+v, want %+v (not deterministic)", id, *coordOf(t, w2, id), snap[id])
		}
	}
}

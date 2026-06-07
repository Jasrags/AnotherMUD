package world

import (
	"fmt"
	"sort"
)

// WindowRoom is one placed room in a local window: the room and a copy
// of its stable area-local coordinate (player-maps §2). Coord is copied
// so a window holder never aliases the shared Room.Coord pointer.
type WindowRoom struct {
	Room  *Room
	Coord Coord
}

// Window is the result of a LocalWindow query (player-maps §2): the
// origin room, its area, and the placed same-area rooms reachable within
// the requested radius, each with its stable coordinate. It is read-only
// and applies NO fog-of-war filter — callers (the minimap, the map verb,
// the GMCP path) intersect Rooms with the viewer's visited set
// themselves (§3). Rooms is sorted by room id for a deterministic result.
type Window struct {
	Origin RoomID
	Area   AreaID
	Rooms  []WindowRoom // placed members, ascending by id; origin included iff placed
}

// Contains reports whether id is a placed member of the window — the
// membership test a renderer uses to classify an exit as a connector
// (target in window) versus a fog/edge stub (target absent).
func (win Window) Contains(id RoomID) bool {
	for _, wr := range win.Rooms {
		if wr.Room.ID == id {
			return true
		}
	}
	return false
}

// OriginCoord returns the origin room's coordinate and true when the
// origin is placed; ok is false when the origin is unplaced (no
// coordinate), which is how a renderer learns it cannot center
// (player-maps §4/§5 degrade to a placeholder).
func (win Window) OriginCoord() (Coord, bool) {
	for _, wr := range win.Rooms {
		if wr.Room.ID == win.Origin {
			return wr.Coord, true
		}
	}
	return Coord{}, false
}

// LocalWindow returns the placed, same-area rooms reachable from origin
// within radius exit-steps, plus their stable coordinates (player-maps
// §2) — the shared seam under both ASCII map surfaces and the GMCP feed.
//
// radius is a STEP bound (BFS depth): the origin is depth 0, its direct
// neighbors depth 1, and so on. A negative radius is unbounded — every
// placed room in the origin's area reachable over intra-area directional
// exits (the "whole area" the map verb draws). The walk:
//
//   - follows only directional exits (Room.Exits); keyword/portal exits
//     are non-directional and never traversed (§2, §6.5);
//   - stops at the area boundary — a cross-area exit target is neither
//     traversed nor included;
//   - crosses doored exits normally (a door never changes whether a room
//     is mapped, §9);
//   - collects a room only when it is placed (Coord != nil), but still
//     expands through it, so an unplaced origin still surfaces its placed
//     neighbours.
//
// It applies no fog filter. Returns ErrRoomNotFound if origin is absent.
func (w *World) LocalWindow(origin RoomID, radius int) (Window, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	start, ok := w.rooms[origin]
	if !ok {
		return Window{}, fmt.Errorf("world.LocalWindow(%q): %w", origin, ErrRoomNotFound)
	}
	area := start.AreaID
	win := Window{Origin: origin, Area: area}

	type queued struct {
		id    RoomID
		depth int
	}
	seen := map[RoomID]bool{origin: true}
	queue := []queued{{origin, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		room := w.rooms[cur.id]
		if room == nil || room.AreaID != area {
			continue
		}
		if room.Coord != nil {
			win.Rooms = append(win.Rooms, WindowRoom{Room: room, Coord: *room.Coord})
		}
		if radius >= 0 && cur.depth >= radius {
			continue // at the step bound: collect but do not expand
		}
		for _, dir := range coordWalkOrder {
			exit, ok := room.Exits[dir]
			if !ok || seen[exit.Target] {
				continue
			}
			target, ok := w.rooms[exit.Target]
			if !ok || target.AreaID != area {
				continue // cross-area or dangling: not walked (§2)
			}
			seen[exit.Target] = true
			queue = append(queue, queued{exit.Target, cur.depth + 1})
		}
	}

	sort.Slice(win.Rooms, func(i, j int) bool { return win.Rooms[i].Room.ID < win.Rooms[j].Room.ID })
	return win, nil
}

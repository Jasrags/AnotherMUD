package world

import "sort"

// Coord is an area-local integer (x, y, z) coordinate
// (room-coordinates §2.1). x is the east(+)/west(−) axis, y is
// north(+)/south(−), z is up(+)/down(−). One exit = one unit step;
// integers, never floats. A placed room has exactly one Coord; an
// unplaced room has none (a nil *Coord, never (0,0,0) — the origin is
// a legitimate placed value).
type Coord struct {
	X, Y, Z int
}

// add returns c offset by d (used to step a coordinate across an exit).
func (c Coord) add(d Coord) Coord {
	return Coord{c.X + d.X, c.Y + d.Y, c.Z + d.Z}
}

// Delta returns the fixed per-direction unit step applied when walking
// a coordinate across an exit in direction d (room-coordinates §2.3,
// PD-5: north = +y, east = +x, up = +z). Non-directional / invalid
// directions return the zero Coord; the walk never steps them.
func Delta(d Direction) Coord {
	switch d {
	case DirNorth:
		return Coord{0, 1, 0}
	case DirSouth:
		return Coord{0, -1, 0}
	case DirEast:
		return Coord{1, 0, 0}
	case DirWest:
		return Coord{-1, 0, 0}
	case DirUp:
		return Coord{0, 0, 1}
	case DirDown:
		return Coord{0, 0, -1}
	}
	return Coord{}
}

// coordWalkOrder is the canonical per-room exit visitation order for
// coordinate derivation (room-coordinates §3.2 step 2: north, south,
// east, west, up, down). Deliberately distinct from door.go's
// canonicalDirections (N, E, S, W, U, D) — the coordinate spec states a
// different order, honored verbatim here and locked by a test so the
// two orders cannot silently converge or drift.
var coordWalkOrder = []Direction{
	DirNorth, DirSouth, DirEast, DirWest, DirUp, DirDown,
}

// CoordWarningKind classifies a non-fatal derivation finding
// (room-coordinates §4).
type CoordWarningKind string

const (
	// CoordWarnCollision: two same-area rooms derive to one cell
	// (§4.1). First placement keeps the cell; the later room keeps
	// the coordinate its own first placement assigned.
	CoordWarnCollision CoordWarningKind = "collision"
	// CoordWarnInconsistent: a room is re-reached by a path implying a
	// different coordinate — a non-square loop (§4.2). First
	// assignment wins; nothing is mutated.
	CoordWarnInconsistent CoordWarningKind = "inconsistent_edge"
	// CoordWarnUnplaced: a room neither pinned nor reachable from any
	// seed via intra-area directional exits (§4.3). It carries no
	// coordinate.
	CoordWarnUnplaced CoordWarningKind = "unplaced_room"
	// CoordWarnPinCollision: two pins declare the same cell (§4.4) — an
	// authoring mistake. First-by-id wins the cell; the load continues.
	CoordWarnPinCollision CoordWarningKind = "pin_collision"
)

// CoordWarning is one non-fatal derivation finding (room-coordinates
// §4, PD-4). DeriveCoordinates returns these for the loader to log; no
// warning aborts the load. The returned-value (rather than log-inline)
// shape keeps this package logger-free and lets tests assert findings.
type CoordWarning struct {
	Kind  CoordWarningKind
	Area  AreaID
	Room  RoomID // primary room (the first/winning room on a collision)
	Other RoomID // the second room on a collision / pin-collision; else ""
	// Dir is the edge direction for an inconsistent-edge warning;
	// DirInvalid for the others.
	Dir Direction
	// At is the cell in question (the occupied cell on a collision; the
	// existing coordinate on an inconsistent re-reach). Zero for an
	// unplaced room, which has no cell.
	At Coord
	// Expect is the coordinate the contradicting edge implied
	// (inconsistent-edge only); zero otherwise.
	Expect Coord
}

// DeriveCoordinates assigns every placed room an area-local Coord by
// walking each area's intra-area directional exit graph outward from a
// seed (room-coordinates §3). It is a pure function of the assembled
// graph plus authored pins (Room.Pin): same graph + pins always yields
// byte-identical coordinates (PD-6). It sets Room.Coord on placed rooms
// and leaves it nil on unplaced ones (§4.3), returning non-fatal
// warnings for collisions, non-square re-reaches, unreachable rooms,
// and pin collisions (§4).
//
// Boot-time only: it takes the write lock and mutates Room.Coord, so it
// must run after the graph is fully assembled and before the world
// serves connections (the same window as the loader's exit validation).
// Re-running is idempotent — prior derived coordinates are cleared
// first; pins (authored input) are left untouched.
func (w *World) DeriveCoordinates() []CoordWarning {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Reset prior derivation so a re-run reproduces from scratch. Pins
	// are authored content, not derived output — leave them.
	byArea := make(map[AreaID][]*Room)
	for _, r := range w.rooms {
		r.Coord = nil
		byArea[r.AreaID] = append(byArea[r.AreaID], r)
	}

	areaIDs := make([]AreaID, 0, len(byArea))
	for id := range byArea {
		areaIDs = append(areaIDs, id)
	}
	sort.Slice(areaIDs, func(i, j int) bool { return areaIDs[i] < areaIDs[j] })

	var warnings []CoordWarning
	for _, aid := range areaIDs {
		warnings = append(warnings, deriveArea(aid, byArea[aid])...)
	}
	return warnings
}

// deriveArea places one area's rooms in three phases — seed the pins or
// default anchor (§3.1), walk the intra-area exit graph (§3.2), then
// report anything left unreachable (§4.3). Caller holds w.mu;
// coordinates are area-local so areas never interact.
func deriveArea(aid AreaID, rooms []*Room) []CoordWarning {
	d := newAreaDeriver(aid, rooms)
	d.placeSeeds()
	d.walk()
	d.collectUnplaced()
	return d.warnings
}

// areaDeriver holds the per-area derivation state shared across the three
// phases: the area's rooms indexed + sorted by id, the placement result,
// and the cell-occupancy index used for collision detection.
type areaDeriver struct {
	aid       AreaID
	byID      map[RoomID]*Room
	ids       []RoomID          // ascending; the deterministic processing order
	placed    map[RoomID]Coord  // rooms assigned a coordinate this run
	occupancy map[Coord]RoomID  // first room to claim each cell
	warnings  []CoordWarning
}

func newAreaDeriver(aid AreaID, rooms []*Room) *areaDeriver {
	d := &areaDeriver{
		aid:       aid,
		byID:      make(map[RoomID]*Room, len(rooms)),
		ids:       make([]RoomID, 0, len(rooms)),
		placed:    make(map[RoomID]Coord),
		occupancy: make(map[Coord]RoomID),
	}
	for _, r := range rooms {
		d.byID[r.ID] = r
		d.ids = append(d.ids, r.ID)
	}
	sort.Slice(d.ids, func(i, j int) bool { return d.ids[i] < d.ids[j] })
	return d
}

// claim records id at c when the cell is free; otherwise reports the
// prior holder (first claimant keeps the cell — §4.1/§4.4).
func (d *areaDeriver) claim(id RoomID, c Coord) (RoomID, bool) {
	if prev, taken := d.occupancy[c]; taken {
		return prev, true
	}
	d.occupancy[c] = id
	return "", false
}

// place assigns room id the coordinate c — recording it in placed and
// stamping a fresh *Coord onto the room (no aliasing of the loop value).
func (d *areaDeriver) place(id RoomID, c Coord) {
	d.placed[id] = c
	cc := c
	d.byID[id].Coord = &cc
}

// placeSeeds fixes the rooms whose coordinates precede the walk (§3.1).
// A pinned area is seeded by its pins (no synthetic anchor); a pin-free
// area is seeded by the lexicographically-smallest room id at the origin.
func (d *areaDeriver) placeSeeds() {
	var pinIDs []RoomID
	for _, id := range d.ids {
		if d.byID[id].Pin != nil {
			pinIDs = append(pinIDs, id)
		}
	}
	if len(pinIDs) > 0 {
		for _, id := range pinIDs {
			c := *d.byID[id].Pin
			if prev, collided := d.claim(id, c); collided {
				d.warnings = append(d.warnings, CoordWarning{
					Kind: CoordWarnPinCollision, Area: d.aid,
					Room: prev, Other: id, At: c, Dir: DirInvalid,
				})
				// First-by-id keeps the cell; this pin still records its
				// own authored coordinate (overlap tolerated, §4.4).
			}
			d.place(id, c)
		}
		return
	}
	if len(d.ids) > 0 {
		// Pin-free area: default anchor = lexicographically-smallest
		// room id at the origin (§3.1, configurable §9).
		anchor := d.ids[0]
		d.claim(anchor, Coord{0, 0, 0})
		d.place(anchor, Coord{0, 0, 0})
	}
}

// walk performs the deterministic BFS (§3.2): repeatedly process the
// smallest-id placed-but-unprocessed room, visiting its directional exits
// in canonical order. Seeds-first / ascending-id / fixed-direction
// ordering makes the result deterministic (PD-6).
func (d *areaDeriver) walk() {
	processed := make(map[RoomID]bool)
	for {
		next, found := d.nextUnprocessed(processed)
		if !found {
			break
		}
		processed[next] = true
		d.step(next)
	}
}

// nextUnprocessed returns the smallest-id room that is placed but not yet
// processed, or found=false when the walk is complete.
func (d *areaDeriver) nextUnprocessed(processed map[RoomID]bool) (RoomID, bool) {
	for _, id := range d.ids { // ids is ascending
		if _, isPlaced := d.placed[id]; isPlaced && !processed[id] {
			return id, true
		}
	}
	return "", false
}

// step visits each directional exit of an already-placed room and places
// or reconciles the same-area target across that exit's delta.
func (d *areaDeriver) step(from RoomID) {
	base := d.placed[from]
	for _, dir := range coordWalkOrder {
		exit, ok := d.byID[from].Exits[dir]
		if !ok {
			continue
		}
		target, sameArea := d.byID[exit.Target]
		if !sameArea {
			continue // cross-area or dangling target: not stepped (§3.3)
		}
		want := base.add(Delta(dir))
		if existing, isPlaced := d.placed[exit.Target]; isPlaced {
			// Already placed (pin or earlier step): never re-placed (§3.2
			// step 4). A different implied coordinate is a non-square
			// re-reach (§4.2) — but a pin is ground truth (§4.4), so
			// re-reaching a pin is silent.
			if existing != want && target.Pin == nil {
				d.warnings = append(d.warnings, CoordWarning{
					Kind: CoordWarnInconsistent, Area: d.aid,
					Room: from, Other: exit.Target, Dir: dir,
					At: existing, Expect: want,
				})
			}
			continue
		}
		// Place the target. A taken cell is a collision (§4.1); this also
		// covers a derived room landing on a pinned cell (§4.4: warned as
		// an ordinary collision, the pin keeps the cell).
		if prev, collided := d.claim(exit.Target, want); collided {
			d.warnings = append(d.warnings, CoordWarning{
				Kind: CoordWarnCollision, Area: d.aid,
				Room: prev, Other: exit.Target, At: want, Dir: DirInvalid,
			})
		}
		d.place(exit.Target, want)
	}
}

// collectUnplaced appends an unplaced-room warning for every room neither
// pinned nor reached by the walk (§4.3); such rooms keep a nil Coord.
func (d *areaDeriver) collectUnplaced() {
	for _, id := range d.ids {
		if _, isPlaced := d.placed[id]; !isPlaced {
			d.warnings = append(d.warnings, CoordWarning{
				Kind: CoordWarnUnplaced, Area: d.aid, Room: id, Dir: DirInvalid,
			})
		}
	}
}

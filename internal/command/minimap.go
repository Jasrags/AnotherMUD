package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// MapViewer is the actor capability the player-maps surfaces read: the
// fog-of-war visited set (player-maps §3) and the persisted active-
// minimap preference (§4). connActor satisfies it; an actor that does
// not is treated as having no map.
type MapViewer interface {
	HasVisited(roomID string) bool
	VisitedRooms() []string
	MinimapEnabled() bool
	SetMinimapEnabled(bool)
}

// defaultMinimapRadius is the step radius of the active minimap window
// (player-maps §4, §10 policy) — small so the bordered widget stays
// terminal-sized; the border then bounds the fog-of-war window the player
// can currently see. The `map` verb uses an unbounded radius instead.
const defaultMinimapRadius = 2

// terrainGlyph maps a room's terrain to a single map glyph (player-maps
// §6.2, §10 policy). Unknown/empty terrain falls back to the default.
var terrainGlyph = map[string]rune{
	"outdoors":    '.',
	"indoors":     'o',
	"underground": '%',
	"water":       '~',
	"road":        '=',
	"forest":      '*',
	"mountain":    '^',
}

func glyphFor(terrain string) string {
	if g, ok := terrainGlyph[strings.ToLower(strings.TrimSpace(terrain))]; ok {
		return string(g)
	}
	return "."
}

// cardinalConn pairs a cardinal direction with its grid connector glyph
// and the (col,row) step from a room cell to the connector cell. The
// grid uses cell spacing 2 so connectors sit between room cells; north
// (+y) is up, so it decreases the row.
type cardinalConn struct {
	dir        world.Direction
	dcol, drow int
	glyph      string
}

var cardinalConns = []cardinalConn{
	{world.DirNorth, 0, -1, "<subtle>|</subtle>"},
	{world.DirSouth, 0, 1, "<subtle>|</subtle>"},
	{world.DirEast, 1, 0, "<subtle>-</subtle>"},
	{world.DirWest, -1, 0, "<subtle>-</subtle>"},
}

// renderLocalMap draws the fog-filtered window as an ASCII grid centred
// on the origin (player-maps §6). It renders only visited rooms on the
// origin's z-level; a cardinal exit to a non-rendered room becomes a
// stub connector into blank (§6.4); vertical and keyword exits from the
// origin are annotated below the grid (§6.5). Returns ("", false) when
// the origin is unplaced and the map cannot be centred (§4/§5).
func renderLocalMap(win world.Window, originID world.RoomID, isVisited func(string) bool) (string, bool) {
	canvas, origin, ok := buildMapCanvas(win, originID, isVisited)
	if !ok {
		return "", false
	}
	out := canvas.render()
	if notes := originNotes(origin); notes != "" {
		out += "\n" + notes
	}
	return out, true
}

// renderFramedMinimap is the active-minimap form: the same grid enclosed
// in a border that bounds the fog-of-war window the player can currently
// see (player-maps §4) — so the boxed extent reads at a glance. The
// vertical/keyword-exit notes sit below the box.
func renderFramedMinimap(win world.Window, originID world.RoomID, isVisited func(string) bool) (string, bool) {
	canvas, origin, ok := buildMapCanvas(win, originID, isVisited)
	if !ok {
		return "", false
	}
	out := frameBox(canvas.render())
	if notes := originNotes(origin); notes != "" {
		out += "\n" + notes
	}
	return out, true
}

// buildMapCanvas places the fog-filtered, z-plane-filtered window rooms
// onto a recentred canvas (viewer at the origin cell): the origin marker,
// terrain glyphs, and a connector per cardinal exit (a link between two
// rendered rooms, or a stub into blank toward a non-rendered room, §6.4).
// Returns ok=false when the origin is unplaced and cannot be centred.
func buildMapCanvas(win world.Window, originID world.RoomID, isVisited func(string) bool) (*mapCanvas, *world.Room, bool) {
	origin, ok := win.OriginCoord()
	if !ok {
		return nil, nil, false
	}
	rendered := renderableRooms(win, originID, origin.Z, isVisited)
	canvas := newMapCanvas()
	for id, wr := range rendered {
		col, row := (wr.Coord.X-origin.X)*2, -(wr.Coord.Y-origin.Y)*2
		if id == originID {
			canvas.set(col, row, "<highlight>@</highlight>")
		} else {
			canvas.set(col, row, glyphFor(wr.Room.Terrain))
		}
		for _, c := range cardinalConns {
			if _, has := wr.Room.Exits[c.dir]; has {
				canvas.set(col+c.dcol, row+c.drow, c.glyph)
			}
		}
	}
	// The origin is always rendered (renderableRooms includes it
	// unconditionally), so this is present whenever OriginCoord succeeded
	// — but guard explicitly rather than rely on that invariant for the
	// *world.Room deref.
	originWR, ok := rendered[originID]
	if !ok {
		return nil, nil, false
	}
	return canvas, originWR.Room, true
}

// frameBox encloses a multi-line map in an ASCII border (player-maps §4):
// the box bounds the mapped fog-of-war window so its extent is legible.
// Border glyphs use <frame> so they theme like the panel renderer and
// degrade to plain ASCII; rows are padded to a common width so the right
// edge stays straight.
func frameBox(content string) string {
	lines := strings.Split(content, "\n")
	width := 0
	for _, ln := range lines {
		if w := markupWidth(ln); w > width {
			width = w
		}
	}
	bar := "<frame>+" + strings.Repeat("-", width+2) + "+</frame>"
	var b strings.Builder
	b.WriteString(bar + "\n")
	for _, ln := range lines {
		b.WriteString("<frame>|</frame> " + padRight(ln, width) + " <frame>|</frame>\n")
	}
	b.WriteString(bar)
	return b.String()
}

// renderableRooms selects the window rooms to draw: the visited rooms on
// the origin's z-plane (single-floor ASCII, PD-7), plus the origin
// itself (always drawn).
func renderableRooms(win world.Window, originID world.RoomID, originZ int, isVisited func(string) bool) map[world.RoomID]world.WindowRoom {
	out := make(map[world.RoomID]world.WindowRoom, len(win.Rooms))
	for _, wr := range win.Rooms {
		if wr.Coord.Z != originZ {
			continue
		}
		if wr.Room.ID == originID || isVisited(string(wr.Room.ID)) {
			out[wr.Room.ID] = wr
		}
	}
	return out
}

// originNotes annotates the origin's non-grid exits (player-maps §6.5):
// vertical exits as up/down, keyword exits as portals. Empty when none.
func originNotes(origin *world.Room) string {
	if origin == nil {
		return ""
	}
	var notes []string
	if _, ok := origin.Exits[world.DirUp]; ok {
		notes = append(notes, "up")
	}
	if _, ok := origin.Exits[world.DirDown]; ok {
		notes = append(notes, "down")
	}
	if len(origin.KeywordExits) > 0 {
		kws := make([]string, 0, len(origin.KeywordExits))
		for k := range origin.KeywordExits {
			kws = append(kws, k)
		}
		sort.Strings(kws)
		notes = append(notes, "portal: "+strings.Join(kws, ", "))
	}
	if len(notes) == 0 {
		return ""
	}
	return "<subtle>(" + strings.Join(notes, ", ") + ")</subtle>"
}

// AppendMinimap appends the active minimap to a room view when the
// viewer is a MapViewer with the minimap toggle on (player-maps §4). It
// is the single seam every "you see the room" render routes through —
// look, movement, recall, teleport, flee, login spawn, link-dead
// reattach — so the minimap tracks the player. Returns base unchanged
// when the world/room is nil, the viewer has no map, or the toggle is
// off; an unplaced current room is silently skipped here (the `map` verb
// reports it explicitly instead).
func AppendMinimap(base string, r *world.Room, viewer Actor, w *world.World) string {
	if w == nil || r == nil {
		return base
	}
	mv, ok := viewer.(MapViewer)
	if !ok || !mv.MinimapEnabled() {
		return base
	}
	win, err := w.LocalWindow(r.ID, defaultMinimapRadius)
	if err != nil {
		return base
	}
	grid, ok := renderFramedMinimap(win, r.ID, mv.HasVisited)
	if !ok || grid == "" {
		return base
	}
	// Beside the room view, not below it (player-maps §4): the room body
	// wraps into a left column and the bordered minimap rides top-right.
	return joinBeside(base, grid, defaultRoomColumnWidth, minimapGap)
}

// MinimapHandler toggles the calling player's active-minimap preference
// (player-maps §4). `minimap` flips it; `minimap on|off` sets it. A
// normal player preference — not role-gated.
func MinimapHandler(ctx context.Context, c *Context) error {
	mv, ok := c.Actor.(MapViewer)
	if !ok {
		return c.Actor.Write(ctx, "Maps are not available.")
	}
	want := !mv.MinimapEnabled()
	if len(c.Args) > 0 {
		switch strings.ToLower(c.Args[0]) {
		case "on":
			want = true
		case "off":
			want = false
		default:
			return c.Actor.Write(ctx, "Usage: minimap [on|off]")
		}
	}
	mv.SetMinimapEnabled(want)
	if want {
		return c.Actor.Write(ctx, "Minimap ON.")
	}
	return c.Actor.Write(ctx, "Minimap OFF.")
}

// MapHandler renders the full discovered map of the player's current
// area on demand (player-maps §5): every visited room in the area,
// fog-filtered and centred on the player.
func MapHandler(ctx context.Context, c *Context) error {
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You are nowhere to be mapped.")
	}
	mv, ok := c.Actor.(MapViewer)
	if !ok || c.World == nil {
		return c.Actor.Write(ctx, "Maps are not available.")
	}
	win, err := c.World.LocalWindow(room.ID, -1) // unbounded = whole area
	if err != nil {
		return c.Actor.Write(ctx, "You cannot map here.")
	}
	grid, ok := renderLocalMap(win, room.ID, mv.HasVisited)
	if !ok {
		return c.Actor.Write(ctx, "You cannot map this area from here.")
	}
	name := string(room.AreaID)
	if a, err := c.World.Area(room.AreaID); err == nil && a.Name != "" {
		name = a.Name
	}
	return c.Actor.Write(ctx, fmt.Sprintf("<title>Map of %s</title>\n%s\n\n%s", name, grid, mapLegend()))
}

// mapLegend explains the map glyphs (player-maps §6.2): the viewer
// marker, the connectors, the terrain glyphs (listed from terrainGlyph
// so the legend can't drift from the renderer), and the stub convention.
func mapLegend() string {
	terrains := make([]string, 0, len(terrainGlyph))
	for t := range terrainGlyph {
		terrains = append(terrains, t)
	}
	sort.Strings(terrains)
	parts := make([]string, 0, len(terrains))
	for _, t := range terrains {
		parts = append(parts, string(terrainGlyph[t])+" "+t)
	}
	var b strings.Builder
	b.WriteString("<subtle>Legend:</subtle>  <highlight>@</highlight> you   <subtle>-</subtle> <subtle>|</subtle> passages\n")
	b.WriteString("<subtle>Terrain:</subtle>  " + strings.Join(parts, "   ") + "\n")
	b.WriteString("<subtle>A passage that leads nowhere on the map is an exit you haven't explored yet.</subtle>")
	return b.String()
}

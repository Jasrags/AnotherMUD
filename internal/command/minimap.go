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
	// MinimapSize is the persisted size preset (player-maps §4):
	// "auto"/"small"/"medium"/"large". "auto" (the default) scales the
	// radius to the client's terminal width.
	MinimapSize() string
	SetMinimapSize(string)
}

// widthViewer is the optional capability a MapViewer may also satisfy to
// report its client's terminal width (RFC 1073 NAWS), letting the room
// body widen to fill a large window instead of wrapping to the fixed
// fallback column. A viewer that doesn't implement it (or reports 0) is
// treated as unknown-width and keeps the narrow default.
type widthViewer interface {
	TerminalWidth() int
}

// viewerTerminalWidth reports v's terminal width, or 0 when the viewer
// doesn't expose one.
func viewerTerminalWidth(v Actor) int {
	if wv, ok := v.(widthViewer); ok {
		return wv.TerminalWidth()
	}
	return 0
}

// Active-minimap window radii (player-maps §4, §10 policy): the step
// radius bounds the fog-of-war window the bordered widget shows. The
// `map` verb uses an unbounded radius instead. The three manual presets
// ladder small→large; `auto` (the default) picks among them by terminal
// width. defaultMinimapRadius is the fallback when the terminal width is
// unknown (no NAWS) — it matches the historical fixed radius so a client
// that reports no size sees exactly the old behavior.
const (
	minimapRadiusSmall   = 2
	minimapRadiusMedium  = 3
	minimapRadiusLarge   = 4
	defaultMinimapRadius = minimapRadiusSmall
)

// Auto-mode terminal-width breakpoints (columns): below autoWidthMedium
// the map is small, below autoWidthLarge it is medium, at or above it is
// large. An unknown width (0) falls below both → small, preserving the
// historical default for clients that don't report a size.
const (
	autoWidthMedium = 100
	autoWidthLarge  = 140
)

// minimapRadiusFor resolves a size preset + terminal width to a window
// radius. Manual presets ignore the width; "auto" (and any unrecognized
// value, including the empty default) scales among the three preset
// radii by the breakpoints above.
func minimapRadiusFor(size string, termWidth int) int {
	switch strings.ToLower(size) {
	case "small":
		return minimapRadiusSmall
	case "medium":
		return minimapRadiusMedium
	case "large":
		return minimapRadiusLarge
	default: // "auto", "", or anything stale
		switch {
		case termWidth >= autoWidthLarge:
			return minimapRadiusLarge
		case termWidth >= autoWidthMedium:
			return minimapRadiusMedium
		default:
			return minimapRadiusSmall
		}
	}
}

// terrainStyle is a terrain's map presentation: a single visible glyph
// plus the theme color tag that tints it (player-maps §6.2, §10 policy).
// Color is a second channel — the glyph alone distinguishes the terrain
// on a no-color client. Glyphs may repeat across terrains that share a
// color family (grass/forest both green, cave/underground both grey);
// the glyph then carries the distinction.
type terrainStyle struct {
	glyph string
	tag   string
}

// terrainStyles maps a room's terrain to its map glyph + color. Covers
// the engine baseline terrains and the content terrains in play
// (grassland, cave, herb-garden) that previously had no glyph and fell
// back to a bare dot.
var terrainStyles = map[string]terrainStyle{
	"outdoors":    {".", "map.grass"},
	"grassland":   {",", "map.grass"},
	"indoors":     {"o", "map.indoor"},
	"underground": {"%", "map.cave"},
	"cave":        {"%", "map.cave"},
	"water":       {"~", "map.water"},
	"road":        {"=", "map.road"},
	"forest":      {"*", "map.forest"},
	"mountain":    {"^", "map.mountain"},
	"herb-garden": {";", "map.grass"},
}

// defaultTerrainStyle is used for unknown/empty terrain — open ground.
var defaultTerrainStyle = terrainStyle{".", "map.grass"}

// poiMarkers maps a room's derived point-of-interest class (world.Room
// .POI, set at load) to its colored map marker. The marker takes the
// room's cell over the terrain glyph (player-maps §6).
var poiMarkers = map[string]terrainStyle{
	"shop":    {"$", "map.shop"},
	"trainer": {"T", "map.trainer"},
	"inn":     {"+", "map.inn"},
}

// poiCell returns the colored marker for a POI class, or "" when the
// class is empty/unknown (so the caller falls back to terrain).
func poiCell(poi string) string {
	if m, ok := poiMarkers[poi]; ok {
		return "<" + m.tag + ">" + m.glyph + "</" + m.tag + ">"
	}
	return ""
}

func terrainStyleFor(terrain string) terrainStyle {
	if s, ok := terrainStyles[strings.ToLower(strings.TrimSpace(terrain))]; ok {
		return s
	}
	return defaultTerrainStyle
}

// terrainCell is the colored, tagged glyph placed on the map canvas for
// a room's terrain.
func terrainCell(terrain string) string {
	s := terrainStyleFor(terrain)
	return "<" + s.tag + ">" + s.glyph + "</" + s.tag + ">"
}

// glyphFor returns the bare (uncolored) terrain glyph — used by the
// legend, which applies its own color around it.
func glyphFor(terrain string) string {
	return terrainStyleFor(terrain).glyph
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
func renderLocalMap(win world.Window, originID world.RoomID, isVisited func(string) bool, w *world.World) (string, bool) {
	canvas, origin, ok := buildMapCanvas(win, originID, isVisited)
	if !ok {
		return "", false
	}
	out := canvas.render()
	if notes := originNotes(origin, w); notes != "" {
		out += "\n" + notes
	}
	return out, true
}

// renderFramedMinimap is the active-minimap form: the same grid enclosed
// in a border that bounds the fog-of-war window the player can currently
// see (player-maps §4) — so the boxed extent reads at a glance. The
// vertical/keyword-exit notes sit below the box.
func renderFramedMinimap(win world.Window, originID world.RoomID, isVisited func(string) bool, w *world.World, radius int) (string, bool) {
	canvas, origin, ok := buildMapCanvas(win, originID, isVisited)
	if !ok {
		return "", false
	}
	// Fixed viewport: the box is always (2*radius+1) rooms square,
	// padding unexplored space as blank so the chosen size is honoured
	// and the box stays a steady footprint as the player moves (the @
	// stays dead centre). Rooms sit on even coords, so the canvas
	// half-span is twice the step radius.
	out := frameBox(canvas.renderFixed(2 * radius))
	// A1: label the box with the current area name so a "fresh" map after
	// an area crossing reads as "you're somewhere new", not a glitch
	// (player-maps §4). Sits above the box as unobtrusive chrome. Shown
	// only when the area resolves to a real name — a nil world or an
	// unnamed area leaves the box unlabelled (the pre-A1 form).
	if w != nil {
		if a, err := w.Area(origin.AreaID); err == nil && a.Name != "" {
			out = "<subtle>" + a.Name + "</subtle>\n" + out
		}
	}
	if notes := originNotes(origin, w); notes != "" {
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
		switch {
		case id == originID:
			canvas.set(col, row, "<highlight>@</highlight>")
		case poiCell(wr.Room.POI) != "":
			// A point of interest (shop/trainer/inn) takes the cell over
			// the terrain glyph so it reads at a glance (player-maps §6).
			canvas.set(col, row, poiCell(wr.Room.POI))
		default:
			canvas.set(col, row, terrainCell(wr.Room.Terrain))
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

// crossAreaNoteDirs is the cardinal scan order for the C1 way-back
// annotation — deterministic so the notes read the same every render.
// Vertical exits keep their plain up/down note (the boundary case for
// them is rare and the note already flags "there's a way the flat map
// can't draw"); only cardinal boundary exits get named.
var crossAreaNoteDirs = []world.Direction{world.DirNorth, world.DirEast, world.DirSouth, world.DirWest}

// originNotes annotates the origin's non-grid exits (player-maps §6.5):
// vertical exits as up/down, cardinal exits that LEAVE the area as
// "<dir> → <neighbour area>" (C1 — the way back/onward the single-area
// map can't draw across a boundary), and keyword exits as portals.
// Empty when none. w may be nil (tests / unwired paths): the cross-area
// clause is then skipped, leaving the pre-C1 behaviour.
func originNotes(origin *world.Room, w *world.World) string {
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
	// C1: cardinal exits whose target sits in a different area — name the
	// neighbour so the player can orient across the boundary the map stops
	// at.
	if w != nil {
		for _, dir := range crossAreaNoteDirs {
			exit, ok := origin.Exits[dir]
			if !ok {
				continue
			}
			target, err := w.Room(exit.Target)
			if err != nil || target.AreaID == origin.AreaID {
				continue
			}
			notes = append(notes, dir.Long()+" → "+MapAreaName(w, target.AreaID))
		}
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
	termWidth := viewerTerminalWidth(viewer)
	mv, isMap := viewer.(MapViewer)
	// No minimap to draw (no world/room, no map capability, or toggled
	// off): still wrap the prose to a readable column so a reflowed
	// description fills a stable width instead of sprawling to the
	// terminal edge. An unknown terminal width (headless/tests) is a
	// no-op, preserving the raw body.
	if w == nil || r == nil || !isMap || !mv.MinimapEnabled() {
		return wrapRoomBody(base, offBodyWidth(termWidth))
	}
	radius := minimapRadiusFor(mv.MinimapSize(), termWidth)
	win, err := w.LocalWindow(r.ID, radius)
	if err != nil {
		return wrapRoomBody(base, offBodyWidth(termWidth))
	}
	grid, ok := renderFramedMinimap(win, r.ID, mv.HasVisited, w, radius)
	if !ok || grid == "" {
		return wrapRoomBody(base, offBodyWidth(termWidth))
	}
	// Beside the room view, not below it (player-maps §4): the room body
	// wraps into a left column and the bordered minimap rides top-right.
	// The column is sized off the minimap's STABLE box width (a function
	// of the size preset alone), NOT the rendered block — the area label
	// and the variable-length way-back note ("(west → The Mountains of
	// Mist)") must not jiggle the column from room to room. The box then
	// sits in a fixed place; a label/note wider than the box overflows
	// to its right under the box, which is fine on a real terminal.
	leftWidth := roomColumnWidth(termWidth, minimapBoxWidth(radius))
	return joinBeside(base, grid, leftWidth, minimapGap)
}

// offBodyWidth is the column the room prose wraps to when no minimap is
// drawn: the terminal width, capped at maxRoomColumnWidth so prose stays
// readable, or 0 (no wrap) when the width is unknown.
func offBodyWidth(termWidth int) int {
	if termWidth <= 0 {
		return 0
	}
	if termWidth > maxRoomColumnWidth {
		return maxRoomColumnWidth
	}
	return termWidth
}

// wrapRoomBody word-wraps each line of a room view to width, re-flowing a
// reflowed (single-line) description into clean column-width lines while
// leaving the short structural lines (name, items, exits) untouched.
// width <= 0 returns the body unchanged.
func wrapRoomBody(s string, width int) string {
	if width <= 0 {
		return s
	}
	var lines []string
	for _, ln := range strings.Split(s, "\n") {
		lines = append(lines, wrapMarkupLine(ln, width)...)
	}
	return strings.Join(lines, "\n")
}

// minimapBoxWidth is the visible width of the framed minimap for a given
// step radius: the fixed viewport renders (4*radius+1) content columns,
// and frameBox adds a one-column border plus a space of padding on each
// side. Deterministic from the radius, so the room column it sizes is
// stable across rooms and notes (player-maps §4).
func minimapBoxWidth(radius int) int {
	return 4*radius + 5
}

// MinimapHandler manages the calling player's active-minimap preference
// (player-maps §4). `minimap` flips visibility; `minimap on|off` sets
// it; `minimap auto|small|medium|large` sets the window size (auto
// scales the radius to the terminal width). A normal player preference
// — not role-gated.
func MinimapHandler(ctx context.Context, c *Context) error {
	mv, ok := c.Actor.(MapViewer)
	if !ok {
		return c.Actor.Write(ctx, "Maps are not available.")
	}
	if len(c.Args) > 0 {
		arg := strings.ToLower(c.Args[0])
		switch arg {
		case "on":
			mv.SetMinimapEnabled(true)
			return c.Actor.Write(ctx, "Minimap ON.")
		case "off":
			mv.SetMinimapEnabled(false)
			return c.Actor.Write(ctx, "Minimap OFF.")
		case "auto", "small", "medium", "large":
			mv.SetMinimapSize(arg)
			msg := fmt.Sprintf("Minimap size set to %s.", arg)
			if !mv.MinimapEnabled() {
				msg += " (Minimap is OFF — type 'minimap on' to show it.)"
			}
			return c.Actor.Write(ctx, msg)
		default:
			return c.Actor.Write(ctx, "Usage: minimap [on|off|auto|small|medium|large]")
		}
	}
	// No argument: flip visibility.
	want := !mv.MinimapEnabled()
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
	grid, ok := renderLocalMap(win, room.ID, mv.HasVisited, c.World)
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
// legendTerrains is the curated terrain key for the map legend — one
// entry per distinct glyph/meaning, ordered for reading (so the legend
// doesn't list near-duplicates like outdoors+grassland or cave+
// underground).
var legendTerrains = []struct{ terrain, label string }{
	{"grassland", "grass"},
	{"forest", "forest"},
	{"mountain", "mtn"},
	{"water", "water"},
	{"road", "road"},
	{"indoors", "indoor"},
	{"cave", "cave"},
}

func mapLegend() string {
	terr := make([]string, 0, len(legendTerrains))
	for _, t := range legendTerrains {
		terr = append(terr, terrainCell(t.terrain)+" "+t.label)
	}
	var b strings.Builder
	b.WriteString("<subtle>Legend:</subtle>  <highlight>@</highlight> you   <subtle>-</subtle> <subtle>|</subtle> passages\n")
	b.WriteString("<subtle>Terrain:</subtle>  " + strings.Join(terr, "   ") + "\n")
	b.WriteString("<subtle>A passage that leads nowhere on the map is an exit you haven't explored yet.</subtle>")
	return b.String()
}

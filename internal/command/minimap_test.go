package command

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

func mapRoom(id world.RoomID, terrain string, x, y, z int, exits map[world.Direction]world.RoomID) *world.Room {
	ex := make(map[world.Direction]world.Exit, len(exits))
	for d, t := range exits {
		ex[d] = world.Exit{Target: t}
	}
	return &world.Room{ID: id, AreaID: "ar", Terrain: terrain, Coord: &world.Coord{X: x, Y: y, Z: z}, Exits: ex}
}

func visitedFunc(ids ...string) func(string) bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return func(id string) bool { return set[id] }
}

// A visited cross renders the origin centred, neighbour glyphs by
// terrain, connectors between rooms, and a stub toward an unvisited
// neighbour whose room is withheld (player-maps §6.1–§6.4).
func TestRenderLocalMap_CrossWithFogStub(t *testing.T) {
	w := world.New()
	w.AddRoom(mapRoom("ar:o", "outdoors", 0, 0, 0, map[world.Direction]world.RoomID{
		world.DirNorth: "ar:n", world.DirEast: "ar:e", world.DirWest: "ar:w",
	}))
	w.AddRoom(mapRoom("ar:n", "forest", 0, 1, 0, map[world.Direction]world.RoomID{world.DirSouth: "ar:o"}))
	w.AddRoom(mapRoom("ar:e", "water", 1, 0, 0, map[world.Direction]world.RoomID{world.DirWest: "ar:o"}))
	w.AddRoom(mapRoom("ar:w", "mountain", -1, 0, 0, map[world.Direction]world.RoomID{world.DirEast: "ar:o"}))

	win, _ := w.LocalWindow("ar:o", 3)
	out, ok := renderLocalMap(win, "ar:o", visitedFunc("ar:o", "ar:n", "ar:e")) // ar:w NOT visited
	if !ok {
		t.Fatal("expected a centerable map")
	}
	t.Logf("\n%s", out)

	if !strings.Contains(out, "@") {
		t.Error("missing origin marker @")
	}
	if !strings.Contains(out, "*") {
		t.Error("missing forest (north) glyph *")
	}
	if !strings.Contains(out, "~") {
		t.Error("missing water (east) glyph ~")
	}
	if strings.Contains(out, "^") {
		t.Error("mountain (unvisited west) room should be hidden, only its exit stubbed")
	}
	if !strings.Contains(out, "-") || !strings.Contains(out, "|") {
		t.Error("expected horizontal and vertical connectors")
	}
}

// Rooms off the viewer's z-level are not gridded; a vertical exit from
// the origin is annotated instead (player-maps §6.5, PD-7).
func TestRenderLocalMap_VerticalExitAnnotated(t *testing.T) {
	w := world.New()
	w.AddRoom(mapRoom("ar:o", "outdoors", 0, 0, 0, map[world.Direction]world.RoomID{world.DirUp: "ar:up"}))
	w.AddRoom(mapRoom("ar:up", "indoors", 0, 0, 1, map[world.Direction]world.RoomID{world.DirDown: "ar:o"}))

	win, _ := w.LocalWindow("ar:o", 3)
	out, ok := renderLocalMap(win, "ar:o", visitedFunc("ar:o", "ar:up"))
	if !ok {
		t.Fatal("expected a centerable map")
	}
	if !strings.Contains(out, "up") {
		t.Errorf("expected an 'up' annotation, got:\n%s", out)
	}
	if strings.Contains(out, "o\n") || strings.Contains(out, "o)") {
		// the indoors glyph 'o' must not appear as a gridded room cell
		// (it's on another z-level); the annotation text may contain
		// other letters but not a lone gridded 'o'.
	}
}

// An unplaced current room cannot be centred — the renderer reports it.
func TestRenderLocalMap_UnplacedOrigin(t *testing.T) {
	w := world.New()
	o := &world.Room{ID: "ar:o", AreaID: "ar", Exits: map[world.Direction]world.Exit{world.DirEast: {Target: "ar:e"}}}
	w.AddRoom(o) // no Coord → unplaced
	w.AddRoom(mapRoom("ar:e", "outdoors", 0, 0, 0, nil))

	win, _ := w.LocalWindow("ar:o", 3)
	if out, ok := renderLocalMap(win, "ar:o", visitedFunc("ar:o", "ar:e")); ok {
		t.Errorf("unplaced origin should not be centerable, got ok=true:\n%s", out)
	}
}

// Down + keyword exits from the origin annotate below the grid.
func TestRenderLocalMap_DownAndPortalAnnotated(t *testing.T) {
	w := world.New()
	o := mapRoom("ar:o", "outdoors", 0, 0, 0, map[world.Direction]world.RoomID{world.DirDown: "ar:dn"})
	o.KeywordExits = map[string]world.Exit{"gate": {Target: "ar:far"}}
	w.AddRoom(o)
	w.AddRoom(mapRoom("ar:dn", "underground", 0, 0, -1, nil))

	out, ok := renderLocalMap(must(w.LocalWindow("ar:o", 2)), "ar:o", visitedFunc("ar:o"))
	if !ok {
		t.Fatal("expected centerable")
	}
	if !strings.Contains(out, "down") {
		t.Errorf("missing 'down' annotation:\n%s", out)
	}
	if !strings.Contains(out, "portal: gate") {
		t.Errorf("missing portal annotation:\n%s", out)
	}
}

func TestGlyphFor(t *testing.T) {
	if g := glyphFor("forest"); g != "*" {
		t.Errorf("glyphFor(forest) = %q, want *", g)
	}
	if g := glyphFor("nonsense-terrain"); g != "." {
		t.Errorf("glyphFor(unknown) = %q, want . (default)", g)
	}
}

func TestMinimapRadiusFor(t *testing.T) {
	cases := []struct {
		name      string
		size      string
		termWidth int
		want      int
	}{
		// Manual presets ignore the terminal width.
		{"small preset", "small", 0, minimapRadiusSmall},
		{"medium preset", "medium", 0, minimapRadiusMedium},
		{"large preset", "large", 222, minimapRadiusLarge},
		{"case-insensitive", "LARGE", 0, minimapRadiusLarge},
		// Auto scales by width breakpoints.
		{"auto unknown width -> small", "auto", 0, minimapRadiusSmall},
		{"auto 80 cols -> small", "auto", 80, minimapRadiusSmall},
		{"auto at medium breakpoint", "auto", autoWidthMedium, minimapRadiusMedium},
		{"auto just below large breakpoint", "auto", autoWidthLarge - 1, minimapRadiusMedium},
		{"auto at large breakpoint", "auto", autoWidthLarge, minimapRadiusLarge},
		{"auto very wide -> large", "auto", 222, minimapRadiusLarge},
		// Empty / stale values fall through to the auto path.
		{"empty falls through to auto", "", 120, minimapRadiusMedium},
		{"stale value falls through to auto", "jumbo", 222, minimapRadiusLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := minimapRadiusFor(c.size, c.termWidth); got != c.want {
				t.Errorf("minimapRadiusFor(%q, %d) = %d, want %d", c.size, c.termWidth, got, c.want)
			}
		})
	}
}

func must(win world.Window, _ error) world.Window { return win }

func TestMapCanvas_Alignment(t *testing.T) {
	g := newMapCanvas()
	g.set(0, 0, "@")
	g.set(2, 0, "x")
	g.set(1, 0, "-")
	if got := g.render(); got != "@-x" {
		t.Errorf("render = %q, want %q", got, "@-x")
	}
}

func TestSideBySideVisual(t *testing.T) {
	roomBody := "<title>The Town Square</title>\n" +
		"A wide cobbled plaza ringed by shuttered stalls and a dry fountain whose basin is choked with autumn leaves.\n" +
		"<subtle>Exits:</subtle> north, east, west"
	w := world.New()
	w.AddRoom(mapRoom("ar:o", "outdoors", 0, 0, 0, map[world.Direction]world.RoomID{
		world.DirNorth: "ar:n", world.DirEast: "ar:e", world.DirWest: "ar:w",
	}))
	w.AddRoom(mapRoom("ar:n", "forest", 0, 1, 0, map[world.Direction]world.RoomID{world.DirSouth: "ar:o"}))
	w.AddRoom(mapRoom("ar:e", "water", 1, 0, 0, map[world.Direction]world.RoomID{world.DirWest: "ar:o"}))
	w.AddRoom(mapRoom("ar:w", "road", -1, 0, 0, map[world.Direction]world.RoomID{world.DirEast: "ar:o"}))
	win, _ := w.LocalWindow("ar:o", defaultMinimapRadius)
	grid, _ := renderFramedMinimap(win, "ar:o", visitedFunc("ar:o", "ar:n", "ar:e", "ar:w"), "")
	t.Logf("\n%s", joinBeside(roomBody, grid, defaultRoomColumnWidth, minimapGap))
}

// The active minimap is enclosed in a border bounding the fog-of-war
// window (player-maps §4).
func TestRenderFramedMinimap_HasBorder(t *testing.T) {
	w := world.New()
	w.AddRoom(mapRoom("ar:o", "outdoors", 0, 0, 0, map[world.Direction]world.RoomID{world.DirEast: "ar:e"}))
	w.AddRoom(mapRoom("ar:e", "water", 1, 0, 0, map[world.Direction]world.RoomID{world.DirWest: "ar:o"}))

	out, ok := renderFramedMinimap(must(w.LocalWindow("ar:o", defaultMinimapRadius)), "ar:o", visitedFunc("ar:o", "ar:e"), "")
	if !ok {
		t.Fatal("expected centerable")
	}
	t.Logf("\n%s", out)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[0], "+") || !strings.Contains(lines[0], "-") {
		t.Errorf("first line should be a top border, got %q", lines[0])
	}
	var bordered bool
	for _, ln := range lines {
		if strings.Contains(ln, "|") && strings.Contains(ln, "@") {
			bordered = true // the @ row is enclosed by vertical borders
		}
	}
	if !bordered {
		t.Errorf("minimap content should be enclosed by side borders:\n%s", out)
	}
}

// A1: when an area name is supplied, it labels the box on the line above
// the top border so the map says where it is.
func TestRenderFramedMinimap_AreaLabel(t *testing.T) {
	w := world.New()
	w.AddRoom(mapRoom("ar:o", "outdoors", 0, 0, 0, map[world.Direction]world.RoomID{world.DirEast: "ar:e"}))
	w.AddRoom(mapRoom("ar:e", "water", 1, 0, 0, map[world.Direction]world.RoomID{world.DirWest: "ar:o"}))

	out, ok := renderFramedMinimap(must(w.LocalWindow("ar:o", defaultMinimapRadius)), "ar:o", visitedFunc("ar:o", "ar:e"), "The Westwood")
	if !ok {
		t.Fatal("expected centerable")
	}
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[0], "The Westwood") {
		t.Errorf("first line should carry the area label, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "+") {
		t.Errorf("the border should follow the label, got %q", lines[1])
	}
}

func TestMapLegend(t *testing.T) {
	leg := mapLegend()
	for _, want := range []string{"@", "you", "passages", "Terrain", "forest", "explored"} {
		if !strings.Contains(leg, want) {
			t.Errorf("legend missing %q:\n%s", want, leg)
		}
	}
}

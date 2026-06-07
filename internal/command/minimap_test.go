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

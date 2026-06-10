package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// mapActor extends the shared testActor with the MapViewer capability:
// a fog-of-war visited set + the persisted minimap toggle.
type mapActor struct {
	*testActor
	visited     map[string]bool
	minimap     bool
	minimapSize string
	termWidth   int
}

// TerminalWidth makes mapActor satisfy the width-aware viewer capability
// so AppendMinimap's column sizing can be exercised; 0 means "unknown".
func (a *mapActor) TerminalWidth() int { return a.termWidth }

func (a *mapActor) HasVisited(id string) bool { return a.visited[id] }
func (a *mapActor) VisitedRooms() []string {
	out := make([]string, 0, len(a.visited))
	for id := range a.visited {
		out = append(out, id)
	}
	return out
}
func (a *mapActor) MinimapEnabled() bool     { return a.minimap }
func (a *mapActor) SetMinimapEnabled(v bool) { a.minimap = v }
func (a *mapActor) MinimapSize() string {
	if a.minimapSize == "" {
		return "auto"
	}
	return a.minimapSize
}
func (a *mapActor) SetMinimapSize(v string) {
	if v == "auto" {
		v = "" // mirror connActor: "" is the canonical on-disk form of auto
	}
	a.minimapSize = v
}

func mapWorld(t *testing.T) (*world.World, *world.Room) {
	t.Helper()
	w := world.New()
	w.AddArea(&world.Area{ID: "ar", Name: "The Hollow"})
	o := &world.Room{ID: "ar:o", AreaID: "ar", Coord: &world.Coord{X: 0, Y: 0, Z: 0},
		Exits: map[world.Direction]world.Exit{world.DirEast: {Target: "ar:e"}}}
	e := &world.Room{ID: "ar:e", AreaID: "ar", Terrain: "water", Coord: &world.Coord{X: 1, Y: 0, Z: 0},
		Exits: map[world.Direction]world.Exit{world.DirWest: {Target: "ar:o"}}}
	w.AddRoom(o)
	w.AddRoom(e)
	return w, o
}

func TestMinimapHandler_Toggles(t *testing.T) {
	a := &mapActor{testActor: newTestActor(nil)}

	if err := command.MinimapHandler(context.Background(), &command.Context{Actor: a}); err != nil {
		t.Fatalf("MinimapHandler: %v", err)
	}
	if !a.MinimapEnabled() || !strings.Contains(a.lastLine(), "ON") {
		t.Errorf("bare toggle should enable; got enabled=%v line=%q", a.MinimapEnabled(), a.lastLine())
	}
	if err := command.MinimapHandler(context.Background(), &command.Context{Actor: a, Args: []string{"off"}}); err != nil {
		t.Fatalf("MinimapHandler off: %v", err)
	}
	if a.MinimapEnabled() {
		t.Error("`minimap off` should disable")
	}
}

func TestMinimapHandler_SetsSize(t *testing.T) {
	a := &mapActor{testActor: newTestActor(nil), minimap: true}
	for _, size := range []string{"auto", "small", "medium", "large"} {
		if err := command.MinimapHandler(context.Background(), &command.Context{Actor: a, Args: []string{size}}); err != nil {
			t.Fatalf("minimap %s: %v", size, err)
		}
		if a.MinimapSize() != size {
			t.Errorf("after `minimap %s`, size = %q, want %q", size, a.MinimapSize(), size)
		}
		if !strings.Contains(a.lastLine(), size) {
			t.Errorf("`minimap %s` should confirm the size; got %q", size, a.lastLine())
		}
	}
	// Setting size must not flip visibility.
	if !a.MinimapEnabled() {
		t.Error("setting size disabled the minimap; it should leave visibility untouched")
	}
}

func TestMinimapHandler_SizeWhileOffHints(t *testing.T) {
	a := &mapActor{testActor: newTestActor(nil), minimap: false}
	if err := command.MinimapHandler(context.Background(), &command.Context{Actor: a, Args: []string{"large"}}); err != nil {
		t.Fatalf("minimap large: %v", err)
	}
	if a.MinimapSize() != "large" {
		t.Errorf("size = %q, want large", a.MinimapSize())
	}
	if !strings.Contains(a.lastLine(), "OFF") {
		t.Errorf("setting size while off should hint the minimap is OFF; got %q", a.lastLine())
	}
}

func TestMinimapHandler_BadArg(t *testing.T) {
	a := &mapActor{testActor: newTestActor(nil)}
	if err := command.MinimapHandler(context.Background(), &command.Context{Actor: a, Args: []string{"huge"}}); err != nil {
		t.Fatalf("minimap huge: %v", err)
	}
	if !strings.Contains(a.lastLine(), "Usage") {
		t.Errorf("unknown arg should print usage; got %q", a.lastLine())
	}
	// A bad arg must not change either preference.
	if a.MinimapEnabled() || a.minimapSize != "" {
		t.Errorf("bad arg mutated state: enabled=%v size=%q", a.MinimapEnabled(), a.minimapSize)
	}
}

func TestMapHandler_RendersArea(t *testing.T) {
	w, o := mapWorld(t)
	a := &mapActor{testActor: newTestActor(o), visited: map[string]bool{"ar:o": true, "ar:e": true}}

	if err := command.MapHandler(context.Background(), &command.Context{Actor: a, World: w}); err != nil {
		t.Fatalf("MapHandler: %v", err)
	}
	out := a.lastLine()
	if !strings.Contains(out, "Map of The Hollow") {
		t.Errorf("map header missing area name:\n%s", out)
	}
	if !strings.Contains(out, "@") || !strings.Contains(out, "~") {
		t.Errorf("map missing origin/neighbour glyphs:\n%s", out)
	}
}

// MapHandler from an unplaced room reports it cannot map (not a broken
// grid); an actor with no map capability gets "not available".
func TestMapHandler_EdgeCases(t *testing.T) {
	w := world.New()
	w.AddArea(&world.Area{ID: "ar", Name: "The Hollow"})
	unplaced := &world.Room{ID: "ar:u", AreaID: "ar"} // no Coord
	w.AddRoom(unplaced)

	a := &mapActor{testActor: newTestActor(unplaced), visited: map[string]bool{"ar:u": true}}
	if err := command.MapHandler(context.Background(), &command.Context{Actor: a, World: w}); err != nil {
		t.Fatalf("MapHandler: %v", err)
	}
	if !strings.Contains(a.lastLine(), "cannot map") {
		t.Errorf("unplaced room should report it cannot map, got %q", a.lastLine())
	}

	// A plain actor (no MapViewer) gets the unavailable message.
	plain := newTestActor(unplaced)
	if err := command.MapHandler(context.Background(), &command.Context{Actor: plain, World: w}); err != nil {
		t.Fatalf("MapHandler (plain): %v", err)
	}
	if !strings.Contains(plain.lastLine(), "not available") {
		t.Errorf("non-map actor should get 'not available', got %q", plain.lastLine())
	}
}

func TestMinimapHandler_BadArgAndNoCapability(t *testing.T) {
	a := &mapActor{testActor: newTestActor(nil)}
	if err := command.MinimapHandler(context.Background(), &command.Context{Actor: a, Args: []string{"sideways"}}); err != nil {
		t.Fatalf("MinimapHandler: %v", err)
	}
	if !strings.Contains(a.lastLine(), "Usage") {
		t.Errorf("bad arg should show usage, got %q", a.lastLine())
	}

	plain := newTestActor(nil)
	if err := command.MinimapHandler(context.Background(), &command.Context{Actor: plain}); err != nil {
		t.Fatalf("MinimapHandler (plain): %v", err)
	}
	if !strings.Contains(plain.lastLine(), "not available") {
		t.Errorf("non-map actor should get 'not available', got %q", plain.lastLine())
	}
}

func TestAppendMinimap_GatedOnToggle(t *testing.T) {
	w, o := mapWorld(t)
	visited := map[string]bool{"ar:o": true, "ar:e": true}

	off := &mapActor{testActor: newTestActor(o), visited: visited, minimap: false}
	if got := command.AppendMinimap("ROOM", o, off, w); got != "ROOM" {
		t.Errorf("toggle off should not append a minimap, got:\n%s", got)
	}

	on := &mapActor{testActor: newTestActor(o), visited: visited, minimap: true}
	got := command.AppendMinimap("ROOM", o, on, w)
	// Beside, not below: the room text leads the first row and the
	// minimap block sits to its right — led by the A1 area label
	// ("The Hollow"), with the bordered box following below it.
	firstLine := strings.SplitN(got, "\n", 2)[0]
	if !strings.HasPrefix(firstLine, "ROOM") {
		t.Errorf("room text should lead the row, got first line:\n%s", firstLine)
	}
	if !strings.Contains(firstLine, "The Hollow") {
		t.Errorf("the area label should ride top-right on the first row, got:\n%s", got)
	}
	if !strings.Contains(got, "+") {
		t.Errorf("the bordered minimap box should still be present, got:\n%s", got)
	}
	if !strings.Contains(got, "@") {
		t.Errorf("minimap should contain the viewer marker, got:\n%s", got)
	}
}

// The minimap's left column must NOT move when only the way-back note
// length changes — the box sits in a stable place regardless of the
// neighbour area's name (the bug where a longer note shifted the map).
func TestAppendMinimap_StableColumnAcrossNoteLength(t *testing.T) {
	build := func(neighbourName string) string {
		w := world.New()
		w.AddArea(&world.Area{ID: "wild", Name: "the Wild"})
		w.AddArea(&world.Area{ID: "nb", Name: neighbourName})
		o := &world.Room{ID: "wild:o", AreaID: "wild", Terrain: "forest",
			Exits: map[world.Direction]world.Exit{world.DirWest: {Target: "nb:x"}}}
		w.AddRoom(o)
		w.AddRoom(&world.Room{ID: "nb:x", AreaID: "nb"})
		a := &mapActor{testActor: newTestActor(o), visited: map[string]bool{"wild:o": true}, minimap: true, minimapSize: "large", termWidth: 120}
		return command.AppendMinimap("ROOM BODY", o, a, w)
	}
	// The first '+' on row 0 marks the box's left edge.
	colOf := func(out string) int {
		return strings.Index(strings.SplitN(out, "\n", 2)[0], "+")
	}
	short := colOf(build("Foo"))                             // note "(west → Foo)"
	long := colOf(build("The Mountains of Mist and Beyond")) // a much longer note
	if short != long {
		t.Errorf("map column moved with note length: short col=%d long col=%d", short, long)
	}
}

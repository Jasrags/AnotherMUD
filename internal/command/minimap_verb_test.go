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
	visited map[string]bool
	minimap bool
}

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
	// Beside, not below: the room text and the minimap share the first row.
	firstLine := strings.SplitN(got, "\n", 2)[0]
	if !strings.HasPrefix(firstLine, "ROOM") {
		t.Errorf("room text should lead the row, got first line:\n%s", firstLine)
	}
	if !strings.Contains(firstLine, "@") {
		t.Errorf("minimap should sit to the RIGHT on the same row, got:\n%s", got)
	}
}

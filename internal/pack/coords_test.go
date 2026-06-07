package pack

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// coordPack writes a minimal one-area pack whose rooms are supplied as
// individual YAML bodies keyed by file stem, then returns its root.
func coordPack(t *testing.T, rooms map[string]string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), `
id: town
name: Town
`)
	for stem, body := range rooms {
		writeFile(t, filepath.Join(pack, "rooms", stem+".yaml"), body)
	}
	return root
}

func loadCoordPack(t *testing.T, rooms map[string]string) *world.World {
	t.Helper()
	regs := NewRegistries()
	if err := Load(context.Background(), coordPack(t, rooms), nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return regs.World
}

func roomCoord(t *testing.T, w *world.World, id world.RoomID) *world.Coord {
	t.Helper()
	r, err := w.Room(id)
	if err != nil {
		t.Fatalf("room %q: %v", id, err)
	}
	return r.Coord
}

// Load derives coordinates: a derived room steps from the anchor, and a
// well-formed pin is honored as ground truth seeding the area.
func TestLoad_DerivesCoordinates(t *testing.T) {
	w := loadCoordPack(t, map[string]string{
		"square": `
id: square
area: town
name: The Square
exits:
  north: fountain
`,
		"fountain": `
id: fountain
area: town
name: The Fountain
exits:
  south: square
`,
	})
	// "tapestry-core:fountain" < "tapestry-core:square" lexically, so the
	// fountain is the default anchor at the origin and the square derives
	// south of it (square→north→fountain ⇒ square is one south).
	if c := roomCoord(t, w, "tapestry-core:fountain"); c == nil || *c != (world.Coord{X: 0, Y: 0, Z: 0}) {
		t.Errorf("fountain coord = %v, want (0,0,0)", c)
	}
	if c := roomCoord(t, w, "tapestry-core:square"); c == nil || *c != (world.Coord{X: 0, Y: -1, Z: 0}) {
		t.Errorf("square coord = %v, want (0,-1,0)", c)
	}
}

// A well-formed `coord:` pin places the room exactly and seeds the walk.
func TestLoad_HonorsCoordPin(t *testing.T) {
	w := loadCoordPack(t, map[string]string{
		"vault": `
id: vault
area: town
name: The Vault
coord: {x: 10, y: -3, z: 2}
exits:
  up: loft
`,
		"loft": `
id: loft
area: town
name: The Loft
`,
	})
	if c := roomCoord(t, w, "tapestry-core:vault"); c == nil || *c != (world.Coord{X: 10, Y: -3, Z: 2}) {
		t.Errorf("vault coord = %v, want (10,-3,2)", c)
	}
	if c := roomCoord(t, w, "tapestry-core:loft"); c == nil || *c != (world.Coord{X: 10, Y: -3, Z: 3}) {
		t.Errorf("loft coord = %v, want (10,-3,3) [vault + up]", c)
	}
}

// A malformed pin (missing axis) warns and falls back to derived
// placement; it never aborts the load.
func TestLoad_MalformedPinFallsBack(t *testing.T) {
	w := loadCoordPack(t, map[string]string{
		"square": `
id: square
area: town
name: The Square
coord: {x: 1, y: 2}
`,
	})
	r, err := w.Room("tapestry-core:square")
	if err != nil {
		t.Fatalf("room: %v", err)
	}
	if r.Pin != nil {
		t.Errorf("malformed pin should not attach: Pin = %+v", r.Pin)
	}
	// Falls back to derived placement (sole room → anchor at origin).
	if r.Coord == nil || *r.Coord != (world.Coord{X: 0, Y: 0, Z: 0}) {
		t.Errorf("square coord = %v, want derived (0,0,0)", r.Coord)
	}
}

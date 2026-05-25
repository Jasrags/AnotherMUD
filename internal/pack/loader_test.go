package pack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// writeFile is a tiny test helper that mkdir -p's and writes body.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// minimalCorePack writes a self-contained pack into root and returns
// root. Two areas, two rooms, both directions wired.
func minimalCorePack(t *testing.T) string {
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
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `
id: a
area: town
name: Room A
exits:
  north: b
`)
	writeFile(t, filepath.Join(pack, "rooms/b.yaml"), `
id: b
area: town
name: Room B
exits:
  south: a
`)
	return root
}

func TestLoadHappyPath(t *testing.T) {
	root := minimalCorePack(t)
	regs := NewRegistries()
	w := regs.World
	if err := Load(context.Background(), root, nil, regs, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	a, err := w.Room("tapestry-core:a")
	if err != nil {
		t.Fatalf("room a missing: %v", err)
	}
	if a.AreaID != "tapestry-core:town" {
		t.Errorf("AreaID = %q, want tapestry-core:town", a.AreaID)
	}
	if exit, ok := a.Exits[world.DirNorth]; !ok || exit.Target != "tapestry-core:b" {
		t.Errorf("north exit = %+v, ok=%v", exit, ok)
	}
	if _, err := w.Area("tapestry-core:town"); err != nil {
		t.Errorf("area town missing: %v", err)
	}
}

func TestLoadMissingArea(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/orphan.yaml"), `
id: orphan
area: ghost-area
name: Orphan
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, ErrMissingArea) {
		t.Fatalf("err = %v, want ErrMissingArea", err)
	}
}

func TestLoadDuplicateRoomID(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), "id: a\narea: town\nname: A\n")
	writeFile(t, filepath.Join(pack, "rooms/a2.yaml"), "id: a\narea: town\nname: Dup\n")

	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, world.ErrDuplicateID) {
		t.Fatalf("err = %v, want world.ErrDuplicateID", err)
	}
}

func TestLoadMissingExitTarget(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `
id: a
area: town
name: A
exits:
  north: nowhere
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, ErrMissingExitRoom) {
		t.Fatalf("err = %v, want ErrMissingExitRoom", err)
	}
}

func TestLoadQualifiedExitTargetCrossPack(t *testing.T) {
	// Two packs: core defines area+rooms, extra adds a room that exits
	// into core via a fully-qualified id ("tapestry-core:a").
	root := t.TempDir()

	core := filepath.Join(root, "core")
	writeFile(t, filepath.Join(core, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(core, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(core, "rooms/a.yaml"), "id: a\narea: town\nname: A\n")

	extra := filepath.Join(root, "extra")
	writeFile(t, filepath.Join(extra, "pack.yaml"), `
name: extra
dependencies:
  tapestry-core: "*"
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(extra, "areas/wild.yaml"), "id: wild\nname: Wild\n")
	writeFile(t, filepath.Join(extra, "rooms/b.yaml"), `
id: b
area: wild
name: B
exits:
  west: "tapestry-core:a"
`)

	regs := NewRegistries()
	w := regs.World
	if err := Load(context.Background(), root, nil, regs, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	b, err := w.Room("extra:b")
	if err != nil {
		t.Fatalf("room b: %v", err)
	}
	if b.Exits[world.DirWest].Target != "tapestry-core:a" {
		t.Errorf("cross-pack exit target = %q", b.Exits[world.DirWest].Target)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/bad.yaml"), "id: [unterminated\n")

	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadBadDirection(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `
id: a
area: town
name: A
exits:
  sideways: a
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadGlobMatchesNothing(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	// No content files at all.
	if err := Load(context.Background(), root, nil, NewRegistries(), nil); err == nil {
		t.Fatal("expected error for empty glob match")
	}
}

func TestLoadGlobPathTraversalRejected(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	// A glob escaping the pack dir must be rejected even if the file
	// would exist — packs may not read host paths outside themselves.
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: ["../escape/*.yaml"]
  rooms: []
`)
	// Place a real file at the escape target so the glob finds something.
	writeFile(t, filepath.Join(root, "escape/town.yaml"), "id: town\nname: Town\n")
	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if err == nil {
		t.Fatal("expected path-traversal rejection, got nil")
	}
}

func TestLoadCrossPackDuplicateIDs(t *testing.T) {
	// Two packs both try to register the area "shared:town" (fully
	// qualified) — second pack must error rather than silently overwrite.
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "a/pack.yaml"), `
name: a
content:
  areas: [areas/*.yaml]
  rooms: []
`)
	writeFile(t, filepath.Join(root, "a/areas/town.yaml"), `id: "shared:town"
name: A's Town
`)
	writeFile(t, filepath.Join(root, "b/pack.yaml"), `
name: b
content:
  areas: [areas/*.yaml]
  rooms: []
`)
	writeFile(t, filepath.Join(root, "b/areas/town.yaml"), `id: "shared:town"
name: B's Town
`)

	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, world.ErrDuplicateID) {
		t.Fatalf("err = %v, want world.ErrDuplicateID", err)
	}
}

func TestLoadMalformedQualifiedID(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	// Whitespace-only namespace half — must be rejected.
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: \"  :town\"\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), "id: a\narea: town\nname: A\n")

	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}

// TestLoadRealCorePack runs the loader against the actual content/core/
// pack shipped in this repo — catches authoring errors in the YAML.
func TestLoadRealCorePack(t *testing.T) {
	// Project root is two levels up from internal/pack/.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Join(cwd, "..", "..")
	contentRoot := filepath.Join(repoRoot, "content")
	if _, err := os.Stat(contentRoot); err != nil {
		t.Skipf("content/ not present at %s: %v", contentRoot, err)
	}

	regs := NewRegistries()
	w := regs.World
	if err := Load(context.Background(), contentRoot, nil, regs, nil); err != nil {
		t.Fatalf("Load(real core pack): %v", err)
	}

	wantRooms := []world.RoomID{
		"tapestry-core:town-square",
		"tapestry-core:forge",
		"tapestry-core:market",
		"tapestry-core:village-gate",
	}
	for _, id := range wantRooms {
		if _, err := w.Room(id); err != nil {
			t.Errorf("missing room %q: %v", id, err)
		}
	}
	wantAreas := []world.AreaID{"tapestry-core:town", "tapestry-core:wilderness"}
	for _, id := range wantAreas {
		if _, err := w.Area(id); err != nil {
			t.Errorf("missing area %q: %v", id, err)
		}
	}
	wantItems := []item.TemplateID{
		"tapestry-core:short-sword",
		"tapestry-core:leather-cap",
		"tapestry-core:canvas-sack",
		"tapestry-core:healing-draught",
	}
	for _, id := range wantItems {
		if !regs.Items.Has(id) {
			t.Errorf("missing item template %q", id)
		}
	}

	// Pack-defined slot from content/core/slots/cloak.yaml. Engine
	// baseline slots are NOT registered here (that's main.go's job),
	// so only the pack slot is expected.
	if !regs.Slots.Has("cloak") {
		t.Error("missing pack-defined slot 'cloak'")
	}
	if def, err := regs.Slots.Get("cloak"); err != nil {
		t.Errorf("Get(cloak): %v", err)
	} else if def.Scope != slot.Scope("tapestry-core") {
		t.Errorf("cloak.Scope = %q, want tapestry-core", def.Scope)
	}
}

func TestLoadItemsHappyPath(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), `
id: town
name: Town
`)
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `
id: a
area: town
name: Room A
`)
	writeFile(t, filepath.Join(pack, "items/short-sword.yaml"), `
id: short-sword
name: a short sword
type: item
tags: [weapon, metal]
keywords: [sword, short]
properties:
  damage: 4
modifiers:
  - stat: str
    value: 1
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	tpl, err := regs.Items.Get("tapestry-core:short-sword")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tpl.Name != "a short sword" {
		t.Errorf("Name = %q", tpl.Name)
	}
	if tpl.Type != "item" {
		t.Errorf("Type = %q, want item", tpl.Type)
	}
	if len(tpl.Tags) != 2 || tpl.Tags[0] != "weapon" {
		t.Errorf("Tags = %v", tpl.Tags)
	}
	if len(tpl.Keywords) != 2 || tpl.Keywords[0] != "sword" {
		t.Errorf("Keywords = %v", tpl.Keywords)
	}
	if got := tpl.Properties["damage"]; got != 4 {
		t.Errorf("Properties[damage] = %v (%T), want 4", got, got)
	}
	if len(tpl.Modifiers) != 1 || tpl.Modifiers[0].Stat != "str" || tpl.Modifiers[0].Value != 1 {
		t.Errorf("Modifiers = %+v", tpl.Modifiers)
	}
}

func TestLoadItemsMissingType(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), `
id: town
name: Town
`)
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `
id: a
area: town
name: Room A
`)
	writeFile(t, filepath.Join(pack, "items/broken.yaml"), `
id: broken
name: a broken thing
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadItemsCrossPackCollision(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	writeFile(t, filepath.Join(a, "pack.yaml"), `
name: shared
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(a, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(a, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(a, "items/dup.yaml"), `id: shared:dup
name: from a
type: item`)
	writeFile(t, filepath.Join(b, "pack.yaml"), `
name: other
content:
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(b, "items/dup.yaml"), `id: shared:dup
name: from b
type: item`)

	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, item.ErrDuplicateID) {
		t.Errorf("err = %v, want ErrDuplicateID", err)
	}
}

func TestLoadItemsModifierMissingStat(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), `id: town
name: Town`)
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `id: a
area: town
name: Room A`)
	writeFile(t, filepath.Join(pack, "items/broken.yaml"), `
id: broken
name: a broken thing
type: item
modifiers:
  - value: 2
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadNilRegistriesRejected(t *testing.T) {
	if err := Load(context.Background(), t.TempDir(), nil, nil, nil); err == nil {
		t.Error("Load(nil dst) returned nil, want error")
	}
	if err := Load(context.Background(), t.TempDir(), nil, &Registries{}, nil); err == nil {
		t.Error("Load(&Registries{}) returned nil, want error")
	}
}

func TestLoadSlotsHappyPath(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  slots: [slots/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), `id: town
name: Town`)
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `id: a
area: town
name: Room A`)
	writeFile(t, filepath.Join(pack, "slots/cloak.yaml"), `
name: cloak
label: worn as cloak
max: 1
`)
	writeFile(t, filepath.Join(pack, "slots/finger.yaml"), `
name: finger
label: worn on finger
max: 2
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	cloak, err := regs.Slots.Get("cloak")
	if err != nil {
		t.Fatalf("Get(cloak): %v", err)
	}
	if cloak.Max != 1 || cloak.Scope != slot.Scope("tapestry-core") {
		t.Errorf("cloak = %+v", cloak)
	}
	finger, _ := regs.Slots.Get("finger")
	if finger.Max != 2 {
		t.Errorf("finger.Max = %d, want 2", finger.Max)
	}
}

func TestLoadSlotsInvalidName(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  slots: [slots/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), `id: town
name: Town`)
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `id: a
area: town
name: Room A`)
	writeFile(t, filepath.Join(pack, "slots/bad.yaml"), `
name: Left-Hand
label: invalid
max: 1
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, slot.ErrInvalidName) {
		t.Errorf("err = %v, want ErrInvalidName", err)
	}
}

func TestLoadSlotsMissingMax(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  slots: [slots/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), `id: town
name: Town`)
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `id: a
area: town
name: Room A`)
	writeFile(t, filepath.Join(pack, "slots/bad.yaml"), `
name: belt
label: worn at waist
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadSlotsCollidesWithEngineBaseline(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  slots: [slots/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), `id: town
name: Town`)
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `id: a
area: town
name: Room A`)
	writeFile(t, filepath.Join(pack, "slots/wield.yaml"), `
name: wield
label: my own wield
max: 1
`)
	regs := NewRegistries()
	// Pre-register engine baseline so the pack collides.
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	err := Load(context.Background(), root, nil, regs, nil)
	if !errors.Is(err, slot.ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

func TestLoadSlotsCrossPackCollision(t *testing.T) {
	// Two packs both registering "belt" must collide via the registry,
	// just like cross-pack item id collisions.
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	writeFile(t, filepath.Join(a, "pack.yaml"), `
name: pack-a
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  slots: [slots/*.yaml]
`)
	writeFile(t, filepath.Join(a, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(a, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(a, "slots/belt.yaml"), `name: belt
label: worn at waist
max: 1`)

	writeFile(t, filepath.Join(b, "pack.yaml"), `
name: pack-b
content:
  slots: [slots/*.yaml]
`)
	writeFile(t, filepath.Join(b, "slots/belt.yaml"), `name: belt
label: also a belt
max: 1`)

	err := Load(context.Background(), root, nil, NewRegistries(), nil)
	if !errors.Is(err, slot.ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

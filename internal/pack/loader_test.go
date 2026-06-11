package pack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
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
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
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

func TestLoadRoomTagsAndHealingRate(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/hub.yaml"), `
id: hub
area: town
name: Hub
healing_rate: 2
tags: [safe-room, safe]
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := regs.World.Room("tapestry-core:hub")
	if err != nil {
		t.Fatalf("room hub missing: %v", err)
	}
	if r.HealingRate != 2 {
		t.Errorf("HealingRate = %d, want 2", r.HealingRate)
	}
	if !r.HasTag("safe-room") || !r.HasTag("safe") {
		t.Errorf("room tags = %v, want safe-room + safe", r.Tags)
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
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrMissingArea) {
		t.Fatalf("err = %v, want ErrMissingArea", err)
	}
}

// lightFloorPack writes a one-area, two-room pack where the area
// declares the given light_floor and room "b" optionally declares its
// own. Returns root.
func lightFloorPack(t *testing.T, areaFloor, roomBFloor string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	area := "id: town\nname: Town\n"
	if areaFloor != "" {
		area += "light_floor: " + areaFloor + "\n"
	}
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), area)
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), "id: a\narea: town\nname: Room A\nexits:\n  north: b\n")
	roomB := "id: b\narea: town\nname: Room B\nexits:\n  south: a\n"
	if roomBFloor != "" {
		roomB += "properties:\n  light_floor: " + roomBFloor + "\n"
	}
	writeFile(t, filepath.Join(pack, "rooms/b.yaml"), roomB)
	return root
}

func TestLoadBakesAreaLightFloorOntoRooms(t *testing.T) {
	root := lightFloorPack(t, "dim", "")
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatal(err)
	}
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, id := range []world.RoomID{"tapestry-core:a", "tapestry-core:b"} {
		r, err := regs.World.Room(id)
		if err != nil {
			t.Fatalf("room %s missing: %v", id, err)
		}
		if got, ok := r.PropertyString("light_floor"); !ok || got != "dim" {
			t.Errorf("room %s light_floor = (%q,%v), want (dim,true) baked from area", id, got, ok)
		}
	}
}

func TestLoadRoomLightFloorWinsOverArea(t *testing.T) {
	root := lightFloorPack(t, "dim", "gloom")
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatal(err)
	}
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Room b declared its own gloom floor; the area's dim must not clobber it.
	b, err := regs.World.Room("tapestry-core:b")
	if err != nil {
		t.Fatalf("room b missing: %v", err)
	}
	if got, _ := b.PropertyString("light_floor"); got != "gloom" {
		t.Errorf("room b light_floor = %q, want gloom (room wins over area)", got)
	}
	// Room a had none, so it inherits the area default.
	a, _ := regs.World.Room("tapestry-core:a")
	if got, _ := a.PropertyString("light_floor"); got != "dim" {
		t.Errorf("room a light_floor = %q, want dim (inherited)", got)
	}
}

func TestLoadInvalidAreaLightFloorErrors(t *testing.T) {
	root := lightFloorPack(t, "nonsense", "")
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatal(err)
	}
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent for bad area light_floor", err)
	}
}

func TestPoiFromMobTags(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{"shop wins over trainer", []string{"humanoid", "skill_trainer", "shop"}, "shop"},
		{"trainer only", []string{"humanoid", "skill_trainer"}, "trainer"},
		{"plain npc", []string{"humanoid"}, ""},
		{"shop only", []string{"shop"}, "shop"},
		{"empty", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := poiFromMobTags(c.tags); got != c.want {
				t.Errorf("poiFromMobTags(%v) = %q, want %q", c.tags, got, c.want)
			}
		})
	}
	// Precedence ladder.
	if !(poiRank("shop") > poiRank("trainer") && poiRank("trainer") > poiRank("inn") && poiRank("inn") > poiRank("")) {
		t.Errorf("poiRank ladder broken: shop=%d trainer=%d inn=%d none=%d",
			poiRank("shop"), poiRank("trainer"), poiRank("inn"), poiRank(""))
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

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
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
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
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
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
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

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadBadWeaponDamageDice(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "items/sword.yaml"), `
id: sword
name: a sword
type: item
weapon_damage: "not-dice"
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent for malformed weapon_damage", err)
	}
}

func TestLoadBadNaturalWeaponDice(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  mobs: [mobs/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "mobs/wolf.yaml"), `
id: wolf
name: a wolf
behavior: stationary
natural_weapon:
  name: fangs
  damage: "1dX"
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent for malformed natural_weapon damage", err)
	}
}

func TestLoadValidWeaponDamageParses(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "items/sword.yaml"), `
id: sword
name: a sword
type: item
weapon_damage: "1d8+2"
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	tpl, err := regs.Items.Get("tapestry-core:sword")
	if err != nil {
		t.Fatalf("Get sword: %v", err)
	}
	if tpl.WeaponDamage != "1d8+2" {
		t.Errorf("WeaponDamage = %q, want %q", tpl.WeaponDamage, "1d8+2")
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
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
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
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); err == nil {
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
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
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

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
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

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
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
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("register engine baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("register engine baseline slots: %v", err)
	}
	w := regs.World
	// Select the demo world explicitly (a boot loads ONE world; the content
	// dir holds starter-world + wot with colliding bare biome ids).
	if err := Load(context.Background(), contentRoot, []string{"starter-world"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load(real core pack): %v", err)
	}

	// World content (rooms/areas/items) lives in the starter-world pack
	// since the M0.1 split; the tapestry-core pack is the engine baseline.
	wantRooms := []world.RoomID{
		"starter-world:town-square",
		"starter-world:forge",
		"starter-world:market",
		"starter-world:village-gate",
	}
	for _, id := range wantRooms {
		if _, err := w.Room(id); err != nil {
			t.Errorf("missing room %q: %v", id, err)
		}
	}
	wantAreas := []world.AreaID{"starter-world:town", "starter-world:wilderness"}
	for _, id := range wantAreas {
		if _, err := w.Area(id); err != nil {
			t.Errorf("missing area %q: %v", id, err)
		}
	}
	wantItems := []item.TemplateID{
		"starter-world:short-sword",
		"starter-world:leather-cap",
		"starter-world:canvas-sack",
		"starter-world:healing-draught",
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
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
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
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
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

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
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
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadNilRegistriesRejected(t *testing.T) {
	if err := Load(context.Background(), t.TempDir(), nil, nil, nil, nil, nil); err == nil {
		t.Error("Load(nil dst) returned nil, want error")
	}
	if err := Load(context.Background(), t.TempDir(), nil, &Registries{}, nil, nil, nil); err == nil {
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
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
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
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
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
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
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
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
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

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, slot.ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

func TestLoadTracksHappyPath(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  tracks: [tracks/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "tracks/adventurer.yaml"), `
id: adventurer
name: Adventurer
max_level: 5
xp_table: [0, 0, 100, 300, 600, 1000]
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	td, ok := regs.Tracks.Get("adventurer")
	if !ok {
		t.Fatal("track adventurer not registered")
	}
	if td.MaxLevel != 5 {
		t.Errorf("MaxLevel = %d, want 5", td.MaxLevel)
	}
	if td.GetXpForLevel(3) != 300 {
		t.Errorf("GetXpForLevel(3) = %d, want 300", td.GetXpForLevel(3))
	}
	if td.Pack != "tapestry-core" {
		t.Errorf("Pack = %q, want tapestry-core", td.Pack)
	}
}

func TestLoadTracksRejectsNonMonotonicTable(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  tracks: [tracks/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "tracks/bad.yaml"), `
id: bad
max_level: 3
xp_table: [0, 0, 100, 50]
`)

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadTracksRejectsEmptyTable(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  tracks: [tracks/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "tracks/bad.yaml"), `
id: bad
max_level: 3
`)

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadTracksRejectsTableShorterThanMaxLevel(t *testing.T) {
	// max_level=10 but only 4 thresholds defined — silently halting
	// cascade past level 3 is exactly the authoring footgun M8.2
	// review flagged. Reject at load.
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  tracks: [tracks/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "tracks/short.yaml"), `
id: short
max_level: 10
xp_table: [0, 0, 100, 200]
`)

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadRacesHappyPath(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  races: [races/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "races/human.yaml"), `
id: human
name: Human
category: humanoid
starting_alignment: 0
stat_caps:
  str: 22
  con: 22
cast_cost_modifier: 0
racial_flags:
  - common-tongue
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, ok := regs.Races.Get("human")
	if !ok {
		t.Fatal("race human not registered")
	}
	if r.DisplayName != "Human" || r.Category != "humanoid" {
		t.Errorf("race = %+v", r)
	}
	if r.StatCaps["str"] != 22 {
		t.Errorf("StatCaps[str] = %d, want 22", r.StatCaps["str"])
	}
	if len(r.RacialFlags) != 1 || r.RacialFlags[0] != "common-tongue" {
		t.Errorf("RacialFlags = %v, want [common-tongue]", r.RacialFlags)
	}
	if r.Pack != "tapestry-core" {
		t.Errorf("Pack = %q, want tapestry-core", r.Pack)
	}
}

func TestLoadRacesRejectsEmptyID(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  races: [races/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "races/bad.yaml"), `
name: Bad
`)

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadRacesRejectsNegativeStatCap(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  races: [races/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "races/bad.yaml"), `
id: bad
stat_caps:
  str: -5
`)

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadClassesHappyPath(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  classes: [classes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "classes/fighter.yaml"), `
id: Fighter
name: Fighter
bound_track: adventurer
stat_growth:
  HP_MAX: 1d8
  STR: 1d3
growth_bonuses:
  HP_MAX: CON
trains_per_level: 5
path:
  - level: 1
    ability: basic-strike
  - level: 3
    ability: cleave
  - level: 5
    ability: locked
    unlocked_via: quest:vigil
allowed_categories:
  - humanoid
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c, ok := regs.Classes.Get("FIGHTER")
	if !ok {
		t.Fatal("class fighter not registered (case-insens)")
	}
	if c.ID != "fighter" {
		t.Errorf("ID = %q, want lowercased fighter", c.ID)
	}
	if c.BoundTrack != "adventurer" {
		t.Errorf("BoundTrack = %q", c.BoundTrack)
	}
	if d, ok := c.StatGrowth["hp_max"]; !ok || d.Count != 1 || d.Sides != 8 {
		t.Errorf("StatGrowth[hp_max] = %+v, want 1d8 (stat key lowercased)", d)
	}
	if src, ok := c.GrowthBonuses["hp_max"]; !ok || src != "con" {
		t.Errorf("GrowthBonuses[hp_max] = %q, want con", src)
	}
	if c.TrainsPerLevel != 5 {
		t.Errorf("TrainsPerLevel = %d, want 5", c.TrainsPerLevel)
	}
	if len(c.Path) != 3 {
		t.Fatalf("Path len = %d, want 3 (including unlocked entries)", len(c.Path))
	}
	if c.Path[2].UnlockedVia != "quest:vigil" {
		t.Errorf("path[2].UnlockedVia = %q", c.Path[2].UnlockedVia)
	}
	if c.Pack != "tapestry-core" {
		t.Errorf("Pack = %q", c.Pack)
	}
}

// Weapon-identity §3: a class declares the weapon proficiency tiers and
// categories it grants; both are lowercased at registration.
func TestLoadClasses_ProficiencyGrants(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  classes: [classes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "classes/armsman.yaml"), `
id: armsman
name: Armsman
bound_track: adventurer
proficiency_tiers: [Simple, Martial]
proficiency_categories: [Two-Rivers-Longbow]
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c, ok := regs.Classes.Get("armsman")
	if !ok {
		t.Fatal("class armsman not registered")
	}
	if want := []string{"simple", "martial"}; !slices.Equal(c.ProficiencyTiers, want) {
		t.Errorf("ProficiencyTiers = %v, want %v (lowercased)", c.ProficiencyTiers, want)
	}
	if want := []string{"two-rivers-longbow"}; !slices.Equal(c.ProficiencyCategories, want) {
		t.Errorf("ProficiencyCategories = %v, want %v (lowercased)", c.ProficiencyCategories, want)
	}
}

func TestLoadClasses_SaveProgressions(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  classes: [classes/*.yaml]
`)
	// Mixed-case axis + progression names must lowercase; an omitted axis
	// (will) stays absent (defaults to weak at composition time).
	writeFile(t, filepath.Join(pack, "classes/warder.yaml"), `
id: warder
name: Warder
bound_track: adventurer
save_progressions:
  Fortitude: Strong
  reflex: WEAK
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c, ok := regs.Classes.Get("warder")
	if !ok {
		t.Fatal("class warder not registered")
	}
	if got := c.SaveProgressions[progression.SaveFortitude]; got != progression.SaveStrong {
		t.Errorf("Fortitude = %q, want strong (lowercased)", got)
	}
	if got := c.SaveProgressions[progression.SaveReflex]; got != progression.SaveWeak {
		t.Errorf("Reflex = %q, want weak", got)
	}
	if _, present := c.SaveProgressions[progression.SaveWill]; present {
		t.Error("Will should be absent (defaults to weak at composition)")
	}
}

func TestLoadClassesRejectsBadSaveAxis(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  classes: [classes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "classes/bad.yaml"), `
id: bad
bound_track: adventurer
save_progressions:
  toughness: strong
`)
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); !errors.Is(err, ErrInvalidContent) {
		t.Errorf("want ErrInvalidContent for unknown save axis, got %v", err)
	}
}

func TestLoadClassesRejectsBadSaveProgression(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  classes: [classes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "classes/bad.yaml"), `
id: bad
bound_track: adventurer
save_progressions:
  fortitude: heroic
`)
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); !errors.Is(err, ErrInvalidContent) {
		t.Errorf("want ErrInvalidContent for unknown progression, got %v", err)
	}
}

func TestLoadClassesRejectsMalformedDice(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  classes: [classes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "classes/bad.yaml"), `
id: bad
bound_track: adventurer
stat_growth:
  hp_max: not-a-die
`)

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadClassesRejectsEmptyID(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  classes: [classes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "classes/bad.yaml"), `
name: NoID
`)

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadAbilitiesHappyPath(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  abilities: [abilities/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "abilities/kick.yaml"), `
id: Kick
name: Kick
type: active
category: skill
default_cap: 75
gain_base_chance: 25
gain_failure_multiplier: 0.5
gain_stat: dex
gain_stat_scale: 0.1
`)
	writeFile(t, filepath.Join(pack, "abilities/second-attack.yaml"), `
id: second-attack
type: passive
category: skill
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	a, ok := regs.Abilities.Get("KICK")
	if !ok {
		t.Fatal("ability kick not registered (case-insens)")
	}
	if a.ID != "kick" {
		t.Errorf("ID = %q, want lowercased kick", a.ID)
	}
	if a.DisplayName != "Kick" {
		t.Errorf("DisplayName = %q", a.DisplayName)
	}
	if a.DefaultCap != 75 {
		t.Errorf("DefaultCap = %d, want 75", a.DefaultCap)
	}
	if a.GainStat != "dex" {
		t.Errorf("GainStat = %q, want dex", a.GainStat)
	}
	if a.Pack != "tapestry-core" {
		t.Errorf("Pack = %q", a.Pack)
	}
	// Display falls back to id when name omitted.
	sa, _ := regs.Abilities.Get("second-attack")
	if sa.DisplayName != "second-attack" {
		t.Errorf("fallback DisplayName = %q, want second-attack", sa.DisplayName)
	}
	if sa.Type != "passive" {
		t.Errorf("Type = %q, want passive", sa.Type)
	}
}

func TestLoadAbilitiesDecodesHandlerAndDice(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  abilities: [abilities/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "abilities/kick.yaml"), `
id: kick
type: active
category: skill
handler: Damage
damage: 1d6
`)
	writeFile(t, filepath.Join(pack, "abilities/heal.yaml"), `
id: heal
type: active
category: spell
handler: heal
heal: 2d4
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	kick, _ := regs.Abilities.Get("kick")
	if kick.HandlerToken != "damage" { // lowercased on decode
		t.Errorf("HandlerToken = %q, want damage", kick.HandlerToken)
	}
	if kick.DamageDice != "1d6" {
		t.Errorf("DamageDice = %q, want 1d6", kick.DamageDice)
	}
	heal, _ := regs.Abilities.Get("heal")
	if heal.HandlerToken != "heal" || heal.HealDice != "2d4" {
		t.Errorf("heal decode = token %q heal %q", heal.HandlerToken, heal.HealDice)
	}
}

func TestLoadAbilitiesDecodesHookAndMaxBonus(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  abilities: [abilities/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "abilities/second-attack.yaml"), `
id: second-attack
type: passive
category: skill
hook: Extra_Attack
max_bonus: 3
variance: 100
max_hit_chance: 50
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	sa, _ := regs.Abilities.Get("second-attack")
	if sa.Hook != "extra_attack" { // lowercased on decode
		t.Errorf("Hook = %q, want extra_attack", sa.Hook)
	}
	if sa.MaxBonus != 3 {
		t.Errorf("MaxBonus = %d, want 3", sa.MaxBonus)
	}
	// variance >= 100 ⇒ the §6.1 binary check uses max_hit_chance, so
	// it must decode (a 0 here would make the passive silently never
	// fire — see m9-5-deferred-fixes loader-validation footgun).
	if sa.MaxHitChance != 50 {
		t.Errorf("MaxHitChance = %d, want 50", sa.MaxHitChance)
	}
	// Discoverable by hook.
	if hits := regs.Abilities.ByHook("extra_attack"); len(hits) != 1 || hits[0].ID != "second-attack" {
		t.Errorf("ByHook(extra_attack) did not return the decoded passive")
	}
}

func TestLoadAbilitiesRejectsDeadPassive(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  abilities: [abilities/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	// Passive with variance>=100 and no max_hit_chance ⇒ §6.1 binary
	// check is always 0 ⇒ never fires. Must be rejected at load.
	writeFile(t, filepath.Join(pack, "abilities/dead.yaml"), `
id: dead
type: passive
category: skill
hook: defensive
variance: 100
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent for dead passive", err)
	}
}

func TestLoadAbilitiesRejectsInvalidType(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  abilities: [abilities/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "abilities/bad.yaml"), `
id: bad
type: bogus
category: skill
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadAbilitiesRejectsEmptyID(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  abilities: [abilities/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "abilities/bad.yaml"), `
type: active
category: skill
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoadThemeHappyPath(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  theme: [theme/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "theme/theme.yaml"), `
tags:
  highlight: { fg: bright-yellow }
  danger: { fg: red, bg: black }
  note: { html: "#888888" }
  "  spacey  ": { fg: green }
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	regs.Theme.Compile()

	if !regs.Theme.IsKnown("highlight") {
		t.Error("highlight not registered")
	}
	// Whitespace-padded tag is trimmed at decode so it resolves by its
	// bare name (regression guard for the decodeTheme trim fix).
	if !regs.Theme.IsKnown("spacey") {
		t.Error("padded tag 'spacey' should be trimmed and known")
	}
	pair, ok := regs.Theme.Resolve("danger")
	if !ok || pair.Open != "\x1b[31m\x1b[40m" {
		t.Errorf("danger resolve = %+v ok=%v", pair, ok)
	}
	// html-only tag is known but has no ANSI pair.
	if !regs.Theme.IsKnown("note") {
		t.Error("note should be known")
	}
	if _, ok := regs.Theme.Resolve("note"); ok {
		t.Error("note should not resolve (no fg/bg)")
	}
	if regs.Theme.GetHtmlMap()["note"] != "#888888" {
		t.Error("note html missing")
	}
}

func TestLoadThemeCrossPackOverride(t *testing.T) {
	root := t.TempDir()
	// Two packs; b depends on a so it loads after and overrides the tag.
	writeFile(t, filepath.Join(root, "a/pack.yaml"), `
name: pack-a
content:
  theme: [theme/*.yaml]
`)
	writeFile(t, filepath.Join(root, "a/theme/t.yaml"), `
tags:
  highlight: { fg: red }
`)
	writeFile(t, filepath.Join(root, "b/pack.yaml"), `
name: pack-b
dependencies:
  pack-a: "*"
content:
  theme: [theme/*.yaml]
`)
	writeFile(t, filepath.Join(root, "b/theme/t.yaml"), `
tags:
  highlight: { fg: green }
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	regs.Theme.Compile()
	pair, _ := regs.Theme.Resolve("highlight")
	if pair.Open != "\x1b[32m" {
		t.Errorf("override = %q, want green (pack-b wins)", pair.Open)
	}
}

func TestLoadQuestNamespacingAndDefaults(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  quests: [quests/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "quests/q.yaml"), `
id: gate
name: Gate
giver: master
stages:
  - id: go
    objectives:
      - type: visit
        target: gate-room
      - type: kill
        target: other-pack:boss
reward:
  items: [reward-item]
  recipes: [reward-recipe, other-pack:shared-recipe]
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	d, ok := regs.Quests.Lookup("tapestry-core:gate")
	if !ok {
		t.Fatal("quest not registered under namespaced id")
	}
	if d.Giver != "tapestry-core:master" {
		t.Errorf("giver = %q, want tapestry-core:master", d.Giver)
	}
	if d.Stages[0].Objectives[0].Target != "tapestry-core:gate-room" {
		t.Errorf("visit target = %q", d.Stages[0].Objectives[0].Target)
	}
	// qualified id crosses packs verbatim
	if d.Stages[0].Objectives[1].Target != "other-pack:boss" {
		t.Errorf("kill target = %q, want other-pack:boss", d.Stages[0].Objectives[1].Target)
	}
	if d.Reward.Items[0] != "tapestry-core:reward-item" {
		t.Errorf("reward item = %q", d.Reward.Items[0])
	}
	// recipe rewards qualify against the pack namespace; qualified ids cross packs verbatim
	if len(d.Reward.Recipes) != 2 || d.Reward.Recipes[0] != "tapestry-core:reward-recipe" || d.Reward.Recipes[1] != "other-pack:shared-recipe" {
		t.Errorf("reward recipes = %v, want [tapestry-core:reward-recipe other-pack:shared-recipe]", d.Reward.Recipes)
	}
	// abandonable absent -> defaults true; objective ids normalized
	if !d.Abandonable {
		t.Error("abandonable should default to true")
	}
	if d.Stages[0].Objectives[0].ID != "go-visit-0" {
		t.Errorf("objective id = %q, want go-visit-0", d.Stages[0].Objectives[0].ID)
	}
}

func TestLoadQuestRejectsMissingStages(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  quests: [quests/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/x.yaml"), `id: x
name: X`)
	writeFile(t, filepath.Join(pack, "rooms/r.yaml"), `id: r
area: x
name: R`)
	writeFile(t, filepath.Join(pack, "quests/bad.yaml"), `
id: empty
name: Empty
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for quest with no stages")
	}
}

// TestLoadEffects pins the M14.2 pack loader hook: effects/*.yaml
// in a pack manifest registers progression.EffectTemplate values
// into registries.Effects, addressable by id.
func TestLoadEffects(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  effects: [effects/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "effects/bless.yaml"), `
id: bless
duration: 300
modifiers:
  - stat: hit_mod
    value: 2
`)
	writeFile(t, filepath.Join(pack, "effects/cursed.yaml"), `
id: cursed
duration: -1
flags: [cursed]
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if regs.Effects.Len() != 2 {
		t.Errorf("effects Len = %d, want 2", regs.Effects.Len())
	}
	bless, ok := regs.Effects.Get("bless")
	if !ok || bless.Duration != 300 || len(bless.Modifiers) != 1 {
		t.Errorf("bless = %+v ok=%v", bless, ok)
	}
	cursed, ok := regs.Effects.Get("CURSED") // case-insensitive
	if !ok || cursed.Duration != -1 || len(cursed.Flags) != 1 {
		t.Errorf("cursed = %+v ok=%v", cursed, ok)
	}
}

// TestLoadEffectsRejectsDuplicateID confirms two packs (or one pack
// with two files for the same id) fail the load at the second
// Register call.
func TestLoadEffectsRejectsDuplicateID(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  effects: [effects/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "effects/a.yaml"), "id: dup\nduration: 10\n")
	writeFile(t, filepath.Join(pack, "effects/b.yaml"), "id: dup\nduration: 20\n")

	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if err == nil {
		t.Fatal("duplicate effect id: expected error")
	}
}

// TestLoadRoomProperties_Validated pins the M14.5 room property
// bag: a known property of the right type loads onto world.Room.
func TestLoadRoomProperties_Validated(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/inn.yaml"), `
id: inn
area: town
name: Inn
properties:
  quest_grant: village-welcome
`)

	regs := NewRegistries()
	if err := regs.Properties.RegisterEngine(property.Entry{
		Name: "quest_grant", Type: property.TypeString,
	}); err != nil {
		t.Fatalf("baseline register: %v", err)
	}
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	inn, err := regs.World.Room("tapestry-core:inn")
	if err != nil {
		t.Fatalf("Room: %v", err)
	}
	if s, ok := inn.PropertyString("quest_grant"); !ok || s != "village-welcome" {
		t.Errorf("inn quest_grant = %q ok=%v", s, ok)
	}
}

// TestLoadRoomProperties_UnknownNameRejected pins that an
// unregistered property name fails the load loudly.
func TestLoadRoomProperties_UnknownNameRejected(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/inn.yaml"), `
id: inn
area: town
name: Inn
properties:
  banana_count: 5
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err == nil {
		t.Fatal("unknown property: want error")
	}
}

// TestLoadRoomProperties_TypeMismatchRejected pins that a known
// name but wrong type fails the load with a clear message.
func TestLoadRoomProperties_TypeMismatchRejected(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/inn.yaml"), `
id: inn
area: town
name: Inn
properties:
  quest_grant: 42
`)
	regs := NewRegistries()
	_ = regs.Properties.RegisterEngine(property.Entry{Name: "quest_grant", Type: property.TypeString})
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if err == nil {
		t.Fatal("type mismatch: want error")
	}
}

// TestLoadRoomProperties_EmptyBagOK pins that a room without a
// properties block (or with an empty one) loads without error.
func TestLoadRoomProperties_EmptyBagOK(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/inn.yaml"), "id: inn\narea: town\nname: Inn\n")

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	inn, _ := regs.World.Room("tapestry-core:inn")
	if len(inn.Properties) != 0 {
		t.Errorf("empty bag: got %v", inn.Properties)
	}
}

// TestLoadRoomDoor_HappyPath pins the M15.1b decoder: a doors:
// block on a room file attaches a DoorState to the matching Exit.
func TestLoadRoomDoor_HappyPath(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "items/gate-key.yaml"),
		"id: gate-key\nname: an iron gate key\ntype: item\n")
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: Square
exits:
  north: gate
`)
	writeFile(t, filepath.Join(pack, "rooms/gate.yaml"), `
id: gate
area: town
name: Gate
exits:
  south: square
doors:
  south:
    name: iron gate
    closed: true
    locked: true
    key: gate-key
`)

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	gate, _ := regs.World.Room("tapestry-core:gate")
	exit := gate.Exits[world.DirSouth]
	if exit.Door == nil {
		t.Fatal("south exit has no door")
	}
	if !exit.Door.Closed || !exit.Door.Locked {
		t.Errorf("door state: closed=%v locked=%v, want both true", exit.Door.Closed, exit.Door.Locked)
	}
	if exit.Door.KeyID != "tapestry-core:gate-key" {
		t.Errorf("KeyID = %q, want tapestry-core:gate-key", exit.Door.KeyID)
	}
	wantKW := []string{"iron", "gate"}
	if len(exit.Door.Keywords) != 2 || exit.Door.Keywords[0] != wantKW[0] || exit.Door.Keywords[1] != wantKW[1] {
		t.Errorf("Keywords = %v, want %v", exit.Door.Keywords, wantKW)
	}
}

// TestLoadRoomDoor_DefaultsClosedTrue pins the §5.1 default that
// an omitted `closed:` field means closed=true (a door is closed
// unless content explicitly opens it).
func TestLoadRoomDoor_DefaultsClosedTrue(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: Square
exits:
  north: gate
doors:
  north:
    name: gate
`)
	writeFile(t, filepath.Join(pack, "rooms/gate.yaml"), `
id: gate
area: town
name: Gate
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	sq, _ := regs.World.Room("tapestry-core:square")
	d := sq.Exits[world.DirNorth].Door
	if d == nil || !d.Closed {
		t.Errorf("default closed: door = %+v", d)
	}
}

// TestLoadRoomDoor_ExplicitlyOpenable pins that closed: false
// leaves the door open at boot.
func TestLoadRoomDoor_ExplicitlyOpenable(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: Square
exits:
  north: gate
doors:
  north:
    name: archway
    closed: false
`)
	writeFile(t, filepath.Join(pack, "rooms/gate.yaml"), `
id: gate
area: town
name: Gate
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	sq, _ := regs.World.Room("tapestry-core:square")
	d := sq.Exits[world.DirNorth].Door
	if d == nil || d.Closed {
		t.Errorf("closed: false: door = %+v", d)
	}
}

// TestLoadRoomDoor_RejectsLockedOpen confirms locked: true without
// closed: true fails at load.
func TestLoadRoomDoor_RejectsLockedOpen(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: Square
exits:
  north: gate
doors:
  north:
    name: gate
    closed: false
    locked: true
`)
	writeFile(t, filepath.Join(pack, "rooms/gate.yaml"), `
id: gate
area: town
name: Gate
`)
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); err == nil {
		t.Fatal("locked + open: want error")
	}
}

// TestLoadRoomDoor_RejectsMissingExit confirms a doors entry whose
// direction has no matching exit fails at load.
func TestLoadRoomDoor_RejectsMissingExit(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: Square
exits:
  north: gate
doors:
  south:
    name: phantom-door
`)
	writeFile(t, filepath.Join(pack, "rooms/gate.yaml"), `
id: gate
area: town
name: Gate
`)
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); err == nil {
		t.Fatal("door without matching exit: want error")
	}
}

// TestLoadDescriptionField round-trips the optional `description:` field
// (the look appearance lens) through YAML → DTO → template for both a mob
// and an item, including a block scalar whose trailing newline must be
// trimmed at decode.
func TestLoadDescriptionField(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  items: [items/*.yaml]
  mobs: [mobs/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "items/sword.yaml"), `
id: sword
name: a short sword
type: weapon
description: |
  A plain soldier's blade, nicked from use.
keywords: [sword]
`)
	writeFile(t, filepath.Join(pack, "mobs/trainer.yaml"), `
id: trainer
name: Maerys
behavior: stationary
description: A broad-shouldered woman with scarred forearms.
keywords: [maerys]
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}

	it, err := regs.Items.Get(item.TemplateID("tapestry-core:sword"))
	if err != nil {
		t.Fatalf("item missing: %v", err)
	}
	if it.Description != "A plain soldier's blade, nicked from use." {
		t.Errorf("item Description = %q, want trimmed block scalar", it.Description)
	}

	mb, err := regs.Mobs.Get(mob.TemplateID("tapestry-core:trainer"))
	if err != nil {
		t.Fatalf("mob missing: %v", err)
	}
	if mb.Description != "A broad-shouldered woman with scarred forearms." {
		t.Errorf("mob Description = %q, want the authored prose", mb.Description)
	}
}

func TestLoadBackgrounds_HappyPath(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  backgrounds: [backgrounds/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "backgrounds/soldier.yaml"), `
id: soldier
name: Soldier
tagline: Steel and discipline.
skills:
  - ability: Open-Lock
    proficiency: 10
items: [Short-Sword]
gold: 25
allowed_categories: [Humanoid]
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	b, ok := regs.Backgrounds.Get("soldier")
	if !ok {
		t.Fatal("background soldier not registered")
	}
	if b.DisplayName != "Soldier" || b.Gold != 25 {
		t.Errorf("decoded %+v", b)
	}
	if len(b.Skills) != 1 || b.Skills[0].AbilityID != "open-lock" || b.Skills[0].Proficiency != 10 {
		t.Errorf("Skills = %+v, want [{open-lock 10}] (lowercased)", b.Skills)
	}
	if len(b.Items) != 1 || b.Items[0] != "tapestry-core:short-sword" {
		t.Errorf("Items = %v, want [tapestry-core:short-sword] (qualified + lowercased)", b.Items)
	}
	if len(b.AllowedCategories) != 1 || b.AllowedCategories[0] != "humanoid" {
		t.Errorf("AllowedCategories = %v, want [humanoid]", b.AllowedCategories)
	}
}

func TestLoadBackgrounds_RejectsMissingID(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  backgrounds: [backgrounds/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "backgrounds/bad.yaml"), "name: No ID\n")
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); !errors.Is(err, ErrInvalidContent) {
		t.Errorf("want ErrInvalidContent for missing id, got %v", err)
	}
}

func TestLoadBackgrounds_RejectsSkillWithoutAbility(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  backgrounds: [backgrounds/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "backgrounds/bad.yaml"), `
id: bad
skills:
  - proficiency: 5
`)
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); !errors.Is(err, ErrInvalidContent) {
		t.Errorf("want ErrInvalidContent for skill missing ability, got %v", err)
	}
}

func TestLoadFeats_HappyPath(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  feats: [feats/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "feats/weapon-focus.yaml"), `
id: Weapon-Focus
name: Weapon Focus
description: +1 to attack rolls with a chosen weapon.
multi_take: per_param
prerequisites:
  - { kind: Ability_Score, target: STR, min: 13 }
  - { kind: feat, target: Power-Attack }
  - { kind: level }
allowed_classes: [Fighter]
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	f, ok := regs.Feats.Get("weapon-focus") // global id, not namespaced
	if !ok {
		t.Fatal("feat weapon-focus not registered")
	}
	if f.DisplayName != "Weapon Focus" || f.MultiTake != feat.MultiTakeParam {
		t.Errorf("decoded %+v", f)
	}
	if len(f.Prerequisites) != 3 {
		t.Fatalf("Prerequisites = %d, want 3", len(f.Prerequisites))
	}
	if p := f.Prerequisites[0]; p.Kind != feat.PrereqAbilityScore || p.Target != "str" || p.Min != 13 {
		t.Errorf("prereq[0] = %+v, want {ability_score str 13}", p)
	}
	if p := f.Prerequisites[2]; p.Kind != feat.PrereqLevel || p.Target != "" {
		t.Errorf("prereq[2] = %+v, want {level } (no target)", p)
	}
	if len(f.AllowedClasses) != 1 || f.AllowedClasses[0] != "fighter" {
		t.Errorf("AllowedClasses = %v, want [fighter]", f.AllowedClasses)
	}
}

func TestLoadFeats_RejectsMissingID(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), "name: tapestry-core\ncontent:\n  feats: [feats/*.yaml]\n")
	writeFile(t, filepath.Join(pack, "feats/bad.yaml"), "name: No ID\n")
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); !errors.Is(err, ErrInvalidContent) {
		t.Errorf("want ErrInvalidContent for missing id, got %v", err)
	}
}

func TestLoadFeats_RejectsUnknownMultiTake(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), "name: tapestry-core\ncontent:\n  feats: [feats/*.yaml]\n")
	writeFile(t, filepath.Join(pack, "feats/bad.yaml"), "id: bad\nmulti_take: triple\n")
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); !errors.Is(err, ErrInvalidContent) {
		t.Errorf("want ErrInvalidContent for unknown multi_take, got %v", err)
	}
}

func TestLoadFeats_RejectsBadPrereq(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), "name: tapestry-core\ncontent:\n  feats: [feats/*.yaml]\n")
	// Unknown prereq kind.
	writeFile(t, filepath.Join(pack, "feats/bad.yaml"), "id: bad\nprerequisites:\n  - { kind: vibes, target: x }\n")
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); !errors.Is(err, ErrInvalidContent) {
		t.Errorf("want ErrInvalidContent for unknown prereq kind, got %v", err)
	}
	// Non-level prereq missing its target.
	root2 := t.TempDir()
	pack2 := filepath.Join(root2, "core")
	writeFile(t, filepath.Join(pack2, "pack.yaml"), "name: tapestry-core\ncontent:\n  feats: [feats/*.yaml]\n")
	writeFile(t, filepath.Join(pack2, "feats/bad.yaml"), "id: bad\nprerequisites:\n  - { kind: feat, min: 1 }\n")
	if err := Load(context.Background(), root2, nil, NewRegistries(), nil, nil, nil); !errors.Is(err, ErrInvalidContent) {
		t.Errorf("want ErrInvalidContent for prereq missing target, got %v", err)
	}
}

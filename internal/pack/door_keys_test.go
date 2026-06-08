package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestLoad_RejectsUnknownDoorKey verifies that a keyed door whose key
// id does not resolve to a known item template is a fatal load error
// (validateDoorKeys). Mirrors TestLoad_RejectsUnknownItemSlot: an
// unknown key would otherwise produce a permanently-unlockable door,
// fail-silent at the unlock attempt.
func TestLoad_RejectsUnknownDoorKey(t *testing.T) {
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
name: Room A
exits:
  north: b
doors:
  north:
    name: an iron door
    locked: true
    key: nonesuch-key
`)
	writeFile(t, filepath.Join(pack, "rooms/b.yaml"), "id: b\narea: town\nname: Room B\nexits:\n  south: a\n")

	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrMissingDoorKey) {
		t.Fatalf("Load err = %v, want ErrMissingDoorKey", err)
	}
}

// TestLoad_AcceptsKnownDoorKey is the happy-path counterpart: a keyed
// door whose key resolves to a loaded item template loads cleanly. Also
// exercises bare-key namespace qualification (key: gate-key resolves to
// tapestry-core:gate-key, the item's qualified id).
func TestLoad_AcceptsKnownDoorKey(t *testing.T) {
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
		"id: gate-key\nname: a brass gate key\ntype: item\n")
	writeFile(t, filepath.Join(pack, "rooms/a.yaml"), `
id: a
area: town
name: Room A
exits:
  north: b
doors:
  north:
    name: an iron gate
    locked: true
    key: gate-key
`)
	writeFile(t, filepath.Join(pack, "rooms/b.yaml"), "id: b\narea: town\nname: Room B\nexits:\n  south: a\n")

	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	door, ok := regs.World.GetDoor("tapestry-core:a", world.DirNorth)
	if !ok {
		t.Fatalf("door on room a north missing")
	}
	if door.KeyID != "tapestry-core:gate-key" {
		t.Errorf("door KeyID = %q, want tapestry-core:gate-key", door.KeyID)
	}
}

package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// mobPlacementPack writes a pack that declares one mob template AND
// a room placing it. Body is the YAML fragment inside the room's
// `mobs:` block so tests can vary entries (single, duplicates,
// unknown ids).
func mobPlacementPack(t *testing.T, mobsBody string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  mobs: [mobs/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), `
id: town
name: Town
`)
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: Town Square
`+mobsBody+`
`)
	writeFile(t, filepath.Join(pack, "mobs/guard.yaml"), `
id: guard
name: a village guard
behavior: stationary
`)
	return root
}

func TestLoad_PlacesMobsFromRoomYAML(t *testing.T) {
	root := mobPlacementPack(t, "mobs:\n  - guard\n")
	spawner := &recordingSpawner{}
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, spawner, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(spawner.calls) != 1 {
		t.Fatalf("mob spawn calls = %d, want 1", len(spawner.calls))
	}
	if spawner.calls[0].TemplateID != "tapestry-core:guard" {
		t.Errorf("templateID = %q", spawner.calls[0].TemplateID)
	}
	if spawner.calls[0].RoomID != "tapestry-core:square" {
		t.Errorf("roomID = %q", spawner.calls[0].RoomID)
	}
}

func TestLoad_MobPlacementUnknownTemplate(t *testing.T) {
	root := mobPlacementPack(t, "mobs:\n  - ghost-mob\n")
	err := Load(context.Background(), root, nil, NewRegistries(), nil, &recordingSpawner{}, nil)
	if !errors.Is(err, ErrMissingMobTemplate) {
		t.Fatalf("err = %v, want ErrMissingMobTemplate", err)
	}
}

// TestLoad_MobPlacementValidatesEvenWithNilSpawner pins the same
// contract items have: bad ids surface as load errors even when the
// caller passes nil spawner (template-only loads).
func TestLoad_MobPlacementValidatesEvenWithNilSpawner(t *testing.T) {
	root := mobPlacementPack(t, "mobs:\n  - ghost-mob\n")
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrMissingMobTemplate) {
		t.Fatalf("err = %v, want ErrMissingMobTemplate (nil mob spawner should still validate)", err)
	}
}

func TestLoad_NilMobSpawnerSkipsActualSpawning(t *testing.T) {
	root := mobPlacementPack(t, "mobs:\n  - guard\n")
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoad_EmptyMobsList(t *testing.T) {
	root := mobPlacementPack(t, "mobs: []\n")
	spawner := &recordingSpawner{}
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, spawner, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(spawner.calls) != 0 {
		t.Fatalf("mob spawn calls = %d, want 0", len(spawner.calls))
	}
}

// TestLoad_DuplicateMobIDsSpawnMultipleInstances mirrors the item
// duplicate test: declaring the same id twice produces two spawn
// calls. Useful for content that wants N identical mobs (a band of
// guards) without inventing N near-duplicate templates.
func TestLoad_DuplicateMobIDsSpawnMultipleInstances(t *testing.T) {
	root := mobPlacementPack(t, "mobs:\n  - guard\n  - guard\n")
	spawner := &recordingSpawner{}
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, spawner, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(spawner.calls) != 2 {
		t.Fatalf("mob spawn calls = %d, want 2", len(spawner.calls))
	}
}

func TestLoad_MobSpawnerErrorPropagates(t *testing.T) {
	root := mobPlacementPack(t, "mobs:\n  - guard\n")
	spawner := &recordingSpawner{err: errors.New("boom")}
	err := Load(context.Background(), root, nil, NewRegistries(), nil, spawner, nil)
	if err == nil || !errors.Is(err, spawner.err) {
		t.Fatalf("err = %v, want wrapping of spawner err", err)
	}
}

func TestLoad_EmptyMobsEntryRejected(t *testing.T) {
	root := mobPlacementPack(t, "mobs:\n  - \"\"\n")
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}

// TestLoad_RoomCanCarryBothItemsAndMobs confirms items and mobs
// placements coexist in the same room without interfering.
func TestLoad_RoomCanCarryBothItemsAndMobs(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  items: [items/*.yaml]
  mobs: [mobs/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: Town Square
items:
  - well
mobs:
  - guard
`)
	writeFile(t, filepath.Join(pack, "items/well.yaml"), `
id: well
name: a stone well
type: fixture
keywords: [well]
`)
	writeFile(t, filepath.Join(pack, "mobs/guard.yaml"), `
id: guard
name: a guard
behavior: idle
`)
	items := &recordingSpawner{}
	mobs := &recordingSpawner{}
	if err := Load(context.Background(), root, nil, NewRegistries(), items, mobs, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(items.calls) != 1 || items.calls[0].TemplateID != "tapestry-core:well" {
		t.Errorf("item spawns = %v", items.calls)
	}
	if len(mobs.calls) != 1 || mobs.calls[0].TemplateID != "tapestry-core:guard" {
		t.Errorf("mob spawns = %v", mobs.calls)
	}
}

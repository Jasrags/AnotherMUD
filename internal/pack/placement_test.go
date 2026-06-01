package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// recordingSpawner captures every SpawnAndPlace call. Tests assert on
// the recorded slice rather than poking at runtime entity-store state
// — the pack package shouldn't know about internal/entities at all.
type recordingSpawner struct {
	calls []spawnCall
	// err, if set, is returned from every call. Lets tests exercise
	// the loader's spawner-error propagation without depending on a
	// specific failure mode of a real spawner.
	err error
}

type spawnCall struct {
	TemplateID string
	RoomID     world.RoomID
}

func (s *recordingSpawner) SpawnAndPlace(_ context.Context, tid string, rid world.RoomID) error {
	if s.err != nil {
		return s.err
	}
	s.calls = append(s.calls, spawnCall{TemplateID: tid, RoomID: rid})
	return nil
}

// SpawnAndPlaceMob lets the same recorder satisfy pack.MobSpawner.
// Mob and item calls share the calls slice — tests inspect TemplateID
// to distinguish — but the err field gates both kinds uniformly so a
// single recorder can simulate either failure surface.
func (s *recordingSpawner) SpawnAndPlaceMob(_ context.Context, tid string, rid world.RoomID) error {
	if s.err != nil {
		return s.err
	}
	s.calls = append(s.calls, spawnCall{TemplateID: tid, RoomID: rid})
	return nil
}

// placementPack writes a minimal pack containing one room that
// declares an `items:` list of qualified ids. Used by several tests
// that just vary what's in the items field.
func placementPack(t *testing.T, items string) string {
	t.Helper()
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
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: Town Square
`+items+`
`)
	writeFile(t, filepath.Join(pack, "items/well.yaml"), `
id: well
name: a stone well
type: fixture
tags: [fixture, fill_source]
keywords: [well]
properties:
  fill_source: water
`)
	return root
}

func TestLoad_PlacesItemsFromRoomYAML(t *testing.T) {
	root := placementPack(t, "items:\n  - well\n")
	spawner := &recordingSpawner{}
	if err := Load(context.Background(), root, nil, NewRegistries(), spawner, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(spawner.calls) != 1 {
		t.Fatalf("spawn calls = %d, want 1", len(spawner.calls))
	}
	got := spawner.calls[0]
	if got.TemplateID != "tapestry-core:well" {
		t.Errorf("templateID = %q, want %q", got.TemplateID, "tapestry-core:well")
	}
	if got.RoomID != "tapestry-core:square" {
		t.Errorf("roomID = %q, want %q", got.RoomID, "tapestry-core:square")
	}
}

func TestLoad_PlacementUnknownTemplate(t *testing.T) {
	root := placementPack(t, "items:\n  - ghost-item\n")
	err := Load(context.Background(), root, nil, NewRegistries(), &recordingSpawner{}, nil, nil)
	if !errors.Is(err, ErrMissingItemTemplate) {
		t.Fatalf("err = %v, want ErrMissingItemTemplate", err)
	}
}

// TestLoad_PlacementValidatesEvenWithNilSpawner confirms that the
// template-existence check fires whether or not the caller actually
// wants spawning. This is the contract the loader_test.go callers
// (which pass nil spawner) implicitly rely on: bad content surfaces
// as a load error even in template-only paths.
func TestLoad_PlacementValidatesEvenWithNilSpawner(t *testing.T) {
	root := placementPack(t, "items:\n  - ghost-item\n")
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrMissingItemTemplate) {
		t.Fatalf("err = %v, want ErrMissingItemTemplate (nil spawner should still validate)", err)
	}
}

func TestLoad_NilSpawnerSkipsActualSpawning(t *testing.T) {
	// Valid placement + nil spawner: load succeeds, no calls happen
	// (which is the point — a recordingSpawner would have caught any
	// stray invocation).
	root := placementPack(t, "items:\n  - well\n")
	if err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

// TestLoad_DuplicateTemplateIDSpawnsMultipleInstances pins the
// no-dedup contract from the spec: `items: [well, well]` produces
// two SpawnAndPlace calls, not one. Useful for content authors
// who want N identical fixtures (multiple torches on a wall, etc.)
// without inventing N near-duplicate templates.
func TestLoad_DuplicateTemplateIDSpawnsMultipleInstances(t *testing.T) {
	root := placementPack(t, "items:\n  - well\n  - well\n")
	spawner := &recordingSpawner{}
	if err := Load(context.Background(), root, nil, NewRegistries(), spawner, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(spawner.calls) != 2 {
		t.Fatalf("spawn calls = %d, want 2 (no dedup)", len(spawner.calls))
	}
	for i, c := range spawner.calls {
		if c.TemplateID != "tapestry-core:well" {
			t.Errorf("call[%d].TemplateID = %q, want tapestry-core:well", i, c.TemplateID)
		}
	}
}

func TestLoad_EmptyItemsList(t *testing.T) {
	root := placementPack(t, "items: []\n")
	spawner := &recordingSpawner{}
	if err := Load(context.Background(), root, nil, NewRegistries(), spawner, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(spawner.calls) != 0 {
		t.Fatalf("spawn calls = %d, want 0", len(spawner.calls))
	}
}

// TestLoad_CrossPackPlacement covers the load-order safety net: pack
// A's room declares an item from pack B. Because validation runs
// AFTER both packs finish loading, this resolves regardless of the
// order discovery returns the packs in.
func TestLoad_CrossPackPlacement(t *testing.T) {
	root := t.TempDir()

	// Pack A — declares a room that places a B-pack item.
	packA := filepath.Join(root, "pack-a")
	writeFile(t, filepath.Join(packA, "pack.yaml"), `
name: pack-a
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
`)
	writeFile(t, filepath.Join(packA, "areas/town.yaml"), "id: town\nname: Town\n")
	writeFile(t, filepath.Join(packA, "rooms/square.yaml"), `
id: square
area: town
name: Square
items:
  - pack-b:lantern
`)

	// Pack B — owns the template.
	packB := filepath.Join(root, "pack-b")
	writeFile(t, filepath.Join(packB, "pack.yaml"), `
name: pack-b
content:
  items: [items/*.yaml]
`)
	writeFile(t, filepath.Join(packB, "items/lantern.yaml"), `
id: lantern
name: a brass lantern
type: fixture
keywords: [lantern]
`)

	spawner := &recordingSpawner{}
	if err := Load(context.Background(), root, nil, NewRegistries(), spawner, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(spawner.calls) != 1 {
		t.Fatalf("spawn calls = %d, want 1", len(spawner.calls))
	}
	if spawner.calls[0].TemplateID != "pack-b:lantern" {
		t.Errorf("templateID = %q, want pack-b:lantern", spawner.calls[0].TemplateID)
	}
	if spawner.calls[0].RoomID != "pack-a:square" {
		t.Errorf("roomID = %q, want pack-a:square", spawner.calls[0].RoomID)
	}
}

func TestLoad_SpawnerErrorPropagates(t *testing.T) {
	root := placementPack(t, "items:\n  - well\n")
	spawner := &recordingSpawner{err: errors.New("boom")}
	err := Load(context.Background(), root, nil, NewRegistries(), spawner, nil, nil)
	if err == nil || !errors.Is(err, spawner.err) {
		t.Fatalf("err = %v, want wrapping of spawner err", err)
	}
}

func TestLoad_EmptyItemsEntryRejected(t *testing.T) {
	// A leading hyphen with nothing after it (or a quoted empty
	// string) is malformed content. The loader catches it as
	// ErrInvalidContent at decode time so authors get a precise
	// error.
	root := placementPack(t, "items:\n  - \"\"\n")
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("err = %v, want ErrInvalidContent", err)
	}
}

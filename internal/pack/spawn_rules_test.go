package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// spawnRulePack writes a minimal pack carrying an area, a room, and
// a mob template. Pieces are stitched together so the area can
// reference both the room and the mob from spawn_rules without the
// post-pass validation tripping on missing refs (unless a test
// wants that).
func spawnRulePack(t *testing.T, areaBody string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  areas: [areas/*.yaml]
  rooms: [rooms/*.yaml]
  mobs:  [mobs/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), areaBody)
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: The Square
description: stones
`)
	writeFile(t, filepath.Join(pack, "mobs/guard.yaml"), `
id: guard
name: a guard
behavior: stationary
`)
	writeFile(t, filepath.Join(pack, "mobs/captain.yaml"), `
id: captain
name: a captain
behavior: stationary
`)
	return root
}

func TestLoad_DecodesAreaSpawnRules(t *testing.T) {
	root := spawnRulePack(t, `
id: town
name: Town
reset_interval: 600
spawn_rules:
  - room: square
    mob: guard
    count: 2
    rare: captain
    rare_chance: 0.05
    reset_interval: 120
    tags: [persistent]
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	area, err := regs.World.Area("tapestry-core:town")
	if err != nil {
		t.Fatalf("Area: %v", err)
	}
	if area.ResetInterval != 600 {
		t.Errorf("ResetInterval = %d, want 600", area.ResetInterval)
	}
	if n := len(area.SpawnRules); n != 1 {
		t.Fatalf("SpawnRules = %d, want 1", n)
	}
	r := area.SpawnRules[0]
	if r.RoomID != world.RoomID("tapestry-core:square") {
		t.Errorf("RoomID = %q", r.RoomID)
	}
	if r.MobTemplateID != "tapestry-core:guard" {
		t.Errorf("MobTemplateID = %q", r.MobTemplateID)
	}
	if r.Count != 2 || r.Rare != "tapestry-core:captain" || r.RareChance != 0.05 {
		t.Errorf("rule = %+v", r)
	}
	if !r.HasTag("persistent") {
		t.Errorf("missing 'persistent' tag: %v", r.Tags)
	}
}

func TestLoad_SpawnRuleMissingRoomRejected(t *testing.T) {
	root := spawnRulePack(t, `
id: town
name: Town
spawn_rules:
  - room: ghost
    mob: guard
    count: 1
`)
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrMissingSpawnRoom) {
		t.Errorf("err = %v, want ErrMissingSpawnRoom", err)
	}
}

func TestLoad_SpawnRuleMissingMobTemplateRejected(t *testing.T) {
	root := spawnRulePack(t, `
id: town
name: Town
spawn_rules:
  - room: square
    mob: phantom
    count: 1
`)
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrMissingMobTemplate) {
		t.Errorf("err = %v, want ErrMissingMobTemplate", err)
	}
}

func TestLoad_SpawnRuleZeroCountRejected(t *testing.T) {
	root := spawnRulePack(t, `
id: town
name: Town
spawn_rules:
  - room: square
    mob: guard
    count: 0
`)
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent", err)
	}
}

func TestLoad_SpawnRuleRareWithoutChanceRejected(t *testing.T) {
	root := spawnRulePack(t, `
id: town
name: Town
spawn_rules:
  - room: square
    mob: guard
    count: 1
    rare: captain
`)
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent (rare without rare_chance)", err)
	}
}

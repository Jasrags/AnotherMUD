package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/srckey"
)

// equipTemplates builds an item registry with a modifier-bearing sword
// and a plain (no-modifier) torch, so tests can exercise both the
// modifier-application and carry-only paths.
func equipTemplates() *item.Templates {
	r := item.NewTemplates()
	r.Add(&item.Template{
		ID:   "core:short-sword",
		Name: "a short sword",
		Type: "item",
		Properties: map[string]any{
			"slot": "wield",
		},
		Modifiers: []item.Modifier{
			{Stat: "str", Value: 4},
			{Stat: "hit_mod", Value: 3},
		},
	})
	r.Add(&item.Template{
		ID:   "core:torch",
		Name: "a torch",
		Type: "item",
	})
	return r
}

func TestEquipMobAtSpawnAppliesModifiersUnderSourceKey(t *testing.T) {
	s := NewStore()
	contents := NewContents()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	baseStr := inst.StatBlock().Effective(progression.StatType("str"))

	res, err := s.EquipMobAtSpawn(inst, []string{"core:short-sword"}, equipTemplates(), contents)
	if err != nil {
		t.Fatalf("EquipMobAtSpawn: %v", err)
	}
	if res.Equipped != 1 {
		t.Errorf("Equipped = %d, want 1", res.Equipped)
	}
	if len(res.Missing) != 0 {
		t.Errorf("Missing = %v, want empty", res.Missing)
	}

	// §3.3 step 3: modifiers raise the mob's effective stats.
	if got := inst.StatBlock().Effective(progression.StatType("str")); got != baseStr+4 {
		t.Errorf("effective str = %d, want %d", got, baseStr+4)
	}

	// §3.3 step 4: the item is filed in the mob's contents so it drops
	// into the corpse on death.
	filed := contents.In(inst.ID())
	if len(filed) != 1 {
		t.Fatalf("contents.In = %d items, want 1", len(filed))
	}

	// §3.3 step 3 (reversibility): modifiers carry a per-item equipment
	// source key so they can be cleanly removed.
	src := srckey.Equipment(string(filed[0]))
	if !inst.StatBlock().HasSource(src) {
		t.Errorf("stat block missing equipment source %v", src)
	}
	if !inst.RemoveBySource(src) {
		t.Errorf("RemoveBySource(%v) = false, want true", src)
	}
	if got := inst.StatBlock().Effective(progression.StatType("str")); got != baseStr {
		t.Errorf("after removal effective str = %d, want %d", got, baseStr)
	}
}

func TestEquipMobAtSpawnSkipsMissingTemplate(t *testing.T) {
	s := NewStore()
	contents := NewContents()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}

	res, err := s.EquipMobAtSpawn(inst,
		[]string{"core:short-sword", "core:does-not-exist", "core:torch"},
		equipTemplates(), contents)
	if err != nil {
		t.Fatalf("EquipMobAtSpawn: %v", err)
	}
	if res.Equipped != 2 {
		t.Errorf("Equipped = %d, want 2", res.Equipped)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "core:does-not-exist" {
		t.Errorf("Missing = %v, want [core:does-not-exist]", res.Missing)
	}
	// Only the two real items are carried.
	if got := len(contents.In(inst.ID())); got != 2 {
		t.Errorf("contents.In = %d, want 2", got)
	}
}

func TestEquipMobAtSpawnCarriesModifierlessItem(t *testing.T) {
	s := NewStore()
	contents := NewContents()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	snap := inst.StatBlock().ModifiersSnapshot()

	res, err := s.EquipMobAtSpawn(inst, []string{"core:torch"}, equipTemplates(), contents)
	if err != nil {
		t.Fatalf("EquipMobAtSpawn: %v", err)
	}
	if res.Equipped != 1 {
		t.Errorf("Equipped = %d, want 1", res.Equipped)
	}
	// A modifier-less item is carried but installs no source.
	if got := len(contents.In(inst.ID())); got != 1 {
		t.Errorf("contents.In = %d, want 1", got)
	}
	if after := inst.StatBlock().ModifiersSnapshot(); len(after) != len(snap) {
		t.Errorf("modifier count changed: before %d, after %d", len(snap), len(after))
	}
}

func TestEquipMobAtSpawnNoopGuards(t *testing.T) {
	s := NewStore()
	inst, err := s.SpawnMob(guardTpl())
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	tpls := equipTemplates()

	// nil mob, empty list, and nil registry are all no-ops with no error.
	if res, err := s.EquipMobAtSpawn(nil, []string{"core:torch"}, tpls, nil); err != nil || res.Equipped != 0 {
		t.Errorf("nil mob: res=%+v err=%v", res, err)
	}
	if res, err := s.EquipMobAtSpawn(inst, nil, tpls, nil); err != nil || res.Equipped != 0 {
		t.Errorf("empty ids: res=%+v err=%v", res, err)
	}
	if res, err := s.EquipMobAtSpawn(inst, []string{"core:torch"}, nil, nil); err != nil || res.Equipped != 0 {
		t.Errorf("nil registry: res=%+v err=%v", res, err)
	}

	// nil contents: modifiers still apply (step 3), step 4 is skipped.
	if res, err := s.EquipMobAtSpawn(inst, []string{"core:short-sword"}, tpls, nil); err != nil || res.Equipped != 1 {
		t.Errorf("nil contents: res=%+v err=%v", res, err)
	}
}

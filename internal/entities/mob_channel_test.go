package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// TestMobInstance_Stats_ChannelMapRouting proves the channel mapping is
// LIVE in mob combat-stat derivation (not dead wiring): a non-baseline
// mapping changes the derived AC, while a nil mapping (test default)
// preserves the direct stat read.
func TestMobInstance_Stats_ChannelMapRouting(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:goblin", Name: "a goblin", Type: "npc",
		Stats: map[string]int{"hp_max": 12, "ac": 8, "hit_mod": 1},
	}

	// nil mapping (default store): direct stat reads preserved.
	plain := NewStore()
	m1, err := plain.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if s := m1.Stats(); s.AC != 8 || s.HitMod != 1 {
		t.Fatalf("nil-mapping Stats = AC %d, HitMod %d; want 8, 1", s.AC, s.HitMod)
	}

	// custom mapping: defense boosted via formula, attack passthrough.
	cm, err := channel.NewMapping(map[channel.Channel]string{
		channel.Attack:  "hit_mod",
		channel.Defense: "ac + 5",
	})
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	mapped := NewStore()
	mapped.SetChannelMap(cm)
	m2, err := mapped.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if s := m2.Stats(); s.AC != 13 || s.HitMod != 1 {
		t.Fatalf("mapped Stats = AC %d, HitMod %d; want 13, 1 (defense=ac+5)", s.AC, s.HitMod)
	}
}

// TestStore_SetChannelMap_RetroStampsSpawnedMob covers the production
// ordering: a mob spawned during pack Load (before the mapping is built
// from content) must pick up the mapping when SetChannelMap is called
// afterward. Without retro-stamping, this mob would keep reading stat keys
// directly while runtime-spawned mobs used the override.
func TestStore_SetChannelMap_RetroStampsSpawnedMob(t *testing.T) {
	tpl := &mob.Template{
		ID: "core:goblin", Name: "a goblin", Type: "npc",
		Stats: map[string]int{"hp_max": 12, "ac": 8, "hit_mod": 1},
	}
	store := NewStore() // no mapping yet — mirrors a load-time spawn
	m, err := store.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	if s := m.Stats(); s.AC != 8 {
		t.Fatalf("pre-stamp AC = %d; want 8 (direct read)", s.AC)
	}

	cm, err := channel.NewMapping(map[channel.Channel]string{channel.Defense: "ac + 5"})
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	store.SetChannelMap(cm) // retro-stamps the already-spawned mob

	if s := m.Stats(); s.AC != 13 {
		t.Fatalf("post-stamp AC = %d; want 13 (defense=ac+5 applied retroactively)", s.AC)
	}
}

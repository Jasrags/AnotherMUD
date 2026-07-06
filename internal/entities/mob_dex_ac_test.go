package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/channel"
)

// TestMobDefenseReadsDexAC is the mob mirror of the player-side cappedDexAC
// producer (sr-m3c-deferred-fixes): a WoT-style mob under `defense: ac + dex_ac`
// must fold its armour-capped Dex modifier into AC. Before this, the mob lookup
// had no dex_ac producer, so dex_ac resolved to 0 and mob AC silently ignored
// Dex. Mirrors TestMobMitigationReadsArmorInput.
func TestMobDefenseReadsDexAC(t *testing.T) {
	s := NewStore()
	m, err := channel.NewMapping(map[channel.Channel]string{channel.Defense: "ac + dex_ac"})
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	s.SetChannelMap(m)

	tpl := guardTpl()
	tpl.Stats = map[string]int{"ac": 10, "dex": 14, "hp_max": 40} // dex 14 → +2 modifier
	tpl.Equipment = nil
	inst, err := s.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}

	// No armour cap: the full Dex modifier (+2) flows into AC → 10 + 2 = 12.
	if got := inst.Stats().AC; got != 12 {
		t.Fatalf("AC = %d, want 12 (ac 10 + dex mod +2 via dex_ac)", got)
	}

	// A restrictive armour max-Dex cap of 1 clamps the +2 to +1 → 11.
	cap1 := 1
	inst.SetArmorDexCap(&cap1)
	if got := inst.Stats().AC; got != 11 {
		t.Errorf("AC with max-dex cap 1 = %d, want 11 (dex mod clamped to +1)", got)
	}
}

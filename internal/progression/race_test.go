package progression_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

func TestRaceRegistryRegister(t *testing.T) {
	r := progression.NewRaceRegistry()
	if err := r.Register(&progression.Race{
		ID:          "human",
		DisplayName: "Human",
		Category:    "humanoid",
		RacialFlags: []string{"common-tongue"},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !r.Has("human") {
		t.Fatal("Has(human) = false after Register")
	}
	got, ok := r.Get("human")
	if !ok {
		t.Fatal("Get(human) = (_, false)")
	}
	if got.DisplayName != "Human" || got.Category != "humanoid" {
		t.Errorf("got = %+v", got)
	}
}

func TestRaceRegistryCaseInsensitive(t *testing.T) {
	r := progression.NewRaceRegistry()
	_ = r.Register(&progression.Race{ID: "Human"})
	if !r.Has("HUMAN") {
		t.Error("Has(HUMAN) = false; lookups should be case-insensitive")
	}
	got, _ := r.Get("hUmAn")
	if got == nil || got.ID != "human" {
		t.Errorf("Get(hUmAn) = %+v, want lowercased id", got)
	}
}

func TestRaceRegistryRejectsEmptyID(t *testing.T) {
	r := progression.NewRaceRegistry()
	if err := r.Register(nil); err == nil {
		t.Error("Register(nil) returned no error")
	}
	if err := r.Register(&progression.Race{ID: ""}); err == nil {
		t.Error("Register(empty id) returned no error")
	}
	if err := r.Register(&progression.Race{ID: "   "}); err == nil {
		t.Error("Register(whitespace id) returned no error")
	}
}

func TestRaceRegistryPriorityOverride(t *testing.T) {
	r := progression.NewRaceRegistry()
	_ = r.Register(&progression.Race{ID: "elf", DisplayName: "Elf", Priority: 0, Pack: "core"})
	_ = r.Register(&progression.Race{ID: "elf", DisplayName: "High Elf", Priority: 5, Pack: "expansion"})
	got, _ := r.Get("elf")
	if got.DisplayName != "High Elf" || got.Pack != "expansion" {
		t.Errorf("higher priority did not win: %+v", got)
	}

	// Equal priority retains existing.
	_ = r.Register(&progression.Race{ID: "elf", DisplayName: "Wood Elf", Priority: 5, Pack: "other"})
	got, _ = r.Get("elf")
	if got.DisplayName != "High Elf" {
		t.Errorf("equal priority displaced existing: %+v", got)
	}

	// Lower priority is dropped.
	_ = r.Register(&progression.Race{ID: "elf", DisplayName: "Common Elf", Priority: 1})
	got, _ = r.Get("elf")
	if got.DisplayName != "High Elf" {
		t.Errorf("lower priority displaced existing: %+v", got)
	}
}

func TestRaceRegistryStatCapsCloned(t *testing.T) {
	// Mutating the caller-side map after Register MUST NOT affect
	// the registered entry.
	caps := map[progression.StatType]int{progression.StatSTR: 18}
	r := progression.NewRaceRegistry()
	_ = r.Register(&progression.Race{ID: "halfling", StatCaps: caps})
	caps[progression.StatSTR] = 99 // tamper after registration

	got, _ := r.Get("halfling")
	if got.StatCaps[progression.StatSTR] != 18 {
		t.Errorf("StatCaps[STR] = %d, want 18 (registry must clone)", got.StatCaps[progression.StatSTR])
	}
}

func TestRaceRegistryRacialFlagsCloned(t *testing.T) {
	flags := []string{"darkvision"}
	r := progression.NewRaceRegistry()
	_ = r.Register(&progression.Race{ID: "drow", RacialFlags: flags})
	flags[0] = "tampered"

	got, _ := r.Get("drow")
	if got.RacialFlags[0] != "darkvision" {
		t.Errorf("RacialFlags[0] = %q, want darkvision (registry must clone)", got.RacialFlags[0])
	}
}

func TestRaceRegistryAllSorted(t *testing.T) {
	r := progression.NewRaceRegistry()
	_ = r.Register(&progression.Race{ID: "Zelf"})
	_ = r.Register(&progression.Race{ID: "human"})
	_ = r.Register(&progression.Race{ID: "Dwarf"})
	all := r.All()
	if len(all) != 3 {
		t.Fatalf("All len = %d, want 3", len(all))
	}
	if all[0].ID != "dwarf" || all[1].ID != "human" || all[2].ID != "zelf" {
		t.Errorf("All not sorted: %v", []string{all[0].ID, all[1].ID, all[2].ID})
	}
}

func TestAdjustCost(t *testing.T) {
	tests := []struct {
		name string
		base int
		race *progression.Race
		want int
	}{
		{"nil race", 10, nil, 10},
		{"zero modifier", 10, &progression.Race{CastCostModifier: 0}, 10},
		{"positive modifier", 10, &progression.Race{CastCostModifier: 3}, 13},
		{"negative modifier", 10, &progression.Race{CastCostModifier: -3}, 7},
		{"clamp at zero", 5, &progression.Race{CastCostModifier: -20}, 0},
		{"zero base + negative mod stays zero", 0, &progression.Race{CastCostModifier: -1}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := progression.AdjustCost(tc.base, tc.race)
			if got != tc.want {
				t.Errorf("AdjustCost(%d, %+v) = %d, want %d", tc.base, tc.race, got, tc.want)
			}
		})
	}
}

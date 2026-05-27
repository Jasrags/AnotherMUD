package progression

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

func TestClassRegistryRegisterAndGet(t *testing.T) {
	r := NewClassRegistry()
	c := &Class{ID: "Fighter", DisplayName: "Fighter", BoundTrack: "adventurer", TrainsPerLevel: 5}
	if err := r.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("FIGHTER")
	if !ok {
		t.Fatalf("Get(FIGHTER) miss")
	}
	if got.ID != "fighter" {
		t.Errorf("ID = %q, want lowercased 'fighter'", got.ID)
	}
	if !r.Has("fighter") {
		t.Error("Has(fighter) = false")
	}
	if r.Has("") {
		t.Error("Has(empty) = true")
	}
}

func TestClassRegisterEmptyIDFails(t *testing.T) {
	if err := NewClassRegistry().Register(&Class{}); err == nil {
		t.Fatal("expected error on empty id")
	}
}

func TestClassRegistryPriorityOverride(t *testing.T) {
	r := NewClassRegistry()
	_ = r.Register(&Class{ID: "fighter", DisplayName: "Old", Priority: 0})
	_ = r.Register(&Class{ID: "fighter", DisplayName: "New", Priority: 1})
	got, _ := r.Get("fighter")
	if got.DisplayName != "New" {
		t.Errorf("higher priority lost: %+v", got)
	}
	// equal priority no-ops
	_ = r.Register(&Class{ID: "fighter", DisplayName: "Same", Priority: 1})
	got, _ = r.Get("fighter")
	if got.DisplayName != "New" {
		t.Errorf("equal-priority overwrote existing: %+v", got)
	}
}

func TestClassRegisterDeepCopies(t *testing.T) {
	r := NewClassRegistry()
	path := []ClassPathEntry{{Level: 1, AbilityID: "slash"}}
	growth := map[StatType]combat.DiceExpr{StatHPMax: {Count: 1, Sides: 8}}
	bonuses := map[StatType]StatType{StatHPMax: StatCON}
	cats := []string{"Humanoid"}
	c := &Class{
		ID: "fighter", BoundTrack: "adventurer",
		Path: path, StatGrowth: growth, GrowthBonuses: bonuses,
		AllowedCategories: cats, TrainsPerLevel: 5,
	}
	if err := r.Register(c); err != nil {
		t.Fatal(err)
	}
	// mutate originals
	path[0].AbilityID = "MUTATED"
	growth[StatHPMax] = combat.DiceExpr{Count: 99, Sides: 99}
	bonuses[StatHPMax] = StatSTR
	cats[0] = "MUTATED"

	got, _ := r.Get("fighter")
	if got.Path[0].AbilityID != "slash" {
		t.Errorf("path not deep-copied: %q", got.Path[0].AbilityID)
	}
	if got.StatGrowth[StatHPMax].Count != 1 {
		t.Errorf("growth not deep-copied")
	}
	if got.GrowthBonuses[StatHPMax] != StatCON {
		t.Errorf("bonuses not deep-copied")
	}
	if got.AllowedCategories[0] != "humanoid" {
		t.Errorf("category not lowercased+deep-copied: %q", got.AllowedCategories[0])
	}
}

func TestClassRegisterClampsNegativeTrains(t *testing.T) {
	r := NewClassRegistry()
	_ = r.Register(&Class{ID: "weird", TrainsPerLevel: -3})
	got, _ := r.Get("weird")
	if got.TrainsPerLevel != 0 {
		t.Errorf("TrainsPerLevel = %d, want 0 (clamped)", got.TrainsPerLevel)
	}
}

func TestClassRegistryAllSorted(t *testing.T) {
	r := NewClassRegistry()
	_ = r.Register(&Class{ID: "ranger"})
	_ = r.Register(&Class{ID: "fighter"})
	_ = r.Register(&Class{ID: "mage"})
	all := r.All()
	if len(all) != 3 || all[0].ID != "fighter" || all[1].ID != "mage" || all[2].ID != "ranger" {
		t.Errorf("All() not id-sorted: %+v", all)
	}
}

func TestGetEligible(t *testing.T) {
	r := NewClassRegistry()
	_ = r.Register(&Class{ID: "fighter", AllowedCategories: []string{"humanoid"}})
	_ = r.Register(&Class{ID: "druid", AllowedCategories: []string{"humanoid", "fey"}, AllowedGenders: []string{"female"}})
	_ = r.Register(&Class{ID: "monk"}) // unrestricted

	cases := []struct {
		cat, gen string
		want     []string
	}{
		{"humanoid", "male", []string{"fighter", "monk"}},
		{"humanoid", "female", []string{"druid", "fighter", "monk"}},
		{"fey", "female", []string{"druid", "monk"}},
		{"fey", "male", []string{"monk"}},
		{"HUMANOID", "MALE", []string{"fighter", "monk"}}, // case-insens input
		{"undead", "", []string{"monk"}},
	}
	for _, c := range cases {
		got := r.GetEligible(c.cat, c.gen)
		if len(got) != len(c.want) {
			t.Errorf("GetEligible(%q,%q) len=%d want %d (%+v)", c.cat, c.gen, len(got), len(c.want), got)
			continue
		}
		for i, w := range c.want {
			if got[i].ID != w {
				t.Errorf("GetEligible(%q,%q)[%d] = %q, want %q", c.cat, c.gen, i, got[i].ID, w)
			}
		}
	}
}

package progression_test

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/progression"
)

func TestTrackGetXpForLevel_Table(t *testing.T) {
	td := &progression.TrackDef{
		Name:     "fighter",
		MaxLevel: 5,
		XPTable:  []int64{0, 0, 100, 300, 600, 1000},
	}
	tests := []struct {
		level int
		want  int64
	}{
		{0, 0},
		{1, 0},
		{2, 100},
		{3, 300},
		{4, 600},
		{5, 1000},
		{6, -1}, // out of range
	}
	for _, tc := range tests {
		got := td.GetXpForLevel(tc.level)
		if got != tc.want {
			t.Errorf("GetXpForLevel(%d) = %d, want %d", tc.level, got, tc.want)
		}
	}
}

func TestTrackGetXpForLevel_Formula(t *testing.T) {
	td := &progression.TrackDef{
		Name:      "explorer",
		MaxLevel:  100,
		XPFormula: func(l int) int64 { return int64(l*l) * 10 },
	}
	if got := td.GetXpForLevel(1); got != 10 {
		t.Errorf("GetXpForLevel(1) = %d, want 10", got)
	}
	if got := td.GetXpForLevel(5); got != 250 {
		t.Errorf("GetXpForLevel(5) = %d, want 250", got)
	}
	// Negative formula return → -1 sentinel.
	td.XPFormula = func(l int) int64 { return -42 }
	if got := td.GetXpForLevel(5); got != -1 {
		t.Errorf("negative formula return → got %d, want -1", got)
	}
}

func TestTrackGetXpForLevel_TableBeatsFormula(t *testing.T) {
	td := &progression.TrackDef{
		Name:      "both",
		MaxLevel:  3,
		XPTable:   []int64{0, 0, 50, 200},
		XPFormula: func(l int) int64 { return 99999 },
	}
	if got := td.GetXpForLevel(2); got != 50 {
		t.Errorf("table-priority broken: GetXpForLevel(2) = %d, want 50", got)
	}
}

func TestTrackGetXpForLevel_NeitherDefined(t *testing.T) {
	td := &progression.TrackDef{Name: "broken", MaxLevel: 10}
	if got := td.GetXpForLevel(1); got != -1 {
		t.Errorf("undefined → got %d, want -1", got)
	}
}

func TestTrackRegistryRegister(t *testing.T) {
	r := progression.NewTrackRegistry()

	if err := r.Register(&progression.TrackDef{Name: "fighter", MaxLevel: 50}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !r.Has("fighter") {
		t.Fatal("Has(fighter) = false after Register")
	}

	got, ok := r.Get("fighter")
	if !ok {
		t.Fatal("Get(fighter) = (_, false)")
	}
	if got.MaxLevel != 50 {
		t.Errorf("MaxLevel = %d, want 50", got.MaxLevel)
	}
}

func TestTrackRegistryRegisterRejectsBadDefs(t *testing.T) {
	r := progression.NewTrackRegistry()
	if err := r.Register(nil); err == nil {
		t.Error("Register(nil) returned no error")
	}
	if err := r.Register(&progression.TrackDef{Name: "", MaxLevel: 10}); err == nil {
		t.Error("Register(empty name) returned no error")
	}
	if err := r.Register(&progression.TrackDef{Name: "x", MaxLevel: 0}); err == nil {
		t.Error("Register(MaxLevel=0) returned no error")
	}
	if err := r.Register(&progression.TrackDef{Name: "x", MaxLevel: -1}); err == nil {
		t.Error("Register(MaxLevel=-1) returned no error")
	}
}

func TestTrackRegistryPriorityOverride(t *testing.T) {
	r := progression.NewTrackRegistry()
	_ = r.Register(&progression.TrackDef{Name: "fighter", MaxLevel: 30, Priority: 0, Pack: "core"})
	_ = r.Register(&progression.TrackDef{Name: "fighter", MaxLevel: 50, Priority: 5, Pack: "expansion"})
	got, _ := r.Get("fighter")
	if got.MaxLevel != 50 || got.Pack != "expansion" {
		t.Errorf("higher priority did not win: %+v", got)
	}

	// Equal priority does not displace.
	_ = r.Register(&progression.TrackDef{Name: "fighter", MaxLevel: 99, Priority: 5, Pack: "other"})
	got, _ = r.Get("fighter")
	if got.MaxLevel != 50 || got.Pack != "expansion" {
		t.Errorf("equal priority displaced existing: %+v", got)
	}

	// Lower priority is dropped.
	_ = r.Register(&progression.TrackDef{Name: "fighter", MaxLevel: 1, Priority: 1, Pack: "ignored"})
	got, _ = r.Get("fighter")
	if got.MaxLevel != 50 {
		t.Errorf("lower priority displaced existing: %+v", got)
	}
}

func TestTrackRegistryCaseSensitive(t *testing.T) {
	r := progression.NewTrackRegistry()
	_ = r.Register(&progression.TrackDef{Name: "Fighter", MaxLevel: 10})
	if r.Has("fighter") {
		t.Error("Has(fighter) = true but only Fighter was registered (spec: case-sensitive)")
	}
}

func TestTrackRegistryAllSorted(t *testing.T) {
	r := progression.NewTrackRegistry()
	_ = r.Register(&progression.TrackDef{Name: "zeta", MaxLevel: 1})
	_ = r.Register(&progression.TrackDef{Name: "alpha", MaxLevel: 1})
	_ = r.Register(&progression.TrackDef{Name: "mu", MaxLevel: 1})
	all := r.All()
	if len(all) != 3 {
		t.Fatalf("All len = %d, want 3", len(all))
	}
	if all[0].Name != "alpha" || all[1].Name != "mu" || all[2].Name != "zeta" {
		t.Errorf("All not sorted: %v", []string{all[0].Name, all[1].Name, all[2].Name})
	}
}

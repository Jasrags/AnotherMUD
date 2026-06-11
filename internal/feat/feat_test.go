package feat

import "testing"

func TestRegister_NormalizesAndDefaults(t *testing.T) {
	r := NewRegistry()
	src := &Feat{
		ID:          "  Weapon-Focus  ",
		DisplayName: "Weapon Focus",
		// MultiTake omitted → defaults to single.
		Prerequisites: []Prerequisite{
			{Kind: "Ability_Score", Target: "  STR  ", Min: 13},
			{Kind: PrereqFeat, Target: "Power-Attack"},
		},
		AllowedClasses: []string{"Fighter"},
	}
	if err := r.Register(src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Mutating the source after Register must not reach into the registry.
	src.Prerequisites[0].Target = "mutated"

	f, ok := r.Get("weapon-focus") // case-insensitive lookup
	if !ok {
		t.Fatal("feat weapon-focus not registered")
	}
	if f.ID != "weapon-focus" {
		t.Errorf("ID = %q, want weapon-focus (trimmed+lowercased)", f.ID)
	}
	if f.MultiTake != MultiTakeSingle {
		t.Errorf("MultiTake = %q, want single (default)", f.MultiTake)
	}
	if len(f.Prerequisites) != 2 {
		t.Fatalf("Prerequisites = %d, want 2", len(f.Prerequisites))
	}
	if got := f.Prerequisites[0]; got.Kind != PrereqAbilityScore || got.Target != "str" || got.Min != 13 {
		t.Errorf("prereq[0] = %+v, want {ability_score str 13} (normalized, un-mutated)", got)
	}
	if got := f.Prerequisites[1]; got.Kind != PrereqFeat || got.Target != "power-attack" {
		t.Errorf("prereq[1] = %+v, want {feat power-attack}", got)
	}
	if len(f.AllowedClasses) != 1 || f.AllowedClasses[0] != "fighter" {
		t.Errorf("AllowedClasses = %v, want [fighter]", f.AllowedClasses)
	}
}

func TestRegister_RejectsNilAndEmptyID(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Error("Register(nil) should error")
	}
	if err := r.Register(&Feat{ID: "   "}); err == nil {
		t.Error("Register with blank id should error")
	}
}

func TestRegister_PriorityOverride(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "toughness", DisplayName: "Base", Priority: 0})
	// Lower/equal priority no-ops.
	_ = r.Register(&Feat{ID: "toughness", DisplayName: "Equal", Priority: 0})
	if f, _ := r.Get("toughness"); f.DisplayName != "Base" {
		t.Errorf("equal priority should no-op, got %q", f.DisplayName)
	}
	// Higher priority replaces.
	_ = r.Register(&Feat{ID: "toughness", DisplayName: "Override", Priority: 5})
	if f, _ := r.Get("toughness"); f.DisplayName != "Override" {
		t.Errorf("higher priority should replace, got %q", f.DisplayName)
	}
}

func TestAll_SortedByID(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "toughness"})
	_ = r.Register(&Feat{ID: "iron-will"})
	_ = r.Register(&Feat{ID: "great-fortitude"})
	all := r.All()
	if len(all) != 3 {
		t.Fatalf("All = %d, want 3", len(all))
	}
	want := []string{"great-fortitude", "iron-will", "toughness"}
	for i, f := range all {
		if f.ID != want[i] {
			t.Errorf("All()[%d] = %q, want %q", i, f.ID, want[i])
		}
	}
}

func TestValidators(t *testing.T) {
	if !ValidMultiTake(MultiTakeParam) || ValidMultiTake("bogus") {
		t.Error("ValidMultiTake wrong")
	}
	if !ValidPrereqKind(PrereqLevel) || ValidPrereqKind("bogus") {
		t.Error("ValidPrereqKind wrong")
	}
}

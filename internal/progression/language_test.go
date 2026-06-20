package progression

import "testing"

func TestLanguageRegistry_RegisterAndGet(t *testing.T) {
	lr := NewLanguageRegistry()
	if err := lr.Register(&Language{ID: "Common-Aiel", Name: "Common (Aiel)", Family: "Common"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Id + family are lowercased on Register; lookup is case-insensitive.
	l, ok := lr.Get("common-aiel")
	if !ok {
		t.Fatalf("Get(common-aiel) miss")
	}
	if l.ID != "common-aiel" {
		t.Errorf("ID = %q, want common-aiel", l.ID)
	}
	if l.Family != "common" {
		t.Errorf("Family = %q, want common (lowercased)", l.Family)
	}
	if l.Name != "Common (Aiel)" {
		t.Errorf("Name = %q, want Common (Aiel)", l.Name)
	}
	if !lr.Has("COMMON-AIEL") {
		t.Error("Has is not case-insensitive")
	}
}

func TestLanguageRegistry_RejectsEmptyID(t *testing.T) {
	lr := NewLanguageRegistry()
	if err := lr.Register(&Language{Name: "No Id"}); err == nil {
		t.Error("Register with empty id should error")
	}
	if err := lr.Register(nil); err == nil {
		t.Error("Register(nil) should error")
	}
}

func TestLanguageRegistry_PriorityOverride(t *testing.T) {
	lr := NewLanguageRegistry()
	_ = lr.Register(&Language{ID: "x", Name: "low", Priority: 1})
	// Equal/lower priority no-ops; higher replaces.
	_ = lr.Register(&Language{ID: "x", Name: "equal", Priority: 1})
	if l, _ := lr.Get("x"); l.Name != "low" {
		t.Errorf("equal priority replaced: Name = %q, want low", l.Name)
	}
	_ = lr.Register(&Language{ID: "x", Name: "high", Priority: 2})
	if l, _ := lr.Get("x"); l.Name != "high" {
		t.Errorf("higher priority did not win: Name = %q, want high", l.Name)
	}
}

func TestLanguageRegistry_DisplayNameFallsBackToID(t *testing.T) {
	lr := NewLanguageRegistry()
	_ = lr.Register(&Language{ID: "ogier", Name: "the Ogier tongue"})
	if got := lr.DisplayName("ogier"); got != "the Ogier tongue" {
		t.Errorf("DisplayName(ogier) = %q, want the Ogier tongue", got)
	}
	// An unregistered id renders by id (lowercased), never empty (languages.md §4).
	if got := lr.DisplayName("wot:unknown-tongue"); got != "wot:unknown-tongue" {
		t.Errorf("DisplayName(unknown) = %q, want the id verbatim", got)
	}
}

func TestLanguageRegistry_AllSorted(t *testing.T) {
	lr := NewLanguageRegistry()
	_ = lr.Register(&Language{ID: "trolloc"})
	_ = lr.Register(&Language{ID: "common-aiel"})
	_ = lr.Register(&Language{ID: "old-tongue"})
	all := lr.All()
	if len(all) != 3 {
		t.Fatalf("All len = %d, want 3", len(all))
	}
	if all[0].ID != "common-aiel" || all[1].ID != "old-tongue" || all[2].ID != "trolloc" {
		t.Errorf("All not id-sorted: %s, %s, %s", all[0].ID, all[1].ID, all[2].ID)
	}
}

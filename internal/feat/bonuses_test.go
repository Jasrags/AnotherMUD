package feat

import "testing"

func TestComputeBonuses_Saves(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "iron-will", Grants: []Grant{{Kind: GrantSaveBonus, Target: "will", Magnitude: 2}}})
	_ = r.Register(&Feat{ID: "great-fortitude", Grants: []Grant{{Kind: GrantSaveBonus, Target: "fortitude", Magnitude: 2}}})
	// A synthetic stackable save feat to exercise the Count multiplier.
	_ = r.Register(&Feat{ID: "warding", MultiTake: MultiTakeStackable, Grants: []Grant{{Kind: GrantSaveBonus, Target: "will", Magnitude: 1}}})

	held := []Taken{
		{FeatID: "iron-will"},
		{FeatID: "great-fortitude"},
		{FeatID: "warding", Count: 3}, // +3 will (1 × 3)
		{FeatID: "ghost-feat"},        // removed content → skipped fail-soft
	}
	b := ComputeBonuses(held, r)
	if b.Saves["will"] != 5 { // 2 + 3
		t.Errorf("will bonus = %d, want 5", b.Saves["will"])
	}
	if b.Saves["fortitude"] != 2 {
		t.Errorf("fortitude bonus = %d, want 2", b.Saves["fortitude"])
	}
	if _, ok := b.Saves["reflex"]; ok {
		t.Errorf("reflex should have no bonus, got %d", b.Saves["reflex"])
	}
}

func TestComputeBonuses_Empties(t *testing.T) {
	r := NewRegistry()
	if b := ComputeBonuses(nil, r); b.Saves != nil {
		t.Errorf("no held feats should yield nil Saves, got %v", b.Saves)
	}
	if b := ComputeBonuses([]Taken{{FeatID: "x"}}, nil); b.Saves != nil {
		t.Errorf("nil registry should yield empty Bonuses, got %v", b.Saves)
	}
	// A held feat with no grants (a prereq-only doorway) contributes nothing.
	_ = r.Register(&Feat{ID: "latent-dreamer"})
	if b := ComputeBonuses([]Taken{{FeatID: "latent-dreamer"}}, r); b.Saves != nil {
		t.Errorf("grantless feat should contribute nothing, got %v", b.Saves)
	}
}

func TestRegister_NormalizesGrants(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{
		ID:     "great-fortitude",
		Grants: []Grant{{Kind: "Save_Bonus", Target: "  Fortitude  ", Magnitude: 2}},
	})
	f, _ := r.Get("great-fortitude")
	if len(f.Grants) != 1 {
		t.Fatalf("Grants = %d, want 1", len(f.Grants))
	}
	if g := f.Grants[0]; g.Kind != GrantSaveBonus || g.Target != "fortitude" || g.Magnitude != 2 {
		t.Errorf("grant = %+v, want {save_bonus fortitude 2} (normalized)", g)
	}
}

func TestValidGrantKindAndAxis(t *testing.T) {
	if !ValidGrantKind(GrantSaveBonus) || ValidGrantKind("bogus") {
		t.Error("ValidGrantKind wrong")
	}
	if !ValidSaveAxis("Fortitude") || !ValidSaveAxis("reflex") || ValidSaveAxis("dodge") {
		t.Error("ValidSaveAxis wrong")
	}
}

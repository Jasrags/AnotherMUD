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

// Toughness (a stackable max_hp feat) sums Magnitude × Count.
func TestComputeBonuses_MaxHP(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "toughness", MultiTake: MultiTakeStackable, Grants: []Grant{{Kind: GrantMaxHP, Magnitude: 3}}})
	if b := ComputeBonuses([]Taken{{FeatID: "toughness", Count: 3}}, r); b.MaxHP != 9 {
		t.Errorf("MaxHP = %d, want 9 (3 × 3)", b.MaxHP)
	}
	// A single take (count 0/1) applies once.
	if b := ComputeBonuses([]Taken{{FeatID: "toughness"}}, r); b.MaxHP != 3 {
		t.Errorf("MaxHP = %d, want 3", b.MaxHP)
	}
}

// The per-weapon/skill kinds key by the take's Param; the ability kind by the
// grant Target (EPIC S4 Phase 3c).
func TestComputeBonuses_PerParamAndAbility(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "weapon-focus", MultiTake: MultiTakeParam, Grants: []Grant{{Kind: GrantHitBonus, Magnitude: 1}}})
	_ = r.Register(&Feat{ID: "improved-critical", MultiTake: MultiTakeParam, Grants: []Grant{{Kind: GrantCritThreat, Magnitude: 2}}})
	_ = r.Register(&Feat{ID: "skill-emphasis", MultiTake: MultiTakeParam, Grants: []Grant{{Kind: GrantSkillBonus, Magnitude: 3}}})
	_ = r.Register(&Feat{ID: "power-attack", Grants: []Grant{{Kind: GrantAbility, Target: "power-attack"}}})

	b := ComputeBonuses([]Taken{
		{FeatID: "weapon-focus", Param: "sword"},
		{FeatID: "improved-critical", Param: "sword"},
		{FeatID: "skill-emphasis", Param: "open-lock"},
		{FeatID: "power-attack"},
	}, r)

	if b.HitByCategory["sword"] != 1 {
		t.Errorf("HitByCategory[sword] = %d, want 1", b.HitByCategory["sword"])
	}
	if b.CritByCategory["sword"] != 2 {
		t.Errorf("CritByCategory[sword] = %d, want 2", b.CritByCategory["sword"])
	}
	if b.SkillByID["open-lock"] != 3 {
		t.Errorf("SkillByID[open-lock] = %d, want 3", b.SkillByID["open-lock"])
	}
	if len(b.Abilities) != 1 || b.Abilities[0] != "power-attack" {
		t.Errorf("Abilities = %v, want [power-attack]", b.Abilities)
	}
	// A per-param grant with no param contributes nothing (guarded).
	if got := ComputeBonuses([]Taken{{FeatID: "weapon-focus"}}, r); got.HitByCategory != nil {
		t.Errorf("paramless per-param feat should contribute nothing, got %v", got.HitByCategory)
	}
}

// A stackable feat with Count 0 (the contract: "non-positive counts as one")
// applies its grant exactly once.
func TestComputeBonuses_StackableZeroCountAppliesOnce(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "warding", MultiTake: MultiTakeStackable, Grants: []Grant{{Kind: GrantSaveBonus, Target: "will", Magnitude: 1}}})
	if b := ComputeBonuses([]Taken{{FeatID: "warding", Count: 0}}, r); b.Saves["will"] != 1 {
		t.Errorf("Count 0 stackable = %d, want 1 (applies once)", b.Saves["will"])
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

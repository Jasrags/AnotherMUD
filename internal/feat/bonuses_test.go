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

// A feat carrying multiple save_bonus grants (Luck of Heroes → all three axes;
// Strong Soul → Fort + Will) sums per axis — the background-feat shape.
func TestComputeBonuses_MultiGrantSaves(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "luck-of-heroes", Grants: []Grant{
		{Kind: GrantSaveBonus, Target: "fortitude", Magnitude: 1},
		{Kind: GrantSaveBonus, Target: "reflex", Magnitude: 1},
		{Kind: GrantSaveBonus, Target: "will", Magnitude: 1},
	}})
	_ = r.Register(&Feat{ID: "strong-soul", Grants: []Grant{
		{Kind: GrantSaveBonus, Target: "fortitude", Magnitude: 1},
		{Kind: GrantSaveBonus, Target: "will", Magnitude: 1},
	}})
	b := ComputeBonuses([]Taken{{FeatID: "luck-of-heroes"}, {FeatID: "strong-soul"}}, r)
	if b.Saves["fortitude"] != 2 || b.Saves["will"] != 2 || b.Saves["reflex"] != 1 {
		t.Errorf("saves = %v, want fort 2 / will 2 / reflex 1", b.Saves)
	}
}

// weapon_proficiency grants collect the targeted weapon categories (Militia).
func TestComputeBonuses_WeaponProficiency(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "militia", Grants: []Grant{
		{Kind: GrantWeaponProficiency, Target: "light-crossbow"},
		{Kind: GrantWeaponProficiency, Target: "pike"},
	}})
	b := ComputeBonuses([]Taken{{FeatID: "militia"}}, r)
	got := map[string]bool{}
	for _, c := range b.WeaponProficiencyCategories {
		got[c] = true
	}
	if !got["light-crossbow"] || !got["pike"] {
		t.Errorf("granted categories = %v, want light-crossbow + pike", b.WeaponProficiencyCategories)
	}
	// No such feat → nil.
	if b2 := ComputeBonuses([]Taken{{FeatID: "iron-will"}}, NewRegistry()); b2.WeaponProficiencyCategories != nil {
		t.Errorf("absent feat should yield nil categories, got %v", b2.WeaponProficiencyCategories)
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

// The two-weapon feats aggregate into the global penalty-reduction fields
// (two-weapon-fighting §4.1, slice 2): Two-Weapon Fighting → TwoWeaponHitReduce
// (both hands), Ambidexterity → OffHandHitReduce (off hand only).
func TestComputeBonuses_TwoWeaponFeats(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "two-weapon-fighting", Grants: []Grant{{Kind: GrantTwoWeaponHit, Magnitude: 2}}})
	_ = r.Register(&Feat{ID: "ambidexterity", Grants: []Grant{{Kind: GrantOffHandHit, Magnitude: 4}}})

	if b := ComputeBonuses([]Taken{{FeatID: "two-weapon-fighting"}}, r); b.TwoWeaponHitReduce != 2 || b.OffHandHitReduce != 0 {
		t.Errorf("TWF alone = (%d,%d), want (2,0)", b.TwoWeaponHitReduce, b.OffHandHitReduce)
	}
	if b := ComputeBonuses([]Taken{{FeatID: "ambidexterity"}}, r); b.OffHandHitReduce != 4 || b.TwoWeaponHitReduce != 0 {
		t.Errorf("Ambidexterity alone = (%d,%d), want (0,4)", b.TwoWeaponHitReduce, b.OffHandHitReduce)
	}
	b := ComputeBonuses([]Taken{{FeatID: "two-weapon-fighting"}, {FeatID: "ambidexterity"}}, r)
	if b.TwoWeaponHitReduce != 2 || b.OffHandHitReduce != 4 {
		t.Errorf("both feats = (%d,%d), want (2,4)", b.TwoWeaponHitReduce, b.OffHandHitReduce)
	}

	// Improved Two-Weapon Fighting → OffHandExtraAttacks (the second strike, §3.1).
	_ = r.Register(&Feat{ID: "itwf", Grants: []Grant{{Kind: GrantOffHandAttack, Magnitude: 1}}})
	if b := ComputeBonuses([]Taken{{FeatID: "itwf"}}, r); b.OffHandExtraAttacks != 1 {
		t.Errorf("Improved TWF OffHandExtraAttacks = %d, want 1", b.OffHandExtraAttacks)
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

// A fixed-axis skill_bonus feat (Alertness → perception) names its skill via
// the grant Target (single-take), the symmetric counterpart to the per-param
// Skill Emphasis form. Two distinct detection feats stack on the same axis.
func TestComputeBonuses_FixedTargetSkill(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "alertness", Grants: []Grant{{Kind: GrantSkillBonus, Target: "perception", Magnitude: 2}}})
	_ = r.Register(&Feat{ID: "sharp-eyed", Grants: []Grant{{Kind: GrantSkillBonus, Target: "perception", Magnitude: 2}}})
	_ = r.Register(&Feat{ID: "stealthy", Grants: []Grant{{Kind: GrantSkillBonus, Target: "stealth", Magnitude: 2}}})

	b := ComputeBonuses([]Taken{
		{FeatID: "alertness"},
		{FeatID: "sharp-eyed"},
		{FeatID: "stealthy"},
	}, r)

	if b.SkillByID["perception"] != 4 {
		t.Errorf("SkillByID[perception] = %d, want 4 (alertness + sharp-eyed stack)", b.SkillByID["perception"])
	}
	if b.SkillByID["stealth"] != 2 {
		t.Errorf("SkillByID[stealth] = %d, want 2", b.SkillByID["stealth"])
	}
}

// Bucket B: damage_bonus is a per-weapon-category grant (Weapon Specialization,
// the damage sibling of Weapon Focus); ac_bonus is a global grant (Dodge, the
// AC sibling of max_hp) that sums across feats.
func TestComputeBonuses_DamageAndAC(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "weapon-specialization", MultiTake: MultiTakeParam, Grants: []Grant{{Kind: GrantDamageBonus, Magnitude: 2}}})
	_ = r.Register(&Feat{ID: "dodge", Grants: []Grant{{Kind: GrantACBonus, Magnitude: 1}}})
	_ = r.Register(&Feat{ID: "fancy-footwork", Grants: []Grant{{Kind: GrantACBonus, Magnitude: 1}}})

	b := ComputeBonuses([]Taken{
		{FeatID: "weapon-specialization", Param: "sword"},
		{FeatID: "dodge"},
		{FeatID: "fancy-footwork"},
	}, r)

	if b.DamageByCategory["sword"] != 2 {
		t.Errorf("DamageByCategory[sword] = %d, want 2", b.DamageByCategory["sword"])
	}
	if b.ACBonus != 2 {
		t.Errorf("ACBonus = %d, want 2 (two AC feats sum)", b.ACBonus)
	}
	// A paramless per-weapon damage grant contributes nothing (guarded).
	if got := ComputeBonuses([]Taken{{FeatID: "weapon-specialization"}}, r); got.DamageByCategory != nil {
		t.Errorf("paramless weapon-specialization should contribute nothing, got %v", got.DamageByCategory)
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

// The reputation feats (Fame / Infamy / Low Profile — reputation.md §7) aggregate
// as a flat renown bonus plus two boolean flags.
func TestComputeBonuses_Reputation(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&Feat{ID: "fame", Grants: []Grant{{Kind: GrantRenownBonus, Magnitude: 2}}})
	_ = r.Register(&Feat{ID: "infamy", Grants: []Grant{{Kind: GrantInfamy}}})
	_ = r.Register(&Feat{ID: "low-profile", Grants: []Grant{{Kind: GrantLowProfile}}})

	b := ComputeBonuses([]Taken{{FeatID: "fame"}, {FeatID: "infamy"}, {FeatID: "low-profile"}}, r)
	if b.RenownBonus != 2 {
		t.Errorf("RenownBonus = %d, want 2", b.RenownBonus)
	}
	if !b.Infamous {
		t.Error("Infamous = false, want true")
	}
	if !b.LowProfile {
		t.Error("LowProfile = false, want true")
	}

	// A character with none of them aggregates to zero/false.
	empty := ComputeBonuses(nil, r)
	if empty.RenownBonus != 0 || empty.Infamous || empty.LowProfile {
		t.Errorf("empty reputation bonuses = %+v, want zero/false", empty)
	}
}

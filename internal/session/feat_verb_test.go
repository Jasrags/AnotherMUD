package session

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

func featTestRegistry() *feat.Registry {
	r := feat.NewRegistry()
	_ = r.Register(&feat.Feat{ID: "iron-will", DisplayName: "Iron Will",
		Grants: []feat.Grant{{Kind: feat.GrantSaveBonus, Target: "will", Magnitude: 2}}})
	_ = r.Register(&feat.Feat{ID: "weapon-focus", DisplayName: "Weapon Focus", MultiTake: feat.MultiTakeParam,
		Grants: []feat.Grant{{Kind: feat.GrantHitBonus, Magnitude: 1}}})
	_ = r.Register(&feat.Feat{ID: "improved-critical", DisplayName: "Improved Critical", MultiTake: feat.MultiTakeParam,
		Grants: []feat.Grant{{Kind: feat.GrantCritThreat, Magnitude: 2}}})
	_ = r.Register(&feat.Feat{ID: "skill-emphasis", DisplayName: "Skill Emphasis", MultiTake: feat.MultiTakeParam,
		Grants: []feat.Grant{{Kind: feat.GrantSkillBonus, Magnitude: 3}}})
	_ = r.Register(&feat.Feat{ID: "power-attack", DisplayName: "Power Attack",
		Grants: []feat.Grant{{Kind: feat.GrantAbility, Target: "power-attack"}}})
	_ = r.Register(&feat.Feat{ID: "toughness", DisplayName: "Toughness", MultiTake: feat.MultiTakeStackable,
		Grants: []feat.Grant{{Kind: feat.GrantMaxHP, Magnitude: 3}}})
	_ = r.Register(&feat.Feat{ID: "born-strong", DisplayName: "Born Strong",
		Prerequisites: []feat.Prerequisite{{Kind: feat.PrereqAbilityScore, Target: "str", Min: 99}}})
	return r
}

func newFeatActor(t *testing.T, credits int) *connActor {
	t.Helper()
	a, _ := newFakeActor("c1", "p1", "acc1", "Hero", &world.Room{ID: "r"})
	a.feats = featTestRegistry()
	a.featCredits = credits
	a.save.FeatCredits = credits
	return a
}

func TestTakeFeat_HappyPathSpendsAndRecords(t *testing.T) {
	a := newFeatActor(t, 1)
	ok, msg := a.TakeFeat("iron-will", "")
	if !ok || !strings.Contains(msg, "Iron Will") {
		t.Fatalf("TakeFeat = (%v, %q)", ok, msg)
	}
	if a.FeatCredits() != 0 {
		t.Errorf("credits = %d, want 0 (spent)", a.FeatCredits())
	}
	if len(a.save.KnownFeats) != 1 || a.save.KnownFeats[0].FeatID != "iron-will" {
		t.Errorf("KnownFeats = %+v", a.save.KnownFeats)
	}
	if a.save.FeatCredits != 0 {
		t.Errorf("save.FeatCredits = %d, want 0", a.save.FeatCredits)
	}
	// The grant takes effect: Will lifts by 2 (Phase 3a consumer).
	if a.Saves().Will < 2 {
		t.Errorf("Will = %d, want >= 2 after Iron Will", a.Saves().Will)
	}
}

func TestTakeFeat_NoCredits(t *testing.T) {
	a := newFeatActor(t, 0)
	if ok, msg := a.TakeFeat("iron-will", ""); ok || !strings.Contains(msg, "no feat slots") {
		t.Errorf("TakeFeat without credits = (%v, %q)", ok, msg)
	}
	if len(a.save.KnownFeats) != 0 {
		t.Error("a failed take must not record a feat")
	}
}

func TestTakeFeat_AlreadyHave(t *testing.T) {
	a := newFeatActor(t, 2)
	a.TakeFeat("iron-will", "")
	if ok, msg := a.TakeFeat("iron-will", ""); ok || !strings.Contains(msg, "already have") {
		t.Errorf("second take of a single feat = (%v, %q)", ok, msg)
	}
	if a.FeatCredits() != 1 {
		t.Errorf("credits = %d, want 1 (the rejected take must not spend)", a.FeatCredits())
	}
}

func TestTakeFeat_Unknown(t *testing.T) {
	a := newFeatActor(t, 1)
	if ok, _ := a.TakeFeat("flibberjib", ""); ok {
		t.Error("unknown feat should not be takeable")
	}
	if a.FeatCredits() != 1 {
		t.Error("an unknown feat must not spend a credit")
	}
}

func TestTakeFeat_PerParamNeedsTarget(t *testing.T) {
	a := newFeatActor(t, 2)
	if ok, msg := a.TakeFeat("weapon-focus", ""); ok || !strings.Contains(msg, "specific target") {
		t.Errorf("per-param without target = (%v, %q)", ok, msg)
	}
	// With a target it succeeds and records the param.
	if ok, _ := a.TakeFeat("weapon-focus", "short-sword"); !ok {
		t.Fatal("per-param with target should succeed")
	}
	if a.save.KnownFeats[0].Param != "short-sword" {
		t.Errorf("param not recorded: %+v", a.save.KnownFeats[0])
	}
	// A different target is a distinct take.
	if ok, _ := a.TakeFeat("weapon-focus", "dagger"); !ok {
		t.Error("a second weapon should be takeable")
	}
	if len(a.save.KnownFeats) != 2 {
		t.Errorf("two weapon-focus instances expected, got %+v", a.save.KnownFeats)
	}
}

func TestTakeFeat_StackableIncrementsCount(t *testing.T) {
	a := newFeatActor(t, 3)
	a.TakeFeat("toughness", "")
	a.TakeFeat("toughness", "")
	if len(a.save.KnownFeats) != 1 || a.save.KnownFeats[0].Count != 2 {
		t.Errorf("stackable take twice = %+v, want one entry count 2", a.save.KnownFeats)
	}
}

// Taking a max_hp feat installs the stat modifier (Phase 3b): the stat block's
// effective hp_max rises by Magnitude × Count. (The vitals ceiling follows via
// the OnMaxChange binding wired in the live login path, exercised end to end by
// the live verify, not the fake actor.)
func TestTakeFeat_ToughnessRaisesHPMaxStat(t *testing.T) {
	a := newFeatActor(t, 2)
	base := a.statBlock.Effective(progression.StatHPMax)
	a.TakeFeat("toughness", "")
	a.TakeFeat("toughness", "")
	if got := a.statBlock.Effective(progression.StatHPMax); got != base+6 {
		t.Errorf("hp_max = %d, want %d (base %d + 3×2)", got, base+6, base)
	}
}

// EPIC S4 Phase 3c: Weapon Focus lifts to-hit and Improved Critical widens the
// threat range in Stats() — but only for the wielded weapon's category.
func TestStats_WeaponFeats(t *testing.T) {
	a := newFeatActor(t, 4)
	a.weapon.Store(&weaponInfo{category: "sword"}) // critThreatLow 0 → treated as 20
	a.GrantFeat("weapon-focus", "sword")
	a.GrantFeat("improved-critical", "sword")

	s := a.Stats()
	if s.HitMod != 1 {
		t.Errorf("HitMod = %d, want 1 (Weapon Focus sword)", s.HitMod)
	}
	if s.CritThreatLow != 18 {
		t.Errorf("CritThreatLow = %d, want 18 (20 widened by 2)", s.CritThreatLow)
	}
	// A different weapon category gets neither bonus: HitMod stays 0, and the
	// threat-low passes through as the weapon's raw value (0 here — the 0→20
	// normalization is the auto-attack's job, not Stats').
	a.weapon.Store(&weaponInfo{category: "axe"})
	if got := a.Stats(); got.HitMod != 0 || got.CritThreatLow != 0 {
		t.Errorf("axe Stats = {Hit %d, Crit %d}, want {0, 0} (focus is on sword)", got.HitMod, got.CritThreatLow)
	}
}

// Skill Emphasis adds a flat per-skill bonus (read at the skill-check site).
func TestFeatSkillBonus(t *testing.T) {
	a := newFeatActor(t, 1)
	a.GrantFeat("skill-emphasis", "open-lock")
	if got := a.FeatSkillBonus("Open-Lock"); got != 3 { // case-insensitive
		t.Errorf("FeatSkillBonus(open-lock) = %d, want 3", got)
	}
	if got := a.FeatSkillBonus("hide"); got != 0 {
		t.Errorf("unemphasized skill = %d, want 0", got)
	}
}

// Power Attack (the ability grant kind) teaches the named ability.
func TestGrantFeat_TeachesAbility(t *testing.T) {
	a := newFeatActor(t, 0)
	abilities := progression.NewAbilityRegistry()
	_ = abilities.Register(&progression.Ability{ID: "power-attack", Type: progression.AbilityPassive, Category: progression.AbilitySkill, DefaultCap: 100})
	a.prof = progression.NewProficiencyManager(abilities, progression.DefaultProficiencyConfig())

	a.GrantFeat("power-attack", "")
	if _, ok := a.prof.Proficiency(a.PlayerID(), "power-attack"); !ok {
		t.Error("Power Attack feat did not teach the power-attack ability")
	}
}

func TestTakeFeat_Ineligible(t *testing.T) {
	a := newFeatActor(t, 1)
	if ok, msg := a.TakeFeat("born-strong", ""); ok || !strings.Contains(msg, "STR 99+") {
		t.Errorf("ineligible take = (%v, %q)", ok, msg)
	}
	if a.FeatCredits() != 1 {
		t.Error("an ineligible take must not spend a credit")
	}
}

func TestFeatListing_ShowsKnownAndAvailable(t *testing.T) {
	a := newFeatActor(t, 1)
	a.TakeFeat("iron-will", "") // now 0 credits, 1 known
	// Re-grant a slot so the available section renders.
	a.featCredits = 1
	out := a.FeatListing()
	if !strings.Contains(out, "Iron Will") {
		t.Errorf("listing should show the held feat: %q", out)
	}
	if !strings.Contains(out, "Available:") {
		t.Errorf("listing should show an Available section with a slot banked: %q", out)
	}
	// Iron Will is single + held, so it should NOT appear as available again.
	avail := out[strings.Index(out, "Available:"):]
	if strings.Contains(avail, "Iron Will") {
		t.Errorf("a held single feat must not be offered again: %q", avail)
	}
}

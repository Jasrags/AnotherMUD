package session

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/size"
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
	// Fixed-axis skill feats wired into the live perception/stealth sites.
	_ = r.Register(&feat.Feat{ID: "alertness", DisplayName: "Alertness",
		Grants: []feat.Grant{{Kind: feat.GrantSkillBonus, Target: "perception", Magnitude: 2}}})
	_ = r.Register(&feat.Feat{ID: "stealthy", DisplayName: "Stealthy",
		Grants: []feat.Grant{{Kind: feat.GrantSkillBonus, Target: "stealth", Magnitude: 2}}})
	// Bucket B: per-weapon damage (Weapon Specialization) + global AC (Dodge).
	_ = r.Register(&feat.Feat{ID: "weapon-specialization", DisplayName: "Weapon Specialization", MultiTake: feat.MultiTakeParam,
		Grants: []feat.Grant{{Kind: feat.GrantDamageBonus, Magnitude: 2}}})
	_ = r.Register(&feat.Feat{ID: "dodge", DisplayName: "Dodge",
		Grants: []feat.Grant{{Kind: feat.GrantACBonus, Magnitude: 1}}})
	// Bucket C: Cleave / Great Cleave grant their marker abilities; HasCleave
	// reads the resulting feat-cache flags.
	_ = r.Register(&feat.Feat{ID: "cleave", DisplayName: "Cleave",
		Grants: []feat.Grant{{Kind: feat.GrantAbility, Target: "cleave"}}})
	_ = r.Register(&feat.Feat{ID: "great-cleave", DisplayName: "Great Cleave",
		Grants: []feat.Grant{{Kind: feat.GrantAbility, Target: "great-cleave"}}})
	return r
}

// Bucket C: HasCleave reports the Cleave / Great Cleave capability the combat
// CleaveFor hook reads; Great Cleave implies Cleave.
func TestHasCleave(t *testing.T) {
	a := newFeatActor(t, 3)
	if c, g := a.HasCleave(); c || g {
		t.Fatalf("fresh actor HasCleave = (%v,%v), want (false,false)", c, g)
	}
	a.GrantFeat("cleave", "")
	if c, g := a.HasCleave(); !c || g {
		t.Errorf("after Cleave HasCleave = (%v,%v), want (true,false)", c, g)
	}
	a.GrantFeat("great-cleave", "")
	if c, g := a.HasCleave(); !c || !g {
		t.Errorf("after Great Cleave HasCleave = (%v,%v), want (true,true)", c, g)
	}
}

// Bucket B: Weapon Specialization adds melee damage for the wielded weapon's
// category only, and not for a ranged wield. Mirrors Weapon Focus's per-weapon
// to-hit, on the damage axis.
func TestStats_WeaponSpecialization(t *testing.T) {
	a := newFeatActor(t, 2)
	a.GrantFeat("weapon-specialization", "sword")

	a.weapon.Store(&weaponInfo{category: "sword", wieldMode: size.OneHanded})
	withSword := a.Stats().DamageBonus
	a.weapon.Store(&weaponInfo{category: "axe", wieldMode: size.OneHanded})
	withAxe := a.Stats().DamageBonus
	if withSword-withAxe != 2 {
		t.Errorf("sword vs axe damage delta = %d, want 2 (specialization is sword-only)", withSword-withAxe)
	}

	// A ranged wield of the specialized category gets no bonus (melee-only).
	a.weapon.Store(&weaponInfo{category: "sword", rangedClass: "bow"})
	rangedDmg := a.Stats().DamageBonus
	a.weapon.Store(&weaponInfo{category: "bow", rangedClass: "bow"})
	otherRanged := a.Stats().DamageBonus
	if rangedDmg != otherRanged {
		t.Errorf("ranged specialization applied: sword-ranged=%d other-ranged=%d, want equal", rangedDmg, otherRanged)
	}
}

// Bucket B: Dodge raises Armor Class via the `ac` stat-modifier surface (like
// Toughness's hp_max), so it shows up in the combat stat block.
func TestStats_DodgeRaisesAC(t *testing.T) {
	a := newFeatActor(t, 1)
	base := a.Stats().AC
	a.GrantFeat("dodge", "")
	if got := a.Stats().AC; got != base+1 {
		t.Errorf("AC = %d, want %d (+1 Dodge)", got, base+1)
	}
}

// Phase 1: a fixed-axis skill feat lifts the live concealment/perception
// checks — Alertness raises the observer's PerceptionBonus, Stealthy raises
// both HideScore (stationary) and SneakDifficulty (moving). This proves the
// feat→skill bridge reaches the sites, not just FeatSkillBonus in isolation.
func TestPerceptionAndStealth_FoldFeatBonus(t *testing.T) {
	a := newFeatActor(t, 5)
	basePer, baseHide, baseSneak := a.PerceptionBonus(), a.HideScore(), a.SneakDifficulty()

	if ok, msg := a.TakeFeat("alertness", ""); !ok {
		t.Fatalf("TakeFeat(alertness) = %q", msg)
	}
	if ok, msg := a.TakeFeat("stealthy", ""); !ok {
		t.Fatalf("TakeFeat(stealthy) = %q", msg)
	}

	if got := a.PerceptionBonus(); got != basePer+2 {
		t.Errorf("PerceptionBonus = %d, want %d (+2 Alertness)", got, basePer+2)
	}
	if got := a.HideScore(); got != baseHide+2 {
		t.Errorf("HideScore = %d, want %d (+2 Stealthy)", got, baseHide+2)
	}
	if got := a.SneakDifficulty(); got != baseSneak+2 {
		t.Errorf("SneakDifficulty = %d, want %d (+2 Stealthy)", got, baseSneak+2)
	}
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

// feats Bucket C: the Power Attack stance trades to-hit for melee damage in
// Stats() — only with the stance on, the feat held, and a melee weapon. A
// two-handed wield doubles the damage half. Measured as deltas (stance on minus
// off) so the test is independent of the actor's base hit/damage.
func TestStats_PowerAttackStance(t *testing.T) {
	const trade = combat.DefaultPowerAttackTrade
	a := newFeatActor(t, 2)
	a.GrantFeat("power-attack", "")

	// One-handed melee: -trade to-hit, +trade damage.
	a.weapon.Store(&weaponInfo{category: "sword", wieldMode: size.OneHanded})
	off := a.Stats()
	a.SetPowerAttack(true)
	on := a.Stats()
	if got := off.HitMod - on.HitMod; got != trade {
		t.Errorf("one-handed HitMod penalty = %d, want %d", got, trade)
	}
	if got := on.DamageBonus - off.DamageBonus; got != trade {
		t.Errorf("one-handed damage bonus = %d, want %d", got, trade)
	}

	// Two-handed melee: same to-hit penalty, DOUBLED damage (size §4.2).
	a.weapon.Store(&weaponInfo{category: "greatsword", wieldMode: size.TwoHanded})
	thOn := a.Stats()
	a.SetPowerAttack(false)
	thOff := a.Stats()
	if got := thOff.HitMod - thOn.HitMod; got != trade {
		t.Errorf("two-handed HitMod penalty = %d, want %d", got, trade)
	}
	if got := thOn.DamageBonus - thOff.DamageBonus; got != 2*trade {
		t.Errorf("two-handed damage bonus = %d, want %d (doubled)", got, 2*trade)
	}

	// Ranged weapon: the melee-only trade does not apply (no stat change on/off).
	a.weapon.Store(&weaponInfo{category: "bow", rangedClass: "bow"})
	rOff := a.Stats()
	a.SetPowerAttack(true)
	rOn := a.Stats()
	if rOn.HitMod != rOff.HitMod || rOn.DamageBonus != rOff.DamageBonus {
		t.Errorf("ranged stance applied a trade: on={%d,%d} off={%d,%d}",
			rOn.HitMod, rOn.DamageBonus, rOff.HitMod, rOff.DamageBonus)
	}
}

// The stance is inert without the feat: a flag set on a character who never
// took Power Attack produces no trade (a stale-on stance after a hypothetical
// respec stays harmless).
func TestStats_PowerAttackWithoutFeatIsInert(t *testing.T) {
	withStance := newFeatActor(t, 0)
	withStance.weapon.Store(&weaponInfo{category: "sword", wieldMode: size.OneHanded})
	withStance.SetPowerAttack(true) // flag on, but no power-attack feat held
	got := withStance.Stats()

	ref := newFeatActor(t, 0)
	ref.weapon.Store(&weaponInfo{category: "sword", wieldMode: size.OneHanded})
	want := ref.Stats()

	if got.HitMod != want.HitMod || got.DamageBonus != want.DamageBonus {
		t.Errorf("stance without feat changed stats: got={%d,%d} want={%d,%d}",
			got.HitMod, got.DamageBonus, want.HitMod, want.DamageBonus)
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

	// Idempotency: practice the granted ability, then re-run applyFeatGrants
	// (as a login / another feat change would) — the practiced value must
	// survive, not reset to the baseline 1.
	a.prof.Learn(a.PlayerID(), "power-attack", 50)
	a.applyFeatGrants()
	if v, _ := a.prof.Proficiency(a.PlayerID(), "power-attack"); v != 50 {
		t.Errorf("granted ability proficiency = %d, want 50 (re-grant must not reset)", v)
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

package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/size"
)

// Stats() builds an off-hand profile (two-weapon-fighting §3/§4) when a melee
// main weapon is paired with a LIGHT off-hand weapon: the main hand takes the
// two-weapon penalty, the off-hand profile carries the larger off-hand penalty
// and the ½× Strength damage.
func TestStats_OffHandProfile(t *testing.T) {
	const str = 16 // STRBonus(16) = 3; ½× ⇒ 1 (round down)
	strBonus := combat.STRBonus(str)
	a := &connActor{statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: str})}
	a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 8}, name: "a sword", wieldMode: size.OneHanded})
	a.offWeapon.Store(&weaponInfo{
		dice: combat.DiceExpr{Count: 1, Sides: 4}, name: "a dagger",
		damageTypes: []string{"slashing"}, critThreatLow: 19, critMultiplier: 2,
		wieldMode: size.Light,
	})

	s := a.Stats()
	if s.OffHand == nil {
		t.Fatal("dual-wielding a light off-hand weapon should produce an OffHand profile")
	}
	// Main hand: full Strength on damage, minus the two-weapon main penalty on hit.
	if s.DamageBonus != strBonus {
		t.Errorf("main DamageBonus = %d, want %d (full Strength)", s.DamageBonus, strBonus)
	}
	if want := -combat.DefaultTwoWeaponMainPenalty; s.HitMod != want {
		t.Errorf("main HitMod = %d, want %d (two-weapon main penalty)", s.HitMod, want)
	}
	// Off hand: its own dice/name/crit, the larger penalty, ½× Strength damage.
	off := s.OffHand
	if off.WeaponName != "a dagger" || off.Damage.Sides != 4 {
		t.Errorf("off-hand weapon = %q %v, want a dagger 1d4", off.WeaponName, off.Damage)
	}
	if off.CritThreatLow != 19 || off.CritMultiplier != 2 {
		t.Errorf("off-hand crit = %d/%d, want 19/2 (the off-hand weapon's own)", off.CritThreatLow, off.CritMultiplier)
	}
	if want := -combat.DefaultTwoWeaponOffHandPenalty; off.HitMod != want {
		t.Errorf("off-hand HitMod = %d, want %d (off-hand penalty off the base)", off.HitMod, want)
	}
	wantOffDmg := strBonus + size.StrBonusDelta(strBonus, size.DefaultOffHandStrFactor) // 3 + (1-3) = 1
	if off.DamageBonus != wantOffDmg {
		t.Errorf("off-hand DamageBonus = %d, want %d (½× Strength)", off.DamageBonus, wantOffDmg)
	}
	if len(off.WeaponDamageTypes) != 1 || off.WeaponDamageTypes[0] != "slashing" {
		t.Errorf("off-hand damage types = %v, want [slashing]", off.WeaponDamageTypes)
	}
}

// The two-weapon feats (slice 2) SUBTRACT from the baked penalties
// (two-weapon-fighting §4.1): Two-Weapon Fighting trims both hands by 2,
// Ambidexterity removes the off-hand-specific extra (4). With both, the baseline
// -4 main / -8 off becomes -2 / -2. The reductions ride the lock-free feat cache.
func TestStats_OffHandFeatReducesPenalty(t *testing.T) {
	newDualWielder := func() *connActor {
		a := &connActor{statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})}
		a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 8}, name: "a sword", wieldMode: size.OneHanded})
		a.offWeapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 4}, name: "a dagger", wieldMode: size.Light})
		return a
	}

	// Two-Weapon Fighting alone: both penalties drop by 2.
	a := newDualWielder()
	a.featWeaponBonus.Store(&featWeaponBonuses{twoWeaponHitReduce: 2})
	s := a.Stats()
	if want := -(combat.DefaultTwoWeaponMainPenalty - 2); s.HitMod != want {
		t.Errorf("TWF main HitMod = %d, want %d", s.HitMod, want)
	}
	if want := -(combat.DefaultTwoWeaponOffHandPenalty - 2); s.OffHand == nil || s.OffHand.HitMod != want {
		t.Errorf("TWF off HitMod = %v, want %d", s.OffHand, want)
	}

	// Two-Weapon Fighting + Ambidexterity: -2 main / -2 off (the canonical result).
	a = newDualWielder()
	a.featWeaponBonus.Store(&featWeaponBonuses{twoWeaponHitReduce: 2, offHandHitReduce: 4})
	s = a.Stats()
	if s.HitMod != -2 {
		t.Errorf("both feats main HitMod = %d, want -2", s.HitMod)
	}
	if s.OffHand == nil || s.OffHand.HitMod != -2 {
		t.Errorf("both feats off HitMod = %v, want -2", s.OffHand)
	}

	// Over-reduction clamps at zero — a feat never turns the penalty into a bonus.
	a = newDualWielder()
	a.featWeaponBonus.Store(&featWeaponBonuses{twoWeaponHitReduce: 99, offHandHitReduce: 99})
	s = a.Stats()
	if s.HitMod != 0 {
		t.Errorf("clamped main HitMod = %d, want 0", s.HitMod)
	}
	if s.OffHand == nil || s.OffHand.HitMod != 0 {
		t.Errorf("clamped off HitMod = %v, want 0", s.OffHand)
	}
}

// Improved Two-Weapon Fighting (slice 3): the off-hand extra-attack count in the
// feat cache raises OffHandProfile.Attacks (1 + extra). Baseline (no cache / no
// feat) stays at one strike.
func TestStats_OffHandAttackCount(t *testing.T) {
	newDualWielder := func() *connActor {
		a := &connActor{statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})}
		a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 8}, name: "a sword", wieldMode: size.OneHanded})
		a.offWeapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 4}, name: "a dagger", wieldMode: size.Light})
		return a
	}

	// No feat cache ⇒ one off-hand strike (the slice-1 baseline).
	if s := newDualWielder().Stats(); s.OffHand == nil || s.OffHand.Attacks != 1 {
		t.Fatalf("baseline OffHand.Attacks = %v, want 1", s.OffHand)
	}

	// Improved TWF (one extra) ⇒ two off-hand strikes.
	a := newDualWielder()
	a.featWeaponBonus.Store(&featWeaponBonuses{offHandExtraAttacks: 1})
	if s := a.Stats(); s.OffHand == nil || s.OffHand.Attacks != 2 {
		t.Fatalf("with Improved TWF OffHand.Attacks = %v, want 2", s.OffHand)
	}
}

// A non-light off-hand weapon (one-handed or larger for the wielder) occupies
// the slot but grants NO off-hand attack, and imposes no two-weapon penalty
// (two-weapon-fighting §2.2).
func TestStats_NonLightOffHandNoAttack(t *testing.T) {
	a := &connActor{statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 12})}
	a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 8}, name: "a sword", wieldMode: size.OneHanded})
	a.offWeapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 6}, name: "a mace", wieldMode: size.OneHanded})

	s := a.Stats()
	if s.OffHand != nil {
		t.Error("a non-light off-hand weapon should not grant an off-hand attack")
	}
	if s.HitMod != 0 {
		t.Errorf("main HitMod = %d, want 0 (no two-weapon penalty without a valid off-hand)", s.HitMod)
	}
}

// A ranged main weapon suppresses the off-hand profile — two-weapon fighting is
// a melee concern (two-weapon-fighting §3).
func TestStats_RangedMainNoOffHand(t *testing.T) {
	a := &connActor{statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 12})}
	a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 6}, name: "a bow", wieldMode: size.TwoHanded, rangedClass: "projectile"})
	a.offWeapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 4}, name: "a dagger", wieldMode: size.Light})

	if s := a.Stats(); s.OffHand != nil {
		t.Error("a ranged main weapon should suppress the off-hand attack")
	}
}

// No off-hand weapon ⇒ no off-hand profile and no two-weapon penalty (single
// weapon or weapon+shield behaves exactly as before).
func TestStats_NoOffHandWeapon(t *testing.T) {
	a := &connActor{statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 14})}
	a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 8}, name: "a sword", wieldMode: size.OneHanded})

	s := a.Stats()
	if s.OffHand != nil {
		t.Error("no off-hand weapon should leave OffHand nil")
	}
	if s.HitMod != 0 {
		t.Errorf("main HitMod = %d, want 0 (no two-weapon penalty)", s.HitMod)
	}
}

// recomputeWeaponLocked picks the MAIN weapon from the wield slot and the
// off-hand weapon from the off-hand slot (two-weapon-fighting §2) — NOT "first
// weapon by sorted key" (offhand sorts before wield). Equipping a light weapon
// to the off hand alongside a one-handed main weapon produces both a main
// weapon and an off-hand attack from the correct slots.
func TestRecompute_TwoWeaponSlotAware(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store) // raceless ⇒ medium-size wielder

	main, _ := store.Spawn(&item.Template{
		ID: "x:sword", Name: "a sword", Type: "weapon",
		Keywords: []string{"sword"}, WeaponDamage: "1d8", // no size ⇒ medium ⇒ one-handed
	})
	off, _ := store.Spawn(&item.Template{
		ID: "x:knife", Name: "a knife", Type: "weapon",
		Keywords: []string{"knife"}, WeaponDamage: "1d3", Size: "small", // small ⇒ light
	})
	a.AddToInventory(main.ID())
	a.AddToInventory(off.ID())
	if !a.Equip([]string{"wield"}, main.ID(), nil) {
		t.Fatal("equip main to wield")
	}
	if !a.Equip([]string{"offhand"}, off.ID(), nil) {
		t.Fatal("equip knife to offhand")
	}

	s := a.Stats()
	if s.Damage.Sides != 8 {
		t.Errorf("main weapon = %v, want 1d8 (from the wield slot, not the off hand)", s.Damage)
	}
	if s.OffHand == nil || s.OffHand.WeaponName != "a knife" {
		t.Fatalf("off-hand = %v, want the knife from the off-hand slot", s.OffHand)
	}
}

// A spanning two-handed weapon (one id under BOTH the wield and off-hand keys)
// is the main weapon, never mistaken for a second off-hand weapon
// (two-weapon-fighting §2 — the id-distinctness guard).
func TestRecompute_TwoHanderIsNotAnOffHandWeapon(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)

	gs, _ := store.Spawn(&item.Template{
		ID: "x:greatsword", Name: "a greatsword", Type: "weapon",
		Keywords: []string{"greatsword"}, WeaponDamage: "2d6", Size: "large", // large ⇒ two-handed for a medium wielder
	})
	a.AddToInventory(gs.ID())
	// The two-handed footprint spans both hands — the same id under both keys.
	if !a.Equip([]string{"wield", "offhand"}, gs.ID(), nil) {
		t.Fatal("equip the two-hander across both hands")
	}

	if s := a.Stats(); s.OffHand != nil {
		t.Error("a spanning two-hander must not produce an off-hand attack")
	}
}

// special-weapons §3 — reach plumbs from the wielded weaponInfo into
// combat.Stats.Reach (the band-gate reads it). A non-reach weapon leaves it 0.
func TestStats_ReachPlumbsToCombatStats(t *testing.T) {
	a := &connActor{statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 10})}
	a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 6}, name: "a quarterstaff", wieldMode: size.TwoHanded, reach: 1})
	if got := a.Stats().Reach; got != 1 {
		t.Errorf("Stats().Reach = %d, want 1 (a reach weapon)", got)
	}

	a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{Count: 1, Sides: 8}, name: "a sword", wieldMode: size.OneHanded})
	if got := a.Stats().Reach; got != 0 {
		t.Errorf("Stats().Reach = %d, want 0 (an ordinary weapon)", got)
	}
}

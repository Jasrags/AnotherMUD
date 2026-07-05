package entities

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/size"
)

// A mob's melee damage earns the two-handed Strength bonus (size-and-wielding
// §4.2, §5) only when its equipped weapon resolves to a two-handed grip FOR ITS
// OWN SIZE — the same relativity a player gets. So a weapon that is two-handed
// for a medium wielder is one-handed (no bonus) for a large mob, and a large
// mob needs a HUGE weapon before the off hand and the bonus come into play.
func TestMobStats_TwoHandedStrengthBonus(t *testing.T) {
	const str = 16 // STRBonus(16) = (16-10)/2 = 3
	strBonus := combat.STRBonus(str)
	twoHandedExtra := size.TwoHandedStrBonus(strBonus, size.DefaultTwoHandedStrFactor)
	if twoHandedExtra <= 0 {
		t.Fatalf("test precondition: expected a positive two-handed extra, got %d", twoHandedExtra)
	}

	tests := []struct {
		name       string
		mobSize    string
		weaponSize string
		want       int
	}{
		{"medium mob + large weapon ⇒ two-handed ⇒ bonus", "medium", "large", strBonus + twoHandedExtra},
		{"medium mob + medium weapon ⇒ one-handed ⇒ no bonus", "medium", "medium", strBonus},
		{"large mob + large weapon ⇒ one-handed ⇒ no bonus", "large", "large", strBonus},
		{"large mob + huge weapon ⇒ two-handed ⇒ bonus", "large", "huge", strBonus + twoHandedExtra},
		{"large mob + small weapon ⇒ light ⇒ no bonus", "large", "small", strBonus},
		{"sizeless mob + sizeless weapon ⇒ baseline one-handed ⇒ no bonus", "", "", strBonus},
	}
	for _, tt := range tests {
		s := NewStore()
		tpl := &mob.Template{
			ID: "test:brute", Name: "a brute", Type: "npc",
			Size:  tt.mobSize,
			Stats: map[string]int{"str": str},
		}
		inst, err := s.SpawnMob(tpl)
		if err != nil {
			t.Fatalf("%s: SpawnMob: %v", tt.name, err)
		}
		// A melee weapon (empty ranged class) of the given size.
		inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 8}, "blade", nil, "", "", "", tt.weaponSize)
		if got := inst.Stats().DamageBonus; got != tt.want {
			t.Errorf("%s: DamageBonus = %d, want %d", tt.name, got, tt.want)
		}
	}
}

// A two-handed RANGED mob weapon does not get the melee two-handed Strength
// bonus (size-and-wielding §4.2 — the ranged Strength rule is a separate
// concern). A medium mob wielding a large bow is two-handed by size, but the
// melee bonus stays out of its ranged damage.
func TestMobStats_TwoHandedRangedExcluded(t *testing.T) {
	const str = 16
	s := NewStore()
	tpl := &mob.Template{
		ID: "test:archer", Name: "an archer", Type: "npc",
		Size:  "medium",
		Stats: map[string]int{"str": str},
	}
	inst, err := s.SpawnMob(tpl)
	if err != nil {
		t.Fatalf("SpawnMob: %v", err)
	}
	// A large (⇒ two-handed for a medium mob) PROJECTILE weapon.
	inst.SetWeapon(combat.DiceExpr{Count: 1, Sides: 6}, "longbow", nil, "projectile", "arrow", "", "large")
	if got := inst.Stats().DamageBonus; got != combat.STRBonus(str) {
		t.Errorf("two-handed ranged DamageBonus = %d, want %d (no melee two-handed bonus)", got, combat.STRBonus(str))
	}
}

package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/size"
)

// Stats() adds the two-handed Strength bonus (size-and-wielding §4.2) to the
// damage bonus when the wielded weapon resolves to a two-handed grip — and
// only the Strength contribution is scaled, not the base or any flat bonus.
func TestStats_TwoHandedStrengthBonus(t *testing.T) {
	const str = 16 // STRBonus(16) = (16-10)/2 = 3
	strBonus := combat.STRBonus(str)
	twoHandedExtra := size.TwoHandedStrBonus(strBonus, size.DefaultTwoHandedStrFactor)
	if twoHandedExtra <= 0 {
		t.Fatalf("test precondition: expected a positive two-handed extra, got %d", twoHandedExtra)
	}

	tests := []struct {
		name string
		mode size.WieldMode
		want int
	}{
		{"one-handed melee uses full 1x Strength", size.OneHanded, strBonus},
		{"light melee uses full 1x Strength", size.Light, strBonus},
		{"two-handed melee adds the extra Strength", size.TwoHanded, strBonus + twoHandedExtra},
	}
	for _, tt := range tests {
		a := &connActor{statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: str})}
		a.weapon.Store(&weaponInfo{dice: combat.DiceExpr{}, name: "blade", wieldMode: tt.mode})
		if got := a.Stats().DamageBonus; got != tt.want {
			t.Errorf("%s: DamageBonus = %d, want %d", tt.name, got, tt.want)
		}
	}
}

// A two-handed RANGED weapon does NOT get the melee two-handed Strength bonus
// (its Strength rule is the ranged concern). A projectile grants no positive
// Strength bonus at all, so the damage bonus floors to zero regardless of grip.
func TestStats_TwoHandedRangedExcluded(t *testing.T) {
	const str = 16
	a := &connActor{statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: str})}
	a.weapon.Store(&weaponInfo{name: "bow", wieldMode: size.TwoHanded, rangedClass: "projectile"})
	if got := a.Stats().DamageBonus; got != 0 {
		t.Errorf("two-handed projectile DamageBonus = %d, want 0 (no melee two-handed bonus, projectile Strength rule)", got)
	}
}

// Size() resolves from the actor's race; raceless ⇒ the baseline size.
func TestActorSize(t *testing.T) {
	a := &connActor{}
	if got := a.Size(); got != size.Baseline {
		t.Errorf("raceless Size() = %q, want baseline %q", got, size.Baseline)
	}
	a.race = &progression.Race{ID: "ogier", Size: "large"}
	if got := a.Size(); got != "large" {
		t.Errorf("Size() = %q, want large (from race)", got)
	}
	a.race = &progression.Race{ID: "human"} // no size declared
	if got := a.Size(); got != size.Baseline {
		t.Errorf("sizeless-race Size() = %q, want baseline %q", got, size.Baseline)
	}
}

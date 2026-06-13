package channel

import (
	"errors"
	"testing"
)

func TestMapping_Value(t *testing.T) {
	// A core-fantasy mapping reproducing today's combat derivation:
	// attack = hit_mod stat, defense = ac stat, damage_bonus = STR modifier.
	m, err := NewMapping(map[Channel]string{
		Attack:      "hit_mod",
		Defense:     "ac",
		DamageBonus: "mod(str)",
	})
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	lookup := statLookup(map[string]int{"hit_mod": 2, "ac": 15, "str": 16})

	if got := m.Value(Attack, lookup); got != 2 {
		t.Errorf("Attack = %d; want 2", got)
	}
	if got := m.Value(Defense, lookup); got != 15 {
		t.Errorf("Defense = %d; want 15", got)
	}
	if got := m.Value(DamageBonus, lookup); got != 3 {
		t.Errorf("DamageBonus = %d; want 3 (mod(16))", got)
	}
}

func TestMapping_UnmappedReadsDefault(t *testing.T) {
	m, _ := NewMapping(map[Channel]string{Attack: "hit_mod"})
	lookup := statLookup(map[string]int{"hit_mod": 5})

	// Defense unmapped → DefaultDefense (10), the legacy "no armor" AC.
	if got := m.Value(Defense, lookup); got != DefaultDefense {
		t.Errorf("unmapped Defense = %d; want %d", got, DefaultDefense)
	}
	// DamageBonus unmapped → 0.
	if got := m.Value(DamageBonus, lookup); got != 0 {
		t.Errorf("unmapped DamageBonus = %d; want 0", got)
	}
	if m.Has(DamageBonus) {
		t.Error("Has(DamageBonus) should be false")
	}
	if !m.Has(Attack) {
		t.Error("Has(Attack) should be true")
	}
}

func TestMapping_NilIsAllDefaults(t *testing.T) {
	var m *Mapping // nil
	if got := m.Value(Defense, nil); got != DefaultDefense {
		t.Errorf("nil mapping Defense = %d; want %d", got, DefaultDefense)
	}
	if got := m.Value(Attack, nil); got != 0 {
		t.Errorf("nil mapping Attack = %d; want 0", got)
	}
	if m.Has(Attack) {
		t.Error("nil mapping Has should be false")
	}
}

func TestNewMapping_BadFormulaErrors(t *testing.T) {
	_, err := NewMapping(map[Channel]string{
		Attack:  "hit_mod",
		Defense: "10 + bogus(", // malformed
	})
	if err == nil {
		t.Fatal("NewMapping with a malformed formula should error")
	}
	var me *MappingError
	if !errors.As(err, &me) {
		t.Fatalf("error should be a *MappingError, got %T", err)
	}
	if me.Channel != Defense {
		t.Errorf("MappingError.Channel = %q; want %q", me.Channel, Defense)
	}
}

// TestBaselineMapping_DamageBonusMatchesIntDivision pins that the baseline
// damage_bonus formula reproduces combat.STRBonus = (str-10)/2 under Go
// integer division (truncation toward zero) for all str — including the odd
// str<10 cases where floor(mod) would diverge. (Asserts the int-division
// value directly rather than importing combat, keeping the channel package
// a leaf in its tests too.)
func TestBaselineMapping_DamageBonusMatchesIntDivision(t *testing.T) {
	m := BaselineMapping()
	for _, str := range []int{7, 8, 9, 10, 11, 14, 15, 100} {
		lookup := statLookup(map[string]int{"str": str})
		want := (str - 10) / 2 // Go int division == combat.STRBonus
		if got := m.Value(DamageBonus, lookup); got != want {
			t.Errorf("baseline damage_bonus(str=%d) = %d; want %d", str, got, want)
		}
	}
	// Mitigation is intentionally unmapped in the baseline → 0.
	if got := m.Value(Mitigation, statLookup(nil)); got != 0 {
		t.Errorf("baseline mitigation = %d; want 0 (unmapped)", got)
	}
}

func TestNewMapping_EmptyIsValid(t *testing.T) {
	m, err := NewMapping(nil)
	if err != nil {
		t.Fatalf("NewMapping(nil) should not error: %v", err)
	}
	if got := m.Value(Defense, nil); got != DefaultDefense {
		t.Errorf("empty mapping Defense = %d; want %d", got, DefaultDefense)
	}
}

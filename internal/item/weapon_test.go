package item

import (
	"slices"
	"testing"
)

func TestWeaponTierVocabulary(t *testing.T) {
	for _, tier := range []string{"simple", "martial", "exotic"} {
		if !ValidTier(tier) {
			t.Errorf("ValidTier(%q) = false, want true", tier)
		}
	}
	if ValidTier("legendary") {
		t.Error("ValidTier(legendary) = true, want false (not a tier)")
	}
	// The empty string is "untiered" — not a tier name; callers treat
	// absence as LowestTier separately.
	if ValidTier("") {
		t.Error(`ValidTier("") = true, want false`)
	}
	if got, want := LowestTier(), "simple"; got != want {
		t.Errorf("LowestTier() = %q, want %q", got, want)
	}
	// Ordered low→high so a future graduated penalty can read distance.
	if got := WeaponTierNames(); !slices.Equal(got, []string{"simple", "martial", "exotic"}) {
		t.Errorf("WeaponTierNames() = %v, want ordered simple/martial/exotic", got)
	}
}

func TestDamageTypeVocabulary(t *testing.T) {
	for _, dt := range []string{"bludgeoning", "piercing", "slashing"} {
		if !ValidDamageType(dt) {
			t.Errorf("ValidDamageType(%q) = false, want true", dt)
		}
	}
	if ValidDamageType("fire") {
		t.Error("ValidDamageType(fire) = true, want false")
	}
	if ValidDamageType("") {
		t.Error(`ValidDamageType("") = true, want false`)
	}
}

package item

import (
	"slices"
	"testing"
)

func TestValidArmorTier(t *testing.T) {
	for _, ok := range []string{"light", "medium", "heavy"} {
		if !ValidArmorTier(ok) {
			t.Errorf("ValidArmorTier(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "plated", "simple", "Heavy"} {
		if ValidArmorTier(bad) {
			t.Errorf("ValidArmorTier(%q) = true, want false (empty/unknown/unnormalized)", bad)
		}
	}
}

func TestArmorProficient(t *testing.T) {
	tests := []struct {
		name      string
		granted   []string
		armorTier string
		want      bool
	}{
		{"untiered armor is always proficient", []string{"light"}, "", true},
		{"untiered armor with no grants is proficient", nil, "", true},
		{"granted tier is proficient", []string{"light", "medium"}, "light", true},
		{"ungranted tier is not proficient", []string{"light"}, "heavy", false},
		{"no grants: any tiered armor is non-proficient", nil, "light", false},
		{"exact-only grant", []string{"heavy"}, "heavy", true},
	}
	for _, tt := range tests {
		if got := ArmorProficient(tt.granted, tt.armorTier); got != tt.want {
			t.Errorf("%s: ArmorProficient(%v, %q) = %v, want %v",
				tt.name, tt.granted, tt.armorTier, got, tt.want)
		}
	}
}

func TestArmorTierNamesOrderedAndCopied(t *testing.T) {
	got := ArmorTierNames()
	if !slices.Equal(got, []string{"light", "medium", "heavy"}) {
		t.Fatalf("ArmorTierNames() = %v, want [light medium heavy] (ordered)", got)
	}
	// Returned slice is a copy: mutating it must not affect the vocabulary.
	got[0] = "mutated"
	if ArmorTierNames()[0] != "light" {
		t.Error("ArmorTierNames() returned an aliased slice; mutation leaked")
	}
}

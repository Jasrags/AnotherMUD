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

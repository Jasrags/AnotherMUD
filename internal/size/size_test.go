package size

import (
	"slices"
	"testing"
)

func TestValidAndNames(t *testing.T) {
	for _, ok := range []string{"tiny", "small", "medium", "large", "huge"} {
		if !Valid(ok) {
			t.Errorf("Valid(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "gigantic", "Medium", "med"} {
		if Valid(bad) {
			t.Errorf("Valid(%q) = true, want false", bad)
		}
	}
	got := Names()
	if !slices.Equal(got, []string{"tiny", "small", "medium", "large", "huge"}) {
		t.Fatalf("Names() = %v, want the ordered vocabulary", got)
	}
	got[0] = "mutated"
	if Names()[0] != "tiny" {
		t.Error("Names() returned an aliased slice; mutation leaked")
	}
}

// Distance is the signed step (weapon − wielder); empty/unknown ⇒ baseline.
func TestDistance(t *testing.T) {
	tests := []struct {
		weapon, wielder string
		want            int
	}{
		{"medium", "medium", 0},
		{"large", "medium", 1},
		{"huge", "medium", 2},
		{"small", "medium", -1},
		{"medium", "large", -1},
		{"", "medium", 0},      // empty weapon ⇒ baseline (medium)
		{"large", "", 1},       // empty wielder ⇒ baseline (medium)
		{"bogus", "medium", 0}, // unknown ⇒ baseline, defensive
		{"large", "bogus", 1},  // unknown wielder ⇒ baseline
	}
	for _, tt := range tests {
		if got := Distance(tt.weapon, tt.wielder); got != tt.want {
			t.Errorf("Distance(%q, %q) = %d, want %d", tt.weapon, tt.wielder, got, tt.want)
		}
	}
}

// Mode derives the wield mode from the signed size distance (§3).
func TestMode(t *testing.T) {
	tests := []struct {
		name            string
		weapon, wielder string
		want            WieldMode
	}{
		{"smaller is light", "small", "medium", Light},
		{"much smaller is light", "tiny", "large", Light},
		{"same size is one-handed", "medium", "medium", OneHanded},
		{"one step larger is two-handed", "large", "medium", TwoHanded},
		{"two steps larger is too large", "huge", "medium", TooLarge},
		{"legacy (no sizes) is one-handed", "", "", OneHanded},
		// Relativity: the same large weapon is two-handed for a medium wielder
		// but one-handed for a large one.
		{"large weapon, large wielder is one-handed", "large", "large", OneHanded},
		{"large weapon, small wielder is too large", "large", "small", TooLarge},
	}
	for _, tt := range tests {
		if got := Mode(tt.weapon, tt.wielder); got != tt.want {
			t.Errorf("%s: Mode(%q, %q) = %v, want %v", tt.name, tt.weapon, tt.wielder, got, tt.want)
		}
	}
}

// TwoHandedStrBonus returns the EXTRA Strength damage from two-handing —
// floor(strBonus × factor) − strBonus — so 1.5× a +3 Strength yields +1 extra.
func TestTwoHandedStrBonus(t *testing.T) {
	tests := []struct {
		strBonus int
		factor   float64
		want     int
	}{
		{3, 1.5, 1},   // floor(4.5)=4, extra 1
		{2, 1.5, 1},   // floor(3.0)=3, extra 1
		{4, 1.5, 2},   // floor(6.0)=6, extra 2
		{1, 1.5, 0},   // floor(1.5)=1, extra 0
		{0, 1.5, 0},   // no Strength, no extra
		{-1, 1.5, -1}, // floor(-1.5)=-2, extra -1 (a penalty worsens — d20-faithful)
		{3, 1.0, 0},   // factor 1 ⇒ no change
	}
	for _, tt := range tests {
		if got := TwoHandedStrBonus(tt.strBonus, tt.factor); got != tt.want {
			t.Errorf("TwoHandedStrBonus(%d, %v) = %d, want %d", tt.strBonus, tt.factor, got, tt.want)
		}
	}
}

func TestDefaultTwoHandedStrFactor(t *testing.T) {
	if DefaultTwoHandedStrFactor != 1.5 {
		t.Errorf("DefaultTwoHandedStrFactor = %v, want 1.5 (the WoT value)", DefaultTwoHandedStrFactor)
	}
}

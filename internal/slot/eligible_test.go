package slot

import "testing"

func TestIsEligible(t *testing.T) {
	tests := []struct {
		name     string
		eligible []string
		base     string
		want     bool
	}{
		{"exact match", []string{"wield"}, "wield", true},
		{"member of multi set", []string{"wield", "offhand"}, "offhand", true},
		{"not a member", []string{"wield"}, "head", false},
		{"empty set matches nothing", nil, "wield", false},
		{"case-insensitive base", []string{"wield"}, "WIELD", true},
		{"case-insensitive set", []string{"Offhand"}, "offhand", true},
		{"whitespace tolerant", []string{" wield "}, "wield", true},
		{"empty base never matches", []string{"wield"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEligible(tt.eligible, tt.base); got != tt.want {
				t.Errorf("IsEligible(%v, %q) = %v, want %v", tt.eligible, tt.base, got, tt.want)
			}
		})
	}
}

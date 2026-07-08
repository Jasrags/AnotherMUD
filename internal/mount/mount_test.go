package mount

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Temperament
	}{
		{"war", "war", War},
		{"steady", "steady", Steady},
		{"skittish", "skittish", Skittish},
		{"empty defaults", "", Default},
		{"unknown defaults", "bogus", Default},
		{"case-insensitive", "WAR", War},
		{"trims space", "  steady  ", Steady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.in); got != tt.want {
				t.Fatalf("Resolve(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValid(t *testing.T) {
	for _, ok := range []string{"war", "steady", "skittish", "WAR", " war "} {
		if !Valid(ok) {
			t.Errorf("Valid(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "bogus", "horse"} {
		if Valid(bad) {
			t.Errorf("Valid(%q) = true, want false", bad)
		}
	}
}

func TestToleratesDanger(t *testing.T) {
	tests := []struct {
		t    Temperament
		want bool
	}{
		{War, true},
		{Steady, true},
		{Skittish, false},
		{Default, false},         // Default == Skittish
		{Temperament(""), false}, // empty resolves to Default (skittish)
		{Temperament("bogus"), false},
	}
	for _, tt := range tests {
		if got := tt.t.ToleratesDanger(); got != tt.want {
			t.Errorf("%q.ToleratesDanger() = %v, want %v", tt.t, got, tt.want)
		}
	}
}

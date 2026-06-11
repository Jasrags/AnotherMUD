package feat

import "testing"

func TestCreditsForLevelChange(t *testing.T) {
	cases := []struct {
		name     string
		old, new int
		want     int
	}{
		{"1 to 2 — no boundary", 1, 2, 0},
		{"2 to 3 — crosses 3rd level", 2, 3, 1},
		{"3 to 4 — already past", 3, 4, 0},
		{"5 to 6 — crosses 6th", 5, 6, 1},
		{"1 to 6 — multi-level jump crosses 3 and 6", 1, 6, 2},
		{"6 to 9 — one boundary", 6, 9, 1},
		{"no change", 4, 4, 0},
		{"backward (de-level) earns nothing", 6, 3, 0},
		{"negative old floored at 0", -2, 3, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CreditsForLevelChange(c.old, c.new); got != c.want {
				t.Errorf("CreditsForLevelChange(%d, %d) = %d, want %d", c.old, c.new, got, c.want)
			}
		})
	}
}

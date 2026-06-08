package command

import "testing"

func TestDisciplineTier_MapShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		disc string
		want int
	}{
		{"map[string]any int", map[string]any{"smithing": 2}, "smithing", 2},
		{"map[string]any float (yaml)", map[string]any{"cooking": float64(1)}, "cooking", 1},
		{"map[string]int", map[string]int{"smithing": 3}, "smithing", 3},
		{"map[any]any", map[any]any{"smithing": 2}, "smithing", 2},
		{"absent discipline", map[string]any{"smithing": 2}, "cooking", 0},
		{"not a map", "nope", "smithing", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := disciplineTier(c.raw, c.disc); got != c.want {
				t.Errorf("disciplineTier(%v, %q) = %d, want %d", c.raw, c.disc, got, c.want)
			}
		})
	}
}

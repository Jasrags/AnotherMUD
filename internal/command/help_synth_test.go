package command

import "testing"

// TestSynthesizeSyntax pins the §8 syntax-line format: required `[name]`,
// optional `([name])`, bulk `[name | all | all.name]`, and a first
// preposition rendered in position.
func TestSynthesizeSyntax(t *testing.T) {
	cases := []struct {
		name string
		kw   string
		args []ArgDefinition
		want string
	}{
		{"required", "kill", []ArgDefinition{{Name: "target", Type: ArgEntity}}, "kill [target]"},
		{"optional", "look", []ArgDefinition{{Name: "target", Type: ArgKeyword, Optional: true}}, "look ([target])"},
		{"bulk", "get", []ArgDefinition{{Name: "item", Type: ArgRoomItem, Bulk: true}}, "get [item | all | all.item]"},
		{"optional bulk", "drop", []ArgDefinition{{Name: "item", Type: ArgInventory, Bulk: true, Optional: true}}, "drop ([item | all | all.item])"},
		{"preposition in position", "put", []ArgDefinition{
			{Name: "item", Type: ArgInventory},
			{Name: "container", Type: ArgContainer, Prepositions: []string{"in"}},
		}, "put [item] in [container]"},
		{"no args", "score", nil, "score"},
	}
	for _, tc := range cases {
		if got := synthesizeSyntax(tc.kw, tc.args); got != tc.want {
			t.Errorf("%s: synthesizeSyntax(%q) = %q, want %q", tc.name, tc.kw, got, tc.want)
		}
	}
}

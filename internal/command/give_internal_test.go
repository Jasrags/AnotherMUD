package command

import "testing"

// TestParseGiveArgs covers the parser corner cases the integration
// tests in give_test.go can't reach (parseGiveArgs is unexported).
// The reverse "to" scan is the load-bearing piece — the table pins
// what each shape resolves to so a future tweak (full-name target
// matching, ordinal targets) doesn't silently change parsing.
func TestParseGiveArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		args           []string
		item, target   string
		ok             bool
	}{
		{"two-token-implicit", []string{"sword", "bob"}, "sword", "bob", true},
		{"to-form", []string{"sword", "to", "bob"}, "sword", "bob", true},
		{"multi-word-item-to-form", []string{"red", "potion", "to", "alice"}, "red potion", "alice", true},
		// "potion of healing to alice" — rightmost "to" wins, item
		// name preserves the "of" connective. This is the corner the
		// code review flagged as untested.
		{"item-name-contains-of-before-to", []string{"potion", "of", "healing", "to", "alice"}, "potion of healing", "alice", true},
		// "give all to bob" — the all selector survives parsing
		// because the keyword resolver handles it downstream.
		{"all-selector", []string{"all", "to", "bob"}, "all", "bob", true},
		// "give sword bob the bard" — without "to", everything but
		// the last token is item; multi-word targets need "to".
		{"no-to-last-token-only", []string{"sword", "bob", "the", "bard"}, "sword bob the", "bard", true},

		// Failure cases — must return ok=false so the handler emits
		// the usage message.
		{"empty", nil, "", "", false},
		{"single-token", []string{"sword"}, "", "", false},
		{"trailing-to", []string{"sword", "to"}, "", "", false},
		// "give to bob" — args[0]="to" is never treated as the
		// preposition (the scan starts at i>=1), so this parses
		// as item="to" target="bob". Downstream keyword resolve
		// will then miss and the user sees "aren't carrying that".
		// Pinning the current behavior so a future "reject literal
		// 'to' as item" tweak is an intentional change.
		{"leading-to-falls-through-to-positional", []string{"to", "bob"}, "to", "bob", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			item, target, ok := parseGiveArgs(tc.args)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (item=%q target=%q)", ok, tc.ok, item, target)
			}
			if !ok {
				return
			}
			if item != tc.item {
				t.Errorf("item = %q, want %q", item, tc.item)
			}
			if target != tc.target {
				t.Errorf("target = %q, want %q", target, tc.target)
			}
		})
	}
}

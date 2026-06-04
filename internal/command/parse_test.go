package command

import (
	"slices"
	"testing"
)

func TestParseInput(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		cap  int
		want []string
	}{
		// §4.1 chaining
		{"single command", "look", 10, []string{"look"}},
		{"chain splits independently", "n;e;w;say done", 10,
			[]string{"n", "e", "w", "say done"}},
		{"empty segments dropped", "n;;; e ;", 10, []string{"n", "e"}},
		{"whitespace trimmed + collapsed", "  get   sword  ", 10, []string{"get sword"}},
		{"cap drops trailing segments", "a;b;c;d;e", 3, []string{"a", "b", "c"}},

		// §4.2 repeat expansion
		{"repeat 3n", "3n", 10, []string{"n", "n", "n"}},
		{"repeat 12east", "12east", 20,
			[]string{"east", "east", "east", "east", "east", "east", "east", "east", "east", "east", "east", "east"}},
		{"repeat with args", "2pick item", 10, []string{"pick item", "pick item"}},
		{"pure digits not expanded", "3", 10, []string{"3"}},
		{"count zero runs once", "0n", 10, []string{"n"}},
		{"count one runs once", "1n", 10, []string{"n"}},
		{"repeat bounded by cap", "999n", 4, []string{"n", "n", "n", "n"}},
		{"overflow count bounded by cap", "99999999999999999999n", 3, []string{"n", "n", "n"}},

		// repeat + chain interplay; cap counts expanded commands
		{"repeat inside chain", "2n;e", 10, []string{"n", "n", "e"}},
		{"repeat fills the cap then drops rest", "3n;e;w", 3, []string{"n", "n", "n"}},

		// §4.3 no quoting — quotes are literal tokens
		{"no quoting interpreted", `say "hello there"`, 10, []string{`say "hello there"`}},

		// edge: empty / whitespace-only input
		{"empty line", "", 10, nil},
		{"only separators", ";;;", 10, nil},

		// cap fallback
		{"non-positive cap uses default", "a;b", 0, []string{"a", "b"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseInput(tc.raw, tc.cap)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("ParseInput(%q, %d) = %v, want %v", tc.raw, tc.cap, got, tc.want)
			}
		})
	}
}

func TestSplitRepeat(t *testing.T) {
	cases := []struct {
		token string
		count int
		verb  string
		isRep bool
	}{
		{"3n", 3, "n", true},
		{"12east", 12, "east", true},
		{"0n", 0, "n", true},
		{"3", 0, "", false},  // all digits, no suffix
		{"n", 0, "", false},  // no leading digit
		{"", 0, "", false},   // empty
		{"n3", 0, "", false}, // digit not leading
		{"2pick", 2, "pick", true},
	}
	for _, tc := range cases {
		count, verb, isRep := splitRepeat(tc.token)
		if count != tc.count || verb != tc.verb || isRep != tc.isRep {
			t.Errorf("splitRepeat(%q) = (%d, %q, %v), want (%d, %q, %v)",
				tc.token, count, verb, isRep, tc.count, tc.verb, tc.isRep)
		}
	}
}

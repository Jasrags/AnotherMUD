package command

import (
	"strings"
	"testing"
)

// parseOrder must disambiguate order keywords that also appear in hireling or
// target names (hireable-mobs.md §8): attack anchors its target, and the no-arg
// stances are terminal.
func TestParseOrder(t *testing.T) {
	for _, tc := range []struct {
		in        string
		hireling  string
		order     string
		orderArgs string
		ok        bool
	}{
		{"sellsword stay", "sellsword", "stay", "", true},
		{"sellsword follow", "sellsword", "follow", "", true},
		{"guard", "", "guard", "", true},                  // sole-hireling stance
		{"follow", "", "follow", "", true},                // sole-hireling stance
		{"town guard follow", "town guard", "follow", "", true}, // name contains a keyword
		{"sellsword attack rat", "sellsword", "attack", "rat", true},
		{"sellsword attack guard", "sellsword", "attack", "guard", true}, // target is a keyword
		{"town guard attack rat", "town guard", "attack", "rat", true},   // name + attack
		{"attack rat", "", "attack", "rat", true},                        // sole hireling, attack
		{"", "", "", "", false},                                          // nothing
		{"sellsword", "", "", "", false},                                 // no command
		{"sellsword dance", "", "", "", false},                           // unknown command
	} {
		t.Run(tc.in, func(t *testing.T) {
			h, o, oa, ok := parseOrder(strings.Fields(tc.in))
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if h != tc.hireling || o != tc.order || strings.Join(oa, " ") != tc.orderArgs {
				t.Errorf("parseOrder(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.in, h, o, strings.Join(oa, " "), tc.hireling, tc.order, tc.orderArgs)
			}
		})
	}
}

package command

import (
	"strings"
	"testing"
)

// recentCompleted must take the last N completed quests, most-recent-first,
// across the truncation boundary (the reviewer-flagged off-by-one surface).
func TestRecentCompleted_TruncationAndOrder(t *testing.T) {
	ids := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = string(rune('a' + i)) // a, b, c, … in completion order
		}
		return out
	}
	tests := []struct {
		name string
		in   []string
		max  int
		want string // joined, most-recent-first
	}{
		{"empty", nil, 5, ""},
		{"under cap", ids(4), 5, "d,c,b,a"},
		{"exactly cap", ids(5), 5, "e,d,c,b,a"},
		{"one over cap", ids(6), 5, "f,e,d,c,b"}, // 'a' dropped
		{"well over cap", ids(11), 5, "k,j,i,h,g"},
		{"single", ids(1), 5, "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(recentCompleted(tc.in, tc.max), ",")
			if got != tc.want {
				t.Errorf("recentCompleted(%v, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}

package session

import "testing"

func TestSanitizeForLog(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain ascii", "look north", "look north"},
		{"tab preserved", "hi\tthere", "hi\tthere"},
		{"strips bell", "ring\abell", "ring�bell"},
		{"strips ansi escape", "\x1b[31mred\x1b[0m", "�[31mred�[0m"},
		{"strips null", "ab\x00cd", "ab�cd"},
		{"replaces invalid utf8", "ab\xffcd", "ab�cd"},
		{"strips DEL", "a\x7fb", "a�b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeForLog(c.in)
			if got != c.want {
				t.Fatalf("sanitizeForLog(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

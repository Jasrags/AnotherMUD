package render

import "testing"

func TestResolveFgColor(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"basic red", "red", "\x1b[31m"},
		{"case insensitive", "RED", "\x1b[31m"},
		{"bright hyphen", "bright-red", "\x1b[91m"},
		{"bright underscore", "bright_red", "\x1b[91m"},
		{"dark-gray alias", "dark-gray", "\x1b[90m"},
		{"unknown", "chartreuse", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveFgColor(tt.in); got != tt.want {
				t.Errorf("ResolveFgColor(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveBgColor(t *testing.T) {
	if got := ResolveBgColor("black"); got != "\x1b[40m" {
		t.Errorf("bg black = %q", got)
	}
	if got := ResolveBgColor("bright_blue"); got != "\x1b[104m" {
		t.Errorf("bg bright_blue = %q", got)
	}
	if got := ResolveBgColor("nope"); got != "" {
		t.Errorf("bg unknown = %q, want empty", got)
	}
}

func TestResolveBrace(t *testing.T) {
	tests := []struct {
		token   string
		want    string
		isReset bool
		ok      bool
	}{
		{"yellow", "\x1b[33m", false, true},
		{"bright_red", "\x1b[91m", false, true},
		{"r", "\x1b[31m", false, true},   // ROM back-compat
		{"R", "\x1b[91m", false, true},   // ROM bright (case-sensitive)
		{"x", Reset, true, true},         // ROM reset
		{"reset", Reset, true, true},     // named reset
		{"/", Reset, true, true},         // slash synonym
		{"bold", "\x1b[1m", false, true}, //
		{"dim", "\x1b[2m", false, true},
		{"frobnitz", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			code, isReset, ok := resolveBrace(tt.token)
			if code != tt.want || isReset != tt.isReset || ok != tt.ok {
				t.Errorf("resolveBrace(%q) = (%q,%v,%v), want (%q,%v,%v)",
					tt.token, code, isReset, ok, tt.want, tt.isReset, tt.ok)
			}
		})
	}
}

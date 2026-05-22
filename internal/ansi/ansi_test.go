package ansi

import (
	"strings"
	"testing"
)

func TestRenderColor(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello world", "hello world"},
		{"single dim", "{r}red{x}", "\x1b[31mred\x1b[0m"},
		{
			name: "single bright",
			in:   "{R}bright red{x}",
			want: "\x1b[91mbright red\x1b[0m",
		},
		{
			name: "auto-reset when unclosed",
			in:   "{g}forgot to close",
			want: "\x1b[32mforgot to close\x1b[0m",
		},
		{
			name: "no reset when no color",
			in:   "no color codes at all",
			want: "no color codes at all",
		},
		{
			// '{{' escapes the opening brace; '}' is never special on its
			// own, so it survives unchanged in surrounding text.
			name: "literal brace",
			in:   "use {{R} to mean bright red",
			want: "use {R} to mean bright red",
		},
		{
			name: "unknown code passes through",
			in:   "{z}weird",
			want: "{z}weird",
		},
		{
			name: "multi-char code passes through",
			in:   "{abc}weird",
			want: "{abc}weird",
		},
		{
			name: "trailing brace at end",
			in:   "ends with {",
			want: "ends with {",
		},
		{
			name: "all eight dim",
			in:   "{k}{r}{g}{y}{b}{m}{c}{w}",
			want: "\x1b[30m\x1b[31m\x1b[32m\x1b[33m\x1b[34m\x1b[35m\x1b[36m\x1b[37m\x1b[0m",
		},
		{
			name: "explicit reset not double-appended",
			in:   "{r}hi{x}",
			want: "\x1b[31mhi\x1b[0m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(tt.in, true)
			if got != tt.want {
				t.Errorf("Render(%q, true) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRenderStrip(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "hello", "hello"},
		{"strips codes", "{r}red{x} word", "red word"},
		{"keeps unknown codes", "{z}leave alone", "{z}leave alone"},
		{"literal brace preserved", "use {{R} here", "use {R} here"},
		{
			name: "no escape bytes in output",
			in:   "{R}bright{x} {g}then dim{x}",
			want: "bright then dim",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(tt.in, false)
			if got != tt.want {
				t.Errorf("Render(%q, false) = %q, want %q", tt.in, got, tt.want)
			}
			if strings.ContainsRune(got, '\x1b') {
				t.Errorf("Render(_, false) leaked escape byte: %q", got)
			}
		})
	}
}

func TestRenderDropsRawESC(t *testing.T) {
	// Pack content may be third-party; arbitrary SGR bytes smuggled
	// past the markup grammar must not reach the wire in either mode.
	// The security property is that no ESC (0x1B) byte appears in the
	// output. Surrounding CSI text without a leading ESC is harmless
	// (visible junk, not an SGR), so we only assert the safety
	// invariant — not output cleanliness.
	smuggled := "before\x1b[31mhidden\x1b[0mafter"

	for _, enabled := range []bool{true, false} {
		got := Render(smuggled, enabled)
		if strings.ContainsRune(got, 0x1B) {
			t.Errorf("Render(_, %v) leaked ESC byte: %q", enabled, got)
		}
		if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
			t.Errorf("Render(_, %v) lost surrounding text: %q", enabled, got)
		}
	}
}

func TestRenderExplicitResetMidStringNoTrailingReset(t *testing.T) {
	// Author closed the color before plain trailing text — no extra
	// Reset should be appended at end of string.
	got := Render("{r}hi{x} done", true)
	want := "\x1b[31mhi\x1b[0m done"
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestRenderEmpty(t *testing.T) {
	if got := Render("", true); got != "" {
		t.Errorf("Render(\"\", true) = %q, want empty", got)
	}
	if got := Render("", false); got != "" {
		t.Errorf("Render(\"\", false) = %q, want empty", got)
	}
}

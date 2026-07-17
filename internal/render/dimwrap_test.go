package render

import (
	"strings"
	"testing"
)

func TestDimWrap(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain text is one dim run", "a quiet plaza", "{dim}a quiet plaza{/}"},
		{
			"highlight is left bare between dim runs",
			"A dead <feature>fountain</feature> holds rainwater",
			"{dim}A dead {/}<feature>fountain</feature>{dim} holds rainwater{/}",
		},
		{
			"multi-word highlight content is not dimmed",
			"a <threat>Knight Errant patrol</threat> lingers",
			"{dim}a {/}<threat>Knight Errant patrol</threat>{dim} lingers{/}",
		},
		{
			"adjacent highlights leave no empty dim run",
			"<exit>north</exit><exit>south</exit>",
			"<exit>north</exit><exit>south</exit>",
		},
		{
			"leading highlight then prose",
			"<cmd>`ask`</cmd> the fixer",
			"<cmd>`ask`</cmd>{dim} the fixer{/}",
		},
		{
			"unmatched < is literal prose",
			"faster < slower",
			"{dim}faster < slower{/}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DimWrap(tt.in); got != tt.want {
				t.Errorf("DimWrap(%q) =\n  %q\nwant\n  %q", tt.in, got, tt.want)
			}
		})
	}
}

// The whole point: after DimWrap, a highlight renders at FULL color (not
// dimmed) and the prose around it renders faint — verified through the
// real color renderer, not just the markup shape.
func TestDimWrapThenColorPops(t *testing.T) {
	theme := NewThemeRegistry()
	theme.Register("feature", ThemeEntry{FG: "yellow"})
	theme.Compile()
	cr := NewColorRenderer(theme)

	got := cr.RenderAnsi(DimWrap("A <feature>fountain</feature> holds"))

	dim := "\x1b[2m"
	amber := ResolveFgColor("yellow")
	// Prose is faint; the feature carries the amber SGR with NO faint code
	// immediately preceding it (it was reset out first), so it pops.
	if !strings.Contains(got, dim+"A ") {
		t.Errorf("prose before the feature should be dim: %q", got)
	}
	if !strings.Contains(got, Reset+amber+"fountain") {
		t.Errorf("feature should render at full amber after a reset (not dimmed): %q", got)
	}
	if !strings.Contains(got, "fountain"+Reset+dim+" holds") {
		t.Errorf("prose should return to dim after the feature: %q", got)
	}
}

// On a no-color client, DimWrap output strips back to clean prose — the
// dim markup and the highlight tags both vanish.
func TestDimWrapStripsToCleanProse(t *testing.T) {
	theme := NewThemeRegistry()
	theme.Register("feature", ThemeEntry{FG: "yellow"})
	theme.Compile()
	cr := NewColorRenderer(theme)

	got := cr.RenderPlain(DimWrap("A dead <feature>fountain</feature> holds rainwater"))
	if got != "A dead fountain holds rainwater" {
		t.Errorf("plain render = %q, want clean prose", got)
	}
}

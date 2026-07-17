package render

import (
	"strings"
	"testing"
)

// cmdTheme is a minimal theme carrying the `cmd` entry the backtick command
// highlight resolves against (bright-green in the shipped core theme).
func cmdTheme() *ThemeRegistry {
	r := NewThemeRegistry()
	r.Register("cmd", ThemeEntry{FG: "bright-green"})
	r.Register("feature", ThemeEntry{FG: "yellow"})
	r.Compile()
	return r
}

func TestCommandSpanEnd(t *testing.T) {
	cases := []struct {
		name string
		s    string
		i    int
		want int
	}{
		{"simple", "`ask`", 0, 4},
		{"multiword", "`ask rook`", 0, 9},
		{"midprose", "type `look` now", 5, 10},
		{"empty span", "``", 0, -1},
		{"lone backtick", "just ` here", 5, -1},
		{"newline inside", "`ask\nrook`", 0, -1},
		{"tag inside", "`a<b>`", 0, -1},
		{"esc inside", "`a\x1bb`", 0, -1},
		{"not a backtick", "ask", 0, -1},
		{"too long", "`" + strings.Repeat("x", maxCommandSpan+1) + "`", 0, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandSpanEnd(tc.s, tc.i); got != tc.want {
				t.Errorf("commandSpanEnd(%q,%d) = %d, want %d", tc.s, tc.i, got, tc.want)
			}
		})
	}
}

func TestRender_BacktickCommandGetsCmdColor(t *testing.T) {
	r := NewColorRenderer(cmdTheme())
	pair, ok := r.theme.ResolveForTier("cmd", ColorTierBasic)
	if !ok {
		t.Fatal("cmd theme entry should resolve")
	}
	out := r.RenderAnsi("First `ask` around, then `ask rook` for work.")

	// Both spans wrapped in the cmd color, backticks kept, color reset after.
	for _, span := range []string{"`ask`", "`ask rook`"} {
		wrapped := pair.Open + span + Reset
		if !strings.Contains(out, wrapped) {
			t.Errorf("want %q wrapped as %q in:\n%q", span, wrapped, out)
		}
	}
}

func TestRender_BacktickPlainModeKeepsLiteral(t *testing.T) {
	r := NewColorRenderer(cmdTheme())
	in := "First `ask` around."
	if got := r.RenderPlain(in); got != in {
		t.Errorf("plain mode must keep backticks verbatim: got %q, want %q", got, in)
	}
}

func TestRender_BacktickNoCmdThemePassesLiteral(t *testing.T) {
	// A theme without `cmd` leaves the span uncolored (backticks intact).
	r := NewColorRenderer(newTestTheme())
	in := "First `ask` around."
	out := r.RenderAnsi(in)
	if strings.Contains(out, "\x1b[") {
		t.Errorf("no cmd entry should mean no SGR codes; got %q", out)
	}
	if !strings.Contains(out, "`ask`") {
		t.Errorf("backticks should survive; got %q", out)
	}
}

func TestRender_LoneBacktickIsLiteral(t *testing.T) {
	r := NewColorRenderer(cmdTheme())
	out := r.RenderAnsi("a lone ` backtick")
	if out != "a lone ` backtick" {
		t.Errorf("lone backtick should pass through; got %q", out)
	}
}

func TestRender_BacktickWithEscIsNotBulkCopied(t *testing.T) {
	// Security: a backtick span holding a raw ESC must NOT be bulk-copied
	// verbatim (that would smuggle the ESC past the per-byte drop). The span
	// is rejected as a command, and the ESC is stripped on the byte path.
	r := NewColorRenderer(cmdTheme())
	out := r.RenderAnsi("evil `a\x1b[31mx` here")
	if strings.Contains(out, "\x1b[31m") {
		t.Errorf("smuggled ESC leaked into output: %q", out)
	}
}

func TestRender_BacktickInsideColorTagDoesNotClobber(t *testing.T) {
	// A `verb` nested inside an open color keeps the enclosing color for the
	// whole tag body — no mid-span reset. The backticks stay literal there.
	th := cmdTheme()
	r := NewColorRenderer(th)
	threat, _ := th.ResolveForTier("feature", ColorTierBasic) // stand-in open color
	out := r.RenderAnsi("<feature>the `blade` gleams</feature>")
	// Exactly one Reset (the tag close), not one from the backtick span too.
	if n := strings.Count(out, Reset); n != 1 {
		t.Errorf("want a single reset (tag close), got %d in %q", n, out)
	}
	// The whole tag body sits under the feature color; backticks retained.
	if !strings.Contains(out, threat.Open+"the `blade` gleams"+Reset) {
		t.Errorf("enclosing color should span the full body: %q", out)
	}
}

func TestDimWrap_LeavesCommandSpanBare(t *testing.T) {
	// A `verb` span in dim prose is emitted bare so it pops at full color.
	got := DimWrap("Try `look` around")
	want := "{dim}Try {/}`look`{dim} around{/}"
	if got != want {
		t.Errorf("DimWrap command span:\n got %q\nwant %q", got, want)
	}
}

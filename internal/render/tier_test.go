package render

import (
	"strings"
	"testing"
)

// tieredTheme constructs a registry with three theme entries
// exercising the per-tier compilation logic:
//   - `flat` has only FG (ANSI-16) — same SGR at every tier.
//   - `rich` has FG + HTML — Basic uses FG, Extended/TrueColor
//     upgrade via HTML.
//   - `htmlonly` has only HTML — Basic skips (no ANSI-16 fallback);
//     Extended/TrueColor still emit.
func tieredTheme(t *testing.T) *ThemeRegistry {
	t.Helper()
	reg := NewThemeRegistry()
	reg.Register("flat", ThemeEntry{FG: "red"})
	reg.Register("rich", ThemeEntry{FG: "red", HTML: "#FF8000"})
	reg.Register("htmlonly", ThemeEntry{HTML: "#00FF80"})
	reg.Compile()
	return reg
}

func TestResolveForTier_BasicUsesAnsi16(t *testing.T) {
	reg := tieredTheme(t)
	pair, ok := reg.ResolveForTier("flat", ColorTierBasic)
	if !ok {
		t.Fatal("flat at Basic should resolve")
	}
	if pair.Open != "\x1b[31m" {
		t.Errorf("flat Basic Open = %q, want \\x1b[31m (red)", pair.Open)
	}
}

func TestResolveForTier_RichDegradesToAnsi16AtBasic(t *testing.T) {
	reg := tieredTheme(t)
	pair, ok := reg.ResolveForTier("rich", ColorTierBasic)
	if !ok || pair.Open != "\x1b[31m" {
		t.Errorf("rich Basic Open = %q (ok=%v), want \\x1b[31m", pair.Open, ok)
	}
}

func TestResolveForTier_RichUsesTrueColorWhenAvailable(t *testing.T) {
	reg := tieredTheme(t)
	pair, ok := reg.ResolveForTier("rich", ColorTierTrueColor)
	if !ok {
		t.Fatal("rich at TrueColor should resolve")
	}
	want := "\x1b[38;2;255;128;0m"
	if pair.Open != want {
		t.Errorf("rich TrueColor Open = %q, want %q", pair.Open, want)
	}
}

func TestResolveForTier_RichUsesExtendedAt256(t *testing.T) {
	reg := tieredTheme(t)
	pair, ok := reg.ResolveForTier("rich", ColorTierExtended)
	if !ok {
		t.Fatal("rich at Extended should resolve")
	}
	// #FF8000 = (255, 128, 0); nearest cube level for 128 is 135
	// (level index 2); for 255 = level 5; for 0 = level 0.
	// 16 + 36*5 + 6*2 + 0 = 16 + 180 + 12 + 0 = 208.
	want := "\x1b[38;5;208m"
	if pair.Open != want {
		t.Errorf("rich Extended Open = %q, want %q", pair.Open, want)
	}
}

func TestResolveForTier_HtmlOnlyMissingAtBasic(t *testing.T) {
	reg := tieredTheme(t)
	if _, ok := reg.ResolveForTier("htmlonly", ColorTierBasic); ok {
		t.Errorf("htmlonly should NOT resolve at Basic (no ANSI-16 fallback)")
	}
}

func TestResolveForTier_HtmlOnlyResolvesAtExtended(t *testing.T) {
	reg := tieredTheme(t)
	pair, ok := reg.ResolveForTier("htmlonly", ColorTierExtended)
	if !ok {
		t.Fatal("htmlonly at Extended should resolve")
	}
	if !strings.HasPrefix(pair.Open, "\x1b[38;5;") {
		t.Errorf("htmlonly Extended Open = %q, want 256-color SGR", pair.Open)
	}
}

func TestResolveForTier_None_AlwaysMisses(t *testing.T) {
	reg := tieredTheme(t)
	for _, tag := range []string{"flat", "rich", "htmlonly"} {
		if _, ok := reg.ResolveForTier(tag, ColorTierNone); ok {
			t.Errorf("ResolveForTier(%q, None) should miss", tag)
		}
	}
}

func TestResolve_BackCompatUsesBasic(t *testing.T) {
	// The tier-less Resolve must keep returning the Basic-tier
	// pair so any pre-M16.6b caller continues to see ANSI-16.
	reg := tieredTheme(t)
	pair, ok := reg.Resolve("rich")
	if !ok || pair.Open != "\x1b[31m" {
		t.Errorf("Resolve(rich) = (%q, %v), want (\\x1b[31m, true)", pair.Open, ok)
	}
}

func TestRenderAnsiForTier_DispatchesByTier(t *testing.T) {
	reg := tieredTheme(t)
	r := NewColorRenderer(reg)

	in := "<rich>hi</rich>"
	basic := r.RenderAnsiForTier(in, ColorTierBasic)
	extended := r.RenderAnsiForTier(in, ColorTierExtended)
	trueColor := r.RenderAnsiForTier(in, ColorTierTrueColor)
	none := r.RenderAnsiForTier(in, ColorTierNone)
	plain := r.RenderPlain(in)

	if !strings.Contains(basic, "\x1b[31m") {
		t.Errorf("Basic rendering missing ANSI-16 red: %q", basic)
	}
	if !strings.Contains(extended, "\x1b[38;5;") {
		t.Errorf("Extended rendering missing 256-color SGR: %q", extended)
	}
	if !strings.Contains(trueColor, "\x1b[38;2;") {
		t.Errorf("TrueColor rendering missing truecolor SGR: %q", trueColor)
	}
	if none != plain {
		t.Errorf("None rendering = %q, want plain = %q", none, plain)
	}
}

func TestRenderAnsiForTier_CacheIsTierAware(t *testing.T) {
	reg := tieredTheme(t)
	r := NewColorRenderer(reg)

	in := "<rich>hi</rich>"
	a := r.RenderAnsiForTier(in, ColorTierBasic)
	b := r.RenderAnsiForTier(in, ColorTierTrueColor)
	if a == b {
		t.Errorf("Basic and TrueColor renderings should differ for %q", in)
	}

	// Both should be cached under distinct keys.
	if _, ok := r.ansiCache.Load(tieredCacheKey{s: in, tier: ColorTierBasic}); !ok {
		t.Error("Basic cache entry missing")
	}
	if _, ok := r.ansiCache.Load(tieredCacheKey{s: in, tier: ColorTierTrueColor}); !ok {
		t.Error("TrueColor cache entry missing")
	}
}

func TestRenderAnsi_BackCompatStillReturnsBasic(t *testing.T) {
	reg := tieredTheme(t)
	r := NewColorRenderer(reg)
	got := r.RenderAnsi("<rich>hi</rich>")
	if !strings.Contains(got, "\x1b[31m") {
		t.Errorf("RenderAnsi should default to Basic ANSI-16: %q", got)
	}
}

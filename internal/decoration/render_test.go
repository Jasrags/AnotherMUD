package decoration

import (
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

// InlineMarkup wraps the visible tag in the item.<key> semantic tag; a
// blank tier produces no markup.
func TestInlineMarkup(t *testing.T) {
	if got := rareTier().InlineMarkup(); got != "<item.rare>[RARE]</item.rare>" {
		t.Errorf("InlineMarkup() = %q", got)
	}
	blank := Tier{Key: "common", Order: 10} // invisible, no display
	if got := blank.InlineMarkup(); got != "" {
		t.Errorf("blank InlineMarkup() = %q, want \"\"", got)
	}
}

// PaddedMarkup centers the tag in the column width; a blank tier becomes
// width spaces so the column stays aligned.
func TestPaddedMarkup(t *testing.T) {
	// "[RARE]" is 6 wide; in a column of 8 → 1 space each side.
	got := rareTier().PaddedMarkup(8)
	want := " <item.rare>[RARE]</item.rare> "
	if got != want {
		t.Errorf("PaddedMarkup(8) = %q, want %q", got, want)
	}
	// Odd remainder biases the extra space to the right: width 9, text 6 →
	// left 1, right 2.
	got = rareTier().PaddedMarkup(9)
	want = " <item.rare>[RARE]</item.rare>  "
	if got != want {
		t.Errorf("PaddedMarkup(9) = %q, want %q", got, want)
	}
	// A blank tier pads to width spaces (no tag).
	blank := Tier{Key: "common"}
	if got := blank.PaddedMarkup(8); got != strings.Repeat(" ", 8) {
		t.Errorf("blank PaddedMarkup(8) = %q, want 8 spaces", got)
	}
	// width ≤ 0 degrades to inline.
	if got := rareTier().PaddedMarkup(0); got != rareTier().InlineMarkup() {
		t.Errorf("PaddedMarkup(0) = %q, want inline", got)
	}
}

// Essence.Markup wraps the glyph in the essence.<key> tag.
func TestEssenceMarkup(t *testing.T) {
	e := Essence{Key: "fire", Glyph: "✦", Color: render.ThemeEntry{FG: "red"}}
	if got := e.Markup(); got != "<essence.fire>(✦)</essence.fire>" {
		t.Errorf("Markup() = %q", got)
	}
	if got := (Essence{Key: "void"}).Markup(); got != "" {
		t.Errorf("no-glyph Markup() = %q, want \"\"", got)
	}
}

// End-to-end: after RegisterTheme + Compile, a marker's markup renders to
// color in ANSI mode and strips to its visible text in plain mode — and the
// raw tag never leaks in either mode.
func TestRenderThroughTheme(t *testing.T) {
	rarity := NewRarityRegistry()
	rarity.Register(rareTier()) // Color FG=blue

	theme := render.NewThemeRegistry()
	rarity.RegisterTheme(theme)
	theme.Compile()
	rdr := render.NewColorRenderer(theme)

	markup := rareTier().InlineMarkup() // <item.rare>[RARE]</item.rare>

	plain := rdr.RenderPlain(markup)
	if plain != "[RARE]" {
		t.Errorf("plain render = %q, want %q", plain, "[RARE]")
	}

	ansi := rdr.RenderAnsiForTier(markup, render.ColorTierBasic)
	if !strings.Contains(ansi, "[RARE]") {
		t.Errorf("ansi render %q missing visible text", ansi)
	}
	if strings.Contains(ansi, "<item.rare>") || strings.Contains(plain, "<item.rare>") {
		t.Errorf("raw tag leaked: ansi=%q plain=%q", ansi, plain)
	}
	if !strings.Contains(ansi, "\x1b[") {
		t.Errorf("ansi render %q carries no SGR escape (color not applied)", ansi)
	}
}

// Guard: WITHOUT RegisterTheme the tag is unknown to the renderer and its
// raw markup leaks — this is exactly why RegisterTheme is mandatory.
func TestUnregisteredTagLeaks(t *testing.T) {
	theme := render.NewThemeRegistry()
	theme.Compile() // nothing registered
	rdr := render.NewColorRenderer(theme)

	plain := rdr.RenderPlain(rareTier().InlineMarkup())
	if !strings.Contains(plain, "<item.rare>") {
		t.Errorf("expected unregistered tag to leak, got %q", plain)
	}
}

// A color-less (declared but no FG/BG/HTML) tier still renders its visible
// text plain — the tag is known, so it strips rather than leaking.
func TestColorlessTierStripsCleanly(t *testing.T) {
	colorless := Tier{Key: "plain", Order: 20, Display: "P", Left: "[", Right: "]", Visible: true}
	rarity := NewRarityRegistry()
	rarity.Register(colorless)

	theme := render.NewThemeRegistry()
	rarity.RegisterTheme(theme)
	theme.Compile()
	rdr := render.NewColorRenderer(theme)

	ansi := rdr.RenderAnsiForTier(colorless.InlineMarkup(), render.ColorTierBasic)
	if ansi != "[P]" {
		t.Errorf("color-less tier ansi = %q, want %q (known tag, no color)", ansi, "[P]")
	}
}

// Essence renders through the theme the same way: registered → colored in
// ANSI, stripped to glyph in plain.
func TestEssenceRenderThroughTheme(t *testing.T) {
	ess := NewEssenceRegistry()
	ess.Register(Essence{Key: "fire", Glyph: "✦", Color: render.ThemeEntry{FG: "red"}})

	theme := render.NewThemeRegistry()
	ess.RegisterTheme(theme)
	theme.Compile()
	rdr := render.NewColorRenderer(theme)

	markup := Essence{Key: "fire", Glyph: "✦"}.Markup()
	if plain := rdr.RenderPlain(markup); plain != "(✦)" {
		t.Errorf("plain = %q, want %q", plain, "(✦)")
	}
	if ansi := rdr.RenderAnsiForTier(markup, render.ColorTierBasic); !strings.Contains(ansi, "\x1b[") {
		t.Errorf("ansi %q carries no SGR escape", ansi)
	}
}

// A visible tag at least as wide as the column emits without padding.
func TestPaddedMarkup_NoRoomToPad(t *testing.T) {
	// "[RARE]" is 6 wide; a column of 4 leaves no room → bare inline.
	if got := rareTier().PaddedMarkup(4); got != rareTier().InlineMarkup() {
		t.Errorf("PaddedMarkup(4) = %q, want bare inline (no padding)", got)
	}
}

// nil theme is a no-op for both registries (defensive — no panic).
func TestRegisterTheme_NilThemeNoOp(t *testing.T) {
	r := NewRarityRegistry()
	r.Register(rareTier())
	r.RegisterTheme(nil) // must not panic
	e := NewEssenceRegistry()
	e.Register(Essence{Key: "fire", Glyph: "✦"})
	e.RegisterTheme(nil) // must not panic
}

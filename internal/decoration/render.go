package decoration

import (
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

// Semantic theme-tag prefixes (item-decorations §4). A marker renders as
// `<item.<rarity-key>>…</item.<rarity-key>>` / `<essence.<key>>…</…>` — the
// angle-bracket semantic-tag form the color renderer resolves through the
// theme registry (render.ColorRenderer). Keeping color in the theme (not on
// the item) is what lets a re-themed pack recolor decorations without
// touching item data (§4).
const (
	rarityTagPrefix  = "item."
	essenceTagPrefix = "essence."
)

func rarityTag(key string) string  { return rarityTagPrefix + key }
func essenceTag(key string) string { return essenceTagPrefix + key }

// wrapTag wraps content in the angle-bracket semantic tag the renderer
// understands: <tag>content</tag>.
func wrapTag(tag, content string) string {
	return "<" + tag + ">" + content + "</" + tag + ">"
}

// InlineMarkup returns the tier's decorated tag wrapped in its semantic
// theme tag — e.g. "<item.rare>[RARE]</item.rare>" — for inline display
// next to an item name (item-decorations §4). A tier that renders as
// nothing (§2) returns "".
//
// The result is MARKUP, not final output: the caller composes it into a
// line and runs the whole line through render.ColorRenderer, which resolves
// the theme tag to color (ANSI mode) or strips it to the visible text
// (plain mode). The tag must be a registered theme entry (RegisterTheme),
// or the renderer would pass the raw markup through unrecognized.
func (t Tier) InlineMarkup() string {
	vis := t.VisibleText()
	if vis == "" {
		return ""
	}
	return wrapTag(rarityTag(t.Key), vis)
}

// PaddedMarkup returns the tier's tag centered in a column of `width`
// display cells — the registry's MaxVisibleWidth — so a list of items
// aligns regardless of which tiers appear (item-decorations §4). The
// visible (colored) portion is wrapped in the theme tag; the centering pad
// is plain spaces OUTSIDE the tag so color never bleeds onto the padding.
//
// A tier that renders as nothing returns `width` plain spaces (blank
// padding keeps the column aligned, §4). A width ≤ 0 degrades to
// InlineMarkup; a visible tag at least as wide as the column is emitted
// without padding.
func (t Tier) PaddedMarkup(width int) string {
	if width <= 0 {
		return t.InlineMarkup()
	}
	vis := t.VisibleText()
	if vis == "" {
		return strings.Repeat(" ", width)
	}
	visW := len([]rune(vis))
	if visW >= width {
		return wrapTag(rarityTag(t.Key), vis)
	}
	left := (width - visW) / 2
	right := width - visW - left
	return strings.Repeat(" ", left) + wrapTag(rarityTag(t.Key), vis) + strings.Repeat(" ", right)
}

// Markup returns the essence's glyph wrapped in its semantic theme tag —
// e.g. "<essence.fire>(✦)</essence.fire>" (item-decorations §4). An essence
// with no glyph returns "". Like the rarity forms, the result is markup for
// the color renderer.
func (e Essence) Markup() string {
	vis := e.VisibleText()
	if vis == "" {
		return ""
	}
	return wrapTag(essenceTag(e.Key), vis)
}

// RegisterTheme registers each tier's color under the semantic tag
// `item.<key>` so the inline/padded markup resolves to color through the
// theme (item-decorations §2/§4). Call at boot after the rarity vocabulary
// is loaded and before render.ThemeRegistry.Compile.
//
// EVERY tier is registered, not only visible/colored ones: an unregistered
// tag is "unknown" to the renderer and its raw markup would leak to the
// client, whereas a registered color-less entry resolves to no color and
// the renderer emits the visible text plain. nil theme is a no-op.
func (r *RarityRegistry) RegisterTheme(theme *render.ThemeRegistry) {
	if theme == nil {
		return
	}
	for _, t := range r.All() {
		theme.Register(rarityTag(t.Key), t.Color)
	}
}

// RegisterTheme registers each essence's color under the semantic tag
// `essence.<key>` (item-decorations §3/§4). Same rationale as
// RarityRegistry.RegisterTheme — every key becomes a known tag so its
// markup never leaks. nil theme is a no-op.
func (r *EssenceRegistry) RegisterTheme(theme *render.ThemeRegistry) {
	if theme == nil {
		return
	}
	for _, e := range r.All() {
		theme.Register(essenceTag(e.Key), e.Color)
	}
}

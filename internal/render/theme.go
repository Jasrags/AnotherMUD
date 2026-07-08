package render

import (
	"strings"
	"sync"
)

// ThemeEntry is a content-declared mapping from a semantic tag name to
// presentation. Any field may be empty:
//   - fg/bg are color names (§2.3) resolved to SGR at Compile.
//   - html is a "#RRGGBB"-style string exposed via GetHtmlMap for GMCP
//     clients; it never affects terminal output.
//
// An entry with only html (no fg/bg) is "declared but color-less":
// IsKnown reports it (so unknown-tag passthrough does not fire) but
// Resolve returns no AnsiPair and the renderer emits its content plain.
type ThemeEntry struct {
	FG   string
	BG   string
	HTML string
}

// AnsiPair is the compiled open/close SGR sequence for a theme tag.
// Close is always Reset (§2.4): a tag close emits an unconditional
// reset, so nested semantic tags lose the outer color on the inner
// close (documented in the spec; brace shorthand is the nesting-safe
// form).
type AnsiPair struct {
	Open  string
	Close string
}

// ThemeRegistry collects ThemeEntry declarations from packs and, after
// Compile, answers the renderer's IsKnown / Resolve lookups. Tag names
// are stored case-insensitively (lower-cased) to match the renderer's
// case-insensitive tag matching (§2.1).
//
// Registration is expected at boot (single goroutine); Compile freezes
// the compiled map and all post-Compile lookups are read-only and
// safe for concurrent use across sessions.
type ThemeRegistry struct {
	mu      sync.RWMutex
	entries map[string]ThemeEntry
	// compiled holds the Basic-tier (ANSI-16) AnsiPair map. Kept
	// for back-compat with the Resolve(tag) accessor and any
	// caller that doesn't care about tier. New M16.6b callers
	// should use ResolveForTier; compiledByTier carries the full
	// per-tier set including TrueColor (24-bit RGB SGR) and
	// Extended (256-color SGR).
	compiled       map[string]AnsiPair
	compiledByTier map[ColorTier]map[string]AnsiPair
}

// NewThemeRegistry returns an empty registry.
func NewThemeRegistry() *ThemeRegistry {
	return &ThemeRegistry{
		entries:        make(map[string]ThemeEntry),
		compiled:       make(map[string]AnsiPair),
		compiledByTier: make(map[ColorTier]map[string]AnsiPair),
	}
}

// Register adds or replaces a theme entry by tag name (case-insensitive).
// A later registration of the same name wins, letting a downstream pack
// override an upstream theme.
func (r *ThemeRegistry) Register(tag string, entry ThemeEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[strings.ToLower(tag)] = entry
}

// Compile builds the fast open/close lookup. It is idempotent: each call
// rebuilds the compiled map from the current entries.
//
// Compile now produces per-tier maps (M16.6b): Basic uses the ANSI-16
// path via ResolveFgColor / ResolveBgColor; Extended maps the entry's
// HTML hex (if present) to the nearest xterm-256 palette index, falling
// back to ANSI-16 when HTML is empty or unparseable; TrueColor emits a
// 24-bit RGB SGR sequence from the HTML hex, falling back the same way.
// An entry produces a tier-pair only when that tier's lookup resolves
// to a non-empty open sequence; html-only entries get tier-pairs for
// Extended/TrueColor (the rich SGR) and skip the Basic map.
func (r *ThemeRegistry) Compile() {
	r.mu.Lock()
	defer r.mu.Unlock()

	basic := make(map[string]AnsiPair, len(r.entries))
	extended := make(map[string]AnsiPair, len(r.entries))
	trueColor := make(map[string]AnsiPair, len(r.entries))

	for tag, e := range r.entries {
		if open := openForBasic(e); open != "" {
			basic[tag] = AnsiPair{Open: open, Close: Reset}
		}
		if open := openForExtended(e); open != "" {
			extended[tag] = AnsiPair{Open: open, Close: Reset}
		}
		if open := openForTrueColor(e); open != "" {
			trueColor[tag] = AnsiPair{Open: open, Close: Reset}
		}
	}

	r.compiled = basic
	r.compiledByTier = map[ColorTier]map[string]AnsiPair{
		ColorTierBasic:     basic,
		ColorTierExtended:  extended,
		ColorTierTrueColor: trueColor,
	}
}

// openForBasic builds the ANSI-16 open sequence for the entry —
// the M0-era behavior preserved unchanged.
func openForBasic(e ThemeEntry) string {
	return ResolveFgColor(e.FG) + ResolveBgColor(e.BG)
}

// openForExtended builds the 256-color open sequence: HTML hex
// quantized to the xterm-256 cube (or grayscale ramp) when
// present, with ANSI-16 fg/bg as fallback. The fg/bg names also
// fall back to ANSI-16 — there's no "256-color" name vocabulary
// in ThemeEntry's FG/BG strings.
func openForExtended(e ThemeEntry) string {
	fg := hexTo256SGR(e.HTML, false)
	if fg == "" {
		fg = ResolveFgColor(e.FG)
	}
	return fg + ResolveBgColor(e.BG)
}

// openForTrueColor builds the 24-bit RGB open sequence: HTML hex
// as truecolor SGR, with ANSI-16 fg/bg as fallback.
func openForTrueColor(e ThemeEntry) string {
	fg := hexToTrueColorSGR(e.HTML, false)
	if fg == "" {
		fg = ResolveFgColor(e.FG)
	}
	return fg + ResolveBgColor(e.BG)
}

// IsKnown reports whether the tag is a declared theme entry. It returns
// true even for entries with no resolvable color (html-only), so the
// renderer's unknown-tag passthrough does not fire on a deliberately
// color-less tag. Uses the raw entry set, not the compiled map.
func (r *ThemeRegistry) IsKnown(tag string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.entries[strings.ToLower(tag)]
	return ok
}

// Resolve returns the Basic-tier (ANSI-16) compiled AnsiPair for
// a tag. Kept for back-compat; new callers should use
// ResolveForTier with the actor's tier.
func (r *ThemeRegistry) Resolve(tag string) (AnsiPair, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.compiled[strings.ToLower(tag)]
	return p, ok
}

// ResolveForTier returns the compiled AnsiPair for a tag at the
// requested color tier (M16.6b). ColorTierNone returns (zero,
// false) — no color is the renderer's job to emit plain.
// Unknown tiers fall through to (zero, false); callers should
// upstream guarantee a valid tier from the per-session capture.
//
// A declared-but-color-less entry (HTML empty AND FG/BG both
// unresolvable) returns (zero, false) at every tier — the
// renderer emits the tag's content plain.
func (r *ThemeRegistry) ResolveForTier(tag string, tier ColorTier) (AnsiPair, bool) {
	if tier == ColorTierNone {
		return AnsiPair{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.compiledByTier[tier]
	if !ok {
		return AnsiPair{}, false
	}
	p, ok := m[strings.ToLower(tag)]
	return p, ok
}

// GetHtmlMap returns a snapshot of {tag → html} for every entry that
// declares an HTML color. Consumed by the GMCP layer to publish a
// stylesheet-like map to capable clients.
func (r *ThemeRegistry) GetHtmlMap() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string)
	for tag, e := range r.entries {
		if e.HTML != "" {
			out[tag] = e.HTML
		}
	}
	return out
}

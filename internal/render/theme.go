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
	mu       sync.RWMutex
	entries  map[string]ThemeEntry
	compiled map[string]AnsiPair
}

// NewThemeRegistry returns an empty registry.
func NewThemeRegistry() *ThemeRegistry {
	return &ThemeRegistry{
		entries:  make(map[string]ThemeEntry),
		compiled: make(map[string]AnsiPair),
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
// rebuilds the compiled map from the current entries. An entry produces
// an AnsiPair only when its fg or bg resolves to a real SGR code;
// html-only (or unresolved-color) entries produce no pair.
func (r *ThemeRegistry) Compile() {
	r.mu.Lock()
	defer r.mu.Unlock()
	compiled := make(map[string]AnsiPair, len(r.entries))
	for tag, e := range r.entries {
		open := ResolveFgColor(e.FG) + ResolveBgColor(e.BG)
		if open == "" {
			continue
		}
		compiled[tag] = AnsiPair{Open: open, Close: Reset}
	}
	r.compiled = compiled
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

// Resolve returns the compiled AnsiPair for a tag and whether one
// exists. A declared-but-color-less tag returns (zero, false): the
// renderer should emit the tag's content plain.
func (r *ThemeRegistry) Resolve(tag string) (AnsiPair, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.compiled[strings.ToLower(tag)]
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

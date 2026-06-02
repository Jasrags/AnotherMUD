// Package decoration implements the two item-marker systems of
// docs/specs/item-decorations.md: an ordered rarity-tier ladder and a flat
// essence glyph set. Both are content-registered presentation markers that
// attach to items via reserved properties and render through the theme
// color pipeline. This package owns the registries and the (theme-free)
// shape of a marker; the themed rendering lives alongside it (render.go),
// and color resolution flows through internal/render's theme registry.
//
// The package is a leaf: it does not import the engine's entity, session,
// or command layers, so items/inventory/shop display can depend on it
// without a cycle.
package decoration

import (
	"sort"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

// Tier is one rarity-tier definition (item-decorations §2). Tiers form an
// ordered ladder via Order (low → high); a tier renders as a decorated,
// colored tag inline next to an item name, or padded to a column width.
//
// A tier renders as **nothing** when it is invisible, has no Display text,
// or has no decorator pair (§2). That is the baseline-tier pattern: a
// `common` tier can carry an Order and Color for ranking/logic while
// leaving every common item's display uncluttered. See VisibleText.
type Tier struct {
	// Key is the short identifier (e.g. "rare"). Compared
	// case-insensitively; unique within the registry.
	Key string
	// Order is the ladder rank, low → high. Establishes "is this rarer
	// than that" and the sort order of All.
	Order int
	// Display is the tag text (e.g. "RARE"). Empty → renders nothing.
	Display string
	// Left and Right are the decorator pair wrapping Display (e.g. "[" /
	// "]"). A missing pair → renders nothing (§2).
	Left  string
	Right string
	// Color is the theme color spec for the tier's tag. It is registered
	// as the theme entry `item.<key>` (§4) so the tag is themed, not
	// raw-colored, and degrades in plain mode.
	Color render.ThemeEntry
	// Visible gates rendering. An invisible tier renders nothing
	// regardless of Display/decorators (§2).
	Visible bool
}

// hasDecorators reports whether the tier carries a full left/right pair.
// A tier with only one side set is treated as having no decorator pair —
// the spec speaks of decorators as a pair (§2).
func (t Tier) hasDecorators() bool {
	return t.Left != "" && t.Right != ""
}

// VisibleText returns the tier's decorated tag text WITHOUT color — e.g.
// "[RARE]" — or the empty string when the tier renders as nothing
// (invisible, no Display, or no decorator pair; §2). Rendering (render.go)
// wraps this in the tier's themed color; an empty result is the signal to
// emit nothing (inline) or blank padding (padded).
func (t Tier) VisibleText() string {
	if !t.Visible || t.Display == "" || !t.hasDecorators() {
		return ""
	}
	return t.Left + t.Display + t.Right
}

// RarityRegistry holds the ordered set of tier definitions. Safe for
// concurrent reads (Get, All, MaxVisibleWidth); Register is the only writer
// and is expected at boot / pack load. Mirrors the property.Registry idiom.
type RarityRegistry struct {
	mu    sync.RWMutex
	tiers map[string]Tier // keyed by normalized Key
}

// NewRarityRegistry returns an empty registry. An empty registry resolves
// no tiers — items render with no rarity tag (item-decorations §1.1).
func NewRarityRegistry() *RarityRegistry {
	return &RarityRegistry{tiers: make(map[string]Tier)}
}

// normalizeKey lowercases + trims a marker key so "Rare", "RARE", and
// " rare " denote the same tier (case-insensitive keys, §2). The empty
// string is not a valid key.
func normalizeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Register installs t under its normalized key, replacing any prior
// definition (idempotent, later-wins — the pack convention, §2). A tier
// with an empty key is ignored (returns false); a successful registration
// returns true. The stored Tier carries the normalized key so lookups and
// the registered value agree.
func (r *RarityRegistry) Register(t Tier) bool {
	k := normalizeKey(t.Key)
	if k == "" {
		return false
	}
	t.Key = k
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tiers[k] = t
	return true
}

// Get resolves a tier by key (case-insensitive). Returns (zero, false) on
// an unknown key — callers render an unknown rarity as unset, never an
// error (§6).
func (r *RarityRegistry) Get(key string) (Tier, bool) {
	k := normalizeKey(key)
	if k == "" {
		return Tier{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tiers[k]
	return t, ok
}

// All returns every tier sorted by Order ascending (the ladder, low →
// high). Ties on Order break by key for a stable order. Fresh slice — safe
// to mutate.
func (r *RarityRegistry) All() []Tier {
	r.mu.RLock()
	out := make([]Tier, 0, len(r.tiers))
	for _, t := range r.tiers {
		out = append(out, t)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// MaxVisibleWidth returns the display width of the widest visible tag in
// the registry (VisibleText length, by rune count). Padded rendering (§4)
// centers each tag in a column of this width so a list aligns regardless of
// which tiers appear. An empty/all-invisible registry returns 0.
func (r *RarityRegistry) MaxVisibleWidth() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	widest := 0
	for _, t := range r.tiers {
		// Rune count, not display-cell width: rarity tag text is
		// conventionally ASCII (e.g. "[RARE]"), so the two agree. If a
		// pack ever uses a wide/double-width rune in a tier's display,
		// revisit this with a cell-width measure (the M20.3 padded
		// renderer is the consumer that would notice).
		if w := len([]rune(t.VisibleText())); w > widest {
			widest = w
		}
	}
	return widest
}

// Len reports the number of registered tiers.
func (r *RarityRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tiers)
}

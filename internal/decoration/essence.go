package decoration

import (
	"sort"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

// Essence is one essence definition (item-decorations §3): a flat,
// glyph-only item marker. Unlike a rarity Tier it has no Order and no
// decorators — it is a single colored symbol. An item carries at most one
// essence key in v1 (multi-essence is deferred, §8).
//
// Essence also participates in **stack identity** (§5): two otherwise
// identical items with different essence keys do not stack. That hook lives
// in the stacking service (M20.5), not here — this type only defines the
// marker and its render shape.
type Essence struct {
	// Key is the short identifier (e.g. "fire"). Compared
	// case-insensitively; unique within the registry.
	Key string
	// Glyph is the symbol drawn for the essence (e.g. "✦"), typically one
	// display column. Empty → renders nothing (VisibleText).
	Glyph string
	// Color is the theme color spec for the glyph. It is registered as the
	// theme entry `essence.<key>` (§4) so the glyph is themed, not
	// raw-colored, and degrades in plain mode.
	Color render.ThemeEntry
}

// VisibleText returns the essence's glyph wrapped in parens WITHOUT color —
// e.g. "(✦)" — or the empty string when the essence has no glyph.
// Rendering (render.go) wraps this in the essence's themed color; this
// mirrors Tier.VisibleText so both markers render through one path.
func (e Essence) VisibleText() string {
	if e.Glyph == "" {
		return ""
	}
	return "(" + e.Glyph + ")"
}

// EssenceRegistry holds the flat set of essence definitions. Safe for
// concurrent reads (Get, All, Len); Register is the only writer and is
// expected at boot / pack load. Mirrors RarityRegistry, minus ordering.
type EssenceRegistry struct {
	mu       sync.RWMutex
	essences map[string]Essence // keyed by normalized Key
}

// NewEssenceRegistry returns an empty registry. An empty registry resolves
// no essences — items render with no essence glyph (item-decorations §1.1).
func NewEssenceRegistry() *EssenceRegistry {
	return &EssenceRegistry{essences: make(map[string]Essence)}
}

// Register installs e under its normalized key (the shared normalizeKey),
// replacing any prior definition (idempotent, later-wins — the pack
// convention, §3). An empty key is ignored (returns false); a successful
// registration returns true. The stored Essence carries the normalized key
// so lookups and the registered value agree.
func (r *EssenceRegistry) Register(e Essence) bool {
	if ValidateKey(e.Key) != nil {
		return false
	}
	e.Key = normalizeKey(e.Key)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.essences[e.Key] = e
	return true
}

// Get resolves an essence by key (case-insensitive). Returns (zero, false)
// on an unknown key — callers render an unknown essence as unset, never an
// error (§6).
func (r *EssenceRegistry) Get(key string) (Essence, bool) {
	k := normalizeKey(key)
	if k == "" {
		return Essence{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.essences[k]
	return e, ok
}

// All returns every essence sorted by key (essence has no order; key sort
// gives a stable listing). Fresh slice — safe to mutate.
func (r *EssenceRegistry) All() []Essence {
	r.mu.RLock()
	out := make([]Essence, 0, len(r.essences))
	for _, e := range r.essences {
		out = append(out, e)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Len reports the number of registered essences.
func (r *EssenceRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.essences)
}

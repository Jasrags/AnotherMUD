package effect

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// Registry holds effect templates by id. Ids are case-insensitive
// on registration and lookup (per progression.EffectTemplate.ID
// conventions); the registry stores the lowercased form as the key
// so duplicate-id detection across (`Bless`, `bless`, `BLESS`)
// catches what content authors would naturally consider the same id.
//
// Safe for concurrent reads; Register is the only writer and is
// expected to be called at boot.
type Registry struct {
	mu      sync.RWMutex
	byID    map[string]progression.EffectTemplate
	ordered []string // ID order, for deterministic iteration
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]progression.EffectTemplate)}
}

// Register installs tpl under its lowercased ID. Duplicate ids
// (case-insensitive) are an error so two packs cannot both ship
// `bless` and silently overwrite one another.
func (r *Registry) Register(tpl progression.EffectTemplate) error {
	id := strings.ToLower(strings.TrimSpace(tpl.ID))
	if id == "" {
		return fmt.Errorf("effect.Register: empty ID")
	}
	tpl.ID = id // canonicalize so consumers always read the lowercased form

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byID[id]; dup {
		return fmt.Errorf("effect.Register: duplicate id %q", id)
	}
	r.byID[id] = tpl
	r.ordered = append(r.ordered, id)
	return nil
}

// Get returns the template registered under id (case-insensitive).
// The returned value is deep-copied — the Modifiers and Flags slices
// are fresh, so callers may mutate the returned template freely
// without polluting the registry. The cost is a small per-call
// allocation; effect templates are tiny (typically <5 modifiers).
func (r *Registry) Get(id string) (progression.EffectTemplate, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return progression.EffectTemplate{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	tpl, ok := r.byID[key]
	if !ok {
		return progression.EffectTemplate{}, false
	}
	return cloneTemplate(tpl), true
}

// All returns every registered template in insertion order. Both
// the outer slice AND each template's inner slices are fresh
// copies; callers may mutate any of it without affecting the
// registry's stored state.
func (r *Registry) All() []progression.EffectTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]progression.EffectTemplate, 0, len(r.ordered))
	for _, id := range r.ordered {
		out = append(out, cloneTemplate(r.byID[id]))
	}
	return out
}

// cloneTemplate deep-copies an EffectTemplate so the registry can
// hand out tear-off copies that callers are free to mutate. The
// progression package has no exported clone for this shape and the
// type is small enough that defining the copy here is cleaner than
// extending its API.
func cloneTemplate(t progression.EffectTemplate) progression.EffectTemplate {
	out := progression.EffectTemplate{
		ID:       t.ID,
		Duration: t.Duration,
	}
	if len(t.Modifiers) > 0 {
		out.Modifiers = make([]stats.Modifier, len(t.Modifiers))
		copy(out.Modifiers, t.Modifiers)
	}
	if len(t.Flags) > 0 {
		out.Flags = make([]string, len(t.Flags))
		copy(out.Flags, t.Flags)
	}
	return out
}

// Len returns the count of registered templates.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.ordered)
}

package effect

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/progression"
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
func (r *Registry) Get(id string) (progression.EffectTemplate, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return progression.EffectTemplate{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	tpl, ok := r.byID[key]
	return tpl, ok
}

// All returns every registered template in insertion order. Fresh
// slice; callers may mutate it.
func (r *Registry) All() []progression.EffectTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]progression.EffectTemplate, 0, len(r.ordered))
	for _, id := range r.ordered {
		out = append(out, r.byID[id])
	}
	return out
}

// Len returns the count of registered templates.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.ordered)
}

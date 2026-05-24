package slot

import (
	"fmt"
	"strings"
	"sync"
)

// Registry holds the registered slot definitions. Safe for concurrent
// reads; mutations (Register) MUST happen at boot before serving —
// same invariant as world.World and item.Templates.
//
// Iteration order across All() preserves registration order (§3.1).
// Lookups by name are case-insensitive but registration is normalized
// to the snake_case form, so two lookups with different casing
// resolve to the same Def.
type Registry struct {
	mu    sync.RWMutex
	order []string        // names in registration order (lowercased)
	defs  map[string]*Def // name (lowercased) → def
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{defs: make(map[string]*Def)}
}

// Register adds d. Returns ErrInvalidName if d.Name is not
// snake_case, ErrInvalidMax if Max is negative, or ErrDuplicate
// (wrapped to name both scopes) if a slot with that name is already
// registered.
//
// A Max of 0 is technically accepted (it is non-negative) but is
// semantically useless — equip will always fail against a cap-0 slot.
// The pack loader's decodeSlot rejects max == 0 at the authoring
// surface; this method's looser check exists so programmatic callers
// (tests, future synthesized slots) can express intermediate states
// without going through YAML.
func (r *Registry) Register(d Def) error {
	if !IsValidName(d.Name) {
		return fmt.Errorf("%w: %q", ErrInvalidName, d.Name)
	}
	if d.Max < 0 {
		return fmt.Errorf("%w: %q max=%d", ErrInvalidMax, d.Name, d.Max)
	}
	key := strings.ToLower(d.Name)
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.defs[key]; ok {
		return fmt.Errorf("%w: %q (existing scope=%q, new scope=%q)",
			ErrDuplicate, d.Name, existing.Scope, d.Scope)
	}
	// Store the normalized name so retrievals return a consistent form.
	dCopy := d
	dCopy.Name = key
	r.defs[key] = &dCopy
	r.order = append(r.order, key)
	return nil
}

// Get returns the slot definition with name (case-insensitive lookup
// per §3.1). Returns ErrNotFound if absent.
//
// Returned by value so callers cannot mutate registry state by writing
// to fields on a shared pointer; Def is small enough that the copy
// cost is negligible.
func (r *Registry) Get(name string) (Def, error) {
	key := strings.ToLower(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.defs[key]
	if !ok {
		return Def{}, fmt.Errorf("slot.Registry.Get(%q): %w", name, ErrNotFound)
	}
	return *d, nil
}

// Has reports whether name is registered (case-insensitive).
func (r *Registry) Has(name string) bool {
	key := strings.ToLower(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.defs[key]
	return ok
}

// Count returns the number of registered slots.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.defs)
}

// All returns a snapshot of registered defs in registration order
// (§3.1). Both the slice and the Def values are copies — neither
// slice-level nor field-level mutation by the caller affects the
// registry.
func (r *Registry) All() []Def {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Def, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, *r.defs[name])
	}
	return out
}

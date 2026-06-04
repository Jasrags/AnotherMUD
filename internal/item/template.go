// Package item owns content-side item data: the template type and the
// registry the pack loader populates at boot. Instances and runtime
// tracking live in internal/entities (M5.2).
//
// Spec: inventory-equipment-items §2.
package item

import (
	"errors"
	"fmt"
	"sync"
)

// TemplateID is a namespace-qualified template identifier
// (e.g. "tapestry-core:short-sword"). Spec §5.2 namespace rule.
type TemplateID string

// Modifier is one stat modification a template grants to its equipper.
// Applied at equip time, not at instantiation (§2.3 step 6).
type Modifier struct {
	Stat  string
	Value int
}

// Template is the recipe an item instance is built from. Fields mirror
// spec §2.2: id, name, type are required; tags, keywords, properties,
// modifiers are optional.
//
// The Properties bag is intentionally untyped: pack content may carry
// arbitrary scalars/maps. Per §2.3 step 4, instantiation normalizes
// nested untyped maps; templates themselves store whatever the decoder
// produced.
type Template struct {
	ID   TemplateID
	Name string
	Type string
	// Description is the optional flavor prose shown by `look <item>`
	// (ui-rendering-help — the appearance lens). Empty means the look
	// handler falls back to a generic "nothing special" line; authoring
	// it is never required.
	Description string
	Tags        []string
	Keywords    []string
	Properties  map[string]any
	Modifiers   []Modifier
	// WeaponDamage is the wielded-weapon damage dice (combat §4.5) as a
	// raw NdM±K string, e.g. "1d6+1". Empty means the item is not a
	// weapon — a holder wielding only non-weapon items rolls the engine's
	// unarmed default. The string is validated at pack load (a malformed
	// expression fails the load, naming the file) and parsed to a typed
	// dice expression when the instance is built; the template stays
	// combat-package-free by holding the canonical string. See
	// entities.ItemInstance.WeaponDamage.
	WeaponDamage string
}

// Errors callers may distinguish at the boundary.
var (
	ErrTemplateNotFound = errors.New("item template not found")
	ErrDuplicateID      = errors.New("duplicate item template id")
)

// Templates is the boot-time registry of item templates. Safe for
// concurrent reads; mutations (Add, TryAdd) MUST happen at boot before
// serving — same invariant as world.World.
type Templates struct {
	mu  sync.RWMutex
	all map[TemplateID]*Template
}

// NewTemplates returns an empty registry.
func NewTemplates() *Templates {
	return &Templates{all: make(map[TemplateID]*Template)}
}

// Add registers t, replacing any existing template with the same id
// (spec §2.1: later registrations replace earlier ones).
func (r *Templates) Add(t *Template) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.all[t.ID] = t
}

// TryAdd registers t and returns ErrDuplicateID if a template with
// that id is already present. Used by the pack loader to catch
// cross-pack id collisions before they silently overwrite.
func (r *Templates) TryAdd(t *Template) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.all[t.ID]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateID, t.ID)
	}
	r.all[t.ID] = t
	return nil
}

// Get returns the template with id and ErrTemplateNotFound if absent.
func (r *Templates) Get(id TemplateID) (*Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.all[id]
	if !ok {
		return nil, fmt.Errorf("item.Templates.Get(%q): %w", id, ErrTemplateNotFound)
	}
	return t, nil
}

// Has reports whether id is registered.
func (r *Templates) Has(id TemplateID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.all[id]
	return ok
}

// Count returns the number of registered templates.
func (r *Templates) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.all)
}

// All returns a snapshot of every registered template. Order is
// unspecified; callers that need determinism must sort.
func (r *Templates) All() []*Template {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Template, 0, len(r.all))
	for _, t := range r.all {
		out = append(out, t)
	}
	return out
}

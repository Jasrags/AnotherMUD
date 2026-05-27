// Package mob owns content-side mob data: the template type and the
// registry the pack loader populates at boot. Instances and the AI
// tick live elsewhere; M6.1 lands templates only.
//
// Spec: mobs-ai-spawning §2.
package mob

import (
	"errors"
	"fmt"
	"sync"
)

// TemplateID is a namespace-qualified template identifier (e.g.
// "tapestry-core:village-guard"). Spec §2.1 + scripting-and-packs §5.2.
type TemplateID string

// Template is the recipe a mob instance is built from. Required fields
// come straight from spec §2.2; optional fields are the ones M6.1 is
// scoped to support — the rest (class/race/level, loot tables, patrol
// routes, idle/battle command sets, scripts, disposition rules, shop
// config) land with later sub-milestones as their consumers arrive.
//
// Equipment is a flat list of item template ids the instantiation
// step (§2.3, M6.2) will equip onto the mob at spawn time. The pack
// loader does NOT cross-validate that those ids resolve — that
// happens at spawn, not load — so a typo here surfaces when the mob
// first spawns rather than at boot. (Aligned with spec §3.1's
// "fail silently and return" semantics for spawn-time misses.)
//
// Properties is intentionally untyped: pack content may carry arbitrary
// scalars/maps that downstream behaviors interpret. The decoder
// normalizes nested untyped maps the same way item templates do.
//
// Stats is a flat map of stat-name to base value. Attribute names are
// not validated at the template layer; the progression spec (M8) owns
// the canonical set, and validating here would prematurely freeze the
// schema before that lands.
type Template struct {
	ID   TemplateID
	Name string
	Type string // default "npc" applied at decode if unset
	// Disposition is the legacy integer base disposition. Retained
	// in the schema for content already authored against it; the
	// runtime no longer reads it. The structured fields below own
	// reaction policy (spec §5.1).
	Disposition int

	// BaseDisposition is the spec §5.1 "static base disposition":
	// a fixed reaction the mob uses regardless of player state.
	// When set to ReactionHostile it short-circuits any
	// DispositionRules and always emits hostile (§5.3 step 3).
	// Empty means "no static reaction; consult DispositionRules".
	BaseDisposition Reaction

	// DispositionRules is the structured policy: default + ordered
	// conditional rules. nil means "no rules" — combined with an
	// empty BaseDisposition the mob never dispatches a reaction.
	DispositionRules *Definition

	Behavior   string
	Tags       []string
	Keywords   []string
	Properties map[string]any
	Stats      map[string]int
	Equipment  []string // item template ids to equip at spawn (§3.3)

	// Race is the optional race id (progression.md §3.1). When set
	// and the id resolves in the race registry at spawn time, the
	// mob's RacialFlags are merged into its tag set (§3.1) and
	// StartingAlignment seeds an alignment property. Unknown ids
	// fail silently at spawn — matching the spec §3.1 mob-spawn
	// "fail-silent on missing template" convention.
	Race string
}

// Errors callers may distinguish at the boundary.
var (
	ErrTemplateNotFound = errors.New("mob template not found")
	ErrDuplicateID      = errors.New("duplicate mob template id")
)

// Templates is the boot-time registry of mob templates. Safe for
// concurrent reads; mutations (Add, TryAdd) MUST happen at boot before
// serving — same invariant as world.World and item.Templates.
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
		return nil, fmt.Errorf("mob.Templates.Get(%q): %w", id, ErrTemplateNotFound)
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

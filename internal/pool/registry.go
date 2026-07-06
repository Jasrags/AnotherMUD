package pool

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Decl is a content-declared resource pool: its Kind, behavior Rules, the
// derived channel whose value drives its ceiling, and which entity kinds seed
// it. It is the shadowrun-mvp SR-M3a generalization of the hardcoded
// mana/movement seed — a world declares its pools (Shadowrun's Stun/Physical
// monitors, the One Power, Essence, Edge) as content instead of Go.
//
// pool stays a LEAF: Decl carries only strings, bools, and the leaf's own Rules
// — no engine types — so the pack loader can build these and the session/entity
// seeders can read them without a dependency cycle. MaxChannel is a plain
// channel/stat name the seeder resolves through the StatBlock (the same way
// mana binds to StatResourceMax today); the leaf never evaluates it.
type Decl struct {
	// Kind is the pool identity ("stun", "mana"); lowercased on Register.
	Kind Kind

	// Rules is the pool's content-declared behavior (floor, overflow, degrade,
	// depletion-event, nonlethal). See Rules.
	Rules Rules

	// MaxChannel names the derived channel/stat whose Effective value is this
	// pool's ceiling — "hp_stun" (8+⌈Willpower/2⌉) for the Stun monitor,
	// "resource_max" for mana. Empty ⇒ the pool seeds with a zero max (inert
	// until content grants one), matching today's default. The seeder binds
	// OnMaxChange to it so the ceiling tracks the stat.
	MaxChannel string

	// SeedOnPlayer / SeedOnMob select which entity kinds receive this pool at
	// creation/spawn. mana/movement seed on players; the Shadowrun monitors seed
	// on both (a mob must be shootable AND stunnable). A decl that seeds on
	// neither is inert (declared but never instantiated).
	SeedOnPlayer bool
	SeedOnMob    bool

	// Pack records which pack declared this pool (diagnostic only).
	Pack string

	// Priority drives override semantics: higher wins on a Kind collision (a
	// world overriding a core pool's rules); equal priority is a no-op (the
	// existing decl is retained).
	Priority int
}

// Registry holds content-declared pool Decls keyed by case-insensitive Kind.
// Additive, not exclusive (unlike an attribute set): every declared pool in the
// active pack closure coexists, and each Decl's SeedOn* flags decide who
// instantiates it. Mirrors progression.AttributeSetRegistry.
type Registry struct {
	mu    sync.RWMutex
	decls map[Kind]*Decl
}

// NewRegistry returns an empty pool registry.
func NewRegistry() *Registry {
	return &Registry{decls: make(map[Kind]*Decl)}
}

// Register installs d. Returns nil on success; an error if d is nil or its Kind
// is empty. The Kind and OverflowTo are lowercased on registration (pool kinds
// are lowercase by convention). Higher Priority replaces an existing decl of
// the same Kind; equal-or-lower Priority no-ops (the first-registered/higher
// entry is retained), so a world pack overrides a core pool by declaring a
// higher priority.
func (r *Registry) Register(d *Decl) error {
	if d == nil {
		return fmt.Errorf("pool: nil Decl")
	}
	kind := Kind(strings.ToLower(strings.TrimSpace(string(d.Kind))))
	if kind == "" {
		return fmt.Errorf("pool: decl missing kind")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.decls[kind]; ok && d.Priority <= existing.Priority {
		return nil
	}
	clone := *d
	clone.Kind = kind
	clone.Rules.OverflowTo = Kind(strings.ToLower(strings.TrimSpace(string(d.Rules.OverflowTo))))
	r.decls[kind] = &clone
	return nil
}

// Get returns the registered Decl for a Kind. Case-insensitive; (nil, false) on
// miss. Returns the registry-owned pointer — callers MUST NOT mutate it.
func (r *Registry) Get(kind Kind) (*Decl, bool) {
	key := Kind(strings.ToLower(strings.TrimSpace(string(kind))))
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.decls[key]
	return d, ok
}

// Has reports whether a pool is declared under kind.
func (r *Registry) Has(kind Kind) bool {
	_, ok := r.Get(kind)
	return ok
}

// All returns every declared pool in Kind-sorted order — deterministic so
// entity seeding is order-stable. Not a hot path. Returns registry-owned
// pointers — callers MUST NOT mutate the returned Decls.
func (r *Registry) All() []*Decl {
	r.mu.RLock()
	defer r.mu.RUnlock()
	kinds := make([]string, 0, len(r.decls))
	for k := range r.decls {
		kinds = append(kinds, string(k))
	}
	sort.Strings(kinds)
	out := make([]*Decl, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, r.decls[Kind(k)])
	}
	return out
}

// PlayerSeed returns the decls flagged SeedOnPlayer, Kind-sorted — the pools a
// character's Set is built from at creation/login. Registry-owned pointers;
// callers MUST NOT mutate them.
func (r *Registry) PlayerSeed() []*Decl {
	return r.seeded(func(d *Decl) bool { return d.SeedOnPlayer })
}

// MobSeed returns the decls flagged SeedOnMob, Kind-sorted — the pools a mob's
// Set is built from at spawn. Registry-owned pointers; callers MUST NOT mutate
// them.
func (r *Registry) MobSeed() []*Decl { return r.seeded(func(d *Decl) bool { return d.SeedOnMob }) }

// seeded returns the decls satisfying keep, preserving All()'s Kind-sorted order.
func (r *Registry) seeded(keep func(*Decl) bool) []*Decl {
	all := r.All()
	out := make([]*Decl, 0, len(all))
	for _, d := range all {
		if keep(d) {
			out = append(out, d)
		}
	}
	return out
}

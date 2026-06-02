package loot

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry is the id-keyed loot-table registry with priority-based
// override, mirroring the other content registries (progression
// ClassRegistry, item.Templates, …). Ids are lowercased on register so
// lookups are case-insensitive. Concurrency-safe.
type Registry struct {
	mu     sync.RWMutex
	tables map[string]*Table
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tables: make(map[string]*Table)}
}

// Register stores a deep copy of t under its lowercased id. A later
// Register for the same id wins only when its Priority is strictly
// greater than the incumbent's (spec convention: a higher-priority pack
// overrides; an equal/lower one is a silent no-op). A nil table or an
// empty id is an error.
func (r *Registry) Register(t *Table) error {
	if t == nil {
		return fmt.Errorf("loot: nil Table")
	}
	id := strings.ToLower(strings.TrimSpace(t.ID))
	if id == "" {
		return fmt.Errorf("loot: table missing id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.tables[id]; ok && t.Priority <= existing.Priority {
		return nil
	}
	r.tables[id] = cloneTable(t, id)
	return nil
}

// Get returns the registry-owned pointer for id (case-insensitive);
// (nil, false) on miss. Callers MUST NOT mutate the returned table.
func (r *Registry) Get(id string) (*Table, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tables[key]
	return t, ok
}

// Has reports whether a table is registered under id.
func (r *Registry) Has(id string) bool {
	_, ok := r.Get(id)
	return ok
}

// All returns every registered table in id-sorted order.
func (r *Registry) All() []*Table {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.tables))
	for id := range r.tables {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Table, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.tables[id])
	}
	return out
}

// cloneTable deep-copies t with the normalized id so post-register
// mutation of the caller's table (or its slices) cannot bleed into the
// registry.
func cloneTable(t *Table, id string) *Table {
	clone := *t
	clone.ID = id
	if len(t.Guaranteed) > 0 {
		g := make([]GuaranteedEntry, len(t.Guaranteed))
		copy(g, t.Guaranteed)
		clone.Guaranteed = g
	}
	if len(t.Weighted) > 0 {
		w := make([]WeightedEntry, len(t.Weighted))
		copy(w, t.Weighted)
		clone.Weighted = w
	}
	if t.RareBonus != nil {
		rb := *t.RareBonus
		if len(t.RareBonus.Entries) > 0 {
			e := make([]WeightedEntry, len(t.RareBonus.Entries))
			copy(e, t.RareBonus.Entries)
			rb.Entries = e
		}
		clone.RareBonus = &rb
	}
	if t.Coin != nil {
		coin := *t.Coin
		clone.Coin = &coin
	}
	return &clone
}

package gathering

import (
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ForageEntry is one candidate in a forage table's weighted pool: an item
// template id, a relative selection Weight (<=0 excludes it), and the Qty
// yielded on selection (default 1). Mirrors loot.WeightedEntry plus a
// quantity.
type ForageEntry struct {
	Item   string
	Weight int
	Qty    int
}

// ForageTable is the ambient-forage resource pool for a biome
// (gathering.md §2): what `forage` rolls against. The biome references it by
// id (Biome.ForageTable). Richness (0..100) feeds the §4 quality roll;
// Ceiling is the rarity-tier key capping the rolled quality (""=uncapped).
// Entries are the weighted item pool.
type ForageTable struct {
	ID       string
	Richness int
	Ceiling  string
	Entries  []ForageEntry
	Pack     string // provenance (diagnostic), set by the loader
}

// ForageFile is the on-disk YAML shape for a forage table.
type ForageFile struct {
	ID       string `yaml:"id"`
	Richness int    `yaml:"richness,omitempty"`
	Ceiling  string `yaml:"ceiling,omitempty"`
	Entries  []struct {
		Item   string `yaml:"item"`
		Weight int    `yaml:"weight"`
		Qty    int    `yaml:"qty,omitempty"`
	} `yaml:"entries"`
}

// DecodeForageTable parses one forage-table YAML document. The id is
// required and at least one entry with a positive weight must exist (an
// empty table can never yield — a content error worth failing loudly).
// Richness is clamped to 0..100; a per-entry Qty < 1 normalizes to 1.
func DecodeForageTable(data []byte) (*ForageTable, error) {
	var f ForageFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("forage table decode: %w", err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("forage table decode: empty id")
	}
	t := &ForageTable{
		ID:       f.ID,
		Richness: clampInt(f.Richness, 0, 100),
		Ceiling:  strings.TrimSpace(f.Ceiling),
	}
	for i, e := range f.Entries {
		if strings.TrimSpace(e.Item) == "" {
			return nil, fmt.Errorf("forage table %q: entry[%d] has empty item", f.ID, i)
		}
		qty := e.Qty
		if qty < 1 {
			qty = 1
		}
		t.Entries = append(t.Entries, ForageEntry{Item: e.Item, Weight: e.Weight, Qty: qty})
	}
	if !t.hasSelectableEntry() {
		return nil, fmt.Errorf("forage table %q: no entry with a positive weight", f.ID)
	}
	return t, nil
}

// hasSelectableEntry reports whether at least one entry can be picked.
func (t *ForageTable) hasSelectableEntry() bool {
	for _, e := range t.Entries {
		if e.Weight > 0 {
			return true
		}
	}
	return false
}

// ForageRegistry is the boot-time registry of forage tables, keyed by
// namespaced id (like loot.Registry). Reads are concurrency-safe; Register
// runs at boot.
type ForageRegistry struct {
	mu     sync.RWMutex
	tables map[string]*ForageTable
}

// NewForageRegistry returns an empty registry.
func NewForageRegistry() *ForageRegistry {
	return &ForageRegistry{tables: make(map[string]*ForageTable)}
}

// Register installs t under its id (lowercased). Duplicate ids error.
func (r *ForageRegistry) Register(t *ForageTable) error {
	if t == nil {
		return fmt.Errorf("forage registry: nil table")
	}
	id := strings.ToLower(strings.TrimSpace(t.ID))
	if id == "" {
		return fmt.Errorf("forage registry: empty id")
	}
	t.ID = id
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.tables[id]; dup {
		return fmt.Errorf("forage registry: duplicate id %q", id)
	}
	r.tables[id] = t
	return nil
}

// Get returns the table registered under id (case-insensitive) and whether
// one exists.
func (r *ForageRegistry) Get(id string) (*ForageTable, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tables[strings.ToLower(strings.TrimSpace(id))]
	return t, ok
}

// Len reports the number of registered tables.
func (r *ForageRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tables)
}

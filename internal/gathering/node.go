package gathering

import (
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// NodeTemplate is a harvestable resource node definition (gathering.md §3):
// an ore vein, a herb patch, a tree. It declares the YieldTable it rolls
// (a ForageTable — node yields reuse the same weighted-pool + richness +
// ceiling shape as ambient forage), a Charges count (harvests before it is
// exhausted), and an optional RequiredTool tag — the ONE permitted refusal
// (§3.3): a hard node refuses a harvester who lacks the tool. The per-room
// count + respawn interval are spawn params and live on the NodeSpawnTable
// entry, not here (a node template may appear in several biomes).
type NodeTemplate struct {
	ID           string
	Name         string
	Description  string
	Keywords     []string
	YieldTable   string // a ForageTable id (the node's yield pool, §3.1)
	Charges      int    // harvests before exhaustion (§3.1); <1 → 1
	RequiredTool string // tool tag the node requires, "" = none (§3.3)
	Pack         string // provenance (diagnostic), set by the loader
}

// NodeSpawnEntry places one node template into a biome's rooms: Count per
// room, ResetInterval ticks before a depleted one respawns (0 → the area's
// default, matching world.SpawnRule.ResetInterval).
type NodeSpawnEntry struct {
	Node          string
	Count         int
	ResetInterval uint64
}

// NodeSpawnTable lists which node templates spawn in a biome's rooms
// (gathering.md §3.1, biomes.md §2). A biome references it by id
// (Biome.NodeSpawnTable); the boot pipeline turns each entry into an area
// spawn rule for every room of that biome.
type NodeSpawnTable struct {
	ID      string
	Entries []NodeSpawnEntry
	Pack    string
}

// --- decode ---

type nodeFile struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name,omitempty"`
	Description  string   `yaml:"description,omitempty"`
	Keywords     []string `yaml:"keywords,omitempty"`
	YieldTable   string   `yaml:"yield_table"`
	Charges      int      `yaml:"charges,omitempty"`
	RequiredTool string   `yaml:"required_tool,omitempty"`
}

// DecodeNodeTemplate parses one node-template YAML document. id + yield_table
// are required; charges < 1 normalizes to 1.
func DecodeNodeTemplate(data []byte) (*NodeTemplate, error) {
	var f nodeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("node template decode: %w", err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("node template decode: empty id")
	}
	if strings.TrimSpace(f.YieldTable) == "" {
		return nil, fmt.Errorf("node template %q: empty yield_table", f.ID)
	}
	charges := f.Charges
	if charges < 1 {
		charges = 1
	}
	return &NodeTemplate{
		ID:           f.ID,
		Name:         f.Name,
		Description:  f.Description,
		Keywords:     append([]string(nil), f.Keywords...),
		YieldTable:   f.YieldTable,
		Charges:      charges,
		RequiredTool: strings.TrimSpace(f.RequiredTool),
	}, nil
}

type nodeSpawnFile struct {
	ID      string `yaml:"id"`
	Entries []struct {
		Node          string `yaml:"node"`
		Count         int    `yaml:"count,omitempty"`
		ResetInterval uint64 `yaml:"reset_interval,omitempty"`
	} `yaml:"entries"`
}

// DecodeNodeSpawnTable parses one node-spawn-table YAML document. id +
// at least one entry with a non-empty node id are required; a per-entry
// Count < 1 normalizes to 1.
func DecodeNodeSpawnTable(data []byte) (*NodeSpawnTable, error) {
	var f nodeSpawnFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("node spawn table decode: %w", err)
	}
	if strings.TrimSpace(f.ID) == "" {
		return nil, fmt.Errorf("node spawn table decode: empty id")
	}
	t := &NodeSpawnTable{ID: f.ID}
	for i, e := range f.Entries {
		if strings.TrimSpace(e.Node) == "" {
			return nil, fmt.Errorf("node spawn table %q: entry[%d] has empty node", f.ID, i)
		}
		count := e.Count
		if count < 1 {
			count = 1
		}
		t.Entries = append(t.Entries, NodeSpawnEntry{Node: e.Node, Count: count, ResetInterval: e.ResetInterval})
	}
	if len(t.Entries) == 0 {
		return nil, fmt.Errorf("node spawn table %q: no entries", f.ID)
	}
	return t, nil
}

// --- registries ---

// NodeRegistry holds node templates + node spawn tables, keyed by
// namespaced id. Reads are concurrency-safe; Register runs at boot.
type NodeRegistry struct {
	mu     sync.RWMutex
	nodes  map[string]*NodeTemplate
	tables map[string]*NodeSpawnTable
}

// NewNodeRegistry returns an empty registry.
func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes:  make(map[string]*NodeTemplate),
		tables: make(map[string]*NodeSpawnTable),
	}
}

// RegisterNode installs a node template (lowercased id). Duplicates error.
func (r *NodeRegistry) RegisterNode(n *NodeTemplate) error {
	if n == nil {
		return fmt.Errorf("node registry: nil node template")
	}
	id := strings.ToLower(strings.TrimSpace(n.ID))
	if id == "" {
		return fmt.Errorf("node registry: empty node id")
	}
	n.ID = id
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.nodes[id]; dup {
		return fmt.Errorf("node registry: duplicate node id %q", id)
	}
	r.nodes[id] = n
	return nil
}

// RegisterSpawnTable installs a node spawn table (lowercased id).
func (r *NodeRegistry) RegisterSpawnTable(t *NodeSpawnTable) error {
	if t == nil {
		return fmt.Errorf("node registry: nil spawn table")
	}
	id := strings.ToLower(strings.TrimSpace(t.ID))
	if id == "" {
		return fmt.Errorf("node registry: empty spawn table id")
	}
	t.ID = id
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.tables[id]; dup {
		return fmt.Errorf("node registry: duplicate spawn table id %q", id)
	}
	r.tables[id] = t
	return nil
}

// Node returns the node template under id (case-insensitive).
func (r *NodeRegistry) Node(id string) (*NodeTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.nodes[strings.ToLower(strings.TrimSpace(id))]
	return n, ok
}

// SpawnTable returns the node spawn table under id (case-insensitive).
func (r *NodeRegistry) SpawnTable(id string) (*NodeSpawnTable, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tables[strings.ToLower(strings.TrimSpace(id))]
	return t, ok
}

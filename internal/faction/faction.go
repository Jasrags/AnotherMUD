// Package faction implements per-character standing with content-defined
// factions (faction.md). It is alignment's architecture (progression §6)
// generalized to N named axes: a signed standing integer per (character,
// faction), content-defined named ranks partitioning the range, rank tags
// mirrored for the world index, a combined bounded history, the cancellable
// Shift pipeline, and the ResolveRanks gating helper content consults.
//
// The package is a leaf: it knows nothing of entities, sessions, or the bus.
// The composition root writes the Entity adapter (for players) and the Sink
// (bridging to eventbus), exactly as it does for progression.AlignmentManager.
package faction

import "sort"

// Rank is one named band in a faction's ladder (faction.md §2). Threshold is
// the lowest standing at/above which the rank applies; the highest ladder
// entry whose threshold a value meets or exceeds is that value's rank (§3.2).
type Rank struct {
	Name      string `yaml:"name"`
	Threshold int    `yaml:"threshold"`
}

// Definition is a content-registered faction (faction.md §2). Loaded with the
// pack, never written to a save. Ranks are kept sorted ascending by threshold
// (Registry.Add sorts a copy); Min/Max/Starting default from the registry
// Config when the faction omits them.
type Definition struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Ranks       []Rank `yaml:"ranks"`
	Min         int    `yaml:"min"`
	Max         int    `yaml:"max"`
	Starting    int    `yaml:"starting"`

	// hasBounds/hasStarting record whether the YAML supplied these so
	// Registry.Add can fill defaults only for omitted fields. Set by Add.
	hasMin      bool
	hasMax      bool
	hasStarting bool
}

// RankOf returns the name of the highest ladder entry whose threshold value
// meets or exceeds (faction.md §3.2). Ranks must be sorted ascending (the
// Registry guarantees this). Empty ladder → "".
func (d *Definition) RankOf(value int) string {
	name := ""
	for _, r := range d.Ranks {
		if value >= r.Threshold {
			name = r.Name
		} else {
			break
		}
	}
	return name
}

// clamp constrains value to the faction's [Min, Max].
func (d *Definition) clamp(value int) int {
	if value < d.Min {
		return d.Min
	}
	if value > d.Max {
		return d.Max
	}
	return value
}

// rankIndex returns the index of the named rank in the (sorted) ladder, or -1.
func (d *Definition) rankIndex(name string) int {
	for i, r := range d.Ranks {
		if r.Name == name {
			return i
		}
	}
	return -1
}

// Config is the §9 configuration surface — the defaults a faction inherits
// when it omits its own ladder/bounds/starting, plus the combined-history cap.
type Config struct {
	DefaultLadder   []Rank
	DefaultMin      int
	DefaultMax      int
	DefaultStarting int
	HistoryCapacity int
	// OnKillDelta is the standing shift applied to a killer when they land the
	// killing blow on a faction member (faction.md §5.2 / §9). Typically
	// negative — killing a faction's own lowers the killer's standing with it.
	// Used by the composition root's on-kill hook, not the manager itself.
	OnKillDelta int
}

// DefaultConfig returns the engine defaults (faction.md §9 example): the
// shared signed ladder Hostile … Allied, bounds ±1000, starting 0 (Neutral),
// history capacity 20.
func DefaultConfig() Config {
	return Config{
		DefaultLadder: []Rank{
			{Name: "Hostile", Threshold: -1000},
			{Name: "Unfriendly", Threshold: -300},
			{Name: "Neutral", Threshold: 0},
			{Name: "Friendly", Threshold: 300},
			{Name: "Honored", Threshold: 700},
			{Name: "Allied", Threshold: 900},
		},
		DefaultMin:      -1000,
		DefaultMax:      1000,
		DefaultStarting: 0,
		HistoryCapacity: 20,
		OnKillDelta:     -100,
	}
}

// Registry maps namespaced faction ids to definitions (faction.md §2). Safe
// for concurrent reads; Add is load-time only.
type Registry struct {
	cfg  Config
	defs map[string]*Definition
}

// NewRegistry returns an empty registry seeded with cfg's defaults.
func NewRegistry(cfg Config) *Registry {
	if cfg.HistoryCapacity < 1 {
		cfg.HistoryCapacity = 1
	}
	if len(cfg.DefaultLadder) == 0 {
		cfg.DefaultLadder = DefaultConfig().DefaultLadder
	}
	return &Registry{cfg: cfg, defs: make(map[string]*Definition)}
}

// Config returns the registry's configuration (for renderers/consumers).
func (r *Registry) Config() Config { return r.cfg }

// Add registers d, filling defaults for any omitted ladder/bounds/starting and
// sorting the ladder ascending by threshold. Returns the stored pointer.
// Re-adding an id overwrites (last-wins, mirroring other registries' priority
// override). A definition with no ladder inherits the default ladder.
func (r *Registry) Add(d Definition) *Definition {
	def := d // copy
	if len(def.Ranks) == 0 {
		def.Ranks = append([]Rank(nil), r.cfg.DefaultLadder...)
	} else {
		def.Ranks = append([]Rank(nil), def.Ranks...)
	}
	sort.SliceStable(def.Ranks, func(i, j int) bool { return def.Ranks[i].Threshold < def.Ranks[j].Threshold })
	if !def.hasMin {
		def.Min = r.cfg.DefaultMin
	}
	if !def.hasMax {
		def.Max = r.cfg.DefaultMax
	}
	if !def.hasStarting {
		def.Starting = r.cfg.DefaultStarting
	}
	def.Starting = def.clamp(def.Starting)
	r.defs[def.ID] = &def
	return &def
}

// AddWithFlags is Add for callers (the content loader) that know which fields
// the source actually supplied, so defaults fill only the omitted ones.
func (r *Registry) AddWithFlags(d Definition, hasMin, hasMax, hasStarting bool) *Definition {
	d.hasMin, d.hasMax, d.hasStarting = hasMin, hasMax, hasStarting
	return r.Add(d)
}

// Get returns the definition for id, or (nil, false).
func (r *Registry) Get(id string) (*Definition, bool) {
	d, ok := r.defs[id]
	return d, ok
}

// All returns every registered definition (unordered copy of the slice of
// pointers) — for admin listing / the standing command.
func (r *Registry) All() []*Definition {
	out := make([]*Definition, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Len returns the number of registered factions.
func (r *Registry) Len() int { return len(r.defs) }

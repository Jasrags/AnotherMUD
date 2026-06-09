// Package biome is the ecological classification layer (biomes.md): a
// pack-registered Biome definition keyed by a room's existing `terrain`
// string. It generalizes the hardcoded weather/time shielding
// (world-rooms-movement §6.4) into biome metadata and carries the data
// biomes drive — idle ambience and the forage / node / mob resource tables
// the gathering feature consumes.
//
// One axis, extended (PD-1): the biome IS the registered definition behind
// the existing `terrain` property — there is no new room field. A `terrain`
// value with no registered biome behaves exactly as before (the §2.3
// backward-compat path), so existing content authored as `terrain: forest`
// keeps working. Biomes resolve by their BARE id (PD-3) because terrain
// strings in room content are bare.
package biome

import (
	"fmt"
	"strings"
	"sync"
)

// Biome is a registered ecological definition (biomes.md §2). Every field
// except ID is optional and defaults to "absent / no effect" so a biome may
// carry only what it drives (a shielding-only biome, an ambience-only
// biome, a resource-only biome).
type Biome struct {
	// ID is the lowercased `terrain` string this biome matches
	// (biomes.md §2). Engine-scope ids (outdoors, indoors) are bare;
	// pack-scope ids are namespaced but still keyed bare for terrain
	// matching.
	ID string

	// DisplayName / Description are presentation (e.g. a look terrain
	// hint, if content surfaces it — §2, §7).
	DisplayName string
	Description string

	// WeatherShielded / TimeShielded generalize the §6.4 shielding flags
	// (biomes.md §3). A shielded room receives sky-driven ambience only
	// when its matching per-room exposed override is set. Wired into the
	// world shielding check in A2.
	WeatherShielded bool
	TimeShielded    bool

	// Ambience is the pool of idle ecological flavor lines delivered
	// periodically to occupied rooms of this biome (biomes.md §4). Empty =
	// a silent biome. Consumed by the biome-ambience tick (A3).
	Ambience []string

	// ForageTable / NodeSpawnTable / MobSpawnTable name the resource and
	// spawn tables this biome offers (biomes.md §2, §5) — what `forage`
	// rolls, which harvestable nodes spawn here, and an optional additive
	// mob spawn source. Ids into the relevant feature's tables; inert
	// until their consumers wire in (gathering = Milestone B). Empty = the
	// biome offers nothing on that axis.
	ForageTable    string
	NodeSpawnTable string
	MobSpawnTable  string

	// Pack records the pack that registered this biome (diagnostic only,
	// like item/recipe provenance). Empty for engine-scope biomes.
	Pack string
}

// DefaultBiomeID is the biome a room with no `terrain` property resolves to
// (biomes.md §2.1 / §7). Matches world.TerrainOutdoors.
const DefaultBiomeID = "outdoors"

// Errors callers may distinguish at the registration boundary.
var (
	ErrEmptyID   = fmt.Errorf("biome: empty id")
	ErrDuplicate = fmt.Errorf("biome: duplicate id")
	ErrShadow    = fmt.Errorf("biome: pack biome shadows an engine biome")
)

// Registry is the boot-time registry of biomes, keyed by bare lowercased
// id. Reads (Get, Resolve, All) are concurrency-safe; Register* are the
// only writers and run at boot. Mirrors property.Registry's engine/pack
// split + shadow protection.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*Biome // keyed by lowercased id
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]*Biome)}
}

// RegisterEngine installs b as an engine-scoped biome (bare id, no pack).
func (r *Registry) RegisterEngine(b *Biome) error {
	return r.register(b, "")
}

// RegisterPack installs b as a pack-scoped biome. A pack biome MUST NOT
// shadow an engine biome (biomes.md PD-3 / property-registry §2.3 rule).
func (r *Registry) RegisterPack(packName string, b *Biome) error {
	if packName == "" {
		if b == nil {
			return ErrEmptyID
		}
		return fmt.Errorf("biome: empty packName for %q", b.ID)
	}
	return r.register(b, packName)
}

// register validates and installs b under its normalized id, scoped to
// pack ("" = engine). It mutates the caller's struct (b.ID, b.Pack) ONLY on
// success — a failed registration (empty id, shadow, duplicate) leaves the
// argument untouched, so a caller may inspect or retry it.
func (r *Registry) register(b *Biome, pack string) error {
	if b == nil {
		return ErrEmptyID
	}
	id := strings.ToLower(strings.TrimSpace(b.ID))
	if id == "" {
		return ErrEmptyID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.entries[id]; ok {
		// A pack biome that collides with an engine biome is a shadow
		// (the stricter error); any other collision is a plain duplicate.
		if pack != "" && existing.Pack == "" {
			return fmt.Errorf("%w: %q", ErrShadow, id)
		}
		return fmt.Errorf("%w: %q", ErrDuplicate, id)
	}
	b.ID = id
	b.Pack = pack
	r.entries[id] = b
	return nil
}

// Get returns the biome registered under id (case-insensitive) and whether
// one exists. No default substitution — see Resolve for the room-resolution
// path.
func (r *Registry) Get(id string) (*Biome, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.entries[strings.ToLower(strings.TrimSpace(id))]
	return b, ok
}

// Resolve maps a room's `terrain` value to its biome (biomes.md §2.1):
//   - empty terrain → the default biome (outdoors), if registered;
//   - a value matching a registered biome → that biome;
//   - a value with no registered biome → (nil, false), the §2.3
//     backward-compat path the caller treats as today's bare-string
//     behavior.
func (r *Registry) Resolve(terrain string) (*Biome, bool) {
	t := strings.ToLower(strings.TrimSpace(terrain))
	if t == "" {
		t = DefaultBiomeID
	}
	return r.Get(t)
}

// All returns every registered biome, unordered. Fresh slice — safe to
// iterate while the registry is otherwise idle (boot-complete).
func (r *Registry) All() []*Biome {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Biome, 0, len(r.entries))
	for _, b := range r.entries {
		out = append(out, b)
	}
	return out
}

// Len reports the number of registered biomes.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

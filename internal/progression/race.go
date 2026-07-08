package progression

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
)

// Race is a content-defined origin record (docs/specs/
// progression.md §3.1): stat caps consumed by training (M8.6),
// a cast-cost modifier consumed by abilities (M9), a racial-flag
// list applied as tags at instantiation (M8.3, this slice),
// category for class eligibility (M8.4), and presentation fields.
//
// Race is value-typed for registry storage. The registry hands
// callers a pointer to its own copy; callers MUST NOT mutate it.
type Race struct {
	// ID is the stable case-insensitive identity used by the
	// registry. Case is normalized to lowercase on Register.
	ID string

	// DisplayName is the human-facing label. Falls back to ID in
	// renderers when empty.
	DisplayName string

	// Tagline is a short flavor line shown in character creation
	// menus and the score panel. Optional.
	Tagline string

	// Description is the long-form flavor text shown when a
	// player inspects the race during character creation.
	// Optional.
	Description string

	// Category is a free-form string the class registry consults
	// for allowed-categories filtering (spec §4.4). Empty
	// category means the race participates in no class
	// restrictions.
	Category string

	// StartingAlignment is the integer alignment seeded on new
	// characters of this race. Spec §6.1 says alignment defaults
	// to zero; a non-zero StartingAlignment overrides that for
	// new characters. Read by character creation; not enforced
	// post-create.
	StartingAlignment int

	// StatCaps map a base attribute to the maximum value a
	// character of this race can train it to (spec §7.4 race
	// cap check). An absent entry uses the engine-wide default
	// (configured externally; defaults to 25 today). M8.6
	// training reads these.
	StatCaps map[StatType]int

	// StatBonuses is a flat starting-attribute grant added ONCE at character
	// creation via AdjustBase, the same seam as Class.StartingStats (so a
	// metatype and a class compose additively). An ork's higher Body/Strength,
	// a troll's far higher Body/Strength — the metatype's *starting* skew,
	// distinct from StatCaps (its train *ceiling*). Absent ⇒ no seed skew.
	// A negative entry is permitted (a metatype attribute penalty).
	StatBonuses map[StatType]int

	// CastCostModifier is added to every ability's base resource
	// cost (spec §3.1 cast-cost). Clamped at zero by
	// cost.AdjustCost. M9 abilities read this.
	CastCostModifier int

	// RacialFlags is the list of tag strings applied to the
	// entity at instantiation (spec §3.1). Mob spawn and the
	// future character-creation flow merge these into the
	// entity's tag set.
	RacialFlags []string

	// Size is the race's nominal size category (size-and-wielding §3.1) from
	// the engine size vocabulary (tiny … huge). Empty ⇒ the baseline size
	// (most playable races). A character's size is read from its race; no
	// per-character size choice exists. Validated at pack load; lowercased.
	Size string

	// Pack is the pack that registered this race. Diagnostic
	// only — used in "where did this come from" logs.
	Pack string

	// Priority drives override semantics: higher wins on a name
	// collision; equal priority is a no-op (existing entry
	// retained). Matches the §4.2 pattern shared across the
	// progression registries.
	Priority int
}

// RaceRegistry holds race definitions keyed by case-insensitive
// id. Mirrors TrackRegistry / ClassRegistry shape; higher-priority
// Register replaces lower-priority ones with the same id, equal-
// priority registrations retain the existing entry.
type RaceRegistry struct {
	mu    sync.RWMutex
	races map[string]*Race
}

// NewRaceRegistry returns an empty registry.
func NewRaceRegistry() *RaceRegistry {
	return &RaceRegistry{races: make(map[string]*Race)}
}

// Register installs r. Returns nil on success; an error if the
// definition is malformed (empty id). Id is lowercased on
// registration so case-insensitive lookups work without per-call
// allocation. Higher priority replaces; equal priority no-ops.
func (rg *RaceRegistry) Register(r *Race) error {
	if r == nil {
		return fmt.Errorf("progression: nil Race")
	}
	id := strings.ToLower(strings.TrimSpace(r.ID))
	if id == "" {
		return fmt.Errorf("progression: race missing id")
	}
	rg.mu.Lock()
	defer rg.mu.Unlock()
	existing, ok := rg.races[id]
	if ok && r.Priority <= existing.Priority {
		return nil
	}
	clone := *r
	clone.ID = id
	if len(r.StatCaps) > 0 {
		caps := make(map[StatType]int, len(r.StatCaps))
		maps.Copy(caps, r.StatCaps)
		clone.StatCaps = caps
	}
	if len(r.StatBonuses) > 0 {
		bonuses := make(map[StatType]int, len(r.StatBonuses))
		maps.Copy(bonuses, r.StatBonuses)
		clone.StatBonuses = bonuses
	}
	if len(r.RacialFlags) > 0 {
		flags := make([]string, len(r.RacialFlags))
		copy(flags, r.RacialFlags)
		clone.RacialFlags = flags
	}
	rg.races[id] = &clone
	return nil
}

// Get returns the registered Race for id. Lookup is case-
// insensitive. Returns (nil, false) on miss.
func (rg *RaceRegistry) Get(id string) (*Race, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return nil, false
	}
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	r, ok := rg.races[key]
	return r, ok
}

// Has reports whether a race is registered under id.
func (rg *RaceRegistry) Has(id string) bool {
	_, ok := rg.Get(id)
	return ok
}

// All returns every registered race in id-sorted order. Used by
// renderers and the character-creation menu; not on the hot path.
func (rg *RaceRegistry) All() []*Race {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	ids := make([]string, 0, len(rg.races))
	for id := range rg.races {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Race, 0, len(ids))
	for _, id := range ids {
		out = append(out, rg.races[id])
	}
	return out
}

// AdjustCost is the §3.3 cost calculator. Returns
// max(0, baseCost + race.CastCostModifier). A nil race yields
// the unchanged base. Used by M9 abilities; lives here so the
// abilities feature can consume it without importing back into
// the registry.
func AdjustCost(baseCost int, race *Race) int {
	if race == nil {
		return baseCost
	}
	adjusted := baseCost + race.CastCostModifier
	if adjusted < 0 {
		return 0
	}
	return adjusted
}

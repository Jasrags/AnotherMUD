package progression

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// AbilityType is the active/passive classification (spec
// abilities-and-effects §2.2). Active abilities are queueable and
// resolve on a combat round pulse (§4); passive abilities are
// evaluated at hook points (§6).
type AbilityType string

const (
	// AbilityActive is a queueable ability resolved in the ability
	// resolution phase (spec §4).
	AbilityActive AbilityType = "active"
	// AbilityPassive is hook-driven and never queued (spec §6).
	AbilityPassive AbilityType = "passive"
)

// AbilityCategory is the skill/spell classification (spec §2.2).
// Drives default offensive classification and the resource pool
// (§4.6, §4.7): skills draw movement, spells draw mana.
type AbilityCategory string

const (
	// AbilitySkill draws from the entity's movement pool (§4.7).
	AbilitySkill AbilityCategory = "skill"
	// AbilitySpell draws from the entity's mana pool (§4.7).
	AbilitySpell AbilityCategory = "spell"
)

// ParseAbilityType normalizes a raw string into AbilityType.
// Case-insensitive; unknown values yield (zero, false).
func ParseAbilityType(s string) (AbilityType, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "active":
		return AbilityActive, true
	case "passive":
		return AbilityPassive, true
	default:
		return "", false
	}
}

// ParseAbilityCategory normalizes a raw string into AbilityCategory.
// Case-insensitive; unknown values yield (zero, false).
func ParseAbilityCategory(s string) (AbilityCategory, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "skill":
		return AbilitySkill, true
	case "spell":
		return AbilitySpell, true
	default:
		return "", false
	}
}

// Ability is a content-defined ability definition (spec
// abilities-and-effects §2). M9.1 carries the minimum surface used
// by the registry + proficiency manager + training-cap raises:
// identity, classification, and learn-time defaults. Resolution
// fields (cost, cooldown, target rules, effect template, variance,
// handler token, metadata) land as later M9 slices consume them
// (M9.2 effects, M9.3 queue+validation, M9.4 resolution, M9.5
// passives).
//
// Value-typed for registry storage; the registry hands callers a
// pointer to its own copy and callers MUST NOT mutate it.
type Ability struct {
	// ID is the stable case-insensitive id (spec §2.1).
	ID string

	// DisplayName is the player-facing name (spec §2.2 required).
	DisplayName string

	// Type is active/passive (spec §2.2 required).
	Type AbilityType

	// Category is skill/spell (spec §2.2 required).
	Category AbilityCategory

	// DefaultCap is the cap value set when an entity learns this
	// ability and has no cap entry yet (spec §3.2). When zero, the
	// proficiency manager falls back to its configured default
	// (ProficiencyConfig.DefaultLearnCap).
	DefaultCap int

	// GainBaseChance is the base probability (in 1..100 units) of
	// a proficiency gain on use (spec §3.5 step 1). Zero disables
	// gain rolls for this ability.
	GainBaseChance int

	// GainFailureMultiplier scales GainBaseChance on a missed
	// invocation (spec §3.5 step 4). 1.0 means miss == hit;
	// values < 1.0 reduce miss-gain. Zero falls back to the
	// proficiency-manager default.
	GainFailureMultiplier float64

	// GainStat names a stat whose effective value contributes to
	// gain probability (spec §3.5 step 3). Empty means no stat
	// boost (gain probability uses base × proficiency-curve only).
	GainStat StatType

	// GainStatScale scales the stat contribution to gain
	// probability. Zero with a non-empty GainStat falls back to
	// the proficiency-manager default.
	GainStatScale float64

	// Pack records the pack that registered this ability.
	// Diagnostic only — mirrors Race.Pack / Class.Pack.
	Pack string

	// Priority drives override semantics on duplicate id: higher
	// wins; equal priority is a no-op (existing entry retained).
	// Mirrors race/class registries (spec §2.1 "higher-priority
	// registration wins").
	Priority int
}

// AbilityRegistry holds ability definitions keyed by case-insensitive
// id. Mirrors ClassRegistry / RaceRegistry shape.
type AbilityRegistry struct {
	mu    sync.RWMutex
	items map[string]*Ability
}

// NewAbilityRegistry returns an empty registry.
func NewAbilityRegistry() *AbilityRegistry {
	return &AbilityRegistry{items: make(map[string]*Ability)}
}

// Register installs a. Returns an error on malformed input (nil,
// missing id, unknown type, unknown category). Id is lowercased on
// registration. Higher priority replaces; equal priority no-ops
// (spec §2.1).
func (r *AbilityRegistry) Register(a *Ability) error {
	if a == nil {
		return fmt.Errorf("progression: nil Ability")
	}
	id := strings.ToLower(strings.TrimSpace(a.ID))
	if id == "" {
		return fmt.Errorf("progression: ability missing id")
	}
	if a.Type != AbilityActive && a.Type != AbilityPassive {
		return fmt.Errorf("progression: ability %q has invalid type %q", id, a.Type)
	}
	if a.Category != AbilitySkill && a.Category != AbilitySpell {
		return fmt.Errorf("progression: ability %q has invalid category %q", id, a.Category)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.items[id]; ok && a.Priority <= existing.Priority {
		return nil
	}
	clone := *a
	clone.ID = id
	if clone.DefaultCap < 0 {
		clone.DefaultCap = 0
	}
	if clone.DefaultCap > 100 {
		clone.DefaultCap = 100
	}
	clone.GainStat = StatType(strings.ToLower(strings.TrimSpace(string(a.GainStat))))
	r.items[id] = &clone
	return nil
}

// Get returns the registered Ability for id. Case-insensitive
// lookup; (nil, false) on miss. The returned pointer is
// registry-owned — callers MUST NOT mutate it.
func (r *AbilityRegistry) Get(id string) (*Ability, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.items[key]
	return a, ok
}

// Has reports whether an ability is registered under id.
func (r *AbilityRegistry) Has(id string) bool {
	_, ok := r.Get(id)
	return ok
}

// All returns every registered ability in id-sorted order.
func (r *AbilityRegistry) All() []*Ability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Ability, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.items[id])
	}
	return out
}

// ByType returns every registered ability whose Type matches t, in
// id-sorted order. Used by passive-hook iteration (spec §6.3) and
// administrative listings.
func (r *AbilityRegistry) ByType(t AbilityType) []*Ability {
	all := r.All()
	out := make([]*Ability, 0, len(all))
	for _, a := range all {
		if a.Type == t {
			out = append(out, a)
		}
	}
	return out
}

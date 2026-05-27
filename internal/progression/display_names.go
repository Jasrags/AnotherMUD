package progression

import (
	"strings"
	"sync"
)

// StatDisplayNames maps lowercase stat names to display strings.
// Spec progression.md §2.5: "the only point at which the feature
// exposes the 'is this resource called mana or essence?' decision".
// Content configures it once at startup; renderers and help
// generators consume it everywhere.
//
// Lookups fall through overrides → defaults → raw name. The default
// set covers the canonical attributes plus the legacy combat surface
// (hit_mod, ac) so renderers do not have to special-case them.
type StatDisplayNames struct {
	mu        sync.RWMutex
	overrides map[string]string
}

// NewStatDisplayNames returns an empty registry. Defaults are not
// stored — they fall through DefaultStatDisplayName for an O(1)
// branch on every miss.
func NewStatDisplayNames() *StatDisplayNames {
	return &StatDisplayNames{overrides: make(map[string]string)}
}

// Set registers an override for stat. The stat name is lowercased
// for the key so callers can pass mixed-case names and still hit
// the registered entry.
func (r *StatDisplayNames) Set(stat, display string) {
	key := strings.ToLower(strings.TrimSpace(stat))
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.overrides[key] = display
}

// Lookup returns the display string for stat. The fallthrough is
// overrides → defaults → raw name. An empty stat name returns
// empty.
func (r *StatDisplayNames) Lookup(stat string) string {
	key := strings.ToLower(strings.TrimSpace(stat))
	if key == "" {
		return ""
	}
	r.mu.RLock()
	if r.overrides != nil {
		if v, ok := r.overrides[key]; ok {
			r.mu.RUnlock()
			return v
		}
	}
	r.mu.RUnlock()
	if v, ok := DefaultStatDisplayName(key); ok {
		return v
	}
	return key
}

// DefaultStatDisplayName returns the engine's built-in display name
// for stat. Exported so callers can probe defaults without
// instantiating a registry, and so tests can assert exact mappings.
//
// The default set covers:
//   - The six classic attributes plus the three vital maxima (§2.1).
//   - The three current-vital names (hp, resource, movement).
//   - The legacy combat surface (hit_mod, ac, damage) so renderers
//     drawing the consider/score panels do not need bespoke labels.
//
// Stat is lowercased before lookup.
func DefaultStatDisplayName(stat string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(stat))
	v, ok := defaultStatDisplay[key]
	return v, ok
}

var defaultStatDisplay = map[string]string{
	"str":          "Strength",
	"int":          "Intelligence",
	"wis":          "Wisdom",
	"dex":          "Dexterity",
	"con":          "Constitution",
	"luck":         "Luck",
	"hp_max":       "Max HP",
	"resource_max": "Max Mana",
	"movement_max": "Max Movement",
	"hp":           "HP",
	"resource":     "Mana",
	"movement":     "Movement",
	"hit_mod":      "Hit",
	"ac":           "AC",
	"damage":       "Damage",
}

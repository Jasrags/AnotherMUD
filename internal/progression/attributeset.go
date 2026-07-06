package progression

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Attribute categories used by the score sheet to group a set's attributes.
// Content declares one per attribute; an unknown/empty value renders under a
// trailing "other" group rather than erroring.
const (
	AttrCategoryPhysical = "physical"
	AttrCategoryMental   = "mental"
	AttrCategorySpecial  = "special"
)

// Attribute is one character attribute in an AttributeSet — a content-declared
// primary stat (Strength, Body, Edge, …). It is DISTINCT from the engine-vital
// stat keys (hp_max, movement_max, hit_mod, ac) that every world carries
// regardless of its attribute set; those are seeded by the engine, not
// declared here.
//
// The ID is a StatType (the lowercased stat key the StatBlock stores under);
// the rest is presentation + chargen/training policy. See shadowrun-mvp.md
// Appendix A (SR-M1) for why the base attribute set is content-declared.
type Attribute struct {
	// ID is the stat key (lowercased on Register): "str", "body", "edge".
	ID StatType

	// Name is the display name shown by `score` ("Strength"). Falls back to
	// the ID when empty.
	Name string

	// Abbrev is the short label the score sheet renders ("STR"). Falls back to
	// the uppercased ID when empty.
	Abbrev string

	// Default is the value seeded onto a brand-new character at creation
	// before class/background/point-buy adjustments.
	Default int

	// Cap is the default ceiling the `train` verb honors. A race/metatype
	// stat_caps entry overrides it per-race; a non-positive value means "no
	// set-level cap" (the race cap, if any, still applies).
	Cap int

	// Trainable reports whether the `train` verb may raise this attribute.
	// The classic six are trainable; a derived or special attribute may not be.
	Trainable bool

	// Category groups the attribute on the score sheet (physical/mental/special).
	Category string
}

// AttributeSet is a content-declared, ordered set of character attributes a
// world seeds its characters from (SR-M1). The engine's six classics ship as
// the `classic` set in the core pack; a world pack (e.g. Shadowrun) declares
// its own. Selection is by world — a world's manifest names its set, and a
// world with no declaration falls back to `classic` (wired in a later SR-M1
// step; this type is the substrate).
//
// Mirrors the Language registry shape: value-typed for storage, an ordered
// slice preserves author order for the score sheet, higher priority wins on an
// id collision. The set id is GLOBAL (not namespace-qualified) — it is a
// vocabulary selected by manifest reference, like feats/abilities.
type AttributeSet struct {
	// ID is the stable case-insensitive set identity ("classic", "shadowrun5");
	// lowercased on Register.
	ID string

	// Name is a human label for the set (diagnostic / listings).
	Name string

	// Attributes is the ordered attribute list, author order preserved.
	Attributes []Attribute

	// Pack records which pack registered this set (diagnostic only).
	Pack string

	// Priority drives override semantics: higher wins on an id collision; equal
	// priority is a no-op (existing entry retained).
	Priority int
}

// Keys returns the attribute stat keys in declared order. Used to seed a new
// character and to iterate the score sheet.
func (s *AttributeSet) Keys() []StatType {
	out := make([]StatType, 0, len(s.Attributes))
	for _, a := range s.Attributes {
		out = append(out, a.ID)
	}
	return out
}

// Defaults returns a fresh map of attribute key → seed value. The engine-vital
// keys (hp_max, hit_mod, …) are NOT included — the seed builder adds those.
func (s *AttributeSet) Defaults() map[StatType]int {
	out := make(map[StatType]int, len(s.Attributes))
	for _, a := range s.Attributes {
		out[a.ID] = a.Default
	}
	return out
}

// Caps returns attribute key → set-level cap for attributes declaring a
// positive cap. Race/metatype caps override these per-race downstream.
func (s *AttributeSet) Caps() map[StatType]int {
	out := make(map[StatType]int, len(s.Attributes))
	for _, a := range s.Attributes {
		if a.Cap > 0 {
			out[a.ID] = a.Cap
		}
	}
	return out
}

// TrainableSet returns the string-keyed trainable map in the same shape as
// TrainingConfig.Trainable. NOTE: the live train gate (entityTrainable) does
// NOT use this — it calls Get() per-stat to avoid allocating a map per train
// attempt. This exists for content tooling + the classic-set regression test
// (TestCorePack_ClassicSetMatchesEngineDefaults); reach for Get, not this, when
// touching the gate.
func (s *AttributeSet) TrainableSet() map[string]bool {
	out := make(map[string]bool, len(s.Attributes))
	for _, a := range s.Attributes {
		if a.Trainable {
			out[string(a.ID)] = true
		}
	}
	return out
}

// Get returns the declared Attribute for a stat key. Case-insensitive;
// (Attribute{}, false) on miss.
func (s *AttributeSet) Get(id StatType) (Attribute, bool) {
	key := StatType(strings.ToLower(strings.TrimSpace(string(id))))
	for _, a := range s.Attributes {
		if a.ID == key {
			return a, true
		}
	}
	return Attribute{}, false
}

// AttributeSetRegistry holds attribute-set definitions keyed by case-insensitive
// id. Mirrors LanguageRegistry.
type AttributeSetRegistry struct {
	mu   sync.RWMutex
	sets map[string]*AttributeSet
}

// NewAttributeSetRegistry returns an empty registry.
func NewAttributeSetRegistry() *AttributeSetRegistry {
	return &AttributeSetRegistry{sets: make(map[string]*AttributeSet)}
}

// Register installs s. Returns nil on success; an error if the definition is
// malformed (nil, empty set id, empty/duplicate attribute id). Set id,
// attribute ids, and categories are lowercased on registration. Higher
// priority replaces; equal priority no-ops.
func (r *AttributeSetRegistry) Register(s *AttributeSet) error {
	if s == nil {
		return fmt.Errorf("progression: nil AttributeSet")
	}
	id := strings.ToLower(strings.TrimSpace(s.ID))
	if id == "" {
		return fmt.Errorf("progression: attribute set missing id")
	}
	if len(s.Attributes) == 0 {
		return fmt.Errorf("progression: attribute set %q has no attributes", id)
	}

	// Clone + normalize the attributes, rejecting empty/duplicate ids so a
	// malformed set fails at load with pack attribution rather than seeding a
	// broken character later.
	attrs := make([]Attribute, 0, len(s.Attributes))
	seen := make(map[StatType]struct{}, len(s.Attributes))
	for i, a := range s.Attributes {
		key := StatType(strings.ToLower(strings.TrimSpace(string(a.ID))))
		if key == "" {
			return fmt.Errorf("progression: attribute set %q: attribute %d missing id", id, i)
		}
		if _, dup := seen[key]; dup {
			return fmt.Errorf("progression: attribute set %q: duplicate attribute id %q", id, key)
		}
		seen[key] = struct{}{}
		clone := a
		clone.ID = key
		clone.Category = strings.ToLower(strings.TrimSpace(a.Category))
		attrs = append(attrs, clone)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.sets[id]
	if ok && s.Priority <= existing.Priority {
		return nil
	}
	clone := *s
	clone.ID = id
	clone.Attributes = attrs
	r.sets[id] = &clone
	return nil
}

// Get returns the registered AttributeSet for id. Case-insensitive; (nil,
// false) on miss. Returns the registry-owned pointer — callers MUST NOT mutate
// it (nor its Attributes slice).
func (r *AttributeSetRegistry) Get(id string) (*AttributeSet, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sets[key]
	return s, ok
}

// Has reports whether an attribute set is registered under id.
func (r *AttributeSetRegistry) Has(id string) bool {
	_, ok := r.Get(id)
	return ok
}

// All returns every registered set in id-sorted order. Listing surface; not a
// hot path.
func (r *AttributeSetRegistry) All() []*AttributeSet {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.sets))
	for id := range r.sets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*AttributeSet, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.sets[id])
	}
	return out
}

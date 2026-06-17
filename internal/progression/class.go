package progression

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// ClassPathEntry is one row in a class's level path (spec
// progression.md §4.1). Level is the bound-track level at which the
// entry fires; AbilityID names the ability to teach; UnlockedVia
// (when non-empty) marks the entry as owned by another subsystem
// (quest reward, scripted hook), excluding it from the level-up
// grant path (§4.5).
type ClassPathEntry struct {
	Level       int
	AbilityID   string
	UnlockedVia string
}

// Class is a content-defined character class (spec §4.1).
//
// StatGrowth maps a base stat to the dice expression rolled on every
// level-up of the bound track. GrowthBonuses maps the same stat to a
// source stat whose effective value contributes the D&D-style
// modifier (max(0, (v-10)/2)) to that level-up's growth roll.
//
// BoundTrack gates the level-up path processor (§4.5 step 2). An
// empty BoundTrack means the class has no growth-on-level behavior;
// stat growth is also skipped.
//
// Class is value-typed for registry storage. The registry hands
// callers a pointer to its own copy; callers MUST NOT mutate it.
type Class struct {
	ID           string
	DisplayName  string
	Tagline      string
	Description  string
	LevelUpFlavor string

	// BoundTrack is the case-insensitive track name a level-up event
	// must match for this class's path + stat growth to fire
	// (spec §4.5 step 2). Empty disables both.
	BoundTrack string

	// StatGrowth maps a base stat to the dice expression rolled on
	// every bound-track level-up (§4.6 step 3). Dice are parsed at
	// load time so the registry holds the parsed form, not the
	// authoring string.
	StatGrowth map[StatType]combat.DiceExpr

	// GrowthBonuses maps a stat to a source stat. When a stat has a
	// GrowthGrowth entry AND a GrowthBonuses entry, the level-up
	// handler adds `max(0, (effective(source)-10)/2)` to the rolled
	// growth (§4.1 / §4.6 step 3 second bullet).
	GrowthBonuses map[StatType]StatType

	// StartingStats is a flat base-stat grant applied ONCE at character
	// creation (added to the base via AdjustBase, so it composes additively
	// across a multiclass character). Unlike StatGrowth (dice rolled on each
	// level-up) this is a deterministic level-1 endowment — the mechanism a
	// channeler uses to begin with a non-zero resource_max (its One Power
	// pool capacity), which DefaultPlayerBase leaves at 0. Persisted into
	// the base snapshot, so it survives relogin without re-applying.
	StartingStats map[StatType]int

	// Path is the (level, abilityId, unlockedVia) list (§4.1). Path
	// entries with non-empty UnlockedVia are skipped by the
	// processor; the field exists so content can declare which
	// abilities exist for documentation and selection UIs.
	Path []ClassPathEntry

	// TrainsPerLevel is the number of trains credited on every
	// bound-track level-up (§4.1; §4.6 step 4). Default 5 in spec;
	// negative values are clamped to zero at registration.
	TrainsPerLevel int

	// AllowedCategories filters character-creation eligibility by
	// race category (§4.1). Empty = unrestricted.
	AllowedCategories []string

	// AllowedGenders filters character-creation eligibility by
	// gender (§4.1). Empty = unrestricted.
	AllowedGenders []string

	// ProficiencyTiers is the set of weapon proficiency tiers this class
	// grants (weapon-identity §3, e.g. "simple", "martial"). A character
	// is proficient with any weapon whose tier is in this set. Composed
	// across a multiclass character's classes. Lowercased at Register.
	ProficiencyTiers []string

	// ProficiencyCategories is the set of specific weapon categories this
	// class grants proficiency with beyond any tier grant (weapon-identity
	// §3, e.g. simple weapons plus a few named martial kinds). Lowercased
	// at Register.
	ProficiencyCategories []string

	// ArmorProficiencyTiers is the set of armor tiers this class may wear
	// without the non-proficient consequence (armor-depth §5, e.g. "light",
	// "medium", "heavy"), mirroring ProficiencyTiers for weapons. Composed
	// across a multiclass character's classes. Empty = no tiered armor is
	// worn proficiently. Lowercased at Register.
	ArmorProficiencyTiers []string

	// SaveProgressions declares this class's base-save curve per axis
	// (saves §2): a SaveType mapped to strong or weak. An axis the class
	// does not list defaults to weak. Composed across a multiclass
	// character by taking the strongest contributing class per axis
	// (saves §2 best-per-axis). Deep-copied at Register.
	SaveProgressions map[SaveType]SaveProgression

	// StartingAlignment seeds new characters' alignment (§4.1
	// "presentation fields"). Read only by M12 character creation;
	// unused at runtime today.
	StartingAlignment int

	// Pack records which pack registered this class. Diagnostic
	// only — mirrors Race.Pack / TrackDef.Pack.
	Pack string

	// Priority drives override semantics: higher wins on a name
	// collision; equal priority is a no-op (existing entry
	// retained). Mirrors the race/track registries.
	Priority int
}

// ClassRegistry holds class definitions keyed by case-insensitive
// id. Mirrors RaceRegistry / TrackRegistry shape.
type ClassRegistry struct {
	mu      sync.RWMutex
	classes map[string]*Class
}

// NewClassRegistry returns an empty registry.
func NewClassRegistry() *ClassRegistry {
	return &ClassRegistry{classes: make(map[string]*Class)}
}

// Register installs c. Returns nil on success; an error if the
// definition is malformed (empty id). Id is lowercased on
// registration so case-insensitive lookups work without per-call
// allocation. Higher priority replaces; equal priority no-ops.
//
// Maps and slices on c are deep-copied so a caller that mutates
// its source after Register cannot reach into the registry.
func (cr *ClassRegistry) Register(c *Class) error {
	if c == nil {
		return fmt.Errorf("progression: nil Class")
	}
	id := strings.ToLower(strings.TrimSpace(c.ID))
	if id == "" {
		return fmt.Errorf("progression: class missing id")
	}
	cr.mu.Lock()
	defer cr.mu.Unlock()
	existing, ok := cr.classes[id]
	if ok && c.Priority <= existing.Priority {
		return nil
	}
	clone := *c
	clone.ID = id
	clone.BoundTrack = strings.TrimSpace(c.BoundTrack)
	if clone.TrainsPerLevel < 0 {
		clone.TrainsPerLevel = 0
	}
	if len(c.StatGrowth) > 0 {
		g := make(map[StatType]combat.DiceExpr, len(c.StatGrowth))
		for k, v := range c.StatGrowth {
			g[StatType(strings.ToLower(string(k)))] = v
		}
		clone.StatGrowth = g
	}
	if len(c.GrowthBonuses) > 0 {
		b := make(map[StatType]StatType, len(c.GrowthBonuses))
		for k, v := range c.GrowthBonuses {
			b[StatType(strings.ToLower(string(k)))] = StatType(strings.ToLower(string(v)))
		}
		clone.GrowthBonuses = b
	}
	if len(c.StartingStats) > 0 {
		s := make(map[StatType]int, len(c.StartingStats))
		for k, v := range c.StartingStats {
			s[StatType(strings.ToLower(string(k)))] = v
		}
		clone.StartingStats = s
	}
	if len(c.Path) > 0 {
		p := make([]ClassPathEntry, len(c.Path))
		copy(p, c.Path)
		clone.Path = p
	}
	if len(c.AllowedCategories) > 0 {
		cats := make([]string, len(c.AllowedCategories))
		for i, v := range c.AllowedCategories {
			cats[i] = strings.ToLower(strings.TrimSpace(v))
		}
		clone.AllowedCategories = cats
	}
	if len(c.AllowedGenders) > 0 {
		gens := make([]string, len(c.AllowedGenders))
		for i, v := range c.AllowedGenders {
			gens[i] = strings.ToLower(strings.TrimSpace(v))
		}
		clone.AllowedGenders = gens
	}
	if len(c.ProficiencyTiers) > 0 {
		ts := make([]string, len(c.ProficiencyTiers))
		for i, v := range c.ProficiencyTiers {
			ts[i] = strings.ToLower(strings.TrimSpace(v))
		}
		clone.ProficiencyTiers = ts
	}
	if len(c.ProficiencyCategories) > 0 {
		cs := make([]string, len(c.ProficiencyCategories))
		for i, v := range c.ProficiencyCategories {
			cs[i] = strings.ToLower(strings.TrimSpace(v))
		}
		clone.ProficiencyCategories = cs
	}
	if len(c.ArmorProficiencyTiers) > 0 {
		at := make([]string, len(c.ArmorProficiencyTiers))
		for i, v := range c.ArmorProficiencyTiers {
			at[i] = strings.ToLower(strings.TrimSpace(v))
		}
		clone.ArmorProficiencyTiers = at
	}
	if len(c.SaveProgressions) > 0 {
		sp := make(map[SaveType]SaveProgression, len(c.SaveProgressions))
		for k, v := range c.SaveProgressions {
			sp[SaveType(strings.ToLower(string(k)))] = SaveProgression(strings.ToLower(string(v)))
		}
		clone.SaveProgressions = sp
	}
	cr.classes[id] = &clone
	return nil
}

// Get returns the registered Class for id. Case-insensitive lookup;
// (nil, false) on miss. Returns the registry-owned pointer — callers
// MUST NOT mutate it.
func (cr *ClassRegistry) Get(id string) (*Class, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return nil, false
	}
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	c, ok := cr.classes[key]
	return c, ok
}

// Has reports whether a class is registered under id.
func (cr *ClassRegistry) Has(id string) bool {
	_, ok := cr.Get(id)
	return ok
}

// All returns every registered class in id-sorted order.
func (cr *ClassRegistry) All() []*Class {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	ids := make([]string, 0, len(cr.classes))
	for id := range cr.classes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Class, 0, len(ids))
	for _, id := range ids {
		out = append(out, cr.classes[id])
	}
	return out
}

// GetEligible returns the classes whose AllowedCategories +
// AllowedGenders accept the supplied race category and gender
// (spec §4.3). Empty list on the class side means "no restriction"
// on that axis. Inputs are case-insensitive.
//
// Used by the M12 character-creation wizard; ships in M8.4 with a
// unit test (per ROADMAP M8.4 acceptance criteria).
func (cr *ClassRegistry) GetEligible(raceCategory, gender string) []*Class {
	cat := strings.ToLower(strings.TrimSpace(raceCategory))
	gen := strings.ToLower(strings.TrimSpace(gender))
	out := make([]*Class, 0)
	for _, c := range cr.All() {
		if !categoryAllowed(c.AllowedCategories, cat) {
			continue
		}
		if !categoryAllowed(c.AllowedGenders, gen) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// categoryAllowed reports whether value is in the allow list, or
// the allow list is empty (unrestricted). Comparisons are
// case-insensitive against pre-lowercased values; the list itself
// was lowercased at Register.
func categoryAllowed(list []string, value string) bool {
	if len(list) == 0 {
		return true
	}
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

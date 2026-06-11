package progression

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Background is a content-defined character-creation origin (docs/specs/
// backgrounds.md §2): a starting package of skill grants, starting items, and
// starting gold, applied ONCE at creation. It is the third creation axis after
// race and class, and the main lever of build variety with a single class.
//
// Background mirrors the Race/Class registry shape: value-typed for storage,
// the registry hands callers a pointer to its own copy (callers MUST NOT
// mutate it), higher priority wins on an id collision.
type Background struct {
	// ID is the stable case-insensitive identity; lowercased on Register.
	ID string

	// DisplayName / Tagline / Description are the presentation fields a
	// creation menu and the score sheet show. DisplayName falls back to ID.
	DisplayName string
	Tagline     string
	Description string

	// Skills are the skill proficiencies this background teaches at creation
	// (backgrounds §2). Granted through the same Teach seam class Path entries
	// use; an unknown ability id is skipped at grant time (fail-soft).
	Skills []BackgroundSkill

	// Items are item template ids placed in the new character's inventory at
	// creation (backgrounds §4). The first starting-loadout mechanism; unknown
	// ids are skipped at grant time.
	Items []string

	// Feats are feat ids granted (authored, free) at creation — the WoT
	// background-feat (backgrounds §2, EPIC S4 Phase 5). Global feat ids (not
	// namespaced, like abilities). Granted bypassing the slot cost + prereqs;
	// an unknown id is skipped at grant time (fail-soft). v1 background feats
	// are non-parameterized; a per-parameter background feat would need a
	// richer shape.
	Feats []string

	// Gold is added to the new character's starting balance (backgrounds §4).
	Gold int

	// AllowedCategories / AllowedGenders gate which characters may pick this
	// background at creation (mirrors Class eligibility, §3). Empty =
	// unrestricted on that axis.
	AllowedCategories []string
	AllowedGenders    []string

	// Pack records which pack registered this background (diagnostic only).
	Pack string

	// Priority drives override semantics: higher wins on an id collision;
	// equal priority is a no-op (existing entry retained).
	Priority int
}

// BackgroundSkill is one skill grant on a Background (backgrounds §2): an
// ability id and the proficiency it starts at. A non-positive Proficiency is
// treated as the baseline trained value (1) at grant time.
type BackgroundSkill struct {
	AbilityID   string
	Proficiency int
}

// BackgroundRegistry holds background definitions keyed by case-insensitive
// id. Mirrors RaceRegistry / ClassRegistry.
type BackgroundRegistry struct {
	mu          sync.RWMutex
	backgrounds map[string]*Background
}

// NewBackgroundRegistry returns an empty registry.
func NewBackgroundRegistry() *BackgroundRegistry {
	return &BackgroundRegistry{backgrounds: make(map[string]*Background)}
}

// Register installs b. Returns nil on success; an error if the definition is
// malformed (nil or empty id). Id is lowercased on registration. Maps and
// slices are deep-copied so a caller mutating its source after Register cannot
// reach into the registry. Higher priority replaces; equal priority no-ops.
func (br *BackgroundRegistry) Register(b *Background) error {
	if b == nil {
		return fmt.Errorf("progression: nil Background")
	}
	id := strings.ToLower(strings.TrimSpace(b.ID))
	if id == "" {
		return fmt.Errorf("progression: background missing id")
	}
	br.mu.Lock()
	defer br.mu.Unlock()
	existing, ok := br.backgrounds[id]
	if ok && b.Priority <= existing.Priority {
		return nil
	}
	clone := *b
	clone.ID = id
	if len(b.Skills) > 0 {
		sk := make([]BackgroundSkill, len(b.Skills))
		for i, s := range b.Skills {
			sk[i] = BackgroundSkill{
				AbilityID:   strings.ToLower(strings.TrimSpace(s.AbilityID)),
				Proficiency: s.Proficiency,
			}
		}
		clone.Skills = sk
	}
	if len(b.Items) > 0 {
		items := make([]string, len(b.Items))
		for i, it := range b.Items {
			items[i] = strings.ToLower(strings.TrimSpace(it))
		}
		clone.Items = items
	}
	if len(b.Feats) > 0 {
		feats := make([]string, len(b.Feats))
		for i, ft := range b.Feats {
			feats[i] = strings.ToLower(strings.TrimSpace(ft))
		}
		clone.Feats = feats
	}
	if len(b.AllowedCategories) > 0 {
		cats := make([]string, len(b.AllowedCategories))
		for i, v := range b.AllowedCategories {
			cats[i] = strings.ToLower(strings.TrimSpace(v))
		}
		clone.AllowedCategories = cats
	}
	if len(b.AllowedGenders) > 0 {
		gens := make([]string, len(b.AllowedGenders))
		for i, v := range b.AllowedGenders {
			gens[i] = strings.ToLower(strings.TrimSpace(v))
		}
		clone.AllowedGenders = gens
	}
	br.backgrounds[id] = &clone
	return nil
}

// Get returns the registered Background for id. Case-insensitive; (nil, false)
// on miss. Returns the registry-owned pointer — callers MUST NOT mutate it.
func (br *BackgroundRegistry) Get(id string) (*Background, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return nil, false
	}
	br.mu.RLock()
	defer br.mu.RUnlock()
	b, ok := br.backgrounds[key]
	return b, ok
}

// Has reports whether a background is registered under id.
func (br *BackgroundRegistry) Has(id string) bool {
	_, ok := br.Get(id)
	return ok
}

// All returns every registered background in id-sorted order. Used by the
// creation menu; not on a hot path.
func (br *BackgroundRegistry) All() []*Background {
	br.mu.RLock()
	defer br.mu.RUnlock()
	ids := make([]string, 0, len(br.backgrounds))
	for id := range br.backgrounds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Background, 0, len(ids))
	for _, id := range ids {
		out = append(out, br.backgrounds[id])
	}
	return out
}

// GetEligible returns the backgrounds whose AllowedCategories + AllowedGenders
// accept the supplied race category and gender (backgrounds §3). Empty list on
// the background side means "no restriction" on that axis. Inputs are
// case-insensitive. Mirrors ClassRegistry.GetEligible.
func (br *BackgroundRegistry) GetEligible(raceCategory, gender string) []*Background {
	cat := strings.ToLower(strings.TrimSpace(raceCategory))
	gen := strings.ToLower(strings.TrimSpace(gender))
	out := make([]*Background, 0)
	for _, b := range br.All() {
		if !categoryAllowed(b.AllowedCategories, cat) {
			continue
		}
		if !categoryAllowed(b.AllowedGenders, gen) {
			continue
		}
		out = append(out, b)
	}
	return out
}

// Package feat holds the content-defined player-chosen feat vocabulary and
// registry (EPIC S4 — docs/proposals/wot-feats.md §2.1). A feat is a binary
// perk: a character either has it or not (no ranks). This Phase-0 substrate
// carries a feat's identity, prerequisites, multi-take rule, and class gate;
// it deliberately holds NO grant payload — what a feat *confers* is wired into
// the modifier surfaces in Phase 3. The registry mirrors the race/background
// registries (deep-copy on register, higher priority wins on an id collision).
package feat

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// MultiTake describes whether a feat may be taken more than once (feats §2.1).
type MultiTake string

const (
	// MultiTakeSingle — take once. The default when a feat omits the rule.
	MultiTakeSingle MultiTake = "single"
	// MultiTakeParam — take multiple times, each instance bound to a distinct
	// parameter (a weapon/skill); effects do NOT stack (Weapon Focus, Improved
	// Critical, Skill Emphasis).
	MultiTakeParam MultiTake = "per_param"
	// MultiTakeStackable — take multiple times, effects DO stack, tracked by a
	// count (Toughness).
	MultiTakeStackable MultiTake = "stackable"
)

// ValidMultiTake reports whether m is a known rule.
func ValidMultiTake(m MultiTake) bool {
	switch m {
	case MultiTakeSingle, MultiTakeParam, MultiTakeStackable:
		return true
	}
	return false
}

// PrereqKind discriminates a feat prerequisite (feats §2.1). Evaluation — the
// Eligible check — is Phase 1; Phase 0 only carries the declaration.
type PrereqKind string

const (
	// PrereqAbilityScore — a minimum ability score (e.g. str 13). Target is the
	// stat name, Min the threshold.
	PrereqAbilityScore PrereqKind = "ability_score"
	// PrereqFeat — another feat must already be held. Target is the feat id;
	// Min is unused.
	PrereqFeat PrereqKind = "feat"
	// PrereqSkill — a minimum proficiency in a skill ability. Target is the
	// ability id, Min the proficiency floor.
	PrereqSkill PrereqKind = "skill"
	// PrereqLevel — a minimum character level. Min is the floor; Target unused.
	PrereqLevel PrereqKind = "level"
)

// ValidPrereqKind reports whether k is a known kind.
func ValidPrereqKind(k PrereqKind) bool {
	switch k {
	case PrereqAbilityScore, PrereqFeat, PrereqSkill, PrereqLevel:
		return true
	}
	return false
}

// Prerequisite is one gate on taking a feat (feats §2.1).
type Prerequisite struct {
	// Kind selects which field of a character the gate reads.
	Kind PrereqKind
	// Target names the stat / feat id / skill ability id the gate reads. Empty
	// for PrereqLevel (which reads character level, not a named target).
	Target string
	// Min is the threshold (score / proficiency / level). Unused for
	// PrereqFeat (presence is the gate, not a magnitude).
	Min int
}

// Feat is a content-defined player-chosen passive perk (feats §2.1). It is
// value-typed for registry storage; the registry hands callers a pointer to
// its own copy — callers MUST NOT mutate it.
type Feat struct {
	// ID is the stable case-insensitive identity; lowercased on Register.
	ID string

	// DisplayName / Description are the presentation fields a selection menu
	// and a `feats` listing show. DisplayName falls back to ID.
	DisplayName string
	Description string

	// Prerequisites are the gates a character must satisfy to take this feat
	// (feats §2.1). Empty = no prerequisite. Evaluated in Phase 1.
	Prerequisites []Prerequisite

	// Grants are the bonuses this feat confers (EPIC S4 Phase 3 — §2.4). Empty
	// is valid (a prereq-only "doorway" feat, like Latent Dreamer). Applied via
	// ComputeBonuses; the kinds a slice consumes grow per phase.
	Grants []Grant

	// MultiTake is the repeat rule (defaults to MultiTakeSingle on Register).
	MultiTake MultiTake

	// AllowedClasses optionally restricts which classes may take this feat
	// (mirrors background eligibility); empty = any. Carries the channeling /
	// class-special restrictions later.
	AllowedClasses []string

	// Pack records which pack registered this feat (diagnostic only).
	Pack string

	// Priority drives override semantics: higher wins on an id collision;
	// equal priority is a no-op (existing entry retained).
	Priority int

	// KarmaCost, when > 0, makes this feat a buyable QUALITY in a karma-ledger
	// world (SR-M5b): the `improve` verb grants it for this many karma instead of
	// spending a feat slot. 0 = not karma-buyable (feat-slot only). A scalar, so
	// Register's shallow clone copies it automatically.
	KarmaCost int
}

// Registry holds feat definitions keyed by case-insensitive id. Mirrors the
// progression race/background registries.
type Registry struct {
	mu    sync.RWMutex
	feats map[string]*Feat
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{feats: make(map[string]*Feat)}
}

// Register installs f. Returns nil on success; an error if the definition is
// malformed (nil or empty id). Id is lowercased on registration; the
// multi-take rule defaults to single; prereq targets / allowed classes are
// lowercased. Slices are deep-copied so a caller mutating its source after
// Register cannot reach into the registry. Higher priority replaces; equal
// priority no-ops.
func (r *Registry) Register(f *Feat) error {
	if f == nil {
		return fmt.Errorf("feat: nil Feat")
	}
	id := strings.ToLower(strings.TrimSpace(f.ID))
	if id == "" {
		return fmt.Errorf("feat: feat missing id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.feats[id]
	if ok && f.Priority <= existing.Priority {
		return nil
	}
	clone := *f
	clone.ID = id
	mt := MultiTake(strings.ToLower(strings.TrimSpace(string(f.MultiTake))))
	if mt == "" {
		mt = MultiTakeSingle
	}
	clone.MultiTake = mt
	if len(f.Prerequisites) > 0 {
		ps := make([]Prerequisite, len(f.Prerequisites))
		for i, p := range f.Prerequisites {
			ps[i] = Prerequisite{
				Kind:   PrereqKind(strings.ToLower(strings.TrimSpace(string(p.Kind)))),
				Target: strings.ToLower(strings.TrimSpace(p.Target)),
				Min:    p.Min,
			}
		}
		clone.Prerequisites = ps
	}
	if len(f.Grants) > 0 {
		gs := make([]Grant, len(f.Grants))
		for i, g := range f.Grants {
			gs[i] = Grant{
				Kind:      GrantKind(strings.ToLower(strings.TrimSpace(string(g.Kind)))),
				Target:    strings.ToLower(strings.TrimSpace(g.Target)),
				Magnitude: g.Magnitude,
			}
		}
		clone.Grants = gs
	}
	if len(f.AllowedClasses) > 0 {
		cs := make([]string, len(f.AllowedClasses))
		for i, c := range f.AllowedClasses {
			cs[i] = strings.ToLower(strings.TrimSpace(c))
		}
		clone.AllowedClasses = cs
	}
	r.feats[id] = &clone
	return nil
}

// Get returns the registered Feat for id. Case-insensitive; (nil, false) on
// miss. Returns the registry-owned pointer — callers MUST NOT mutate it.
func (r *Registry) Get(id string) (*Feat, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.feats[key]
	return f, ok
}

// Has reports whether a feat is registered under id.
func (r *Registry) Has(id string) bool {
	_, ok := r.Get(id)
	return ok
}

// All returns every registered feat in id-sorted order. Used by the selection
// menu / `feats` listing; not on a hot path.
func (r *Registry) All() []*Feat {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.feats))
	for id := range r.feats {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Feat, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.feats[id])
	}
	return out
}

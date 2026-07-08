package progression

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// TrackDef is a progression-track definition (docs/specs/
// progression.md §5.1). One track = one XP ladder; an entity has a
// (level, xp) pair per track it has ever earned on.
//
// XPTable and XPFormula are mutually exclusive on the "which one
// drives GetXpForLevel" axis: when both are set the table takes
// priority per spec §5.1. Both unset is a degenerate track — every
// threshold returns -1 (the sentinel for "undefined") and
// GrantExperience cascades stop immediately.
type TrackDef struct {
	// Name is the stable identity, canonicalized to lowercase at
	// Register and on lookup (like ability/class/attribute ids), so a
	// class `bound_track` resolves regardless of authored case.
	// Content authors that want a display string layer that on
	// DisplayName below.
	Name string

	// DisplayName is the human-facing label. Falls back to Name in
	// the score panel renderer when empty.
	DisplayName string

	// MaxLevel is the cap. GrantExperience stops cascading once the
	// entity reaches MaxLevel; further XP accumulates as overflow
	// (reported by TrackInfo, never clamped).
	MaxLevel int

	// XPTable[L] is the total XP required to reach level L. The
	// slice is indexed by level: XPTable[1] is the threshold for
	// level 1 (almost always 0), XPTable[2] is the threshold for
	// level 2, etc. A nil table delegates to XPFormula.
	XPTable []int64

	// XPFormula returns the total XP to reach the given level.
	// Called only when XPTable is nil. Should return a non-negative
	// integer for valid levels; any negative value short-circuits
	// the level-up cascade (caller treats it as "undefined").
	XPFormula func(level int) int64

	// OnLevelUp, if non-nil, fires once per level-up step in
	// addition to the eventbus event the Manager emits. Used for
	// track-specific side effects that are too narrow for a
	// subscriber (e.g. a one-off achievement on the "first kill"
	// track). Runs synchronously inside GrantExperience; should not
	// block.
	OnLevelUp func(entityID, trackName string, newLevel int)

	// DeathPenalty is the fraction of XP-into-current-level lost on
	// death (spec §5.5 acknowledges but does not drive). 0.0
	// disables. Live wiring lands when the death-flow subscriber
	// for progression ships.
	DeathPenalty float64

	// Pack records which pack registered this track. Mirrors the
	// race/class registries (M8.3/M8.4) for "where did this come
	// from" diagnostics.
	Pack string

	// Priority drives override semantics in the registry: higher
	// wins on a name collision, equal-priority duplicates are
	// no-ops (existing entry retained). Matches the §4.2 pattern
	// the spec calls out across the progression registries.
	Priority int
}

// GetXpForLevel returns the total XP required to reach level. The
// rule (spec §5.1):
//   - level <= 0: 0 (level 1 has threshold 0 by convention).
//   - XPTable defined and level <= len(XPTable)-1: table[level].
//   - XPTable defined but level out of range: -1 (undefined).
//   - XPTable nil, XPFormula defined: formula(level), or -1 if it
//     returns a negative value.
//   - neither defined: -1.
//
// -1 is the spec-defined sentinel that short-circuits the
// GrantExperience cascade.
func (t *TrackDef) GetXpForLevel(level int) int64 {
	if level <= 0 {
		return 0
	}
	if len(t.XPTable) > 0 {
		if level < len(t.XPTable) {
			return t.XPTable[level]
		}
		return -1
	}
	if t.XPFormula != nil {
		v := t.XPFormula(level)
		if v < 0 {
			return -1
		}
		return v
	}
	return -1
}

// TrackRegistry holds track definitions keyed by case-sensitive
// name. Mirrors the race/class registry pattern: higher-priority
// Register replaces lower; equal-priority is a no-op (existing
// entry wins to keep the registration order behavior stable across
// pack discovery shuffles).
type TrackRegistry struct {
	mu     sync.RWMutex
	tracks map[string]*TrackDef
}

// NewTrackRegistry returns an empty registry.
func NewTrackRegistry() *TrackRegistry {
	return &TrackRegistry{tracks: make(map[string]*TrackDef)}
}

// Register installs t. Returns nil on success, an error if the
// definition is malformed (empty name, max level <= 0). Higher
// priority replaces an existing entry under the same name; equal
// priority is a no-op (existing entry retained).
//
// Validation here catches the simplest content-authoring footguns
// — empty Name or non-positive MaxLevel — at the boundary so the
// loader surfaces a precise error. Cross-cutting validation (does
// every class's bound track exist?) is the class registry's
// concern.
// canonTrackName normalizes a track name to its canonical lookup key —
// lowercased + trimmed, mirroring ability/attribute id normalization. Track
// names key BOTH the registry and per-character progression state, and a
// class's `bound_track` is authored freely, so a `bound_track: Martial` against
// a registered `martial` must resolve to one key. Without this the grant path
// (case-sensitive `Get`) silently misses while `score` (EqualFold) still shows
// a level — XP that appears earned but is never banked.
func canonTrackName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (r *TrackRegistry) Register(t *TrackDef) error {
	if t == nil {
		return fmt.Errorf("progression: nil TrackDef")
	}
	name := canonTrackName(t.Name)
	if name == "" {
		return fmt.Errorf("progression: track missing name")
	}
	if t.MaxLevel <= 0 {
		return fmt.Errorf("progression: track %q: max_level must be > 0 (got %d)", name, t.MaxLevel)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.tracks[name]
	if ok && t.Priority <= existing.Priority {
		return nil
	}
	clone := *t
	clone.Name = name
	r.tracks[name] = &clone
	return nil
}

// Get returns the registered TrackDef for name. (nil, false) on
// miss. Returns a pointer to the registry-owned struct — callers
// MUST NOT mutate it.
func (r *TrackRegistry) Get(name string) (*TrackDef, bool) {
	name = canonTrackName(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tracks[name]
	return t, ok
}

// Has reports whether a track is registered under name.
func (r *TrackRegistry) Has(name string) bool {
	name = canonTrackName(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tracks[name]
	return ok
}

// All returns a sorted-by-name copy of every registered track.
// Used by admin tooling and renderers; not on the hot path.
func (r *TrackRegistry) All() []*TrackDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tracks))
	for n := range r.tracks {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*TrackDef, 0, len(names))
	for _, n := range names {
		out = append(out, r.tracks[n])
	}
	return out
}

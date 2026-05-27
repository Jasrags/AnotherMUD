package progression

import (
	"sort"
	"sync"
)

// ProgressionState is the per-entity progression-track state (spec
// §5.2): two maps keyed by track name, holding the entity's current
// level and total XP earned on that track. Safe for concurrent use
// — the mutex is internal so callers (connActor today, MobInstance
// later) don't have to thread their own lock through.
//
// The state lives ON the entity per spec. Manager mutates it
// through this package's helpers; persistence rides on Snapshot /
// Restore. An entity with no recorded interaction on a track has
// no entry in either map; Manager.lazyInit seeds (level=1, xp=0)
// on first interaction.
type ProgressionState struct {
	mu     sync.Mutex
	levels map[string]int
	xp     map[string]int64
}

// NewProgressionState returns a fresh empty state. No tracks are
// pre-initialized — every track lazy-inits on first interaction
// (spec §5.3).
func NewProgressionState() *ProgressionState {
	return &ProgressionState{
		levels: make(map[string]int),
		xp:     make(map[string]int64),
	}
}

// Level returns the entity's current level on track. Returns 0 for
// a track the entity has never touched — the caller (typically
// Manager.GrantExperience) lazy-inits to 1 on first interaction
// per spec §5.3.
func (s *ProgressionState) Level(track string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.levels[track]
}

// XP returns the entity's total XP on track. Returns 0 for an
// uninitialized track.
func (s *ProgressionState) XP(track string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.xp[track]
}

// setLocked updates both level and XP under the state's lock.
// Package-private — callers go through Manager.
func (s *ProgressionState) setLocked(track string, level int, xp int64) {
	s.levels[track] = level
	s.xp[track] = xp
}

// Snapshot returns a deterministically-ordered serialization of the
// state suitable for persistence. Tracks are sorted by name so the
// YAML output is stable across saves.
func (s *ProgressionState) Snapshot() ProgressionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.levels) == 0 && len(s.xp) == 0 {
		return nil
	}
	// Union of track names across both maps — defensive against a
	// future code path that registers XP without level (or vice
	// versa).
	names := make(map[string]struct{}, len(s.levels))
	for n := range s.levels {
		names[n] = struct{}{}
	}
	for n := range s.xp {
		names[n] = struct{}{}
	}
	ordered := make([]string, 0, len(names))
	for n := range names {
		ordered = append(ordered, n)
	}
	sort.Strings(ordered)
	out := make(ProgressionSnapshot, 0, len(ordered))
	for _, n := range ordered {
		out = append(out, TrackEntry{
			Name:  n,
			Level: s.levels[n],
			XP:    s.xp[n],
		})
	}
	return out
}

// Restore replaces the state from a snapshot. Used at login.
// Empty snapshot resets to empty maps (the lazy-init invariant
// holds: uninitialized tracks read as (0, 0)).
func (s *ProgressionState) Restore(snap ProgressionSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.levels = make(map[string]int, len(snap))
	s.xp = make(map[string]int64, len(snap))
	for _, e := range snap {
		if e.Level > 0 {
			s.levels[e.Name] = e.Level
		}
		if e.XP > 0 {
			s.xp[e.Name] = e.XP
		}
	}
}

// TrackEntry is one (track, level, xp) tuple in the persisted
// shape.
type TrackEntry struct {
	Name  string `yaml:"name"`
	Level int    `yaml:"level"`
	XP    int64  `yaml:"xp"`
}

// ProgressionSnapshot is the persisted shape of a ProgressionState
// — an ordered list of TrackEntry. A list (not a map) so YAML
// round-trips preserve order; matches the BaseSnapshot /
// stats.Snapshot conventions in this package.
type ProgressionSnapshot []TrackEntry

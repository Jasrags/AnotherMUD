package progression

import (
	"context"
	"maps"
	"sort"
	"strings"
	"sync"
)

var _ AbilityGranter = (*ProficiencyManager)(nil)

// ProficiencyEntry is one (id, value) pair returned by Snapshot.
// Used by save/load to round-trip per-entity proficiency and cap
// maps in a stable order.
type ProficiencyEntry struct {
	ID    string `yaml:"id"`
	Value int    `yaml:"value"`
}

// AbilitySnapshot is the per-entity persisted shape: parallel
// (lowercased-id → value) maps for proficiency and cap. Maps are
// preferred over slices so YAML round-trips merge naturally; the
// sorted Snapshot accessor exists for deterministic iteration.
type AbilitySnapshot struct {
	Proficiency map[string]int `yaml:"proficiency,omitempty"`
	Cap         map[string]int `yaml:"cap,omitempty"`
}

// ProficiencyConfig is the host-side configuration for the
// proficiency manager (spec abilities-and-effects §8 "Default
// initial proficiency cap" / "Default proficiency cap when none
// set"). Construction-time only.
type ProficiencyConfig struct {
	// DefaultLearnCap is the cap value written at Learn time when
	// no per-ability DefaultCap is set and no cap entry exists yet
	// (spec §3.2). Defaults to 100 when zero or negative.
	DefaultLearnCap int
	// DefaultUnsetCap is the cap value reported by GetCap when no
	// cap entry exists for the entity (spec §3.3 "returns 100 if
	// not known"). Defaults to 100 when zero or negative.
	DefaultUnsetCap int
}

// DefaultProficiencyConfig returns the engine defaults: learn cap
// 100, unset-cap 100. Both match the spec §3.3 / §3.4 ceiling.
func DefaultProficiencyConfig() ProficiencyConfig {
	return ProficiencyConfig{DefaultLearnCap: 100, DefaultUnsetCap: 100}
}

// ProficiencyManager tracks per-entity proficiency and cap maps
// (spec abilities-and-effects §3). The manager is process-wide;
// per-entity state lives in two id-keyed maps guarded by a single
// RWMutex.
//
// Lookups against the ability registry are case-insensitive; the
// registry is consulted at Learn time to apply per-ability
// DefaultCap and at AbilityName lookup time so the training verb
// can render the player-facing name.
//
// The manager satisfies the AbilityProficiency seam declared in
// training.go (M8.6) so the M8.6 train/practice verbs become
// functional once a host wires one in.
type ProficiencyManager struct {
	registry *AbilityRegistry
	cfg      ProficiencyConfig

	mu   sync.RWMutex
	prof map[string]map[string]int // entityID -> abilityID -> proficiency
	caps map[string]map[string]int // entityID -> abilityID -> cap
}

// NewProficiencyManager returns a manager bound to registry. A nil
// registry is legal — Learn falls back to the configured
// DefaultLearnCap and AbilityName returns ("", false) — so tests
// can run without a fully-populated registry.
func NewProficiencyManager(registry *AbilityRegistry, cfg ProficiencyConfig) *ProficiencyManager {
	if cfg.DefaultLearnCap <= 0 {
		cfg.DefaultLearnCap = 100
	}
	if cfg.DefaultUnsetCap <= 0 {
		cfg.DefaultUnsetCap = 100
	}
	return &ProficiencyManager{
		registry: registry,
		cfg:      cfg,
		prof:     make(map[string]map[string]int),
		caps:     make(map[string]map[string]int),
	}
}

// normalizeID lowercases + trims; empty is rejected by callers.
func normalizeID(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// effectiveCap returns min(cap, 100). Spec §3.4: proficiency is
// clamped to [1, min(cap, 100)].
func effectiveCap(capValue int) int {
	if capValue < 0 {
		return 0
	}
	if capValue > 100 {
		return 100
	}
	return capValue
}

// clampProf clamps value to [1, min(cap, 100)] (spec §3.4).
// A zero or negative cap yields a zero return (the ability is
// effectively unteachable; manager keeps the entry intact with
// value 1 so subsequent Learns re-establish a working cap).
func clampProf(value, capValue int) int {
	ec := effectiveCap(capValue)
	if ec <= 0 {
		return 1
	}
	if value < 1 {
		return 1
	}
	if value > ec {
		return ec
	}
	return value
}

// Learn sets the proficiency entry for (entityID, abilityID) to
// value (clamped to [1, effective-cap]). If no cap entry exists,
// one is established from the registry's DefaultCap (when present)
// or from the configured DefaultLearnCap (spec §3.2).
//
// Empty ids are silent no-ops.
func (m *ProficiencyManager) Learn(entityID, abilityID string, value int) {
	eid := normalizeID(entityID)
	aid := normalizeID(abilityID)
	if eid == "" || aid == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Establish cap if missing.
	if _, ok := m.caps[eid][aid]; !ok {
		capValue := m.cfg.DefaultLearnCap
		if m.registry != nil {
			if a, ok := m.registry.Get(aid); ok && a.DefaultCap > 0 {
				capValue = a.DefaultCap
			}
		}
		if m.caps[eid] == nil {
			m.caps[eid] = make(map[string]int)
		}
		m.caps[eid][aid] = capValue
	}

	capValue := m.caps[eid][aid]
	if m.prof[eid] == nil {
		m.prof[eid] = make(map[string]int)
	}
	m.prof[eid][aid] = clampProf(value, capValue)
}

// Forget removes the proficiency entry for (entityID, abilityID).
// The cap entry is preserved so a re-learn restores the prior
// effective cap (spec §3.2 "cap is treated as character
// progression, not as a skill memory"). Empty ids and unknown
// entries are silent no-ops.
func (m *ProficiencyManager) Forget(entityID, abilityID string) {
	eid := normalizeID(entityID)
	aid := normalizeID(abilityID)
	if eid == "" || aid == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.prof[eid]; ok {
		delete(entry, aid)
		if len(entry) == 0 {
			delete(m.prof, eid)
		}
	}
}

// Has reports whether entityID carries a proficiency entry for
// abilityID. Spec §3.1: "An entity has an ability when its
// proficiency map contains an entry for that id."
func (m *ProficiencyManager) Has(entityID, abilityID string) bool {
	eid := normalizeID(entityID)
	aid := normalizeID(abilityID)
	if eid == "" || aid == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.prof[eid][aid]
	return ok
}

// Proficiency returns the proficiency value for (entityID,
// abilityID). The second return is false when no proficiency entry
// exists; the value is always 0 in that case.
func (m *ProficiencyManager) Proficiency(entityID, abilityID string) (int, bool) {
	eid := normalizeID(entityID)
	aid := normalizeID(abilityID)
	if eid == "" || aid == "" {
		return 0, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.prof[eid][aid]
	return v, ok
}

// Cap returns the cap value for (entityID, abilityID), falling
// back to the configured DefaultUnsetCap when no entry exists
// (spec §3.3). Always returns a positive value.
func (m *ProficiencyManager) Cap(entityID, abilityID string) int {
	eid := normalizeID(entityID)
	aid := normalizeID(abilityID)
	if eid == "" || aid == "" {
		return m.cfg.DefaultUnsetCap
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if v, ok := m.caps[eid][aid]; ok {
		return v
	}
	return m.cfg.DefaultUnsetCap
}

// SetCap writes a cap entry for (entityID, abilityID). Caps are
// clamped to [0, 100]. The entity need not have a proficiency
// entry first (training raises caps in advance of practice). If a
// proficiency entry exists and now exceeds the new effective cap,
// it is re-clamped to the new ceiling.
//
// Satisfies the AbilityProficiency seam (training.go).
func (m *ProficiencyManager) SetCap(entityID, abilityID string, capValue int) {
	eid := normalizeID(entityID)
	aid := normalizeID(abilityID)
	if eid == "" || aid == "" {
		return
	}
	if capValue < 0 {
		capValue = 0
	}
	if capValue > 100 {
		capValue = 100
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.caps[eid] == nil {
		m.caps[eid] = make(map[string]int)
	}
	m.caps[eid][aid] = capValue
	if entry, ok := m.prof[eid]; ok {
		if v, present := entry[aid]; present {
			entry[aid] = clampProf(v, capValue)
		}
	}
}

// AddProficiency increments the proficiency entry by delta,
// clamped to the effective cap. The entry is created at value 1
// before adding when no entry exists (matching the training
// catch-up boost path that calls AddProficiency on a freshly-set
// cap with no prior proficiency).
//
// Negative deltas are honored but the result is still floor-clamped
// to 1 (proficiency may never drop below 1 once learned, spec §3.4).
//
// Satisfies the AbilityProficiency seam (training.go).
func (m *ProficiencyManager) AddProficiency(entityID, abilityID string, delta int) {
	eid := normalizeID(entityID)
	aid := normalizeID(abilityID)
	if eid == "" || aid == "" || delta == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	capValue := m.cfg.DefaultUnsetCap
	if v, ok := m.caps[eid][aid]; ok {
		capValue = v
	}
	if m.prof[eid] == nil {
		m.prof[eid] = make(map[string]int)
	}
	current, ok := m.prof[eid][aid]
	if !ok {
		current = 1
	}
	m.prof[eid][aid] = clampProf(current+delta, capValue)
}

// GetCap implements the AbilityProficiency seam used by
// TrainingManager.TryPractice (M8.6). Returns (capValue,
// proficiency, learned) for (entityID, abilityID).
//
// Spec note: TryPractice keys on "has the entity learned this
// ability?" which is the proficiency-entry presence; the cap is
// reported even when the entity has not learned the ability so
// trainers can lift caps before first practice (matching the
// progression spec §7.5 catch-up flow).
func (m *ProficiencyManager) GetCap(entityID, abilityID string) (int, int, bool) {
	eid := normalizeID(entityID)
	aid := normalizeID(abilityID)
	if eid == "" || aid == "" {
		return 0, 0, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	capValue := m.cfg.DefaultUnsetCap
	if v, ok := m.caps[eid][aid]; ok {
		capValue = v
	}
	prof, learned := m.prof[eid][aid]
	return capValue, prof, learned
}

// RollUseGain rolls a single §3.5 use-based proficiency gain for abilityID
// on one use. hit reports whether the use succeeded (a miss gains at the
// ability's failure-multiplier rate). Returns true when proficiency
// increased. This is the shared use-driven gain path the passive resolver
// also implements (passive.go rollGain) — crafting and any future
// use-driven skill route through it rather than re-deriving the formula.
//
// roller must be non-nil. stats may be nil, in which case the optional
// gain-stat factor is skipped (gain rolls at the un-scaled rate). An
// unknown or unlearned ability never gains.
func (m *ProficiencyManager) RollUseGain(entityID, abilityID string, hit bool, roller Roller, stats StatReader) bool {
	if roller == nil || m.registry == nil {
		return false
	}
	ab, ok := m.registry.Get(abilityID)
	if !ok || ab == nil {
		return false
	}
	capValue, prof, learned := m.GetCap(entityID, abilityID)
	if !learned {
		return false
	}
	statFactor := 1.0
	if stats != nil && ab.GainStat != "" && ab.GainStatScale != 0 {
		statFactor = 1 + float64(stats.StatValue(entityID, ab.GainStat))*ab.GainStatScale
	}
	threshold := gainThreshold(
		ab.GainBaseChance, prof, effectiveCap(capValue),
		statFactor, ab.GainFailureMultiplier, hit,
	)
	if threshold <= 0 {
		return false
	}
	if roller.IntN(100)+1 <= threshold {
		m.AddProficiency(entityID, abilityID, 1)
		return true
	}
	return false
}

// AbilityName implements the AbilityProficiency seam: returns the
// registered DisplayName for abilityID, or ("", false) when the
// ability is unknown. Pass-through to the registry; the seam
// exists so TrainingManager need not import the registry directly.
func (m *ProficiencyManager) AbilityName(abilityID string) (string, bool) {
	if m.registry == nil {
		return "", false
	}
	if a, ok := m.registry.Get(abilityID); ok {
		return a.DisplayName, true
	}
	return "", false
}

// LearnedAbilities returns a snapshot of (id, proficiency) pairs
// for entityID, sorted by id. Spec §3.3 "All learned abilities of
// entity X". The returned slice is a fresh copy; mutating it does
// not affect manager state.
func (m *ProficiencyManager) LearnedAbilities(entityID string) []ProficiencyEntry {
	eid := normalizeID(entityID)
	if eid == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry := m.prof[eid]
	if len(entry) == 0 {
		return nil
	}
	ids := make([]string, 0, len(entry))
	for id := range entry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]ProficiencyEntry, 0, len(ids))
	for _, id := range ids {
		out = append(out, ProficiencyEntry{ID: id, Value: entry[id]})
	}
	return out
}

// Snapshot returns the persisted shape (parallel maps) for
// entityID. Maps are deep-copied so the caller may freely mutate
// the returned structure. Returns zero-value AbilitySnapshot when
// the entity has neither proficiency nor cap entries (which the
// player save layer should treat as "omit the abilities block").
func (m *ProficiencyManager) Snapshot(entityID string) AbilitySnapshot {
	eid := normalizeID(entityID)
	if eid == "" {
		return AbilitySnapshot{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := AbilitySnapshot{}
	if p := m.prof[eid]; len(p) > 0 {
		out.Proficiency = make(map[string]int, len(p))
		maps.Copy(out.Proficiency, p)
	}
	if c := m.caps[eid]; len(c) > 0 {
		out.Cap = make(map[string]int, len(c))
		maps.Copy(out.Cap, c)
	}
	return out
}

// Restore writes snap as the in-memory state for entityID,
// replacing any existing entries. Values are clamped on ingest:
// caps to [0, 100], proficiency to [1, effective-cap]. Used by the
// session-load path to rehydrate a player's abilities from the
// persisted Save.
//
// Empty entityID and an entirely-empty snapshot are silent no-ops;
// they intentionally leave the manager untouched so login of a
// brand-new character (with no abilities block) doesn't inflate
// internal maps.
func (m *ProficiencyManager) Restore(entityID string, snap AbilitySnapshot) {
	eid := normalizeID(entityID)
	if eid == "" {
		return
	}
	if len(snap.Proficiency) == 0 && len(snap.Cap) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Build cap map first so we can clamp proficiency against it.
	caps := make(map[string]int, len(snap.Cap))
	for k, v := range snap.Cap {
		id := normalizeID(k)
		if id == "" {
			continue
		}
		if v < 0 {
			v = 0
		}
		if v > 100 {
			v = 100
		}
		caps[id] = v
	}
	if len(caps) > 0 {
		m.caps[eid] = caps
	} else {
		delete(m.caps, eid)
	}
	prof := make(map[string]int, len(snap.Proficiency))
	for k, v := range snap.Proficiency {
		id := normalizeID(k)
		if id == "" {
			continue
		}
		capValue, ok := caps[id]
		if !ok {
			capValue = m.cfg.DefaultUnsetCap
		}
		prof[id] = clampProf(v, capValue)
	}
	if len(prof) > 0 {
		m.prof[eid] = prof
	} else {
		delete(m.prof, eid)
	}
}

// Teach implements the AbilityGranter seam (level_up.go) so the
// ClassPathProcessor's level-up grants land as real proficiency
// entries (spec abilities-and-effects §3 / progression.md §4.5
// step 4). Returns ("", false) when abilityID is not registered
// so the caller logs + skips per progression §4.5. On hit, the
// entity learns the ability at proficiency 1 with the ability's
// DefaultCap applied (Learn falls back to DefaultLearnCap when
// the ability omits one).
//
// Idempotent: granting an already-learned ability re-Learns at
// proficiency 1 only when the existing entry is missing. We
// preserve existing proficiency by checking Has first — a class
// path that grants the same ability at multiple levels (e.g.
// after a respec) shouldn't reset accumulated training progress.
func (m *ProficiencyManager) Teach(ctx context.Context, entityID, abilityID string) (string, bool) {
	name, ok := m.AbilityName(abilityID)
	if !ok {
		return "", false
	}
	if !m.Has(entityID, abilityID) {
		m.Learn(entityID, abilityID, 1)
	}
	return name, true
}

// Drop removes all in-memory state for entityID. Used at logout so
// the manager's working set stays bounded to currently-connected
// players. Persisted Save state is unaffected.
func (m *ProficiencyManager) Drop(entityID string) {
	eid := normalizeID(entityID)
	if eid == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.prof, eid)
	delete(m.caps, eid)
}

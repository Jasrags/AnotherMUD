package entities

import (
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/mob"
)

// MobInstance is a live mob built from a mob.Template. Mirrors
// ItemInstance in shape (id/type/name/tags/keywords/properties +
// per-instance bag), differing only in the originating template type
// and a couple of mob-specific reserved property keys (behavior is
// carried as a property rather than a typed field so future behavior
// dispatch can read it without an entity-type case split).
//
// M6.2 scope: the §2.3 instantiation minimum (steps 1-5). Equipment
// (§3.3), patrol/idle/battle command sets (§2.3 step 6), disposition
// rules (§2.3 step 7), and scripts (§2.3 step 8) are not carried here
// yet — they arrive with the slices that consume them. Template.Stats
// is copied into Properties under each stat-name key, satisfying
// §2.3 step 3 without forcing a typed vitals struct before combat /
// progression need one.
type MobInstance struct {
	id         EntityID
	typ        string
	name       string
	tags       []string
	keywords   []string
	templateID mob.TemplateID

	// propsMu guards properties access against the cross-goroutine
	// reader/writer hazard from m6-4 deferred fix. Tick-side
	// writers (ai/wander updating PropWanderNextAt) and session-
	// side readers (disposition evaluator invoked from
	// OnPlayerEnteredImmediate; future verb handlers reading mob
	// state) coexist. Map access in Go is not concurrent-safe even
	// for disjoint keys — the internal hashmap can reorganize
	// under a concurrent write. All property access goes through
	// Property / SetProperty / Properties, each of which holds
	// the appropriate lock.
	//
	// Scope-bound: this lock covers properties ONLY. Tags share
	// their own pre-existing surface (single-goroutine writers
	// today: ApplyRacialFlags from boot spawner, SetAlignmentTag
	// from manager which is itself caller-serialized). If tags
	// grow a session-side mutator a follow-up slice will extend
	// the lock; out of scope here.
	propsMu    sync.RWMutex
	properties map[string]any

	// vitals carries mutable HP state for the combat loop (M7.1). The
	// pointer is established at spawn time from the template's
	// hp_max (or the engine default) and never reassigned for the
	// lifetime of the instance — combat applies damage/heal through
	// the pointer, which carries its own mutex.
	vitals *combat.Vitals
	// stats is the per-mob derived block read by combat each round
	// (combat §4.4-4.5). Captured at spawn from the template's Stats
	// map; equipment-driven modifiers will overlay on top of this when
	// mobs grow real equipment slots. Today it's a static snapshot.
	stats combat.Stats

	// race is the optional race id copied from the template at
	// construction (M8.3). The spawn pipeline reads this via
	// RaceID() to resolve a *progression.Race and applies racial
	// flags via ApplyRacialFlags. Empty when the template declares
	// no race.
	race string
}

// Reserved property keys with engine-defined semantics on MobInstance.
// Mirror inventory-equipment-items §2.3's reserved-key approach for
// items; the names live here so the spawn helper and any future
// behavior dispatch reference one source of truth.
const (
	// PropBehavior records the behavior name copied from the template
	// (spec mobs-ai-spawning §2.3 step 5). AI dispatch reads it to
	// look up the behavior function.
	PropBehavior = "behavior"
)

// TagMob is the synthetic tag applied to every MobInstance at
// instantiation. Lets the AI dispatcher cheaply iterate all live
// mobs via Store.GetByTag without needing a per-instance type
// switch or a parallel registry. The tag is invisible to content
// authors (a template that re-declares it would be a no-op because
// §2.3 step 2 drops tags that match the implicit type) — but the
// content-side mob type ("npc", "monster", etc.) is what the spec's
// implicit-type-tag rule strips, not this engine-synthetic tag.
const TagMob = "mob"

// ID implements Entity.
func (m *MobInstance) ID() EntityID { return m.id }

// Type implements Entity.
func (m *MobInstance) Type() string { return m.typ }

// Tags implements Entity. Returns a fresh slice so callers cannot
// alias the backing storage; required for safe coexistence with the
// Store's tag index.
func (m *MobInstance) Tags() []string {
	return append([]string(nil), m.tags...)
}

// Name returns the display name copied from the template at
// construction time (spec §2.3 step 1).
func (m *MobInstance) Name() string { return m.name }

// Keywords returns the per-instance keyword list (used by the keyword
// resolver). Returns a fresh slice so callers cannot alias the backing
// storage — mirrors Tags() on the same type for consistency.
func (m *MobInstance) Keywords() []string {
	return append([]string(nil), m.keywords...)
}

// Properties returns a SNAPSHOT of the per-instance property bag
// (spec §2.3 step 6). Callers that need to mutate must use
// SetProperty — the returned map is detached from m and writes to
// it do not flow back. Returning a snapshot rather than the live
// map closes the m6-4 deferred fix: a session-goroutine reader
// (disposition evaluator's PropTemplateID lookup) no longer
// races the tick-goroutine writer (ai/wander's PropWanderNextAt
// update).
//
// Snapshot cost is O(n) per call where n is the property count
// (typically small — under 10 for a normal mob). Hot-path
// readers that need a single key should use Property(key) which
// avoids the copy.
func (m *MobInstance) Properties() map[string]any {
	m.propsMu.RLock()
	defer m.propsMu.RUnlock()
	if len(m.properties) == 0 {
		return nil
	}
	out := make(map[string]any, len(m.properties))
	for k, v := range m.properties {
		out[k] = v
	}
	return out
}

// Property reads a single property by key under RLock. Returns
// (zero, false) on miss. Use for tick-hot paths where the
// Properties() snapshot allocation is wasteful.
func (m *MobInstance) Property(key string) (any, bool) {
	m.propsMu.RLock()
	defer m.propsMu.RUnlock()
	v, ok := m.properties[key]
	return v, ok
}

// SetProperty writes a property under Lock. Replaces any prior
// value. The map is lazy-initialized — a mob whose template
// carried no properties still admits SetProperty calls.
func (m *MobInstance) SetProperty(key string, value any) {
	m.propsMu.Lock()
	defer m.propsMu.Unlock()
	if m.properties == nil {
		m.properties = make(map[string]any)
	}
	m.properties[key] = value
}

// TemplateID returns the source template id (§2.3 step 4 → set on
// the entity's properties; here we additionally surface a typed
// accessor so loot listeners and AI don't have to round-trip through
// the property bag for a value that never changes).
func (m *MobInstance) TemplateID() mob.TemplateID { return m.templateID }

// CombatantID returns the combat-side identity of this mob. The
// MobPrefix keeps the namespace disjoint from player ids (see
// combat.CombatantID); resolves to a unique string within the run
// because EntityID itself is unique within the entity store.
func (m *MobInstance) CombatantID() combat.CombatantID {
	return combat.NewMobCombatantID(string(m.id))
}

// Vitals returns the mob's mutable hit-point state. The pointer is
// stable for the life of the instance; combat applies damage through
// the pointer under its own lock.
func (m *MobInstance) Vitals() *combat.Vitals { return m.vitals }

// Stats returns a copy of the mob's combat stat block (combat §4.4-4.5).
// A value copy is intentional — the round loop's hit/damage rolls read
// a fresh block per swing so equipment changes between rounds cannot
// tear the inputs to a single swing.
func (m *MobInstance) Stats() combat.Stats { return m.stats }

// RaceID returns the optional race id copied from the template at
// construction (M8.3 — progression.md §3.1). Empty for mobs whose
// template declares no race. Spawn-side code reads this to resolve
// the race definition and apply ApplyRacialFlags + alignment.
func (m *MobInstance) RaceID() string { return m.race }

// ApplyRacialFlags merges flags into the mob's tag list and seeds
// the alignment property. Called from the spawn pipeline AFTER
// Store.SpawnMob returns the freshly-tracked instance, so the tag
// index sees the additional tags via the next tag-swap tick (the
// underlying tags slice is appended to in place — duplicates are
// deduped by callers if they care).
//
// Primitive arguments (no *progression.Race) so this method does
// not pull progression into entities; the spawn-side adapter that
// resolves the race against the registry passes already-extracted
// materials in. Spec §3.1: racial flags + starting alignment apply
// at instantiation.
func (m *MobInstance) ApplyRacialFlags(flags []string, alignment int) {
	// Dedupe against existing tags so a flag that overlaps with a
	// template-declared tag doesn't produce two entries.
	if len(flags) > 0 {
		have := make(map[string]struct{}, len(m.tags))
		for _, t := range m.tags {
			have[t] = struct{}{}
		}
		for _, f := range flags {
			if _, exists := have[f]; exists {
				continue
			}
			m.tags = append(m.tags, f)
			have[f] = struct{}{}
		}
	}
	if alignment != 0 {
		m.SetProperty(PropAlignment, alignment)
	}
}

// PropAlignment is the reserved property key for the integer
// alignment value (spec progression.md §6.1). Written by the
// M8.5 AlignmentManager via SetAlignment; M8.3 seeds it from
// the race's StartingAlignment at instantiation.
const PropAlignment = "alignment"

// AlignmentBucketTags are the spec §6.2 mutually-exclusive
// alignment tag strings managed by SetAlignmentTag.
//
// Kept as bare strings (not a typed enum) so the entities
// package does not have to import progression; progression
// re-declares the canonical values as TagAlignmentEvil/Neutral/
// Good. Comparison is exact-string.
var alignmentBucketTags = [...]string{"alignment_evil", "alignment_neutral", "alignment_good"}

// Alignment returns the integer alignment stored in the property
// bag, or 0 when missing / malformed. Consumed by the M8.5
// AlignmentEntity adapter; matches the lenient-numeric handling
// in WimpyThreshold (YAML decode produces int / int64 / float64).
func (m *MobInstance) Alignment() int {
	m.propsMu.RLock()
	defer m.propsMu.RUnlock()
	switch v := m.properties[PropAlignment].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// SetAlignment writes the integer alignment property. Used by
// the AlignmentEntity adapter under the manager's lock. Does NOT
// adjust tags — SetAlignmentTag is the paired call.
func (m *MobInstance) SetAlignment(value int) {
	m.SetProperty(PropAlignment, value)
}

// SetAlignmentTag installs the bucket tag and removes the other
// two alignment_* tags (spec §6.2 "exactly one present at a
// time"). Empty tag clears all three.
//
// Mutates m.tags in place; the store's tag index does not
// automatically reflect the change because re-indexing happens
// only at Track/Untrack. AI disposition matching consumes
// PlayerView/MobView tags built directly from the entity (not
// from the store index), so the in-place mutation is sufficient
// for M8.5's evaluator wiring. A future GetByTag consumer would
// need a Store-side Retag mechanism — recorded as a deferred fix.
func (m *MobInstance) SetAlignmentTag(tag string) {
	out := m.tags[:0]
	for _, t := range m.tags {
		isBucket := false
		for _, b := range alignmentBucketTags {
			if t == b {
				isBucket = true
				break
			}
		}
		if !isBucket {
			out = append(out, t)
		}
	}
	if tag != "" {
		out = append(out, tag)
	}
	m.tags = out
}

// HasTag reports whether tag is present on the mob's tag list.
// Used by the AlignmentEntity adapter to detect the admin role
// bypass (spec §6.4 Shift step 2).
func (m *MobInstance) HasTag(tag string) bool {
	for _, t := range m.tags {
		if t == tag {
			return true
		}
	}
	return false
}

// WimpyThreshold reports the mob's HP-percent flee threshold (spec
// combat §5.1). Read from the template's properties bag at spawn
// time (key "wimpy_threshold"); 0 (or any non-int / out-of-range
// value) disables wimpy. Satisfies combat.WimpyHolder so the wimpy
// phase in the heartbeat triggers a §5.2 flee attempt at or below
// the threshold.
//
// Reads via propsMu.RLock — closes the m6-4 deferred race
// between the heartbeat tick goroutine and any session-side
// property writer that might land later (M9 effects, M11
// economy, etc.).
func (m *MobInstance) WimpyThreshold() int {
	m.propsMu.RLock()
	defer m.propsMu.RUnlock()
	raw, ok := m.properties["wimpy_threshold"]
	if !ok {
		return 0
	}
	// YAML decode produces int OR int64 OR float64 depending on the
	// magnitude and document context. gopkg.in/yaml.v3 will pick
	// int for small bare integers but int64 for some paths and
	// float64 for any value with a decimal point. A naive raw.(int)
	// silently maps int64-decoded 50 → 0, which would create a
	// silent "my mob never flees" content-author trap. Switch over
	// the common numeric types and convert.
	var v int
	switch t := raw.(type) {
	case int:
		v = t
	case int64:
		v = int(t)
	case float64:
		v = int(t)
	default:
		return 0
	}
	if v < 0 || v > 100 {
		return 0
	}
	return v
}

// buildMobFromTemplate is the §2.3 instantiation algorithm. The id is
// assigned by the caller (Store.SpawnMob) so id generation stays
// under the store's lock.
//
// §2.3 step 1: fresh entity with the template's name + type.
// Step 2: copy tags, dropping any tag matching the entity's own type
// (implicit).
// Step 3: copy stats into properties under their declared keys. The
// "current vitals = max" rule from the spec is honored implicitly —
// we copy the template's declared max values; no separate "current"
// counterpart exists yet because no combat or regen system consumes
// it. When those land, they'll initialize current-vital keys (e.g.
// `hp`, `resource`, `movement`) to mirror the corresponding `*_max`.
// Step 4: PropTemplateID set on properties.
// Step 5: PropBehavior set on properties.
func buildMobFromTemplate(tpl *mob.Template, id EntityID) *MobInstance {
	// Properties bag: start from the template's free-form props,
	// then add stats and reserved keys. Stats live in their own
	// nested map on the template so we lift them flat into the
	// instance for direct read access (the spec doesn't dictate
	// shape; flat keys match how `fill` reads max_charges et al).
	props := make(map[string]any, len(tpl.Properties)+len(tpl.Stats)+2)
	for k, v := range tpl.Properties {
		props[k] = v
	}
	for k, v := range tpl.Stats {
		props[k] = v
	}
	props[PropTemplateID] = string(tpl.ID)
	props[PropBehavior] = tpl.Behavior

	// Tags: copy template tags minus any matching the entity's own
	// type (§2.3 step 2 — "implicit"). Append the engine-synthetic
	// TagMob so Store.GetByTag("mob") enumerates every mob without
	// the AI dispatcher needing a type switch over the by-id index.
	//
	// A template that accidentally declares `tags: [mob]` would
	// otherwise produce a duplicate entry in the slice (the store's
	// tag bucket is a map and dedupes silently, but the slice
	// surface is observable via Tags() and shouldn't lie). Track
	// whether the template already carried the synthetic tag and
	// skip the second append in that case.
	tags := make([]string, 0, len(tpl.Tags)+1)
	hasMobTag := false
	for _, t := range tpl.Tags {
		if t == tpl.Type {
			continue
		}
		if t == TagMob {
			hasMobTag = true
		}
		tags = append(tags, t)
	}
	if !hasMobTag {
		tags = append(tags, TagMob)
	}

	keywords := append([]string(nil), tpl.Keywords...)

	// M7.1: derive combat-side state from the template's free-form
	// Stats map. FromTemplateStats applies engine defaults for any
	// missing keys so a template that forgot hp_max still spawns a
	// fightable mob (better to be slightly off-balance than to spawn
	// a corpse). Vitals start at full per spec §2.3 — current HP
	// mirrors max at spawn.
	statBlock, maxHP := combat.FromTemplateStats(tpl.Stats)

	return &MobInstance{
		id:         id,
		typ:        tpl.Type,
		name:       tpl.Name,
		tags:       tags,
		keywords:   keywords,
		properties: props,
		templateID: tpl.ID,
		vitals:     combat.NewVitals(maxHP),
		stats:      statBlock,
		race:       tpl.Race,
	}
}

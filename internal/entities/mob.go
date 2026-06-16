package entities

import (
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/srckey"
	"github.com/Jasrags/AnotherMUD/internal/stats"
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
	desc       string
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
	// statBlock is the per-mob progression stat block read by combat
	// each round (combat §4.4-4.5) via Stats(). Built at spawn from the
	// template's Stats map; effect-driven modifiers (a poison/bless cast
	// on the mob) overlay through AddModifiers/RemoveBySource — the
	// MobInstance now satisfies progression.EffectTarget. The block
	// carries its own RWMutex, so Stats() reads and effect writes are
	// safe across the combat / effect tick goroutines without
	// MobInstance-level locking. Mirrors the player's connActor.statBlock.
	statBlock *progression.StatBlock

	// channelMap is the active ruleset's stat→combat-channel derivation,
	// stamped by the Store at spawn (and retro-stamped by SetChannelMap
	// for mobs spawned during pack Load). Nil in bare test-built mobs.
	// When set, Stats() derives HitMod/AC through it; when nil it reads the
	// stat keys directly — and since the baseline mapping reproduces those
	// exact reads, both paths yield identical numbers. Written only at
	// spawn / composition (before the tick loop), then read lock-free by
	// Stats() on the tick goroutine.
	channelMap *channel.Mapping

	// race is the optional race id copied from the template at
	// construction (M8.3). The spawn pipeline reads this via
	// RaceID() to resolve a *progression.Race and applies racial
	// flags via ApplyRacialFlags. Empty when the template declares
	// no race.
	race string

	// trainerTier / trainerTeach are the primitive trainer payload
	// copied from the template (M8.6 — progression.md §7.3). The
	// session-side TrainerInRoom adapter reconstructs a
	// *progression.TrainerConfig from these when a `practice`
	// verb scans the room. Carrying primitives instead of
	// pulling in progression keeps entities a lower-level
	// package than the progression service it serves.
	trainerTier  int
	trainerTeach []string

	// proficiencies maps ability id -> proficiency value for the mob's
	// passive abilities (M9.5 #3 — abilities-and-effects §6). Seeded
	// once from the template at construction and never mutated
	// thereafter (mobs neither learn nor train), so — like keywords and
	// race — it is read without a lock. Keys are already lowercased by
	// the loader; Proficiency re-normalizes defensively. nil when the
	// template declares no passive proficiencies. Read by the host's
	// passive-proficiency resolver so a mob's extra_attack / defensive
	// passives fire in combat the same way a player's do.
	proficiencies map[string]int

	// weapon / weaponName are the mob's attack dice + display name fed
	// into combat.Stats (combat §4.5). Set at construction from the
	// template's natural weapon (a beast's "fangs"/"1d6"), then
	// optionally overridden at spawn by EquipMobAtSpawn when the mob
	// equips a weapon item (equipped beats innate). A zero DiceExpr
	// means the mob rolls the engine's unarmed default. Mutated only
	// during the spawn pipeline (before the mob is placed/targetable),
	// then read by combat — no lock, like proficiencies/race.
	weapon     combat.DiceExpr
	weaponName string
	// weaponDamageTypes are the equipped weapon's damage type(s)
	// (weapon-identity §2), fed into combat.Stats so a defender's per-type
	// resistance applies (armor-depth §4). nil = untyped (the default for a
	// natural weapon, which declares no types). Set during the spawn
	// pipeline by SetWeapon, read lock-free by Stats.
	weaponDamageTypes []string
	// resistances is the mob's aggregated per-damage-type damage reduction
	// from worn armor (armor-depth §4), summed across equipped armor at
	// spawn (EquipMobAtSpawn → SetResistances). nil = none. Mutated only
	// during the spawn pipeline, then read lock-free by Stats — same
	// discipline as weapon/proficiencies.
	resistances map[string]int
}

// Proficiency reports the mob's proficiency for abilityID (M9.5 #3).
// Returns (value, true) when the mob was seeded with that ability's
// proficiency, else (0, false). Mirrors progression.ProficiencyManager's
// Proficiency accessor so the host composite resolver can route a mob
// id here without the passive resolver knowing players from mobs. The
// map is immutable post-construction, so no lock is taken.
func (m *MobInstance) Proficiency(abilityID string) (int, bool) {
	if m.proficiencies == nil {
		return 0, false
	}
	v, ok := m.proficiencies[strings.ToLower(strings.TrimSpace(abilityID))]
	return v, ok
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

// Description returns the flavor prose snapshotted from the template at
// spawn (alongside name). Empty when the template authored none; the
// `look` handler renders a generic fallback. `consider` ignores this —
// it owns the tactical lens.
func (m *MobInstance) Description() string { return m.desc }

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

// Stats derives the mob's combat stat block from its progression
// StatBlock (combat §4.4-4.5), applying any live effect modifiers via
// Effective(). A value is returned per call so the round loop's
// hit/damage rolls read a consistent snapshot per swing. Mirrors
// connActor.Stats() on the player side.
func (m *MobInstance) Stats() combat.Stats {
	str := m.statBlock.Effective(progression.StatSTR)
	hitMod := m.statBlock.Effective(progression.StatHitMod)
	ac := m.statBlock.Effective(progression.StatAC)
	// Same fallback as connActor.Stats: STRBonus when unmapped (bare test
	// mobs), the mapping's damage_bonus/mitigation when present.
	damageBonus := combat.STRBonus(str)
	mitigation := 0
	if m.channelMap != nil {
		lookup := func(name string) int { return m.statBlock.Effective(progression.StatType(name)) }
		hitMod = m.channelMap.Value(channel.Attack, lookup)
		ac = m.channelMap.Value(channel.Defense, lookup)
		damageBonus = m.channelMap.Value(channel.DamageBonus, lookup)
		mitigation = m.channelMap.Value(channel.Mitigation, lookup)
	}
	s := combat.Stats{
		HitMod:      hitMod,
		AC:          ac,
		STR:         str,
		DamageBonus: damageBonus,
		Mitigation:  mitigation,
	}
	// Weapon dice (combat §4.5): the equipped or natural weapon set at
	// spawn. Zero falls through to the unarmed default via
	// Stats.EffectiveDamage; WeaponName likewise falls back when empty.
	if !m.weapon.IsZero() {
		s.Damage = m.weapon
		s.WeaponName = m.weaponName
		s.WeaponDamageTypes = append([]string(nil), m.weaponDamageTypes...) // copy: combat.Stats is self-contained
	}
	// Per-type resistance from worn armor (armor-depth §4). Copy out so
	// combat.Stats does not alias the instance's cached map (matches the
	// per-round self-contained-snapshot contract on combat.Stats).
	if len(m.resistances) > 0 {
		s.Resistances = make(map[string]int, len(m.resistances))
		for k, v := range m.resistances {
			s.Resistances[k] = v
		}
	}
	return s
}

// SetWeapon installs the mob's attack dice + display name (combat §4.5).
// Called during the spawn pipeline only: buildMobFromTemplate seeds the
// natural weapon, then EquipMobAtSpawn overrides it with an equipped
// weapon. Not safe to call after the mob is targetable in combat (the
// field is read lock-free by Stats on the tick goroutine).
func (m *MobInstance) SetWeapon(dice combat.DiceExpr, name string, damageTypes []string) {
	m.weapon = dice
	m.weaponName = name
	m.weaponDamageTypes = damageTypes
}

// SetResistances installs the mob's aggregated per-damage-type damage
// reduction from worn armor (armor-depth §4). Called once during the spawn
// pipeline (EquipMobAtSpawn) after gear is placed; not safe to call after
// the mob is targetable (read lock-free by Stats on the tick goroutine).
func (m *MobInstance) SetResistances(resistances map[string]int) {
	m.resistances = resistances
}

// EntityID implements progression.EffectTarget: the bare id the effect
// manager keys this mob under.
func (m *MobInstance) EntityID() string { return string(m.id) }

// AddModifiers implements progression.EffectTarget — installs an
// effect's stat modifiers on the mob's block under src (combat-wise:
// a poison lowering AC/STR, a bless raising hit_mod). Reversed by
// RemoveBySource when the effect expires.
func (m *MobInstance) AddModifiers(src srckey.SourceKey, mods []stats.Modifier) {
	m.statBlock.AddModifiers(src, mods)
}

// StatBlock returns the mob's underlying progression.StatBlock for
// callers that need to read or compose against it directly — e.g.
// the M14.3 spawn path running progression.ApplyMobClassGrowth.
// The returned pointer is the live block, not a snapshot; StatBlock
// owns its own mutex so concurrent callers do not need to
// coordinate.
func (m *MobInstance) StatBlock() *progression.StatBlock {
	return m.statBlock
}

// RemoveBySource implements progression.EffectTarget — drops the
// modifier set installed under src; reports whether anything was
// removed.
func (m *MobInstance) RemoveBySource(src srckey.SourceKey) bool {
	return m.statBlock.RemoveBySource(src)
}

// TrainerTier returns the cap-tier value (0/25/50/75/100) the
// mob can raise abilities TO when acting as a `skill_trainer`
// (M8.6 — progression.md §7.3). Zero on non-trainer mobs.
func (m *MobInstance) TrainerTier() int { return m.trainerTier }

// TrainerTeach returns a copy of the ability ids the mob teaches.
// Returns nil for non-trainer mobs. Fresh slice on every call so
// callers cannot alias the backing storage.
func (m *MobInstance) TrainerTeach() []string {
	if len(m.trainerTeach) == 0 {
		return nil
	}
	return append([]string(nil), m.trainerTeach...)
}

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

	// Derive combat-side state from the template's free-form Stats map
	// into a progression StatBlock (so effects can later modify it).
	// Engine defaults fill any missing key so a template that forgot
	// hp_max still spawns a fightable mob. Vitals start at full per spec
	// §2.3 — current HP mirrors the block's effective hp_max at spawn.
	sb := mobStatBlock(tpl.Stats)
	maxHP := sb.Effective(progression.StatHPMax)

	mob := &MobInstance{
		id:           id,
		typ:          tpl.Type,
		name:         tpl.Name,
		desc:         tpl.Description, // snapshot prose alongside name (§2.3).
		tags:         tags,
		keywords:     keywords,
		properties:   props,
		templateID:   tpl.ID,
		vitals:       combat.NewVitals(maxHP),
		statBlock:    sb,
		race:         tpl.Race,
		trainerTier:  tpl.TrainerTier,
		trainerTeach: append([]string(nil), tpl.TrainerTeach...),
		// Copy (not alias) the template's passive proficiencies so a
		// future template mutation can't reach into a live instance.
		// nil-in stays nil-out — Proficiency handles the nil map.
		proficiencies: copyProficiencies(tpl.Proficiencies),
	}
	// M14.1: keep Vitals.Max in lockstep with StatBlock's effective
	// hp_max so an effect that raises CON / hp_max actually changes
	// the mob's max-HP ceiling. Vitals.SetMax also clamps current
	// down when the new max is lower.
	mob.statBlock.OnMaxChange(progression.StatHPMax, func(_, newMax int) {
		mob.vitals.SetMax(newMax)
	})

	// Natural weapon (combat §4.5): a beast with no item still attacks.
	// The damage string was validated at pack load; a parse error on a
	// hand-built template (tests) leaves the mob unarmed rather than
	// panicking. An equipped weapon overrides this at EquipMobAtSpawn.
	if tpl.NaturalWeaponDamage != "" {
		if d, err := combat.ParseDice(tpl.NaturalWeaponDamage); err == nil {
			mob.weapon = d
			mob.weaponName = tpl.NaturalWeaponName
		}
	}
	return mob
}

// copyProficiencies returns a defensive copy of a template's passive
// proficiency map (M9.5 #3). Returns nil for a nil/empty input so a
// mob with no passives carries no map (Proficiency handles nil).
func copyProficiencies(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// mobStatBlock builds a progression.StatBlock seeded from a mob
// template's free-form Stats map, falling back to the combat-side mob
// defaults for any missing combat stat (matching combat.FromTemplateStats'
// old behavior). Template keys are the StatType string values
// ("hit_mod"/"ac"/"str"/"hp_max"), so they map directly. A non-positive
// hp_max is treated as absent so a misconfigured template gets the
// default rather than a zero-HP corpse.
func mobStatBlock(tplStats map[string]int) *progression.StatBlock {
	base := map[progression.StatType]int{
		progression.StatSTR:    combat.DefaultSTR,
		progression.StatAC:     combat.DefaultAC,
		progression.StatHitMod: 0,
		progression.StatHPMax:  combat.DefaultMobMaxHP,
	}
	for k, v := range tplStats {
		base[progression.StatType(k)] = v
	}
	if base[progression.StatHPMax] <= 0 {
		base[progression.StatHPMax] = combat.DefaultMobMaxHP
	}
	return progression.NewWithBase(base)
}

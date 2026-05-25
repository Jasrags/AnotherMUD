package entities

import (
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
	properties map[string]any
	templateID mob.TemplateID
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

// Properties returns the per-instance property bag (the live map,
// not a copy). Spec §2.3 step 6 expects gameplay-mutable per-mob
// state to live here.
func (m *MobInstance) Properties() map[string]any { return m.properties }

// TemplateID returns the source template id (§2.3 step 4 → set on
// the entity's properties; here we additionally surface a typed
// accessor so loot listeners and AI don't have to round-trip through
// the property bag for a value that never changes).
func (m *MobInstance) TemplateID() mob.TemplateID { return m.templateID }

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

	return &MobInstance{
		id:         id,
		typ:        tpl.Type,
		name:       tpl.Name,
		tags:       tags,
		keywords:   keywords,
		properties: props,
		templateID: tpl.ID,
	}
}

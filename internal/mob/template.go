// Package mob owns content-side mob data: the template type and the
// registry the pack loader populates at boot. Instances and the AI
// tick live elsewhere; M6.1 lands templates only.
//
// Spec: mobs-ai-spawning §2.
package mob

import (
	"errors"
	"fmt"
	"sync"
)

// TemplateID is a namespace-qualified template identifier (e.g.
// "tapestry-core:village-guard"). Spec §2.1 + scripting-and-packs §5.2.
type TemplateID string

// Template is the recipe a mob instance is built from. Required fields
// come straight from spec §2.2; optional fields are the ones M6.1 is
// scoped to support — the rest (class/race/level, loot tables, patrol
// routes, idle/battle command sets, scripts, disposition rules, shop
// config) land with later sub-milestones as their consumers arrive.
//
// Equipment is a flat list of item template ids the instantiation
// step (§2.3, M6.2) will equip onto the mob at spawn time. The pack
// loader does NOT cross-validate that those ids resolve — that
// happens at spawn, not load — so a typo here surfaces when the mob
// first spawns rather than at boot. (Aligned with spec §3.1's
// "fail silently and return" semantics for spawn-time misses.)
//
// Properties is intentionally untyped: pack content may carry arbitrary
// scalars/maps that downstream behaviors interpret. The decoder
// normalizes nested untyped maps the same way item templates do.
//
// Stats is a flat map of stat-name to base value. Attribute names are
// not validated at the template layer; the progression spec (M8) owns
// the canonical set, and validating here would prematurely freeze the
// schema before that lands.
type Template struct {
	ID   TemplateID
	Name string
	Type string // default "npc" applied at decode if unset
	// Disposition is the legacy integer base disposition. Retained
	// in the schema for content already authored against it; the
	// runtime no longer reads it. The structured fields below own
	// reaction policy (spec §5.1).
	Disposition int

	// BaseDisposition is the spec §5.1 "static base disposition":
	// a fixed reaction the mob uses regardless of player state.
	// When set to ReactionHostile it short-circuits any
	// DispositionRules and always emits hostile (§5.3 step 3).
	// Empty means "no static reaction; consult DispositionRules".
	BaseDisposition Reaction

	// DispositionRules is the structured policy: default + ordered
	// conditional rules. nil means "no rules" — combined with an
	// empty BaseDisposition the mob never dispatches a reaction.
	DispositionRules *Definition

	// Description is the optional flavor prose shown by `look <mob>`
	// (the appearance lens — ui-rendering-help). Empty falls back to a
	// generic line in the look handler; never required. `consider` reads
	// none of this — it owns the tactical lens (HP/threat).
	Description string

	// Faction is the optional faction-membership id (faction.md §5.2): the
	// single faction this mob belongs to. When a player lands the killing
	// blow, the on-kill hook shifts the killer's standing with this faction by
	// the configured on-kill delta. Namespace-qualified at load. Empty = the
	// mob belongs to no faction (no on-kill standing change).
	Faction string

	// XPValue is the experience a lethal kill of this mob awards (grouping.md
	// §4), split among the killer's same-room party (solo = full). 0 = none.
	XPValue int

	Behavior   string
	Tags       []string
	Keywords   []string
	Properties map[string]any
	Stats      map[string]int
	Equipment  []string // item template ids to equip at spawn (§3.3)

	// NaturalWeaponName / NaturalWeaponDamage describe an innate attack
	// (combat §4.5) for a mob that fights with no item — a wolf's
	// "fangs" rolling "1d6". The damage is a raw NdM±K string validated
	// at pack load and parsed to a dice expression at spawn; both empty
	// means the mob uses the engine's unarmed default. An equipped weapon
	// (§3.3) overrides the natural weapon. Carried as plain strings so
	// the mob package stays independent of the combat package.
	NaturalWeaponName   string
	NaturalWeaponDamage string

	// LootTable is the optional loot-table id (mobs-ai-spawning §6.3).
	// When set and the id resolves in the loot registry, the spawn
	// pipeline rolls the table once and files the resulting item
	// instances into the mob's contents at spawn (§3.1 step 8) — the
	// mob carries its loot from the moment it appears. An unknown id
	// fails silently at spawn (consistent with race/class). Carried as
	// a plain id string so mob/entities stay independent of the loot
	// package.
	LootTable string

	// Proficiencies is the optional map of ability id -> proficiency
	// value the mob holds for PASSIVE abilities (abilities-and-effects
	// §6). Unlike players, mobs neither learn nor train: these are
	// fixed content read once at spawn into the MobInstance's immutable
	// proficiency map. They feed the same combat passive hooks
	// (extra_attack, defensive) a player's proficiency does — e.g. a
	// guard with `second-attack: 70` earns extra swings. Ability keys
	// are lowercased + trimmed by the loader; values are content's
	// responsibility (the passive resolver treats <=0 as unlearned).
	Proficiencies map[string]int

	// Race is the optional race id (progression.md §3.1). When set
	// and the id resolves in the race registry at spawn time, the
	// mob's RacialFlags are merged into its tag set (§3.1) and
	// StartingAlignment seeds an alignment property. Unknown ids
	// fail silently at spawn — matching the spec §3.1 mob-spawn
	// "fail-silent on missing template" convention.
	Race string

	// Class is the optional class id (progression.md §4.1). When
	// set together with a positive Level, spawn applies
	// `averageDice(class.StatGrowth[stat]) × level` to each
	// stat-growth entry on the mob's StatBlock — the M14.3
	// implementation of mobs-ai-spawning §3.2. Unknown class ids
	// fail silently at spawn (consistent with race).
	Class string

	// Level is the mob's effective level for class-growth
	// derivation. Zero (the default) disables the growth path
	// regardless of Class — a templated mob with `class: fighter`
	// but no level still spawns at base stats.
	Level int

	// Size is the mob's size category (size-and-wielding §3.2) from the
	// engine size vocabulary (tiny … huge). Empty ⇒ the baseline size. Drives
	// the size-relative wield mode of any weapon the mob wields. Validated at
	// pack load; lowercased.
	Size string

	// TrainerTier is the cap-tier value (0/25/50/75/100) the mob
	// can raise abilities TO when running as a `skill_trainer`
	// (progression.md §7.3). Zero on non-trainer mobs. The pack
	// loader pairs this with the `skill_trainer` tag — either
	// both are present or neither — so the runtime can scan
	// rooms by tag and trust the tier+list are set.
	//
	// Carried as primitives instead of a *progression.TrainerConfig
	// to keep mob/entities independent of progression (avoids the
	// entities→mob→progression→entities import cycle).
	TrainerTier int
	// TrainerTeach is the list of ability ids the mob teaches.
	// Lowercased + de-trimmed by the loader.
	TrainerTeach []string

	// Mount is the optional rideable-mount descriptor (mounts.md §2). A
	// non-nil Mount marks this mob as a mount: a rideable, owned creature
	// that becomes the metered mover while ridden (§5) and gates danger
	// entry by temperament (§7.2). Nil ⇒ an ordinary mob. Decoded and
	// validated at pack load (the temperament string is checked against the
	// mount vocabulary there, like Size against the size vocabulary); carried
	// as a plain struct so mob stays independent of the mount package.
	Mount *MountSpec

	// Hireling is the optional hireable-companion descriptor (hireable-mobs.md
	// §2). A non-nil Hireling marks this mob as **hireable**: a character may
	// `hire` it as an owned companion that follows and fights for them. Nil ⇒ an
	// ordinary mob. Carries the hire cost (and upkeep for a later slice).
	Hireling *HirelingSpec

	// Recruiter is the optional recruiter descriptor (hireable-mobs.md §3.1). A
	// non-nil Recruiter marks this mob as a hiring access point (a mercenary-post
	// NPC): a character may `hire` one of the hirelings it Offers while in its
	// room. Nil ⇒ not a recruiter.
	Recruiter *RecruiterSpec
}

// HirelingSpec is the hireable-companion descriptor copied from a mob template's
// `hireling:` block (hireable-mobs.md §2). Its presence is what makes a mob
// hireable; the values are the gold sinks (§3.1 hire cost, §7 upkeep).
type HirelingSpec struct {
	// HireCost is the up-front gold charged to hire this companion
	// (hireable-mobs.md §3.1). Required non-negative at load.
	HireCost int
	// Upkeep is the recurring gold charged to keep this hireling
	// (hireable-mobs.md §7). Consumed by the upkeep tick in a later slice; zero ⇒
	// no upkeep. Non-negative at load.
	Upkeep int
}

// RecruiterSpec is the recruiter descriptor copied from a mob template's
// `recruiter:` block (hireable-mobs.md §3.1). Its presence makes a mob a hiring
// access point. Offers lists the hireling template ids it will hire out — the
// catalog a player sees with a bare `hire` and resolves a `hire <name>` against.
type RecruiterSpec struct {
	// Offers is the hireling templates this recruiter hires out. Required
	// non-empty at load; each entry resolves to a hireable template (one carrying
	// a `hireling:` block) at hire time. An entry may be the full template id or a
	// name/keyword of the offered hireling.
	Offers []string
}

// MountSpec is the rideable-mount descriptor copied from a mob template's
// `mount:` block (mounts.md §2.1). Its presence is what makes a mob a mount.
// Temperament is a load-validated plain string (resolved against the mount
// vocabulary by the consumer) so the mob package stays free of a mount-package
// import — the same discipline Size uses for the size vocabulary.
type MountSpec struct {
	// Temperament governs danger entry (mounts.md §7.2): "war" / "steady" /
	// "skittish". Empty resolves to the cautious default at the consumer.
	Temperament string
	// TravelMax is the mount's travel-resource ceiling (mounts.md §5.1) — the
	// renewable movement budget a mounted step spends instead of the rider's.
	// A larger budget out-travels legs; required positive at load.
	TravelMax int
	// TravelRegen is the per-regen-tick travel-pool restore amount (§5.4).
	// Zero ⇒ the engine default applied by the regen tick.
	TravelRegen int
	// Impassable lists terrain ids a mount of this type cannot enter at all
	// (§5.3 — a cramped tunnel, a building interior). A mounted step into such
	// a destination is refused; the rider dismounts and proceeds on foot.
	// Optional; nil ⇒ the mount is bound only by the room-level
	// mount-impassable flag. Consumed by mounted travel (§5.3).
	Impassable []string
}

// Errors callers may distinguish at the boundary.
var (
	ErrTemplateNotFound = errors.New("mob template not found")
	ErrDuplicateID      = errors.New("duplicate mob template id")
)

// Templates is the boot-time registry of mob templates. Safe for
// concurrent reads; mutations (Add, TryAdd) MUST happen at boot before
// serving — same invariant as world.World and item.Templates.
type Templates struct {
	mu  sync.RWMutex
	all map[TemplateID]*Template
}

// NewTemplates returns an empty registry.
func NewTemplates() *Templates {
	return &Templates{all: make(map[TemplateID]*Template)}
}

// Add registers t, replacing any existing template with the same id
// (spec §2.1: later registrations replace earlier ones).
func (r *Templates) Add(t *Template) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.all[t.ID] = t
}

// TryAdd registers t and returns ErrDuplicateID if a template with
// that id is already present. Used by the pack loader to catch
// cross-pack id collisions before they silently overwrite.
func (r *Templates) TryAdd(t *Template) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.all[t.ID]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateID, t.ID)
	}
	r.all[t.ID] = t
	return nil
}

// Get returns the template with id and ErrTemplateNotFound if absent.
func (r *Templates) Get(id TemplateID) (*Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.all[id]
	if !ok {
		return nil, fmt.Errorf("mob.Templates.Get(%q): %w", id, ErrTemplateNotFound)
	}
	return t, nil
}

// Has reports whether id is registered.
func (r *Templates) Has(id TemplateID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.all[id]
	return ok
}

// Count returns the number of registered templates.
func (r *Templates) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.all)
}

// All returns a snapshot of every registered template. Order is
// unspecified; callers that need determinism must sort.
func (r *Templates) All() []*Template {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Template, 0, len(r.all))
	for _, t := range r.all {
		out = append(out, t)
	}
	return out
}

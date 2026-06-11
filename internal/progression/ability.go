package progression

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// AbilityType is the active/passive classification (spec
// abilities-and-effects §2.2). Active abilities are queueable and
// resolve on a combat round pulse (§4); passive abilities are
// evaluated at hook points (§6).
type AbilityType string

const (
	// AbilityActive is a queueable ability resolved in the ability
	// resolution phase (spec §4).
	AbilityActive AbilityType = "active"
	// AbilityPassive is hook-driven and never queued (spec §6).
	AbilityPassive AbilityType = "passive"
)

// AbilityCategory is the skill/spell classification (spec §2.2).
// Drives default offensive classification and the resource pool
// (§4.6, §4.7): skills draw movement, spells draw mana.
type AbilityCategory string

const (
	// AbilitySkill draws from the entity's movement pool (§4.7).
	AbilitySkill AbilityCategory = "skill"
	// AbilitySpell draws from the entity's mana pool (§4.7).
	AbilitySpell AbilityCategory = "spell"
)

// ParseAbilityType normalizes a raw string into AbilityType.
// Case-insensitive; unknown values yield (zero, false).
func ParseAbilityType(s string) (AbilityType, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "active":
		return AbilityActive, true
	case "passive":
		return AbilityPassive, true
	default:
		return "", false
	}
}

// ParseAbilityCategory normalizes a raw string into AbilityCategory.
// Case-insensitive; unknown values yield (zero, false).
func ParseAbilityCategory(s string) (AbilityCategory, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "skill":
		return AbilitySkill, true
	case "spell":
		return AbilitySpell, true
	default:
		return "", false
	}
}

// Ability is a content-defined ability definition (spec
// abilities-and-effects §2). M9.1 wired identity + classification +
// gain. M9.3 grows the surface used by validation (spec §4.3):
// cost, pulse-delay cooldown, initiate-only flag, target-type list,
// equipment-slot requirement, alignment range, and an optional
// effect template (consumed by §5.2 single-instance check + §5.1
// build-on-hit). Resolution-only fields (variance, max-chance,
// handler token, metadata) land in M9.4 as their consumers arrive.
//
// Value-typed for registry storage; the registry hands callers a
// pointer to its own copy and callers MUST NOT mutate it.
type Ability struct {
	// ID is the stable case-insensitive id (spec §2.1).
	ID string

	// DisplayName is the player-facing name (spec §2.2 required).
	DisplayName string

	// Type is active/passive (spec §2.2 required).
	Type AbilityType

	// Category is skill/spell (spec §2.2 required).
	Category AbilityCategory

	// DefaultCap is the cap value set when an entity learns this
	// ability and has no cap entry yet (spec §3.2). When zero, the
	// proficiency manager falls back to its configured default
	// (ProficiencyConfig.DefaultLearnCap).
	DefaultCap int

	// GainBaseChance is the base probability (in 1..100 units) of
	// a proficiency gain on use (spec §3.5 step 1). Zero disables
	// gain rolls for this ability.
	GainBaseChance int

	// GainFailureMultiplier scales GainBaseChance on a missed
	// invocation (spec §3.5 step 4). 1.0 means miss == hit;
	// values < 1.0 reduce miss-gain. Zero falls back to the
	// proficiency-manager default.
	GainFailureMultiplier float64

	// GainStat names a stat whose effective value contributes to
	// gain probability (spec §3.5 step 3). Empty means no stat
	// boost (gain probability uses base × proficiency-curve only).
	GainStat StatType

	// GainStatScale scales the stat contribution to gain
	// probability. Zero with a non-empty GainStat falls back to
	// the proficiency-manager default.
	GainStatScale float64

	// Cost is the unmodified resource cost (spec §2.2 / §4.7).
	// Race-adjusted at validation + deduction time. Zero means
	// no resource check (the §4.3 step 9 check is skipped).
	Cost int

	// PulseDelay is the per-entity cooldown in pulses (spec §2.2
	// / §4.3 step 8 / §4.5 step 3). Zero means no cooldown.
	PulseDelay int

	// InitiateOnly marks combat-opening moves that fizzle when
	// the source is already in combat (spec §2.2 / §4.3 step 5).
	InitiateOnly bool

	// TargetTypes is the list of permitted target classifications
	// ("enemy", "self", "ally") (spec §2.2). Empty = no
	// per-ability restriction; the engine's offensive classifier
	// (§4.6) still gates auto-target-current-enemy fallback.
	// Stored normalized lowercase.
	TargetTypes []string

	// EquipmentSlot is the optional required slot id (spec §2.2
	// / §4.3 step 4). Empty disables the slot check. Slot names
	// are global (mirrors slot registry).
	EquipmentSlot string

	// EquipmentTag is the optional required tag on the item in
	// EquipmentSlot (spec §2.2 / §4.3 step 4). Only consulted
	// when EquipmentSlot is non-empty. Empty means any item in
	// the slot satisfies the check.
	EquipmentTag string

	// HasAlignmentRange selects whether AlignmentMin / Max gate
	// usage (spec §2.2 / §4.3 step 2). Necessary because zero
	// is a valid alignment (neutral); we can't piggyback on
	// "both zero == unset".
	HasAlignmentRange bool

	// AlignmentMin / AlignmentMax bound the entity's alignment
	// inclusively when HasAlignmentRange is true (spec §2.2 /
	// §4.3 step 2). When Min > Max the range is empty and every
	// invocation fizzles `alignment_restricted`.
	AlignmentMin int
	AlignmentMax int

	// Effect is the optional effect template applied to the
	// target on hit (spec §2.2 / §5.1). When present, the
	// effect-present validation check (§4.3 step 7) uses Effect.ID
	// to decide whether the source already carries the effect.
	// nil disables both build-on-hit and the effect-present
	// check.
	Effect *EffectTemplate

	// ApplySave is the optional entry save the target rolls to RESIST the
	// ability's effect (conditions §4 — a save-gated condition like trip or
	// bash). A made save means the effect is not applied. nil ⇒ the effect
	// always lands on hit. Only meaningful with a non-self Effect.
	ApplySave *ConditionSave

	// Variance is the hit-chance variance band in percentage
	// points (spec §4.5 step 4). Zero means the invocation always
	// hits (no roll). Otherwise the engine computes
	// `chance = clamp(proficiency × variance / 100, 1,
	// MaxHitChance|100)` and rolls 1..100; hit when roll ≤ chance.
	// Values are clamped to [0, 100] at registration.
	Variance int

	// MaxHitChance optionally caps the rolled hit chance at the
	// top end so even a fully-proficient invocation can still
	// miss (spec §4.5 / §8 "engine configuration"). Zero ⇒ no
	// ability-specific cap; the resolver falls back to its
	// configured ceiling (default 100).
	MaxHitChance int

	// HandlerToken is the §4.5 step-8 dispatch key carried on the
	// "ability used" event so a subscriber knows which side effect
	// to apply (M9.6b: "damage" rolls DamageDice onto the target's
	// HP; "heal" rolls HealDice; empty ⇒ no engine side effect, e.g.
	// an effect-only buff like bless whose payload the resolver
	// already applied in §4.5 step 7). When scripting lands the
	// token becomes the script dispatch key. Stored lowercase.
	HandlerToken string

	// DamageDice is the NdM±K expression the "damage" handler rolls
	// against the target's HP (spec §4.6 "damage dice metadata").
	// Non-empty also makes a spell-with-no-effect offensive (§4.6).
	// Parsed by the host handler, not the registry, so progression
	// stays free of the combat dice type. Empty ⇒ no damage.
	DamageDice string

	// HealDice is the NdM±K expression the "heal" handler rolls to
	// restore the target's HP. Heal abilities are NOT offensive even
	// when they could target an enemy (§4.6). Empty ⇒ no heal.
	HealDice string

	// Hook is the passive-discovery key (spec §6.3): subsystems
	// iterate "all passive abilities tagged with hook H" to find the
	// passives that apply to an event. Canonical hooks today are
	// "extra_attack" (combat §4.2 swing count) and "defensive"
	// (combat §4.3 step 2 evade), but the set is content-defined.
	// Only meaningful on passive abilities. Stored lowercase.
	Hook string

	// MaxBonus is the ceiling for the §6.2 scaling-bonus building
	// block: the bonus a passive contributes is MaxBonus ×
	// proficiency / 100. Zero ⇒ no scaling contribution. Independent
	// of the §6.1 binary check (which uses Variance + MaxHitChance).
	MaxBonus int

	// Pack records the pack that registered this ability.
	// Diagnostic only — mirrors Race.Pack / Class.Pack.
	Pack string

	// Priority drives override semantics on duplicate id: higher
	// wins; equal priority is a no-op (existing entry retained).
	// Mirrors race/class registries (spec §2.1 "higher-priority
	// registration wins").
	Priority int
}

// AbilityRegistry holds ability definitions keyed by case-insensitive
// id. Mirrors ClassRegistry / RaceRegistry shape.
type AbilityRegistry struct {
	mu    sync.RWMutex
	items map[string]*Ability
}

// NewAbilityRegistry returns an empty registry.
func NewAbilityRegistry() *AbilityRegistry {
	return &AbilityRegistry{items: make(map[string]*Ability)}
}

// Register installs a. Returns an error on malformed input (nil,
// missing id, unknown type, unknown category). Id is lowercased on
// registration. Higher priority replaces; equal priority no-ops
// (spec §2.1).
func (r *AbilityRegistry) Register(a *Ability) error {
	if a == nil {
		return fmt.Errorf("progression: nil Ability")
	}
	id := strings.ToLower(strings.TrimSpace(a.ID))
	if id == "" {
		return fmt.Errorf("progression: ability missing id")
	}
	if a.Type != AbilityActive && a.Type != AbilityPassive {
		return fmt.Errorf("progression: ability %q has invalid type %q", id, a.Type)
	}
	if a.Category != AbilitySkill && a.Category != AbilitySpell {
		return fmt.Errorf("progression: ability %q has invalid category %q", id, a.Category)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.items[id]; ok && a.Priority <= existing.Priority {
		return nil
	}
	clone := *a
	clone.ID = id
	if clone.DefaultCap < 0 {
		clone.DefaultCap = 0
	}
	if clone.DefaultCap > 100 {
		clone.DefaultCap = 100
	}
	clone.GainStat = StatType(strings.ToLower(strings.TrimSpace(string(a.GainStat))))
	// A negative failure multiplier is meaningless (gain can't be
	// negative); normalize to 0 so rollGain's `<= 0 ⇒ default 1.0`
	// guard treats it as "unset". A value > 1.0 (miss gains faster
	// than hit) is left as-authored — unusual but a deliberate
	// content choice the spec doesn't forbid.
	if clone.GainFailureMultiplier < 0 {
		clone.GainFailureMultiplier = 0
	}
	if clone.Cost < 0 {
		clone.Cost = 0
	}
	if clone.PulseDelay < 0 {
		clone.PulseDelay = 0
	}
	if clone.Variance < 0 {
		clone.Variance = 0
	}
	if clone.Variance > 100 {
		clone.Variance = 100
	}
	if clone.MaxHitChance < 0 {
		clone.MaxHitChance = 0
	}
	if clone.MaxHitChance > 100 {
		clone.MaxHitChance = 100
	}
	clone.EquipmentSlot = strings.ToLower(strings.TrimSpace(a.EquipmentSlot))
	clone.EquipmentTag = strings.ToLower(strings.TrimSpace(a.EquipmentTag))
	clone.HandlerToken = strings.ToLower(strings.TrimSpace(a.HandlerToken))
	clone.DamageDice = strings.TrimSpace(a.DamageDice)
	clone.HealDice = strings.TrimSpace(a.HealDice)
	clone.Hook = strings.ToLower(strings.TrimSpace(a.Hook))
	if clone.MaxBonus < 0 {
		clone.MaxBonus = 0
	}
	if len(a.TargetTypes) > 0 {
		tt := make([]string, 0, len(a.TargetTypes))
		seen := make(map[string]struct{}, len(a.TargetTypes))
		for _, t := range a.TargetTypes {
			n := strings.ToLower(strings.TrimSpace(t))
			if n == "" {
				continue
			}
			if _, dup := seen[n]; dup {
				continue
			}
			seen[n] = struct{}{}
			tt = append(tt, n)
		}
		if len(tt) > 0 {
			clone.TargetTypes = tt
		} else {
			clone.TargetTypes = nil
		}
	}
	if a.Effect != nil {
		eff := *a.Effect
		eff.ID = strings.ToLower(strings.TrimSpace(eff.ID))
		clone.Effect = &eff
	}
	r.items[id] = &clone
	return nil
}

// Get returns the registered Ability for id. Case-insensitive
// lookup; (nil, false) on miss. The returned pointer is
// registry-owned — callers MUST NOT mutate it.
func (r *AbilityRegistry) Get(id string) (*Ability, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.items[key]
	return a, ok
}

// Has reports whether an ability is registered under id.
func (r *AbilityRegistry) Has(id string) bool {
	_, ok := r.Get(id)
	return ok
}

// All returns every registered ability in id-sorted order.
func (r *AbilityRegistry) All() []*Ability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Ability, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.items[id])
	}
	return out
}

// ByType returns every registered ability whose Type matches t, in
// id-sorted order. Used by passive-hook iteration (spec §6.3) and
// administrative listings.
func (r *AbilityRegistry) ByType(t AbilityType) []*Ability {
	all := r.All()
	out := make([]*Ability, 0, len(all))
	for _, a := range all {
		if a.Type == t {
			out = append(out, a)
		}
	}
	return out
}

// ByHook returns every PASSIVE ability whose Hook matches hook, in
// id-sorted order (spec §6.3 hook-based discovery). The match is by
// metadata hook key, never by hardcoded ability id, so content can
// add new passives to an existing hook without engine changes. An
// empty hook argument returns nil (a passive with no hook is not
// discoverable). Active abilities are never returned — they resolve
// through the queue, not hooks.
func (r *AbilityRegistry) ByHook(hook string) []*Ability {
	h := strings.ToLower(strings.TrimSpace(hook))
	if h == "" {
		return nil
	}
	all := r.All()
	out := make([]*Ability, 0)
	for _, a := range all {
		if a.Type == AbilityPassive && a.Hook == h {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

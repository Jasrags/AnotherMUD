package progression

import (
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/srckey"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// EffectTemplate is the immutable description on an ability of the
// effect it produces (spec abilities-and-effects §2.2, §5.1). M9.2
// keeps this constructed programmatically — the ability YAML schema
// grows the `effect:` block in M9.4 when resolution starts wiring
// templates through ability invocations.
//
// Duration is in pulses. A negative value means **permanent** —
// never decremented by Tick, removed only by explicit
// RemoveByID/RemoveByFlag or by another system's dispel/cleanse.
//
// Modifiers and Flags are normalized at NewEffect time; callers do
// not need to lowercase before passing.
type EffectTemplate struct {
	// ID is the stable case-insensitive effect id. Drives the
	// single-instance rule (§5.2) — a target may carry at most
	// one active effect per id.
	ID string

	// Duration is the remaining-pulse counter at apply time. >0
	// for time-bounded effects; <0 for permanent.
	Duration int

	// Modifiers are the stat deltas applied to the target's stat
	// block under a derived source key (EffectSourceKey). Empty
	// list = the effect carries no numeric impact (flag-only).
	Modifiers []stats.Modifier

	// Flags are the string tags installed on the target via the
	// EffectManager's per-entity flag set (M9.2 keeps flags
	// owned by the manager rather than mutating an entity-side
	// Tags surface — see m9-2 design note in ROADMAP).
	Flags []string

	// Refreshable opts this effect into duration-refresh on re-apply
	// (crafting-and-cooking §6 well-fed). When false (the default), a
	// re-apply while the effect is already active is ignored (§5.2
	// no-stack). When true, a re-apply resets the live effect's remaining
	// duration to Duration instead of being dropped — so re-eating a
	// well-fed meal extends the buff rather than wasting the food.
	Refreshable bool

	// RecurringSave, when non-nil, gives the target a saving throw on every
	// effect tick to shake the condition off early (conditions §4): a made
	// save removes the effect before its duration would expire. nil ⇒ the
	// effect runs its full duration. The save is rolled through the
	// EffectManager's injected SaveResolver; with no resolver wired the
	// effect always runs its full duration (the safe default). The
	// *entry* save (resist-on-apply, conditions §4) is the applier's job —
	// the ability rolls it before calling Apply — not a template field.
	RecurringSave *ConditionSave
}

// ConditionSave declares a saving throw a condition is checked against
// (conditions §4): a save axis (Fortitude / Reflex / Will) and a difficulty
// class. Used for the per-tick shake-off save carried on an EffectTemplate;
// the entry save is supplied by the applier, not stored here.
type ConditionSave struct {
	Axis SaveType
	DC   int
}

// EffectSourceKey returns the srckey.SourceKey used when an
// effect's stat modifiers are applied to a target's stat block.
// Derives from the effect id so removal can target the exact
// modifier set without tracking the runtime Effect instance's
// memory identity.
//
// Mirrors EquipmentSourceKey shape; the "effect:" prefix segregates
// these from equipment keys so a typoed item id can't collide with
// a real effect by accident.
func EffectSourceKey(effectID string) srckey.SourceKey {
	return srckey.SourceKey(effectSourcePrefix + strings.ToLower(strings.TrimSpace(effectID)))
}

// effectSourcePrefix segregates effect-installed stat modifiers from
// equipment ("equipment:") and any other source. Exported behavior is
// via IsEffectSource so callers don't hardcode the literal.
const effectSourcePrefix = "effect:"

// IsEffectSource reports whether src was produced by EffectSourceKey.
// The persistence layer uses it to keep effect-driven stat modifiers
// out of the saved stat block: active effects are ephemeral (spec
// abilities-and-effects §5.5 — the effect LIST is not persisted), so
// their stat mods must be equally ephemeral, otherwise a buff active
// at logout round-trips into a permanent bonus.
func IsEffectSource(src srckey.SourceKey) bool {
	return strings.HasPrefix(string(src), effectSourcePrefix)
}

// Effect is the runtime instance of an EffectTemplate applied to a
// target (spec §5.1). The manager owns the lifetime; callers query
// via EffectManager.Effects / Has and mutate via Remove* / Tick.
//
// EntityID identifies the target (the entity the effect is
// applied to). SourceEntityID identifies the caster — empty when
// the effect was applied without an explicit source (admin grant,
// world hook). SourceAbilityID names the ability that produced
// the effect, empty when not driven by an ability invocation.
//
// Remaining is the live pulse counter. <0 == permanent (Tick
// skips). 0 reached during Tick marks the effect for expiration
// in the current tick's batch.
//
// The struct is value-typed for snapshot returns; manager-owned
// pointers live in the active-list. Callers receive deep copies
// from snapshot accessors and may freely mutate them.
type Effect struct {
	ID              string
	EntityID        string
	SourceEntityID  string
	SourceAbilityID string
	Remaining       int
	Modifiers       []stats.Modifier
	Flags           []string
	// RecurringSave carries the template's per-tick shake-off save onto the
	// runtime instance so Tick can roll it (conditions §4). nil ⇒ no
	// shake-off; the effect runs its full duration. Immutable content — the
	// pointer is shared with the template, never mutated.
	RecurringSave *ConditionSave
}

// IsPermanent reports whether the effect's remaining counter
// disables Tick decrement (spec §5.1 "negative duration means
// permanent"). Wraps a magic comparison so call sites read
// declaratively.
func (e Effect) IsPermanent() bool { return e.Remaining < 0 }

// HasFlag reports whether flag is in the effect's flag list,
// case-insensitive. Used by RemoveByFlag's iteration and by
// snapshot consumers (M9.5 passive matchers).
func (e Effect) HasFlag(flag string) bool {
	target := strings.ToLower(strings.TrimSpace(flag))
	if target == "" {
		return false
	}
	for _, f := range e.Flags {
		if f == target {
			return true
		}
	}
	return false
}

// newEffectFromTemplate constructs a runtime Effect from a
// template plus per-invocation identity. Defensive copies of the
// modifier and flag slices isolate the runtime instance from
// later template mutation (templates are content-defined and
// nominally immutable, but a content author who reuses a slice
// across templates would otherwise see surprising aliasing). All
// string fields are normalized (lowercased, trimmed) at this
// boundary so manager internals can rely on canonical form.
func newEffectFromTemplate(tpl EffectTemplate, entityID, sourceEntityID, sourceAbilityID string) *Effect {
	out := &Effect{
		ID:              strings.ToLower(strings.TrimSpace(tpl.ID)),
		EntityID:        strings.ToLower(strings.TrimSpace(entityID)),
		SourceEntityID:  strings.ToLower(strings.TrimSpace(sourceEntityID)),
		SourceAbilityID: strings.ToLower(strings.TrimSpace(sourceAbilityID)),
		Remaining:       tpl.Duration,
		RecurringSave:   tpl.RecurringSave,
	}
	if len(tpl.Modifiers) > 0 {
		out.Modifiers = make([]stats.Modifier, len(tpl.Modifiers))
		for i, m := range tpl.Modifiers {
			out.Modifiers[i] = stats.Modifier{
				Stat:  strings.ToLower(strings.TrimSpace(m.Stat)),
				Value: m.Value,
			}
		}
	}
	if len(tpl.Flags) > 0 {
		out.Flags = make([]string, 0, len(tpl.Flags))
		for _, f := range tpl.Flags {
			n := strings.ToLower(strings.TrimSpace(f))
			if n == "" {
				continue
			}
			out.Flags = append(out.Flags, n)
		}
	}
	return out
}

package combat

import "github.com/Jasrags/AnotherMUD/internal/world"

// Saving-throw resolution — EPIC sub-epic S6, spec docs/specs/saves.md §3.
// The check is purely numeric (a d20 roll against a DC), so it lives in the
// combat layer next to the to-hit roll and reuses the same Roller seam. The
// save *axis* (fortitude / reflex / will) is a progression concept; combat
// cannot import progression (progression imports combat — class.go), so the
// axis travels as a plain string on the event, exactly how combat already
// decouples stat keys ("hp_max", …) from progression.StatType. The host
// maps progression.SaveType → string at the call site.

// Save axis names and cause labels carried as plain strings on
// SaveResolved (combat cannot import progression — see the package note).
// The axis values MUST match progression's SaveType values so display and
// GMCP read consistently; there are only three fixed axes, so the small
// duplication is preferable to a cross-package coupling of progression's
// const block to this layer.
const (
	SaveAxisFortitude = "fortitude"
	SaveAxisReflex    = "reflex"
	SaveAxisWill      = "will"

	// SaveCauseMassiveDamage labels the saves §4 massive-damage save.
	SaveCauseMassiveDamage = "massive_damage"
)

// DefaultMassiveDamageThreshold / DefaultMassiveDamageDC are the engine
// defaults for the saves §4 massive-damage Fortitude save. The threshold is
// set high enough that ordinary low-level swing damage never reaches it — the
// rule stays inert until the WoT damage curve grows or a pack lowers it
// (env ANOTHERMUD_MASSIVE_DAMAGE_THRESHOLD / _DC). The DC mirrors the WoT
// source's fixed massive-damage save DC.
const (
	DefaultMassiveDamageThreshold = 50
	DefaultMassiveDamageDC        = 15
)

// SaveOutcome is the result of one resolved saving throw (saves §3). It
// carries the full roll detail — not just the success boolean — so callers
// can render the math and a future margin-sensitive consumer (evasion) can
// inspect Total vs DC.
type SaveOutcome struct {
	// Roll is the raw d20 face (1..20).
	Roll int
	// Bonus is the creature's save bonus for the axis being checked.
	Bonus int
	// Total is Roll + Bonus.
	Total int
	// DC is the difficulty class the total was checked against.
	DC int
	// Success reports whether the save was made.
	Success bool
	// Natural1 / Natural20 flag the auto-fail / auto-succeed edges so a
	// renderer can dramatize them (mirrors the to-hit fumble/auto rules,
	// combat §4.4).
	Natural1  bool
	Natural20 bool
}

// ResolveSave rolls one saving throw: d20 + bonus vs dc (saves §3). A
// natural 1 always fails and a natural 20 always succeeds regardless of
// bonus or DC — the same edge semantics the to-hit roll uses (combat §4.4)
// — otherwise success is `Total >= DC`. Pure over the injected Roller, so
// it is deterministic under a seeded roller and carries no global state.
func ResolveSave(r Roller, bonus, dc int) SaveOutcome {
	roll := r.IntN(20) + 1
	out := SaveOutcome{Roll: roll, Bonus: bonus, Total: roll + bonus, DC: dc}
	switch roll {
	case 1:
		out.Natural1 = true
		out.Success = false
	case 20:
		out.Natural20 = true
		out.Success = true
	default:
		out.Success = out.Total >= dc
	}
	return out
}

// SaveResolved is dispatched whenever a saving throw is resolved (saves
// §3). It is the seam future systems hook — a weave or condition that
// "allows a save" emits one and reacts to Success; the combat log and a
// GMCP feed consume it for display. The event is INFORMATIONAL this slice:
// it reports a resolved save, it does not itself veto the triggering action
// (the consumer owns the consequence). Reserved-ahead-of-broad-use the same
// way Evade was, so the EventSink contract stabilizes before S2/S5 wiring.
//
// SaveType is the axis name as a plain string (e.g. "fortitude"); see the
// package note above for why combat does not import progression.SaveType.
// Cause names what forced the save ("massive_damage") for log/quest hooks.
type SaveResolved struct {
	CreatureID CombatantID
	// CreatureName is a convenience snapshot for log/GMCP consumers. The
	// production sink re-derives the live display name through the session
	// manager (as OnVitalDepleted does) rather than trusting this, so it
	// may be empty when emitted from a context without a name to hand.
	CreatureName string
	SaveType     string
	Cause        string
	Outcome      SaveOutcome
	RoomID       world.RoomID
}

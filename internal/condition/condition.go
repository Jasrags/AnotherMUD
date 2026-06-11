// Package condition is the leaf vocabulary for WoT status conditions
// (docs/specs/conditions.md, EPIC sub-epic S5). A condition is an ordinary
// effect (progression.EffectManager) carrying one of the recognized flags
// below; this package turns an entity's active condition flags into the
// aggregate combat/save Impact the host feeds to the combat hooks
// (conditions §3) and the save bonus (conditions §4).
//
// It imports nothing — the magnitudes are config (conditions §8) and the
// translation is a pure fold over the flag set, so it is trivially testable
// and the host owns the wiring (read flags from the effect manager → Resolve
// → drive AutoAttackConfig.Incapacitated / DefenderHitAdjust, the HitModAdjust
// penalty, and the save-bonus reduction).
package condition

import "strings"

// Recognized condition flags (conditions §2). A condition effect carries one
// of these on its Flags list. Values are lowercase because the effect manager
// normalizes flags to lowercase at apply time; matching here relies on that.
// The `condition:` prefix segregates them from ordinary effect flags
// (bless/cursed/well-fed) so an arbitrary effect flag is inert as a condition.
const (
	FlagFatigued   = "condition:fatigued"
	FlagProne      = "condition:prone"
	FlagBlinded    = "condition:blinded"
	FlagFrightened = "condition:frightened"
	FlagStunned    = "condition:stunned"
)

// flagPrefix segregates condition flags from ordinary effect flags.
const flagPrefix = "condition:"

// Flags returns the recognized condition flags in a stable order (the Core 5,
// conditions §2). Used by the `cure` verb to clear every condition.
func Flags() []string {
	return []string{FlagFatigued, FlagProne, FlagBlinded, FlagFrightened, FlagStunned}
}

// IsConditionFlag reports whether f is a condition flag (carries the
// `condition:` prefix) — the gate the `afflict` verb uses to refuse applying
// a non-condition effect (e.g. bless).
func IsConditionFlag(f string) bool { return strings.HasPrefix(f, flagPrefix) }

// AnyCondition reports whether any flag in the set is a condition flag.
func AnyCondition(flags []string) bool {
	for _, f := range flags {
		if IsConditionFlag(f) {
			return true
		}
	}
	return false
}

// Config holds the per-condition magnitudes (conditions §8). All values are
// positive magnitudes; Resolve applies the sign (penalties are subtracted by
// the consumer). The fatigued condition has no entry here — its effect is pure
// stat modifiers carried on the effect itself, not a combat hook.
type Config struct {
	ProneAttackPenalty   int // a prone attacker's melee to-hit penalty
	ProneVulnerability   int // to-hit bonus against a prone defender
	BlindedAttackPenalty int // a blinded attacker's to-hit penalty
	BlindedVulnerability int // to-hit bonus against a blinded defender
	StunnedVulnerability int // to-hit bonus against a stunned defender
	FearPenalty          int // morale penalty: a fear condition's −to attack AND saves
}

// DefaultConfig returns the engine-default magnitudes (the WoT pack may tune
// them). Rough translation of the d20 condition table (encounters.md): prone
// is ±4 melee; blinded is a heavy attacker penalty + a vulnerability bonus;
// stunned grants foes the source's +2; fear is the source's −2 morale.
func DefaultConfig() Config {
	return Config{
		ProneAttackPenalty:   4,
		ProneVulnerability:   4,
		BlindedAttackPenalty: 4,
		BlindedVulnerability: 2,
		StunnedVulnerability: 2,
		FearPenalty:          2,
	}
}

// Impact is the aggregate combat/save effect of an entity's active condition
// flags (conditions §3/§4). The host reads the directional pieces into the
// matching seams: AttackerHitPenalty subtracts from the attacker's to-hit,
// DefenderVulnerability adds to incoming to-hit, Incapacitated skips the
// attacker's swings, SavePenalty subtracts from the entity's save bonus, and
// ForcesFlee compels a frightened victim to flee.
type Impact struct {
	AttackerHitPenalty    int
	DefenderVulnerability int
	Incapacitated         bool
	SavePenalty           int
	ForcesFlee            bool
}

// Resolve folds a flag set into its aggregate Impact under cfg. Unrecognized
// flags are ignored (an arbitrary effect flag is inert as a condition);
// multiple conditions sum their penalties/bonuses and OR their booleans. The
// flags are expected lowercase (the effect manager's canonical form).
func Resolve(flags []string, cfg Config) Impact {
	var im Impact
	for _, f := range flags {
		switch f {
		case FlagFatigued:
			// Pure stat modifiers on the effect (−Str/−Dex); no combat-hook
			// contribution beyond what the stat block already applies.
		case FlagProne:
			im.AttackerHitPenalty += cfg.ProneAttackPenalty
			im.DefenderVulnerability += cfg.ProneVulnerability
		case FlagBlinded:
			im.AttackerHitPenalty += cfg.BlindedAttackPenalty
			im.DefenderVulnerability += cfg.BlindedVulnerability
		case FlagStunned:
			im.Incapacitated = true
			im.DefenderVulnerability += cfg.StunnedVulnerability
		case FlagFrightened:
			im.AttackerHitPenalty += cfg.FearPenalty
			im.SavePenalty += cfg.FearPenalty
			im.ForcesFlee = true
		}
	}
	return im
}

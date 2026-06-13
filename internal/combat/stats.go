package combat

// Stats is the per-combatant derived stat block the hit and damage
// rolls consume (spec combat §4.4-4.5). M7.1 carries only what combat
// itself reads; richer attributes (DEX, CON, race, class, derived
// modifiers) arrive with the M8 progression slice.
//
// Stats is a value type. Equipment changes between rounds publish a
// fresh block; the round loop reads a copy each round so a swap in
// mid-resolution cannot tear the inputs to a swing. Per-damage-type
// AC tables live in M8+ — a single AC field fits every damage type
// today.
type Stats struct {
	// HitMod is the modifier added to the attacker's d20 before the
	// comparison to AC. May be negative.
	HitMod int

	// AC is the defender's armor class for the only damage type the
	// M7.4 round loop will know how to compute. Higher = harder to
	// hit. A default of 10 is "no armor, no defensive stats".
	AC int

	// STR is the attacker's strength score. Retained as the raw attribute
	// (read by FromTemplateStats / score). Damage scaling no longer reads
	// it directly — that moved to DamageBonus (the channel layer), so a
	// non-d20 ruleset can scale damage off a different stat.
	STR int

	// DamageBonus is the flat amount added to rolled weapon damage before
	// mitigation (the channel layer's `damage_bonus` channel). Populated by
	// the holder's Stats() builder: from the channel mapping when present
	// (the baseline maps it to trunc((str-10)/2) = the old STRBonus), or
	// from STRBonus(STR) directly when no mapping is wired (bare test
	// actors). A zero value adds nothing — so a direct Stats{} literal that
	// omits it (most combat tests, STR 10) is unchanged.
	DamageBonus int

	// Mitigation is the defender's flat damage soak, subtracted from a
	// hit's raw damage (the channel layer's `mitigation` channel, design
	// §6). Zero for fantasy (armor folds into AC); a soak-based ruleset
	// (Shadowrun) maps it. The per-swing minimum-1 rule still applies after
	// subtraction, so mitigation never zeroes a landed hit.
	Mitigation int

	// Damage is the wielded-weapon damage expression (combat §4.5). A
	// zero DiceExpr means "use the engine's unarmed default" — see
	// EffectiveDamage. Populated by the holder's Stats(): players from
	// the wielded-slot item's dice, mobs from an equipped or natural
	// weapon set at spawn. A holder with no weapon leaves this zero.
	Damage DiceExpr

	// WeaponName is the display name carried on hit / miss events
	// alongside Damage. Empty falls back to the unarmed name when the
	// auto-attack phase composes its event payload.
	WeaponName string

	// CritThreatLow is the lowest d20 face that threatens a critical for
	// the wielded weapon (weapon-identity §4). Zero means "unset" — the
	// auto-attack phase defaults it to the natural maximum (20), the
	// pre-weapon-identity behavior. Populated from the wielded weapon.
	CritThreatLow int

	// CritMultiplier is the wielded weapon's critical damage-dice
	// multiplier (weapon-identity §4). Zero means "unset" — the
	// auto-attack phase falls back to the configured global default
	// (AutoAttackConfig.CritMultiplier). Populated from the wielded weapon.
	CritMultiplier int
}

// EffectiveDamage returns the dice expression the auto-attack phase
// should roll: the configured weapon damage if non-zero, otherwise the
// engine's unarmed default. The split keeps the spec §4.5 "default
// unarmed expression" rule in exactly one place.
func (s Stats) EffectiveDamage() DiceExpr {
	if s.Damage.IsZero() {
		return DefaultUnarmedDamage()
	}
	return s.Damage
}

// EffectiveWeaponName returns the display name for hit/miss events:
// the configured WeaponName if non-empty, otherwise the unarmed
// default. Mirrors EffectiveDamage so callers don't have to coordinate
// two fallback checks.
func (s Stats) EffectiveWeaponName() string {
	if s.WeaponName != "" {
		return s.WeaponName
	}
	return DefaultUnarmedWeaponName
}

// DefaultUnarmedWeaponName is the label shown on hit/miss events when
// no weapon is wielded. Centralized so a localization pass can swap
// it out in one place.
const DefaultUnarmedWeaponName = "fists"

// DefaultPlayerMaxHP is the hardcoded starting max HP applied to every
// connActor at login. M8 (progression: race + class + level) replaces
// this with a real derivation. Living next to DefaultPlayerStats so
// "replace player defaults" is a single-PR change later.
const DefaultPlayerMaxHP = 20

// DefaultPlayerStats is the hardcoded stat block every connActor reads
// today. Until M8 lands a real progression layer, every player rolls
// with the same numbers — that is enough for the round loop to have a
// non-zero input to compute against, and it puts off the
// race/class/level questions until they're actually being designed.
func DefaultPlayerStats() Stats {
	return Stats{HitMod: 0, AC: 10, STR: 10}
}

// Reserved stat-name keys in mob.Template.Stats consumed by combat.
// Templates may declare other keys; combat ignores them. M8 will
// formalize a fuller schema, but the keys it cares about today are
// listed here so a typo in a template is fixable by reading one file.
const (
	// StatKeyHPMax is the mob's maximum hit points. Defaulted to
	// DefaultMobMaxHP when absent or non-positive — a mob template
	// that forgot to declare HP still spawns and can fight rather
	// than dying on the first damage tick.
	StatKeyHPMax = "hp_max"
	// StatKeyHitMod is the mob's d20 hit modifier. Default 0.
	StatKeyHitMod = "hit_mod"
	// StatKeyAC is the mob's armor class. Default DefaultAC.
	StatKeyAC = "ac"
	// StatKeySTR is the mob's strength score. Default DefaultSTR.
	StatKeySTR = "str"
)

// DefaultMobMaxHP / DefaultAC / DefaultSTR are the spec-neutral
// fallbacks FromTemplateStats applies when a mob template omits the
// matching key. They are *engine* defaults, not balance defaults; a
// game that wants different floors should override them at the
// template level.
const (
	DefaultMobMaxHP = 10
	DefaultAC       = 10
	DefaultSTR      = 10
)

// FromTemplateStats lifts the combat-relevant fields out of a mob
// template's free-form Stats map and returns the derived block plus
// the spawn-time max HP. The split return is deliberate: max HP is a
// constructor input to Vitals (which lives next to the combatant),
// while the rest of the block is a value the combatant exposes
// per-round.
//
// Missing keys fall back to engine defaults (see DefaultMobMaxHP
// et al.) rather than zero — a template that declared no stats still
// produces a working combatant. A non-positive hp_max is treated as
// missing for the same reason: a template that accidentally wrote
// `hp_max: 0` should not spawn corpses.
func FromTemplateStats(in map[string]int) (Stats, int) {
	s := Stats{HitMod: 0, AC: DefaultAC, STR: DefaultSTR}
	maxHP := DefaultMobMaxHP
	if v, ok := in[StatKeyHPMax]; ok && v > 0 {
		maxHP = v
	}
	if v, ok := in[StatKeyHitMod]; ok {
		s.HitMod = v
	}
	if v, ok := in[StatKeyAC]; ok {
		s.AC = v
	}
	if v, ok := in[StatKeySTR]; ok {
		s.STR = v
	}
	return s, maxHP
}

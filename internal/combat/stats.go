package combat

// Ranged class values carried on Stats.RangedClass (ranged-combat §2).
// These MUST match the item package's RangedThrown / RangedProjectile wire
// values; combat keeps its own copy so the round loop can branch on the
// class without importing the item feature (the same decoupling that keeps
// the save axis and the damage-type label plain strings).
const (
	RangedThrown     = "thrown"
	RangedProjectile = "projectile"
)

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

	// RangedClass is the wielded weapon's ranged class (ranged-combat §2):
	// RangedThrown, RangedProjectile, or "" for a melee weapon. Populated from the
	// wielded weapon by the holder's Stats() builder. The round loop reads
	// it to drive ammo consumption (projectile) and, in Slice B, the band /
	// point-blank rules. Empty means melee, resolved exactly as today.
	RangedClass string

	// AmmoKind is the ammunition kind a projectile weapon consumes
	// (ranged-combat §3), matched verbatim against ammo items. Empty for a
	// thrown/melee weapon. The host's ammo hook reads it to find a matching
	// unit in the wielder's inventory.
	AmmoKind string

	// RangeIncrement is the wielded weapon's distance-falloff unit
	// (ranged-combat §2). Zero = unset. Carried for Slice B's band to-hit
	// falloff; inert in Slice A.
	RangeIncrement int

	// Reach is the wielded weapon's reach rating (special-weapons §3) — a
	// numeric, cross-ruleset weapon stat (0 = an ordinary close weapon). In WoT
	// the round loop reads `Reach > 0` so a reach wielder swings at the `near`
	// band as well as melee instead of auto-closing — the polearm's opening blows
	// on a foe still closing. A Shadowrun pack instead reads the NET reach
	// (attacker − defender) as a defense-roll modifier. Inert for a projectile
	// (RangedClass governs those) and at the melee band. Populated from the
	// wielded weapon by the holder's Stats() builder.
	Reach int

	// Set reports whether the wielded weapon carries the `set` special tag
	// (special-weapons §4 — set vs a charge: pike/bill/poleaxe/boarspear). A set
	// weapon braced against a foe that CHARGED into strike range this round (the
	// foe closed a band toward the wielder) lands a bonus blow — the polearm
	// receiving a charge. Read live from the wielded weapon by the holder's
	// Stats() builder; false for an ordinary weapon (every weapon without the
	// tag), so a non-set fight is unchanged.
	Set bool

	// WeaponDamageTypes are the wielded weapon's damage type(s)
	// (weapon-identity §2 — bludgeoning/piercing/slashing, extensible).
	// Empty means untyped. The damage application reads them to select the
	// defender's per-type Resistances (armor-depth §4). Populated from the
	// wielded weapon; a non-weapon/untyped attacker leaves this nil and is
	// reduced only by the type-agnostic Mitigation.
	WeaponDamageTypes []string

	// Resistances is the DEFENDER's per-damage-type damage reduction
	// (armor-depth §4), keyed by damage type, value the amount soaked.
	// Aggregated from worn armor at the holder's Stats() build time; nil
	// when unarmored or no resistances apply. Reduction is additive with
	// Mitigation and the per-swing minimum-1 rule still applies after both,
	// so resistance never zeroes a landed hit.
	Resistances map[string]int

	// OffHand is the off-hand weapon profile for a dual-wielding combatant
	// (two-weapon-fighting §3). nil ⇒ no off-hand attack (the common case:
	// single weapon, weapon+shield, or a two-hander). When set, the round
	// loop resolves ONE extra swing using these fields after the main swing(s).
	// The two-weapon to-hit penalty and the reduced (½×) off-hand Strength are
	// already baked in by the producer (the holder's Stats() builder), so
	// combat stays rules-agnostic — it just swings a second weapon profile.
	OffHand *OffHandProfile
}

// OffHandProfile is the weapon a dual-wielding combatant strikes with in its
// off hand (two-weapon-fighting §3). It mirrors the wielded-weapon fields of
// Stats for the off-hand swing: its own dice, crit range/multiplier, damage
// type(s), the off-hand-penalized HitMod, and the reduced-Strength DamageBonus.
// The off-hand swing is resolved by the same machinery as the main swing — only
// these fields and the per-attacker hit adjustments feed it.
type OffHandProfile struct {
	// Damage is the off-hand weapon's damage expression (zero ⇒ unarmed default).
	Damage DiceExpr
	// WeaponName is the display name carried on the off-hand swing's events.
	WeaponName string
	// WeaponDamageTypes are the off-hand weapon's damage type(s), read for the
	// defender's per-type resistance on the off-hand swing.
	WeaponDamageTypes []string
	// CritThreatLow / CritMultiplier are the off-hand weapon's §4 crit params
	// (zero ⇒ the round loop's defaults), exactly as for the main weapon.
	CritThreatLow  int
	CritMultiplier int
	// HitMod is the off-hand swing's to-hit modifier with the two-weapon
	// penalty already applied; the round loop still adds the same per-attacker
	// adjustments (darkness/armor/condition) it adds to the main swing.
	HitMod int
	// DamageBonus is the off-hand swing's flat damage bonus — the reduced (½×)
	// Strength share plus any flat bonuses (two-weapon-fighting §4.2).
	DamageBonus int
	// Attacks is the number of off-hand strikes this profile grants per round
	// (two-weapon-fighting §3.1 — the "off-hand attacks granted" count). Zero or
	// one means a single off-hand strike (the slice-1 baseline); Improved
	// Two-Weapon Fighting raises it to two. Each strike AFTER the first takes the
	// cumulative AutoAttackConfig.SecondaryOffHandPenalty to hit (§4.3); the
	// damage is identical across strikes. The round loop floors this at one.
	Attacks int
}

// DefaultTwoWeaponMainPenalty / DefaultTwoWeaponOffHandPenalty are the to-hit
// penalties applied while fighting with two weapons (two-weapon-fighting §4.1):
// a smaller penalty on the main hand, a larger one on the off hand. The WoT/d20
// light-weapon baseline (-4 main / -8 off); the Two-Weapon Fighting and
// Ambidexterity feats reduce these (slice 2). Constants for now; the spec's
// configuration surface anticipates env-wiring these like the other knobs.
const (
	DefaultTwoWeaponMainPenalty    = 4
	DefaultTwoWeaponOffHandPenalty = 8
)

// DefaultPowerAttackTrade is the Power Attack stance magnitude (feats Bucket C):
// the attacker subtracts this from to-hit and adds it to melee damage while the
// stance is on (a two-handed wield adds twice this to damage — size-and-wielding
// §4.2). d20 lets the attacker pick any value up to BAB per action; this engine
// has no BAB and no per-swing choice (Decision 0), so the trade is a fixed
// posture. A constant for now; env-wiring is a follow-up like the two-weapon
// penalties above.
const DefaultPowerAttackTrade = 2

// TypedResistance returns the defender's damage reduction against an
// attack of the given damage type(s) (armor-depth §4). It returns the
// resistance of the FIRST of the attacker's declared types that the
// defender has an entry for (iterating in the weapon's declared order, so
// content controls the precedence); an untyped attacker, an empty map, or
// no matching type yields zero. This composes additively with the
// type-agnostic Mitigation at the damage step — it does not replace it.
func TypedResistance(resistances map[string]int, damageTypes []string) int {
	if len(resistances) == 0 {
		return 0
	}
	for _, t := range damageTypes {
		if r, ok := resistances[t]; ok {
			return r
		}
	}
	return 0
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
	// StatKeyDamageMod is the flat weapon-damage modifier composed into the
	// `damage_bonus` channel (the sibling of hit_mod for the damage axis).
	// Default 0; a power-wrought weapon contributes it on equip (masterwork
	// §3). The baseline channel map adds it to the STR-derived damage bonus.
	StatKeyDamageMod = "damage_mod"
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

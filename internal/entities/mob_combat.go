package entities

import (
	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/size"
)

// This file holds the combat-facing surface of MobInstance: its combat
// identity, hit-point handle, the per-swing combat.Stats snapshot, and the
// spawn-pipeline setters that arm the mob (weapon + armor resistance). The
// instance's fields and lifecycle live in mob.go; these methods read them.

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
		// Ranged class (ranged-combat §2): a projectile-wielding mob shoots
		// from range — the round loop opens it at far, applies the per-band
		// falloff/point-blank, and (via the AmmoFor hook's mob branch) fires
		// it free. Empty for a melee/natural weapon.
		s.RangedClass = m.weaponRangedClass
		s.AmmoKind = m.weaponAmmoKind
		// size-and-wielding §4.2 / §5: a two-handed MELEE wield multiplies the
		// Strength contribution to damage by the two-handed factor — derived
		// from the weapon's size relative to THIS mob's size, so a large mob
		// one-hands (no bonus) a weapon a medium player must two-hand. Add only
		// the EXTRA Strength on top of the 1× already in DamageBonus, and only
		// for a melee weapon (ranged Strength is the ranged concern). Mirrors
		// connActor.Stats. A natural/sizeless weapon resolves to baseline.
		if m.weaponRangedClass == "" && size.Mode(m.weaponSize, m.size) == size.TwoHanded {
			s.DamageBonus += size.TwoHandedStrBonus(combat.STRBonus(str), size.DefaultTwoHandedStrFactor)
		}
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

// SetWeapon installs the mob's attack dice + display name (combat §4.5) plus
// the equipped weapon's damage types, ranged class (ranged-combat §2 — a
// bow-wielding mob shoots from range), and size (size-and-wielding §2 — the
// two-handed grip is derived relative to the mob's own size). Called during the
// spawn pipeline only:
// buildMobFromTemplate seeds the natural weapon (melee — empty ranged class),
// then EquipMobAtSpawn overrides it with an equipped weapon. Not safe to call
// after the mob is targetable in combat (read lock-free by Stats on the tick
// goroutine).
func (m *MobInstance) SetWeapon(dice combat.DiceExpr, name string, damageTypes []string, rangedClass, ammoKind, weaponSize string) {
	m.weapon = dice
	m.weaponName = name
	m.weaponDamageTypes = damageTypes
	m.weaponRangedClass = rangedClass
	m.weaponAmmoKind = ammoKind
	m.weaponSize = weaponSize
}

// SetResistances installs the mob's aggregated per-damage-type damage
// reduction from worn armor (armor-depth §4). Called once during the spawn
// pipeline (EquipMobAtSpawn) after gear is placed; not safe to call after
// the mob is targetable (read lock-free by Stats on the tick goroutine).
func (m *MobInstance) SetResistances(resistances map[string]int) {
	m.resistances = resistances
}

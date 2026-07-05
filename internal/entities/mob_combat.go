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
	// Capture the un-penalized hit modifier BEFORE building the block, so the
	// off-hand profile prices its larger penalty off the same base the main hand
	// does even if a future modifier mutates s.HitMod between here and the
	// off-hand branch (mirrors the player producer's baseHitMod discipline).
	baseHitMod := hitMod
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
		s.RangedStyle = m.weaponRangedStyle
		// subdual-damage §2: an equipped nonlethal weapon (a mob's sap) knocks the
		// victim out on a finishing blow. A natural weapon leaves this false (lethal).
		s.Subdual = m.weaponSubdual
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
		// two-weapon-fighting §2.3: a mob that spawned with a second weapon in the
		// off-hand slot dual-wields. The off hand grants an extra strike only when
		// the main weapon is MELEE (§3) and the off-hand weapon resolves the LIGHT
		// wield mode for the MOB's own size (§2.2, relative to the mob) — a two-
		// handed main ties up the off hand at equip time, so it never reaches here.
		// A mob holds no feats, so it always fights at the full un-feated penalty:
		// the main-hand penalty folds into s.HitMod, the off-hand profile carries
		// the larger off-hand penalty off the pre-penalty hit and the ½× Strength
		// damage, and makes exactly ONE strike. This mirrors connActor.Stats minus
		// the feat-cache reads. Crit fields are left default, as the mob main weapon
		// also omits them.
		if m.weaponRangedClass == "" && !m.offWeapon.IsZero() && size.Mode(m.offWeaponSize, m.size) == size.Light {
			s.HitMod -= combat.DefaultTwoWeaponMainPenalty
			strBonus := combat.STRBonus(str)
			s.OffHand = &combat.OffHandProfile{
				Damage:            m.offWeapon,
				WeaponName:        m.offWeaponName,
				WeaponDamageTypes: append([]string(nil), m.offWeaponDamageTypes...), // copy: self-contained
				HitMod:            baseHitMod - combat.DefaultTwoWeaponOffHandPenalty,
				// ½× Strength on the off hand (only the Strength term reduced, flat
				// bonuses stay 1×), mirroring the player and the two-handed rule.
				DamageBonus: damageBonus + size.StrBonusDelta(strBonus, size.DefaultOffHandStrFactor),
				Attacks:     1, // mobs never take Improved Two-Weapon Fighting
			}
		}
	}
	// subdual-damage §6: the worn-armor rating the whip anti-armor gate reads
	// when this mob is the target. 0 = unarmored.
	s.ArmorRating = m.armorRating
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
func (m *MobInstance) SetWeapon(dice combat.DiceExpr, name string, damageTypes []string, rangedClass, ammoKind, rangedStyle, weaponSize string) {
	m.weapon = dice
	m.weaponName = name
	m.weaponDamageTypes = damageTypes
	m.weaponRangedClass = rangedClass
	m.weaponAmmoKind = ammoKind
	m.weaponRangedStyle = rangedStyle
	m.weaponSize = weaponSize
}

// SetWeaponSubdual marks the mob's equipped weapon nonlethal (subdual-damage §2
// — a sap/whip-wielding mob), so its finishing blow knocks out (subdual-damage
// §4). Called during the spawn pipeline only (EquipMobAtSpawn, after SetWeapon),
// read lock-free by Stats — the same write-once-at-spawn contract. The natural
// weapon never calls it, so a bite/claw stays lethal (the default).
func (m *MobInstance) SetWeaponSubdual(subdual bool) {
	m.weaponSubdual = subdual
}

// SetOffWeapon installs the mob's OFF-HAND weapon (two-weapon-fighting §2.3) —
// the dice, display name, damage types, and size of a second equipped weapon
// that fits the off-hand slot. Called during the spawn pipeline only
// (EquipMobAtSpawn, after the main weapon is set), then read lock-free by Stats
// on the tick goroutine — the same write-once-at-spawn contract as SetWeapon.
// Whether the off-hand weapon actually grants the off-hand attack is decided in
// Stats (melee main + light-for-the-mob off-hand); this setter only records it.
func (m *MobInstance) SetOffWeapon(dice combat.DiceExpr, name string, damageTypes []string, weaponSize string) {
	m.offWeapon = dice
	m.offWeaponName = name
	m.offWeaponDamageTypes = damageTypes
	m.offWeaponSize = weaponSize
}

// SetArmorRating installs the mob's worn-armor AC sum (subdual-damage §6 — the
// whip anti-armor gate's defender rating). Called once during the spawn pipeline
// (EquipMobAtSpawn) after gear is placed; read lock-free by Stats on the tick
// goroutine — the same write-once-at-spawn contract as SetResistances.
func (m *MobInstance) SetArmorRating(rating int) {
	m.armorRating = rating
}

// SetResistances installs the mob's aggregated per-damage-type damage
// reduction from worn armor (armor-depth §4). Called once during the spawn
// pipeline (EquipMobAtSpawn) after gear is placed; not safe to call after
// the mob is targetable (read lock-free by Stats on the tick goroutine).
func (m *MobInstance) SetResistances(resistances map[string]int) {
	m.resistances = resistances
}

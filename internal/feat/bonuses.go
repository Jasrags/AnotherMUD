package feat

import "strings"

// Taken is one feat a character holds (EPIC S4 Phase 3). It mirrors
// player.KnownFeat without importing player — keeping feat a leaf package; the
// session converts between the two. FeatID resolves in the Registry; Param
// binds a per-parameter feat (a weapon/skill id); Count is the stack size for
// a stackable feat (a Toughness taken 3 times applies its grant 3×).
type Taken struct {
	FeatID string
	Param  string
	Count  int
}

// Bonuses is the aggregate of every modifier a character's held feats confer
// (EPIC S4 Phase 3). Phase 3a populates Saves; later consumer slices add
// fields (max HP, per-weapon hit/crit, per-skill, granted abilities) — each a
// new field read by its own surface, computed by the same ComputeBonuses.
type Bonuses struct {
	// Saves is the additive per-axis saving-throw bonus, keyed by the lowercased
	// axis name ("fortitude"/"reflex"/"will"). Nil/absent = no bonus.
	Saves map[string]int
	// MaxHP is the additive maximum-HP bonus (Toughness and friends). Zero =
	// no bonus.
	MaxHP int
	// HitByCategory is the per-weapon-category to-hit bonus (Weapon Focus),
	// keyed by the lowercased weapon category. Nil = none.
	HitByCategory map[string]int
	// CritByCategory is the per-weapon-category critical threat-range WIDEN
	// (Improved Critical) — faces to lower the weapon's threat-low by. Nil = none.
	CritByCategory map[string]int
	// SkillByID is the per-skill check bonus (Skill Emphasis), keyed by the
	// skill ability id. Nil = none.
	SkillByID map[string]int
	// Abilities are the ability ids a feat teaches (Power Attack). Nil = none.
	Abilities []string
	// TwoWeaponHitReduce is the additive reduction to the two-weapon to-hit
	// penalty on BOTH hands (Two-Weapon Fighting — two-weapon-fighting §4.1).
	// Zero = none. The consumer clamps the resulting penalty at zero.
	TwoWeaponHitReduce int
	// OffHandHitReduce is the additive reduction to the OFF-HAND two-weapon
	// penalty only (Ambidexterity, removing the off-hand-specific extra — §4.1).
	// Zero = none. Composes additively with TwoWeaponHitReduce on the off hand.
	OffHandHitReduce int
	// OffHandExtraAttacks is the number of EXTRA off-hand strikes beyond the
	// first (Improved Two-Weapon Fighting — §3.1). Zero = the one baseline
	// strike. The consumer sets OffHandProfile.Attacks = 1 + this.
	OffHandExtraAttacks int
	// DamageByCategory is the per-weapon-category melee damage bonus (Weapon
	// Specialization), keyed by the lowercased weapon category. Nil = none. The
	// damage sibling of HitByCategory.
	DamageByCategory map[string]int
	// ACBonus is the additive Armor Class bonus (Dodge and friends). Zero = no
	// bonus. The AC sibling of MaxHP.
	ACBonus int
}

// ComputeBonuses aggregates the bonuses conferred by the held feats, resolving
// each against reg (EPIC S4 Phase 3). A held feat absent from the registry
// (content removed since it was taken) is skipped fail-soft. A stackable feat
// applies its grants Count times (a non-positive count counts as one); a
// per-parameter or single feat applies them once.
func ComputeBonuses(held []Taken, reg *Registry) Bonuses {
	var b Bonuses
	if reg == nil {
		return b
	}
	for _, t := range held {
		f, ok := reg.Get(t.FeatID)
		if !ok {
			continue // removed-content feat, fail-soft
		}
		mult := 1
		if f.MultiTake == MultiTakeStackable && t.Count > 1 {
			mult = t.Count
		}
		for _, g := range f.Grants {
			switch g.Kind {
			case GrantSaveBonus:
				if b.Saves == nil {
					b.Saves = make(map[string]int)
				}
				// g.Target is already lowercased+trimmed by Register, and
				// ComputeBonuses only ever reads registry-owned feats.
				b.Saves[g.Target] += g.Magnitude * mult
			case GrantMaxHP:
				b.MaxHP += g.Magnitude * mult
			case GrantACBonus:
				b.ACBonus += g.Magnitude * mult
			case GrantHitBonus:
				// per-weapon-category: the take's Param names the category.
				if t.Param != "" {
					if b.HitByCategory == nil {
						b.HitByCategory = make(map[string]int)
					}
					b.HitByCategory[t.Param] += g.Magnitude * mult
				}
			case GrantCritThreat:
				if t.Param != "" {
					if b.CritByCategory == nil {
						b.CritByCategory = make(map[string]int)
					}
					b.CritByCategory[t.Param] += g.Magnitude * mult
				}
			case GrantDamageBonus:
				// per-weapon-category: the take's Param names the category
				// (Weapon Specialization, the damage sibling of Weapon Focus).
				if t.Param != "" {
					if b.DamageByCategory == nil {
						b.DamageByCategory = make(map[string]int)
					}
					b.DamageByCategory[t.Param] += g.Magnitude * mult
				}
			case GrantSkillBonus:
				// Two forms (symmetric with GrantSaveBonus): a per-param feat
				// (Skill Emphasis) names its skill via the take's Param; a
				// fixed-axis feat (Alertness → perception) names it via the
				// grant Target, already lowercased+trimmed by Register. Param
				// wins when both are present.
				key := t.Param
				if key == "" {
					key = g.Target
				}
				if key != "" {
					if b.SkillByID == nil {
						b.SkillByID = make(map[string]int)
					}
					b.SkillByID[key] += g.Magnitude * mult
				}
			case GrantTwoWeaponHit:
				b.TwoWeaponHitReduce += g.Magnitude * mult
			case GrantOffHandHit:
				b.OffHandHitReduce += g.Magnitude * mult
			case GrantOffHandAttack:
				b.OffHandExtraAttacks += g.Magnitude * mult
			case GrantAbility:
				if g.Target != "" {
					// Normalize for uniformity with the other keyed kinds
					// (prof.Learn also normalizes, so this is belt-and-braces).
					b.Abilities = append(b.Abilities, strings.ToLower(strings.TrimSpace(g.Target)))
				}
			}
		}
	}
	return b
}

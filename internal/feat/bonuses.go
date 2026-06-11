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
				b.Saves[strings.ToLower(strings.TrimSpace(g.Target))] += g.Magnitude * mult
			}
		}
	}
	return b
}

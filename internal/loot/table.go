// Package loot models content-defined loot tables and the spawn-time
// roll that turns a table into a list of item-template ids
// (mobs-ai-spawning §6.3). It is a leaf package: it depends only on a
// minimal Roller interface, never on the entity, item, or pack layers,
// so both the spawn pipeline and tests can drive it without a cycle.
//
// Loot *generation* (rolling the table into a mob's contents at spawn)
// lives here; loot *delivery* (corpse creation on death) lives in the
// loot-and-corpses feature and consumes the contents this produced.
package loot

// Roller is the minimal randomness surface the table roll needs. It
// mirrors combat.Roller and progression.Roller so a *math/rand/v2.Rand
// satisfies it directly. Implementations MUST panic on n <= 0, matching
// math/rand/v2.Rand.IntN.
type Roller interface {
	// IntN returns a non-negative pseudo-random integer in [0, n).
	IntN(n int) int
}

// GuaranteedEntry names an item that always drops, Count times
// (mobs-ai-spawning §6.3 step 1). A Count <= 0 or empty ItemID
// contributes nothing.
type GuaranteedEntry struct {
	ItemID string
	Count  int
}

// WeightedEntry is one candidate in a weighted pool: an item id and a
// relative selection Weight. A weight <= 0 excludes the entry from
// selection.
type WeightedEntry struct {
	ItemID string
	Weight int
}

// RareBonus is the optional one-roll bonus pool (mobs-ai-spawning §6.3
// step 3). Chance is a percentage in [0, 100]; on success exactly one
// weighted selection is taken from Entries. Chance <= 0 never fires;
// Chance >= 100 always fires.
type RareBonus struct {
	Chance  int
	Entries []WeightedEntry
}

// Table is a content-defined loot table (mobs-ai-spawning §6.3). The
// coin block (loot-and-corpses §3) is not modeled here yet — it lands
// with corpse creation (M22.2), where it is rolled on death rather than
// at spawn.
//
// Table is value-typed for registry storage; the registry hands callers
// a pointer to its own deep copy. Callers MUST NOT mutate it.
type Table struct {
	ID         string
	Priority   int
	Guaranteed []GuaranteedEntry
	Weighted   []WeightedEntry
	// PoolRolls is the number of independent weighted selections taken
	// from Weighted (step 2). Zero means the weighted pool never rolls.
	PoolRolls int
	RareBonus *RareBonus
}

// RollItems resolves a table into the list of item-template ids to
// instantiate, in spec order (mobs-ai-spawning §6.3): all guaranteed
// entries first, then PoolRolls independent weighted selections, then —
// last — a single rare-bonus roll. Each roll is independent. A nil
// table or a table with no productive entries yields a nil/empty slice
// without touching the roller.
func RollItems(t *Table, r Roller) []string {
	if t == nil {
		return nil
	}
	var out []string

	// 1. Guaranteed pool: each entry appended Count times.
	for _, g := range t.Guaranteed {
		if g.ItemID == "" || g.Count <= 0 {
			continue
		}
		for i := 0; i < g.Count; i++ {
			out = append(out, g.ItemID)
		}
	}

	// 2. Weighted pool: PoolRolls independent selections.
	for i := 0; i < t.PoolRolls; i++ {
		if id, ok := selectWeighted(t.Weighted, r); ok {
			out = append(out, id)
		}
	}

	// 3. Rare bonus: one chance roll, then one weighted selection.
	if rb := t.RareBonus; rb != nil && rb.Chance > 0 && len(rb.Entries) > 0 {
		if r.IntN(100) < rb.Chance {
			if id, ok := selectWeighted(rb.Entries, r); ok {
				out = append(out, id)
			}
		}
	}

	return out
}

// selectWeighted picks one entry with probability proportional to its
// weight. Entries with weight <= 0 are ignored. Returns ("", false)
// when no entry has positive weight, leaving the roller untouched.
func selectWeighted(entries []WeightedEntry, r Roller) (string, bool) {
	total := 0
	for _, e := range entries {
		if e.Weight > 0 {
			total += e.Weight
		}
	}
	if total <= 0 {
		return "", false
	}
	roll := r.IntN(total)
	for _, e := range entries {
		if e.Weight <= 0 {
			continue
		}
		if roll < e.Weight {
			return e.ItemID, true
		}
		roll -= e.Weight
	}
	// Unreachable: roll is in [0, total) and the weights sum to total.
	return "", false
}

package entities

import (
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// EquipResult reports the outcome of EquipMobAtSpawn so the caller can
// log skipped items at its chosen level. Equipped counts the items
// successfully instantiated, applied, and filed; Missing lists the
// equipment template ids that had no matching item template — skipped
// silently per mobs-ai-spawning §3.3 step 1.
type EquipResult struct {
	Equipped int
	Missing  []string
}

// EquipMobAtSpawn instantiates and equips each item id on a mob
// template's equipment list (mobs-ai-spawning §3.3). For each id it:
//
//  1. Looks up the item template; a miss is recorded in Missing and
//     skipped silently (§3.3 step 1).
//  2. Spawns an item instance through the store, which tracks it in the
//     live-entity set (§3.3 step 4).
//  3. Applies the item's stat modifiers to the mob's stat block under
//     EquipmentSourceKey(item id) so the set can be reversed cleanly
//     later (§3.3 step 3) — the same per-item grouping the player
//     `equip`/`unequip` path uses.
//  4. Files the item in the mob's Contents so it travels with the mob
//     and drops into the corpse on death (loot-and-corpses §2.1),
//     mirroring how spawn-time loot is carried.
//
// Mobs carry no equipment slot index today: nothing reads "which slot a
// mob's sword occupies" the way players' Equip enforces slot capacity.
// §3.3 step 2 ("equip into that slot") is therefore modeled as
// carry-plus-modifiers; the item's declared `slot` property survives on
// the instance for a future `look <mob>` that wants to render worn gear,
// but no capacity check happens here. A spawn error (only possible if
// the atomic id generator is broken) aborts and is returned so the
// caller can surface it the way the loot path does.
//
// A nil mob, nil registry, or empty id list is a no-op. A nil contents
// index skips step 4 only (modifiers still apply) — convenient for tests
// that don't wire a Contents.
func (s *Store) EquipMobAtSpawn(m *MobInstance, ids []string, items *item.Templates, contents *Contents) (EquipResult, error) {
	var res EquipResult
	if m == nil || items == nil || len(ids) == 0 {
		return res, nil
	}
	for _, id := range ids {
		tpl, err := items.Get(item.TemplateID(id))
		if err != nil {
			res.Missing = append(res.Missing, id)
			continue
		}
		it, err := s.Spawn(tpl)
		if err != nil {
			// Track on a fresh atomic id only fails when id generation is
			// broken; surface it like the loot path rather than swallow.
			return res, fmt.Errorf("equip mob %s: spawn item %q: %w", m.ID(), id, err)
		}
		if mods := it.Modifiers(); len(mods) > 0 {
			translated := make([]stats.Modifier, 0, len(mods))
			for _, mod := range mods {
				translated = append(translated, stats.Modifier{Stat: mod.Stat, Value: mod.Value})
			}
			m.AddModifiers(EquipmentSourceKey(it.ID()), translated)
		}
		if contents != nil {
			contents.Put(m.ID(), it.ID())
		}
		res.Equipped++
	}
	return res, nil
}

package entities

import (
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// EquipResult reports the outcome of EquipMobAtSpawn so the caller can
// log skipped items at its chosen level. Equipped counts the items
// instantiated and filed (carried); Missing lists the equipment template
// ids that had no matching item template — skipped silently per
// mobs-ai-spawning §3.3 step 1. Skipped lists items that were CARRIED but
// could not be slot-equipped (no eligible free slot, or not equippable) —
// so their modifiers were NOT applied to the mob's stat block (inventory-
// equipment-items §3.7). A carried-only item still drops as loot.
type EquipResult struct {
	Equipped int
	Missing  []string
	Skipped  []string
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
// Slot enforcement (inventory-equipment-items §3.7): mobs equip through
// the same eligibility/capacity/footprint rules as players. Each item is
// placed into a free eligible slot (via the shared slot.Footprint helper),
// marking that footprint occupied so a second item competing for the same
// slot cannot also apply its modifiers — bounding the mob's stat budget.
// An item that does not fit (no eligible free slot, or no eligible slots
// at all) is NOT slot-equipped: its modifiers are skipped and it cannot
// arm the mob, but it is still CARRIED (filed in Contents) so it drops as
// loot, and recorded in EquipResult.Skipped for the caller to log. Unlike
// players, mobs never auto-swap their own gear at spawn — a conflict skips
// the later item rather than displacing the earlier one.
//
// When slots is nil the registry is unavailable, so no slot check is
// possible: the method falls back to the legacy apply-all behavior (every
// item's modifiers apply, first weapon wins). Production always supplies a
// registry; the fallback keeps registry-free tests and callers working.
//
// A spawn error (only possible if the atomic id generator is broken)
// aborts and is returned so the caller can surface it like the loot path.
// A nil mob, nil item registry, or empty id list is a no-op. A nil
// contents index skips the carry step only (modifiers still apply to
// fitting items) — convenient for tests that don't wire a Contents.
func (s *Store) EquipMobAtSpawn(m *MobInstance, ids []string, items *item.Templates, contents *Contents, slots *slot.Registry) (EquipResult, error) {
	var res EquipResult
	if m == nil || items == nil || len(ids) == 0 {
		return res, nil
	}
	occupied := make(map[string]bool) // slot keys claimed by fitting gear
	weaponSet := false                // main weapon chosen (overrides the natural weapon)
	offSet := false                   // off-hand weapon chosen (two-weapon-fighting §2.3)
	var resist map[string]int         // per-type resistance summed across fitting armor (armor-depth §4)
	armorRating := 0                  // worn armor AC sum (subdual-damage §6, the whip anti-armor gate)
	var armorDexCap *int              // most restrictive worn-armor max-Dex cap (armor-depth §3; the dex_ac producer)
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

		// Decide whether this item occupies a slot, and which. With no registry
		// we cannot enforce slots, so every item is treated as fitting (legacy
		// apply-all) and the base slot is unknown.
		fits := true
		base := ""
		if slots != nil {
			base, fits = placeMobItem(slots, it, occupied)
		}

		if fits {
			if mods := it.Modifiers(); len(mods) > 0 {
				translated := make([]stats.Modifier, 0, len(mods))
				for _, mod := range mods {
					translated = append(translated, stats.Modifier{Stat: mod.Stat, Value: mod.Value})
				}
				m.AddModifiers(EquipmentSourceKey(it.ID()), translated)
			}
			// Weapon assignment (combat §4.5, two-weapon-fighting §2.3). The MAIN
			// weapon is the one in the wield slot (overriding any natural weapon);
			// a second weapon in the OFF-HAND slot becomes the off-hand weapon,
			// lighting up dual-wield. Whether the off-hand actually attacks is
			// decided in Stats (melee main + light-for-the-mob). With no registry
			// the slot is unknown, so we keep the legacy "first weapon is the main
			// weapon" rule and grant no off-hand (it needs the slot signal).
			if dice, ok := it.WeaponDamage(); ok {
				switch {
				case slots == nil:
					if !weaponSet {
						m.SetWeapon(dice, it.Name(), it.DamageTypes(), it.RangedClass(), it.AmmoKind(), it.RangedStyle(), it.WeaponSize())
						m.SetWeaponSubdual(it.Subdual())       // subdual-damage §2: a mob's nonlethal weapon
						m.SetWeaponTargetPool(it.TargetPool()) // shadowrun-mvp SR-M3b: a mob's stun weapon
						weaponSet = true
					}
				case base == slot.WieldSlot && !weaponSet:
					m.SetWeapon(dice, it.Name(), it.DamageTypes(), it.RangedClass(), it.AmmoKind(), it.RangedStyle(), it.WeaponSize())
					m.SetWeaponSubdual(it.Subdual())       // subdual-damage §2: a mob's nonlethal weapon
					m.SetWeaponTargetPool(it.TargetPool()) // shadowrun-mvp SR-M3b: a mob's stun weapon
					weaponSet = true
				case base == slot.OffHandSlot && !offSet:
					// First off-hand weapon wins. The off-hand slot's Max-1 cap
					// already prevents a second item landing here, so !offSet is
					// belt-and-braces, parallel to the main-weapon guard.
					// TODO(SR-M3b): target_pool is not wired for the off-hand — an
					// off-hand stun weapon routes to hp, not the Stun monitor
					// (OffHandProfile carries no TargetPool). Fine for the main-hand-
					// forward MVP; revisit before authoring dual-wield SR content.
					m.SetOffWeapon(dice, it.Name(), it.DamageTypes(), it.WeaponSize())
					offSet = true
				}
			}
			// armor-depth §4: sum per-type resistance across fitting armor
			// (only slot-equipped gear soaks, mirroring the modifier rule).
			for dt, amt := range it.Resistances() {
				if resist == nil {
					resist = make(map[string]int)
				}
				resist[dt] += amt
			}
			armorRating += it.ArmorBonus() // subdual-damage §6: worn armor rating (whip gate)
			// armor-depth §3: the most restrictive (lowest) max-Dex cap across worn
			// armor, feeding cappedDexAC (the dex_ac producer). Mirrors the player's
			// recomputeWeaponLocked snapshot. ArmorMaxDex returns a fresh copy.
			if mdx := it.ArmorMaxDex(); mdx != nil && (armorDexCap == nil || *mdx < *armorDexCap) {
				armorDexCap = mdx
			}
		} else {
			// Carried but not slot-equipped: modifiers skipped (§3.7).
			res.Skipped = append(res.Skipped, id)
		}

		// Carry regardless of fit so the item still drops as loot
		// (loot-and-corpses §2.1).
		if contents != nil {
			contents.Put(m.ID(), it.ID())
		}
		res.Equipped++
	}
	if resist != nil {
		m.SetResistances(resist)
	}
	if armorRating != 0 {
		m.SetArmorRating(armorRating)
	}
	if armorDexCap != nil {
		m.SetArmorDexCap(armorDexCap)
	}
	return res, nil
}

// placeMobItem reports whether it fits a free eligible slot and, when it
// does, marks its whole footprint occupied and returns the BASE slot it was
// placed into (the eligible-slot key, e.g. slot.WieldSlot or slot.OffHandSlot —
// used by the caller to tell a main weapon from an off-hand one,
// two-weapon-fighting §2.3). Mirrors the player equip path's eligibility +
// footprint logic (shared slot helpers) but without auto-swap: a slot already
// taken by earlier gear is simply unavailable, so the item is skipped rather
// than displacing the occupant. Returns ("", false) when the item declares no
// eligible slots or none can host its footprint with every key free.
func placeMobItem(reg *slot.Registry, it *ItemInstance, occupied map[string]bool) (string, bool) {
	eligible := it.EligibleSlots()
	if len(eligible) == 0 {
		return "", false
	}
	companions := it.CompanionSlots()
	for _, base := range eligible {
		fp, err := reg.Footprint(base, companions, occupied)
		if err != nil {
			continue
		}
		// Footprint returns lowest-free keys but FALLS BACK to an occupied
		// index-0 key when a slot is full (its contract serves the player
		// auto-swap path, where that key marks the occupant to displace).
		// Mobs do not auto-swap their own gear, so this all-free re-check is
		// load-bearing: it rejects a footprint that would require evicting
		// already-placed gear. Do not drop it.
		allFree := true
		for _, k := range fp {
			if occupied[k] {
				allFree = false
				break
			}
		}
		if !allFree {
			continue
		}
		for _, k := range fp {
			occupied[k] = true
		}
		return base, true
	}
	return "", false
}

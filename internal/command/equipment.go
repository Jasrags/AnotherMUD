package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// statKeyArmorCheck is the stat carrying the wearer's total worn-armor check
// penalty MAGNITUDE (armor-depth §6) — the sum of each worn armor/shield's
// grade-reduced penalty, applied as equipment modifiers. A Str/Dex skill
// check subtracts it from the roll bonus.
const statKeyArmorCheck = "armor_check"

// EquipHandler implements `equip <item> <slot>` per spec
// inventory-equipment-items §3.3.
//
// Flow:
//  1. Parse the slot argument and validate it against the registry.
//  2. Resolve the item argument against the actor's inventory using
//     the shared keyword resolver.
//  3. If the slot is at capacity, unequip the occupant of the
//     lowest-indexed sub-slot (the "displaced" item) and report it.
//  4. Pick the lowest free sub-slot key for the new item.
//  5. Call Actor.Equip with the item's modifier set translated to
//     the holder's stat-block form.
//
// Two-actor / lock-order safety: this handler only mutates the
// invoking actor's state (its inventory, equipment, and stat block).
// No cross-actor lock is taken. Auto-swap is the unequip + equip
// composition on the same actor, so the actor mutex protects both
// halves end to end.
// weaponProficiencyChecker is the optional actor capability that reports
// whether the actor's currently-wielded weapon is one their class is
// proficient with (weapon-identity §3). connActor implements it; actors
// that don't model proficiency simply don't, and the equip path skips the
// non-proficient warning for them.
type weaponProficiencyChecker interface {
	IsWeaponProficient() bool
}

// armorProficiencyChecker mirrors weaponProficiencyChecker for armor
// (armor-depth §5): whether every worn tiered armor is one the actor's
// class(es) grant. connActor implements it; actors that don't model armor
// proficiency simply don't, and the equip path skips the clumsy-wear cue.
type armorProficiencyChecker interface {
	IsArmorProficient() bool
}

func EquipHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Slots == nil {
		return c.Actor.Write(ctx, "You can't equip anything right now.")
	}

	// `equip <item> [slot]` declares item (ArgInventory) then an OPTIONAL
	// slot (ArgKeyword). The item is resolved first so an omitted slot can
	// be resolved against the item's eligible set (Decision A, §3.4 step 1).
	// Single-token item references only.
	item, ok := resolvedItemInstance(c, "item")
	if !ok {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

	// §3.4 step 3 (part 1): an item with no eligible slots is never
	// equippable. Eligible slots are lifted onto the instance at spawn (a
	// single legacy `properties.slot` became the one-element form).
	eligible := item.EligibleSlots()
	if len(eligible) == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("You can't equip %s.", item.Name()))
	}

	// §3.4 step 1 / Decision A: resolve the target slot. Named slot wins;
	// with none named, a sole-eligible item auto-targets, a multi-eligible
	// item asks which (rather than silently mis-targeting).
	slotArg, _ := c.Resolved["slot"].(string)
	slotArg = strings.TrimSpace(slotArg)
	// Bound the player-supplied slot token before the registry lookup so a
	// pathologically long argument can't force a large ToLower allocation
	// on the command path. Real slot names are a handful of characters.
	if len(slotArg) > maxSlotNameLen {
		return c.Actor.Write(ctx, "No such slot.")
	}
	if slotArg == "" {
		if len(eligible) == 1 {
			slotArg = eligible[0]
		} else {
			return c.Actor.Write(ctx, fmt.Sprintf(
				"Which slot? %s can be equipped to: %s.",
				item.Name(), strings.Join(eligible, ", ")))
		}
	}

	def, err := c.Slots.Get(slotArg)
	if err != nil {
		return c.Actor.Write(ctx, fmt.Sprintf("No such slot: %q.", slotArg))
	}

	// §3.4 step 3 (part 2): the item must be eligible for the named slot.
	// Distinct reason from "No such slot" so the player can tell a typo'd
	// slot from a mismatched item.
	if !slot.IsEligible(eligible, def.Name) {
		return c.Actor.Write(ctx, fmt.Sprintf("You can't equip %s in the %s slot.", item.Name(), def.Name))
	}

	// §3.4 step 5: compute the footprint — the target slot plus the item's
	// companion slots — as concrete keys, lowest free index per slot.
	equipped := c.Actor.Equipment()
	occupied := make(map[string]bool, len(equipped))
	for k := range equipped {
		occupied[k] = true
	}
	footprint, err := c.Slots.Footprint(def.Name, item.CompanionSlots(), occupied)
	if err != nil {
		return c.Actor.Write(ctx, fmt.Sprintf("Can't equip to %s right now.", def.Name))
	}

	// §3.4 step 6: determine the items to displace — the DISTINCT items
	// occupying any footprint key (a spanning occupant counts once). A
	// representative key per item lets the commit unequip its whole
	// footprint. Computed WITHOUT mutating so the no-remove guard and the
	// veto can still abort with no state change.
	type displacedEntry struct {
		it       *entities.ItemInstance
		key      string // a footprint key currently mapping to it
		baseName string // its base slot, for the unequip event
	}
	var toDisplace []displacedEntry
	seenDisp := make(map[entities.EntityID]bool)
	for _, k := range footprint {
		id, occ := equipped[k]
		if !occ || seenDisp[id] {
			continue
		}
		seenDisp[id] = true
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		base, _, perr := slot.ParseKey(k)
		if perr != nil {
			base = k
		}
		toDisplace = append(toDisplace, displacedEntry{it: it, key: k, baseName: base})
	}

	// Decision B (§3.4 step 6, §9): structural auto-swap must not force a
	// no-remove item off. If any required eviction targets one, fail the
	// whole equip with no mutation.
	for _, d := range toDisplace {
		if isNoRemove(d.it) {
			return c.Actor.Write(ctx, fmt.Sprintf("You can't remove %s to make room.", d.it.Name()))
		}
	}

	// §3.4 step 7: cancellable pre-equip veto. Published BEFORE any
	// mutation; a veto aborts with the slot, inventory, and displaced
	// items all untouched. This is the seam for policy rules (class/level/
	// curse gates, non-geometric contention) layered outside the engine.
	room := c.Actor.Room()
	var roomID world.RoomID
	if room != nil {
		roomID = room.ID
	}
	holder := holderEntityIDForPlayer(c.Actor.PlayerID())
	if c.PublishCancellable(ctx, eventbus.NewEntityEquipping(holder, item.ID(), roomID, def.Name)) {
		return c.Actor.Write(ctx, fmt.Sprintf("You can't equip %s.", item.Name()))
	}

	// Commit. Displace each occupant first (Unequip frees the occupant's
	// WHOLE footprint, §3.5 step 2), then equip the new item across its
	// footprint. After displacement every footprint key is free.
	for _, d := range toDisplace {
		c.Actor.Unequip(d.key)
	}

	// Translate the item's transient modifier list into the holder-side
	// Modifier form; equip groups them under one EquipmentSourceKey(item)
	// for reversible removal (§3.4 step 9 — applied once per item, not per
	// footprint key).
	mods := make([]stats.Modifier, 0, len(item.Modifiers()))
	for _, m := range item.Modifiers() {
		mods = append(mods, stats.Modifier{Stat: m.Stat, Value: m.Value})
	}
	// Masterwork grade (masterwork §3): a graded WEAPON adds its grade-scaled
	// bonuses while wielded, delivered as stat modifiers under the SAME
	// EquipmentSourceKey so they compose with the item's own modifiers and
	// the combat channels, and reverse cleanly on unequip — no new
	// resolution path. A graded weapon adds to-hit (hit_mod); a power-wrought
	// weapon ALSO adds flat damage (damage_mod, composed into the damage_bonus
	// channel post-crit, masterwork §3). Armor-check / tool-skill grades are
	// later increments.
	if c.Grades != nil {
		if _, isWeapon := item.WeaponDamage(); isWeapon {
			if g, ok := c.Grades.Get(item.Grade()); ok {
				if g.WeaponToHit != 0 {
					mods = append(mods, stats.Modifier{Stat: combat.StatKeyHitMod, Value: g.WeaponToHit})
				}
				if g.WeaponDamage != 0 {
					mods = append(mods, stats.Modifier{Stat: combat.StatKeyDamageMod, Value: g.WeaponDamage})
				}
			}
		}
	}
	// Armor check penalty (armor-depth §6) + masterwork armor grade
	// (masterwork §3): a worn armor/shield's check penalty reduces the
	// wearer's Str/Dex skill checks; a quality grade IMPROVES (reduces) that
	// penalty, floored at zero. Applied as an armor_check stat modifier the
	// skill check subtracts, under the SAME EquipmentSourceKey so it stacks
	// across pieces and reverses cleanly on unequip.
	if penalty := item.ArmorCheckPenalty(); penalty > 0 {
		if c.Grades != nil {
			if g, ok := c.Grades.Get(item.Grade()); ok {
				penalty -= g.ArmorCheckImprove
			}
		}
		if penalty > 0 {
			mods = append(mods, stats.Modifier{Stat: statKeyArmorCheck, Value: penalty})
		}
	}
	// Armor bonus (armor-depth §3): a worn armor/shield's structured AC
	// contribution applies as an `ac` stat modifier under the SAME
	// EquipmentSourceKey — additive, stacking across distinct pieces (body
	// armor + shield), reversing cleanly on unequip. The defense channel reads
	// `ac`, so this makes armor harder-to-hit in EVERY ruleset; the max-Dex
	// cap (WoT only, via the synthetic `dex_ac` channel input) is computed at
	// recompute time, not here.
	if ab := item.ArmorBonus(); ab != 0 {
		mods = append(mods, stats.Modifier{Stat: string(progression.StatAC), Value: ab})
	}

	if !c.Actor.Equip(footprint, item.ID(), mods) {
		// TOCTOU: inventory lost the item between resolve and equip (a
		// concurrent drop). Any displaced items are already back in
		// inventory; tell the player what happened.
		if len(toDisplace) > 0 {
			names := make([]string, 0, len(toDisplace))
			for _, d := range toDisplace {
				names = append(names, d.it.Name())
			}
			return c.Actor.Write(ctx, fmt.Sprintf(
				"You aren't carrying that anymore. (Returned %s to your inventory.)",
				strings.Join(names, ", ")))
		}
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

	// Auto-light on equip (light-and-darkness §3.1): when a source is
	// equipped into the light slot and the policy is on, ignite it so a
	// player who slots a torch sees by it without a second command.
	// Off by default; extinguishing stays explicit to conserve fuel. A
	// spent fuel source (fuel present and zero) is not auto-lit.
	autoLit := false
	if c.Light != nil && c.Light.Config().AutoLightOnEquip && def.Name == "light" &&
		light.IsSource(item) && !light.IsLit(item) {
		spent := false
		if fuel, ok := item.Property(light.PropItemFuel); ok {
			if n, _ := fuel.(int); n <= 0 {
				spent = true
			}
		}
		if !spent {
			item.SetProperty(light.PropItemLit, true)
			autoLit = true
		}
	}

	// User-facing messages. Report each displacement before the equip
	// confirmation so the order matches the mental model; a single equip
	// can now displace more than one item (a companion-bearing item
	// evicting both a worn spanning item and a companion occupant).
	for _, d := range toDisplace {
		_ = c.Actor.Write(ctx, fmt.Sprintf("You stop using %s.", d.it.Name()))
	}
	if autoLit {
		_ = c.Actor.Write(ctx, fmt.Sprintf("You equip %s, and it flares to life.", item.Name()))
	} else {
		_ = c.Actor.Write(ctx, fmt.Sprintf("You equip %s.", item.Name()))
	}

	// weapon-identity §3: the non-proficient to-hit penalty is otherwise
	// silent, so warn when wielding a weapon the actor's class is not
	// proficient with. Checked after Equip so the wielded-weapon snapshot
	// (which IsWeaponProficient reads) reflects the item just equipped; a
	// non-weapon (no damage dice) never triggers it.
	if _, isWeapon := item.WeaponDamage(); isWeapon {
		if wpc, ok := c.Actor.(weaponProficiencyChecker); ok && !wpc.IsWeaponProficient() {
			_ = c.Actor.Write(ctx, fmt.Sprintf(
				"You handle %s clumsily — it is not a weapon you were trained to wield.", item.Name()))
		}
	}
	// armor-depth §5: the non-proficient armor penalty (its check penalty
	// extended to attack rolls) is otherwise silent, so warn when wearing
	// tiered armor the class is not trained in. Checked after Equip so the
	// worn-armor-tier snapshot IsArmorProficient reads reflects the item just
	// equipped; an untiered piece never triggers it.
	if item.ArmorTier() != "" {
		if apc, ok := c.Actor.(armorProficiencyChecker); ok && !apc.IsArmorProficient() {
			_ = c.Actor.Write(ctx, fmt.Sprintf(
				"You wear %s clumsily — it is not armor you were trained to use.", item.Name()))
		}
	}

	// Broadcast uses the base slot name (no :index) per §3.4 step 10.
	if c.Broadcaster != nil && room != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s equips %s.", c.Actor.Name(), item.Name()),
			c.Actor.PlayerID())
	}
	// Auto-swap (§3.4 step 6) emits each displaced item's unequip event
	// before the new placement so observers see removals first.
	for _, d := range toDisplace {
		c.Publish(ctx, eventbus.EntityUnequipped{
			HolderID: holder,
			RoomID:   roomID,
			ItemID:   d.it.ID(),
			SlotName: d.baseName,
		})
	}
	c.Publish(ctx, eventbus.EntityEquipped{
		HolderID: holder,
		RoomID:   roomID,
		ItemID:   item.ID(),
		SlotName: def.Name,
	})
	return nil
}

// maxSlotNameLen caps the player-supplied slot token EquipHandler will
// look up — a defensive bound on the command path (the longest real slot
// name is well under this).
const maxSlotNameLen = 64

// noRemoveTag marks an equipped item that structural auto-swap must not
// forcibly remove (Decision B / spec §3.4 step 6, §9). Hardcoded for now;
// the §8 configuration surface lists it as externalizable once a curse /
// soulbound mechanic ships. No content carries this tag today, so the
// guard is inert in practice — the seam exists for the rules layer.
const noRemoveTag = "no_remove"

// isNoRemove reports whether it carries the no-remove tag.
func isNoRemove(it *entities.ItemInstance) bool {
	for _, t := range it.Tags() {
		if t == noRemoveTag {
			return true
		}
	}
	return false
}

// UnequipHandler implements `unequip <item>` per spec §3.4.
//
// The argument names an equipped item, NOT a slot key — players
// don't think about slot keys. The handler resolves the item via the
// keyword resolver over the equipped set, locates its slot key, and
// calls Actor.Unequip.
//
// Voluntary unequip is intentionally NOT gated by the no_remove tag: that
// guard (§3.4 step 6 / Decision B) only blocks STRUCTURAL auto-swap from
// forcing a cursed item off to make room. A future curse/soulbound
// mechanic that must also block deliberate removal would add its own
// check here.
func UnequipHandler(ctx context.Context, c *Context) error {
	if c.Items == nil {
		return c.Actor.Write(ctx, "You can't unequip anything right now.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Unequip what?")
	}

	equipped := c.Actor.Equipment()
	if len(equipped) == 0 {
		return c.Actor.Write(ctx, "You aren't wearing anything.")
	}

	// Build (slot key, ItemInstance) pairs in deterministic order so
	// keyword resolution against duplicate items (two rings) is stable
	// across calls. Dedupe by id: a spanning item appears under several
	// keys but must be a single resolution candidate (so `2.sword` can't
	// count one two-hander twice). The first (lexically lowest) key wins
	// as the unequip handle; Unequip frees the whole footprint anyway.
	type pair struct {
		key string
		it  *entities.ItemInstance
	}
	keys := sortedSlotKeys(equipped)
	pairs := make([]pair, 0, len(keys))
	items := make([]*entities.ItemInstance, 0, len(keys))
	seen := make(map[entities.EntityID]bool, len(keys))
	for _, k := range keys {
		id := equipped[k]
		if seen[id] {
			continue
		}
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		seen[id] = true
		pairs = append(pairs, pair{key: k, it: it})
		items = append(items, it)
	}
	if len(items) == 0 {
		return c.Actor.Write(ctx, "You aren't wearing anything.")
	}

	match := keyword.Resolve(asNamed(items), strings.Join(c.Args, " "))
	if match == nil {
		return c.Actor.Write(ctx, "You aren't wearing that.")
	}
	target := match.(*entities.ItemInstance)

	var slotKey string
	for _, p := range pairs {
		if p.it.ID() == target.ID() {
			slotKey = p.key
			break
		}
	}
	if slotKey == "" {
		return c.Actor.Write(ctx, "You aren't wearing that.")
	}

	if _, ok := c.Actor.Unequip(slotKey); !ok {
		// Lost a race with a concurrent unequip / cleanup.
		return c.Actor.Write(ctx, "You aren't wearing that.")
	}

	_ = c.Actor.Write(ctx, fmt.Sprintf("You stop using %s.", target.Name()))
	room := c.Actor.Room()
	if c.Broadcaster != nil && room != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s stops using %s.", c.Actor.Name(), target.Name()),
			c.Actor.PlayerID())
	}
	// §3.4 step 4: event carries the BASE slot name, never the
	// index suffix. ParseKey is a pure string operation so a
	// stale slot key still parses; ignore the (rare) error and
	// fall back to the raw key as the base name.
	base, _, err := slot.ParseKey(slotKey)
	if err != nil {
		base = slotKey
	}
	var roomID world.RoomID
	if room != nil {
		roomID = room.ID
	}
	c.Publish(ctx, eventbus.EntityUnequipped{
		HolderID: holderEntityIDForPlayer(c.Actor.PlayerID()),
		RoomID:   roomID,
		ItemID:   target.ID(),
		SlotName: base,
	})
	return nil
}

// sortedSlotKeys returns the keys of m in lexical order. Used to give
// unequip's keyword scan a deterministic candidate ordering.
// Lexical sort puts "finger:0" before "finger:1" and "wield" before
// "wield:1" — good enough for the deterministic-ordering promise. A
// registration-order sort would be better but requires the registry;
// not worth the dependency for M5.6.
func sortedSlotKeys(m map[string]entities.EntityID) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

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
func EquipHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Slots == nil {
		return c.Actor.Write(ctx, "You can't equip anything right now.")
	}
	if len(c.Args) < 2 {
		return c.Actor.Write(ctx, "Equip what, where? Usage: equip <item> <slot>")
	}

	// Slot is the LAST token; everything before is the item phrase, so
	// multi-word item names ("red potion of healing wield") still parse
	// cleanly.
	slotArg := c.Args[len(c.Args)-1]
	itemPhrase := strings.Join(c.Args[:len(c.Args)-1], " ")

	def, err := c.Slots.Get(slotArg)
	if err != nil {
		return c.Actor.Write(ctx, fmt.Sprintf("No such slot: %q.", slotArg))
	}

	candidates := collectItems(c.Items, c.Actor.Inventory())
	if len(candidates) == 0 {
		return c.Actor.Write(ctx, "You aren't carrying anything to equip.")
	}
	match := keyword.Resolve(asNamed(candidates), itemPhrase)
	if match == nil {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	item := match.(*entities.ItemInstance)

	// Determine target sub-slot. For cap-1: the bare name. For cap-N:
	// scan occupancy in registration order (index 0, 1, ...).
	equipped := c.Actor.Equipment()
	targetKey, displacedKey, swap, err := pickSlotKey(def, equipped)
	if err != nil {
		return c.Actor.Write(ctx, fmt.Sprintf("Can't equip to %s right now.", def.Name))
	}

	// Auto-swap: unequip the displaced item first so the slot is empty
	// when Equip runs. Failure here aborts before any state change.
	var displacedItem *entities.ItemInstance
	if swap {
		displacedID, ok := c.Actor.Unequip(displacedKey)
		if !ok {
			// Slot was reported occupied by Equipment() but unequip
			// found nothing — concurrent unequip raced us. Re-resolve
			// the slot key as if it were empty and proceed.
			if newKey, err := slot.BuildKey(def.Name, 0, def.Max); err == nil {
				targetKey = newKey
			}
		} else if e, ok := c.Items.GetByID(displacedID); ok {
			if it, ok := e.(*entities.ItemInstance); ok {
				displacedItem = it
			}
		}
	}

	// Translate the item's transient modifier list into the
	// holder-side Modifier form. The InstanceModifier.Source field
	// (set at Spawn to "entity:<id>") is dropped here — equip groups
	// the whole set under one EquipmentSourceKey(item.ID()) for
	// reversible removal at unequip time (§3.3 step 6).
	mods := make([]stats.Modifier, 0, len(item.Modifiers()))
	for _, m := range item.Modifiers() {
		mods = append(mods, stats.Modifier{Stat: m.Stat, Value: m.Value})
	}

	if !c.Actor.Equip(targetKey, item.ID(), mods) {
		// Inventory lost the item between resolve and equip — likely a
		// concurrent drop. If we did an auto-swap unequip, the
		// displaced item is now sitting in inventory; tell the player
		// what happened.
		if displacedItem != nil {
			return c.Actor.Write(ctx,
				fmt.Sprintf("You aren't carrying that anymore. (Returned %s to your inventory.)",
					displacedItem.Name()))
		}
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

	// User-facing messages. Auto-swap reports the displacement before
	// the equip confirmation so the order matches the mental model.
	if displacedItem != nil {
		_ = c.Actor.Write(ctx, fmt.Sprintf("You stop using %s.", displacedItem.Name()))
	}
	_ = c.Actor.Write(ctx, fmt.Sprintf("You equip %s.", item.Name()))

	// Broadcast uses the base slot name (no :index) per §3.3 step 7.
	if c.Broadcaster != nil && c.Actor.Room() != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, c.Actor.Room().ID,
			fmt.Sprintf("%s equips %s.", c.Actor.Name(), item.Name()),
			c.Actor.PlayerID())
	}
	return nil
}

// UnequipHandler implements `unequip <item>` per spec §3.4.
//
// The argument names an equipped item, NOT a slot key — players
// don't think about slot keys. The handler resolves the item via the
// keyword resolver over the equipped set, locates its slot key, and
// calls Actor.Unequip.
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
	// keyword resolution against duplicate items (two rings) is
	// stable across calls.
	type pair struct {
		key string
		it  *entities.ItemInstance
	}
	keys := sortedSlotKeys(equipped)
	pairs := make([]pair, 0, len(keys))
	items := make([]*entities.ItemInstance, 0, len(keys))
	for _, k := range keys {
		e, ok := c.Items.GetByID(equipped[k])
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
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
	if c.Broadcaster != nil && c.Actor.Room() != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, c.Actor.Room().ID,
			fmt.Sprintf("%s stops using %s.", c.Actor.Name(), target.Name()),
			c.Actor.PlayerID())
	}
	return nil
}

// pickSlotKey returns the target key for equip and, if the slot is
// full, the key of the displaced occupant. swap is true when an
// auto-swap is needed (§3.3 step 3). For cap-1 slots: target is the
// bare name; if occupied, the same name is also the displaced key.
// For cap-N slots: prefer the lowest unoccupied index; if all
// indices are occupied, displace index 0 and target index 0.
func pickSlotKey(def slot.Def, equipped map[string]entities.EntityID) (target, displaced string, swap bool, err error) {
	if def.Max <= 0 {
		return "", "", false, slot.ErrInvalidMax
	}
	if def.Max == 1 {
		key, kerr := slot.BuildKey(def.Name, 0, def.Max)
		if kerr != nil {
			return "", "", false, kerr
		}
		if _, occupied := equipped[key]; occupied {
			return key, key, true, nil
		}
		return key, "", false, nil
	}
	for i := 0; i < def.Max; i++ {
		key, kerr := slot.BuildKey(def.Name, i, def.Max)
		if kerr != nil {
			return "", "", false, kerr
		}
		if _, occupied := equipped[key]; !occupied {
			return key, "", false, nil
		}
	}
	// All indices full: displace index 0.
	key, kerr := slot.BuildKey(def.Name, 0, def.Max)
	if kerr != nil {
		return "", "", false, kerr
	}
	return key, key, true, nil
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

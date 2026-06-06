package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stats"
	"github.com/Jasrags/AnotherMUD/internal/world"
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

	// M17.2d₃: `equip <item> <slot>` declares item (ArgInventory) then
	// slot (ArgKeyword). Both are resolved by the §5 pipeline before
	// this runs — note this flips the old precedence (the hand-parsed
	// form validated the slot first); a not-carried item now reports
	// "You aren't carrying that." before the slot is examined. The slot
	// keyword arrives as a raw string and is validated against the slot
	// registry here (the keyword resolver does not know slot names).
	// Single-token item references only (the multi-word item phrase the
	// old trailing-slot parse allowed is gone).
	slotArg, _ := c.Resolved["slot"].(string)
	def, err := c.Slots.Get(slotArg)
	if err != nil {
		return c.Actor.Write(ctx, fmt.Sprintf("No such slot: %q.", slotArg))
	}

	item, ok := resolvedItemInstance(c, "item")
	if !ok {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

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

	// User-facing messages. Auto-swap reports the displacement before
	// the equip confirmation so the order matches the mental model.
	if displacedItem != nil {
		_ = c.Actor.Write(ctx, fmt.Sprintf("You stop using %s.", displacedItem.Name()))
	}
	if autoLit {
		_ = c.Actor.Write(ctx, fmt.Sprintf("You equip %s, and it flares to life.", item.Name()))
	} else {
		_ = c.Actor.Write(ctx, fmt.Sprintf("You equip %s.", item.Name()))
	}

	// Broadcast uses the base slot name (no :index) per §3.3 step 7.
	room := c.Actor.Room()
	if c.Broadcaster != nil && room != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s equips %s.", c.Actor.Name(), item.Name()),
			c.Actor.PlayerID())
	}
	var roomID world.RoomID
	if room != nil {
		roomID = room.ID
	}
	holder := holderEntityIDForPlayer(c.Actor.PlayerID())
	// Auto-swap (§3.3 step 3) emits its unequip event first so
	// observers see the displaced removal before the new placement.
	if displacedItem != nil {
		c.Publish(ctx, eventbus.EntityUnequipped{
			HolderID: holder,
			RoomID:   roomID,
			ItemID:   displacedItem.ID(),
			SlotName: def.Name,
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

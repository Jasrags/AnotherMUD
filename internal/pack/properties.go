package pack

import (
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/property"
)

// RegisterEngineBaselineProperties seeds the engine-known property keys
// into reg. These are the reserved content-property names the room/item
// loaders validate against (an unregistered property in a content file
// is a load error). Defined here — in the package that owns the
// property registry and runs the validating loader — so the composition
// root and the loader tests register the same set rather than drifting
// from a copy in main. Pack-scoped properties belong in their owning
// feature's boot code via Registry.RegisterPack.
//
// nil reg is a no-op (tests that don't validate properties).
func RegisterEngineBaselineProperties(reg *property.Registry) error {
	if reg == nil {
		return nil
	}
	baseline := []property.Entry{
		{
			Name:          "quest_grant",
			Type:          property.TypeString,
			Description:   "Quest id auto-accepted on item pickup or room entry (spec quests §7.2).",
			AppliesTo:     []string{"item", "room"},
			AdminSettable: true,
		},
		{
			Name:          "key_for",
			Type:          property.TypeString,
			Description:   "Door id this item unlocks (spec world-rooms-movement §5.3 + PD-4).",
			AppliesTo:     []string{"item"},
			AdminSettable: true,
		},
		{
			Name:          "rarity",
			Type:          property.TypeString,
			Description:   "Rarity-tier key decorating the item's display (spec item-decorations §5).",
			AppliesTo:     []string{"item"},
			AdminSettable: true,
		},
		{
			Name:          "essence",
			Type:          property.TypeString,
			Description:   "Essence key decorating the item's display (spec item-decorations §5).",
			AppliesTo:     []string{"item"},
			AdminSettable: true,
		},
		{
			Name:          "light",
			Type:          property.TypeString,
			Description:   "Light level (black/gloom/dim/lit): on a room it overrides ambient; on an item it is the level the source contributes when lit (spec light-and-darkness §2.4/§3.1/§9).",
			AppliesTo:     []string{"room", "item"},
			AdminSettable: true,
		},
		{
			Name:          "light_floor",
			Type:          property.TypeString,
			Description:   "Light floor (black/gloom/dim/lit) that lifts a room's dark ambient without capping daylight — the lamp-lit settlement knob; an area-level light_floor bakes onto member rooms at load (spec light-and-darkness §2.4/§9).",
			AppliesTo:     []string{"room"},
			AdminSettable: true,
		},
		{
			Name:          "lit",
			Type:          property.TypeBool,
			Description:   "Light source lit state; lives on the item instance so it survives pickup/drop/give/store (spec light-and-darkness §3.1).",
			AppliesTo:     []string{"item"},
			AdminSettable: true,
		},
		{
			Name:          "fuel",
			Type:          property.TypeInt,
			Description:   "Remaining fuel for a fuel-burning source; absent = permanent, zero = spent (spec light-and-darkness §3.2).",
			AppliesTo:     []string{"item"},
			AdminSettable: true,
		},
		{
			Name:          "dark_blocked",
			Type:          property.TypeBool,
			Description:   "Room opts into the darkness-movement hazard: a mover who cannot see it at all (effective black) is refused entry (spec light-and-darkness §5.4).",
			AppliesTo:     []string{"room"},
			AdminSettable: true,
		},
		{
			Name:        "craft_stations",
			Type:        property.TypeMapInt,
			Description: "Per-discipline crafting station tier this room provides (discipline → tier); gates craft attempts + sets the quality ceiling (spec crafting-and-cooking §4).",
			AppliesTo:   []string{"room"},
		},
	}
	for _, e := range baseline {
		if err := reg.RegisterEngine(e); err != nil {
			return fmt.Errorf("baseline property %q: %w", e.Name, err)
		}
	}
	return nil
}

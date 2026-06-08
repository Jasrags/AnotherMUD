package command

import (
	"context"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Crafting station property keys (crafting-and-cooking §4). A room declares
// a per-discipline station tier map; a portable tool item declares the
// discipline it serves and the field tier it grants.
const (
	propCraftStations = "craft_stations"  // room: map[discipline]tier
	propCraftTool     = "craft_tool"      // item: discipline string it serves
	propCraftToolTier = "craft_tool_tier" // item: field tier (default 1)
)

// CraftHandler implements `craft [<recipe>]` (crafting-and-cooking §3, §4).
// With no argument it lists the recipes the player knows; with one it
// resolves a known recipe and routes to the crafting service. The present
// station tier (room station ∪ carried tools) gates the attempt and sets
// the quality ceiling.
func CraftHandler(ctx context.Context, c *Context) error {
	if c.Craft == nil {
		return c.Actor.Write(ctx, "Crafting is not enabled in this build.")
	}

	entityID := c.Actor.PlayerID()
	if entityID == "" {
		entityID = c.Actor.ID()
	}

	// No-arg form: list what the player can make.
	if len(c.Args) == 0 {
		names := c.Craft.KnownRecipeNames(entityID)
		if len(names) == 0 {
			return c.Actor.Write(ctx, "You don't know any recipes. Find a trainer to learn a craft.")
		}
		sort.Strings(names)
		return c.Actor.Write(ctx, "You know how to craft:\n  "+strings.Join(names, "\n  "))
	}

	query := strings.Join(c.Args, " ")
	res := c.Craft.Craft(ctx, c.Actor, query, func(discipline string) int {
		return craftStationTier(c, discipline)
	})
	return c.Actor.Write(ctx, res.Message)
}

// craftStationTier reports the present station tier for discipline at the
// crafter's location: the max of the room's declared station tier and any
// portable tool the crafter carries for that discipline (§4).
func craftStationTier(c *Context, discipline string) int {
	discipline = strings.ToLower(strings.TrimSpace(discipline))
	tier := 0

	room := c.Actor.Room()
	if room != nil {
		if raw, ok := room.Property(propCraftStations); ok {
			if t := disciplineTier(raw, discipline); t > tier {
				tier = t
			}
		}
	}

	// Ground-placed station entities (a built campfire, §4): an item in the
	// room carrying craft_stations contributes its tier, symmetric with a
	// fixed room station.
	if room != nil && c.Items != nil && c.Placement != nil {
		for _, id := range c.Placement.InRoom(room.ID) {
			e, ok := c.Items.GetByID(id)
			if !ok {
				continue
			}
			it, ok := e.(*entities.ItemInstance)
			if !ok {
				continue
			}
			if raw, ok := it.Property(propCraftStations); ok {
				if t := disciplineTier(raw, discipline); t > tier {
					tier = t
				}
			}
		}
	}

	if c.Items != nil {
		for _, id := range c.Actor.Inventory() {
			e, ok := c.Items.GetByID(id)
			if !ok {
				continue
			}
			it, ok := e.(*entities.ItemInstance)
			if !ok {
				continue
			}
			served, ok := it.Property(propCraftTool)
			if !ok {
				continue
			}
			s, ok := served.(string)
			if !ok || !strings.EqualFold(strings.TrimSpace(s), discipline) {
				continue
			}
			t := craftPropInt(it, propCraftToolTier)
			if t <= 0 {
				t = 1 // a tool grants Tier 1 by default (§4 "one tier up")
			}
			if t > tier {
				tier = t
			}
		}
	}
	return tier
}

// disciplineTier reads a discipline's tier out of a room's craft_stations
// property, tolerating the map shapes yaml.v3 produces.
func disciplineTier(raw any, discipline string) int {
	switch m := raw.(type) {
	case map[string]any:
		return toInt(m[discipline])
	case map[string]int:
		return m[discipline]
	case map[any]any:
		for k, v := range m {
			if ks, ok := k.(string); ok && strings.ToLower(strings.TrimSpace(ks)) == discipline {
				return toInt(v)
			}
		}
	}
	return 0
}

func craftPropInt(it *entities.ItemInstance, key string) int {
	v, _ := it.Property(key)
	return toInt(v)
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

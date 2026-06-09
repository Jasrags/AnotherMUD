package command

import (
	"context"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/crafting"
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
	stationFn := func(discipline string) int { return craftStationTier(c, discipline) }

	// Timed crafts (recipe time_pulses > 0) occupy the player: BeginCraft
	// runs the read-only gates, and a successful start arms the
	// craft-complete tick to finish it later. Instant crafts (time_pulses
	// <= 0, or an actor that doesn't model occupation) fall through to the
	// synchronous Craft path unchanged (crafting-and-cooking §3).
	busy, canOccupy := c.Actor.(crafting.CraftBusy)
	if canOccupy && c.NowTick != nil {
		if pending, inFlight := busy.PendingCraft(); inFlight {
			return c.Actor.Write(ctx, "You're still busy trying to "+pending.DisplayName+".")
		}
		begin := c.Craft.BeginCraft(ctx, c.Actor, query, stationFn)
		if begin.Outcome != crafting.CraftOK {
			return c.Actor.Write(ctx, begin.Message)
		}
		if begin.TimePulses > 0 {
			return c.beginTimedCraft(ctx, busy, begin)
		}
	}

	res := c.Craft.Craft(ctx, c.Actor, query, stationFn)
	return c.Actor.Write(ctx, res.Message)
}

// beginTimedCraft arms the occupation timer for a craft that passed
// BeginCraft's gates and announces the start. The craft-complete tick
// finishes it when the engine tick reaches ReadyAt.
func (c *Context) beginTimedCraft(ctx context.Context, busy crafting.CraftBusy, begin crafting.BeginCraftResult) error {
	readyAt := c.NowTick() + uint64(begin.TimePulses)
	if !busy.SetPendingCraft(crafting.PendingCraft{
		RecipeID:    begin.RecipeID,
		ReadyAt:     readyAt,
		StationTier: begin.PresentTier,
		DisplayName: begin.DisplayName,
	}) {
		// Lost a race to another craft start — treat as already busy.
		return c.Actor.Write(ctx, "You're already hard at work on something.")
	}
	if room := c.Actor.Room(); room != nil && c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			c.Actor.Name()+" sets to work.", c.Actor.PlayerID())
	}
	return c.Actor.Write(ctx, "You begin to "+begin.DisplayName+".")
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

package command

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/campfire"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// fuelTag marks an item that can feed a built fire (crafting-and-cooking
// §4 "consuming fuel/materials"). The build verb consumes one.
const fuelTag = "fuel"

// wetWeather is the set of weather states that won't let a fire catch. Only
// "rain" exists in the core zone today; the rest are forward-compat.
var wetWeather = map[string]bool{
	"rain": true, "storm": true, "snow": true, "sleet": true, "drizzle": true,
}

// BuildHandler implements `build <thing>` (crafting-and-cooking §4). Today
// the only buildable is a campfire: an improvised Tier-1 station placed in
// the room, refused indoors/underground or in wet weather, consuming a unit
// of fuel, and decaying after a TTL (swept by the campfire-decay tick).
func BuildHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Build what? (try: build campfire)")
	}
	what := strings.ToLower(strings.TrimSpace(c.Args[0]))
	if what != "campfire" && what != "fire" {
		return c.Actor.Write(ctx, "You don't know how to build that.")
	}
	if c.Items == nil || c.Placement == nil {
		return c.Actor.Write(ctx, "You can't build anything right now.")
	}

	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You are nowhere a fire could sit.")
	}

	// Terrain gate (§4): no fire indoors or underground.
	if world.IsShielded(room) {
		return c.Actor.Write(ctx, "There's no safe place for a fire here.")
	}

	// Weather gate (§4): no fire in the rain.
	if c.WeatherState != nil && wetWeather[strings.ToLower(c.WeatherState(room.AreaID))] {
		return c.Actor.Write(ctx, "The weather won't let a fire catch.")
	}

	// One fire at a time per room.
	if campfireInRoom(c, room.ID) {
		return c.Actor.Write(ctx, "There is already a fire burning here.")
	}

	// Fuel gate (§4): take one unit of fuel from the pack, but DON'T
	// destroy it until the fire is actually placed — same loss-free
	// ordering as the craft path. On a placement failure the fuel is
	// re-added (it was only removed from the bag, never untracked).
	fuelID, ok := findFuel(c)
	if !ok {
		return c.Actor.Write(ctx, "You need some firewood to build a fire.")
	}
	if !c.Actor.RemoveFromInventory(fuelID) {
		return c.Actor.Write(ctx, "You fumble your firewood.")
	}

	now := uint64(0)
	if c.NowTick != nil {
		now = c.NowTick()
	}
	if _, err := campfire.Place(c.Items, c.Placement, room.ID, now); err != nil {
		c.Actor.AddToInventory(fuelID) // rollback: fuel still live, just back in the bag
		return c.Actor.Write(ctx, "The fire won't catch.")
	}
	_ = c.Items.Untrack(fuelID) // destroy the fuel only now that the fire exists

	if c.Broadcaster != nil {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			c.Actor.Name()+" builds a campfire; it crackles to life.", c.Actor.PlayerID())
	}
	return c.Actor.Write(ctx, "You build a campfire; it crackles to life. You can cook here now.")
}

// campfireInRoom reports whether a campfire is already placed in roomID.
func campfireInRoom(c *Context, roomID world.RoomID) bool {
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		if it, ok := e.(*entities.ItemInstance); ok && hasTag(it.Tags(), campfire.Tag) {
			return true
		}
	}
	return false
}

// findFuel returns the first fuel-tagged item in the crafter's inventory.
func findFuel(c *Context) (entities.EntityID, bool) {
	for _, id := range c.Actor.Inventory() {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		if it, ok := e.(*entities.ItemInstance); ok && hasTag(it.Tags(), fuelTag) {
			return id, true
		}
	}
	return "", false
}

// hasTag reports whether tag is in tags (case-insensitive).
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

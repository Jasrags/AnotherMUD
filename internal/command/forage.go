package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/gathering"
)

// ForageHandler implements `forage` (gathering.md §2): ambient gathering
// from the current room's biome. It resolves the room biome → its forage
// table, fires the cancellable resource.gathering event, and routes through
// the gathering service (cooldown gate → quality roll → yield). A biome
// with no forage table reports "nothing to forage here" — absence of a
// source, NOT a punishing refusal (§1.1). Never refused for lack of a tool.
func ForageHandler(ctx context.Context, c *Context) error {
	if c.Gathering == nil || c.Biomes == nil || c.ForageTables == nil {
		return c.Actor.Write(ctx, "You can't forage right now.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "There's nowhere to forage here.")
	}

	// Resolve the room's biome → its forage table. No biome, no table, or an
	// unregistered table id all mean "nothing grows here to gather" (§2.1
	// step 1 — absence, not a refusal).
	b, ok := c.Biomes.Resolve(room.Terrain)
	if !ok || b.ForageTable == "" {
		return c.Actor.Write(ctx, "There's nothing to forage here.")
	}
	table, ok := c.ForageTables.Get(b.ForageTable)
	if !ok {
		return c.Actor.Write(ctx, "There's nothing to forage here.")
	}

	gatherer, ok := c.Actor.(gathering.Gatherer)
	if !ok {
		return c.Actor.Write(ctx, "You can't forage right now.")
	}

	// Cancellable resource.gathering (§6): content may forbid gathering in a
	// protected/quest-gated spot. A veto aborts with a generic line.
	pre := eventbus.NewResourceGathering(c.Actor.PlayerID(), room.ID, "forage", b.ID, "")
	if c.PublishCancellable(ctx, pre) {
		return c.Actor.Write(ctx, "Something stays your hand.")
	}

	now := uint64(0)
	if c.NowTick != nil {
		now = c.NowTick()
	}
	res := c.Gathering.Forage(ctx, gatherer, table, now)
	switch res.Outcome {
	case gathering.ForageOK:
		// resource.gathered (§6): the quest advance-on-event hook.
		c.Publish(ctx, eventbus.ResourceGathered{
			ActorID: c.Actor.PlayerID(), RoomID: room.ID,
			Source: "forage", Biome: b.ID,
			Items: []string{res.ItemID}, Tiers: []string{res.QualityKey},
		})
		return c.Actor.Write(ctx, "You forage "+res.ItemName+".")
	case gathering.ForageCoolingDown:
		return c.Actor.Write(ctx, "You've picked this area over; give it time to recover.")
	default:
		return c.Actor.Write(ctx, "You find nothing worth gathering.")
	}
}

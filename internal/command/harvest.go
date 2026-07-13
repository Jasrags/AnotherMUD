package command

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/gathering"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// HarvestHandler implements `harvest <node>` (gathering.md §3.2): harvest a
// resource node in the room (an ore vein, a tree). Resolves the node by
// keyword, fires the cancellable resource.gathering event, enforces the
// node's tool requirement (§3.3 — the one allowed refusal), rolls the
// yield, decrements the node's charges, and removes a depleted node
// (firing node.depleted; the spawn scheduler respawns it).
func HarvestHandler(ctx context.Context, c *Context) error {
	if c.Gathering == nil || c.ForageTables == nil || c.Items == nil || c.Placement == nil {
		return c.Actor.Write(ctx, "You can't harvest right now.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Harvest what?")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "There's nothing to harvest here.")
	}

	node := c.resolveNodeInRoom(room.ID, strings.Join(c.Args, " "))
	if node == nil {
		return c.Actor.Write(ctx, "There's nothing like that to harvest here.")
	}
	gatherer, ok := c.Actor.(gathering.Gatherer)
	if !ok {
		return c.Actor.Write(ctx, "You can't harvest right now.")
	}

	yieldID, _ := node.Property(gathering.PropNodeYieldTable)
	yieldStr, _ := yieldID.(string)
	yield, ok := c.ForageTables.Get(yieldStr)
	if !ok {
		return c.Actor.Write(ctx, "There's nothing to gather from that.")
	}

	biomeID := ""
	if c.Biomes != nil {
		if b, ok := c.Biomes.Resolve(room.Terrain); ok {
			biomeID = b.ID
		}
	}

	// Cancellable resource.gathering (§6, source=node).
	pre := eventbus.NewResourceGathering(c.Actor.PlayerID(), room.ID, "node", biomeID, string(node.ID()))
	if c.PublishCancellable(ctx, pre) {
		return c.Actor.Write(ctx, "Something stays your hand.")
	}

	res := c.Gathering.Harvest(ctx, gatherer, node, yield)
	switch res.Outcome {
	case gathering.HarvestOK:
		c.Publish(ctx, eventbus.ResourceGathered{
			ActorID: c.Actor.PlayerID(), RoomID: room.ID,
			Source: "node", Biome: biomeID,
			Items: []string{res.ItemID}, Tiers: []string{res.QualityKey},
		})
		if res.Depleted {
			// Remove the node from the world; the §3.6 reset algorithm
			// respawns it after its interval (the tracker sees it gone).
			c.Placement.Remove(node.ID())
			_ = c.Items.Untrack(node.ID())
			c.Publish(ctx, eventbus.NodeDepleted{RoomID: room.ID, Node: string(node.ID()), Biome: biomeID})
			return c.Actor.Write(ctx, "You harvest "+res.ItemName+". "+capitalize(node.Name())+" is exhausted.")
		}
		return c.Actor.Write(ctx, "You harvest "+res.ItemName+".")
	case gathering.HarvestNeedsTool:
		return c.Actor.Write(ctx, "You need "+article(res.RequiredTool)+res.RequiredTool+" to harvest that.")
	case gathering.HarvestNoCharges:
		return c.Actor.Write(ctx, capitalize(node.Name())+" has nothing left to give.")
	default:
		return c.Actor.Write(ctx, "You can't gather anything from that.")
	}
}

// resolveNodeInRoom finds a resource-node entity in roomID matching query
// by the shared keyword rules. Only NodeTag-tagged item instances are
// candidates, so `harvest vein` ignores ordinary room items.
func (c *Context) resolveNodeInRoom(roomID world.RoomID, query string) *entities.ItemInstance {
	cands := make([]keyword.Named, 0, 4)
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		if c.questSpawnBlockedFrom(e) {
			continue // foreign quest spawn — not interactable (quest-spawns.md Phase 2)
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok || !hasTag(it.Tags(), gathering.NodeTag) {
			continue
		}
		cands = append(cands, it)
	}
	if m := keyword.Resolve(cands, query); m != nil {
		return m.(*entities.ItemInstance)
	}
	return nil
}

// article returns "a " / "an " for a tool-name prompt, or "" when blank.
func article(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	switch s[0] {
	case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
		return "an "
	default:
		return "a "
	}
}

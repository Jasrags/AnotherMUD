package command

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// SpawnService is the admin builder-spawn pipeline (admin-verbs §5 builder
// tooling — the creation counterpart to purge). It mints an item or mob from a
// content template directly into the world, identical to a naturally-spawned
// one: an item runs the store's build path; a mob runs the full spawn pipeline
// (racial flags, class growth, loot, equipment). Implemented at the composition
// root over the same bootSpawner the pack loader and area-reset use, so a
// hand-spawned entity is indistinguishable from a rule-spawned one. nil
// disables the `spawn` verb (tests / headless).
type SpawnService interface {
	// SpawnMob mints a mob from templateID into roomID (the full mob spawn
	// pipeline: placement + mob.spawned event) and returns the live entity id +
	// display name. Error on an unknown template.
	SpawnMob(ctx context.Context, templateID string, roomID world.RoomID) (entities.EntityID, string, error)
	// SpawnItem mints an item from templateID into the entity store WITHOUT
	// placing it, returning the new item id + display name. The caller files it
	// into a room (Placement) or an inventory (Actor.AddToInventory). Error on
	// an unknown template.
	SpawnItem(ctx context.Context, templateID string) (entities.EntityID, string, error)
}

// errNoSpawn is the initial error the candidate-resolution loops carry so an
// empty candidate list (never happens — always ≥1) still reads as "unresolved".
var errNoSpawn = errors.New("no spawn candidate resolved")

// roomNamespace returns the pack namespace of the actor's current room (the
// prefix before ':' in a namespaced room id, e.g. "wot" from "wot:the-forge").
// Empty when there's no room or the id carries no namespace.
func roomNamespace(c *Context) string {
	room := c.Actor.Room()
	if room == nil {
		return ""
	}
	if i := strings.IndexByte(string(room.ID), ':'); i > 0 {
		return string(room.ID)[:i]
	}
	return ""
}

// spawnCandidates lists the template ids to try for a spawn request, in order.
// It mirrors the engine's content-id rule: a fully-qualified id ("pack:foo") is
// used verbatim; a bare id is tried as-typed first, then qualified against the
// current room's pack namespace ("wot:foo") — since pack loading namespaces all
// template ids, the qualified form is what actually resolves for bare input.
func spawnCandidates(templateID, ns string) []string {
	if strings.Contains(templateID, ":") || ns == "" {
		return []string{templateID}
	}
	return []string{templateID, ns + ":" + templateID}
}

// spawnUsage is the self-documenting panel a bare or malformed `spawn` prints,
// mirroring the `set` usage convention (admin-verbs §4).
const spawnUsage = "Spawn what?\n" +
	"  spawn item <template-id> [here|me]   (default: into your hands)\n" +
	"  spawn mob  <template-id>             (into this room)\n" +
	"  spawn gold <amount>                  (into your purse)\n" +
	"Template ids are namespaced (e.g. wot:short-sword) or bare within the active pack."

// SpawnHandler implements `spawn <item|mob|gold> …` (admin-verbs §5 builder
// tooling): conjure an item, mob, or currency into the world. Admin-marked
// (the M19.3 dispatch gate) and audited via the auditAdmin choke point, exactly
// like its removal counterpart, purge. A bare or unknown kind renders the usage
// panel rather than failing silently.
func SpawnHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, spawnUsage)
	}
	rest := c.Args[1:]
	switch strings.ToLower(c.Args[0]) {
	case "item", "obj", "object":
		return c.spawnItemHere(ctx, rest)
	case "mob", "npc", "creature", "monster":
		return c.spawnMobHere(ctx, rest)
	case "gold", "coin", "coins", "currency", "money":
		return c.spawnGold(ctx, rest)
	default:
		return c.Actor.Write(ctx, spawnUsage)
	}
}

// spawnItemHere mints `spawn item <id> [here|me]`. The optional trailing
// destination chooses the room floor (here/room/floor) or the actor's inventory
// (me/inv/bag); inventory is the default, since a builder usually wants the item
// in hand to place or hand off.
func (c *Context) spawnItemHere(ctx context.Context, args []string) error {
	if c.Spawn == nil {
		return c.Actor.Write(ctx, "Spawning is not available.")
	}
	if len(args) == 0 {
		return c.Actor.Write(ctx, "Spawn which item?  (spawn item <template-id> [here|me])")
	}
	templateID := args[0]
	toRoom := false
	if len(args) > 1 {
		switch strings.ToLower(args[1]) {
		case "here", "room", "floor", "ground":
			toRoom = true
		case "me", "inv", "inventory", "bag", "hand", "hands":
			toRoom = false
		default:
			return c.Actor.Write(ctx, fmt.Sprintf("Spawn %q where?  (here or me)", args[1]))
		}
	}

	var (
		id   entities.EntityID
		name string
		err  error = errNoSpawn
	)
	for _, cand := range spawnCandidates(templateID, roomNamespace(c)) {
		if id, name, err = c.Spawn.SpawnItem(ctx, cand); err == nil {
			break
		}
	}
	if err != nil {
		return c.Actor.Write(ctx, fmt.Sprintf("No item template %q.", templateID))
	}

	if toRoom {
		room := c.Actor.Room()
		if room == nil || c.Placement == nil {
			// The item is already minted; without a room to hold it, fall back to
			// the actor's inventory rather than leak an unplaced instance.
			c.Actor.AddToInventory(id)
			auditAdmin(ctx, c, "spawn", string(id), "item:"+templateID+"@inv")
			return c.Actor.Write(ctx, fmt.Sprintf("You conjure %s into your hands.", name))
		}
		c.Placement.Place(id, room.ID)
		if c.Broadcaster != nil {
			c.Broadcaster.SendToRoom(ctx, room.ID,
				fmt.Sprintf("%s appears on the ground.", capitalize(name)), c.Actor.PlayerID())
		}
		auditAdmin(ctx, c, "spawn", string(id), "item:"+templateID+"@room")
		return c.Actor.Write(ctx, fmt.Sprintf("You conjure %s onto the ground.", name))
	}

	c.Actor.AddToInventory(id)
	auditAdmin(ctx, c, "spawn", string(id), "item:"+templateID+"@inv")
	return c.Actor.Write(ctx, fmt.Sprintf("You conjure %s into your hands.", name))
}

// spawnMobHere mints `spawn mob <id>` into the actor's current room, running the
// full mob spawn pipeline through the service.
func (c *Context) spawnMobHere(ctx context.Context, args []string) error {
	if c.Spawn == nil {
		return c.Actor.Write(ctx, "Spawning is not available.")
	}
	if len(args) == 0 {
		return c.Actor.Write(ctx, "Spawn which mob?  (spawn mob <template-id>)")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You're nowhere to spawn a mob.")
	}
	templateID := args[0]
	var (
		id   entities.EntityID
		name string
		err  error = errNoSpawn
	)
	for _, cand := range spawnCandidates(templateID, roomNamespace(c)) {
		if id, name, err = c.Spawn.SpawnMob(ctx, cand, room.ID); err == nil {
			break
		}
	}
	if err != nil {
		return c.Actor.Write(ctx, fmt.Sprintf("No mob template %q.", templateID))
	}
	if c.Broadcaster != nil {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s appears in a shimmer of air.", capitalize(name)), c.Actor.PlayerID())
	}
	auditAdmin(ctx, c, "spawn", string(id), "mob:"+templateID)
	return c.Actor.Write(ctx, fmt.Sprintf("You spawn %s.", name))
}

// spawnGold mints `spawn gold <amount>` into the actor's purse through the
// authoritative currency service (ADD, not set — `set gold amount` is the
// absolute-write counterpart). Amount must be a positive integer.
func (c *Context) spawnGold(ctx context.Context, args []string) error {
	holder, ok := c.Actor.(economy.Entity)
	if !ok || c.Currency == nil {
		return c.Actor.Write(ctx, "You can't hold currency right now.")
	}
	if len(args) == 0 {
		return c.Actor.Write(ctx, "Spawn how much gold?  (spawn gold <amount>)")
	}
	amount, err := strconv.Atoi(args[0])
	if err != nil || amount <= 0 {
		return c.Actor.Write(ctx, "Spawn how much gold?  (a positive whole number)")
	}
	balance := c.Currency.AddGold(ctx, holder, amount, "spawn")
	auditAdmin(ctx, c, "spawn", c.Actor.PlayerID(), fmt.Sprintf("gold:%d", amount))
	return c.Actor.Write(ctx, fmt.Sprintf("You conjure %d gold. (You now have %d.)", amount, balance))
}

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

// maxSpawnCount caps a single multi-mint so a fat-fingered count can't flood a
// room or the entity store (admin-verbs §5 — a builder guardrail, not a balance
// knob). A magic value externalized as a named constant per coding-style.
const maxSpawnCount = 100

// countable renders a name with an optional multiplier suffix. For a single
// entity it returns the name verbatim (so count==1 output is byte-identical to
// the pre-count behavior — the exact strings existing callers/tests rely on);
// for n>1 it appends "(xN)". The article baked into item names ("a short
// sword") makes a suffix cleaner than pluralization.
func countable(n int, name string) string {
	if n <= 1 {
		return name
	}
	return fmt.Sprintf("%s (x%d)", name, n)
}

// parseSpawnCount pulls an optional positive-integer count out of args. It scans
// for the first all-digits token so callers can accept the count in any position
// relative to a keyword (e.g. a destination). It returns the count (default 1),
// args with that token removed, and ok=false when a numeric token is present but
// out of the 1..maxSpawnCount range — letting the caller reject it explicitly
// rather than silently clamp.
func parseSpawnCount(args []string) (count int, rest []string, ok bool) {
	for i, a := range args {
		n, err := strconv.Atoi(a)
		if err != nil {
			continue
		}
		if n < 1 || n > maxSpawnCount {
			return 0, nil, false
		}
		rest = append(append([]string{}, args[:i]...), args[i+1:]...)
		return n, rest, true
	}
	return 1, args, true
}

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
	"  spawn item <template-id> [count] [here|me]   (default: 1 into your hands)\n" +
	"  spawn mob  <template-id> [count]             (into this room)\n" +
	"  spawn gold <amount>                          (into your purse)\n" +
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

// spawnItemHere mints `spawn item <id> [count] [here|me]`. The optional count
// (default 1, capped at maxSpawnCount) mints that many instances; the optional
// destination chooses the room floor (here/room/floor) or the actor's inventory
// (me/inv/bag); inventory is the default, since a builder usually wants the item
// in hand to place or hand off.
func (c *Context) spawnItemHere(ctx context.Context, args []string) error {
	if c.Spawn == nil {
		return c.Actor.Write(ctx, "Spawning is not available.")
	}
	if len(args) == 0 {
		return c.Actor.Write(ctx, "Spawn which item?  (spawn item <template-id> [count] [here|me])")
	}
	templateID := args[0]
	count, rest, ok := parseSpawnCount(args[1:])
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("Spawn how many?  (1–%d)", maxSpawnCount))
	}
	toRoom := false
	if len(rest) > 0 {
		switch strings.ToLower(rest[0]) {
		case "here", "room", "floor", "ground":
			toRoom = true
		case "me", "inv", "inventory", "bag", "hand", "hands":
			toRoom = false
		default:
			return c.Actor.Write(ctx, fmt.Sprintf("Spawn %q where?  (here or me)", rest[0]))
		}
	}

	// A missing room/placement falls back to the inventory rather than leaking an
	// unplaced instance — same fallback the single-spawn path had.
	room := c.Actor.Room()
	placeInRoom := toRoom && room != nil && c.Placement != nil

	var name string
	spawned := 0
	for range count {
		id, n, err := c.spawnOneItem(ctx, templateID)
		if err != nil {
			if spawned == 0 {
				return c.Actor.Write(ctx, fmt.Sprintf("No item template %q.", templateID))
			}
			break // partial mint: report what did land rather than abort silently
		}
		name = n
		if placeInRoom {
			c.Placement.Place(id, room.ID)
			auditAdmin(ctx, c, "spawn", string(id), "item:"+templateID+"@room")
		} else {
			c.Actor.AddToInventory(id)
			auditAdmin(ctx, c, "spawn", string(id), "item:"+templateID+"@inv")
		}
		spawned++
	}

	shown := countable(spawned, name)
	if placeInRoom {
		if c.Broadcaster != nil {
			c.Broadcaster.SendToRoom(ctx, room.ID,
				fmt.Sprintf("%s appears on the ground.", capitalize(shown)), c.Actor.PlayerID())
		}
		return c.Actor.Write(ctx, fmt.Sprintf("You conjure %s onto the ground.", shown))
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You conjure %s into your hands.", shown))
}

// spawnOneItem mints a single item, trying the template-id candidates (verbatim,
// then room-namespace-qualified) in order and returning the first that resolves.
func (c *Context) spawnOneItem(ctx context.Context, templateID string) (entities.EntityID, string, error) {
	var (
		id   entities.EntityID
		name string
		err  error = errNoSpawn
	)
	for _, cand := range spawnCandidates(templateID, roomNamespace(c)) {
		if id, name, err = c.Spawn.SpawnItem(ctx, cand); err == nil {
			return id, name, nil
		}
	}
	return "", "", err
}

// spawnMobHere mints `spawn mob <id> [count]` into the actor's current room,
// running the full mob spawn pipeline per instance through the service. The
// optional count (default 1, capped at maxSpawnCount) mints that many.
func (c *Context) spawnMobHere(ctx context.Context, args []string) error {
	if c.Spawn == nil {
		return c.Actor.Write(ctx, "Spawning is not available.")
	}
	if len(args) == 0 {
		return c.Actor.Write(ctx, "Spawn which mob?  (spawn mob <template-id> [count])")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You're nowhere to spawn a mob.")
	}
	templateID := args[0]
	count, _, ok := parseSpawnCount(args[1:])
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("Spawn how many?  (1–%d)", maxSpawnCount))
	}

	var name string
	spawned := 0
	for range count {
		id, n, err := c.spawnOneMob(ctx, templateID, room.ID)
		if err != nil {
			if spawned == 0 {
				return c.Actor.Write(ctx, fmt.Sprintf("No mob template %q.", templateID))
			}
			break // partial mint: report what did land
		}
		name = n
		auditAdmin(ctx, c, "spawn", string(id), "mob:"+templateID)
		spawned++
	}

	shown := countable(spawned, name)
	if c.Broadcaster != nil {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s appears in a shimmer of air.", capitalize(shown)), c.Actor.PlayerID())
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You spawn %s.", shown))
}

// spawnOneMob mints a single mob into roomID, trying the template-id candidates
// (verbatim, then room-namespace-qualified) in order.
func (c *Context) spawnOneMob(ctx context.Context, templateID string, roomID world.RoomID) (entities.EntityID, string, error) {
	var (
		id   entities.EntityID
		name string
		err  error = errNoSpawn
	)
	for _, cand := range spawnCandidates(templateID, roomNamespace(c)) {
		if id, name, err = c.Spawn.SpawnMob(ctx, cand, roomID); err == nil {
			return id, name, nil
		}
	}
	return "", "", err
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
	// Currency-label seam: the confirmation reads "100¥" / "100 gold" even though
	// the `spawn gold` subcommand keyword stays fixed (it's an input token).
	return c.Actor.Write(ctx, fmt.Sprintf("You conjure %s. (You now have %s.)", c.Money.Format(amount), c.Money.Format(balance)))
}

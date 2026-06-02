package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/corpse"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// LootHandler implements `loot [<corpse>]` (loot-and-corpses §5.1):
// take every item the actor may carry plus all coins from a corpse.
//
// No argument picks a default corpse — the most recently created one in
// the room the actor is allowed to loot (§5.1). A keyword argument
// resolves a corpse by name/ordinal. Looting is gated by the §4
// ownership window; a refusal never names the owner.
//
// Capacity: there is no carry cap in the engine yet, so every item
// fits and the loot is never partial. The loop is structured so a
// future cap drops in as a per-item "does it fit?" gate (§5.1
// "partial on capacity"); until then nothing is left behind.
func LootHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Placement == nil || c.Contents == nil {
		return c.Actor.Write(ctx, "You can't loot anything right now.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "There is nothing here to loot.")
	}

	actorID := string(combat.NewPlayerCombatantID(c.Actor.PlayerID()))
	now := uint64(0)
	if c.NowTick != nil {
		now = c.NowTick()
	}

	target, ok := c.resolveCorpse(room.ID, actorID, now)
	if !ok {
		if len(c.Args) == 0 {
			return c.Actor.Write(ctx, "There is nothing here to loot.")
		}
		return c.Actor.Write(ctx, "You don't see that corpse here.")
	}

	// §4 rights — refuse without naming the owner.
	if !corpse.MayLoot(target, actorID, now, c.CorpseOwnershipWindow) {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't have the right to loot %s yet.", target.Name()))
	}

	// §5.1 / §3 — transfer items + coins (no rights gate here; checked
	// above). Shared with the autoloot path via TransferCorpse.
	taken, credited := TransferCorpse(ctx, c.lootGrant(), c.Actor, target, room.ID, actorID)
	if len(taken) == 0 && credited == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("There is nothing you can take from %s.", target.Name()))
	}

	_ = c.Actor.Write(ctx, lootMessage(c, target.Name(), taken, credited))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s loots %s.", c.Actor.Name(), target.Name()),
			c.Actor.PlayerID())
	}
	return nil
}

// LootGrant bundles the world singletons TransferCorpse needs — a subset
// of the dispatch Env, so the loot verb (from a Context) and the
// autoloot path (from the composition root, reacting to corpse.created)
// share one transfer implementation.
type LootGrant struct {
	Items     *entities.Store
	Contents  *entities.Contents
	Placement *entities.Placement
	Currency  *economy.CurrencyService
	Bus       *eventbus.Bus
}

func (c *Context) lootGrant() LootGrant {
	return LootGrant{Items: c.Items, Contents: c.Contents, Placement: c.Placement, Currency: c.Currency, Bus: c.Bus}
}

// TransferCorpse moves every fitting item plus all coins from target to
// actor (loot-and-corpses §5.1), WITHOUT a rights check — the caller
// asserts the actor may loot (the loot verb checks §4; autoloot acts on
// the killer's own kill). Returns the items taken and coins credited.
//
// Concurrency mirrors the loot verb's guarantees: Contents.Take is the
// single-winner item claim and corpse.ClaimCoins the single-winner coin
// claim, so two looters (or a looter racing the decay sweep) never
// duplicate. The corpse is removed + corpse.looted emitted exactly once,
// gated on Placement.Remove's bool.
func TransferCorpse(ctx context.Context, g LootGrant, actor Actor, target *entities.ItemInstance, roomID world.RoomID, looterID string) ([]*entities.ItemInstance, int) {
	if g.Contents == nil || g.Items == nil || g.Placement == nil {
		return nil, 0
	}

	var takenIDs []entities.EntityID
	for _, id := range g.Contents.In(target.ID()) {
		if g.Contents.Take(id) {
			actor.AddToInventory(id)
			takenIDs = append(takenIDs, id)
		}
	}
	taken := collectItems(g.Items, takenIDs)

	credited := 0
	if g.Currency != nil {
		if holder, ok := actor.(economy.Entity); ok {
			if coins := corpse.ClaimCoins(target); coins > 0 {
				g.Currency.AddGold(ctx, holder, coins, "loot:"+string(target.ID()))
				credited = coins
			}
		}
	}

	if len(g.Contents.In(target.ID())) == 0 && corpse.Coins(target) == 0 {
		if g.Placement.Remove(target.ID()) {
			_ = g.Items.Untrack(target.ID())
			if g.Bus != nil {
				g.Bus.Publish(ctx, eventbus.CorpseLooted{
					CorpseID:  target.ID(),
					RoomID:    roomID,
					LooterID:  looterID,
					ItemCount: len(taken),
					Coins:     credited,
				})
			}
		}
	}
	return taken, credited
}

// AutolootHandler implements `autoloot [on|off]` (loot-and-corpses §6):
// reports or toggles the actor's persisted autoloot preference. The
// auto-loot-on-kill behavior is driven separately by a corpse.created
// subscriber at the composition root.
func AutolootHandler(ctx context.Context, c *Context) error {
	pref, ok := c.Actor.(interface {
		Autoloot() bool
		SetAutoloot(bool)
	})
	if !ok {
		return c.Actor.Write(ctx, "You can't change that right now.")
	}
	if len(c.Args) == 0 {
		state := "off"
		if pref.Autoloot() {
			state = "on"
		}
		return c.Actor.Write(ctx, fmt.Sprintf("Autoloot is currently %s. Use 'autoloot on' or 'autoloot off'.", state))
	}
	switch strings.ToLower(c.Args[0]) {
	case "on":
		pref.SetAutoloot(true)
		return c.Actor.Write(ctx, "Autoloot enabled — you will loot your own kills automatically.")
	case "off":
		pref.SetAutoloot(false)
		return c.Actor.Write(ctx, "Autoloot disabled.")
	default:
		return c.Actor.Write(ctx, "Usage: autoloot [on|off]")
	}
}

// resolveCorpse picks the corpse the loot verb acts on. With no
// argument it returns the most recently created corpse the actor may
// loot (§5.1 default). With a keyword it resolves a corpse by name/
// ordinal among the room's corpses (rights are checked by the caller).
func (c *Context) resolveCorpse(roomID world.RoomID, actorID string, now uint64) (*entities.ItemInstance, bool) {
	corpses := c.roomCorpses(roomID)
	if len(corpses) == 0 {
		return nil, false
	}

	if len(c.Args) == 0 {
		var best *entities.ItemInstance
		for _, cor := range corpses {
			if !corpse.MayLoot(cor, actorID, now, c.CorpseOwnershipWindow) {
				continue
			}
			if best == nil || corpse.CreatedTick(cor) > corpse.CreatedTick(best) {
				best = cor
			}
		}
		return best, best != nil
	}

	match := keyword.Resolve(asNamed(corpses), c.Args[0])
	if match == nil {
		return nil, false
	}
	it, ok := match.(*entities.ItemInstance)
	return it, ok
}

// roomCorpses returns the corpse-tagged items placed in roomID.
func (c *Context) roomCorpses(roomID world.RoomID) []*entities.ItemInstance {
	var out []*entities.ItemInstance
	for _, id := range c.Placement.InRoom(roomID) {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		if it, ok := e.(*entities.ItemInstance); ok && corpse.IsCorpse(it) {
			out = append(out, it)
		}
	}
	return out
}

// lootMessage builds "You loot a, b and 5 gold from the corpse." from
// the taken items (decorated) plus credited coins.
func lootMessage(c *Context, corpseName string, items []*entities.ItemInstance, coins int) string {
	parts := make([]string, 0, len(items)+1)
	for _, it := range items {
		parts = append(parts, decoratedName(c, it))
	}
	if coins > 0 {
		parts = append(parts, fmt.Sprintf("%d gold", coins))
	}
	return fmt.Sprintf("You loot %s from %s.", humanList(parts), corpseName)
}

// humanList joins items as "a", "a and b", or "a, b and c".
func humanList(parts []string) string {
	switch len(parts) {
	case 0:
		return "nothing"
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
}

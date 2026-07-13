package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// GiveHandler implements the `give <item> [to] <player>` verb (spec
// inventory-equipment-items §4.4).
//
// Parses giver-side input as "<item-words> [to] <target>" where the
// target is either the token following an explicit "to" or, absent
// "to", the last whitespace-separated token. The remainder is the
// item argument fed to the shared keyword resolver against the
// actor's inventory.
//
// Validation order matches §4.4:
//  1. Locate recipient in the same room.
//  2. Refuse self-give (not in spec, but the alternative is a
//     silent no-op that masks finger-fumble; explicit message is
//     cheaper than a confused player).
//  3. Resolve the item against inventory contents only — equipped
//     items must be unequipped first. Same shape as drop.
//  4. Remove from giver, add to recipient. The two mutations run
//     under each actor's own lock independently; see the long
//     comment below for why we do not hold both locks at once.
//  5. Emit one ItemGiven event.
//
// Currency auto-convert (§4.4 step 3 / economy-survival §2.3) runs
// after the item leaves the giver: a currency-tagged item given to a
// player credits their gold and suppresses the ItemGiven event.
func GiveHandler(ctx context.Context, c *Context) error {
	if c.Items == nil {
		return c.Actor.Write(ctx, "You can't give anything right now.")
	}
	if c.Locator == nil {
		// No way to find the recipient; behave the same as no-such-target
		// rather than panicking. Production always wires a Locator.
		return c.Actor.Write(ctx, "You can't give anything to anyone right now.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You aren't anywhere; there is no one to give anything to.")
	}

	// M17.2d₄: item (inventory) and target (player — excludes self,
	// preposition "to") are resolved by the §5 pipeline before this
	// runs. The player arg yields an EntityRef; re-fetch the live
	// recipient Actor by the resolved exact name so the two-actor
	// transfer below has a real handle.
	tref, _ := c.Resolved["target"].(EntityRef)

	// Give to an NPC (the quest `deliver` path): the resolver matched a mob, not
	// a player. The NPC takes the item into its contents and an ItemGiven event
	// fires so a deliver objective can advance (quests §7). No currency
	// auto-convert (an NPC has no gold purse) and no two-actor player transfer.
	if tref.Type == entityTypeMob {
		// The NPC files the item into its Contents, so a missing Contents index
		// must fail BEFORE we remove the item from the giver — otherwise the
		// item would be orphaned (gone from inventory, in no container),
		// breaking the entities.Contents "exactly one container" invariant.
		// Mirrors the up-front guard in put.go / loot.go. Always wired in
		// production (main.go); this catches a partially-wired test/script ctx.
		if c.Contents == nil {
			return c.Actor.Write(ctx, "You can't hand anything over right now.")
		}
		npc := findMobByKeyword(c, room.ID, tref.Name)
		if npc == nil {
			return c.Actor.Write(ctx, fmt.Sprintf("%q isn't here.", tref.Name))
		}
		item, ok := resolvedItemInstance(c, "item")
		if !ok {
			return c.Actor.Write(ctx, "You aren't carrying that.")
		}
		if !c.Actor.RemoveFromInventory(item.ID()) {
			return c.Actor.Write(ctx, "You aren't carrying that.")
		}
		c.Contents.Put(npc.ID(), item.ID())
		giverName := c.Actor.Name()
		_ = c.Actor.Write(ctx, fmt.Sprintf("You give %s to %s.", item.Name(), npc.Name()))
		if c.Broadcaster != nil && giverName != "" {
			c.Broadcaster.SendToRoom(ctx, room.ID,
				fmt.Sprintf("%s gives %s to %s.", giverName, item.Name(), npc.Name()),
				c.Actor.PlayerID(), "")
		}
		c.Publish(ctx, eventbus.ItemGiven{
			GiverID:     holderEntityIDForPlayer(c.Actor.PlayerID()),
			RecipientID: npc.ID(),
			RoomID:      room.ID,
			ItemID:      item.ID(),
			ItemName:    item.Name(),
			TemplateID:  string(item.TemplateID()),
		})
		return nil
	}

	target := c.Locator.FindInRoom(room.ID, tref.Name)
	if target == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("%q isn't here.", tref.Name))
	}
	// Self-give defense: the player arg already excludes self from the
	// candidate set (so `give x <ownname>` fails at resolution), but
	// keep the identity guard for the degenerate empty-PlayerID /
	// reconnect-double-handle cases.
	if target == c.Actor || (c.Actor.PlayerID() != "" && target.PlayerID() == c.Actor.PlayerID()) {
		return c.Actor.Write(ctx, "You can't give things to yourself.")
	}

	item, ok := resolvedItemInstance(c, "item")
	if !ok {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

	// Two-actor transfer: the giver's RemoveFromInventory and the
	// recipient's AddToInventory each take their owning actor's mutex
	// independently. We deliberately do NOT hold both locks at once
	// because no other path in the codebase locks two actors
	// together; introducing that here would create a new lock-order
	// regime (every other two-actor caller would need to honor it)
	// for the sake of one operation.
	//
	// Race window: between Remove (succeeds) and Add, the recipient
	// could log out — fullTeardown removes them from the manager and
	// untracks their inventory. The Add then appends to an actor that
	// is no longer reachable through the manager and whose
	// untrackInventory pass has already run, so the item stays in the
	// entity store dangling. This is a vanishingly rare leak (one
	// item per coincident-logout transfer), tolerable for M5.9a, and
	// will be tightened when M6 mob loot drops force a more general
	// "transfer to a specific holder" primitive.
	if !c.Actor.RemoveFromInventory(item.ID()) {
		// Lost a race with our own drop/equip on a parallel session
		// (or a concurrent give from another sender — only one wins).
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}

	giverName := c.Actor.Name()
	recipName := target.Name()

	// Currency auto-convert (spec §2.3 / §4.4 step 3): giving a
	// currency-tagged item to a player credits their gold instead of
	// placing the item in their inventory. The item has already left
	// the giver; the hook untracks it and credits the recipient. The
	// visible give is suppressed of its ItemGiven bus event (the
	// currency feature emits currency.credited on the recipient).
	if value, converted := tryAutoConvert(ctx, c, target, item); converted {
		_ = c.Actor.Write(ctx, fmt.Sprintf("You give %s to %s.", item.Name(), recipName))
		_ = target.Write(ctx, fmt.Sprintf("%s gives you %s (%s).", giverName, item.Name(), c.Money.Format(value)))
		if c.Broadcaster != nil && giverName != "" {
			c.Broadcaster.SendToRoom(ctx, room.ID,
				fmt.Sprintf("%s gives %s to %s.", giverName, item.Name(), recipName),
				c.Actor.PlayerID(), target.PlayerID())
		}
		return nil
	}

	target.AddToInventory(item.ID())

	_ = c.Actor.Write(ctx, fmt.Sprintf("You give %s to %s.", item.Name(), recipName))
	_ = target.Write(ctx, fmt.Sprintf("%s gives you %s.", giverName, item.Name()))
	if c.Broadcaster != nil && giverName != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s gives %s to %s.", giverName, item.Name(), recipName),
			c.Actor.PlayerID(), target.PlayerID())
	}
	c.Publish(ctx, eventbus.ItemGiven{
		GiverID:     holderEntityIDForPlayer(c.Actor.PlayerID()),
		RecipientID: holderEntityIDForPlayer(target.PlayerID()),
		RoomID:      room.ID,
		ItemID:      item.ID(),
		ItemName:    item.Name(),
		TemplateID:  string(item.TemplateID()),
	})
	return nil
}

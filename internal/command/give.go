package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
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
	itemArg, targetArg, ok := parseGiveArgs(c.Args)
	if !ok {
		return c.Actor.Write(ctx, "Give what to whom?")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You aren't anywhere; there is no one to give anything to.")
	}

	target := c.Locator.FindInRoom(room.ID, targetArg)
	if target == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("%q isn't here.", targetArg))
	}
	// Self-give check: interface identity first (catches the case where
	// PlayerID is empty — e.g. a degenerate test actor or a future
	// mob-as-target that hasn't been wired with stable ids); PID
	// string as a belt-and-suspenders fallback for a hypothetical
	// reconnect path that hands out two distinct Actor handles for
	// the same character.
	if target == c.Actor || (c.Actor.PlayerID() != "" && target.PlayerID() == c.Actor.PlayerID()) {
		return c.Actor.Write(ctx, "You can't give things to yourself.")
	}

	candidates := collectItems(c.Items, c.Actor.Inventory())
	if len(candidates) == 0 {
		return c.Actor.Write(ctx, "You aren't carrying anything.")
	}
	match := keyword.Resolve(asNamed(candidates), itemArg)
	if match == nil {
		return c.Actor.Write(ctx, "You aren't carrying that.")
	}
	item := match.(*entities.ItemInstance)

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
		_ = target.Write(ctx, fmt.Sprintf("%s gives you %s (%d gold).", giverName, item.Name(), value))
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

// parseGiveArgs splits the verb's arguments into (itemArg, targetArg).
// Returns ok=false when fewer than two meaningful tokens are present.
//
// Forms accepted:
//
//	give sword bob           → item="sword",     target="bob"
//	give red potion to alice → item="red potion", target="alice"
//	give all to bob          → item="all",       target="bob"
//
// The explicit "to" form wins when present; otherwise the last token
// is the target. "to" as the literal target (`give x to`) is treated
// as missing — there is no recipient.
func parseGiveArgs(args []string) (itemArg, targetArg string, ok bool) {
	if len(args) < 2 {
		return "", "", false
	}
	for i := len(args) - 1; i >= 1; i-- {
		if strings.EqualFold(args[i], "to") {
			if i == len(args)-1 {
				// "give x to" — no target after the preposition.
				return "", "", false
			}
			itemArg = strings.Join(args[:i], " ")
			targetArg = strings.Join(args[i+1:], " ")
			if itemArg == "" || targetArg == "" {
				return "", "", false
			}
			return itemArg, targetArg, true
		}
	}
	itemArg = strings.Join(args[:len(args)-1], " ")
	targetArg = args[len(args)-1]
	return itemArg, targetArg, true
}

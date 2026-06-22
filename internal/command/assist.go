package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// AssistHandler implements `assist <ally>` (grouping.md §9): join the fight an
// ally in your room is already in — you engage whatever they're fighting. The
// common case is a party-mate, but it works for anyone you can see (no party
// requirement; the party benefit is that kill-XP + loot are already shared).
// Manual only — auto-assist (a party-mate's engage pulling the rest in) is a
// deferred follow-up.
func AssistHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Assist whom?")
	}
	allyArg := strings.Join(c.Args, " ")
	if isSelfReference(c.Actor.Name(), allyArg) {
		return c.Actor.Write(ctx, "You can't assist yourself.")
	}
	if c.Combat == nil {
		return c.Actor.Write(ctx, "You can't fight right now.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see no one to assist here.")
	}
	attacker, ok := c.Actor.(combat.Combatant)
	if !ok {
		return c.Actor.Write(ctx, "You aren't able to fight.")
	}

	ally, allyName, found := findCombatantInRoom(c, room.ID, allyArg)
	if !found {
		return c.Actor.Write(ctx, "You don't see them here.")
	}
	oppID, fighting := c.Combat.PrimaryTargetOf(ally.CombatantID())
	if !fighting {
		return c.Actor.Write(ctx, fmt.Sprintf("%s isn't fighting anyone.", allyName))
	}
	// If the ally is fighting YOU, there's no one to assist against.
	if combat.EntityIDOf(oppID) == c.Actor.PlayerID() {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is fighting you — defend yourself!", allyName))
	}
	_, oppName, ok := resolveCombatantByID(c, oppID)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("You can't make out what %s is fighting.", allyName))
	}

	if handled, err := c.tryEngage(ctx, attacker.CombatantID(), oppID, room.ID, oppName); handled {
		return err
	}

	if err := c.Actor.Write(ctx, fmt.Sprintf("You move to assist %s, attacking %s!", allyName, oppName)); err != nil {
		return err
	}
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s moves to assist %s against %s!", c.Actor.Name(), allyName, oppName),
			c.Actor.PlayerID())
	}
	return nil
}

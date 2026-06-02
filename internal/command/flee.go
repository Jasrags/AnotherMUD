package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// FleeHandler implements `flee` — the verb-triggered version of the
// §5.2 flee attempt that the wimpy phase runs automatically. Same
// primitive, same outcomes; the only difference is the message the
// player sees.
//
// Refusal paths and their messages:
//   - actor not a combatant     → "You can't flee from anything."
//   - flee primitive not wired  → "You can't flee right now."
//   - no-flee tag               → "Something stops you from fleeing."
//   - no exits in room          → "There's nowhere to run."
//   - unknown room              → "You stumble in confusion."
//
// On success: the §5.2 bus event fires (which the broadcaster will
// later turn into "X flees!" room broadcasts via the production
// Mover); we tell the fleer "You flee in panic!" and then render the
// destination room. The flee Mover has already SetRoom'd the actor by
// the time c.Flee returns, so c.Actor.Room() is the new room — without
// this render the player lands in an unseen room; this mirrors a normal
// movement (builtins.go movementHandler).
func FleeHandler(ctx context.Context, c *Context) error {
	cb, ok := c.Actor.(combat.Combatant)
	if !ok {
		return c.Actor.Write(ctx, "You can't flee from anything.")
	}
	if c.Flee == nil {
		// Test path without the flee primitive wired — refuse
		// cleanly rather than nil-deref. Production main always
		// supplies Flee.
		return c.Actor.Write(ctx, "You can't flee right now.")
	}

	switch c.Flee(ctx, cb.CombatantID()) {
	case combat.FleeOutcomeSuccess:
		_ = c.Actor.Write(ctx, "You flee in panic!")
		if room := c.Actor.Room(); room != nil {
			return c.Actor.Write(ctx, RenderRoom(room, c.Placement, c.Items, c.questMarker(), c.Ambience, c.otherPlayerNames(room.ID)...))
		}
		return nil
	case combat.FleeOutcomePrevented:
		return c.Actor.Write(ctx, "Something stops you from fleeing.")
	case combat.FleeOutcomeFailedNoExits:
		return c.Actor.Write(ctx, "There's nowhere to run.")
	case combat.FleeOutcomeFailedUnknownRoom:
		return c.Actor.Write(ctx, "You stumble in confusion.")
	default:
		return c.Actor.Write(ctx, "You can't flee right now.")
	}
}

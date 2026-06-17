package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// AdvanceHandler implements `advance` (ranged-combat §5.4): close one range
// band toward melee with your current target — the way a melee fighter shuts
// the distance an archer is keeping open.
func AdvanceHandler(ctx context.Context, c *Context) error {
	return moveBandVerb(ctx, c, true)
}

// WithdrawHandler implements `withdraw` (ranged-combat §5.4): open one range
// band away from melee, staying in the room. A ranged fighter withdrawing while
// a melee opponent advances is kiting — keeping the distance open to keep
// shooting. Distinct from `flee`, which leaves the room entirely.
func WithdrawHandler(ctx context.Context, c *Context) error {
	return moveBandVerb(ctx, c, false)
}

// moveBandVerb is the shared body for advance/withdraw: it moves the band
// against the actor's PRIMARY target one step (closing = toward melee). The
// move + its narration happen in combat.Manager.MoveBand (it emits the
// BandChange), so a success returns nil here.
func moveBandVerb(ctx context.Context, c *Context, closing bool) error {
	if c.Combat == nil {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}
	attacker, ok := c.Actor.(combat.Combatant)
	if !ok {
		return c.Actor.Write(ctx, "You aren't able to fight.")
	}
	targetID, inCombat := c.Combat.PrimaryTargetOf(attacker.CombatantID())
	if !inCombat {
		return c.Actor.Write(ctx, "You aren't fighting anyone.")
	}
	roomID := world.RoomID("")
	if room := c.Actor.Room(); room != nil {
		roomID = room.ID
	}
	if _, moved := c.Combat.MoveBand(ctx, attacker.CombatantID(), targetID, roomID, closing); !moved {
		if closing {
			return c.Actor.Write(ctx, "You're already in melee range.")
		}
		return c.Actor.Write(ctx, "You can't open the distance any further.")
	}
	return nil
}

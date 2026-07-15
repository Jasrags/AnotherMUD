package command

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TransitService is the conveyance call-control surface the command layer
// depends on (transit.md §4.1, §8) — implemented at the composition root by
// transit.Service. It handles a press/call from a room and reports whether that
// room is a transit room at all. Defined here (point of use) per the
// small-interface convention.
type TransitService interface {
	// Press resolves a call control at room: inside a car, arg selects a
	// destination stop; at a landing, the car is summoned (arg ignored). It
	// returns the actor-facing message and whether room is a transit room
	// (false → the caller reports there is nothing to press).
	Press(ctx context.Context, room world.RoomID, arg, actorID string) (msg string, handled bool)
}

// pressHandler implements `press <floor>` and `call` (transit.md §8). Inside an
// elevator car, `press <floor>` sends it to that floor; at a landing, either
// verb summons the car. nil Transit (tests / worlds without conveyances) → a
// generic reply.
func pressHandler(ctx context.Context, c *Context) error {
	if c.Transit == nil {
		return c.Actor.Write(ctx, "There's nothing to press here.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "There's nothing to press here.")
	}
	arg := strings.TrimSpace(strings.Join(c.Args, " "))
	msg, handled := c.Transit.Press(ctx, room.ID, arg, c.Actor.PlayerID())
	if !handled {
		return c.Actor.Write(ctx, "There's nothing to press here.")
	}
	return c.Actor.Write(ctx, msg)
}

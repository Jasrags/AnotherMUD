package command

import "context"

// emitRangedFlavor resolves a ranged-weapon moment's lines for the actor's
// wielded-weapon style (rangedflavor) and delivers them: the second-person
// `self` line to the actor, and the third-person `room` line (when non-empty)
// to everyone else in the room. c.RangedFlavor may be nil — the resolver
// nil-guards and falls back to the neutral engine floor — so this works headless
// and in tests without wiring. Returns the actor Write's error.
func (c *Context) emitRangedFlavor(ctx context.Context, style, key string, params map[string]string) error {
	self, room := c.RangedFlavor.Resolve(style, key, params)
	if room != "" && c.Broadcaster != nil && c.Actor.Name() != "" {
		if r := c.Actor.Room(); r != nil {
			c.Broadcaster.SendToRoom(ctx, r.ID, room, c.Actor.PlayerID())
		}
	}
	return c.Actor.Write(ctx, self)
}

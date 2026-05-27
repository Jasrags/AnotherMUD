package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// wimpyController is the tiny mutation surface a connActor exposes
// for `wimpy <pct>`. Defined here so the command package doesn't
// import session/combat just for two methods. The production actor
// (session.connActor) satisfies it; test fakes that don't care
// about wimpy don't.
type wimpyController interface {
	WimpyThreshold() int
	SetWimpyThreshold(pct int)
}

// WimpyHandler implements `wimpy [<pct>]` — read or set the §5.1
// flee threshold.
//
// No argument: report the current setting.
// Numeric argument 0..100: set the threshold. Zero disables wimpy.
// "off" / "none": alias for zero.
// Anything else: usage message.
//
// The actor must satisfy wimpyController; test actors without combat
// state see the standard "you can't" message.
func WimpyHandler(ctx context.Context, c *Context) error {
	ctrl, ok := c.Actor.(wimpyController)
	if !ok {
		return c.Actor.Write(ctx, "You aren't able to flee.")
	}
	if len(c.Args) == 0 {
		cur := ctrl.WimpyThreshold()
		if cur <= 0 {
			return c.Actor.Write(ctx, "Your wimpy threshold is off.")
		}
		return c.Actor.Write(ctx,
			fmt.Sprintf("Your wimpy threshold is %d%% HP.", cur))
	}

	arg := strings.ToLower(c.Args[0])
	if arg == "off" || arg == "none" {
		ctrl.SetWimpyThreshold(0)
		return c.Actor.Write(ctx, "Wimpy disabled.")
	}
	pct, err := strconv.Atoi(arg)
	if err != nil {
		return c.Actor.Write(ctx, "Usage: wimpy <0-100|off>")
	}
	if pct < 0 || pct > 100 {
		return c.Actor.Write(ctx, "Wimpy threshold must be between 0 and 100.")
	}
	ctrl.SetWimpyThreshold(pct)
	if pct == 0 {
		return c.Actor.Write(ctx, "Wimpy disabled.")
	}
	return c.Actor.Write(ctx,
		fmt.Sprintf("Wimpy threshold set to %d%% HP.", pct))
}

package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/light"
)

// DaylightHandler implements the `daylight` probe verb (light-and-
// darkness §8): a read-only report of the current time-of-day period
// and how well the viewer can see here right now. It lets a player read
// the light/time directly rather than inferring it from the room render.
func DaylightHandler(ctx context.Context, c *Context) error {
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You float in formless void; there is neither day nor night here.")
	}
	if c.Light == nil {
		// Light gating unwired — everything reads as lit.
		return c.Actor.Write(ctx, "The light here is steady and clear.")
	}
	period := c.Light.Period()
	lvl := c.effectiveLight(room)
	return c.Actor.Write(ctx, fmt.Sprintf("It is %s. %s", period, describeLightLevel(lvl)))
}

// describeLightLevel renders a short sentence for how an effective light
// level feels to the viewer (§8 probe). Hardcoded for v1.
func describeLightLevel(lvl light.Level) string {
	switch lvl {
	case light.Lit:
		return "You can see clearly here."
	case light.Dim:
		return "The light here is dim, but you can make things out."
	case light.Gloom:
		return "It is gloomy here; you can sense only shapes and directions."
	default:
		return "It is pitch black here; you can see nothing."
	}
}

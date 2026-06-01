package command

import (
	"context"
	"fmt"
)

// ReloadHandler implements the `reload` admin verb (M17.3): re-read
// pack Lua from disk and hot-swap the scripting runtime without a
// server restart. Touches only the scripting layer — world.World and
// the content registries are left alone (script-only reload).
//
// Ungated for now. Like the `xp` probe, it is registered bare (no
// listing metadata) so it stays out of the player-facing help list
// until the role system can restrict it to admins (M10+/M12). The
// composition root supplies the ReloadScripts closure; when it is nil
// (tests, or a build without scripting) the verb reports that reloading
// is unavailable rather than pretending to succeed.
func ReloadHandler(ctx context.Context, c *Context) error {
	if c.ReloadScripts == nil {
		return c.Actor.Write(ctx, "Reloading is not enabled.")
	}
	n, err := c.ReloadScripts(ctx)
	if err != nil {
		// Surface the underlying error verbatim — a reload is an admin
		// diagnostic, and the *scripting.Error carries the pack +
		// script + Lua message the operator needs to fix the edit.
		return c.Actor.Write(ctx, fmt.Sprintf("Reload failed: %s", err))
	}
	return c.Actor.Write(ctx, fmt.Sprintf("Reloaded %d script(s).", n))
}

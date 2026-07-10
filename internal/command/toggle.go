package command

import (
	"context"
	"fmt"
	"strings"
)

// applyBinaryToggle implements the standard binary on/off verb grammar shared by
// every simple preference toggle (commands-and-dispatch): NO argument flips the
// current state, an explicit `on` / `off` sets it, anything else is a usage error.
// cur is the current state; set applies the new one; onMsg / offMsg are the
// confirmations; name seeds the "Usage: <name> [on|off]" line. Handlers that need
// extra gating on the on-transition (a feat check, say) roll their own instead.
func applyBinaryToggle(ctx context.Context, c *Context, name string, cur bool, set func(bool), onMsg, offMsg string) error {
	target := !cur
	if len(c.Args) > 0 {
		switch strings.ToLower(c.Args[0]) {
		case "on":
			target = true
		case "off":
			target = false
		default:
			return c.Actor.Write(ctx, fmt.Sprintf("Usage: %s [on|off]", name))
		}
	}
	set(target)
	if target {
		return c.Actor.Write(ctx, onMsg)
	}
	return c.Actor.Write(ctx, offMsg)
}

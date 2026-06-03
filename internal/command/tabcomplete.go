package command

import (
	"context"
	"strings"
)

// CharModeController is the actor capability the `tabcomplete` verb toggles
// — server-side character-at-a-time editing for raw-telnet TAB completion
// (tab-completion Phase 2). The session's connActor implements it
// (delegating to the connection); WebSocket and test actors don't, so the
// verb reports it's unavailable there.
type CharModeController interface {
	// SetCharMode turns char-mode on/off; returns false when the transport
	// can't do it (ws).
	SetCharMode(ctx context.Context, on bool) bool
	// CharModeActive reports the current mode.
	CharModeActive() bool
}

// TabCompleteHandler implements `tabcomplete [on|off]` — toggles server-side
// TAB completion for raw telnet (it's default-on for raw clients). On
// GMCP-capable or WebSocket clients there's no char-mode (they complete via
// the client / GMCP), so the verb says so.
func TabCompleteHandler(ctx context.Context, c *Context) error {
	ctrl, ok := c.Actor.(CharModeController)
	if !ok {
		return c.Actor.Write(ctx, "Tab-completion isn't available on this connection.")
	}
	switch strings.ToLower(strings.TrimSpace(strings.Join(c.Args, " "))) {
	case "", "status":
		if ctrl.CharModeActive() {
			return c.Actor.Write(ctx, "Tab-completion is ON — press Tab to complete a partial command. `tabcomplete off` to disable.")
		}
		return c.Actor.Write(ctx, "Tab-completion is OFF. `tabcomplete on` to enable (raw telnet only).")
	case "on":
		if !ctrl.SetCharMode(ctx, true) {
			return c.Actor.Write(ctx, "Tab-completion isn't available on this client (use a raw telnet client, or GMCP completion).")
		}
		return c.Actor.Write(ctx, "Tab-completion ON. Press Tab to complete a partial command.")
	case "off":
		ctrl.SetCharMode(ctx, false)
		return c.Actor.Write(ctx, "Tab-completion OFF.")
	default:
		return c.Actor.Write(ctx, "Usage: tabcomplete [on|off]")
	}
}

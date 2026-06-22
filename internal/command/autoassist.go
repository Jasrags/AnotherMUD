package command

import (
	"context"
	"fmt"
	"strings"
)

// AutoAssistHandler implements `autoassist [on|off]` (grouping.md §9): reports
// or toggles the actor's persisted auto-assist preference. When on, the actor
// is automatically pulled into a party-mate's fight (engaging whatever the
// mate is fighting) the moment that fight begins in the same room — the
// automatic counterpart to the manual `assist` verb. Off by default so a
// party member's engage doesn't yank everyone into every fight; the
// engage-on-engagement behavior is driven by the combat sink at the
// composition root, which reads this preference.
func AutoAssistHandler(ctx context.Context, c *Context) error {
	pref, ok := c.Actor.(interface {
		AutoAssistEnabled() bool
		SetAutoAssist(bool)
	})
	if !ok {
		return c.Actor.Write(ctx, "You can't change that right now.")
	}
	if len(c.Args) == 0 {
		state := "off"
		if pref.AutoAssistEnabled() {
			state = "on"
		}
		return c.Actor.Write(ctx, fmt.Sprintf("Auto-assist is currently %s. Use 'autoassist on' or 'autoassist off'.", state))
	}
	switch strings.ToLower(c.Args[0]) {
	case "on":
		pref.SetAutoAssist(true)
		return c.Actor.Write(ctx, "Auto-assist enabled — you will join your party's fights automatically.")
	case "off":
		pref.SetAutoAssist(false)
		return c.Actor.Write(ctx, "Auto-assist disabled.")
	default:
		return c.Actor.Write(ctx, "Usage: autoassist [on|off]")
	}
}

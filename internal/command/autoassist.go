package command

import "context"

// AutoAssistHandler implements `autoassist [on|off]` (grouping.md §9): with no
// argument it flips the actor's persisted auto-assist preference, or `on`/`off`
// sets it explicitly. When on, the actor
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
	return applyBinaryToggle(ctx, c, "autoassist", pref.AutoAssistEnabled(), pref.SetAutoAssist,
		"Auto-assist enabled — you will join your party's fights automatically.",
		"Auto-assist disabled.")
}

package command

import (
	"context"
	"fmt"
	"strings"
)

// AutoreloadHandler implements the `autoreload` toggle verb (autoreload.md §2):
// a per-character preference that auto-reloads a dry wielded firearm from a
// compatible clip/holder (or loose rounds) instead of leaving it dry. Off by
// default — opt-in, since an auto-reload spends the shot's beat and ejects a
// clip on the wielder's behalf. Mirrors the `autoloot` toggle exactly.
func AutoreloadHandler(ctx context.Context, c *Context) error {
	pref, ok := c.Actor.(interface {
		Autoreload() bool
		SetAutoreload(bool)
	})
	if !ok {
		return c.Actor.Write(ctx, "You can't change that right now.")
	}
	if len(c.Args) == 0 {
		state := "off"
		if pref.Autoreload() {
			state = "on"
		}
		return c.Actor.Write(ctx, fmt.Sprintf("Autoreload is currently %s. Use 'autoreload on' or 'autoreload off'.", state))
	}
	switch strings.ToLower(c.Args[0]) {
	case "on":
		pref.SetAutoreload(true)
		return c.Actor.Write(ctx, "Autoreload enabled — a dry firearm reloads itself from a spare clip when you fire.")
	case "off":
		pref.SetAutoreload(false)
		return c.Actor.Write(ctx, "Autoreload disabled — reload manually when a firearm runs dry.")
	default:
		return c.Actor.Write(ctx, "Usage: autoreload [on|off]")
	}
}

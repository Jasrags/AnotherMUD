package command

import "context"

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
	return applyBinaryToggle(ctx, c, "autoreload", pref.Autoreload(), pref.SetAutoreload,
		"Autoreload enabled — a dry firearm reloads itself from a spare clip when you fire.",
		"Autoreload disabled — reload manually when a firearm runs dry.")
}

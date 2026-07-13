package command

import "context"

// QuestSpawnViewer is the optional actor capability the `showspawns` toggle and
// the quest-spawn admin bypass read: a persisted staff preference for whether
// this viewer sees OTHER players' quest spawns (quest-spawns.md §10 admin
// bypass). connActor satisfies it; an actor that does not implement it is
// treated as showing them (the default staff-bypass behavior), so tests and
// headless paths keep the prior behavior.
type QuestSpawnViewer interface {
	ShowOtherQuestSpawns() bool
	SetShowOtherQuestSpawns(bool)
}

// viewerShowsOtherQuestSpawns reports whether the viewer's staff bypass of the
// quest-spawn gate is enabled. Default true for a non-implementer (test /
// headless), matching the historical always-bypass behavior; the bypass itself
// only takes effect for an actor holding the admin role (checked separately).
func viewerShowsOtherQuestSpawns(actor Actor) bool {
	v, ok := actor.(QuestSpawnViewer)
	return !ok || v.ShowOtherQuestSpawns()
}

// ShowSpawnsHandler toggles whether the calling staff member sees other
// players' quest spawns (quest-spawns.md §10 admin bypass). Admin-only (gated
// at dispatch): `showspawns` flips it, `showspawns on|off` sets it explicitly.
// "on" restores the default staff bypass (see every runner's spawns for
// moderation); "off" silences the clutter so the staffer sees only their own,
// exactly as an ordinary player does. Persists across logins. The preference
// only matters while the viewer holds the admin role — the bypass gates on the
// role, so a saved flag does nothing for a non-admin.
func ShowSpawnsHandler(ctx context.Context, c *Context) error {
	v, ok := c.Actor.(QuestSpawnViewer)
	if !ok {
		return c.Actor.Write(ctx, "Quest-spawn visibility is not available.")
	}
	return applyBinaryToggle(ctx, c, "showspawns", v.ShowOtherQuestSpawns(), v.SetShowOtherQuestSpawns,
		"Now showing other players' quest spawns (staff bypass ON).",
		"Now hiding other players' quest spawns (staff bypass OFF) — you see only your own.")
}

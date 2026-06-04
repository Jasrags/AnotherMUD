package command

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// WhoEntry is one roster row for the `who` verb — a value snapshot built
// by the session layer under its own locks, so the command layer never
// reaches into connActor internals. RoleMarker is the bracket-less staff
// tag (e.g. "Admin") or "" for an ordinary player.
type WhoEntry struct {
	Name       string
	Idle       bool
	RoleMarker string
}

// Roster is the world-wide online-player snapshot the `who` verb reads
// (who §2–§4). The session Manager satisfies it via an adapter. nil
// disables `who` (tests / headless). v1 returns every playing session
// (the actor included); per-viewer visibility filtering attaches here when
// visibility rules land.
//
// OnlineRoster MUST return a freshly-allocated slice each call — the
// handler sorts it in place. Implementations must not hand back a slice
// they retain a reference to.
type Roster interface {
	OnlineRoster() []WhoEntry
}

// WhoHandler implements the `who` verb (who §2–§4): one line per connected,
// playing character plus a summary count. Presence only — no rooms, no
// vitals (who §1.1). Lines render through the normal color pipeline via
// Actor.Write. Ordering is stable + alphabetical (who §2).
func WhoHandler(ctx context.Context, c *Context) error {
	if c.Roster == nil {
		return c.Actor.Write(ctx, "Nobody seems to be around.")
	}
	entries := c.Roster.OnlineRoster()
	// Stable, alphabetical-by-name (who §2): presentational, not ranked.
	sort.SliceStable(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	var b strings.Builder
	for _, e := range entries {
		b.WriteString("  ")
		b.WriteString(e.Name)
		if e.RoleMarker != "" {
			fmt.Fprintf(&b, " [%s]", e.RoleMarker)
		}
		if e.Idle {
			b.WriteString(" (idle)")
		}
		b.WriteByte('\n')
	}
	b.WriteString(whoSummary(len(entries)))
	return c.Actor.Write(ctx, b.String())
}

// whoSummary renders the count line (who §3) — the number of lines actually
// shown, singular/plural aware.
func whoSummary(n int) string {
	if n == 1 {
		return "1 player online."
	}
	return fmt.Sprintf("%d players online.", n)
}

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
	// PlayerID identifies the character so the viewer can always see their
	// own row even when admin-invisible. A value snapshot — the command layer
	// never reaches into connActor (who §4).
	PlayerID string
	// AdminInvisible marks a character walking via `wizinvis` (visibility
	// §3.4): excluded from a non-staff viewer's roster and count, per-viewer.
	AdminInvisible bool
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
	// Per-viewer visibility filter (who §4 / visibility §3.4): an admin
	// walking via `wizinvis` is excluded from a non-staff viewer's roster and
	// count. The viewer always sees their own row (self is always visible,
	// §2.1). Done before the count line so the summary reflects what's shown.
	viewerIsAdmin := actorIsAdmin(c.Actor, c.AdminRole)
	self := c.Actor.PlayerID()
	entries = filterWhoVisible(entries, viewerIsAdmin, self)
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

// filterWhoVisible drops admin-invisible characters that the viewer may not
// see (who §4 / visibility §3.4): a non-staff viewer never sees a wizinvis
// admin, but every viewer always sees their own row. Returns the input
// unchanged when nothing is filtered (the common case).
func filterWhoVisible(entries []WhoEntry, viewerIsAdmin bool, self string) []WhoEntry {
	if viewerIsAdmin {
		return entries // staff see everyone (rank ≥ any wizinvis rank)
	}
	out := entries[:0:0] // fresh backing array; never alias the caller's slice
	for _, e := range entries {
		if e.AdminInvisible && e.PlayerID != self {
			continue
		}
		out = append(out, e)
	}
	return out
}

// whoSummary renders the count line (who §3) — the number of lines actually
// shown, singular/plural aware.
func whoSummary(n int) string {
	if n == 1 {
		return "1 player online."
	}
	return fmt.Sprintf("%d players online.", n)
}

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
	// walking via `wizinvis` is excluded from a non-staff viewer; a
	// magically-invisible character from a viewer lacking the see-invisible
	// counter. Both are independent (admin rank does NOT pierce magical invis,
	// §4.3). The viewer always sees their own row (self is always visible,
	// §2.1). Done before the count line so the summary reflects what's shown.
	viewerIsAdmin := actorIsAdmin(c.Actor, c.AdminRole)
	viewerSeesInvis := c.viewerSeesInvisible()
	self := c.Actor.PlayerID()
	// Magical invisibility is sourced from active effects, which the command
	// layer queries directly (the roster needs no effect coupling). nil-safe.
	isInvisible := func(playerID string) bool {
		return c.Effects != nil && c.Effects.HasFlag(playerID, InvisibleFlag)
	}
	entries = filterWhoVisible(entries, viewerIsAdmin, viewerSeesInvis, self, isInvisible)
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

// filterWhoVisible drops characters the viewer may not see (who §4 /
// visibility §3.4): a non-staff viewer never sees a wizinvis admin, and a
// viewer lacking the see-invisible counter never sees a magically-invisible
// character. The two gates are independent (admin rank does not pierce magical
// invis, §4.3). Every viewer always sees their own row (self is always
// visible, §2.1). isInvisible reports magical invisibility per player id.
func filterWhoVisible(entries []WhoEntry, viewerIsAdmin, viewerSeesInvis bool, self string, isInvisible func(string) bool) []WhoEntry {
	out := entries[:0:0] // fresh backing array; never alias the caller's slice
	for _, e := range entries {
		if e.PlayerID == self {
			out = append(out, e) // self always visible
			continue
		}
		if e.AdminInvisible && !viewerIsAdmin {
			continue
		}
		if !viewerSeesInvis && isInvisible(e.PlayerID) {
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

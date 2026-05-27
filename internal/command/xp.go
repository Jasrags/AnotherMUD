package command

import (
	"context"
	"fmt"
	"strconv"
)

// XPHandler implements the M8.2 admin xp command:
//
//	xp                  → show every track's TrackInfo for the actor
//	xp <amount> [track] → grant amount XP to the actor on track
//	                      (default track: "adventurer")
//
// The verb is intentionally simple and self-grants only. A
// full admin role gate + target-by-name form lands when the role
// system arrives (M10+). The verb is the end-to-end probe spec
// §5.4 wants: it exercises lazy init, cascade, event emission,
// and persistence (the GrantXP wrapper flips the actor's dirty
// bit, so the next Persist commits the XP to disk).
//
// Args:
//   - 0 args: print TrackInfo for every track the registry knows.
//   - 1 arg: amount only. Track defaults to "adventurer".
//   - 2 args: amount + track name. Unknown tracks render a precise
//     diagnostic.
//   - 3+ args: usage line.
func XPHandler(ctx context.Context, c *Context) error {
	if c.Progression == nil {
		return c.Actor.Write(ctx, "Progression is not enabled in this build.")
	}
	holder, ok := c.Actor.(ProgressionHolder)
	if !ok {
		return c.Actor.Write(ctx, "You can't earn experience.")
	}

	if len(c.Args) == 0 {
		return renderAllTracks(ctx, c, holder)
	}
	if len(c.Args) > 2 {
		return c.Actor.Write(ctx, "Usage: xp [<amount> [<track>]]")
	}

	amount, err := strconv.ParseInt(c.Args[0], 10, 64)
	if err != nil || amount <= 0 {
		return c.Actor.Write(ctx, "Amount must be a positive integer.")
	}
	track := "adventurer"
	if len(c.Args) == 2 {
		track = c.Args[1]
	}

	res := holder.GrantXP(ctx, c.Progression, track, "admin:xp", amount)
	if res.TrackUnknown {
		return c.Actor.Write(ctx, fmt.Sprintf("No such track: %q", track))
	}
	msg := fmt.Sprintf("You gain %d XP on %s (now %d).", res.XPAdded, res.Track, res.NewXP)
	if res.NewLevel > res.OldLevel {
		msg += fmt.Sprintf(" You reach level %d!", res.NewLevel)
	}
	return c.Actor.Write(ctx, msg)
}

// renderAllTracks prints a TrackInfo line for every track in the
// registry. Tracks the entity has never touched still show
// (level=1, xp=0) after lazy-init — keeping the output stable
// regardless of interaction history.
func renderAllTracks(ctx context.Context, c *Context, holder ProgressionHolder) error {
	tracks := c.Progression.Tracks().All()
	if len(tracks) == 0 {
		return c.Actor.Write(ctx, "(no progression tracks defined)")
	}
	for _, td := range tracks {
		info, ok := holder.TrackInfo(c.Progression, td.Name)
		if !ok {
			continue
		}
		label := td.DisplayName
		if label == "" {
			label = td.Name
		}
		var line string
		if info.Level >= info.MaxLevel {
			line = fmt.Sprintf("%-20s  level %d (MAX)   xp %d   overflow %d",
				label, info.Level, info.XP, info.Overflow)
		} else {
			line = fmt.Sprintf("%-20s  level %d         xp %d   to next %d",
				label, info.Level, info.XP, info.XpToNext)
		}
		if err := c.Actor.Write(ctx, line); err != nil {
			return err
		}
	}
	return nil
}

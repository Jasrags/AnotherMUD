package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// AreaTracker is the optional actor capability that drives area-
// transition messaging (player-maps §4). The single AreaTransition call
// atomically records the new area as last-seen and reports the crossing
// decision: prev (the "from" of the leave/enter zone-line, A2; empty
// before the first render), changed (false for an intra-area render —
// nothing mutated), and firstEntry (true the once-ever first time the
// character enters newArea, gating the first-entry banner B1). Folding the
// read-modify-write into one call keeps the "detect once per crossing"
// decision atomic in the actor, where its lock lives — the command layer
// only builds strings from the result. An actor that does not implement
// it (test fakes without the capability) gets no transition lines.
type AreaTracker interface {
	AreaTransition(newArea world.AreaID) (prev world.AreaID, changed, firstEntry bool)
}

// FirstEntryBanner is the once-ever "new territory" line shown the first
// time a character enters an area (player-maps §4, B1). Exported so the
// session-package login-spawn render can show it for a brand-new
// character's home area without re-deriving the styling.
func FirstEntryBanner(areaName string) string {
	return fmt.Sprintf("<highlight>*** You have entered %s for the first time! ***</highlight>", areaName)
}

// writeRoomView writes the room view for r to the actor, prefixed with
// any area-transition banner (a leave/enter zone-line when the actor has
// crossed into a new area since the last room they were shown). Every
// Context-based arrival render — look, movement, flee, recall, teleport
// — routes through here so a crossing reads the same regardless of how
// it happened. The last-seen-area update is a side effect of *showing*
// the room, which is why it lives in this write wrapper rather than the
// pure renderRoomWithData builder. The login-spawn and link-dead
// reattach renders live in the session package and seed the tracker
// directly instead of routing through here.
func (c *Context) writeRoomView(ctx context.Context, r *world.Room, lvl light.Level) error {
	view := c.renderRoomWithData(r, lvl)
	if banner := c.areaTransitionBanner(r); banner != "" {
		view = banner + "\n" + view
	}
	if err := c.Actor.Write(ctx, view); err != nil {
		return err
	}
	// One-time contextual tips after the room is shown (ui-rendering-help §12).
	c.maybeShowRoomTips(ctx, r, lvl)
	return nil
}

// areaTransitionBanner returns the leave/enter zone-line when the actor
// has crossed an area boundary since the last room they were shown, and
// records the new area as last-seen. Empty when: the actor isn't an
// AreaTracker, the world isn't wired, the area is unchanged (a look or
// an intra-area move), or there is no prior area (the first render after
// login — there is no "from" to narrate, per the spawn-suppression
// rule). Mutating the tracker here keeps the "detect once per crossing"
// invariant in one place.
func (c *Context) areaTransitionBanner(r *world.Room) string {
	at, ok := c.Actor.(AreaTracker)
	if !ok || c.World == nil || r == nil {
		return ""
	}
	newArea := r.AreaID
	// One atomic call does the whole detect-once-per-crossing decision; the
	// command layer only renders the result.
	prev, changed, firstEntry := at.AreaTransition(newArea)
	if !changed {
		return "" // same area: a look, or an intra-area step
	}

	var lines []string
	// A2: leave/enter zone-line — only when there is a "from" (not the
	// first render after login, which the spawn render seeds silently).
	if prev != "" {
		lines = append(lines, fmt.Sprintf("<subtle>You leave</subtle> %s <subtle>and enter</subtle> %s<subtle>.</subtle>",
			c.areaName(prev), c.areaName(newArea)))
	}
	// B1: once-ever first-entry banner, below the zone-line. AreaTransition
	// already persisted the seen-area set, so this fires exactly once ever.
	if firstEntry {
		lines = append(lines, FirstEntryBanner(c.areaName(newArea)))
	}
	return strings.Join(lines, "\n")
}

// areaName resolves an area id to its display name via the Context's
// world, falling back to the raw id when the area is unknown or unnamed.
func (c *Context) areaName(id world.AreaID) string {
	return MapAreaName(c.World, id)
}

// MapAreaName resolves an area id to its display name against w,
// falling back to the raw id when w is nil or the area is unknown or
// unnamed. Exported so the session-package room renders (login spawn,
// first-entry banner) share one area-name resolver instead of
// re-deriving it; the minimap label (A1) and zone-line (A2) use it too.
func MapAreaName(w *world.World, id world.AreaID) string {
	if w == nil || id == "" {
		return string(id)
	}
	if a, err := w.Area(id); err == nil && a.Name != "" {
		return a.Name
	}
	return string(id)
}

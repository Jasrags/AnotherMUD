package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// AreaTracker is the optional actor capability that drives area-
// transition messaging (player-maps §4):
//   - LastAreaSeen/SetLastAreaSeen — the session-scoped area the actor
//     was last shown, the "from" of the leave/enter zone-line (A2).
//   - HasSeenArea/MarkAreaSeen — the persisted set of areas ever
//     entered, gating the once-ever first-entry banner (B1).
//
// An actor that does not implement it (test fakes) gets no transition
// lines.
type AreaTracker interface {
	LastAreaSeen() world.AreaID
	SetLastAreaSeen(world.AreaID)
	HasSeenArea(world.AreaID) bool
	MarkAreaSeen(world.AreaID)
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
	return c.Actor.Write(ctx, view)
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
	prev := at.LastAreaSeen()
	if prev == newArea {
		return "" // same area: a look, or an intra-area step
	}
	at.SetLastAreaSeen(newArea)

	var lines []string
	// A2: leave/enter zone-line — only when there is a "from" (not the
	// first render after login, which the spawn render seeds silently).
	if prev != "" {
		lines = append(lines, fmt.Sprintf("<subtle>You leave</subtle> %s <subtle>and enter</subtle> %s<subtle>.</subtle>",
			c.areaName(prev), c.areaName(newArea)))
	}
	// B1: once-ever first-entry banner, below the zone-line. Marking the
	// area seen persists, so it fires exactly once per area ever.
	if !at.HasSeenArea(newArea) {
		at.MarkAreaSeen(newArea)
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

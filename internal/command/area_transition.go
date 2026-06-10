package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// AreaTracker is the optional actor capability that drives area-
// transition messaging (player-maps §4). LastAreaSeen is the
// session-scoped area the actor was last shown — the "from" of the
// leave/enter zone-line; SetLastAreaSeen records the area as each room
// view is written. An actor that does not implement it (test fakes)
// gets no transition lines. The persisted first-entry half
// (HasSeenArea/MarkAreaSeen) lands with B1.
type AreaTracker interface {
	LastAreaSeen() world.AreaID
	SetLastAreaSeen(world.AreaID)
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
	if prev == "" {
		return "" // first render after login — nothing to leave
	}
	return fmt.Sprintf("<subtle>You leave</subtle> %s <subtle>and enter</subtle> %s<subtle>.</subtle>",
		c.areaName(prev), c.areaName(newArea))
}

// areaName resolves an area id to its display name, falling back to the
// raw id when the area is unknown or unnamed.
func (c *Context) areaName(id world.AreaID) string {
	if c.World == nil || id == "" {
		return string(id)
	}
	if a, err := c.World.Area(id); err == nil && a.Name != "" {
		return a.Name
	}
	return string(id)
}

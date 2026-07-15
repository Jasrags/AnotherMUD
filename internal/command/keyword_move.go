package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// traverseKeywordExit relocates the actor through the keyword exit named kw in
// their current room — the player-facing traversal of a keyword exit (a transit
// doorway, transit.md §8; a temporary portal, world-rooms-movement §3.6). It
// runs the same departure/arrival broadcasts, room render, and player.moved
// publish a direction move does, but as a lighter path: a keyword step (boarding
// an elevator, stepping through a portal) is deliberately FREE of the
// movement-cost pool (transit.md §7 — the ride is effortless).
//
// Returns handled=false when no such keyword exit exists in the current room, so
// the caller can report it; true (with any Write error) once it has acted.
func traverseKeywordExit(ctx context.Context, c *Context, kw string) (bool, error) {
	room := c.Actor.Room()
	if room == nil {
		return false, nil
	}
	key := strings.ToLower(strings.TrimSpace(kw))
	// Resolve through MoveByKeyword, which reads KeywordExits under the world's
	// read lock. A direct map index here would race the transit tick handler,
	// which rebinds/unbinds the same doorway map from another goroutine under
	// the world write lock (a concurrent map read/write = fatal crash).
	dst, err := c.World.MoveByKeyword(room.ID, key)
	if err != nil {
		if errors.Is(err, world.ErrNoExit) {
			return false, nil // no such doorway in this room
		}
		return true, c.Actor.Write(ctx, "That way leads nowhere.")
	}
	name := c.Actor.Name()
	pid := c.Actor.PlayerID()
	if c.Broadcaster != nil && name != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID, fmt.Sprintf("%s steps through the %s.", name, key), pid)
	}
	c.Actor.SetRoom(dst)
	if c.Broadcaster != nil && name != "" {
		c.Broadcaster.SendToRoom(ctx, dst.ID, fmt.Sprintf("%s steps in.", name), pid)
	}
	c.Publish(ctx, eventbus.PlayerMoved{PlayerID: pid, From: room.ID, To: dst.ID})
	return true, c.writeRoomView(ctx, dst, c.effectiveLight(dst))
}

// keywordExitsSnapshot is a nil-safe wrapper over World.KeywordExitsSnapshot for
// render paths where the world may be nil (render-only tests). It reads the map
// under the world's read lock, so it is safe against the transit tick handler's
// concurrent doorway rebinds. Returns nil when w is nil.
func keywordExitsSnapshot(w *world.World, room world.RoomID) map[string]world.RoomID {
	if w == nil {
		return nil
	}
	return w.KeywordExitsSnapshot(room)
}

// enterHandler implements `enter <keyword>` / `board <keyword>`: step through a
// named keyword exit (e.g. `enter elevator`). With no argument, or when the room
// has no such doorway, it reports so.
func enterHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Enter what?")
	}
	kw := strings.Join(c.Args, " ")
	handled, err := traverseKeywordExit(ctx, c, kw)
	if err != nil {
		return err
	}
	if !handled {
		return c.Actor.Write(ctx, "You don't see a way to enter that here.")
	}
	return nil
}

// outHandler implements `out` / `exit`: step back out through the "out" keyword
// exit (an elevator car's alight doorway, transit.md §8). Absent while the car
// is between floors — the doors are closed.
func outHandler(ctx context.Context, c *Context) error {
	handled, err := traverseKeywordExit(ctx, c, "out")
	if err != nil {
		return err
	}
	if !handled {
		return c.Actor.Write(ctx, "There's no obvious way out from here.")
	}
	return nil
}

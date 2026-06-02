package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// PlayerRoomResolver finds an online player's current room by name,
// world-wide (admin-verbs §3 — world-scoped admin resolution reaches a
// player regardless of the actor's room). The session Manager satisfies
// it. Handlers MUST tolerate a nil resolver (teleport-to-player is then
// unavailable; teleport-to-room still works).
type PlayerRoomResolver interface {
	ResolvePlayerRoom(name string) (world.RoomID, bool)
}

// TeleportHandler implements `teleport <room-id|player>` (alias `goto`,
// admin-verbs §5): moves the actor to a target room — addressed by room id
// or by the room of a named online player (§3 world-scoped resolution).
// Admin-marked (M19.3 gate); audited via the M19.4a auditAdmin choke point.
//
// v1 moves the ACTOR only. Moving another named player (summon / teleport-
// a-player) is a world-scoped mutation deferred to a later slice.
func TeleportHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Usage: teleport <room-id|player>")
	}
	token := c.Args[0]

	dst, ok := resolveTeleportDest(c, token)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("There is no room or online player named %q.", token))
	}

	src := c.Actor.Room()
	if src != nil && src.ID == dst.ID {
		return c.Actor.Write(ctx, "You're already there.")
	}

	// The move itself. SetRoom updates the room index + persists the new
	// location; the vanish/appear broadcasts mirror the recall teleport's
	// cosmetic layer.
	var srcID world.RoomID
	if src != nil {
		srcID = src.ID
	}
	name := c.Actor.Name()
	pid := c.Actor.PlayerID()
	if c.Broadcaster != nil && src != nil && name != "" {
		c.Broadcaster.SendToRoom(ctx, srcID, fmt.Sprintf("%s disappears in a flash.", name), pid)
	}
	c.Actor.SetRoom(dst)
	if c.Broadcaster != nil && name != "" {
		c.Broadcaster.SendToRoom(ctx, dst.ID, fmt.Sprintf("%s appears in a flash.", name), pid)
	}

	// Publish player.moved — the normal room-change event (§5), mirroring
	// the walk handler — so room-entry subscribers (questwatch, the AI
	// disposition per-room reset) react to the arrival. SetRoom itself does
	// not emit this; the move handlers own it.
	c.Publish(ctx, eventbus.PlayerMoved{PlayerID: pid, From: srcID, To: dst.ID})

	auditAdmin(ctx, c, "teleport", string(dst.ID), token)

	return c.Actor.Write(ctx, RenderRoom(dst, c.Placement, c.Items, c.questMarker(), c.Ambience))
}

// resolveTeleportDest maps the token to a destination room: a literal room
// id first, then — failing that — the room of an online player by name
// (§3). Returns (nil, false) when neither resolves.
func resolveTeleportDest(c *Context, token string) (*world.Room, bool) {
	if c.World != nil {
		if dst, err := c.World.Room(world.RoomID(token)); err == nil {
			return dst, true
		}
	}
	if c.PlayerRoom != nil {
		if rid, ok := c.PlayerRoom.ResolvePlayerRoom(strings.TrimSpace(token)); ok && c.World != nil {
			if dst, err := c.World.Room(rid); err == nil {
				return dst, true
			}
		}
	}
	return nil, false
}

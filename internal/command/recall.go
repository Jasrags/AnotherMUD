package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// recallController is the tiny mutation surface a connActor exposes
// for the `recall set` / `recall` verb pair. Defined here so the
// command package doesn't import session just for two methods. The
// production actor (session.connActor) satisfies it; test fakes that
// don't care about recall don't.
type recallController interface {
	Recall() string
	SetRecall(roomID string)
}

// setRecall implements `recall set` (spec recall.md §2). Binds the
// actor's recall point to their current room.
//
// (M19.4c: the binding verb moved from the former `set recall` to
// `recall set` when the admin field-write verb reclaimed the top-level
// `set` keyword — admin-verbs §4. The behavior is unchanged; only the verb
// surface moved into the `recall` family.)
func setRecall(ctx context.Context, c *Context) error {
	ctrl, ok := c.Actor.(recallController)
	if !ok {
		return c.Actor.Write(ctx, "You can't bind a recall point.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You can't bind a recall point from nowhere.")
	}
	prior := ctrl.Recall()
	id := string(room.ID)
	if prior == id {
		// §2.1 idempotent re-bind in the same room.
		return c.Actor.Write(ctx, "Your recall is already bound here.")
	}
	ctrl.SetRecall(id)
	c.Publish(ctx, eventbus.RecallSet{
		PlayerID: c.Actor.PlayerID(),
		RoomID:   room.ID,
	})
	logging.From(ctx).Debug("recall set",
		slog.String("player", c.Actor.PlayerID()),
		slog.String("room", id))
	return c.Actor.Write(ctx, "You bind your recall to this place.")
}

// RecallHandler implements `recall` (spec recall.md §3) and routes the
// `recall set` binding sub-form (§2). Bare `recall` teleports the actor to
// the bound recall room if one is set, resolvable, and the pre-event is not
// cancelled; `recall set` binds the current room as the recall point.
func RecallHandler(ctx context.Context, c *Context) error {
	// `recall set` — bind the current room (spec recall.md §2). Any other
	// trailing token is ignored and treated as a bare recall.
	if len(c.Args) > 0 && strings.EqualFold(c.Args[0], "set") {
		return setRecall(ctx, c)
	}

	ctrl, ok := c.Actor.(recallController)
	if !ok {
		return c.Actor.Write(ctx, "You don't know how to recall.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You can't recall from nowhere.")
	}
	saved := ctrl.Recall()
	if saved == "" {
		c.Publish(ctx, eventbus.RecallNoPoint{PlayerID: c.Actor.PlayerID()})
		return c.Actor.Write(ctx,
			"You have no recall point set. Use `recall set` somewhere first.")
	}

	// §3.1 step 3 — saved id must still resolve in the world.
	dstID := world.RoomID(saved)
	dst, err := c.World.Room(dstID)
	if err != nil {
		c.Publish(ctx, eventbus.RecallUnresolved{
			PlayerID:    c.Actor.PlayerID(),
			MissingRoom: dstID,
		})
		logging.From(ctx).Info("recall unresolved",
			slog.String("player", c.Actor.PlayerID()),
			slog.String("missing_room", saved),
			slog.Any("err", err))
		return c.Actor.Write(ctx, "Your recall point is no longer there.")
	}

	// §3.1 step 4 — same-room no-op. No events, no broadcasts; the
	// actor stays put with a confirmation line. The
	// RecallSamePointFires config in §7 is deferred.
	if room.ID == dstID {
		return c.Actor.Write(ctx, "You are already at your recall point.")
	}

	// §3.1 step 5 — cancellable pre-event.
	pre := eventbus.NewRecallBefore(c.Actor.PlayerID(), room.ID, dstID)
	cancelled := false
	if c.Bus != nil {
		cancelled = c.Bus.PublishCancellable(ctx, pre)
	}
	logging.From(ctx).Debug("recall before",
		slog.String("player", c.Actor.PlayerID()),
		slog.String("from", string(room.ID)),
		slog.String("to", string(dstID)),
		slog.Bool("cancelled", cancelled))
	if cancelled {
		return c.Actor.Write(ctx, "You can't recall right now.")
	}

	// §3.2 — source-room "vanishes" broadcast, before the teleport
	// so it announces in the room the actor is actually leaving.
	srcID := room.ID
	name := c.Actor.Name()
	pid := c.Actor.PlayerID()
	if c.Broadcaster != nil && name != "" {
		c.Broadcaster.SendToRoom(ctx, srcID,
			fmt.Sprintf("%s vanishes.", name), pid)
	}

	// §3.1 step 6 — commit. SetRoom updates a.room, persists
	// save.Location, and publishes player.moved through the
	// existing room-change path (§3.3 — exactly one player.moved
	// per recall).
	c.Actor.SetRoom(dst)

	// Destination-room "appears" broadcast.
	if c.Broadcaster != nil && name != "" {
		c.Broadcaster.SendToRoom(ctx, dst.ID,
			fmt.Sprintf("%s appears in a swirl of light.", name), pid)
	}

	// Render the destination to the actor. RenderRoom is the same
	// renderer movement uses so the post-recall view is consistent.
	if err := c.Actor.Write(ctx, RenderRoom(dst, c.Placement, c.Items, c.questMarker(), c.Ambience, c.hostileMarker(), c.otherPlayerNames(dst.ID)...)); err != nil {
		// Surface the write error so the dispatcher can decide
		// (e.g., a closed connection bubbles up the same way
		// movement handlers surface it).
		return fmt.Errorf("recall render: %w", err)
	}

	// §3.1 step 7 — post-fact event.
	c.Publish(ctx, eventbus.RecallAfter{
		PlayerID: pid,
		From:     srcID,
		To:       dst.ID,
	})
	return nil
}

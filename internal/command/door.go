package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// OpenHandler implements `open <target>` (M15.1). The target is a
// direction or a door keyword (with optional ordinal); the world's
// ResolveDoorTarget produces a Direction. A locked door cannot be
// opened — the verb routes through UnlockDoor implicitly only when
// the player explicitly types `unlock`.
//
// Spec: docs/specs/world-rooms-movement.md §5.2 (Open operation),
// §5.5 (target resolution).
func OpenHandler(ctx context.Context, c *Context) error {
	return doorOpHandler(ctx, c, "open")
}

// CloseHandler implements `close <target>`.
func CloseHandler(ctx context.Context, c *Context) error {
	return doorOpHandler(ctx, c, "close")
}

// LockHandler implements `lock <target>`. Requires the actor to
// hold the door's key item (when the door declares a KeyID).
func LockHandler(ctx context.Context, c *Context) error {
	return doorOpHandler(ctx, c, "lock")
}

// UnlockHandler implements `unlock <target>`. Same key-check as
// Lock.
func UnlockHandler(ctx context.Context, c *Context) error {
	return doorOpHandler(ctx, c, "unlock")
}

// doorOpHandler is the shared verb implementation. The op string
// is the verb's name; chosen so the user-facing copy reads
// naturally without a switch in every error path.
func doorOpHandler(ctx context.Context, c *Context, op string) error {
	if c.World == nil {
		return c.Actor.Write(ctx, "There is nothing here to "+op+".")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see nothing here.")
	}

	// M17.2c/d: the `door` arg resolved the target (a direction or door
	// keyword, with optional ordinal) before this runs — missing-arg,
	// ambiguous, and not-found are reported by the dispatcher with the
	// §5.4 / door sentinels. We parse the resolved short direction back
	// to a world.Direction and re-fetch the LIVE DoorState: the snapshot
	// the resolver took may be stale, and the per-op checks below want
	// current state (mirrors the old GetDoor-before-switch behavior).
	ref, ok := c.Resolved["door"].(DoorRef)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't see anything to %s there.", op))
	}
	dir, ok := world.ParseDirection(ref.Direction)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't see anything to %s there.", op))
	}
	door, ok := c.World.GetDoor(room.ID, dir)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't see anything to %s there.", op))
	}

	switch op {
	case "open":
		return handleOpen(ctx, c, room.ID, dir, door)
	case "close":
		return handleClose(ctx, c, room.ID, dir, door)
	case "lock":
		return handleLock(ctx, c, room.ID, dir, door)
	case "unlock":
		return handleUnlock(ctx, c, room.ID, dir, door)
	default:
		return c.Actor.Write(ctx, "Huh?")
	}
}

func handleOpen(ctx context.Context, c *Context, src world.RoomID, dir world.Direction, door world.DoorState) error {
	if door.Locked {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is locked.", capitalize(door.Name)))
	}
	if !door.Closed {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is already open.", capitalize(door.Name)))
	}
	if !c.World.OpenDoor(src, dir) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s won't budge.", capitalize(door.Name)))
	}
	c.Publish(ctx, eventbus.DoorOpened{DoorEvent: doorEvent(c, src, dir, door, "")})
	return c.Actor.Write(ctx, fmt.Sprintf("You open %s.", door.Name))
}

func handleClose(ctx context.Context, c *Context, src world.RoomID, dir world.Direction, door world.DoorState) error {
	if door.Closed {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is already closed.", capitalize(door.Name)))
	}
	if !c.World.CloseDoor(src, dir) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s won't budge.", capitalize(door.Name)))
	}
	c.Publish(ctx, eventbus.DoorClosed{DoorEvent: doorEvent(c, src, dir, door, "")})
	return c.Actor.Write(ctx, fmt.Sprintf("You close %s.", door.Name))
}

func handleLock(ctx context.Context, c *Context, src world.RoomID, dir world.Direction, door world.DoorState) error {
	// Policy (world-rooms-movement §5.3): a door is lockable only if it
	// has a lock — i.e. it declares a key. A keyless door is a plain door,
	// not a free latch, so refuse before the close/lock-state checks.
	if door.KeyID == "" {
		return c.Actor.Write(ctx, fmt.Sprintf("There's no lock on %s.", door.Name))
	}
	if !door.Closed {
		return c.Actor.Write(ctx, fmt.Sprintf("You'll need to close %s first.", door.Name))
	}
	if door.Locked {
		return c.Actor.Write(ctx, fmt.Sprintf("%s is already locked.", capitalize(door.Name)))
	}
	if !actorHasKey(c, door.KeyID) {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't have a key for %s.", door.Name))
	}
	if !c.World.LockDoor(src, dir) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s won't lock.", capitalize(door.Name)))
	}
	c.Publish(ctx, eventbus.DoorLocked{DoorEvent: doorEvent(c, src, dir, door, door.KeyID)})
	return c.Actor.Write(ctx, fmt.Sprintf("You lock %s.", door.Name))
}

func handleUnlock(ctx context.Context, c *Context, src world.RoomID, dir world.Direction, door world.DoorState) error {
	// A keyless door has no lock to work (mirror of handleLock policy).
	if door.KeyID == "" {
		return c.Actor.Write(ctx, fmt.Sprintf("There's no lock on %s.", door.Name))
	}
	if !door.Locked {
		return c.Actor.Write(ctx, fmt.Sprintf("%s isn't locked.", capitalize(door.Name)))
	}
	if !actorHasKey(c, door.KeyID) {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't have a key for %s.", door.Name))
	}
	if !c.World.UnlockDoor(src, dir) {
		return c.Actor.Write(ctx, fmt.Sprintf("%s won't unlock.", capitalize(door.Name)))
	}
	c.Publish(ctx, eventbus.DoorUnlocked{DoorEvent: doorEvent(c, src, dir, door, door.KeyID)})
	return c.Actor.Write(ctx, fmt.Sprintf("You unlock %s.", door.Name))
}

// doorEvent builds the shared DoorEvent payload for the five door
// lifecycle events. KeyID is only meaningful on lock / unlock; the
// open / close / blocked builders pass an empty string.
func doorEvent(c *Context, src world.RoomID, dir world.Direction, door world.DoorState, keyID string) eventbus.DoorEvent {
	return eventbus.DoorEvent{
		RoomID:    src,
		Direction: dir.Short(),
		ActorID:   entities.EntityID(c.Actor.PlayerID()),
		DoorName:  door.Name,
		KeyID:     keyID,
	}
}

// actorHasKey reports whether the actor's inventory carries any
// item whose template id equals keyID (case-insensitive) OR whose
// `key_for` property names a door that resolves to keyID. The
// first form is the spec's literal §5.3 check; the second is the
// PD-4 hook so content can declare a key with a property rather
// than expecting the door to name an item by template id.
//
// Returns false when Items / Templates are unwired (test envs) —
// the verb's caller renders a clear "no key" message.
func actorHasKey(c *Context, keyID string) bool {
	if c.Items == nil || keyID == "" {
		return false
	}
	wantTpl := item.TemplateID(strings.ToLower(strings.TrimSpace(keyID)))
	for _, id := range c.Actor.Inventory() {
		ent, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := ent.(*entities.ItemInstance)
		if !ok {
			continue
		}
		// Direct template id match (spec §5.3).
		if it.TemplateID() == wantTpl {
			return true
		}
		// PD-4 property hook: an item declares `key_for: <door-id>`
		// to act as a key for any door whose KeyID matches.
		if pv, ok := it.Property("key_for"); ok {
			if s, _ := pv.(string); strings.EqualFold(s, keyID) {
				return true
			}
		}
	}
	return false
}

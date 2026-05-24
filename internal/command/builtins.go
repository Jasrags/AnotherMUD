package command

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// RegisterBuiltins binds the M1 verbs into r: look, quit, and one
// keyword per movement direction (long + short form). Movement uses
// world.World.Move; look renders the actor's current room.
func RegisterBuiltins(r *Registry) error {
	bindings := []struct {
		key string
		h   Handler
	}{
		{"look", LookHandler},
		{"quit", QuitHandler},
		{"color", ColorHandler},
		{"get", GetHandler},
		{"drop", DropHandler},
	}
	for _, d := range []world.Direction{
		world.DirNorth, world.DirSouth, world.DirEast, world.DirWest,
		world.DirUp, world.DirDown,
	} {
		dir := d
		mh := movementHandler(dir)
		bindings = append(bindings,
			struct {
				key string
				h   Handler
			}{dir.Long(), mh},
			struct {
				key string
				h   Handler
			}{dir.Short(), mh},
		)
	}
	for _, b := range bindings {
		if err := r.Register(b.key, b.h); err != nil {
			return err
		}
	}
	return nil
}

// LookHandler renders the actor's current room.
func LookHandler(ctx context.Context, c *Context) error {
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You float in formless void.")
	}
	return c.Actor.Write(ctx, RenderRoom(room))
}

// ColorHandler implements the `color` verb (spec ui-rendering-help —
// color subset). With no argument it reports the current state; with
// "on"/"off" it toggles the per-actor flag.
func ColorHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		state := "off"
		if c.Actor.ColorEnabled() {
			state = "on"
		}
		return c.Actor.Write(ctx, "Color is currently "+state+". Use 'color on' or 'color off'.")
	}
	switch strings.ToLower(c.Args[0]) {
	case "on":
		c.Actor.SetColorEnabled(true)
		// Confirm in color so the user sees it took effect; the auto-reset
		// in ansi.Render closes the sequence cleanly.
		return c.Actor.Write(ctx, "{G}Color enabled.{x}")
	case "off":
		c.Actor.SetColorEnabled(false)
		return c.Actor.Write(ctx, "Color disabled.")
	default:
		return c.Actor.Write(ctx, "Usage: color [on|off]")
	}
}

// QuitHandler signals the session loop to disconnect cleanly.
//
// The farewell Write error is intentionally discarded: ErrQuit drives
// the session loop to close the connection regardless of whether the
// peer received the goodbye, and surfacing a write failure here would
// only escalate a benign condition (peer already gone) into a warning
// in the connection's tear-down path.
func QuitHandler(ctx context.Context, c *Context) error {
	_ = c.Actor.Write(ctx, "Goodbye.")
	return ErrQuit
}

func movementHandler(dir world.Direction) Handler {
	return func(ctx context.Context, c *Context) error {
		room := c.Actor.Room()
		if room == nil {
			return c.Actor.Write(ctx, "You cannot move from nowhere.")
		}
		dst, err := c.World.Move(room.ID, dir)
		if err != nil {
			if errors.Is(err, world.ErrNoExit) {
				return c.Actor.Write(ctx, "You cannot go that way.")
			}
			return c.Actor.Write(ctx, "Something blocks your way.")
		}
		srcID := room.ID
		name := c.Actor.Name()
		pid := c.Actor.PlayerID()
		// Announce departure to the source room before the actor
		// leaves so other occupants there see it. Broadcaster is
		// optional (tests pass nil); skip the announcement when name
		// or PlayerID is empty (test actors that don't participate in
		// presence).
		if c.Broadcaster != nil && name != "" {
			c.Broadcaster.SendToRoom(ctx, srcID,
				fmt.Sprintf("%s heads %s.", name, dir.Long()), pid)
		}
		c.Actor.SetRoom(dst)
		if c.Broadcaster != nil && name != "" {
			from := dir.Opposite().Long()
			if from == "" {
				from = "elsewhere"
			}
			c.Broadcaster.SendToRoom(ctx, dst.ID,
				fmt.Sprintf("%s arrives from the %s.", name, from), pid)
		}
		return c.Actor.Write(ctx, RenderRoom(dst))
	}
}

// RenderRoom is the M1 room renderer. Replaced by the ui-rendering-help
// pipeline in a later milestone; lives here for now so the session
// layer has something to call.
func RenderRoom(r *world.Room) string {
	var b strings.Builder
	b.WriteString(r.Name)
	b.WriteString("\n")
	b.WriteString(r.Description)
	b.WriteString("\n")
	b.WriteString(renderExits(r))
	return b.String()
}

func renderExits(r *world.Room) string {
	if len(r.Exits) == 0 {
		return "Exits: none"
	}
	dirs := make([]string, 0, len(r.Exits))
	for d := range r.Exits {
		dirs = append(dirs, d.Long())
	}
	sort.Strings(dirs)
	return fmt.Sprintf("Exits: %s", strings.Join(dirs, ", "))
}

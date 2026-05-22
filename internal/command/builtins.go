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

// QuitHandler signals the session loop to disconnect cleanly.
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
		c.Actor.SetRoom(dst)
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

package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/emote"
)

// MakeEmoteHandler returns a verb handler for a registered emote.
// The composition root calls this once per emote.Registry entry to
// install the per-emote verb (smile, nod, wave, …).
//
// Spec: docs/specs/emotes.md §4.
func MakeEmoteHandler(e *emote.Emote) func(context.Context, *Context) error {
	return func(ctx context.Context, c *Context) error {
		// No target: render the no_target form (when allowed) or
		// reject (when RequiresTarget).
		if len(c.Args) == 0 {
			if e.RequiresTarget {
				return c.Actor.Write(ctx, fmt.Sprintf("%s whom?", e.DisplayName))
			}
			return renderNoTarget(ctx, c, e)
		}
		target := strings.Join(c.Args, " ")
		return renderTargeted(ctx, c, e, target)
	}
}

// EmoteFreeformHandler implements `emote <text>` (spec §4.1). The
// actor's name is prepended to the supplied text and broadcast as
// the room view; the actor sees the same line as confirmation.
func EmoteFreeformHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Emote what?")
	}
	text := strings.TrimSpace(strings.Join(c.Args, " "))
	if text == "" {
		return c.Actor.Write(ctx, "Emote what?")
	}
	line := fmt.Sprintf("%s %s", c.Actor.Name(), text)
	// Actor sees the same line as observers (confirmation copy).
	if err := c.Actor.Write(ctx, line); err != nil {
		return err
	}
	if c.Broadcaster != nil && c.Actor.Room() != nil {
		c.Broadcaster.SendToRoom(ctx, c.Actor.Room().ID, line, c.Actor.PlayerID())
	}
	return nil
}

// renderNoTarget builds and dispatches the actor + room views of a
// non-targeted emote (e.g., `smile` with no argument).
func renderNoTarget(ctx context.Context, c *Context, e *emote.Emote) error {
	actor := emote.Subject{Name: c.Actor.Name(), Pronouns: emote.DefaultPronouns}
	if err := c.Actor.Write(ctx, emote.Substitute(e.NoTarget.ActorView, actor, emote.Subject{})); err != nil {
		return err
	}
	roomLine := emote.Substitute(e.NoTarget.RoomView, actor, emote.Subject{})
	if c.Broadcaster != nil && c.Actor.Room() != nil {
		c.Broadcaster.SendToRoom(ctx, c.Actor.Room().ID, roomLine, c.Actor.PlayerID())
	}
	return nil
}

// renderTargeted resolves the target, builds the three views, and
// dispatches. Players and mobs are valid v1 targets; items defer to
// M13.7b.
func renderTargeted(ctx context.Context, c *Context, e *emote.Emote, targetArg string) error {
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see nothing here.")
	}

	actor := emote.Subject{Name: c.Actor.Name(), Pronouns: emote.DefaultPronouns}

	var (
		target         emote.Subject
		targetActor    Actor // populated when target is a player
		targetPlayerID string
		resolved       bool
		isSelf         bool
	)

	// Self-reference resolves to the actor (spec §5).
	if isSelfReference(c.Actor.Name(), targetArg) {
		target = actor
		targetActor = c.Actor
		targetPlayerID = c.Actor.PlayerID()
		resolved = true
		isSelf = true
	}

	// Player in room (case-insensitive name match via the Locator).
	if !resolved && c.Locator != nil {
		if other := c.Locator.FindInRoom(room.ID, targetArg); other != nil {
			target = emote.Subject{Name: other.Name(), Pronouns: emote.DefaultPronouns}
			targetActor = other
			targetPlayerID = other.PlayerID()
			resolved = true
		}
	}

	// Mob in room (Placement-tracked keyword resolver).
	if !resolved {
		if mob := findMobByKeyword(c, room.ID, targetArg); mob != nil {
			target = emote.Subject{Name: mob.Name(), Pronouns: emote.DefaultPronouns}
			resolved = true
		}
	}

	if !resolved {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't see %q here.", targetArg))
	}

	// Actor view always goes to the actor.
	if err := c.Actor.Write(ctx, emote.Substitute(e.Targeted.ActorView, actor, target)); err != nil {
		return err
	}

	// Target view: only if the target is a separate player session.
	// Self-target produces only actor + room; spec §5 says authors
	// write templates that read sensibly for self-reference.
	if !isSelf && targetActor != nil {
		_ = targetActor.Write(ctx, emote.Substitute(e.Targeted.TargetView, actor, target))
	}

	// Room view goes to everyone else in the room, excluding
	// actor and (if applicable) the target player.
	if c.Broadcaster != nil {
		roomLine := emote.Substitute(e.Targeted.RoomView, actor, target)
		excludes := []string{c.Actor.PlayerID()}
		if !isSelf && targetPlayerID != "" {
			excludes = append(excludes, targetPlayerID)
		}
		c.Broadcaster.SendToRoom(ctx, room.ID, roomLine, excludes...)
	}
	return nil
}

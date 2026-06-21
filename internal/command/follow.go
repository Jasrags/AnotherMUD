package command

import (
	"context"
	"errors"
	"fmt"
)

// Follow errors the FollowService returns; the verbs map them to player text.
var (
	// ErrFollowSelf — you tried to follow yourself.
	ErrFollowSelf = errors.New("follow self")
	// ErrFollowCycle — the follow would close a loop (you'd transitively follow
	// someone who already follows you).
	ErrFollowCycle = errors.New("follow cycle")
)

// FollowService is the engine's move-with-leader relationship graph (follow.md).
// The session Manager implements it; the verbs call it. All ids are player ids.
// nil disables following (the verbs then refuse cleanly).
type FollowService interface {
	// Follow records followerID trailing leaderID, replacing any prior leader.
	// Returns ErrFollowSelf / ErrFollowCycle on the degenerate cases.
	Follow(followerID, leaderID string) error
	// Unfollow ends followerID's relationship, returning the former leader id
	// and whether there was one.
	Unfollow(followerID string) (leaderID string, had bool)
	// Lose drops every follower of leaderID, returning their ids (to notify).
	Lose(leaderID string) []string
	// Following reports followerID's current leader, if any.
	Following(followerID string) (leaderID string, had bool)
}

// FollowHandler implements `follow [<target>]` (follow.md §2): with a target, a
// visible player in the room, begin trailing them; with no target, report the
// current leader. Consent-free — the target isn't asked (the leader uses `lose`).
func FollowHandler(ctx context.Context, c *Context) error {
	if c.Follow == nil {
		return c.Actor.Write(ctx, "You can't follow anyone right now.")
	}
	tref, ok := c.Resolved["target"].(EntityRef)
	if !ok {
		// No argument — report who we follow.
		if lid, has := c.Follow.Following(c.Actor.PlayerID()); has {
			return c.Actor.Write(ctx, fmt.Sprintf("You are following %s.", c.actorName(lid)))
		}
		return c.Actor.Write(ctx, "You aren't following anyone.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You're nowhere to follow from.")
	}
	target := c.Locator.FindInRoom(room.ID, tref.Name)
	if target == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("%q isn't here.", tref.Name))
	}
	switch err := c.Follow.Follow(c.Actor.PlayerID(), target.PlayerID()); {
	case errors.Is(err, ErrFollowSelf):
		return c.Actor.Write(ctx, "You can't follow yourself.")
	case errors.Is(err, ErrFollowCycle):
		return c.Actor.Write(ctx, fmt.Sprintf("You can't follow %s — they're already following you.", target.Name()))
	case err != nil:
		return c.Actor.Write(ctx, "You can't follow that.")
	}
	_ = target.Write(ctx, fmt.Sprintf("%s begins following you.", c.Actor.Name()))
	return c.Actor.Write(ctx, fmt.Sprintf("You begin following %s.", target.Name()))
}

// UnfollowHandler implements `unfollow` (follow.md §2): stop trailing your
// current leader.
func UnfollowHandler(ctx context.Context, c *Context) error {
	if c.Follow == nil {
		return c.Actor.Write(ctx, "You aren't following anyone.")
	}
	leaderID, had := c.Follow.Unfollow(c.Actor.PlayerID())
	if !had {
		return c.Actor.Write(ctx, "You aren't following anyone.")
	}
	if la, ok := c.actorByID(leaderID); ok {
		_ = la.Write(ctx, fmt.Sprintf("%s stops following you.", c.Actor.Name()))
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You stop following %s.", c.actorName(leaderID)))
}

// LoseHandler implements `lose` (follow.md §2): shake off everyone following you.
func LoseHandler(ctx context.Context, c *Context) error {
	if c.Follow == nil {
		return c.Actor.Write(ctx, "No one is following you.")
	}
	lost := c.Follow.Lose(c.Actor.PlayerID())
	if len(lost) == 0 {
		return c.Actor.Write(ctx, "No one is following you.")
	}
	for _, id := range lost {
		if fa, ok := c.actorByID(id); ok {
			_ = fa.Write(ctx, fmt.Sprintf("%s slips away, and you lose the trail.", c.Actor.Name()))
		}
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You shake off %s.", pluralFollowers(len(lost))))
}

// actorName resolves a player id to a display name for follow messaging, falling
// back to "someone" when the actor can't be located (offline/edge).
func (c *Context) actorName(id string) string {
	if a, ok := c.actorByID(id); ok {
		return a.Name()
	}
	return "someone"
}

func (c *Context) actorByID(id string) (Actor, bool) {
	if c.ActorByID == nil || id == "" {
		return nil, false
	}
	return c.ActorByID(id)
}

func pluralFollowers(n int) string {
	if n == 1 {
		return "your follower"
	}
	return fmt.Sprintf("your %d followers", n)
}

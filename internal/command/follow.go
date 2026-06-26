package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/entities"
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
// The session Manager implements it; the verbs call it. Follower ids are always
// player ids; a leader id may be a player id OR a mob entity id (follow.md §3 —
// the two spaces never collide: player ids are hex, mob ids are "entity-N").
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

// FollowHandler implements `follow [<target>]` (follow.md §2): with a target —
// a visible player OR mob in the room — begin trailing them; with no target,
// report the current leader. Consent-free — the target isn't asked (a player
// leader uses `lose`; a mob can't be asked at all). A mob leader (follow.md §3)
// is keyed in the follow graph by its entity id and reacts to MobMoved exactly
// as a player leader reacts to PlayerMoved.
func FollowHandler(ctx context.Context, c *Context) error {
	if c.Follow == nil {
		return c.Actor.Write(ctx, "You can't follow anyone right now.")
	}
	tref, ok := c.Resolved["target"].(EntityRef)
	if !ok {
		// No argument — report who we follow.
		if lid, has := c.Follow.Following(c.Actor.PlayerID()); has {
			return c.Actor.Write(ctx, fmt.Sprintf("You are following %s.", c.leaderName(lid)))
		}
		return c.Actor.Write(ctx, "You aren't following anyone.")
	}
	if tref.Type == entityTypeMob {
		return followMob(ctx, c, tref)
	}
	return followPlayer(ctx, c, tref)
}

// followMob begins trailing a mob (follow.md §3). The mob's entity id is the
// leader key; the mob isn't notified (it has no session). A cycle is impossible
// (a mob never follows), so only the generic-failure branch can fire.
func followMob(ctx context.Context, c *Context, tref EntityRef) error {
	if tref.ID == "" {
		return c.Actor.Write(ctx, "You can't follow that.")
	}
	if err := c.Follow.Follow(c.Actor.PlayerID(), tref.ID); err != nil {
		return c.Actor.Write(ctx, "You can't follow that.")
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You begin following %s.", tref.Name))
}

// followPlayer begins trailing another player, notifying that player (the
// original follow.md §2 behavior).
func followPlayer(ctx context.Context, c *Context, tref EntityRef) error {
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
	return c.Actor.Write(ctx, fmt.Sprintf("You stop following %s.", c.leaderName(leaderID)))
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

// leaderName resolves a follow-leader id to a display name. A leader is either
// a player (an online actor) or a mob the player is trailing (follow.md §3), so
// it tries the actor lookup first, then the entity store for a mob name, then
// the generic fallback. The two id spaces never collide (player ids are hex,
// mob entity ids are "entity-N").
func (c *Context) leaderName(id string) string {
	if a, ok := c.actorByID(id); ok {
		return a.Name()
	}
	if c.Items != nil && id != "" {
		if e, ok := c.Items.GetByID(entities.EntityID(id)); ok {
			if mob, ok := e.(*entities.MobInstance); ok {
				return mob.Name()
			}
		}
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

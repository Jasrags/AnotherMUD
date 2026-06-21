package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Group errors the GroupService returns; the verbs map them to player text.
var (
	ErrGroupSelf       = errors.New("group self")          // invited yourself
	ErrGroupCapFull    = errors.New("group full")          // party at the size cap
	ErrGroupHasParty   = errors.New("group target busy")   // target already grouped
	ErrGroupNoInvite   = errors.New("group no invite")     // accept with no standing invite
	ErrGroupInviterBad = errors.New("group inviter party") // inviter already in someone else's party
)

// GroupService is the engine's party roster (grouping.md). The session Manager
// implements it; the verbs call it. All ids are player ids. nil disables
// grouping (the verbs refuse cleanly).
type GroupService interface {
	// Invite records a pending invitation from leaderID to inviteeID, creating
	// leaderID's party (leader = leaderID) if they have none.
	Invite(leaderID, inviteeID string) error
	// Accept consumes inviteeID's pending invite from leaderID, adding them.
	Accept(inviteeID, leaderID string) error
	// Leave removes memberID from their party. disbanded is true when the party
	// dissolved (the leader left, or it dropped to one); others is everyone else
	// who was in the party (to notify); had reports whether they were grouped.
	Leave(memberID string) (disbanded bool, others []string, had bool)
	// Disband dissolves leaderID's party (only if they are the leader),
	// returning the other members to notify.
	Disband(leaderID string) (others []string, ok bool)
	// Members returns the party member ids (including playerID), or nil when
	// ungrouped.
	Members(playerID string) []string
	// LeaderOf returns playerID's party leader and whether they're grouped.
	LeaderOf(playerID string) (leaderID string, inParty bool)
}

// GroupHandler implements `group [<player>]` (grouping.md §2): with a player, a
// visible target in the room, invite them (creating your party if needed); with
// no argument, list your party.
func GroupHandler(ctx context.Context, c *Context) error {
	if c.Group == nil {
		return c.Actor.Write(ctx, "You can't form a party right now.")
	}
	tref, ok := c.Resolved["target"].(EntityRef)
	if !ok {
		return c.listParty(ctx)
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You're nowhere to gather a party.")
	}
	target := c.Locator.FindInRoom(room.ID, tref.Name)
	if target == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("%q isn't here.", tref.Name))
	}
	switch err := c.Group.Invite(c.Actor.PlayerID(), target.PlayerID()); {
	case errors.Is(err, ErrGroupSelf):
		return c.Actor.Write(ctx, "You can't invite yourself to a party.")
	case errors.Is(err, ErrGroupCapFull):
		return c.Actor.Write(ctx, "Your party is full.")
	case errors.Is(err, ErrGroupHasParty):
		return c.Actor.Write(ctx, fmt.Sprintf("%s is already in a party.", target.Name()))
	case errors.Is(err, ErrGroupInviterBad):
		return c.Actor.Write(ctx, "Only your party's leader can invite others.")
	case err != nil:
		return c.Actor.Write(ctx, "You can't invite them.")
	}
	_ = target.Write(ctx, fmt.Sprintf("%s invites you to their party. Type `join %s` to accept.",
		c.Actor.Name(), c.Actor.Name()))
	return c.Actor.Write(ctx, fmt.Sprintf("You invite %s to your party.", target.Name()))
}

// JoinHandler implements `join <leader>` (grouping.md §2): accept a pending party
// invitation. Hand-parsed — the leader is named by keyword, resolved against the
// room (the inviter is typically present) or any online player by exact name.
func JoinHandler(ctx context.Context, c *Context) error {
	if c.Group == nil {
		return c.Actor.Write(ctx, "You can't join a party right now.")
	}
	name := strings.TrimSpace(strings.Join(c.Args, " "))
	if name == "" {
		return c.Actor.Write(ctx, "Join whose party?  (try: join <leader>)")
	}
	leader := c.resolveByName(name)
	if leader == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("%q isn't here to join.", name))
	}
	switch err := c.Group.Accept(c.Actor.PlayerID(), leader.PlayerID()); {
	case errors.Is(err, ErrGroupNoInvite):
		return c.Actor.Write(ctx, fmt.Sprintf("%s hasn't invited you to a party.", leader.Name()))
	case errors.Is(err, ErrGroupCapFull):
		return c.Actor.Write(ctx, fmt.Sprintf("%s's party is full.", leader.Name()))
	case errors.Is(err, ErrGroupHasParty):
		return c.Actor.Write(ctx, "You're already in a party — `leave` it first.")
	case err != nil:
		return c.Actor.Write(ctx, "You can't join that party.")
	}
	// Announce to the party (including the leader).
	for _, id := range c.Group.Members(c.Actor.PlayerID()) {
		if id == c.Actor.PlayerID() {
			continue
		}
		if a, ok := c.actorByID(id); ok {
			_ = a.Write(ctx, fmt.Sprintf("%s joins the party.", c.Actor.Name()))
		}
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You join %s's party.", leader.Name()))
}

// LeaveHandler implements `leave` / `ungroup` (grouping.md §2/§3).
func LeaveHandler(ctx context.Context, c *Context) error {
	if c.Group == nil {
		return c.Actor.Write(ctx, "You aren't in a party.")
	}
	disbanded, others, had := c.Group.Leave(c.Actor.PlayerID())
	if !had {
		return c.Actor.Write(ctx, "You aren't in a party.")
	}
	noun := c.Actor.Name() + " leaves the party."
	if disbanded {
		noun = "The party disbands."
	}
	for _, id := range others {
		if a, ok := c.actorByID(id); ok {
			_ = a.Write(ctx, noun)
		}
	}
	return c.Actor.Write(ctx, "You leave the party.")
}

// DisbandHandler implements `disband` (grouping.md §3): the leader dissolves it.
func DisbandHandler(ctx context.Context, c *Context) error {
	if c.Group == nil {
		return c.Actor.Write(ctx, "You aren't leading a party.")
	}
	others, ok := c.Group.Disband(c.Actor.PlayerID())
	if !ok {
		return c.Actor.Write(ctx, "You aren't leading a party.")
	}
	for _, id := range others {
		if a, ok := c.actorByID(id); ok {
			_ = a.Write(ctx, "Your party leader disbands the party.")
		}
	}
	return c.Actor.Write(ctx, "You disband your party.")
}

// GtellHandler implements `gtell <message>` (grouping.md §6): the party channel.
func GtellHandler(ctx context.Context, c *Context) error {
	if c.Group == nil {
		return c.Actor.Write(ctx, "You have no party to talk to.")
	}
	msg := strings.TrimSpace(strings.Join(c.Args, " "))
	if msg == "" {
		return c.Actor.Write(ctx, "Tell your party what?")
	}
	members := c.Group.Members(c.Actor.PlayerID())
	if len(members) == 0 {
		return c.Actor.Write(ctx, "You have no party to talk to.")
	}
	for _, id := range members {
		if id == c.Actor.PlayerID() {
			continue
		}
		if a, ok := c.actorByID(id); ok {
			_ = a.Write(ctx, fmt.Sprintf("[party] %s: %s", c.Actor.Name(), msg))
		}
	}
	return c.Actor.Write(ctx, fmt.Sprintf("[party] You: %s", msg))
}

// listParty renders the caller's party roster (grouping.md §2).
func (c *Context) listParty(ctx context.Context) error {
	members := c.Group.Members(c.Actor.PlayerID())
	if len(members) == 0 {
		return c.Actor.Write(ctx, "You aren't in a party.")
	}
	leaderID, _ := c.Group.LeaderOf(c.Actor.PlayerID())
	var b strings.Builder
	b.WriteString("Your party:\n")
	for _, id := range members {
		tag := ""
		if id == leaderID {
			tag = " (leader)"
		}
		b.WriteString("  " + c.actorName(id) + tag + "\n")
	}
	return c.Actor.Write(ctx, strings.TrimRight(b.String(), "\n"))
}

// resolveByName finds an online actor by name in the caller's room (the inviter
// is normally still present to accept). Room-scoped by design — if the inviter
// walked off, `join` reports they aren't here. Used by `join`.
func (c *Context) resolveByName(name string) Actor {
	if c.Locator == nil {
		return nil
	}
	room := c.Actor.Room()
	if room == nil {
		return nil
	}
	return c.Locator.FindInRoom(room.ID, name)
}

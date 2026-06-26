package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Group errors the GroupService returns; the verbs map them to player text.
var (
	ErrGroupSelf           = errors.New("group self")            // invited yourself
	ErrGroupCapFull        = errors.New("group full")            // party at the size cap
	ErrGroupHasParty       = errors.New("group target busy")     // target already grouped
	ErrGroupNoInvite       = errors.New("group no invite")       // accept with no standing invite
	ErrGroupInviterBad     = errors.New("group inviter party")   // inviter already in someone else's party
	ErrGroupNotLeader      = errors.New("group not leader")      // a non-leader tried a leader-only action
	ErrLootMasterNotMember = errors.New("loot master nonmember") // designated master isn't in the party
	ErrGroupPromoteTarget  = errors.New("group promote target")  // promote target isn't a member (or is self)
)

// LootMode is a party's loot-distribution policy (grouping.md §9). It governs
// the corpse owner set for the party's kills: who may loot during the
// rights window.
type LootMode int

const (
	// LootFFA — free-for-all: the whole party owns the kill, so any member may
	// loot during the rights window. The v1 default (grouping.md §5).
	LootFFA LootMode = iota
	// LootMaster — master-looter: only the designated member owns the kill, so
	// loot funnels through them; they distribute it (e.g. via `give`).
	LootMaster
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
	// dissolved (the leader left with no one to succeed, or it dropped to one).
	// newLeaderID is non-empty when a departing leader passed leadership to a
	// remaining member (succession); others is everyone still in the party (the
	// new leader included) to notify; had reports whether they were grouped.
	Leave(memberID string) (disbanded bool, newLeaderID string, others []string, had bool)
	// Disband dissolves leaderID's party (only if they are the leader),
	// returning the other members to notify.
	Disband(leaderID string) (others []string, ok bool)
	// Promote hands leadership of leaderID's party to targetID (leader only →
	// ErrGroupNotLeader otherwise; targetID must be a member other than the
	// leader → ErrGroupPromoteTarget). The old leader stays as a member. On
	// success it returns the party's members (snapshotted under the lock) so the
	// caller can announce the handoff without a second, racy read.
	Promote(leaderID, targetID string) (members []string, err error)
	// Members returns the party member ids (including playerID), or nil when
	// ungrouped.
	Members(playerID string) []string
	// LeaderOf returns playerID's party leader and whether they're grouped.
	LeaderOf(playerID string) (leaderID string, inParty bool)
	// LootPolicy returns the party's loot mode and (for master-looter) the
	// designated master's player id; inParty is false when ungrouped.
	LootPolicy(playerID string) (mode LootMode, masterID string, inParty bool)
	// SetLootMode sets the party's loot policy (leader only → ErrGroupNotLeader
	// otherwise). For LootMaster, masterID names the designated member; "" means
	// the leader. A masterID that isn't a member → ErrLootMasterNotMember. On
	// success it returns the RESOLVED policy (mode + effective master) so the
	// caller can announce it without a second, racy read.
	SetLootMode(leaderID string, mode LootMode, masterID string) (LootMode, string, error)
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
	// Auto-follow the leader on join (grouping.md §9): a party travels together by
	// default. Best-effort — skip silently if following is unavailable or would
	// self/cycle. The relationship stays independent: `unfollow` ends it without
	// leaving the party, and `leave` ends it symmetrically (LeaveHandler).
	if c.autoFollowLeader(leader.PlayerID()) {
		return c.Actor.Write(ctx, fmt.Sprintf("You join %s's party and begin following them.", leader.Name()))
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You join %s's party.", leader.Name()))
}

// autoFollowLeader starts the joining member trailing leaderID (grouping.md §9),
// reporting whether the follow took. Best-effort: a nil follow service, a self-
// follow, or a cycle leaves the join unaffected.
func (c *Context) autoFollowLeader(leaderID string) bool {
	if c.Follow == nil || leaderID == "" || leaderID == c.Actor.PlayerID() {
		return false
	}
	return c.Follow.Follow(c.Actor.PlayerID(), leaderID) == nil
}

// autoUnfollowLeader ends a party-induced follow when leaving (grouping.md §9):
// the member stops trailing priorLeaderID, but ONLY if that is who they are
// currently following — a manual follow of someone else (or none) is left
// untouched. Silent: the leader is already told the member left.
func (c *Context) autoUnfollowLeader(priorLeaderID string) {
	if c.Follow == nil || priorLeaderID == "" || priorLeaderID == c.Actor.PlayerID() {
		return
	}
	if cur, has := c.Follow.Following(c.Actor.PlayerID()); has && cur == priorLeaderID {
		c.Follow.Unfollow(c.Actor.PlayerID())
	}
}

// LeaveHandler implements `leave` / `ungroup` (grouping.md §2/§3).
func LeaveHandler(ctx context.Context, c *Context) error {
	if c.Group == nil {
		return c.Actor.Write(ctx, "You aren't in a party.")
	}
	// Capture the leader before leaving so the symmetric auto-unfollow (below) can
	// tell a party-induced follow from a manual one.
	priorLeaderID, _ := c.Group.LeaderOf(c.Actor.PlayerID())
	disbanded, newLeaderID, others, had := c.Group.Leave(c.Actor.PlayerID())
	if !had {
		return c.Actor.Write(ctx, "You aren't in a party.")
	}
	c.autoUnfollowLeader(priorLeaderID)
	leaverName := c.Actor.Name()
	newLeaderName := ""
	if newLeaderID != "" {
		newLeaderName = c.actorName(newLeaderID)
	}
	for _, id := range others {
		a, ok := c.actorByID(id)
		if !ok {
			continue
		}
		switch {
		case disbanded:
			_ = a.Write(ctx, "The party disbands.")
		case newLeaderID != "" && id == newLeaderID:
			_ = a.Write(ctx, fmt.Sprintf("%s leaves; you now lead the party.", leaverName))
		case newLeaderID != "":
			_ = a.Write(ctx, fmt.Sprintf("%s leaves; %s now leads the party.", leaverName, newLeaderName))
		default:
			_ = a.Write(ctx, leaverName+" leaves the party.")
		}
	}
	if newLeaderID != "" {
		return c.Actor.Write(ctx, fmt.Sprintf("You leave the party; %s now leads it.", newLeaderName))
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

// PromoteHandler implements `promote <member>` (grouping.md §3): the leader hands
// leadership to a chosen party member (who must be online to be named), staying
// in the party as a regular member. Distinct from succession, which fires on an
// unplanned leader departure and picks the longest-tenured member.
func PromoteHandler(ctx context.Context, c *Context) error {
	if c.Group == nil {
		return c.Actor.Write(ctx, "You aren't leading a party.")
	}
	name := strings.TrimSpace(strings.Join(c.Args, " "))
	if name == "" {
		return c.Actor.Write(ctx, "Promote whom?  (try: promote <member>)")
	}
	target, ok := c.partyMemberByName(name)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("%q isn't in your party.", name))
	}
	members, err := c.Group.Promote(c.Actor.PlayerID(), target)
	switch {
	case errors.Is(err, ErrGroupNotLeader):
		return c.Actor.Write(ctx, "Only the party leader can promote a new leader.")
	case errors.Is(err, ErrGroupPromoteTarget):
		return c.Actor.Write(ctx, "You can't promote them.")
	case err != nil:
		return c.Actor.Write(ctx, "You can't promote them right now.")
	}
	newLeaderName := c.actorName(target)
	for _, id := range members {
		a, ok := c.actorByID(id)
		if !ok {
			continue
		}
		switch id {
		case c.Actor.PlayerID():
			_ = a.Write(ctx, fmt.Sprintf("You hand leadership of the party to %s.", newLeaderName))
		case target:
			_ = a.Write(ctx, "You are now the party leader.")
		default:
			_ = a.Write(ctx, fmt.Sprintf("%s now leads the party.", newLeaderName))
		}
	}
	return nil
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

// LootModeHandler implements `lootmode [ffa | master [<member>]]` (grouping.md
// §9): with no argument, show the party's current loot policy; otherwise the
// leader sets it. (`loot` is the corpse-looting verb, so the policy lives under
// its own keyword.)
func LootModeHandler(ctx context.Context, c *Context) error {
	if c.Group == nil {
		return c.Actor.Write(ctx, "You can't set party loot rules right now.")
	}
	mode, masterID, inParty := c.Group.LootPolicy(c.Actor.PlayerID())
	if !inParty {
		return c.Actor.Write(ctx, "You aren't in a party.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, c.lootModeStatus(mode, masterID))
	}
	switch strings.ToLower(c.Args[0]) {
	case "ffa", "free", "freeforall", "free-for-all", "all":
		return c.setLootMode(ctx, LootFFA, "")
	case "master", "ml", "masterlooter":
		master := ""
		if name := strings.TrimSpace(strings.Join(c.Args[1:], " ")); name != "" {
			id, ok := c.partyMemberByName(name)
			if !ok {
				return c.Actor.Write(ctx, fmt.Sprintf("%q isn't in your party.", name))
			}
			master = id
		}
		return c.setLootMode(ctx, LootMaster, master)
	default:
		return c.Actor.Write(ctx, "Set party loot: `lootmode ffa` or `lootmode master [<member>]`.")
	}
}

// setLootMode applies a loot-policy change (leader only) and announces the new
// policy to every online member, the leader included.
func (c *Context) setLootMode(ctx context.Context, mode LootMode, masterID string) error {
	newMode, newMaster, err := c.Group.SetLootMode(c.Actor.PlayerID(), mode, masterID)
	switch {
	case errors.Is(err, ErrGroupNotLeader):
		return c.Actor.Write(ctx, "Only the party leader can set the loot rules.")
	case errors.Is(err, ErrLootMasterNotMember):
		return c.Actor.Write(ctx, "That player isn't in your party.")
	case err != nil:
		return c.Actor.Write(ctx, "You can't set the loot rules right now.")
	}
	// The resolved policy (any defaulting applied — e.g. master mode with no
	// member named → the leader) comes back from SetLootMode, so the
	// announcement needs no second, racy read.
	msg := c.lootModeChange(newMode, newMaster)
	for _, id := range c.Group.Members(c.Actor.PlayerID()) {
		if a, ok := c.actorByID(id); ok {
			_ = a.Write(ctx, msg)
		}
	}
	return nil
}

// lootModeStatus describes the current policy for `lootmode` with no argument.
func (c *Context) lootModeStatus(mode LootMode, masterID string) string {
	if mode == LootMaster {
		return fmt.Sprintf("Party loot: master-looter — only %s may loot a kill.", c.actorName(masterID))
	}
	return "Party loot: free-for-all — any party member may loot a kill."
}

// lootModeChange is the announcement broadcast when the policy changes.
func (c *Context) lootModeChange(mode LootMode, masterID string) string {
	if mode == LootMaster {
		return fmt.Sprintf("The party's loot now goes to %s (master-looter).", c.actorName(masterID))
	}
	return "The party's loot is now free-for-all."
}

// partyMemberByName resolves a party member by (case-insensitive) name to their
// player id. Room-independent — it scans the roster, not the room — so a leader
// can name any member as master, but the member must be online (named by an
// actor the session can resolve).
func (c *Context) partyMemberByName(name string) (string, bool) {
	for _, id := range c.Group.Members(c.Actor.PlayerID()) {
		if strings.EqualFold(c.actorName(id), name) {
			return id, true
		}
	}
	return "", false
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

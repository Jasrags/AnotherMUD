package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/quest"
)

// TalkHandler implements `talk <npc>` (alias `ask`) — the quest giver
// interaction surface (quests.md §3 discovery / §4.3 turn-in). Talking to
// an NPC in the room does two things, in order:
//
//  1. Claims every quest of yours that is ready to turn in at this giver
//     (def.TurnIn quests whose objectives are done). The completion
//     banner + rewards flow through the quest notifier, so this handler
//     writes nothing for the turn-in itself.
//  2. Lists the quests the giver can offer you right now, with the
//     `accept <name>` line to take one.
//
// This is how a player discovers a quest exists (rather than having to
// know its name) and how they close the loop on a turn-in quest. Kept a
// hand-resolved verb (no typed arg) because it resolves a room MOB by
// keyword, which the §5 entity arg pipeline already handles via
// findMobByKeyword.
func TalkHandler(ctx context.Context, c *Context) error {
	if c.Quests == nil {
		return c.Actor.Write(ctx, "There is no one here to talk to about that.")
	}
	term := strings.TrimSpace(strings.Join(c.Args, " "))
	if term == "" {
		return c.Actor.Write(ctx, "Talk to whom?")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You aren't anywhere; there is no one to talk to.")
	}
	npc := findMobByKeyword(c, room.ID, term)
	if npc == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("There is no %q here to talk to.", term))
	}
	player, ok := c.Actor.(quest.Player)
	if !ok {
		// connActor implements quest.Player; a miss is a wiring
		// regression, not a player-facing state.
		return c.Actor.Write(ctx, "You can't talk to quest givers right now.")
	}
	giverID := string(npc.TemplateID())

	turnedIn := claimTurnInsAt(c, player, giverID)
	offers := c.Quests.OffersFrom(player, giverID)

	switch {
	case turnedIn == 0 && len(offers) == 0:
		return c.Actor.Write(ctx, fmt.Sprintf("%s has nothing for you right now.", capitalize(npc.Name())))
	case len(offers) == 0:
		// Only turn-ins happened; the notifier already wrote the completion
		// banner. Add a gentle close so the `talk` isn't left unanswered.
		return c.Actor.Write(ctx, fmt.Sprintf("%s thanks you.", capitalize(npc.Name())))
	default:
		return c.Actor.Write(ctx, renderOffers(npc.Name(), offers))
	}
}

// claimTurnInsAt turns in every quest of the player's that is ready to
// claim at the giver (template id giverID), returning how many were
// claimed. The quest notifier writes each completion banner during
// TurnIn, so callers only need the count. Iterates a state snapshot
// (a detached clone) so mutating the live state via TurnIn is safe.
func claimTurnInsAt(c *Context, player quest.Player, giverID string) int {
	snap := c.Quests.Snapshot(c.Actor.PlayerID())
	if snap == nil {
		return 0
	}
	claimed := 0
	for i := range snap.Active {
		aq := snap.Active[i]
		if !aq.AwaitingTurnIn {
			continue
		}
		def, ok := c.Quests.Definition(aq.QuestID)
		if !ok || def.Giver != giverID {
			continue
		}
		if r := c.Quests.TurnIn(player, aq.QuestID); r.Status == quest.TurnedIn {
			claimed++
		}
	}
	return claimed
}

// renderOffers builds the "<npc> offers:" block — one entry per offer
// with its pitch and the `accept <name>` line to take it.
func renderOffers(npcName string, offers []quest.Offer) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<subtle>%s offers:</subtle>", npcName)
	for _, o := range offers {
		fmt.Fprintf(&b, "\r\n  <title>%s</title>", o.Name)
		if o.Pitch != "" {
			fmt.Fprintf(&b, "\r\n    <subtle>%s</subtle>", o.Pitch)
		}
		fmt.Fprintf(&b, "\r\n    <good>accept %s</good>", o.Name)
	}
	return b.String()
}

package command

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/help"
)

// HelpHandler implements the `help` verb (ui-rendering-help §9/§10). With
// no argument it shows a category index; with a term it queries the help
// service and renders the topic, a disambiguation list, or a no-match
// line by query status. Visibility is gated by the actor's role tier
// (the actor's player id is the requester identity).
func HelpHandler(ctx context.Context, c *Context) error {
	if c.Help == nil {
		return c.Actor.Write(ctx, "Help is not available right now.")
	}
	entityID := c.Actor.PlayerID()
	term := strings.TrimSpace(strings.Join(c.Args, " "))
	if term == "" {
		return c.Actor.Write(ctx, renderHelpIndex(c.Help, entityID))
	}

	res := c.Help.Query(entityID, term)
	switch res.Status {
	case help.StatusOK:
		return c.Actor.Write(ctx, help.RenderTopic(res.Topic, 0))
	case help.StatusMultiple:
		return c.Actor.Write(ctx, help.RenderDisambiguation(res.Term, res.Matches, 0))
	default:
		return c.Actor.Write(ctx, help.RenderNoMatch(res.Term))
	}
}

// renderHelpIndex lists the help categories visible to the requester
// with a usage hint. Kept here (not in the help package) because it's a
// command-surface convenience, not part of the §10 canonical renderers.
func renderHelpIndex(svc *help.Service, entityID string) string {
	cats := svc.Categories(entityID)
	var b strings.Builder
	b.WriteString("<title>Help</title>\r\n")
	if len(cats) == 0 {
		b.WriteString("<subtle>No help topics are available.</subtle>")
		return b.String()
	}
	b.WriteString("<subtle>Categories:</subtle>\r\n")
	for _, cat := range cats {
		b.WriteString("  " + cat + "\r\n")
	}
	b.WriteString("<subtle>Type 'help <topic>' for a specific topic.</subtle>")
	return b.String()
}

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

	// A category name drills into its topic list (§9.7) before falling
	// back to a topic query. Without this, `help commands` would fuzzy-
	// match the `commands` keyword on the help topic instead of listing
	// the commands category — the dead-end a new player hits first.
	if items := c.Help.List(entityID, term); len(items) > 0 {
		return c.Actor.Write(ctx, help.RenderCategory(term, items, 0))
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
	b.WriteString("<subtle>Type 'help <category>' to list its topics (e.g. 'help commands'),\r\n")
	b.WriteString("or 'help <topic>' to read a specific one.</subtle>")
	return b.String()
}

// GenerateHelpTopics backfills the help service with a topic for every
// discoverable command (spec commands-and-dispatch §8). A verb that already
// has an authored topic is skipped so pack content always wins; the rest
// get a topic built from their registration metadata at load order 0, the
// lowest precedence. Idempotent only at the service level — call once at
// boot after RegisterBuiltins and after pack help has loaded.
func GenerateHelpTopics(r *Registry, svc *help.Service) {
	if r == nil || svc == nil {
		return
	}
	for _, cmd := range r.Commands() {
		if svc.HasTopic(cmd.Keyword) {
			continue
		}
		// Keywords seed fuzzy search: the primary plus multi-char aliases
		// (single-letter aliases like `i` would match too broadly).
		keywords := []string{cmd.Keyword}
		for _, a := range cmd.Aliases {
			if len(a) > 1 {
				keywords = append(keywords, a)
			}
		}
		var body string
		if len(cmd.Aliases) > 0 {
			body = "Aliases: " + strings.Join(cmd.Aliases, ", ")
		}
		// Admin commands (admin-verbs §2) take the admin help tier so the
		// help service hides them from non-admins, closing the enumeration
		// vector the dispatch gate leaves open in help. Today requesterTier
		// caps at player, so they're hidden from everyone; once tier
		// resolution consults HasRole (ui §9.5), they surface for admins.
		role := help.RoleNone
		if cmd.Admin {
			role = help.RoleAdmin
		}
		svc.AddTopic(&help.Topic{
			ID:       cmd.Keyword,
			Title:    help.Capitalize(cmd.Keyword),
			Category: cmd.Category,
			Brief:    cmd.Brief,
			Body:     body,
			Syntax:   cmd.Syntax,
			Keywords: keywords,
			Role:     role,
		}, 0)
	}
}

package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/help"
)

// indexGridWidth is the visible width the bare-`help` command grid wraps to.
// Kept a little under the 78-column help width so the grid has breathing room
// on an 80-column terminal.
const indexGridWidth = 74

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

// renderHelpIndex renders the bare-`help` screen: every command grouped under
// its category (categories.go) in canonical order, each group a compact grid of
// verb keywords. Groups the requester can't see (e.g. the admin group for a
// non-admin) come back empty from List and are skipped. Categories outside
// categoryOrder — the default "commands" bucket dynamically-registered ability
// verbs land in, or pack-defined categories — render after the canonical ones so
// nothing is ever hidden. Kept here (not in the help package) because it's a
// command-surface convenience, not part of the §10 canonical renderers.
func renderHelpIndex(svc *help.Service, entityID string) string {
	var b strings.Builder
	b.WriteString("<title>Help — Command Categories</title>\r\n")
	b.WriteString("<subtle>Type 'help <category>' to see a group's commands with descriptions,\r\n")
	b.WriteString("or 'help <command>' for details on a single command.</subtle>\r\n")

	any := false
	rendered := make(map[string]bool)
	emit := func(key, title string) {
		items := svc.List(entityID, key)
		if len(items) == 0 {
			return
		}
		any = true
		b.WriteString("\r\n<highlight>" + title + "</highlight>\r\n")
		b.WriteString(commandGrid(items, indexGridWidth))
	}
	for _, m := range categoryOrder {
		rendered[m.Key] = true
		emit(m.Key, m.Title)
	}
	for _, key := range svc.Categories(entityID) {
		if rendered[key] {
			continue
		}
		rendered[key] = true
		emit(key, categoryTitle(key))
	}

	if !any {
		return "<title>Help</title>\r\n<subtle>No help topics are available.</subtle>"
	}
	return b.String()
}

// commandGrid lays out topic ids as a left-aligned, column-padded grid that
// wraps to width. Ids are bare identifiers (command keywords / topic ids), not
// display prose — no color tags — so byte length is the column width and no tag
// stripping is needed. The last cell of each row is left unpadded so lines carry
// no trailing whitespace.
func commandGrid(items []help.Summary, width int) string {
	if len(items) == 0 {
		return ""
	}
	colW := 0
	for _, it := range items {
		if l := len(it.ID); l > colW {
			colW = l
		}
	}
	colW += 2 // inter-column gutter
	const indent = 2
	perLine := (width - indent) / colW
	if perLine < 1 {
		perLine = 1
	}
	var b strings.Builder
	for i := 0; i < len(items); i += perLine {
		end := i + perLine
		if end > len(items) {
			end = len(items)
		}
		b.WriteString(strings.Repeat(" ", indent))
		for j := i; j < end; j++ {
			if j == end-1 {
				b.WriteString(items[j].ID) // last cell: no trailing pad
			} else {
				b.WriteString(fmt.Sprintf("%-*s", colW, items[j].ID))
			}
		}
		b.WriteString("\r\n")
	}
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
		// Synthesize the syntax line from the typed-arg declaration when the
		// command has one (§8); otherwise fall back to the hand-authored
		// Syntax. A typed command therefore never has to hand-write its
		// syntax, and the two can't drift.
		syntax := cmd.Syntax
		if len(cmd.Args) > 0 {
			syntax = []string{synthesizeSyntax(cmd.Keyword, cmd.Args)}
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
			Syntax:   syntax,
			Keywords: keywords,
			Role:     role,
		}, 0)
	}
}

// usageLine renders a command's synthesized syntax as a subtle usage hint. The
// dispatcher appends it after a missing-argument resolution error so the player
// learns the command's shape instead of just seeing "What <arg>?" (§5.4 /
// ui-rendering-help §10.4). Only meaningful for typed commands (len(args) > 0),
// which is exactly the dispatch branch that calls it.
func usageLine(keyword string, args []ArgDefinition) string {
	return "<subtle>Usage: " + synthesizeSyntax(keyword, args) + "</subtle>"
}

// synthesizeSyntax builds a command's syntax line from its typed-argument
// declaration (commands-and-dispatch §8): the keyword followed by each
// argument rendered as `[name]` (required) or `([name])` (optional), with a
// bulk-capable argument rendered as `[name | all | all.name]`. An argument's
// first declared preposition precedes its token in position
// (`put [gem] in [chest]`).
func synthesizeSyntax(keyword string, args []ArgDefinition) string {
	parts := make([]string, 0, len(args)*2+1)
	parts = append(parts, keyword)
	for _, a := range args {
		if len(a.Prepositions) > 0 {
			parts = append(parts, a.Prepositions[0])
		}
		parts = append(parts, renderArgToken(a))
	}
	return strings.Join(parts, " ")
}

// renderArgToken renders one argument's bracket form for the synthesized
// syntax line. Bulk widens the inner text; Optional wraps the whole bracket
// in parentheses (so an optional bulk arg is `([name | all | all.name])`).
func renderArgToken(a ArgDefinition) string {
	inner := a.Name
	if a.Bulk {
		inner = a.Name + " | all | all." + a.Name
	}
	token := "[" + inner + "]"
	if a.Optional {
		token = "(" + token + ")"
	}
	return token
}

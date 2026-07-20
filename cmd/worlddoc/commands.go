package main

import (
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/command"
)

// commandsEmitter writes commands.html — the engine's built-in commands, grouped
// by category with a usage line and description. Unlike every other section this
// content is engine-wide (identical for every pack): the verbs come from the live
// command registry (command.RegisterBuiltins), not the pack's YAML, so the docs
// track the code rather than drifting. It is NOT world-only — commands apply to
// library packs too — and admin verbs ARE surfaced (in their own "Admin"
// category, last), since these docs are an authoring reference, not the in-game
// player `help`.
var commandsEmitter = emitter{
	name: "commands",
	render: func(m *worldModel, packDir string) ([]string, error) {
		body, err := renderCommands()
		if err != nil {
			return nil, err
		}
		page, err := renderPage(m.Pack, "commands", "Commands",
			"The engine's built-in commands, grouped by category (including admin verbs). These are shared across every world.", body)
		if err != nil {
			return nil, err
		}
		return writeSitePage(packDir, "commands.html", page)
	},
}

// buildCommandCatalog registers the engine builtins into a fresh registry and
// returns the full command catalog, admin verbs included. RegisterBuiltins only
// binds each verb's metadata + handler — there is no server boot — so it is safe
// to call from this static doc tool.
func buildCommandCatalog() ([]command.CatalogCategory, error) {
	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		return nil, fmt.Errorf("registering builtins: %w", err)
	}
	return r.Catalog(true), nil
}

// renderCommands builds the grouped command reference: a sub-nav mirroring the
// catalogs page, then one table per category (Command · Usage · Description).
func renderCommands() (string, error) {
	cats, err := buildCommandCatalog()
	if err != nil {
		return "", err
	}
	if len(cats) == 0 {
		return `<p class="empty">No commands are registered.</p>`, nil
	}

	var b strings.Builder

	// Grouped sub-nav (same shape as catalogs.html).
	b.WriteString(`<p class="note">`)
	links := make([]string, len(cats))
	for i, c := range cats {
		links[i] = fmt.Sprintf(`<a href="#%s">%s (%d)</a>`, esc(c.Key), esc(c.Title), len(c.Commands))
	}
	b.WriteString(strings.Join(links, " · "))
	b.WriteString("</p>")

	// One table per category.
	for _, c := range cats {
		fmt.Fprintf(&b, `<h2 id="%s">%s <span class="count %s">%d</span></h2>`,
			esc(c.Key), esc(c.Title), countClass(len(c.Commands)), len(c.Commands))
		rows := make([][]string, 0, len(c.Commands))
		for _, cmd := range c.Commands {
			usage := cmd.Syntax
			if usage == "" {
				usage = cmd.Keyword
			}
			rows = append(rows, []string{
				codeID(cmd.Keyword),
				"<code>" + esc(usage) + "</code>",
				escName(cmd.Brief),
			})
		}
		b.WriteString(htmlTable([]string{"Command", "Usage", "Description"}, rows))
	}
	return b.String(), nil
}

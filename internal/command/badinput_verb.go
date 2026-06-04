package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/render"
)

// BadInputHandler implements the `badinput` admin verb (commands-and-dispatch
// §6): renders the unknown-verb tracker's snapshot ranked by count, for
// operator triage. `badinput clear` resets it. Admin-gated like the other
// admin verbs (the dispatch gate refuses non-admins with "Huh?").
func BadInputHandler(ctx context.Context, c *Context) error {
	if c.BadInput == nil {
		return c.Actor.Write(ctx, "Bad-input tracking is not enabled.")
	}
	if len(c.Args) > 0 && strings.EqualFold(c.Args[0], "clear") {
		c.BadInput.Clear()
		return c.Actor.Write(ctx, "Bad-input tracker cleared.")
	}

	snap := c.BadInput.Snapshot()
	if len(snap) == 0 {
		return c.Actor.Write(ctx, "No unknown verbs recorded.")
	}

	rows := []render.Row{render.TitleRow("Unknown verbs", fmt.Sprintf("%d distinct", len(snap)))}
	for _, e := range snap {
		rows = append(rows, render.TextRow(fmt.Sprintf("%5d  %s", e.Count, e.Verb), render.AlignLeft, false))
	}
	out, err := render.Panel{Width: 48, Sections: []render.Section{{Rows: rows}}}.Render()
	if err != nil {
		return c.Actor.Write(ctx, "Could not render the bad-input report.")
	}
	return c.Actor.Write(ctx, out)
}

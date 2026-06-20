package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/faction"
)

// StandingHandler implements `standing` — the player's faction standing sheet
// (faction.md §6). Lists every registered faction with the character's current
// standing value and the rank it falls in (the starting standing for an
// untouched faction). nil manager → the verb reports it's disabled.
func StandingHandler(ctx context.Context, c *Context) error {
	if c.Faction == nil {
		return c.Actor.Write(ctx, "Faction standing is not enabled in this world.")
	}
	fe, ok := c.Actor.(faction.Entity)
	if !ok {
		return c.Actor.Write(ctx, "You stand in relation to no faction.")
	}
	defs := c.Faction.Registry().All()
	if len(defs) == 0 {
		return c.Actor.Write(ctx, "There are no factions in this world.")
	}

	widest := 0
	for _, def := range defs {
		name := def.Name
		if name == "" {
			name = def.ID
		}
		if len(name) > widest {
			widest = len(name)
		}
	}

	var b strings.Builder
	b.WriteString("Your standing:\n")
	for _, def := range defs {
		name := def.Name
		if name == "" {
			name = def.ID
		}
		v := c.Faction.Get(fe, def)
		rank := def.RankOf(v)
		if rank == "" {
			rank = "—"
		}
		b.WriteString(fmt.Sprintf("  %-*s  %s (%d)\n", widest, name, rank, v))
	}
	return c.Actor.Write(ctx, b.String())
}

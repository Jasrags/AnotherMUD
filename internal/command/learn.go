package command

import (
	"context"
	"fmt"
	"strings"
)

// LearnHandler implements the `learn <discipline>` verb
// (crafting-and-cooking §2). Acquiring a crafting discipline is the
// distinct entry event — unlike `practice`, which only raises the cap of
// an already-known skill. Learning is trainer-gated: a craft-trainer mob
// in the room (one whose teach list includes the discipline) is required,
// and that same mob may also be a shopkeeper selling basic ingredients
// (the two roles are orthogonal). On success the discipline proficiency is
// seeded at 1 and the discipline's baseline recipes are granted (§2, §7).
//
// A "discipline" is simply any ability id referenced by a recipe's
// `discipline:` field — no separate registry. Skill then rises through use
// when crafting (Phase 2); `practice` raises the cap at a trainer.
func LearnHandler(ctx context.Context, c *Context) error {
	if c.Training == nil || c.Proficiency == nil || c.Recipes == nil || c.Known == nil {
		return c.Actor.Write(ctx, "Learning is not enabled in this build.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Learn what?")
	}
	if len(c.Args) > 1 {
		return c.Actor.Write(ctx, "Usage: learn <discipline>")
	}
	discipline := strings.ToLower(strings.TrimSpace(c.Args[0]))

	// Discipline-ness: the id must be referenced by at least one recipe.
	if len(c.Recipes.ByDiscipline(discipline)) == 0 {
		return c.Actor.Write(ctx, "There is no such craft to learn.")
	}

	// A discipline MUST be a registered ability: its DefaultCap + gain
	// params drive the proficiency cap and the §5 quality roll. A recipe
	// that references a discipline with no registered ability is a content
	// bug — refuse rather than seed a default-cap (100) proficiency that
	// would silently grant a too-high quality ceiling.
	if c.Abilities == nil {
		return c.Actor.Write(ctx, "Learning is not enabled in this build.")
	}
	ab, ok := c.Abilities.Get(discipline)
	if !ok {
		return c.Actor.Write(ctx, "There is no such craft to learn.")
	}
	name := ab.DisplayName

	entityID := c.Actor.PlayerID()
	if entityID == "" {
		entityID = c.Actor.ID()
	}

	if c.Proficiency.Has(entityID, discipline) {
		return c.Actor.Write(ctx, fmt.Sprintf("You already know the basics of %s.", name))
	}

	// Trainer-in-room gate (the same TrainerSource the practice verb uses).
	if c.Training.Trainers == nil {
		return c.Actor.Write(ctx, "There is no one here who can teach you that.")
	}
	tc, trainerName, ok := c.Training.Trainers.TrainerInRoom(entityID)
	if !ok || tc == nil || !tc.CanTeach(discipline) {
		return c.Actor.Write(ctx, "There is no one here who can teach you that.")
	}

	// Acquire: seed the proficiency at 1 (cap from the ability's
	// DefaultCap) and grant the discipline's baseline recipes. The two
	// writes are logically one acquire but not transactional; neither call
	// is fallible, and both are captured by the next Persist (which syncs
	// abilities and recipes unconditionally), so a dropped connection
	// mid-acquire still persists a consistent result.
	c.Proficiency.Learn(entityID, discipline, 1)
	granted := c.Known.GrantBaseline(entityID, discipline)

	msg := fmt.Sprintf("%s teaches you the basics of %s.", trainerName, name)
	if n := len(granted); n > 0 {
		msg += fmt.Sprintf(" You learn %d starting %s.", n, plural(n, "recipe", "recipes"))
	}
	return c.Actor.Write(ctx, msg)
}

// plural picks the singular or plural word for n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

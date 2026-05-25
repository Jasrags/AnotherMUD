package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ConsiderHandler implements `consider <target>` (aliased `con`) —
// the M7.1 status query that surfaces a combatant's HP and AC. It is
// the first command to read through the combat.Combatant surface, so
// it doubles as the end-to-end check that mobs and players both
// satisfy the interface.
//
// Resolution order:
//
//  1. Self synonyms ("me", "self", the actor's own name) bypass the
//     keyword resolver — they exercise the actor-as-Combatant path
//     and let the player check their own HP without scanning the
//     room.
//  2. Mobs in the current room via Placement + keyword resolver
//     (item entities placed in the room are filtered out — they're
//     not Combatants).
//  3. Players in the current room via the session Locator. The
//     Locator is name-based (not keyword-based) so "consider al"
//     does not partial-match "Alice"; the player's full name is
//     required. That asymmetry mirrors how `give` and other targeted
//     verbs resolve players today.
//
// On a hit, the response is two lines: the target's HP fraction +
// qualitative descriptor, and the target's AC. On a miss, a single
// "you don't see them here" line.
func ConsiderHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Consider whom?")
	}
	target := strings.Join(c.Args, " ")

	if isSelfReference(c.Actor.Name(), target) {
		if cb, ok := c.Actor.(combat.Combatant); ok {
			return c.Actor.Write(ctx, renderConsider("yourself", cb))
		}
		// Actor doesn't carry combat state — surface a clear message
		// rather than fall through to room search (which would treat
		// the self-reference as a stranger name and miss).
		return c.Actor.Write(ctx, "You can't size yourself up.")
	}

	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see nothing here.")
	}

	if mob := findMobByKeyword(c, room.ID, target); mob != nil {
		return c.Actor.Write(ctx, renderConsider(mob.Name(), mob))
	}

	if c.Locator != nil {
		if other := c.Locator.FindInRoom(room.ID, target); other != nil {
			if cb, ok := other.(combat.Combatant); ok {
				return c.Actor.Write(ctx, renderConsider(other.Name(), cb))
			}
		}
	}

	return c.Actor.Write(ctx, "You don't see them here.")
}

// isSelfReference reports whether target is one of the standard
// self-aliases or matches the actor's own display name. Case-insensitive
// on the name compare so "consider ALICE" resolves the same as
// "consider alice".
func isSelfReference(actorName, target string) bool {
	t := strings.TrimSpace(target)
	if t == "" {
		return false
	}
	lower := strings.ToLower(t)
	if lower == "self" || lower == "me" {
		return true
	}
	return strings.EqualFold(actorName, t)
}

// findMobByKeyword scans Placement-tracked entities in roomID, filters
// to *MobInstance (item entities and any other future Entity type
// drop out), and runs the shared keyword resolver. Returns nil if any
// of Placement / Items is unwired (tests) or no mob matches.
//
// The resolver runs against a Named slice built from MobInstance
// directly; mobs already expose Name() + Keywords() so no adapter is
// needed.
func findMobByKeyword(c *Context, roomID world.RoomID, target string) *entities.MobInstance {
	if c.Placement == nil || c.Items == nil {
		return nil
	}
	ids := c.Placement.InRoom(roomID)
	if len(ids) == 0 {
		return nil
	}
	candidates := make([]keyword.Named, 0, len(ids))
	for _, id := range ids {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		mob, ok := e.(*entities.MobInstance)
		if !ok {
			continue
		}
		candidates = append(candidates, mob)
	}
	if len(candidates) == 0 {
		return nil
	}
	hit := keyword.Resolve(candidates, target)
	if hit == nil {
		return nil
	}
	mob, _ := hit.(*entities.MobInstance)
	return mob
}

// renderConsider formats the two-line HP/AC report for displayName.
// Pulls a Snapshot from Vitals so current and max are read under a
// single lock rather than two; the qualitative descriptor is derived
// from the same pair to avoid a render where current%max looks
// inconsistent with the descriptor word.
func renderConsider(displayName string, cb combat.Combatant) string {
	cur, max := cb.Vitals().Snapshot()
	descriptor := vitalsDescriptor(cur, max)
	stats := cb.Stats()
	return fmt.Sprintf("%s: %d/%d HP (%s). AC %d.",
		displayName, cur, max, descriptor, stats.AC)
}

// vitalsDescriptor maps an HP fraction to a human-readable word. The
// thresholds are policy and a content pack will likely want to skin
// them; until then these defaults give players coarse but useful
// signal. The dead band is special-cased because a Percent-only
// scheme would group "0%" with "near death" and that's misleading
// when the target is already a corpse.
func vitalsDescriptor(cur, max int) string {
	if cur <= 0 {
		return "dead"
	}
	if max <= 0 {
		// Defensive — NewVitals enforces max >= 1, but a future
		// SetMax(0) caller would otherwise produce a divide-by-zero
		// here. Treat unknown-max as "unknown" rather than crash.
		return "unknown"
	}
	pct := float64(cur) / float64(max)
	switch {
	case pct >= 0.90:
		return "uninjured"
	case pct >= 0.70:
		return "lightly wounded"
	case pct >= 0.40:
		return "moderately wounded"
	case pct >= 0.15:
		return "badly wounded"
	default:
		return "near death"
	}
}


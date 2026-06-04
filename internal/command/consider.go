package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
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
	// Target-only: consider sizes up someone ELSE. Self stats moved to the
	// dedicated `score` sheet, so a bare `consider` (or a self-reference)
	// points there rather than echoing a thin self line.
	target := strings.Join(c.Args, " ")
	if target == "" || isSelfReference(c.Actor.Name(), target) {
		return c.Actor.Write(ctx, "Consider whom? (Use `score` for your own stats.)")
	}

	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see nothing here.")
	}

	if cb, name, ok := findCombatantInRoom(c, room.ID, target); ok {
		// The viewer is a combatant in production (connActor); test stubs
		// may not be. nil viewer → condition-only render (no threat read).
		viewer, _ := c.Actor.(combat.Combatant)
		return c.Actor.Write(ctx, renderConsider(viewer, name, cb))
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

// renderConsider formats the qualitative size-up of a target: how hurt
// they look (the condition descriptor, observable by looking) plus —
// when the viewer is itself a combatant — a relative-threat read that
// answers "can I take them?" WITHOUT leaking raw HP/AC numbers. The
// tactical lens is deliberately impressionistic; `score` is where a
// player reads their own exact stats. Condition is read from a single
// Vitals Snapshot so current/max stay consistent with the word.
func renderConsider(viewer combat.Combatant, displayName string, cb combat.Combatant) string {
	cur, max := cb.Vitals().Snapshot()
	condition := vitalsDescriptor(cur, max)
	line := fmt.Sprintf("%s appears %s.", displayName, condition)
	if viewer != nil {
		line += " " + threatPhrase(viewer, cb)
	}
	return line
}

// threatPhrase buckets the target's combat power relative to the
// viewer's into a coarse "can I win?" read. Power is a max-HP-dominant
// proxy plus the core combat stats (STR/AC/hit) — enough to rank a
// fight without promising the precise math. Uses MAX hp (full
// potential); the condition descriptor already signals if the target is
// currently wounded, so threat reflects the fight at full strength.
func threatPhrase(viewer, target combat.Combatant) string {
	vp := combatPower(viewer)
	tp := combatPower(target)
	if vp <= 0 {
		return "You cannot gauge your chances."
	}
	switch r := float64(tp) / float64(vp); {
	case r < 0.5:
		return "You could crush them without effort."
	case r < 0.85:
		return "You have the upper hand."
	case r <= 1.2:
		return "It would be an even fight."
	case r <= 2.0:
		return "They have the advantage — be careful."
	default:
		return "You wouldn't stand a chance."
	}
}

// combatPower is the relative-strength proxy: max HP (durability, the
// dominant term) plus the core offensive/defensive stats. Deliberately
// simple — consider only needs to rank, not simulate.
func combatPower(cb combat.Combatant) int {
	_, max := cb.Vitals().Snapshot()
	s := cb.Stats()
	return max + s.STR + s.AC + s.HitMod
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

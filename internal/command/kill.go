package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// KillHandler implements `kill <target>` — the M7.2 verb that starts
// combat. Resolves a Combatant in the actor's room via the shared
// findCombatantInRoom helper, then calls combat.Manager.Engage.
//
// The verb is bookkeeping-only in M7.2: it commits the engagement,
// emits the Engagement event, and tells the player + target. No
// damage is dealt — the round loop (M7.3) and auto-attacks (M7.4)
// land separately. From the player's perspective, "kill <x>" + a few
// seconds of waiting is how this slice degrades into M7.3+.
//
// Refusal paths and their messages:
//   - no argument             → "Kill whom?"
//   - actor has no room       → "You see no targets here."
//   - actor has no Combatant  → "You aren't able to fight."
//   - actor missing Combat env → "You can't attack right now."
//   - target self / actor's own name → "You can't fight yourself."
//   - target not in room      → "You don't see them here."
//   - target not a combatant  → "You can't attack that."
//   - already engaged target  → "You're already fighting <name>."
//
// On success: a confirmation to the attacker, an "X attacks you!"
// to the target (when the target is a player), and a third-person
// announce to the room. The Engagement event also fires from
// Manager — for now no engine listener consumes it, but future
// UI/quest/log hooks subscribe through cmd/anothermud's adapter.
func KillHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Kill whom?")
	}

	target := strings.Join(c.Args, " ")

	// Self-check FIRST — the most specific refusal wins over the more
	// generic "can't attack right now" / "you aren't able to fight"
	// messages, so a player who types `kill <ownname>` always sees the
	// self-targeting message regardless of which other env pieces are
	// wired. Mirrors consider's resolution order.
	if isSelfReference(c.Actor.Name(), target) {
		return c.Actor.Write(ctx, "You can't fight yourself.")
	}

	if c.Combat == nil {
		// Test path without combat wired — refuse cleanly rather
		// than nil-deref. Production main always wires Combat.
		return c.Actor.Write(ctx, "You can't attack right now.")
	}

	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see no targets here.")
	}

	attacker, ok := c.Actor.(combat.Combatant)
	if !ok {
		// Test actors without combat state get a clear refusal.
		// Production connActor implements Combatant since M7.1.
		return c.Actor.Write(ctx, "You aren't able to fight.")
	}

	targetCombatant, targetName, found := findCombatantInRoom(c, room.ID, target)
	if !found {
		return c.Actor.Write(ctx, "You don't see them here.")
	}

	attackerID := attacker.CombatantID()
	targetID := targetCombatant.CombatantID()

	// Engage with explicit refusal code (M7.6 added tag and cooldown
	// gates). Map each refusal to a precise player-facing message;
	// the EngageWithReason result removes the TOCTOU window the
	// M7.2 OpponentsOf-post-check had.
	switch reason, ok := c.Combat.EngageWithReason(ctx, attackerID, targetID, room.ID); {
	case ok:
		// fall through to success path below.
	case reason == combat.EngageRefusalAlreadyEngaged:
		return c.Actor.Write(ctx,
			fmt.Sprintf("You're already fighting %s.", targetName))
	case reason == combat.EngageRefusalSafeRoom:
		return c.Actor.Write(ctx, "Violence is forbidden here.")
	case reason == combat.EngageRefusalNoKill:
		return c.Actor.Write(ctx,
			fmt.Sprintf("You can't bring yourself to attack %s.", targetName))
	case reason == combat.EngageRefusalFleeCooldown:
		return c.Actor.Write(ctx, "You're still catching your breath.")
	default:
		return c.Actor.Write(ctx, "You can't attack that.")
	}

	if err := c.Actor.Write(ctx,
		fmt.Sprintf("You attack %s!", targetName)); err != nil {
		return err
	}

	// Third-person announce to the room, excluding the attacker
	// (who got the first-person line above). Broadcaster may be
	// nil in tests.
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s attacks %s!", c.Actor.Name(), targetName),
			c.Actor.PlayerID())
	}

	return nil
}

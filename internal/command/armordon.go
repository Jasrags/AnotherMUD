package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Armor don/doff timers (armor-depth §7), translated for a real-time tick engine.
// In the source, donning plate takes ~4 minutes and even a mail shirt a minute —
// the meaningful consequence is that you cannot armor up (or shed armor) once a
// fight is on you. Rather than make a player wait wall-clock minutes (Decision 0:
// keep the meaningful choice, drop the bookkeeping), bulky armor simply cannot be
// equipped or removed WHILE IN COMBAT. Light armor, shields, and untiered gear are
// quick enough to manage mid-fight and stay free.
//
// The §7 "hastily donned" escape (fast but a worse bonus + check penalty) is a
// deferred follow-up — it needs per-slot hasty state and an armor-aggregation
// change; the combat gate is the substantive rule.

// isSlowArmor reports whether the item is bulky enough that donning/removing it
// is gated in combat — medium or heavy armor tier (a mail hauberk, a breastplate,
// plate, a great helm). Light armor and untiered gear (shields, weapons) are not.
func isSlowArmor(it *entities.ItemInstance) bool {
	switch it.ArmorTier() {
	case "medium", "heavy":
		return true
	default:
		return false
	}
}

// actorInCombat reports whether the actor is engaged, probed via an optional
// capability (like the carry-weight limit). A test stub or a mob without combat
// state is never gated.
func (c *Context) actorInCombat() bool {
	if cs, ok := c.Actor.(interface{ InCombat() bool }); ok {
		return cs.InCombat()
	}
	return false
}

// armorChangeBlockedInCombat refuses a don/doff of slow armor mid-fight
// (armor-depth §7). Returns a non-nil error (the player message already written)
// when the change is blocked; nil to proceed. verb is "buckle on" / "shed".
func (c *Context) armorChangeBlockedInCombat(ctx context.Context, it *entities.ItemInstance, refusal string) (error, bool) {
	if isSlowArmor(it) && c.actorInCombat() {
		return c.Actor.Write(ctx, refusal), true
	}
	return nil, false
}

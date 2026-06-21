package command

import (
	"context"
	"fmt"

	"github.com/Jasrags/AnotherMUD/internal/action"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// KindArmorDon is the action.Kind tag for a timed armor don/doff
// (action-economy.md §7.2). The action-complete sweep routes a completed
// occupation of this kind back through the ORIGINAL equip/remove command
// (replaying Context.Raw with Env.ReplayAction set), which then performs the
// deferred slot mutation instantly.
const KindArmorDon action.Kind = "armor-don"

// defaultDonTicks is the fallback occupation length (in engine ticks) for
// donning/doffing slow armor when Env.DonTicks is unset. At the 100ms baseline
// tick this is a few seconds — long enough to be a real-time commitment, short
// enough not to be the wall-clock minutes the d20 table prescribes (Decision 0).
const defaultDonTicks = 30

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
// (armor-depth §7). Returns blocked=true (with the player refusal already
// written) when the change must be refused, or (false, nil) to proceed. The
// returned error is the refusal Write's result, propagated by the caller.
func (c *Context) armorChangeBlockedInCombat(ctx context.Context, it *entities.ItemInstance, refusal string) (blocked bool, err error) {
	if isSlowArmor(it) && c.actorInCombat() {
		return true, c.Actor.Write(ctx, refusal)
	}
	return false, nil
}

// beginArmorTimer defers a slow-armor don/doff as a timed occupation
// (action-economy.md §7.2). It returns deferred=true (with the begin notice
// already written) when the change became an in-flight action the
// action-complete sweep will finish — the caller must then return WITHOUT
// committing. It returns (false, nil) — proceed to the instant commit — when
// timed actions are disabled (no tracker / no clock), when this dispatch is the
// deferred completion replaying the command (ReplayAction), or when the item is
// light gear (no occupation). An in-combat slow-armor change never reaches here:
// armorChangeBlockedInCombat refuses it earlier, so a deferred don is always an
// out-of-combat act.
func (c *Context) beginArmorTimer(ctx context.Context, it *entities.ItemInstance, remove bool) (deferred bool, err error) {
	if c.ReplayAction || c.Actions == nil || c.NowTick == nil || !isSlowArmor(it) {
		return false, nil
	}
	ider, ok := c.Actor.(interface{ PlayerID() string })
	if !ok || ider.PlayerID() == "" {
		return false, nil
	}
	ticks := c.DonTicks
	if ticks <= 0 {
		ticks = defaultDonTicks
	}
	gerund := "buckling on"
	if remove {
		gerund = "unstrapping"
	}
	if !c.Actions.Begin(ider.PlayerID(), action.Action{
		Kind:          KindArmorDon,
		ReadyAt:       c.NowTick() + uint64(ticks),
		Interruptible: true,
		Label:         gerund + " " + it.Name(),
		Payload:       c.Raw, // the sweep replays the exact original command
	}) {
		// Lost the slot to another in-flight action — refuse this change.
		return true, c.Actor.Write(ctx, "You're already busy with something.")
	}
	if room := c.Actor.Room(); room != nil && c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s begins %s %s.", c.Actor.Name(), gerund, it.Name()),
			ider.PlayerID())
	}
	return true, c.Actor.Write(ctx, fmt.Sprintf("You begin %s %s.", gerund, it.Name()))
}

// StopHandler cancels the actor's in-flight timed action (action-economy.md §5,
// the manual-cancel path). It interrupts an interruptible occupation, reports an
// occupation that cannot be stopped, and tells an idle actor there's nothing to
// stop. Deliberately NOT an IsAction command — a busy player must always be able
// to stop.
func StopHandler(ctx context.Context, c *Context) error {
	if c.Actions == nil {
		return c.Actor.Write(ctx, "You aren't doing anything.")
	}
	ider, ok := c.Actor.(interface{ PlayerID() string })
	if !ok || ider.PlayerID() == "" {
		return c.Actor.Write(ctx, "You aren't doing anything.")
	}
	id := ider.PlayerID()
	act, busy := c.Actions.Active(id)
	if !busy {
		return c.Actor.Write(ctx, "You aren't doing anything.")
	}
	if !act.Interruptible {
		return c.Actor.Write(ctx, "You can't stop what you're doing now.")
	}
	c.Actions.Interrupt(id)
	label := act.Label
	if label == "" {
		label = "what you were doing"
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You stop %s.", label))
}

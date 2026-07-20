package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/condition"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// Downed-foe verbs (subdual-damage): once a foe is knocked out (helpless), a
// runner can either FINISH them (a lethal coup-de-grace → corpse + loot + kill
// credit + murder-heat) or ROB them (take their carry non-lethally and leave
// them breathing — cheaper on heat, but a live witness). Both gate on the target
// actually carrying the `unconscious` condition, so a conscious foe is never a
// free kill or a free steal.

// resolveDownedMob resolves the verb's target arg to a mob in the room and
// verifies it is helpless (unconscious). On any failure it returns a nil mob and
// the player-facing line the caller should write.
func resolveDownedMob(c *Context, verbPrompt string) (*entities.MobInstance, string) {
	room := c.Actor.Room()
	if room == nil {
		return nil, "There is no one here."
	}
	term := strings.TrimSpace(strings.Join(c.Args, " "))
	if term == "" {
		return nil, verbPrompt
	}
	cb, name, ok := findCombatantInRoom(c, room.ID, term)
	if !ok {
		return nil, fmt.Sprintf("You don't see %q here.", term)
	}
	m, ok := cb.(*entities.MobInstance)
	if !ok {
		// Players aren't downed-lootable/finishable through this path.
		return nil, "You can't do that to them."
	}
	if c.Effects == nil || !c.Effects.HasFlag(string(m.ID()), condition.FlagUnconscious) {
		return nil, fmt.Sprintf("%s isn't helpless — put them down first.", name)
	}
	return m, ""
}

// FinishHandler implements `finish <mob>` (coup-de-grace, subdual-damage §8): a
// guaranteed lethal blow to a helpless foe, routed through the normal death
// pipeline so it drops a corpse, credits the kill, and draws heat like any other
// killing.
func FinishHandler(ctx context.Context, c *Context) error {
	if c.Finish == nil {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}
	m, msg := resolveDownedMob(c, "Finish off whom?")
	if m == nil {
		return c.Actor.Write(ctx, msg)
	}
	name := m.Name()
	room := c.Actor.Room()
	attacker := combat.NewPlayerCombatantID(c.Actor.PlayerID())
	target := combat.NewMobCombatantID(string(m.ID()))
	if !c.Finish(ctx, target, attacker, room.ID) {
		return c.Actor.Write(ctx, fmt.Sprintf("You fail to finish %s off.", name))
	}
	// The death pipeline announces the kill + corpse to the room; keep the
	// verb's own line to the actor a terse, cold confirmation.
	return c.Actor.Write(ctx, fmt.Sprintf("<warning>You deliver a killing blow to the helpless %s.</warning>", name))
}

// RobHandler implements `rob <mob>` (non-lethal loot, subdual-damage): take a
// helpless foe's carried items + coin purse and leave them alive. Marked
// `looted` so a wake → re-knock-out can't re-roll it, and its eventual corpse
// (if it is later killed) drops nothing.
func RobHandler(ctx context.Context, c *Context) error {
	if c.Contents == nil || c.Items == nil {
		return c.Actor.Write(ctx, "You can't rob anyone right now.")
	}
	m, msg := resolveDownedMob(c, "Rob whom?")
	if m == nil {
		return c.Actor.Write(ctx, msg)
	}
	name := m.Name()
	room := c.Actor.Room()
	// Atomic single-claim BEFORE any transfer: the first robber to reach a downed
	// mob wins, a loser racing the same target (another connection goroutine)
	// sees it already looted and takes nothing. This is the coin-dupe + re-rob
	// guard — claiming up front means two goroutines can't both roll coins.
	// Also suppresses the mob's corpse coin drop later (corpse reads IsLooted).
	if !m.ClaimLooted() {
		return c.Actor.Write(ctx, fmt.Sprintf("%s has already been picked clean.", name))
	}

	// Take the carried items (rolled at spawn, held in the mob's contents), then
	// roll + credit the coin purse now. Single-claim Contents.Take mirrors the
	// corpse-loot transfer, so two robbers can't duplicate.
	var takenIDs []entities.EntityID
	for _, id := range c.Contents.In(m.ID()) {
		if c.Contents.Take(id) {
			c.Actor.AddToInventory(id)
			takenIDs = append(takenIDs, id)
		}
	}
	taken := collectItems(c.Items, takenIDs)

	credited := 0
	if c.RobCoins != nil && c.Currency != nil {
		if holder, ok := c.Actor.(economy.Entity); ok {
			if coins := c.RobCoins(string(m.TemplateID())); coins > 0 {
				c.Currency.AddGold(ctx, holder, coins, "rob:"+string(m.ID()))
				credited = coins
			}
		}
	}

	if len(taken) == 0 && credited == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s has nothing worth taking.", name))
	}
	_ = c.Actor.Write(ctx, lootMessage(c, name, taken, credited))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s rifles through the unconscious %s.", c.Actor.Name(), name),
			c.Actor.PlayerID())
	}
	return nil
}

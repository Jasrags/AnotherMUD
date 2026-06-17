package command

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
)

// ThrowHandler implements the ranged-combat §3 thrown-weapon verb:
// `throw <target>` hurls the wielded thrown weapon at a target in the room.
// Thrown is one-shot (you only have the one knife), so it is a discrete action
// rather than a repeating auto-attack: it resolves a single swing (full
// Strength, via the wielded weapon's combat.Stats), engages combat so the
// target retaliates, then removes the weapon from hand and lands it in the
// room — recoverable, unless it is a quality-graded (masterwork) weapon, which
// is destroyed on use (§3).
//
// The weapon is implicit (whatever thrown-class weapon is wielded); throwing a
// weapon straight from inventory is a later refinement.
func ThrowHandler(ctx context.Context, c *Context) error {
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Throw at whom?")
	}
	if c.Combat == nil || c.ResolveAttack == nil || c.Items == nil || c.Placement == nil {
		// Test/headless path without combat wired — refuse cleanly.
		return c.Actor.Write(ctx, "You can't throw right now.")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You see no targets here.")
	}
	attacker, ok := c.Actor.(combat.Combatant)
	if !ok {
		return c.Actor.Write(ctx, "You aren't able to fight.")
	}

	// Find the wielded thrown weapon. Equipment is a map; iterate in sorted
	// key order so a multi-slot layout resolves deterministically (mirrors
	// recomputeWeaponLocked's stable pick).
	wieldedID, wieldedIt, wieldedSlot := c.wieldedThrownWeapon()
	if wieldedIt == nil {
		return c.Actor.Write(ctx, "You aren't wielding anything you can throw.")
	}

	target := strings.Join(c.Args, " ")
	if isSelfReference(c.Actor.Name(), target) {
		return c.Actor.Write(ctx, "You can't throw at yourself.")
	}
	targetCombatant, targetName, found := findCombatantInRoom(c, room.ID, target)
	if !found {
		return c.Actor.Write(ctx, "You don't see them here.")
	}

	attackerID := attacker.CombatantID()
	targetID := targetCombatant.CombatantID()

	// Engage so the target retaliates, honoring the same §2.1 gates as kill
	// (safe room, no-kill, flee cooldown). Already-engaged is fine — you can
	// throw at someone you're already fighting.
	switch reason, ok := c.Combat.EngageWithReason(ctx, attackerID, targetID, room.ID); {
	case ok, reason == combat.EngageRefusalAlreadyEngaged:
		// proceed
	case reason == combat.EngageRefusalSafeRoom:
		return c.Actor.Write(ctx, "Violence is forbidden here.")
	case reason == combat.EngageRefusalNoKill:
		return c.Actor.Write(ctx, fmt.Sprintf("You can't bring yourself to attack %s.", targetName))
	case reason == combat.EngageRefusalFleeCooldown:
		return c.Actor.Write(ctx, "You're still catching your breath.")
	default:
		return c.Actor.Write(ctx, "You can't attack that.")
	}

	// Announce the throw; the single-swing hit/miss is narrated by the combat
	// sink (the same renderer as a weapon swing), so we don't double-report it.
	weaponName := wieldedIt.Name()
	_ = c.Actor.Write(ctx, fmt.Sprintf("You hurl %s at %s!", weaponName, targetName))
	if c.Broadcaster != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID,
			fmt.Sprintf("%s hurls %s at %s!", c.Actor.Name(), weaponName, targetName),
			c.Actor.PlayerID())
	}

	// Resolve one swing while the weapon is still in hand (so its full-STR
	// damage + crit profile are in combat.Stats), THEN remove it.
	c.ResolveAttack(ctx, attackerID, targetID, room.ID)

	// Remove the weapon from hand (Unequip returns it to inventory; then drop
	// from inventory) and land it in the room — unless it is graded, in which
	// case it is destroyed on use (§3) and not recoverable. Only place the
	// weapon when it was actually removed from inventory, so a (defensive)
	// failed unequip/remove can never duplicate a still-held item into the room.
	graded := wieldedIt.Grade() != "" && c.Grades != nil && c.Grades.Has(wieldedIt.Grade())
	c.Actor.Unequip(wieldedSlot)
	removed := c.Actor.RemoveFromInventory(wieldedID)
	switch {
	case removed && graded:
		_ = c.Actor.Write(ctx, fmt.Sprintf("%s shatters on impact.", weaponName))
	case removed:
		c.Placement.Place(wieldedID, room.ID)
	}
	return nil
}

// wieldedThrownWeapon returns the actor's currently-wielded thrown-class weapon
// (its id, live instance, and slot key), or (_, nil, _) when none is wielded.
// Equipment keys are scanned in sorted order so the pick is deterministic when
// several slots are occupied.
func (c *Context) wieldedThrownWeapon() (entities.EntityID, *entities.ItemInstance, string) {
	equip := c.Actor.Equipment()
	keys := make([]string, 0, len(equip))
	for k := range equip {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		id := equip[k]
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		if it.RangedClass() == item.RangedThrown {
			return id, it, k
		}
	}
	return "", nil, ""
}

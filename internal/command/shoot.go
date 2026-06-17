package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// ammoConsumer is the local view of a player actor that can spend a projectile's
// ammunition (ranged-combat §3, the session.AmmoConsumer seam). Defined here so
// the command package needn't import session — the live *connActor satisfies it,
// a test/headless actor does not (and then a projectile fires freely, matching
// the mob path in the round loop's AmmoFor hook).
type ammoConsumer interface {
	ConsumeAmmo(kind string) (gradeKey string, ok bool)
}

// ShootHandler implements ranged-combat Model C (§9) slice 1: the cross-room
// opportunistic shot. `shoot <target> <direction>` looses a single projectile
// through one open exit at a target in the ADJACENT room. This is the engine's
// first cross-room interaction, and it is deliberately an ACTION, not a
// sustained engagement: the same-room invariant (combat §4.1 / ranged-combat
// §1) is untouched — the round loop still only sustains a fight within one room.
// You snipe; if you want a fight you close in (or the target comes to you, a
// later slice).
//
// Line of sight is "what you could walk through": the exit must exist and be
// visible to you (an undiscovered hidden exit reads as no exit, hidden-exits
// §4.1), its door must be open (World.Move resolves both), and the target room
// must not be pitch-black to you. Ammo, Strength rules, and the weapon's
// damage/crit profile all ride the wielded bow's combat.Stats — the same data
// the same-room ranged path (slices A/B) reads — so a cross-room shot resolves
// identically to a point-blank one, only the audience differs.
//
// Render is two-room with NO event-struct change: the swing's events are stamped
// with the TARGET's room, so the third-person hit/miss/death announce lands
// where the target is (where the spectacle is) and the second-person tells route
// by player id to each participant regardless of room. The verb adds the
// directional flavor either side — an outbound line in the shooter's room, an
// inbound "from the <reverse>" line in the target's room.
func ShootHandler(ctx context.Context, c *Context) error {
	if c.Combat == nil || c.ResolveAttack == nil || c.World == nil {
		// Test/headless path without combat/world wired — refuse cleanly.
		return c.Actor.Write(ctx, "You can't shoot right now.")
	}
	if len(c.Args) < 2 {
		return c.Actor.Write(ctx, "Shoot at whom, and which way?  (try: shoot goblin north)")
	}
	room := c.Actor.Room()
	if room == nil {
		return c.Actor.Write(ctx, "You have nowhere to shoot from.")
	}
	attacker, ok := c.Actor.(combat.Combatant)
	if !ok {
		return c.Actor.Write(ctx, "You aren't able to fight.")
	}

	// Last token is the direction; everything before it is the target keyword.
	dirTok := c.Args[len(c.Args)-1]
	dir, ok := world.ParseDirection(dirTok)
	if !ok {
		return c.Actor.Write(ctx, "Shoot in which direction?  (try: shoot goblin north)")
	}
	targetStr := strings.TrimSpace(strings.Join(c.Args[:len(c.Args)-1], " "))
	if targetStr == "" {
		return c.Actor.Write(ctx, "Shoot at whom?")
	}

	// You need a projectile weapon wielded. combat.Stats already resolved the
	// wielded weapon's ranged class (slice A), so we read it rather than
	// re-scanning equipment. Thrown weapons use the same-room `throw` verb.
	st := attacker.Stats()
	if st.RangedClass != combat.RangedProjectile {
		return c.Actor.Write(ctx, "You aren't wielding anything you can shoot.")
	}

	// Line of sight through the exit. An absent or undiscovered-hidden exit
	// reads the same (indistinguishable from a wall, hidden-exits §4.1).
	exit, hasExit := room.Exits[dir]
	if !hasExit || !c.canSeeExit(dir, exit) {
		return c.Actor.Write(ctx, fmt.Sprintf("There's no way to shoot to the %s.", dir.Long()))
	}
	// World.Move is a pure graph resolve (no relocation) that already enforces
	// the closed-door block — exactly the LoS rule we want.
	dst, err := c.World.Move(room.ID, dir)
	if err != nil {
		switch {
		case errors.Is(err, world.ErrDoorClosed):
			return c.Actor.Write(ctx, fmt.Sprintf("The way %s is closed; you can't shoot through it.", dir.Long()))
		case errors.Is(err, world.ErrNoExit):
			return c.Actor.Write(ctx, fmt.Sprintf("There's no way to shoot to the %s.", dir.Long()))
		default:
			return c.Actor.Write(ctx, "Something blocks your shot.")
		}
	}
	// Darkness LoS: a target room that is black to you offers nothing to aim at.
	if c.Light != nil && c.effectiveLight(dst) <= light.Black {
		return c.Actor.Write(ctx, fmt.Sprintf("It's too dark to make out anything to the %s.", dir.Long()))
	}

	// Resolve the target in the adjacent room.
	targetCombatant, targetName, found := findCombatantInOtherRoom(c, dst.ID, targetStr)
	if !found {
		return c.Actor.Write(ctx, fmt.Sprintf("You don't see anyone like that to the %s.", dir.Long()))
	}

	// Spend one matching ammo unit. A live player actor satisfies ammoConsumer;
	// a headless/test actor does not and fires freely (mirrors the mob path).
	// The consumed unit's masterwork grade is not yet folded into to-hit on this
	// one-shot path (ResolveSingleAttack reads Stats().HitMod only) — a recorded
	// slice-1 limitation, not a correctness bug.
	if consumer, ok := c.Actor.(ammoConsumer); ok && st.AmmoKind != "" {
		if _, consumed := consumer.ConsumeAmmo(st.AmmoKind); !consumed {
			_ = c.Actor.Write(ctx, fmt.Sprintf("*click* — you are out of %s!", st.AmmoKind))
			if c.Broadcaster != nil && c.Actor.Name() != "" {
				c.Broadcaster.SendToRoom(ctx, room.ID,
					fmt.Sprintf("%s grasps for ammunition that isn't there.", c.Actor.Name()),
					c.Actor.PlayerID())
			}
			return nil
		}
	}

	// Two-room narration. The hit/miss/death lines come from the combat sink
	// (stamped to the target room below); here we add the directional flavor.
	_ = c.Actor.Write(ctx, fmt.Sprintf("You loose a shot to the %s at %s!", dir.Long(), targetName))
	if c.Broadcaster != nil {
		if c.Actor.Name() != "" {
			c.Broadcaster.SendToRoom(ctx, room.ID,
				fmt.Sprintf("%s looses a shot to the %s.", c.Actor.Name(), dir.Long()),
				c.Actor.PlayerID())
		}
		// Inbound line in the target's room, from the reverse direction. Exclude
		// the target (a player) — they get the sink's second-person "hits you".
		c.Broadcaster.SendToRoom(ctx, dst.ID,
			fmt.Sprintf("A shot streaks in from the %s at %s!", dir.Opposite().Long(), targetName),
			combat.EntityIDOf(targetCombatant.CombatantID()))
	}

	// Resolve exactly one swing, stamped to the TARGET room so the third-person
	// announce lands where the target is. No engagement: cross-room combat does
	// not sustain (the round loop would disengage it next tick anyway).
	alive := c.ResolveAttack(ctx, attacker.CombatantID(), targetCombatant.CombatantID(), dst.ID)

	// Retaliation (Model C §10 slice 2): a living MOB that was shot bears a
	// grudge — it will path toward the shooter and engage on the AI tick. Stamp
	// the shooter's id + room; the AI retaliation step consumes it. Only mobs
	// retaliate automatically (a player target chooses their own response), and
	// only survivors (a kill ends it).
	if alive {
		if mob, ok := targetCombatant.(*entities.MobInstance); ok {
			mob.SetProperty(entities.PropRetaliateTarget, c.Actor.PlayerID())
			mob.SetProperty(entities.PropRetaliateRoom, string(room.ID))
		}
	}
	return nil
}

// findCombatantInOtherRoom resolves target against the combatants in an
// arbitrary room (not the actor's own) — the cross-room shot's targeting.
// Mobs resolve by keyword (the shared resolver), players by exact name via the
// Locator, mirroring findCombatantInRoom's two channels and mob-wins-ties rule.
// It is deliberately room-parameterized rather than reusing findCombatantInRoom,
// whose arg pipeline is bound to the actor's own room.
func findCombatantInOtherRoom(c *Context, roomID world.RoomID, target string) (combat.Combatant, string, bool) {
	if mob := findMobByKeyword(c, roomID, target); mob != nil {
		return mob, mob.Name(), true
	}
	if c.Locator != nil {
		if other := c.Locator.FindInRoom(roomID, target); other != nil {
			if cb, ok := other.(combat.Combatant); ok {
				return cb, other.Name(), true
			}
		}
	}
	return nil, "", false
}

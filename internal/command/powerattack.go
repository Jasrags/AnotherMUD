package command

import (
	"context"
	"strings"
)

// powerAttackController is the tiny mutation surface a connActor exposes for
// `powerattack [on|off]` (feats Bucket C). Defined here so the command package
// doesn't import session just for these methods. The production actor
// (session.connActor) satisfies it; test fakes that don't care opt out and see
// the "you can't" message.
type powerAttackController interface {
	PowerAttackActive() bool
	HasPowerAttackFeat() bool
	SetPowerAttack(on bool)
}

// PowerAttackHandler implements `powerattack [on|off]` — read or set the Power
// Attack combat stance (feats Bucket C). While on, the attacker trades to-hit
// for melee damage every swing.
//
//	No argument:   report the current stance.
//	"on":          enter the stance (refused without the Power Attack feat).
//	"off":         leave the stance.
//	anything else: usage message.
func PowerAttackHandler(ctx context.Context, c *Context) error {
	ctrl, ok := c.Actor.(powerAttackController)
	if !ok {
		return c.Actor.Write(ctx, "You can't fight that way.")
	}

	if len(c.Args) == 0 {
		if ctrl.PowerAttackActive() {
			return c.Actor.Write(ctx, "Power Attack is on — you're trading accuracy for power.")
		}
		return c.Actor.Write(ctx, "Power Attack is off.")
	}

	switch strings.ToLower(c.Args[0]) {
	case "on":
		if !ctrl.HasPowerAttackFeat() {
			return c.Actor.Write(ctx, "You don't know how to fight with Power Attack.")
		}
		if ctrl.PowerAttackActive() {
			return c.Actor.Write(ctx, "Power Attack is already on.")
		}
		ctrl.SetPowerAttack(true)
		return c.Actor.Write(ctx, "You set yourself to attack with raw power, trading accuracy for damage.")
	case "off":
		if !ctrl.PowerAttackActive() {
			return c.Actor.Write(ctx, "Power Attack is already off.")
		}
		ctrl.SetPowerAttack(false)
		return c.Actor.Write(ctx, "You return to a measured, accurate fighting style.")
	default:
		return c.Actor.Write(ctx, "Usage: powerattack <on|off>")
	}
}

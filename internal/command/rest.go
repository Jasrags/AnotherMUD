package command

import (
	"context"

	"github.com/Jasrags/AnotherMUD/internal/economy"
)

// Rest verbs (spec economy-survival §5.3). `rest`, `sleep`, and `wake`
// drive the rest state machine through the RestService. There is no
// furniture system yet, so transitions carry no furniture id — the
// rest-target aux field stays empty until furniture lands.

// RestHandler implements `rest` → resting (spec §5.3).
func RestHandler(ctx context.Context, c *Context) error {
	return changeRestState(ctx, c, economy.StateResting,
		"You sit down and rest.",
		"sits down to rest.",
		"You are already resting.")
}

// SleepHandler implements `sleep` → sleeping (spec §5.3).
func SleepHandler(ctx context.Context, c *Context) error {
	return changeRestState(ctx, c, economy.StateSleeping,
		"You lie down and drift off to sleep.",
		"lies down and goes to sleep.",
		"You are already asleep.")
}

// WakeHandler implements `wake` (and `stand`) → awake (spec §5.3).
func WakeHandler(ctx context.Context, c *Context) error {
	// conditions §5: `stand` (a wake alias) also gets you up from prone.
	// Prone takes priority — a prone combatant is not resting, so clearing
	// it is what "stand" means in that moment; otherwise fall through to the
	// ordinary wake-from-rest behavior.
	if c.Effects != nil && c.Effects.RemoveByID(ctx, c.Actor.PlayerID(), conditionProneEffectID) {
		// Narrate to the actor and the room (the notifier deliberately skips
		// prone-clear to avoid double-messaging, so the room line lives here).
		if err := c.Actor.Write(ctx, "You climb back to your feet."); err != nil {
			return err
		}
		if room := c.Actor.Room(); c.Broadcaster != nil && room != nil && c.Actor.Name() != "" {
			c.Broadcaster.SendToRoom(ctx, room.ID, c.Actor.Name()+" climbs back to their feet.", c.Actor.PlayerID())
		}
		return nil
	}
	return changeRestState(ctx, c, economy.StateAwake,
		"You wake up and stand.",
		"wakes up and stands.",
		"You are already awake.")
}

// changeRestState routes a transition through the service and renders
// each outcome (spec §5.3 return reasons). selfMsg/roomMsg render the
// success; alreadyMsg renders the already_in_state no-op.
func changeRestState(ctx context.Context, c *Context, newState economy.RestState, selfMsg, roomVerb, alreadyMsg string) error {
	if c.Rest == nil {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}
	re, ok := c.Actor.(economy.RestEntity)
	if !ok {
		return c.Actor.Write(ctx, "You can't do that right now.")
	}

	ok, reason := c.Rest.SetRestState(ctx, re, newState, "")
	if !ok {
		switch reason {
		case "already_in_state":
			return c.Actor.Write(ctx, alreadyMsg)
		case "cancelled":
			return c.Actor.Write(ctx, "Something prevents you.")
		default:
			return c.Actor.Write(ctx, "You can't do that right now.")
		}
	}

	if err := c.Actor.Write(ctx, selfMsg); err != nil {
		return err
	}
	room := c.Actor.Room()
	if c.Broadcaster != nil && room != nil && c.Actor.Name() != "" {
		c.Broadcaster.SendToRoom(ctx, room.ID, c.Actor.Name()+" "+roomVerb, c.Actor.PlayerID())
	}
	return nil
}

package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/economy"
)

// RestoreHandler implements `restore [<target>]` (admin-verbs §5): the
// mercy verb — set a target's vitals to full AND top off its sustenance
// (hunger/thirst). No argument restores the actor; otherwise the target
// resolves in the room (player or mob), the same scope set/inspect use
// (§3). Admin-marked (M19.3 gate); audited via the M19.4a auditAdmin
// choke point.
func RestoreHandler(ctx context.Context, c *Context) error {
	var (
		target   any
		name, id string
		ok       bool
	)
	if token := strings.TrimSpace(strings.Join(c.Args, " ")); token == "" {
		target, name, id = c.Actor, "yourself", c.Actor.PlayerID()
	} else {
		target, name, id, ok = resolveSetTarget(c, token)
		if !ok {
			return c.Actor.Write(ctx, "You don't see them here.")
		}
	}

	cb, isCombatant := target.(combat.Combatant)
	if !isCombatant {
		return c.Actor.Write(ctx, "That target has no vitals to restore.")
	}

	_, max := cb.Vitals().Snapshot()
	newHP := cb.Vitals().SetCurrent(max)

	// Top off the sustenance pool (hunger/thirst) when the target carries
	// one. Players do (connActor implements economy.SustenanceEntity);
	// mobs and other entities don't, so the cast simply skips them — HP is
	// the universal part. (Today sustenance is a single pool fed by both
	// food and drink; if it ever splits into hunger+thirst — BACKLOG §2 —
	// this refill broadens with it.)
	fed := ""
	if se, ok := target.(economy.SustenanceEntity); ok {
		se.SetSustenance(economy.MaxSustenance)
		fed = ", fully fed"
	}

	auditAdmin(ctx, c, "restore", id, "")
	return c.Actor.Write(ctx, fmt.Sprintf("%s — restored to %d/%d HP%s.", name, newHP, max, fed))
}

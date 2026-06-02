package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// RestoreHandler implements `restore [<target>]` (admin-verbs §5): the
// mercy verb — set a target's vitals to full. No argument restores the
// actor; otherwise the target resolves in the room (player or mob), the
// same scope set/inspect use (§3). Admin-marked (M19.3 gate); audited via
// the M19.4a auditAdmin choke point.
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

	auditAdmin(ctx, c, "restore", id, "")
	return c.Actor.Write(ctx, fmt.Sprintf("%s — restored to %d/%d HP.", name, newHP, max))
}

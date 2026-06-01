package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// RoleController is the authorization surface the grant/revoke verbs need
// on a character: the read check (HasRole) plus the mutators (Grant/Revoke),
// each reporting whether the set actually changed. The session connActor
// satisfies it. Defined here so the command package doesn't import session.
type RoleController interface {
	HasRole(role string) bool
	Grant(role string) bool  // returns whether the set changed (idempotent)
	Revoke(role string) bool // returns whether the set changed (idempotent)
	Name() string
	PlayerID() string
}

// RoleTargetResolver maps a player name to the live RoleController of an
// ONLINE character (roles-and-permissions §4 — v1 grants/revokes target
// online players; offline targets are deferred per §9). Returns
// (nil, false) when no such player is currently logged in.
type RoleTargetResolver interface {
	ResolveRoleTarget(name string) (RoleController, bool)
}

// defaultGrantingRole is used when Context.GrantingRole is unset so role
// administration still works out of the box (roles-and-permissions §8 —
// the granting role is configuration, conventionally `admin`).
const defaultGrantingRole = "admin"

// GrantHandler implements `grant <role> to <player>`
// (roles-and-permissions §4): an actor holding the granting role grants a
// role to another online character.
func GrantHandler(ctx context.Context, c *Context) error {
	return roleChange(ctx, c, true)
}

// RevokeHandler implements `revoke <role> from <player>`.
func RevokeHandler(ctx context.Context, c *Context) error {
	return roleChange(ctx, c, false)
}

// roleChange is the shared grant/revoke path. granting selects the
// direction; everything else (gate, parse, self-block, idempotency, event)
// is identical.
func roleChange(ctx context.Context, c *Context, granting bool) error {
	verb, prep, done := "grant", "to", "Granted"
	if !granting {
		verb, prep, done = "revoke", "from", "Revoked"
	}

	grantingRole := c.GrantingRole
	if grantingRole == "" {
		grantingRole = defaultGrantingRole
	}

	// Gate: the actor must hold the granting role. Refuse WITHOUT
	// disclosing the gating role's existence or name (§3) — a generic
	// refusal, the same an unprivileged player gets for anything.
	actor, ok := c.Actor.(RoleController)
	if !ok || !actor.HasRole(grantingRole) {
		return c.Actor.Write(ctx, "You can't do that.")
	}

	role, targetName, ok := parseRoleArgs(c.Args)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("Usage: %s <role> %s <player>", verb, prep))
	}
	role = strings.ToLower(strings.TrimSpace(role))

	if c.RoleTargetResolver == nil {
		return c.Actor.Write(ctx, "Role administration is not enabled.")
	}
	target, ok := c.RoleTargetResolver.ResolveRoleTarget(targetName)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("No one named %q is online.", targetName))
	}

	// §1.1 — not self-service: a character cannot grant or revoke their own
	// roles, even an admin. Privilege comes from someone else (or the seed).
	if target.PlayerID() != "" && target.PlayerID() == c.Actor.PlayerID() {
		return c.Actor.Write(ctx, fmt.Sprintf("You can't %s roles %s yourself.", verb, prep))
	}

	var changed bool
	if granting {
		changed = target.Grant(role)
	} else {
		changed = target.Revoke(role)
	}
	if !changed {
		// Idempotent no-op (§2): granting a held role / revoking an unheld
		// one changes nothing and emits no event.
		state := "already has"
		if !granting {
			state = "doesn't have"
		}
		return c.Actor.Write(ctx, fmt.Sprintf("%s %s the %q role.", target.Name(), state, role))
	}

	// §7 — emit the observable fact, only on an actual change.
	if granting {
		c.Publish(ctx, eventbus.RoleGranted{Actor: c.Actor.PlayerID(), Target: target.PlayerID(), Role: role})
	} else {
		c.Publish(ctx, eventbus.RoleRevoked{Actor: c.Actor.PlayerID(), Target: target.PlayerID(), Role: role})
	}
	logging.From(ctx).Info("role "+verb,
		slog.String("actor", c.Actor.PlayerID()),
		slog.String("target", target.PlayerID()),
		slog.String("role", role))

	return c.Actor.Write(ctx, fmt.Sprintf("%s %q %s %s.", done, role, prep, target.Name()))
}

// parseRoleArgs extracts (role, targetName) from `<role> [to|from] <player>`.
// Lenient on the middle preposition: role is the first token, target the
// last, so both `grant admin alice` and `grant admin to alice` parse. ok is
// false with fewer than two meaningful tokens or when role == target.
func parseRoleArgs(args []string) (role, target string, ok bool) {
	if len(args) < 2 {
		return "", "", false
	}
	role = strings.TrimSpace(args[0])
	target = strings.TrimSpace(args[len(args)-1])
	if role == "" || target == "" || strings.EqualFold(role, target) {
		return "", "", false
	}
	return role, target, true
}

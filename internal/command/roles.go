package command

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// GrantTarget is the mutation surface the generalized grant/revoke verbs need
// on the TARGET character: identity plus the polymorphic attribute mutators.
// GrantAttribute/RevokeAttribute dispatch a canonical kind ("role"/"feat"/
// "ability"/"recipe"/"language") to the right store, returning whether the set
// changed (idempotency) and a user-facing error when the value doesn't name a
// real thing. The session connActor satisfies it; defined here so the command
// package doesn't import session.
type GrantTarget interface {
	PlayerID() string
	Name() string
	GrantAttribute(kind, value string) (changed bool, err error)
	RevokeAttribute(kind, value string) (changed bool, err error)
}

// grantKinds maps a kind keyword (including the `skill` alias for `ability`) to
// its canonical name. The command owns this vocabulary; the target's
// GrantAttribute owns the mutation.
var grantKinds = map[string]string{
	"role":     "role",
	"feat":     "feat",
	"ability":  "ability",
	"skill":    "ability", // alias
	"recipe":   "recipe",
	"language": "language",
}

// grantKindList is the canonical kind set for the usage message.
var grantKindList = []string{"role", "feat", "ability", "recipe", "language"}

// RoleHolder is the minimal read surface the dispatcher's admin gate needs
// from an actor — just the authorization check (admin-verbs §2). connActor
// satisfies it (and the richer RoleController). An actor that does not
// implement it is treated as holding no roles.
type RoleHolder interface {
	HasRole(role string) bool
}

// defaultAdminRole is the role an admin-marked command requires when
// Env.AdminRole is unset (admin-verbs §8 — conventionally `admin`).
const defaultAdminRole = "admin"

// RoleTargetResolver maps a player name to the live GrantTarget of an ONLINE
// character (admin-verbs — v1 grants/revokes target online players; offline
// targets are deferred). Returns (nil, false) when no such player is currently
// logged in. (Name kept for back-compat; it now resolves a full GrantTarget.)
type RoleTargetResolver interface {
	ResolveRoleTarget(name string) (GrantTarget, bool)
}

// defaultGrantingRole is used when Context.GrantingRole is unset so grant
// administration still works out of the box (admin-verbs §8 — the granting role
// is configuration, conventionally `admin`).
const defaultGrantingRole = "admin"

// GrantHandler implements `grant <kind> <value> to <player>` (admin-verbs): an
// actor holding the granting role adds an attribute (role/feat/ability/recipe/
// language) to another online character.
func GrantHandler(ctx context.Context, c *Context) error {
	return grantChange(ctx, c, true)
}

// RevokeHandler implements `revoke <kind> <value> from <player>`.
func RevokeHandler(ctx context.Context, c *Context) error {
	return grantChange(ctx, c, false)
}

// grantChange is the shared grant/revoke path across all kinds. granting
// selects the direction; the gate, parse, self-block, idempotency, and the
// (role-only) event are otherwise identical.
func grantChange(ctx context.Context, c *Context, granting bool) error {
	verb, prep, done := "grant", "to", "Granted"
	if !granting {
		verb, prep, done = "revoke", "from", "Revoked"
	}

	// Gate + parse + resolve (refusals written inside). A nil target means a
	// refusal already went out; return its (possible) write error.
	target, kind, value, refusal := resolveGrantChange(ctx, c, verb, prep)
	if target == nil {
		return refusal
	}

	var (
		changed bool
		err     error
	)
	if granting {
		changed, err = target.GrantAttribute(kind, value)
	} else {
		changed, err = target.RevokeAttribute(kind, value)
	}
	if err != nil {
		// A validation failure (no such feat/ability/recipe/language) — the
		// target's error text is user-appropriate.
		return c.Actor.Write(ctx, capitalize(err.Error())+".")
	}
	if !changed {
		// Idempotent no-op: granting a held attribute / revoking an unheld one.
		state := "already has"
		if !granting {
			state = "doesn't have"
		}
		return c.Actor.Write(ctx, fmt.Sprintf("%s %s the %q %s.", target.Name(), state, value, kind))
	}

	// Roles keep their observable events (the role-change subscriber / audit key
	// on them). Other kinds are v1-silent — add a per-kind event when a consumer
	// needs one. Only on an actual change.
	if kind == "role" {
		if granting {
			c.Publish(ctx, eventbus.RoleGranted{Actor: c.Actor.PlayerID(), Target: target.PlayerID(), Role: value})
		} else {
			c.Publish(ctx, eventbus.RoleRevoked{Actor: c.Actor.PlayerID(), Target: target.PlayerID(), Role: value})
		}
	}
	logging.From(ctx).Info(verb+" "+kind,
		slog.String("actor", c.Actor.PlayerID()),
		slog.String("target", target.PlayerID()),
		slog.String("kind", kind),
		slog.String("value", value))

	return c.Actor.Write(ctx, fmt.Sprintf("%s %s %q %s %s.", done, kind, value, prep, target.Name()))
}

// resolveGrantChange runs the gate → parse → kind-resolve → target-resolve →
// self-block prologue. On any refusal it writes the message and returns a nil
// target; on success it returns (target, canonical kind, value).
func resolveGrantChange(ctx context.Context, c *Context, verb, prep string) (GrantTarget, string, string, error) {
	grantingRole := c.GrantingRole
	if grantingRole == "" {
		grantingRole = defaultGrantingRole
	}

	// Gate: the actor must hold the granting role. Refuse WITHOUT disclosing the
	// gating role's existence or name — a generic refusal.
	actor, ok := c.Actor.(RoleHolder)
	if !ok || !actor.HasRole(grantingRole) {
		return nil, "", "", c.Actor.Write(ctx, "You can't do that.")
	}

	kindWord, value, targetName, ok := parseGrantArgs(c.Args)
	if !ok {
		return nil, "", "", c.Actor.Write(ctx, fmt.Sprintf(
			"Usage: %s <kind> <value> %s <player>  (kinds: %s)", verb, prep, strings.Join(grantKindList, ", ")))
	}
	kind, ok := grantKinds[strings.ToLower(kindWord)]
	if !ok {
		return nil, "", "", c.Actor.Write(ctx, fmt.Sprintf(
			"Unknown kind %q. Kinds: %s.", kindWord, strings.Join(grantKindList, ", ")))
	}
	// All grantable ids are lower-case by convention (roles are case-insensitive;
	// feat/ability/recipe/language are authored lower-case), so normalize here —
	// the display, the event, and the mutation all see the canonical form.
	value = strings.ToLower(value)

	if c.RoleTargetResolver == nil {
		return nil, "", "", c.Actor.Write(ctx, "Grant administration is not enabled.")
	}
	target, ok := c.RoleTargetResolver.ResolveRoleTarget(targetName)
	if !ok {
		return nil, "", "", c.Actor.Write(ctx, fmt.Sprintf("No one named %q is online.", targetName))
	}

	// Not self-service for ROLES: a character cannot grant/revoke their own
	// roles, even an admin (privilege comes from someone else). Non-role
	// attributes (a test feat/skill) may be self-granted — that's not an
	// escalation, and it's convenient for GMing.
	if kind == "role" && target.PlayerID() != "" && target.PlayerID() == c.Actor.PlayerID() {
		return nil, "", "", c.Actor.Write(ctx, fmt.Sprintf("You can't %s roles %s yourself.", verb, prep))
	}

	return target, kind, value, nil
}

// parseGrantArgs extracts (kind, value, targetName) from
// `<kind> <value> [to|from] <player>`. kind is the first token, value the
// second, target the last (the middle preposition is optional and ignored, and
// values are single-token content ids). ok is false with fewer than three
// meaningful tokens.
func parseGrantArgs(args []string) (kind, value, target string, ok bool) {
	if len(args) < 3 {
		return "", "", "", false
	}
	kind = strings.TrimSpace(args[0])
	value = strings.TrimSpace(args[1])
	target = strings.TrimSpace(args[len(args)-1])
	if kind == "" || value == "" || target == "" {
		return "", "", "", false
	}
	return kind, value, target, true
}

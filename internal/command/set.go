package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
)

// settableKind is one entry in the admin `set` field catalogue
// (admin-verbs §4): a kind of settable field (vital / — later — property /
// tag), the enumerated types it accepts (nil for free-form kinds whose
// type is an arbitrary name), a human description of which entity kinds it
// applies to (for the usage panel), and the mutation itself.
//
// apply receives the resolved live target (combat.Combatant today; a
// broader handle when property/tag land) and the (type, value) pair. It
// returns a human change description on success, or a usage error — in
// which case nothing is written (§4: a type-mismatched value is refused
// without writing garbage).
type settableKind struct {
	name      string
	types     []string
	appliesTo string
	apply     func(ctx context.Context, c *Context, target any, typeName, value string) (string, error)
}

// setCatalogue is the admin-settable field catalogue. M19.4c ships the
// `vital` kind; `property` and `tag` (admin-verbs §4) land as additional
// entries in later slices. Roles are deliberately absent and never join
// here — privilege changes go through the separately-audited grant/revoke
// surface (§4 / roles-and-permissions §4), never an incidental `set`.
var setCatalogue = []settableKind{
	{
		name:      "vital",
		types:     []string{"hp"},
		appliesTo: "player, npc",
		apply:     applyVital,
	},
}

func lookupSetKind(name string) (settableKind, bool) {
	for _, k := range setCatalogue {
		if strings.EqualFold(k.name, name) {
			return k, true
		}
	}
	return settableKind{}, false
}

// SetHandler implements `set <kind> <type> <target> <value>` (admin-verbs
// §4): the general-purpose admin write. Admin-marked (M19.3 gate); audited
// via the M19.4a auditAdmin choke point on success. Target resolution is
// the same room/self scope inspect uses (§3); the visibility bypass is the
// same documented no-op until hide/sneak rules exist.
//
// A bare or incomplete invocation renders the self-documenting usage panel
// rather than failing silently (§4).
func SetHandler(ctx context.Context, c *Context) error {
	if len(c.Args) < 4 {
		return renderSetUsage(ctx, c)
	}
	kindName, typeName, targetTok := c.Args[0], c.Args[1], c.Args[2]
	value := strings.Join(c.Args[3:], " ")

	kind, ok := lookupSetKind(kindName)
	if !ok {
		if err := c.Actor.Write(ctx, fmt.Sprintf("Unknown set kind %q.", kindName)); err != nil {
			return err
		}
		return renderSetUsage(ctx, c)
	}

	target, displayName, targetID, ok := resolveSetTarget(c, targetTok)
	if !ok {
		return c.Actor.Write(ctx, "You don't see them here.")
	}

	change, err := kind.apply(ctx, c, target, typeName, value)
	if err != nil {
		// Usage / type error: nothing was written (§4).
		return c.Actor.Write(ctx, err.Error())
	}

	auditAdmin(ctx, c, "set", targetID, fmt.Sprintf("%s %s=%s", kindName, typeName, value))
	return c.Actor.Write(ctx, fmt.Sprintf("%s — %s", displayName, change))
}

// resolveSetTarget resolves the target token to a live entity, its display
// name, and its id (for the audit). No-arg-style self-references resolve
// the actor; otherwise the actor's room is searched (player or mob), the
// same path inspect uses. Returns ok=false on a miss.
func resolveSetTarget(c *Context, token string) (target any, displayName, id string, ok bool) {
	if isSelfReference(c.Actor.Name(), token) {
		return c.Actor, "yourself", c.Actor.PlayerID(), true
	}
	room := c.Actor.Room()
	if room == nil {
		return nil, "", "", false
	}
	cb, name, found := findCombatantInRoom(c, room.ID, token)
	if !found {
		return nil, "", "", false
	}
	return cb, name, inspectID(cb), true
}

// applyVital sets a live vital on the target, clamped to its maximum
// (admin-verbs §4). M19.4c settable vital: hp. The value must be numeric;
// a non-numeric value is a usage error that writes nothing.
func applyVital(ctx context.Context, c *Context, target any, typeName, value string) (string, error) {
	cb, ok := target.(combat.Combatant)
	if !ok {
		return "", fmt.Errorf("That target has no vitals to set.")
	}
	if !strings.EqualFold(typeName, "hp") {
		return "", fmt.Errorf("Unknown vital %q. Settable vitals: hp.", typeName)
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("Vital value must be a whole number.")
	}
	newHP := cb.Vitals().SetCurrent(n)
	_, max := cb.Vitals().Snapshot()
	return fmt.Sprintf("HP set to %d/%d.", newHP, max), nil
}

// renderSetUsage writes the self-documenting usage panel: the grammar plus
// every catalogue kind and its types (admin-verbs §4 — a bare/incomplete
// set is self-documenting, not a silent failure).
func renderSetUsage(ctx context.Context, c *Context) error {
	lines := []string{
		"Usage: set <kind> <type> <target> <value>",
		"Settable kinds:",
	}
	for _, k := range setCatalogue {
		types := "<name>"
		if len(k.types) > 0 {
			types = strings.Join(k.types, ", ")
		}
		lines = append(lines, fmt.Sprintf("  %-10s %-14s [%s]", k.name, types, k.appliesTo))
	}
	for _, line := range lines {
		if err := c.Actor.Write(ctx, line); err != nil {
			return err
		}
	}
	return nil
}

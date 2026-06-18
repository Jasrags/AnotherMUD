package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/world"
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

// setCatalogue is the admin-settable field catalogue. M19.4c shipped the
// `vital` kind; M19.4h adds `property` (on room mobs/items). The `tag` kind
// (admin-verbs §4) is still pending its substrate (no runtime tag mutator on
// players/mobs). Roles are deliberately absent and never join here —
// privilege changes go through the separately-audited grant/revoke surface
// (§4 / roles-and-permissions §4), never an incidental `set`.
var setCatalogue = []settableKind{
	{
		name:      "vital",
		types:     []string{"hp"},
		appliesTo: "player, npc",
		apply:     applyVital,
	},
	{
		name:      "property",
		types:     nil, // free-form: the `type` slot is the property name, validated against the registry
		appliesTo: "npc, item",
		apply:     applyProperty,
	},
	{
		name:      "gold",
		types:     []string{"amount"},
		appliesTo: "player",
		apply:     applyGold,
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
// the actor; otherwise the actor's room is searched — a player or mob first
// (the same path inspect uses), then, failing that, a room item (M19.4h
// `set property` on items). Returns ok=false on a miss.
func resolveSetTarget(c *Context, token string) (target any, displayName, id string, ok bool) {
	if isSelfReference(c.Actor.Name(), token) {
		return c.Actor, "yourself", c.Actor.PlayerID(), true
	}
	room := c.Actor.Room()
	if room == nil {
		return nil, "", "", false
	}
	if cb, name, found := findCombatantInRoom(c, room.ID, token); found {
		return cb, name, inspectID(cb), true
	}
	// Items aren't combatants, so they resolve only here — after no
	// player/mob in the room matched the token.
	if it, found := findRoomItem(c, room.ID, token); found {
		return it, it.Name(), string(it.ID()), true
	}
	return nil, "", "", false
}

// findRoomItem resolves a room item by keyword, reusing the same
// roomItems + keyword.Resolve match chain the fill/put verbs use. Returns
// (nil, false) when the room holds no items or none match the token.
func findRoomItem(c *Context, roomID world.RoomID, token string) (*entities.ItemInstance, bool) {
	items := roomItems(c, roomID)
	if len(items) == 0 {
		return nil, false
	}
	match := keyword.Resolve(asNamed(items), token)
	if match == nil {
		return nil, false
	}
	it, ok := match.(*entities.ItemInstance)
	return it, ok
}

// applyVital sets a live vital on the target, clamped to its maximum
// (admin-verbs §4). M19.4c settable vital: hp. The value must be numeric;
// a non-numeric value is a usage error that writes nothing.
// applyGold sets a currency holder's balance through the authoritative
// currency service (admin-verbs §4 — the general-purpose admin write; also the
// supported way to fund a character for testing/GMing). The `type` slot is
// "amount". Mirrors applyVital's shape.
func applyGold(ctx context.Context, c *Context, target any, typeName, value string) (string, error) {
	if c.Currency == nil {
		return "", fmt.Errorf("Currency is not enabled.")
	}
	holder, ok := target.(economy.Entity)
	if !ok {
		return "", fmt.Errorf("That target has no purse to set.")
	}
	if !strings.EqualFold(typeName, "amount") {
		return "", fmt.Errorf("Unknown gold field %q. Settable: amount.", typeName)
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return "", fmt.Errorf("Gold amount must be a non-negative whole number.")
	}
	if err := c.Currency.SetGold(ctx, holder, n, "admin_set"); err != nil {
		return "", fmt.Errorf("Couldn't set gold: %v", err)
	}
	return fmt.Sprintf("Gold set to %d.", c.Currency.Read(holder)), nil
}

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

// applyProperty writes a registered, admin-settable property onto a live
// mob or item (admin-verbs §4). The `type` slot carries the property NAME,
// validated against the property registry: the property must exist, be
// flagged AdminSettable, and the value must coerce to its declared type —
// otherwise nothing is written (§4: a type-mismatched value is refused).
//
// Scope limits (M19.4h): the write is live-only — room mobs and items are
// transient, so there is no save-integration here. Player targets are NOT
// supported yet (connActor has no property bag — the M19.4h+ deferral) and
// are refused without writing. Engine properties resolve by bare name; a
// pack property needs the qualified `pack:name` form.
func applyProperty(ctx context.Context, c *Context, target any, propName, value string) (string, error) {
	setter, ok := target.(interface {
		Properties() map[string]any
		SetProperty(key string, value any)
	})
	if !ok {
		// A player (connActor) lands here: no property bag yet.
		return "", fmt.Errorf("That target has no settable properties.")
	}
	if c.Properties == nil {
		return "", fmt.Errorf("Properties cannot be set right now.")
	}
	if propName == entities.PropTemplateID || propName == entities.PropRoomID {
		return "", fmt.Errorf("Property %q is reserved and cannot be set.", propName)
	}
	entry, ok := c.Properties.Get(propName, "")
	if !ok {
		return "", fmt.Errorf("Unknown property %q.", propName)
	}
	if !entry.AdminSettable {
		return "", fmt.Errorf("Property %q is not admin-settable.", propName)
	}
	parsed, err := parsePropertyValue(entry.Type, value)
	if err != nil {
		return "", err
	}
	// Store under the bare Name, NOT entry.Key(): instance property bags are
	// keyed by the bare property name (that's how template YAML authors them
	// and how readers like stringProp look them up). A pack property's
	// `pack:` namespace lives on the registry, not in the bag — keying the
	// write by Key() would hide it from every bare-name reader.
	setter.SetProperty(entry.Name, parsed)
	return fmt.Sprintf("property %s set to %q.", entry.Name, value), nil
}

// parsePropertyValue coerces the raw value token to the property's declared
// Go type, returning a usage error (writing nothing) on a mismatch. The
// collection types (map/list) are excluded from the simple `set` path —
// admin-verbs §4 reserves them for dedicated verbs.
func parsePropertyValue(t property.ValueType, value string) (any, error) {
	v := strings.TrimSpace(value)
	switch t {
	case property.TypeString:
		return value, nil // keep interior spacing the admin typed
	case property.TypeInt:
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("Value must be a whole number.")
		}
		return n, nil
	case property.TypeInt64:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("Value must be a whole number.")
		}
		return n, nil
	case property.TypeFloat64:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("Value must be a number.")
		}
		return f, nil
	case property.TypeBool:
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("Value must be true or false.")
		}
		return b, nil
	default:
		return nil, fmt.Errorf("Property type %s can't be set with a single value.", t)
	}
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

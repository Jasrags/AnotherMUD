package command

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/logging"
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
// `vital` kind; M19.4h added `property` (on room mobs/items); M19.4i adds
// `tag` (add/remove a gameplay tag on a player or mob). Roles are
// deliberately absent and never join here — privilege changes go through the
// separately-audited grant/revoke surface (§4 / roles-and-permissions §4),
// never an incidental `set`.
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
		name:      "tag",
		types:     []string{"add", "remove"},
		appliesTo: "player, npc",
		apply:     applyTag,
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

// maxTagNameLen caps a `set tag` name so a pathological string can't bloat the
// save (it lands in Save.AdminTags, written to YAML) or every Tags() snapshot
// the AI evaluator reads. Admin-only input, but validated at the boundary all
// the same.
const maxTagNameLen = 64

// reservedTagNamespaces are the tag prefixes owned by a manager (progression
// alignment, faction, reputation). A tag in one of these namespaces is derived
// from the manager's state and reconstructed at login, so `set tag` must never
// write it directly — a hand-written `alignment_evil` / `faction:x:y` /
// `renown:z` would desync the owning manager. This is the tag analogue of
// applyProperty refusing the reserved template_id/room_id property keys. Held
// as literals (rather than importing three manager packages just for prefix
// strings); set_test.go feeds real manager tag outputs through the guard so a
// prefix rename in progression/faction/reputation trips a test.
var reservedTagNamespaces = []struct{ label, prefix string }{
	{"alignment", "alignment_"}, // internal/progression: alignment_{evil,neutral,good}
	{"faction", "faction:"},     // internal/faction: RankTag = "faction:<id>:<rank>"
	{"reputation", "renown:"},   // internal/reputation: TierTagPrefix
}

// reservedStructuralTags are engine-synthetic tags whose presence a subsystem
// depends on for enumeration — removing one silently breaks that subsystem for
// the entity's lifetime. entities.TagMob is the one that matters here: the AI
// dispatcher enumerates mobs solely via GetByTag(TagMob), so `set tag remove
// <mob> mob` would drop the mob out of every AI turn (an inert statue until it
// respawns). `set tag` refuses these exact tags on either op.
var reservedStructuralTags = []string{entities.TagMob}

// reservedTagNamespace reports the owning system if tag falls in a
// manager-owned namespace or is a structural tag, so applyTag can refuse it.
func reservedTagNamespace(tag string) (label string, reserved bool) {
	for _, ns := range reservedTagNamespaces {
		if strings.HasPrefix(tag, ns.prefix) {
			return ns.label, true
		}
	}
	if slices.Contains(reservedStructuralTags, tag) {
		return "engine", true
	}
	return "", false
}

// applyTag adds or removes a free-form gameplay tag on a live player or mob
// (admin-verbs §4). The `type` slot is the op (add|remove); the value is the
// tag name. A tag in a manager-owned namespace (alignment / faction /
// reputation) is refused — those are derived and reconstructed at login, so a
// direct write would desync the manager (mirrors applyProperty refusing the
// reserved property keys; roles are likewise never settable via `set`).
//
// Scope (M19.4i): a player's tag persists in the AdminTags save bag — a player
// is not transient, so an admin tag must survive relog — while a room mob's
// tag is live-only (the mob is transient) but re-indexes the store so a later
// GetByTag reflects it. Both targets satisfy the AddTag/RemoveTag interface.
func applyTag(ctx context.Context, c *Context, target any, op, value string) (string, error) {
	tagger, ok := target.(interface {
		AddTag(string) bool
		RemoveTag(string) bool
	})
	if !ok {
		return "", fmt.Errorf("That target cannot carry gameplay tags.")
	}
	tag := strings.TrimSpace(value)
	if tag == "" {
		return "", fmt.Errorf("Tag name cannot be empty.")
	}
	if len(tag) > maxTagNameLen {
		return "", fmt.Errorf("Tag name is too long (max %d characters).", maxTagNameLen)
	}
	if label, reserved := reservedTagNamespace(tag); reserved {
		return "", fmt.Errorf("Tag %q is owned by the %s system and cannot be set directly.", tag, label)
	}

	var (
		changed bool
		verb    string
	)
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "add":
		changed, verb = tagger.AddTag(tag), "added"
	case "remove":
		changed, verb = tagger.RemoveTag(tag), "removed"
	default:
		return "", fmt.Errorf("Unknown tag op %q. Settable: add, remove.", op)
	}

	// A no-op (tag already present on add, or already absent on remove) writes
	// nothing, so it is reported like a usage error — the non-nil error makes
	// SetHandler skip auditAdmin, keeping the audit log to genuine tag changes.
	if !changed {
		if verb == "added" {
			return "", fmt.Errorf("Already tagged %q; nothing changed.", tag)
		}
		return "", fmt.Errorf("Not tagged %q; nothing changed.", tag)
	}

	// A tracked mob mutated its tag list in place; refresh the store's tag
	// index so GetByTag sees the change. A player (connActor) is not tracked in
	// the store, so there is nothing to re-index for it.
	if m, ok := target.(*entities.MobInstance); ok && c.Items != nil {
		if err := c.Items.Retag(m.ID()); err != nil {
			logging.From(ctx).Warn("set tag: store retag failed",
				slog.String("mob", string(m.ID())),
				slog.String("err", err.Error()))
		}
	}

	return fmt.Sprintf("tag %q %s.", tag, verb), nil
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

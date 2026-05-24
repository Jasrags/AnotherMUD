package command

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/slot"
)

// InventoryHandler implements `inventory` (alias `i`) — renders the
// items the actor is currently carrying, in pickup order.
//
// Items whose entity is no longer tracked in the store are skipped
// silently (matches the syncInventoryToSaveLocked drop policy: a
// runtime/store divergence is recoverable, not user-facing).
//
// Stacking ("3 healing potions") lands when the stacking service
// arrives — until then this is one line per instance.
func InventoryHandler(ctx context.Context, c *Context) error {
	if c.Items == nil {
		return c.Actor.Write(ctx, "You are carrying nothing.")
	}
	ids := c.Actor.Inventory()
	names := resolveItemNames(c.Items, ids)
	if len(names) == 0 {
		return c.Actor.Write(ctx, "You are carrying nothing.")
	}
	var b strings.Builder
	b.WriteString("You are carrying:")
	for _, n := range names {
		b.WriteString("\n  ")
		b.WriteString(n)
	}
	return c.Actor.Write(ctx, b.String())
}

// EquipmentHandler implements `equipment` (alias `eq`) — renders
// equipped items grouped by slot in slot-registration order. Empty
// slots are omitted (occupied-only listing); the help system, when
// it lands, is the right surface for "here are the slots you have."
//
// Multi-cap slots emit one line per occupied sub-slot, each with the
// base slot label — players don't see the `:index` suffix. Labels
// are padded to align item names, padding driven by the longest
// label across occupied slots in this render (not across the
// registry, so a rarely-used pack slot can't permanently widen the
// column).
func EquipmentHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Slots == nil {
		return c.Actor.Write(ctx, "You are wearing nothing.")
	}
	equipped := c.Actor.Equipment()
	if len(equipped) == 0 {
		return c.Actor.Write(ctx, "You are wearing nothing.")
	}

	type row struct {
		label string
		name  string
	}
	rows := make([]row, 0, len(equipped))
	// Iterate slot defs in registration order so the listing is stable
	// and matches the registration ordering documented in §3.1.
	for _, def := range c.Slots.All() {
		for i := 0; i < def.Max; i++ {
			key, err := slot.BuildKey(def.Name, i, def.Max)
			if err != nil {
				continue
			}
			id, ok := equipped[key]
			if !ok {
				continue
			}
			name, ok := lookupItemName(c.Items, id)
			if !ok {
				// Entity gone from store; skip silently to match the
				// inventory render's tolerance for store divergence.
				continue
			}
			rows = append(rows, row{label: def.Label, name: name})
		}
	}
	if len(rows) == 0 {
		return c.Actor.Write(ctx, "You are wearing nothing.")
	}

	// Width measured in runes, not bytes, so multi-byte UTF-8 labels
	// (pack-authored slots may use accented characters) still line up
	// visually. Engine baseline labels are ASCII so today the two are
	// equivalent.
	width := 0
	for _, r := range rows {
		if n := utf8.RuneCountInString(r.label); n > width {
			width = n
		}
	}
	var b strings.Builder
	b.WriteString("You are wearing:")
	for _, r := range rows {
		b.WriteString("\n  <")
		b.WriteString(r.label)
		b.WriteString(">")
		for k := utf8.RuneCountInString(r.label); k < width; k++ {
			b.WriteString(" ")
		}
		b.WriteString("  ")
		b.WriteString(r.name)
	}
	return c.Actor.Write(ctx, b.String())
}

// resolveItemNames returns the display names of ids in input order,
// skipping ids that don't resolve to a tracked ItemInstance.
func resolveItemNames(store *entities.Store, ids []entities.EntityID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := lookupItemName(store, id); ok {
			out = append(out, name)
		}
	}
	return out
}

// lookupItemName fetches the display name for id from the store.
// Returns ok=false if the entity is unknown or not an item.
func lookupItemName(store *entities.Store, id entities.EntityID) (string, bool) {
	e, ok := store.GetByID(id)
	if !ok {
		return "", false
	}
	it, ok := e.(*entities.ItemInstance)
	if !ok {
		return "", false
	}
	return it.Name(), true
}

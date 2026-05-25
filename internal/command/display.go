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
// Containers (M5.9b) get their contents listed one level deeper
// with extra indentation so the player can see what they've put
// where. Recursion stops at depth 1 — containers-in-containers
// (a pouch inside a backpack) print "(contents not shown)" rather
// than expand, on the theory that an unbounded tree could flood
// the terminal. Deepening this is a UI policy choice; the
// underlying Contents substrate supports arbitrary nesting.
//
// Stacking ("3 healing potions") lands when the stacking service
// arrives — until then this is one line per instance.
func InventoryHandler(ctx context.Context, c *Context) error {
	if c.Items == nil {
		return c.Actor.Write(ctx, "You are carrying nothing.")
	}
	ids := c.Actor.Inventory()
	if len(ids) == 0 {
		return c.Actor.Write(ctx, "You are carrying nothing.")
	}
	var b strings.Builder
	b.WriteString("You are carrying:")
	any := false
	for _, id := range ids {
		e, ok := c.Items.GetByID(id)
		if !ok {
			continue
		}
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		any = true
		b.WriteString("\n  ")
		b.WriteString(it.Name())
		if c.Contents != nil && it.Type() == itemTypeContainer {
			renderContainerContents(&b, c, it.ID(), 1)
		}
	}
	if !any {
		return c.Actor.Write(ctx, "You are carrying nothing.")
	}
	return c.Actor.Write(ctx, b.String())
}

// renderContainerContents appends the children of containerID to b
// at one indent level deeper than the parent line. depth caps the
// recursion: depth==1 prints children with name only; nested
// containers are summarized as "(contents not shown)" rather than
// expanded.
func renderContainerContents(b *strings.Builder, c *Context, containerID entities.EntityID, depth int) {
	children := c.Contents.In(containerID)
	if len(children) == 0 {
		return
	}
	indent := strings.Repeat("  ", depth+1)
	for _, childID := range children {
		e, ok := c.Items.GetByID(childID)
		if !ok {
			continue
		}
		child, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		b.WriteString("\n")
		b.WriteString(indent)
		b.WriteString(child.Name())
		if child.Type() == itemTypeContainer && len(c.Contents.In(child.ID())) > 0 {
			b.WriteString(" (contents not shown)")
		}
	}
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

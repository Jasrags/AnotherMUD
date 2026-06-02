package command

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Jasrags/AnotherMUD/internal/corpse"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stacking"
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
// Stacking (M21.2): identical items group into one line per stack with a
// trailing "(xN)" count. Singletons carry no count and, if they are
// containers, still expand their contents; a qty>1 stack shows only the
// count (the instances differ, so expanding one would mislead). A nil
// stacking service degrades to one line per item.
//
// Item decorations (M20.5): each line shows the (first item's) name with
// its rarity tag and essence glyph trailing inline (decoratedName,
// item-decorations §4). An undecorated, unstacked item renders exactly its
// bare name, so the listing is byte-for-byte what it was before (§1.1).
func InventoryHandler(ctx context.Context, c *Context) error {
	if c.Items == nil {
		return c.Actor.Write(ctx, "You are carrying nothing.")
	}
	items := collectItems(c.Items, c.Actor.Inventory())
	if len(items) == 0 {
		return c.Actor.Write(ctx, "You are carrying nothing.")
	}
	byID := make(map[entities.EntityID]*entities.ItemInstance, len(items))
	for _, it := range items {
		byID[it.ID()] = it
	}

	var b strings.Builder
	b.WriteString("You are carrying:")
	for _, st := range stackItems(c, items) {
		if len(st.ItemIDs) == 0 {
			continue
		}
		first := byID[st.ItemIDs[0]]
		if first == nil {
			// Defensive: every stack id comes from `items`, which built
			// byID, so this can't happen today. Guards against a future
			// service returning an id outside the input set — skip rather
			// than panic in decoratedName(nil).
			continue
		}
		b.WriteString("\n  ")
		b.WriteString(decoratedName(c, first))
		if st.Quantity > 1 {
			fmt.Fprintf(&b, " (x%d)", st.Quantity)
		} else if c.Contents != nil && first.Type() == itemTypeContainer {
			renderContainerContents(&b, c, first.ID(), 1)
		}
	}
	return c.Actor.Write(ctx, b.String())
}

// stackItems groups items through the stacking service (M21.1). When no
// service is wired (tests / headless paths), it falls back to one singleton
// stack per item, so the listing degrades to one line per item rather than
// failing.
func stackItems(c *Context, items []*entities.ItemInstance) []stacking.StackEntry {
	if c.Stacking != nil {
		return c.Stacking.Stack(items)
	}
	out := make([]stacking.StackEntry, 0, len(items))
	for _, it := range items {
		out = append(out, stacking.StackEntry{Quantity: 1, ItemIDs: []entities.EntityID{it.ID()}})
	}
	return out
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
		// Same trailing-inline decoration as the top-level lines.
		b.WriteString(decoratedName(c, child))
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

// lookAtTarget implements `look [in|at] <target>` for items and
// containers (loot-and-corpses §2.2). The target resolves among the
// actor's inventory and the room's items; a container (incl. a corpse)
// lists its contents and any coin pile, a plain item shows its name.
// Looking is gated only by presence, not by §4 looting rights — anyone
// in the room may see what a corpse holds; only taking is restricted.
func (c *Context) lookAtTarget(ctx context.Context, toks []string) error {
	if c.Items == nil || len(toks) == 0 {
		return c.Actor.Write(ctx, "You don't see that here.")
	}
	target := c.resolveLookTarget(toks[0])
	if target == nil {
		return c.Actor.Write(ctx, "You don't see that here.")
	}
	if target.Type() == entities.ContainerType || corpse.IsCorpse(target) {
		return c.Actor.Write(ctx, c.renderContainerLook(target))
	}
	return c.Actor.Write(ctx, fmt.Sprintf("You see %s.", decoratedName(c, target)))
}

// resolveLookTarget keyword-matches an item across the actor's inventory
// and the current room's items (mobs/players are not look-at targets
// here — `consider` covers creatures). Returns nil on no match.
func (c *Context) resolveLookTarget(token string) *entities.ItemInstance {
	cands := collectItems(c.Items, c.Actor.Inventory())
	if room := c.Actor.Room(); room != nil && c.Placement != nil {
		cands = append(cands, collectItems(c.Items, c.Placement.InRoom(room.ID))...)
	}
	match := keyword.Resolve(asNamed(cands), token)
	if match == nil {
		return nil
	}
	it, _ := match.(*entities.ItemInstance)
	return it
}

// renderContainerLook formats a container's contents for look-in: the
// container name, then one line per contained item (decorated), then a
// coin line for a corpse with a coin pile. An empty container reports so.
func (c *Context) renderContainerLook(target *entities.ItemInstance) string {
	var contents []entities.EntityID
	if c.Contents != nil {
		contents = c.Contents.In(target.ID())
	}
	coins := 0
	if corpse.IsCorpse(target) {
		coins = corpse.Coins(target)
	}

	var b strings.Builder
	b.WriteString(decoratedName(c, target))
	if len(contents) == 0 && coins == 0 {
		b.WriteString(" is empty.")
		return b.String()
	}
	b.WriteString(" contains:")
	if c.Contents != nil {
		renderContainerContents(&b, c, target.ID(), 0)
	}
	if coins > 0 {
		b.WriteString("\n  ")
		b.WriteString(fmt.Sprintf("%d gold", coins))
	}
	return b.String()
}

package command

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Jasrags/AnotherMUD/internal/corpse"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
	"github.com/Jasrags/AnotherMUD/internal/light"
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

// EquipmentHandler implements `equipment` (alias `eq`) — renders EVERY
// equip slot in slot-registration order, so the player can see what
// slots exist (and their names) without guessing. An empty slot renders
// as `(empty)`; an occupied one shows the item colored by rarity. A
// multi-cap slot (e.g. two ring fingers) emits one line per sub-slot,
// each with the base slot label — players never see the `:index` suffix.
// Shares the score sheet's gatherer (gatherScoreEquip) so the focused
// `eq` view and the score sheet's Equipment section list the same slots
// with the same coloring.
func EquipmentHandler(ctx context.Context, c *Context) error {
	if c.Items == nil || c.Slots == nil {
		return c.Actor.Write(ctx, "You have no equipment slots right now.")
	}
	rows := gatherScoreEquip(c)
	if len(rows) == 0 {
		// No slots registered at all — nothing to show.
		return c.Actor.Write(ctx, "You have no equipment slots.")
	}
	return c.Actor.Write(ctx, renderEquipRows("You are wearing:", rows))
}

// renderEquipRows formats equipment rows as a "label  name" block under a
// <title> header: labels are <subtle>, item names arrive already wrapped
// in their rarity markup. Label width is measured in runes (not bytes) so
// multi-byte pack-authored slot labels still line up; engine-baseline
// labels are ASCII so today the two are equivalent.
func renderEquipRows(header string, rows []equipRow) string {
	width := 0
	for _, r := range rows {
		if n := utf8.RuneCountInString(r.Label); n > width {
			width = n
		}
	}
	var b strings.Builder
	b.WriteString("<title>")
	b.WriteString(header)
	b.WriteString("</title>")
	for _, r := range rows {
		b.WriteString("\n  <subtle>")
		b.WriteString(r.Label)
		b.WriteString("</subtle>")
		for k := utf8.RuneCountInString(r.Label); k < width; k++ {
			b.WriteString(" ")
		}
		b.WriteString("  ")
		b.WriteString(r.Name)
	}
	return b.String()
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
	if len(toks) == 0 {
		return c.Actor.Write(ctx, "You don't see that here.")
	}
	token := toks[0]

	// Light gate (light-and-darkness §5.2): examining (and reading, which
	// is `look` at a sign/label) anything in the room requires at least
	// `dim`. Below that, only items the viewer is carrying can be made out
	// (you can feel what you hold); room items, containers, and creatures
	// return a too-dark response. Held-item inspection is never gated.
	tooDark := c.Light != nil && c.Actor.Room() != nil &&
		c.effectiveLight(c.Actor.Room()) < light.Dim
	if tooDark {
		if c.Items != nil {
			if target := c.resolveHeldItem(token); target != nil {
				if target.Type() == entities.ContainerType || corpse.IsCorpse(target) {
					return c.Actor.Write(ctx, c.renderContainerLook(target))
				}
				return c.Actor.Write(ctx, describeThing(decoratedName(c, target), target.Description()))
			}
		}
		return c.Actor.Write(ctx, tooDarkToSeeText)
	}

	// 1. Items (inventory + floor). Containers/corpses are looked INTO;
	//    a plain item renders its appearance prose (or a fallback).
	if c.Items != nil {
		if target := c.resolveLookTarget(token); target != nil {
			if target.Type() == entities.ContainerType || corpse.IsCorpse(target) {
				return c.Actor.Write(ctx, c.renderContainerLook(target))
			}
			return c.Actor.Write(ctx, describeThing(decoratedName(c, target), target.Description()))
		}
	}

	// 2. Creatures — the appearance lens (mobs by keyword, players by
	//    name). `consider` owns the tactical lens; `look` never shows
	//    HP/AC, only how the target appears.
	if name, desc, ok := c.resolveLookCreature(token); ok {
		return c.Actor.Write(ctx, describeThing(name, desc))
	}

	return c.Actor.Write(ctx, "You don't see that here.")
}

// tooDarkToSeeText is the response when a viewer tries to examine or
// read something in the room below `dim` light (§5.2). Hardcoded for
// v1; externalizing it (§11 examination/reading) is deferred.
const tooDarkToSeeText = "It is too dark to make it out."

// resolveHeldItem keyword-matches an item across the actor's inventory
// ONLY (not the room). Used by the dark-look path: you can examine what
// you carry by feel even when the room is too dark to see.
func (c *Context) resolveHeldItem(token string) *entities.ItemInstance {
	match := keyword.Resolve(asNamed(collectItems(c.Items, c.Actor.Inventory())), token)
	if match == nil {
		return nil
	}
	it, _ := match.(*entities.ItemInstance)
	return it
}

// describeThing renders the appearance line for a look target: the
// authored (item/mob) or generated (player) prose when present, else a
// generic fallback that still names the thing. Shared by the item and
// creature paths so the two lenses read consistently.
func describeThing(name, desc string) string {
	if strings.TrimSpace(desc) != "" {
		return desc
	}
	return fmt.Sprintf("You see nothing special about %s.", name)
}

// resolveLookCreature finds a creature (mob or player) in the current
// room matching token and returns its display name + description for the
// appearance lens. Mobs resolve by keyword (the shared resolver); players
// by case-insensitive name prefix via the Locator — mirroring the
// mobs-first / players-by-name asymmetry the targeted verbs use. The
// player description is generated by the session layer (connActor.
// Description); a test actor without that method yields an empty desc and
// the generic fallback. Returns ok=false when nothing matches.
func (c *Context) resolveLookCreature(token string) (name, desc string, ok bool) {
	room := c.Actor.Room()
	if room == nil {
		return "", "", false
	}
	if mob := findMobByKeyword(c, room.ID, token); mob != nil {
		return mob.Name(), mob.Description(), true
	}
	if c.Locator != nil {
		lower := strings.ToLower(strings.TrimSpace(token))
		for _, p := range c.Locator.PlayersInRoom(room.ID) {
			if p == nil || p.ID() == c.Actor.ID() {
				continue // never look at yourself via the creature path
			}
			if strings.HasPrefix(strings.ToLower(p.Name()), lower) {
				return p.Name(), describePlayer(p), true
			}
		}
	}
	return "", "", false
}

// describePlayer pulls a generated description off an actor that
// implements the optional Describer surface (the session connActor does;
// test stubs may not). Empty when unavailable — the caller renders the
// generic fallback.
func describePlayer(a Actor) string {
	if d, ok := a.(interface{ Description() string }); ok {
		return d.Description()
	}
	return ""
}

// resolveLookTarget keyword-matches an item across the actor's inventory
// and the current room's items. Creatures are resolved separately (see
// resolveLookCreature) — `look` now spans both lenses' appearance layer.
// Returns nil on no match.
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

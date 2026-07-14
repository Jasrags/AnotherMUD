package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
)

// Item modification verbs (item-modification.md — Slice A, capacity).
//
//   - `modify <armor>`            — show the host's capacity + installed mods.
//   - `modify <armor> <mod>`      — install a carried modification into a carried host.
//   - `unmodify <armor> <mod>`    — remove an installed modification back to inventory.
//
// v1 scope decision: both the host and the modification are resolved from the
// actor's INVENTORY. A host must be carried (unequipped) to be modified — a bench
// action — so the effect aggregation is always computed fresh on the next equip
// and there is no reverse-while-worn recompute (item-modification §5 note).

// resolveCarried keyword-matches token against the actor's carried items.
func resolveCarried(c *Context, token string) (*entities.ItemInstance, bool) {
	items := collectItems(c.Items, c.Actor.Inventory())
	named := keyword.Resolve(asNamed(items), token)
	if named == nil {
		return nil, false
	}
	it, ok := named.(*entities.ItemInstance)
	return it, ok
}

// namesEquippedItem reports whether token keyword-matches something the actor is
// wearing/wielding — so `modify` can tell a player to take it off first rather
// than just "you aren't carrying that".
func namesEquippedItem(c *Context, token string) bool {
	ids := make([]entities.EntityID, 0, len(c.Actor.Equipment()))
	seen := make(map[entities.EntityID]bool)
	for _, id := range c.Actor.Equipment() {
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return keyword.Resolve(asNamed(collectItems(c.Items, ids)), token) != nil
}

// upFirst capitalizes the first rune so an item name can open a sentence.
func upFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ModifyHandler implements `modify` (item-modification §4 + §8).
func ModifyHandler(ctx context.Context, c *Context) error {
	if c.Items == nil {
		return c.Actor.Write(ctx, "You can't modify anything right now.")
	}
	if len(c.Args) == 0 {
		return c.Actor.Write(ctx, "Modify what? (modify <armor> [modification])")
	}
	host, ok := resolveCarried(c, c.Args[0])
	if !ok {
		if namesEquippedItem(c, c.Args[0]) {
			return c.Actor.Write(ctx, "Take it off first — you can only modify gear you're carrying.")
		}
		return c.Actor.Write(ctx, fmt.Sprintf("You aren't carrying %q.", c.Args[0]))
	}
	if host.Capacity() <= 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s can't be modified.", upFirst(host.Name())))
	}

	// One arg: the info form (§8) — capacity + installed mods.
	if len(c.Args) == 1 {
		return c.Actor.Write(ctx, modInfoLines(host))
	}

	// Two args: install a carried modification (§4).
	modToken := c.Args[1]
	mod, ok := resolveCarried(c, modToken)
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("You aren't carrying %q.", modToken))
	}
	if mod.ID() == host.ID() {
		return c.Actor.Write(ctx, "You can't install something into itself.")
	}
	if !mod.IsModification() {
		return c.Actor.Write(ctx, fmt.Sprintf("%s isn't a modification.", upFirst(mod.Name())))
	}
	switch err := host.InstallMod(mod); {
	case errors.Is(err, entities.ErrModIncompatible):
		return c.Actor.Write(ctx, fmt.Sprintf("%s doesn't fit %s.", upFirst(mod.Name()), host.Name()))
	case errors.Is(err, entities.ErrModNoCapacity):
		return c.Actor.Write(ctx, fmt.Sprintf("%s needs %d capacity, but %s has only %d free.",
			upFirst(mod.Name()), mod.ModCapacityCost(), host.Name(), host.FreeCapacity()))
	case err != nil:
		return c.Actor.Write(ctx, fmt.Sprintf("You can't install %s into %s.", mod.Name(), host.Name()))
	}
	// Installed: the mod is now host state, not a carried entity — consume it.
	c.Actor.RemoveFromInventory(mod.ID())
	_ = c.Items.Untrack(mod.ID())
	return c.Actor.Write(ctx, fmt.Sprintf("You install %s into %s. (%d capacity free.)",
		mod.Name(), host.Name(), host.FreeCapacity()))
}

// UnmodifyHandler implements `unmodify` (item-modification §5).
func UnmodifyHandler(ctx context.Context, c *Context) error {
	if c.Items == nil {
		return c.Actor.Write(ctx, "You can't modify anything right now.")
	}
	if len(c.Args) < 2 {
		return c.Actor.Write(ctx, "Remove which modification from what? (unmodify <armor> <modification>)")
	}
	host, ok := resolveCarried(c, c.Args[0])
	if !ok {
		if namesEquippedItem(c, c.Args[0]) {
			return c.Actor.Write(ctx, "Take it off first — you can only modify gear you're carrying.")
		}
		return c.Actor.Write(ctx, fmt.Sprintf("You aren't carrying %q.", c.Args[0]))
	}
	if host.Capacity() <= 0 || len(host.InstalledMods()) == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s has no modifications to remove.", upFirst(host.Name())))
	}
	removed, ok := host.RemoveMod(c.Args[1])
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("%s has no modification matching %q.", upFirst(host.Name()), c.Args[1]))
	}
	// Re-spawn the modification as a carried item (§5 — recovered by default).
	if c.Spawn == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("You pry %s out of %s. (%d capacity free.)",
			removed.Name, host.Name(), host.FreeCapacity()))
	}
	id, _, err := c.Spawn.SpawnItem(ctx, string(removed.TemplateID))
	if err != nil {
		// Content removed since install: the mod can't be re-materialized. The
		// slot is freed regardless; tell the player it was lost.
		return c.Actor.Write(ctx, fmt.Sprintf("You pry %s out of %s, but it crumbles. (%d capacity free.)",
			removed.Name, host.Name(), host.FreeCapacity()))
	}
	c.Actor.AddToInventory(id)
	return c.Actor.Write(ctx, fmt.Sprintf("You remove %s from %s and pocket it. (%d capacity free.)",
		removed.Name, host.Name(), host.FreeCapacity()))
}

// withModLook appends a one-line capacity/mods summary to a modifiable host's
// look/examine text (item-modification §8); returns the plain description for an
// unmodifiable item.
func withModLook(it *entities.ItemInstance) string {
	desc := it.Description()
	line := modLookLine(it)
	switch {
	case line == "":
		return desc
	case desc == "":
		return line
	default:
		return desc + "\n" + line
	}
}

// modLookLine is the one-line capacity summary shown on look/examine; "" for an
// unmodifiable item.
func modLookLine(it *entities.ItemInstance) string {
	if it.Capacity() <= 0 {
		return ""
	}
	mods := it.InstalledMods()
	if len(mods) == 0 {
		return fmt.Sprintf("Capacity %d (all free).", it.Capacity())
	}
	names := make([]string, 0, len(mods))
	for _, m := range mods {
		names = append(names, m.Name)
	}
	return fmt.Sprintf("Capacity %d (%d free). Installed: %s.",
		it.Capacity(), it.FreeCapacity(), strings.Join(names, ", "))
}

// modInfoLines renders the host's capacity + installed-mod list (§8).
func modInfoLines(host *entities.ItemInstance) string {
	mods := host.InstalledMods()
	if len(mods) == 0 {
		return fmt.Sprintf("%s has %d capacity, all free — no modifications installed.",
			upFirst(host.Name()), host.Capacity())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — capacity %d (%d used, %d free):",
		upFirst(host.Name()), host.Capacity(), host.UsedCapacity(), host.FreeCapacity())
	for _, m := range mods {
		fmt.Fprintf(&b, "\n  - %s [%d]", m.Name, m.CapacityCost)
	}
	return b.String()
}

package command

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/keyword"
)

// Item modification verbs (item-modification.md).
//
//   - `modify <host>`            — show the host's capacity/mounts + installed mods.
//   - `modify <host> <mod>`      — install a carried modification into the host.
//   - `unmodify <host> <mod>`    — remove an installed modification back to inventory.
//
// The mod is resolved from the actor's inventory; the host may be carried OR worn
// (§5). Modifying a WORN host re-applies its equip modifier group + recomputes so
// the change lands live, and is barred in combat (the don-doff gate); a carried
// host applies everything on its next equip.

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

// resolveModHost resolves the host of a modify/unmodify operation from the
// actor's INVENTORY or EQUIPMENT, reporting whether the resolved host is
// currently worn/wielded (item-modification §5 — worn hosts are modifiable, with
// a live re-apply). Inventory is checked first; equipment second.
func resolveModHost(c *Context, token string) (host *entities.ItemInstance, worn bool, ok bool) {
	carried := collectItems(c.Items, c.Actor.Inventory())
	wornItems := equippedItems(c)

	// Prefer a MODIFIABLE match so a keyword shared with a non-modifiable item
	// doesn't shadow the real host — e.g. a loaded clip whose NAME carries "Ares
	// Predator V" would otherwise win `modify Ares laser` over the gun itself.
	// Carried modifiable first (mirrors the original inventory-before-equipment
	// order), then worn modifiable.
	if h := resolveModifiable(carried, token); h != nil {
		return h, false, true
	}
	if h := resolveModifiable(wornItems, token); h != nil {
		return h, true, true
	}

	// Fallback: any carried/worn match, so `modify <non-host>` still resolves and
	// reports "… can't be modified." rather than "you aren't carrying that."
	if h := resolveItem(carried, token); h != nil {
		return h, false, true
	}
	if h := resolveItem(wornItems, token); h != nil {
		return h, true, true
	}
	return nil, false, false
}

// equippedItems returns the actor's equipped item instances, de-duplicated
// (a spanning item occupies multiple slot keys under one id).
func equippedItems(c *Context) []*entities.ItemInstance {
	ids := make([]entities.EntityID, 0, len(c.Actor.Equipment()))
	seen := make(map[entities.EntityID]bool)
	for _, id := range c.Actor.Equipment() {
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return collectItems(c.Items, ids)
}

// resolveModifiable keyword-matches token against only the MODIFIABLE items in
// the set (capacity or mount hosts); nil when none match.
func resolveModifiable(items []*entities.ItemInstance, token string) *entities.ItemInstance {
	hosts := make([]*entities.ItemInstance, 0, len(items))
	for _, it := range items {
		if it.IsModifiable() {
			hosts = append(hosts, it)
		}
	}
	return resolveItem(hosts, token)
}

// resolveItem keyword-resolves token against an item set; nil when no match.
func resolveItem(items []*entities.ItemInstance, token string) *entities.ItemInstance {
	if named := keyword.Resolve(asNamed(items), token); named != nil {
		if it, ok := named.(*entities.ItemInstance); ok {
			return it
		}
	}
	return nil
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
	host, worn, ok := resolveModHost(c, c.Args[0])
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("You aren't carrying or wearing %q.", c.Args[0]))
	}
	if !host.IsModifiable() {
		return c.Actor.Write(ctx, fmt.Sprintf("%s can't be modified.", upFirst(host.Name())))
	}

	// One arg: the info form (§8) — capacity/mounts + installed mods.
	if len(c.Args) == 1 {
		return c.Actor.Write(ctx, modInfoLines(host))
	}

	// Modifying WORN gear is a bench action — barred in a firefight (§5 /
	// the action-economy don-doff gate). Carried gear is always free to work on.
	if worn && c.Actor.InCombat() {
		return c.Actor.Write(ctx, "You can't re-work your gear in the middle of a firefight.")
	}

	// Two args: install a carried modification. The admission rule is chosen by
	// the host — a capacity budget (item-modification §4) or named mount slots
	// (weapon-accessories §4).
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
	var mount string
	var err error
	if host.Capacity() > 0 {
		err = host.InstallMod(mod)
	} else {
		mount, err = host.AttachAccessory(mod)
	}
	switch {
	case errors.Is(err, entities.ErrModIncompatible):
		return c.Actor.Write(ctx, fmt.Sprintf("%s doesn't fit %s.", upFirst(mod.Name()), host.Name()))
	case errors.Is(err, entities.ErrNotAModification):
		return c.Actor.Write(ctx, fmt.Sprintf("%s isn't an accessory for %s.", upFirst(mod.Name()), host.Name()))
	case errors.Is(err, entities.ErrModNoCapacity):
		return c.Actor.Write(ctx, fmt.Sprintf("%s needs %d capacity, but %s has only %d free.",
			upFirst(mod.Name()), mod.ModCapacityCost(), host.Name(), host.FreeCapacity()))
	case errors.Is(err, entities.ErrMountOccupied):
		return c.Actor.Write(ctx, fmt.Sprintf("%s has no free mount that fits %s.", upFirst(host.Name()), mod.Name()))
	case err != nil:
		return c.Actor.Write(ctx, fmt.Sprintf("You can't install %s into %s.", mod.Name(), host.Name()))
	}
	// Installed: the mod is now host state, not a carried entity — consume it.
	c.Actor.RemoveFromInventory(mod.ID())
	_ = c.Items.Untrack(mod.ID())
	// A WORN host's contribution just changed — re-apply its modifier group and
	// recompute so the mod takes effect immediately (item-modification §5).
	// Resistances/protection refresh via the recompute; a stat-modifier/AC mod via
	// the re-applied group. A carried host applies everything on its next equip.
	if worn {
		c.Actor.RefreshEquipped(host.ID(), EquipModifiers(host, c.Grades, false))
	}
	if mount != "" {
		return c.Actor.Write(ctx, fmt.Sprintf("You attach %s to %s's %s mount.", mod.Name(), host.Name(), mount))
	}
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
	host, worn, ok := resolveModHost(c, c.Args[0])
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("You aren't carrying or wearing %q.", c.Args[0]))
	}
	if !host.IsModifiable() || len(host.InstalledMods()) == 0 {
		return c.Actor.Write(ctx, fmt.Sprintf("%s has no modifications to remove.", upFirst(host.Name())))
	}
	if worn && c.Actor.InCombat() {
		return c.Actor.Write(ctx, "You can't re-work your gear in the middle of a firefight.")
	}
	removed, ok := host.RemoveMod(c.Args[1])
	if !ok {
		return c.Actor.Write(ctx, fmt.Sprintf("%s has no modification matching %q.", upFirst(host.Name()), c.Args[1]))
	}
	// A WORN host's contribution just shrank — re-apply its modifier group and
	// recompute so the removed mod's effect reverses immediately (§5).
	if worn {
		c.Actor.RefreshEquipped(host.ID(), EquipModifiers(host, c.Grades, false))
	}
	// Re-spawn the modification as a carried item (§5 — recovered by default).
	note := capacityNote(host)
	if c.Spawn == nil {
		return c.Actor.Write(ctx, fmt.Sprintf("You pry %s out of %s.%s", removed.Name, host.Name(), note))
	}
	id, _, err := c.Spawn.SpawnItem(ctx, string(removed.TemplateID))
	if err != nil {
		// Content removed since install: the mod can't be re-materialized. The
		// slot is freed regardless; tell the player it was lost.
		return c.Actor.Write(ctx, fmt.Sprintf("You pry %s out of %s, but it crumbles.%s", removed.Name, host.Name(), note))
	}
	c.Actor.AddToInventory(id)
	return c.Actor.Write(ctx, fmt.Sprintf("You remove %s from %s and pocket it.%s", removed.Name, host.Name(), note))
}

// capacityNote is the " (N capacity free.)" suffix for a capacity host, or "" for
// a mount host (which has no numeric budget).
func capacityNote(host *entities.ItemInstance) string {
	if host.Capacity() > 0 {
		return fmt.Sprintf(" (%d capacity free.)", host.FreeCapacity())
	}
	return ""
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

// modLookLine is the one-line modification summary shown on look/examine; "" for
// an unmodifiable item.
func modLookLine(it *entities.ItemInstance) string {
	switch {
	case len(it.Mounts()) > 0: // mount host (weapon-accessories §8)
		mods := it.InstalledMods()
		if len(mods) == 0 {
			return fmt.Sprintf("Accessory mounts: %d (all free).", len(it.Mounts()))
		}
		return fmt.Sprintf("Accessory mounts: %d (%d free). Attached: %s.",
			len(it.Mounts()), len(it.FreeMounts()), modNamesWithEffects(mods))
	case it.Capacity() > 0: // capacity host (item-modification §8)
		mods := it.InstalledMods()
		if len(mods) == 0 {
			return fmt.Sprintf("Capacity %d (all free).", it.Capacity())
		}
		return fmt.Sprintf("Capacity %d (%d free). Installed: %s.",
			it.Capacity(), it.FreeCapacity(), modNamesWithEffects(mods))
	default:
		return ""
	}
}

// modNamesWithEffects joins installed mods as "name (effect)" for the one-line
// look summary, so the wearer sees what each mod gives — e.g. "a ballistic weave
// insert (+2 piercing soak)". A mod with no surfaced effect shows just its name.
func modNamesWithEffects(mods []entities.InstalledMod) string {
	parts := make([]string, 0, len(mods))
	for _, m := range mods {
		if eff := modEffectSummary(m); eff != "" {
			parts = append(parts, fmt.Sprintf("%s (%s)", m.Name, eff))
		} else {
			parts = append(parts, m.Name)
		}
	}
	return strings.Join(parts, ", ")
}

// modEffectSummary renders what an installed mod PROVIDES while its host is
// equipped (item-modification §8) — the armor/soak/to-hit/protection/grant it
// contributes — so a player can see the payoff, not just the mod's name. Empty
// when the mod carries no surfaced effect (an inert record-now accessory).
// Note a resistance (e.g. a ballistic weave's piercing soak) is NOT the same as
// the garment's flat armor rating: it reduces one damage type, so the base armor
// number is unchanged — surfacing it here is how the wearer sees the benefit.
func modEffectSummary(m entities.InstalledMod) string {
	parts := make([]string, 0, 4)
	if m.ArmorBonus != 0 {
		parts = append(parts, fmt.Sprintf("%+d armor", m.ArmorBonus))
	}
	for _, dt := range sortedIntKeys(m.Resistances) {
		parts = append(parts, fmt.Sprintf("%+d %s soak", m.Resistances[dt], dt))
	}
	for _, mod := range m.Modifiers {
		parts = append(parts, fmt.Sprintf("%+d %s", mod.Value, friendlyStat(mod.Stat)))
	}
	for _, p := range m.Protection {
		parts = append(parts, "protects vs "+p)
	}
	for _, g := range m.Grants {
		parts = append(parts, "grants "+g)
	}
	return strings.Join(parts, ", ")
}

// friendlyStat maps an engine stat key to a player-facing label for the mod
// effect summary; unknown keys pass through unchanged.
func friendlyStat(stat string) string {
	switch stat {
	case "hit_mod":
		return "to-hit"
	case "armor_check":
		return "armor check"
	default:
		return stat
	}
}

// sortedIntKeys returns a map's keys in deterministic (sorted) order so the
// effect summary reads the same every time.
func sortedIntKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// modInfoLines renders the host's capacity/mounts + installed-mod list (§8),
// dispatching on the host's admission rule.
func modInfoLines(host *entities.ItemInstance) string {
	if len(host.Mounts()) > 0 {
		return mountInfoLines(host)
	}
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
		if eff := modEffectSummary(m); eff != "" {
			fmt.Fprintf(&b, " — %s", eff)
		}
	}
	return b.String()
}

// mountInfoLines lists a weapon host's mount points, each with its occupant or
// "(empty)" (weapon-accessories §8).
func mountInfoLines(host *entities.ItemInstance) string {
	occupant := make(map[string]entities.InstalledMod)
	for _, m := range host.InstalledMods() {
		occupant[m.Mount] = m
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — accessory mounts:", upFirst(host.Name()))
	for _, mount := range host.Mounts() {
		m, ok := occupant[mount]
		if !ok {
			fmt.Fprintf(&b, "\n  - %s: (empty)", mount)
			continue
		}
		fmt.Fprintf(&b, "\n  - %s: %s", mount, m.Name)
		if eff := modEffectSummary(m); eff != "" {
			fmt.Fprintf(&b, " — %s", eff)
		}
	}
	return b.String()
}

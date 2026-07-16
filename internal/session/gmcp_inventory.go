package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/stacking"
)

// Affordance verbs a Char.Inventory row maps to. Each is a real command a
// player could type (the authority invariant); the row carries the FULL command
// string so the client sends it verbatim.
const (
	actionEquip   = "equip"
	actionUnequip = "unequip"
	actionDrop    = "drop"
	actionReload  = "reload"
	actionLoad    = "load"
)

// flushGmcpInventory snapshots the actor's carried + worn items into the rich
// Char.Inventory payload (web-client-plan P3) and emits a frame when it differs
// from the last-sent shadow. Rides the same gmcp-items-flush tick pass as
// flushGmcpItems (its sibling Char.Items.List) — one poll, two packages — with
// the same no-op guards (non-GMCP conn, GMCP inactive, no entity store).
//
// The diff is a marshaled-bytes compare (like Char.Vitals' gmcpLastVitalsJSON)
// rather than an element-wise struct compare: the payload nests slices, and
// comparing the encoded bytes is both simpler and exactly what "did the wire
// frame change" means. Guarded by gmcpItemsMu (shared with the sibling; both
// run in the same flush pass, so no extra lock is warranted).
func (a *connActor) flushGmcpInventory(ctx context.Context) {
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}
	if a.items == nil {
		return
	}

	payload := gmcp.CharInventory{
		Carried: a.buildCarriedItems(),
		Worn:    a.buildWornItems(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	a.gmcpItemsMu.Lock()
	unchanged := a.gmcpInventoryValid && string(a.gmcpInventoryLast) == string(data)
	if !unchanged {
		a.gmcpInventoryLast = data
		a.gmcpInventoryValid = true
	}
	a.gmcpItemsMu.Unlock()
	if unchanged {
		return
	}

	if err := sender.SendGmcp(ctx, gmcp.PackageCharInventory, data); err != nil {
		logging.From(ctx).Debug("gmcp inventory send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// buildCarriedItems resolves the actor's carried (unequipped) items into
// InventoryItem rows, in pickup order (matching the `inventory` verb). Stack-
// identical items collapse into one row with a quantity (M21 stacking) — EXCEPT
// ammunition holders (clips), which are listed individually so each shows its
// own load state, exactly as the CLI does.
func (a *connActor) buildCarriedItems() []gmcp.InventoryItem {
	insts, byID := a.carriedInstances()
	entries := a.stackEntries(insts)
	out := make([]gmcp.InventoryItem, 0, len(entries))
	for _, e := range entries {
		if len(e.ItemIDs) == 0 {
			continue
		}
		first, ok := byID[e.ItemIDs[0]]
		if !ok {
			continue
		}
		// Clips (holder + magazine) never collapse: each holder carries its own
		// loaded/type state, so a stack would misreport all as the first's.
		if isAmmoHolder(first) {
			for _, id := range e.ItemIDs {
				if it, ok := byID[id]; ok {
					out = append(out, carriedRow(it, 0))
				}
			}
			continue
		}
		out = append(out, carriedRow(first, e.Quantity))
	}
	return out
}

// carriedRow builds one carried InventoryItem: name, stack qty (omitted when 1),
// a clip's ammo detail, and the affordances that apply.
func carriedRow(it *entities.ItemInstance, quantity int) gmcp.InventoryItem {
	qty := quantity
	if qty <= 1 {
		qty = 0 // omit on the wire; a client reads absent as 1
	}
	return gmcp.InventoryItem{
		ID:      string(it.ID()),
		Name:    it.Name(),
		Qty:     qty,
		Detail:  holderDetail(it),
		Actions: carriedActions(it),
	}
}

// carriedActions returns the affordances for a carried item. A clip gets
// `reload <clip>` (fill it with loose rounds); an equippable item gets `equip`;
// everything gets `drop`.
func carriedActions(it *entities.ItemInstance) []gmcp.InvAction {
	token := itemToken(it)
	var acts []gmcp.InvAction
	switch {
	case isAmmoHolder(it):
		acts = append(acts, invAction(actionReload, actionReload+" "+token))
	case len(it.EligibleSlots()) > 0:
		acts = append(acts, invAction(actionEquip, actionEquip+" "+token))
	}
	return append(acts, invAction(actionDrop, actionDrop+" "+token))
}

// buildWornItems enumerates EVERY equipment slot in registration order (via the
// slot registry), so empty slots appear too — mirroring the `equipment` verb. A
// spanning item (a two-handed weapon) shows under each slot it fills, exactly as
// that verb lists it. When no slot registry is wired (headless/test), it falls
// back to listing only occupied slots (one row per unique item under its
// primary slot).
func (a *connActor) buildWornItems() []gmcp.WornItem {
	if a.slots == nil {
		return a.buildWornItemsFallback()
	}
	attrs := a.AttributeSet()
	equipped := a.Equipment()
	var out []gmcp.WornItem
	for _, def := range a.slots.All() {
		for i := 0; i < def.Max; i++ {
			key, err := slot.BuildKey(def.Name, i, def.Max)
			if err != nil {
				continue
			}
			id, occupied := equipped[key]
			if !occupied {
				out = append(out, gmcp.WornItem{Slot: def.Name, Empty: true})
				continue
			}
			it, ok := a.itemInstanceByID(id)
			if !ok {
				out = append(out, gmcp.WornItem{Slot: def.Name, Empty: true})
				continue
			}
			out = append(out, wornRow(it, def.Name, attrs))
		}
	}
	return out
}

// buildWornItemsFallback lists only occupied slots when no slot registry is
// available: one row per unique item under its lexicographically-smallest slot
// (a spanning item shows once), sorted by slot then id for a stable shadow.
func (a *connActor) buildWornItemsFallback() []gmcp.WornItem {
	eq := a.Equipment()
	primary := make(map[entities.EntityID]string, len(eq))
	for slotName, id := range eq {
		if cur, seen := primary[id]; !seen || slotName < cur {
			primary[id] = slotName
		}
	}
	out := make([]gmcp.WornItem, 0, len(primary))
	for id, slotName := range primary {
		it, ok := a.itemInstanceByID(id)
		if !ok {
			continue
		}
		out = append(out, wornRow(it, slotName, a.AttributeSet()))
	}
	sortWorn(out)
	return out
}

// wornRow builds one occupied WornItem: name, a mechanical detail (stat mods +
// armor + a wielded weapon's ammo state), and the affordances (unequip, plus
// reload/load for a reloadable ranged weapon).
func wornRow(it *entities.ItemInstance, slotName string, attrs *progression.AttributeSet) gmcp.WornItem {
	return gmcp.WornItem{
		Slot:    slotName,
		ID:      string(it.ID()),
		Name:    it.Name(),
		Detail:  joinDetail(effectDetail(attrs, it), weaponAmmoDetail(it)),
		Actions: wornActions(it),
	}
}

// wornActions returns the affordances for a worn item: reload (a holder-fed OR
// internally-fed-magazine firearm) or load (a reload-gated projectile like a
// crossbow) — all three target the wielded weapon, so the command is the bare
// verb — then unequip. The three feed models mirror the CLI's ReloadHandler
// switch (AcceptsHolder → Magazine → ReloadTicks) so the panel offers reload
// exactly when the command applies.
func wornActions(it *entities.ItemInstance) []gmcp.InvAction {
	var acts []gmcp.InvAction
	switch {
	case it.AcceptsHolder() != "" || it.Magazine() > 0:
		acts = append(acts, invAction(actionReload, actionReload))
	case it.ReloadTicks() > 0:
		acts = append(acts, invAction(actionLoad, actionLoad))
	}
	return append(acts, invAction(actionUnequip, actionUnequip+" "+itemToken(it)))
}

// --- detail helpers (plain text, ruleset-agnostic — no color markup) ---

// isAmmoHolder reports whether it is an ammunition holder (a clip/magazine):
// the same test the CLI inventory + fill paths use.
func isAmmoHolder(it *entities.ItemInstance) bool {
	return it.HolderFits() != "" && it.Magazine() > 0
}

// holderDetail is a carried clip's plain load readout: "15/15 APDS" loaded,
// "2/15 standard", or "empty". Returns "" for anything that isn't a holder.
func holderDetail(it *entities.ItemInstance) string {
	if !isAmmoHolder(it) {
		return ""
	}
	loaded, capacity := it.MagazineLoaded(), it.Magazine()
	if loaded <= 0 {
		return "empty"
	}
	return fmt.Sprintf("%d/%d %s", loaded, capacity, ammoTypeLabel(it.HolderAmmoGrade()))
}

// weaponAmmoDetail is a wielded weapon's plain ammo readout — the plain-text
// analogue of the CLI's weaponAmmoState. A holder-fed firearm reads "7 rds APDS"
// / "empty"; an internally-fed magazine reads "12/15 rds". Empty for a weapon
// that takes no ammo.
func weaponAmmoDetail(it *entities.ItemInstance) string {
	switch {
	case it.AcceptsHolder() != "":
		_, rounds, grade, has := it.InsertedHolder()
		if has && rounds > 0 {
			return fmt.Sprintf("%d rds %s", rounds, ammoTypeLabel(grade))
		}
		return "empty"
	case it.Magazine() > 0:
		rounds := it.MagazineLoaded()
		if rounds > 0 {
			return fmt.Sprintf("%d/%d rds %s", rounds, it.Magazine(), ammoTypeLabel(it.HolderAmmoGrade()))
		}
		return fmt.Sprintf("%d/%d rds", rounds, it.Magazine())
	}
	return ""
}

// effectDetail renders an item's mechanical grants as plain text: stat modifiers
// ("+1 Intuition") and any armor bonus ("Armor 4"). Modifier labels resolve
// through the world's attribute set (a nil set humanizes the raw stat key). "" when
// the item grants nothing. Mirrors the CLI's itemEffectSummary without markup.
func effectDetail(attrs *progression.AttributeSet, it *entities.ItemInstance) string {
	var parts []string
	for _, m := range it.Modifiers() {
		parts = append(parts, fmt.Sprintf("%+d %s", m.Value, statLabel(attrs, m.Stat)))
	}
	if ab := it.ArmorBonus(); ab != 0 {
		parts = append(parts, fmt.Sprintf("Armor %d", ab))
	}
	return strings.Join(parts, ", ")
}

// statLabel is the modifier label lookup: the attribute set's display name, else
// the humanized stat key. Mirrors the command layer's statLabel so the readouts
// match `eq`/`score`.
func statLabel(attrs *progression.AttributeSet, stat string) string {
	if attrs != nil {
		if at, ok := attrs.Get(progression.StatType(stat)); ok && at.Name != "" {
			return at.Name
		}
	}
	return humanizeStat(stat)
}

// humanizeStat turns a snake_case stat key into a spaced, title-cased label
// ("body" → "Body", "one_power" → "One Power") for the no-attribute-set fallback.
func humanizeStat(stat string) string {
	words := strings.Split(strings.ReplaceAll(stat, "_", " "), " ")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// ammoTypeLabel renders a loaded round's type from its grade: "" → "standard",
// else the uppercased grade key ("apds" → "APDS"). Mirrors the CLI helper.
func ammoTypeLabel(grade string) string {
	if grade == "" {
		return "standard"
	}
	return strings.ToUpper(grade)
}

// joinDetail joins the non-empty detail segments (effect, ammo) with " · " so a
// worn firearm reads "…mods… · 7 rds APDS" and an item with only one segment
// carries no separator.
func joinDetail(parts ...string) string {
	nonEmpty := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, " · ")
}

// action builds an InvAction from a label and its full command string.
func invAction(label, cmd string) gmcp.InvAction {
	return gmcp.InvAction{Label: label, Cmd: cmd}
}

// itemToken is the resolvable command argument for an item: its first keyword,
// else the last word of its name (its noun), else the whole name. Matches how a
// player refers to the item at the CLI.
func itemToken(it *entities.ItemInstance) string {
	if kw := it.Keywords(); len(kw) > 0 {
		return kw[0]
	}
	if words := strings.Fields(it.Name()); len(words) > 0 {
		return words[len(words)-1]
	}
	return it.Name()
}

// --- resolution helpers ---

// carriedInstances resolves the actor's carried ids to concrete item instances
// (for stacking) plus an id→instance index (for per-stack field lookup). Ids
// that no longer resolve or aren't items are skipped.
func (a *connActor) carriedInstances() ([]*entities.ItemInstance, map[entities.EntityID]*entities.ItemInstance) {
	ids := a.Inventory()
	insts := make([]*entities.ItemInstance, 0, len(ids))
	byID := make(map[entities.EntityID]*entities.ItemInstance, len(ids))
	for _, id := range ids {
		it, ok := a.itemInstanceByID(id)
		if !ok {
			continue
		}
		insts = append(insts, it)
		byID[id] = it
	}
	return insts, byID
}

// stackEntries groups instances via the M21 stacking service when one is wired,
// else falls back to one singleton entry per item (mirrors the command layer's
// stackItems fallback). Either way each entry carries its ItemIDs + Quantity.
func (a *connActor) stackEntries(insts []*entities.ItemInstance) []stacking.StackEntry {
	if a.stacking != nil {
		return a.stacking.Stack(insts)
	}
	out := make([]stacking.StackEntry, 0, len(insts))
	for _, it := range insts {
		out = append(out, stacking.StackEntry{Quantity: 1, ItemIDs: []entities.EntityID{it.ID()}})
	}
	return out
}

// itemInstanceByID resolves an entity id to a concrete *entities.ItemInstance,
// or (nil, false) if the id no longer resolves or isn't an item.
func (a *connActor) itemInstanceByID(id entities.EntityID) (*entities.ItemInstance, bool) {
	ent, ok := a.items.GetByID(id)
	if !ok {
		return nil, false
	}
	it, ok := ent.(*entities.ItemInstance)
	return it, ok
}

// sortWorn orders worn rows by (slot, id) for a stable shadow in the fallback
// path (the registry path is already deterministic in registration order).
func sortWorn(rows []gmcp.WornItem) {
	// Small n (worn slots); insertion sort keeps it dependency-free and stable.
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && wornLess(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func wornLess(x, y gmcp.WornItem) bool {
	if x.Slot != y.Slot {
		return x.Slot < y.Slot
	}
	return x.ID < y.ID
}

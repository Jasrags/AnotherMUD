package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// FlushGmcpItems walks every live session and emits Char.Items.List
// GMCP frames for actors whose inventory or equipment snapshot has
// changed since the last emission. Called once per simulation tick
// from the gmcp-items-flush handler.
//
// Mirrors FlushGmcpVitals: cadence-1 poll-and-diff, per-actor
// snapshots compared against a last-sent shadow. The diff fan-out
// is per-LOCATION (inv vs wear) so a player who only equips a
// helm sees one wear-list frame and no inv-list frame, not both.
func (m *Manager) FlushGmcpItems(ctx context.Context) {
	m.mu.RLock()
	snapshot := make([]*connActor, 0, len(m.byConn))
	for _, a := range m.byConn {
		snapshot = append(snapshot, a)
	}
	m.mu.RUnlock()

	for _, a := range snapshot {
		if a.isLinkDead() {
			continue
		}
		a.flushGmcpItems(ctx)
	}
}

// flushGmcpItems snapshots the actor's inventory + equipment,
// builds per-location CharItem lists, and emits per-location
// Char.Items.List frames when the snapshot differs from the
// last-sent shadow.
//
// Silent no-op when:
//
//   - the underlying conn doesn't speak GMCP;
//   - GMCP hasn't been negotiated;
//   - the actor has no entity store (test fakes).
//
// Per-location diff: inv and wear are tracked separately so a
// pure-inventory change skips the wear frame and vice versa.
// The valid flag (gmcpItemsLastValid) distinguishes "never sent"
// from "sent and matches empty" — without it, a freshly-reset
// shadow would silently swallow the first frame of an actor
// whose inventory is genuinely empty.
func (a *connActor) flushGmcpItems(ctx context.Context) {
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}
	if a.items == nil {
		return
	}

	inv := a.snapshotItemsForInventory()
	wear := a.snapshotItemsForEquipment()

	a.gmcpItemsMu.Lock()
	wasValid := a.gmcpItemsLastValid
	invChanged := !wasValid || !charItemsEqual(a.gmcpItemsLastInv, inv)
	wearChanged := !wasValid || !charItemsEqual(a.gmcpItemsLastWear, wear)
	if invChanged {
		a.gmcpItemsLastInv = inv
	}
	if wearChanged {
		a.gmcpItemsLastWear = wear
	}
	a.gmcpItemsLastValid = true
	a.gmcpItemsMu.Unlock()

	if invChanged {
		a.sendItemsList(ctx, sender, gmcp.LocationInventory, inv)
	}
	if wearChanged {
		a.sendItemsList(ctx, sender, gmcp.LocationWear, wear)
	}
}

// sendItemsList builds + ships one Char.Items.List frame. Does
// no locking itself — the caller (flushGmcpItems) has already
// released gmcpItemsMu by the time it gets here, so the send
// runs only under the conn's own write mutex.
func (a *connActor) sendItemsList(ctx context.Context, sender gmcpSender, location string, items []gmcp.CharItem) {
	payload := gmcp.CharItemsList{Location: location, Items: items}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if err := sender.SendGmcp(ctx, gmcp.PackageCharItemsList, data); err != nil {
		logging.From(ctx).Debug("gmcp items.list send failed",
			slog.String("player", a.PlayerName()),
			slog.String("location", location),
			slog.Any("err", err))
	}
}

// snapshotItemsForInventory builds the sorted CharItem slice for
// the actor's inventory. Sorted by id so the shadow diff is
// stable across iteration-order variation (Go map ranging is
// random, but a.inventory is a slice and stable on its own; the
// sort is defensive against future ordering changes).
func (a *connActor) snapshotItemsForInventory() []gmcp.CharItem {
	ids := a.Inventory()
	return a.entityIDsToCharItems(ids)
}

// snapshotItemsForEquipment builds the sorted CharItem slice for
// the actor's equipped items. The equipment map is iterated and
// the resulting items sorted by id so two equip operations
// applied in different orders produce the same shadow.
func (a *connActor) snapshotItemsForEquipment() []gmcp.CharItem {
	eq := a.Equipment()
	ids := make([]entities.EntityID, 0, len(eq))
	for _, id := range eq {
		ids = append(ids, id)
	}
	return a.entityIDsToCharItems(ids)
}

// entityIDsToCharItems resolves each id through the entity store
// and returns a sorted CharItem slice. Entities that fail
// resolution (the id was removed between snapshot and lookup —
// vanishingly rare on a single goroutine but possible across
// goroutines) are silently dropped. Always returns a non-nil
// slice (possibly length zero) so the JSON encoder emits `[]`
// rather than `null` per the CharItemsList contract.
func (a *connActor) entityIDsToCharItems(ids []entities.EntityID) []gmcp.CharItem {
	out := make([]gmcp.CharItem, 0, len(ids))
	for _, id := range ids {
		ent, ok := a.items.GetByID(id)
		if !ok {
			continue
		}
		name := ""
		if named, ok := ent.(interface{ Name() string }); ok {
			name = named.Name()
		}
		out = append(out, gmcp.CharItem{ID: string(id), Name: name})
	}
	slices.SortFunc(out, func(x, y gmcp.CharItem) int {
		if x.ID < y.ID {
			return -1
		}
		if x.ID > y.ID {
			return 1
		}
		return 0
	})
	return out
}

// charItemsEqual reports whether two CharItem slices have the
// same length and the same ID/Name at every index. Both inputs
// are expected sorted (see entityIDsToCharItems) so the
// element-wise compare is meaningful.
func charItemsEqual(a, b []gmcp.CharItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// resetGmcpItemsShadow marks the last-sent inventory/equipment
// shadows invalid so the next flushGmcpItems call emits
// unconditionally. Called on link-dead reattach: the new peer's
// Mudlet inventory module needs a baseline frame even when the
// engine-side state hasn't changed across the drop.
func (a *connActor) resetGmcpItemsShadow() {
	a.gmcpItemsMu.Lock()
	a.gmcpItemsLastValid = false
	a.gmcpItemsLastInv = nil
	a.gmcpItemsLastWear = nil
	a.gmcpItemsMu.Unlock()
}

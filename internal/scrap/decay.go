// Package scrap decays ephemeral dropped items — the spent ammunition holders a
// firearm ejects to the ground (ammo-and-reloading §7). A dropped holder is
// recoverable (pick it up, refill it, reuse it) for a lifetime window, then a
// tick sweep removes it so a firefight doesn't permanently litter a room with
// brass. Mirrors the corpse-decay pattern (loot-and-corpses §7): a tag marks the
// scrap, a creation tick stamps it, and a periodic sweep removes the expired.
package scrap

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
)

const (
	// TagScrap marks an item as ephemeral dropped scrap that the decay sweep
	// removes after its lifetime. Runtime-added on ejection (not a template tag).
	TagScrap = "scrap"
	// propDroppedTick is the tick the item was dropped as scrap (uint64).
	propDroppedTick = "scrap_dropped_tick"
)

// Mark flags a just-dropped item as ephemeral scrap and stamps the drop tick so
// the decay sweep can remove it once its lifetime elapses. Re-indexes the store
// so GetByTag(TagScrap) finds it. A no-op on nil inputs.
func Mark(store *entities.Store, it *entities.ItemInstance, nowTick uint64) {
	if store == nil || it == nil {
		return
	}
	it.SetProperty(propDroppedTick, nowTick)
	if it.AddTag(TagScrap) {
		_ = store.Retag(it.ID())
	}
}

// droppedTick returns the tick an item was dropped as scrap (0 if absent).
func droppedTick(it *entities.ItemInstance) uint64 {
	if v, ok := it.Property(propDroppedTick); ok {
		if n, ok := v.(uint64); ok {
			return n
		}
	}
	return 0
}

// Sweep removes every scrap item whose lifetime (in ticks) has elapsed since it
// was dropped, returning the number removed. An item that was picked up (no
// longer in a room) is skipped — Placement.Remove is the single-winner claim, so
// a recovered holder survives in the finder's hands rather than being swept.
//
// Wired as a tick handler; scrap is transient (never persisted), so a restart
// removes it too — this sweep just bounds growth on a live server.
func Sweep(store *entities.Store, placement *entities.Placement, nowTick, lifetime uint64) int {
	if store == nil || placement == nil {
		return 0
	}
	removed := 0
	for _, e := range store.GetByTag(TagScrap) {
		it, ok := e.(*entities.ItemInstance)
		if !ok {
			continue
		}
		dropped := droppedTick(it)
		// Subtract-first (never dropped+lifetime) avoids uint64 overflow;
		// nowTick < dropped (clock skew) means "not expired".
		if nowTick < dropped || nowTick-dropped < lifetime {
			continue
		}
		if !placement.Remove(it.ID()) {
			continue // picked up (or already removed) — not on the ground to sweep
		}
		_ = store.Untrack(it.ID())
		removed++
	}
	return removed
}

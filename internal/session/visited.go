package session

import "github.com/Jasrags/AnotherMUD/internal/world"

// Fog-of-war exploration tracking (player-maps §3). The persisted
// authority is player.Save.VisitedRooms; connActor.visited is the O(1)
// in-memory membership index over it, lazily seeded from the save on
// first use. The map renderers (later phases) read HasVisited /
// VisitedRooms; the room-entry hook (SetRoom) writes via
// markVisitedLocked.

// ensureVisitedLocked lazily builds the in-memory visited set from the
// persisted save the first time it is needed. Caller holds a.mu.
func (a *connActor) ensureVisitedLocked() {
	if a.visited != nil {
		return
	}
	if a.save == nil {
		a.visited = make(map[string]struct{})
		return
	}
	a.visited = make(map[string]struct{}, len(a.save.VisitedRooms))
	for _, id := range a.save.VisitedRooms {
		a.visited[id] = struct{}{}
	}
}

// markVisitedLocked records roomID in the character's fog-of-war visited
// set (player-maps §3): a new id is appended to the persisted save slice
// and the save marked dirty so autosave commits it; a repeat is a no-op.
// Caller holds a.mu. No-op when there is no save (ephemeral/test actors)
// or the id is empty.
func (a *connActor) markVisitedLocked(roomID string) {
	if a.save == nil || roomID == "" {
		return
	}
	a.ensureVisitedLocked()
	if _, seen := a.visited[roomID]; seen {
		return
	}
	a.visited[roomID] = struct{}{}
	a.save.VisitedRooms = append(a.save.VisitedRooms, roomID)
	a.markDirtyLocked()
}

// HasVisited reports whether the character has entered roomID — the
// fog-of-war gate the map renderers apply to the window query
// (player-maps §3). False for ephemeral actors with no save.
func (a *connActor) HasVisited(roomID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false // no save → no map; avoid allocating an index we never fill
	}
	a.ensureVisitedLocked()
	_, ok := a.visited[roomID]
	return ok
}

// VisitedRooms returns a snapshot of the character's visited-room ids
// (player-maps §3), in first-seen order. Nil for an actor with no save.
func (a *connActor) VisitedRooms() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return nil
	}
	return append([]string(nil), a.save.VisitedRooms...)
}

// MinimapEnabled reports the persisted active-minimap preference
// (player-maps §4). False for an actor with no save.
func (a *connActor) MinimapEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false
	}
	return a.save.MinimapEnabled
}

// SetMinimapEnabled stores the minimap preference and marks the save
// dirty so it persists. A no-op when unchanged or when there is no save.
func (a *connActor) SetMinimapEnabled(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil || a.save.MinimapEnabled == v {
		return
	}
	a.save.MinimapEnabled = v
	a.markDirtyLocked()
}

// ensureSeenAreasLocked lazily builds the in-memory seen-areas set from
// the persisted save the first time it is needed. Caller holds a.mu.
func (a *connActor) ensureSeenAreasLocked() {
	if a.seenAreas != nil {
		return
	}
	if a.save == nil {
		a.seenAreas = make(map[world.AreaID]struct{})
		return
	}
	a.seenAreas = make(map[world.AreaID]struct{}, len(a.save.SeenAreas))
	for _, id := range a.save.SeenAreas {
		a.seenAreas[world.AreaID(id)] = struct{}{}
	}
}

// HasSeenArea reports whether the character has ever entered the area
// (player-maps §4) — the gate for the once-ever first-entry banner.
// False for ephemeral actors with no save.
func (a *connActor) HasSeenArea(id world.AreaID) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false
	}
	a.ensureSeenAreasLocked()
	_, ok := a.seenAreas[id]
	return ok
}

// MarkAreaSeen records id in the character's seen-areas set: a new id is
// appended to the persisted save slice and the save marked dirty so
// autosave commits it; a repeat is a no-op. No-op when there is no save
// or the id is empty.
func (a *connActor) MarkAreaSeen(id world.AreaID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil || id == "" {
		return
	}
	a.ensureSeenAreasLocked()
	if _, seen := a.seenAreas[id]; seen {
		return
	}
	a.seenAreas[id] = struct{}{}
	a.save.SeenAreas = append(a.save.SeenAreas, string(id))
	a.markDirtyLocked()
}

// LastAreaSeen reports the area id of the room this actor was most
// recently shown (command.AreaTracker) — the "from" of the
// area-transition zone-line. Empty before the first render.
func (a *connActor) LastAreaSeen() world.AreaID {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastAreaSeen
}

// SetLastAreaSeen records the area the actor was just shown. In-memory
// session state — not persisted (unlike the minimap prefs), so it never
// marks the save dirty.
func (a *connActor) SetLastAreaSeen(id world.AreaID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastAreaSeen = id
}

// AreaTransition atomically records newArea as the actor's last-seen area
// and reports the area-transition decision the command layer needs to
// render a crossing (player-maps §4). It folds what were four separate,
// individually-locked accessor calls (LastAreaSeen → SetLastAreaSeen →
// HasSeenArea → MarkAreaSeen) into one a.mu critical section so the
// check-then-act cannot interleave with a concurrent render of the same
// actor — closing the window in which two goroutines could both observe
// "not yet seen" and double-fire the first-entry banner / double-append
// SeenAreas. Returns:
//   - prev: the area the actor was last shown (empty before the first
//     render — there is no "from" to narrate);
//   - changed: false for an intra-area render (a look or an intra-area
//     step), in which case nothing is mutated;
//   - firstEntry: true the first time the character ever enters newArea,
//     which (for a saved actor with a non-empty id) persists newArea in
//     the seen-area set and dirties the save as a side effect.
//
// Behavior matches the old four-call sequence exactly, including the
// no-save case (every crossing reads as a first entry because nothing
// persists) and the empty-id case (reported as a first entry but never
// persisted), so it is a faithful, lock-correct drop-in.
func (a *connActor) AreaTransition(newArea world.AreaID) (prev world.AreaID, changed, firstEntry bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	prev = a.lastAreaSeen
	if prev == newArea {
		return prev, false, false // same area: a look, or an intra-area step
	}
	a.lastAreaSeen = newArea

	// firstEntry mirrors !HasSeenArea (a save-less actor never persists, so
	// every crossing reads as not-yet-seen); persistence mirrors
	// MarkAreaSeen (guarded by save != nil and a non-empty id).
	if a.save == nil {
		return prev, true, true
	}
	a.ensureSeenAreasLocked()
	if _, seen := a.seenAreas[newArea]; seen {
		return prev, true, false
	}
	if newArea != "" {
		a.seenAreas[newArea] = struct{}{}
		a.save.SeenAreas = append(a.save.SeenAreas, string(newArea))
		a.markDirtyLocked()
	}
	return prev, true, true
}

// MinimapSize reports the persisted active-minimap size preset
// (player-maps §4): "auto"/"small"/"medium"/"large". Returns "auto"
// for an actor with no save or an unset preference, so the resolver
// scales the radius to the terminal by default.
func (a *connActor) MinimapSize() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil || a.save.MinimapSize == "" {
		return "auto"
	}
	return a.save.MinimapSize
}

// SetMinimapSize stores the minimap size preset and marks the save
// dirty so it persists. A no-op when there is no save or the effective
// value is unchanged. The empty stored value and "auto" are equivalent
// (both mean "scale to terminal"), so setting "auto" on a character
// that never set a size does not dirty the save — unlike the bool
// MinimapEnabled, the zero value ("") differs from the canonical
// default string ("auto"), so the comparison must normalize first.
func (a *connActor) SetMinimapSize(v string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return
	}
	// "" is the canonical on-disk form of "auto" (omitempty elides it),
	// so normalize the incoming value before storing: setting "auto"
	// always clears the field rather than writing the literal "auto".
	if v == "auto" {
		v = ""
	}
	if a.save.MinimapSize == v {
		return
	}
	a.save.MinimapSize = v
	a.markDirtyLocked()
}

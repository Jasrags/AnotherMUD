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
	stored := a.save.MinimapSize
	if stored == "" {
		stored = "auto"
	}
	if stored == v {
		return
	}
	a.save.MinimapSize = v
	a.markDirtyLocked()
}

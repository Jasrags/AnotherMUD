package session

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

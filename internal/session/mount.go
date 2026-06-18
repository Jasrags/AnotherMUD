package session

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// This file holds the connActor's mount-ownership surface (mounts.md §2.2,
// §10), satisfying the command package's mountOwner interface. Durable
// ownership is the save's Mounts list; the live-materialized overlay
// (liveMounts) is transient session state — which owned mounts currently have
// a creature in the world. Both are guarded by a.mu.

// OwnedMountTemplates returns the template ids of every mount this character
// owns (stabled or out in the world), in save order. Fresh slice.
func (a *connActor) OwnedMountTemplates() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil || len(a.save.Mounts) == 0 {
		return nil
	}
	out := make([]string, 0, len(a.save.Mounts))
	for _, m := range a.save.Mounts {
		out = append(out, m.TemplateID)
	}
	return out
}

// AddMount records durable ownership of a new mount (mounts.md §3.1 purchase /
// content grant) and marks the save dirty. The mount starts stabled — a record
// with no live creature until retrieved.
func (a *connActor) AddMount(templateID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return
	}
	a.save.Mounts = append(a.save.Mounts, player.MountRecord{TemplateID: templateID})
	a.markDirtyLocked()
}

// RemoveMount drops one ownership record matching templateID (a sale, a mount's
// death — mounts.md §9), marking the save dirty. Reports whether a record was
// removed. Removing ownership does NOT dematerialize a live creature; the
// caller dematerializes + UntrackLiveMount separately.
func (a *connActor) RemoveMount(templateID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false
	}
	for i, m := range a.save.Mounts {
		if m.TemplateID == templateID {
			a.save.Mounts = append(a.save.Mounts[:i], a.save.Mounts[i+1:]...)
			a.markDirtyLocked()
			return true
		}
	}
	return false
}

// TrackLiveMount records that an owned mount has been materialized into the
// world as the given entity (mounts.md §3.2 retrieve / §5.5). Transient — never
// persisted.
func (a *connActor) TrackLiveMount(id entities.EntityID, templateID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.liveMounts == nil {
		a.liveMounts = make(map[entities.EntityID]string)
	}
	a.liveMounts[id] = templateID
}

// UntrackLiveMount forgets a materialized mount (it was stabled, died, or the
// session is ending), returning its template id. Reports whether it was tracked.
func (a *connActor) UntrackLiveMount(id entities.EntityID) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.liveMounts[id]
	if ok {
		delete(a.liveMounts, id)
	}
	return t, ok
}

// LiveMountTemplates returns the template ids of this character's currently
// materialized mounts (a multiset — one entry per live creature). Fresh slice.
func (a *connActor) LiveMountTemplates() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.liveMounts) == 0 {
		return nil
	}
	out := make([]string, 0, len(a.liveMounts))
	for _, t := range a.liveMounts {
		out = append(out, t)
	}
	return out
}

// drainLiveMounts atomically snapshots AND clears the live-mount set, returning
// the entity ids to dematerialize. Used at logout to remove every live mount
// from the world (§9, §10). Snapshot-and-clear in ONE lock acquisition so a
// concurrent StableHandler (which dematerializes + UntrackLiveMount on the same
// actor) cannot race this into a double-remove: after the drain there is
// nothing left in liveMounts for the verb to act on.
func (a *connActor) drainLiveMounts() []entities.EntityID {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.liveMounts) == 0 {
		return nil
	}
	out := make([]entities.EntityID, 0, len(a.liveMounts))
	for id := range a.liveMounts {
		out = append(out, id)
	}
	a.liveMounts = nil
	return out
}

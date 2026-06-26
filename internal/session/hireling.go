package session

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// This file holds the connActor's hireling-ownership surface (hireable-mobs.md
// §2, §9), satisfying the command package's hirelingOwner interface. Durable
// ownership is the save's Hirelings list; the live-materialized overlay
// (liveHirelings) is transient session state — which owned hirelings currently
// have a creature in the world. Both are guarded by a.mu. This mirrors the mount
// surface (mount.go); a hireling fights where a mount carries.

// OwnedHirelingTemplates returns the template ids of every hireling this
// character owns, in save order. Fresh slice.
func (a *connActor) OwnedHirelingTemplates() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil || len(a.save.Hirelings) == 0 {
		return nil
	}
	out := make([]string, 0, len(a.save.Hirelings))
	for _, h := range a.save.Hirelings {
		out = append(out, h.TemplateID)
	}
	return out
}

// HirelingCount returns how many hire contracts this character holds (for the
// cap check, §3.3).
func (a *connActor) HirelingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return 0
	}
	return len(a.save.Hirelings)
}

// AddHireling records durable ownership of a new hire contract (hireable-mobs.md
// §3.1) and marks the save dirty.
func (a *connActor) AddHireling(templateID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return
	}
	a.save.Hirelings = append(a.save.Hirelings, player.HirelingRecord{TemplateID: templateID})
	a.markDirtyLocked()
}

// RemoveHireling drops one ownership record matching templateID (a dismiss, a
// hireling's death, an upkeep lapse — §7), marking the save dirty. Reports
// whether a record was removed. Removing ownership does NOT dematerialize a live
// creature; the caller dematerializes + UntrackLiveHireling separately.
func (a *connActor) RemoveHireling(templateID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return false
	}
	for i, h := range a.save.Hirelings {
		if h.TemplateID == templateID {
			a.save.Hirelings = append(a.save.Hirelings[:i], a.save.Hirelings[i+1:]...)
			a.markDirtyLocked()
			return true
		}
	}
	return false
}

// TrackLiveHireling records that an owned hireling has been materialized into the
// world as the given entity (hireable-mobs.md §3.1 hire / §9 login). Transient —
// never persisted.
func (a *connActor) TrackLiveHireling(id entities.EntityID, templateID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.liveHirelings == nil {
		a.liveHirelings = make(map[entities.EntityID]string)
	}
	a.liveHirelings[id] = templateID
}

// UntrackLiveHireling forgets a materialized hireling (it was dismissed, died, or
// the session is ending), returning its template id. Reports whether it was
// tracked.
func (a *connActor) UntrackLiveHireling(id entities.EntityID) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.liveHirelings[id]
	if ok {
		delete(a.liveHirelings, id)
	}
	return t, ok
}

// LiveHireling returns the entity id of a currently-materialized hireling whose
// template matches templateID, and whether one was found. Used by `dismiss` to
// resolve which live creature to remove for a named contract.
func (a *connActor) LiveHireling(templateID string) (entities.EntityID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, t := range a.liveHirelings {
		if t == templateID {
			return id, true
		}
	}
	return "", false
}

// drainLiveHirelings atomically snapshots AND clears the live-hireling set,
// returning the entity ids to dematerialize. Used at logout to remove every live
// hireling from the world (§4, §9). Snapshot-and-clear in ONE lock acquisition so
// a concurrent DismissHandler (which dematerializes + UntrackLiveHireling on the
// same actor) cannot race this into a double-remove.
func (a *connActor) drainLiveHirelings() []entities.EntityID {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.liveHirelings) == 0 {
		return nil
	}
	out := make([]entities.EntityID, 0, len(a.liveHirelings))
	for id := range a.liveHirelings {
		out = append(out, id)
	}
	a.liveHirelings = nil
	return out
}

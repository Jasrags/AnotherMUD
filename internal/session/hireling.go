package session

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
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

// liveHirelingTemplate returns the template id a live hireling entity belongs to,
// and whether this character owns it (used to identify a slain hireling, §6.2).
func (a *connActor) liveHirelingTemplate(id entities.EntityID) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.liveHirelings[id]
	return t, ok
}

// liveHirelingIDs snapshots the entity ids of this character's currently
// materialized hirelings (for the move-with-owner relocate, §5). Fresh slice.
func (a *connActor) liveHirelingIDs() []entities.EntityID {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.liveHirelings) == 0 {
		return nil
	}
	out := make([]entities.EntityID, 0, len(a.liveHirelings))
	for id := range a.liveHirelings {
		out = append(out, id)
	}
	return out
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

// rematerializeHirelings spawns the actor's owned hirelings into their room on
// login (hireable-mobs.md §9): each persisted contract gets a fresh live creature
// so the owner finds their help with them. A template no longer in content is
// skipped (fail-soft, like a stale mount record). No-op when the service is
// unwired or the actor has no hirelings.
func rematerializeHirelings(ctx context.Context, cfg Config, a *connActor) {
	if cfg.Hirelings == nil {
		return
	}
	room := a.Room()
	if room == nil {
		return
	}
	for _, templateID := range a.OwnedHirelingTemplates() {
		id, err := cfg.Hirelings.Materialize(ctx, a.PlayerID(), templateID, room.ID)
		if err != nil {
			logging.From(ctx).Warn("hireling re-materialize failed",
				slog.String("template", templateID), slog.Any("err", err))
			continue
		}
		a.TrackLiveHireling(id, templateID)
	}
}

// PullHirelings relocates the owner's live hirelings to follow them (hireable-mobs.md
// §5): when ownerID arrives in `to`, each of their materialized hirelings is moved
// to `to` so a hireling stays at its owner's side — it is BOUND to the owner
// (always co-located), distinct from a player follower that trails and can be left
// behind. The relocate is a placement move (the entity Placement is mutex-guarded);
// `from` is unused in v1 (the hireling is glued regardless of where it was). Runs
// on the mover's goroutine, like PullFollowers. No-op when the owner is offline or
// the placement isn't wired.
func (m *Manager) PullHirelings(ctx context.Context, ownerID string, from, to world.RoomID) {
	if m == nil {
		return
	}
	owner, ok := m.GetByPlayerID(ownerID)
	if !ok || owner == nil {
		return
	}
	place := m.actionEnv.Placement
	if place == nil {
		return
	}
	for _, id := range owner.liveHirelingIDs() {
		place.Place(id, to)
	}
}

// HirelingCombatantsOf returns the entity ids of ownerPID's live hirelings, for
// the combat-assist seam (hireable-mobs.md §6.1). Hirelings are bound to (always
// co-located with) their owner, so when the owner engages a foe in their room the
// hirelings are right there — no room filter is needed; the caller applies the
// in-combat guard. Empty when the owner is offline or has no live hireling.
func (m *Manager) HirelingCombatantsOf(ownerPID string) []string {
	if m == nil {
		return nil
	}
	owner, ok := m.GetByPlayerID(ownerPID)
	if !ok || owner == nil {
		return nil
	}
	ids := owner.liveHirelingIDs()
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}

// OnHirelingDeath ends the hire contract for a slain hireling (hireable-mobs.md
// §6.2): it finds the online owner whose live-hireling set holds entityID, drops
// the contract (untrack + remove the save record) and tells them. Reports whether
// entityID was an owned hireling. A live hireling implies an online owner (logout
// dematerializes them), so scanning the online roster is sufficient. The slain
// creature is removed from the world by the ordinary death/corpse path; this only
// tears down the OWNERSHIP. Called from the mob-killed reaction.
func (m *Manager) OnHirelingDeath(ctx context.Context, entityID entities.EntityID) bool {
	if m == nil {
		return false
	}
	owner, templateID, ok := m.ownerOfLiveHireling(entityID)
	if !ok {
		return false // not a hireling (or the owner went offline first)
	}
	owner.UntrackLiveHireling(entityID)
	owner.RemoveHireling(templateID)
	_ = owner.Write(ctx, "Your hireling falls in battle; the contract ends.")
	return true
}

// ownerOfLiveHireling finds the online actor whose live-hireling set holds
// entityID, returning them + the hireling's template id. Scans the online roster
// (mob deaths are infrequent and hirelings rare; a reverse index is a later
// optimization if this shows in a profile). Snapshots the actor set under m.mu,
// then probes each off-lock.
func (m *Manager) ownerOfLiveHireling(entityID entities.EntityID) (*connActor, string, bool) {
	m.mu.RLock()
	actors := make([]*connActor, 0, len(m.byPlayerID))
	for _, a := range m.byPlayerID {
		actors = append(actors, a)
	}
	m.mu.RUnlock()
	for _, a := range actors {
		if t, ok := a.liveHirelingTemplate(entityID); ok {
			return a, t, true
		}
	}
	return nil, "", false
}

// hirelingRef pairs a live hireling's entity id with its template (for the upkeep
// sweep, which needs both: the template to price upkeep, the id to dematerialize).
type hirelingRef struct {
	id       entities.EntityID
	template string
}

// liveHirelingPairs snapshots this character's live hirelings as (id, template)
// pairs (for the upkeep sweep, §7). Fresh slice.
func (a *connActor) liveHirelingPairs() []hirelingRef {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.liveHirelings) == 0 {
		return nil
	}
	out := make([]hirelingRef, 0, len(a.liveHirelings))
	for id, t := range a.liveHirelings {
		out = append(out, hirelingRef{id: id, template: t})
	}
	return out
}

// SweepHirelingUpkeep charges each online owner the recurring upkeep for their
// live hirelings (hireable-mobs.md §7) — the tick handler's body. A hireling whose
// upkeep its owner cannot pay DEPARTS: the contract ends and the creature leaves
// the world (lapsed-upkeep-departs, §7). upkeepOf returns the per-template upkeep
// cost (0 = free, skipped). No-op when the currency or hireling service is unwired.
func (m *Manager) SweepHirelingUpkeep(ctx context.Context, upkeepOf func(templateID string) int) {
	if m == nil || upkeepOf == nil {
		return
	}
	currency := m.actionEnv.Currency
	if currency == nil || m.hirelings == nil {
		return
	}
	m.mu.RLock()
	actors := make([]*connActor, 0, len(m.byPlayerID))
	for _, a := range m.byPlayerID {
		actors = append(actors, a)
	}
	m.mu.RUnlock()
	for _, a := range actors {
		for _, ref := range a.liveHirelingPairs() {
			amount := upkeepOf(ref.template)
			if amount <= 0 {
				continue
			}
			// The snapshot can go stale: OnHirelingDeath runs on the combat goroutine
			// and may end a contract between the snapshot and here. Skip a
			// no-longer-live hireling so we never charge for (or depart) a dead one.
			if _, live := a.liveHirelingTemplate(ref.id); !live {
				continue
			}
			if _, ok := currency.Debit(ctx, a, amount, "hireling-upkeep:"+ref.template); ok {
				continue
			}
			// Re-check after the debit (which dropped + re-took a.mu): if the hireling
			// died in that window, OnHirelingDeath already ended the contract — skip the
			// depart path to avoid a duplicate remove (which, with a same-template
			// sibling, could drop the WRONG record) and a contradictory message.
			if _, live := a.liveHirelingTemplate(ref.id); !live {
				continue
			}
			// Can't pay → the hireling departs (§7): drop the contract + the creature.
			m.hirelings.Dematerialize(ctx, ref.id)
			a.UntrackLiveHireling(ref.id)
			a.RemoveHireling(ref.template)
			_ = a.Write(ctx, "You can no longer pay your hireling; they shoulder their pack and depart.")
		}
	}
}

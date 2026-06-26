package session

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// liveHireling is the transient overlay value for a materialized hireling: the
// template it was hired from plus its current order stance (hireable-mobs.md §8).
// Both live only while the creature is in the world; logout/death drops the entry.
type liveHireling struct {
	template string
	stance   string // one of command.HirelingStance*; "" treated as follow
}

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
		a.liveHirelings = make(map[entities.EntityID]liveHireling)
	}
	// A freshly materialized hireling defaults to follow (hireable-mobs.md §8);
	// stance is transient, so re-materialize (login) always resets it.
	a.liveHirelings[id] = liveHireling{template: templateID, stance: command.HirelingStanceFollow}
}

// SetHirelingStance updates a live hireling's order stance (hireable-mobs.md §8).
// No-op when the id isn't currently materialized for this character.
func (a *connActor) SetHirelingStance(id entities.EntityID, stance string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	h, ok := a.liveHirelings[id]
	if !ok {
		return
	}
	h.stance = stance
	a.liveHirelings[id] = h
}

// UntrackLiveHireling forgets a materialized hireling (it was dismissed, died, or
// the session is ending), returning its template id. Reports whether it was
// tracked.
func (a *connActor) UntrackLiveHireling(id entities.EntityID) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	h, ok := a.liveHirelings[id]
	if ok {
		delete(a.liveHirelings, id)
	}
	return h.template, ok
}

// LiveHireling returns the entity id of a currently-materialized hireling whose
// template matches templateID, and whether one was found. Used by `dismiss` to
// resolve which live creature to remove for a named contract.
func (a *connActor) LiveHireling(templateID string) (entities.EntityID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, h := range a.liveHirelings {
		if h.template == templateID {
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
	h, ok := a.liveHirelings[id]
	return h.template, ok
}

// hirelingStanceRef pairs a live hireling's entity id with its order stance,
// for the stance-aware relocate (§5) and assist (§6.1) reads.
type hirelingStanceRef struct {
	id     entities.EntityID
	stance string
}

// LiveHirelings returns this character's materialized hirelings in a STABLE order
// (by the numeric "entity-N" mint order), satisfying command.hirelingOwner. Entity
// ids are minted monotonically, so this is hire order, and a 1-based index over it
// is a durable per-session targeting handle (hireable-mobs.md §3.3 — how the verbs
// address same-template duplicates). Sorting on the NUMERIC suffix matters: a plain
// string sort would put "entity-10" before "entity-9". Fresh slice.
func (a *connActor) LiveHirelings() []command.LiveHirelingRef {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.liveHirelings) == 0 {
		return nil
	}
	out := make([]command.LiveHirelingRef, 0, len(a.liveHirelings))
	for id, h := range a.liveHirelings {
		out = append(out, command.LiveHirelingRef{ID: id, TemplateID: h.template})
	}
	sort.Slice(out, func(i, j int) bool { return entityIDLess(out[i].ID, out[j].ID) })
	return out
}

// entityIDLess orders two entity ids by their numeric "entity-N" suffix (the mint
// order), falling back to a plain string compare for any id that doesn't carry a
// numeric suffix. A bare string sort misorders across digit-length boundaries
// ("entity-9" vs "entity-10"), which would shuffle the hireling roster numbers.
func entityIDLess(a, b entities.EntityID) bool {
	na, oka := entityIDNum(a)
	nb, okb := entityIDNum(b)
	if oka && okb {
		return na < nb
	}
	return a < b
}

// entityIDNum extracts the trailing numeric suffix of an "...-N" entity id.
func entityIDNum(id entities.EntityID) (uint64, bool) {
	s := string(id)
	i := strings.LastIndexByte(s, '-')
	if i < 0 {
		return 0, false
	}
	n, err := strconv.ParseUint(s[i+1:], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// liveHirelingStances snapshots this character's live hirelings as (id, stance)
// pairs. An empty stance is normalized to follow (the default). Fresh slice.
func (a *connActor) liveHirelingStances() []hirelingStanceRef {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.liveHirelings) == 0 {
		return nil
	}
	out := make([]hirelingStanceRef, 0, len(a.liveHirelings))
	for id, h := range a.liveHirelings {
		st := h.stance
		if st == "" {
			st = command.HirelingStanceFollow
		}
		out = append(out, hirelingStanceRef{id: id, stance: st})
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
// behind. The relocate is a placement move (the entity Placement is mutex-guarded),
// and bystanders in the `from`/`to` rooms see the hireling leave and arrive (the
// move handler already broadcast the owner's own lines before publishing the move).
// Runs on the mover's goroutine, like PullFollowers. No-op when the owner is offline
// or the placement isn't wired.
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
	cm := m.actionEnv.Combat
	dir, adjacent := m.directionBetween(from, to)
	ownerName := owner.Name()
	heldByCombat := 0
	// Only a follow-stance hireling trails (hireable-mobs.md §8); a stay/guard
	// hireling holds the room it was left in.
	for _, ref := range owner.liveHirelingStances() {
		if ref.stance != command.HirelingStanceFollow {
			continue
		}
		// Don't yank a hireling out of its own fight (hireable-mobs.md §5/§6): a
		// follow hireling that is mid-combat holds its ground rather than being
		// teleported away mid-round. It rejoins on the owner's next move once the
		// fight is over (the bind is re-evaluated each move).
		if cm != nil && cm.InCombat(combat.NewMobCombatantID(string(ref.id))) {
			heldByCombat++
			continue
		}
		name := m.mobName(ref.id)
		m.announceHirelingDeparture(ctx, name, ownerName, from, dir, adjacent)
		place.Place(ref.id, to)
		m.announceHirelingArrival(ctx, name, ownerName, ownerID, to)
	}
	switch {
	case heldByCombat == 1:
		_ = owner.Write(ctx, "Your hireling stays behind, locked in combat.")
	case heldByCombat > 1:
		_ = owner.Write(ctx, "Your hirelings stay behind, locked in combat.")
	}
}

// announceHirelingDeparture broadcasts a bound hireling leaving `from` to trail
// its owner (hireable-mobs.md §5). The phrasing names the owner so the hireling
// reads as a follower, not a wanderer; a non-adjacent owner move (recall/teleport)
// drops the direction. No exclusion: the owner has already left `from`, so the
// line only reaches the bystanders left behind. A no-name hireling (store drift)
// or unwired Broadcaster suppresses it.
func (m *Manager) announceHirelingDeparture(ctx context.Context, name, ownerName string, from world.RoomID, dir world.Direction, adjacent bool) {
	b := m.actionEnv.Broadcaster
	if b == nil || name == "" {
		return
	}
	if adjacent {
		b.SendToRoom(ctx, from, name+" follows "+ownerName+" "+dir.Long()+".")
	} else {
		b.SendToRoom(ctx, from, name+" slips away, following "+ownerName+".")
	}
}

// announceHirelingArrival broadcasts a bound hireling arriving in `to`, trailing
// its owner (hireable-mobs.md §5). Fired AFTER the placement move so a bystander
// who looks finds the hireling present. The owner is excluded — they're in `to`
// and don't need to be told their own help arrived on every step.
func (m *Manager) announceHirelingArrival(ctx context.Context, name, ownerName, ownerID string, to world.RoomID) {
	b := m.actionEnv.Broadcaster
	if b == nil || name == "" {
		return
	}
	b.SendToRoom(ctx, to, name+" arrives, following "+ownerName+".", ownerID)
}

// mobName resolves a live mob's display name from the entity store, or "" when the
// id no longer resolves to a mob (the caller suppresses output on empty).
func (m *Manager) mobName(id entities.EntityID) string {
	if store := m.actionEnv.Items; store != nil {
		if e, ok := store.GetByID(id); ok {
			if mob, ok := e.(*entities.MobInstance); ok {
				return mob.Name()
			}
		}
	}
	return ""
}

// HirelingCombatantsOf returns the entity ids of ownerPID's live hirelings that
// should join combat happening in `room` (hireable-mobs.md §6.1, §8). A stay
// hireling never assists; a follow or guard hireling assists only when it is
// actually in the combat room. For a follow hireling that is the owner's room (it
// is bound there); for a guard hireling that is the room it was left holding —
// "guard still assists if combat reaches the room". The caller applies the
// in-combat guard. Empty when the owner is offline or no hireling qualifies.
func (m *Manager) HirelingCombatantsOf(ownerPID string, room world.RoomID) []string {
	if m == nil {
		return nil
	}
	owner, ok := m.GetByPlayerID(ownerPID)
	if !ok || owner == nil {
		return nil
	}
	refs := owner.liveHirelingStances()
	if len(refs) == 0 {
		return nil
	}
	place := m.actionEnv.Placement
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.stance == command.HirelingStanceStay {
			continue // stood down — never assists
		}
		// Room gate: a hireling only assists combat in its own room. Skipped when
		// placement isn't wired (test paths) so unit tests need not stage rooms.
		if place != nil {
			if r, ok := place.RoomOf(ref.id); !ok || r != room {
				continue
			}
		}
		out = append(out, string(ref.id))
	}
	if len(out) == 0 {
		return nil
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
	for id, h := range a.liveHirelings {
		out = append(out, hirelingRef{id: id, template: h.template})
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

package session

import (
	"context"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// This file holds the onboarding-guide lifecycle (onboarding-guide.md): the
// connActor's single live-guide overlay, and the Manager methods that spawn it on
// world-entry, trail it on move, retire it at the graduation level, and (in
// manager.go) drain it on logout. A guide is a simplified hireling — one per
// character, never persisted, never fights — so there is no durable save half and
// no stance/upkeep/combat machinery; just a bound trailing NPC that teaches by
// presence and leaves when the character has found their feet.

// SetLiveGuide records this character's materialized guide entity.
func (a *connActor) SetLiveGuide(id entities.EntityID) {
	a.mu.Lock()
	a.liveGuide = id
	a.mu.Unlock()
}

// HasLiveGuide reports whether a guide is currently materialized for this
// character (the spawn gate's dedup — never grant a second).
func (a *connActor) HasLiveGuide() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.liveGuide != ""
}

// LiveGuideID returns the character's live guide entity id without clearing it
// (the trail relocate reads it every move). ok is false when there is no guide.
func (a *connActor) LiveGuideID() (entities.EntityID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.liveGuide, a.liveGuide != ""
}

// DrainLiveGuide reads-and-clears the live guide in one lock acquisition,
// returning its entity id. The atomic read-clear makes the three teardown paths
// (shoo, graduation, logout drain) mutually race-safe — only the caller that
// observes a non-empty id dematerializes, so the creature is never double-removed.
func (a *connActor) DrainLiveGuide() (entities.EntityID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := a.liveGuide
	a.liveGuide = ""
	return id, id != ""
}

// SpawnGuideFor materializes an onboarding guide at the character's side when they
// enter the world (creation + login), provided the feature is configured and the
// character is below the graduation level and has no guide yet (onboarding-guide.md
// §Materialization). Called from the session enter-world path with the just-placed
// actor. No-op (fail-soft) on any gate miss or a spawn error.
func (m *Manager) SpawnGuideFor(ctx context.Context, a *connActor) {
	if m == nil || a == nil {
		return
	}
	m.mu.RLock()
	svc := m.guides
	template := m.guideTemplate
	levelCap := m.guideLevelCap
	m.mu.RUnlock()
	if svc == nil || template == "" {
		return // feature off for this world
	}
	if a.HasLiveGuide() {
		return // already has one (defensive — enter-world runs once)
	}
	if a.characterLevel() >= levelCap {
		return // already graduated — no guide
	}
	room := a.Room()
	if room == nil {
		return
	}
	id, err := svc.Materialize(ctx, a.PlayerID(), template, room.ID)
	if err != nil {
		logging.From(ctx).Warn("guide materialize failed",
			slog.String("template", template), slog.Any("err", err))
		return
	}
	a.SetLiveGuide(id)
	name := m.guideName(id)
	_ = a.Write(ctx, name+" falls into step beside you, ready to show you the ropes. (`shoo` to send them off.)")
	m.SendToRoom(ctx, room.ID, name+" falls into step beside "+a.Name()+".", a.PlayerID())
}

// PullGuide relocates the owner's live guide to their new room so it stays at their
// side (onboarding-guide.md §Trailing) — the hireling bound-follow model, minus the
// combat/stance gates (a guide never fights, so it is never held mid-round). Runs
// on the mover's goroutine, like PullHirelings. No-op when the owner is offline or
// has no live guide.
func (m *Manager) PullGuide(ctx context.Context, ownerID string, from, to world.RoomID) {
	if m == nil {
		return
	}
	owner, ok := m.GetByPlayerID(ownerID)
	if !ok || owner == nil {
		return
	}
	id, ok := owner.LiveGuideID()
	if !ok {
		return
	}
	place := m.actionEnv.Placement
	if place == nil {
		return
	}
	// Self-heal a dead guide: if the entity no longer resolves in the store (a
	// guide that trailed its owner into a fight and was killed — the v1 open
	// question, onboarding-guide.md §Open), clear the stale overlay and stop
	// trailing rather than re-inserting a phantom placement entry for a gone id.
	name := m.mobName(id)
	if name == "" {
		owner.DrainLiveGuide()
		return
	}
	dir, adjacent := m.directionBetween(from, to)
	ownerName := owner.Name()
	if adjacent {
		m.SendToRoom(ctx, from, name+" follows "+ownerName+" "+dir.Long()+".")
	} else {
		m.SendToRoom(ctx, from, name+" slips away, following "+ownerName+".")
	}
	place.Place(id, to)
	m.SendToRoom(ctx, to, name+" arrives, following "+ownerName+".", ownerID)
}

// GraduateGuide retires the character's live guide when a level-up reaches the
// graduation level (onboarding-guide.md §Graduation) — called from the level-up
// reaction with the level-up's NEW level. Gating on the event's newLevel (rather
// than re-reading characterLevel()) uses the authoritative, already-applied value
// the event carries, and matches the feat-credit hook beside it. For a single-
// class character newLevel == character level; the multiclass total is a future
// refinement (same caveat as the feat-slot credit). Below-cap level-ups are a
// no-op; so is a world with no cap or no live guide.
func (m *Manager) GraduateGuide(ctx context.Context, playerID string, newLevel int) {
	if m == nil {
		return
	}
	m.mu.RLock()
	svc := m.guides
	levelCap := m.guideLevelCap
	m.mu.RUnlock()
	if svc == nil || levelCap <= 0 || newLevel < levelCap {
		return // feature off, or this level-up hasn't reached the graduation level
	}
	owner, ok := m.GetByPlayerID(playerID)
	if !ok || owner == nil {
		return
	}
	id, ok := owner.DrainLiveGuide()
	if !ok {
		return // no live guide (already gone / shooed)
	}
	name := m.guideName(id)
	room := owner.Room()
	// Only farewell a guide that was actually present: a guide already gone from
	// the world (died in combat) leaves nothing to say goodbye to.
	if !svc.Dematerialize(ctx, id) {
		return
	}
	_ = owner.Write(ctx, name+" claps you on the shoulder — \"You've got the hang of it now. Watch yourself out there.\" — and melts into the crowd.")
	if room != nil {
		m.SendToRoom(ctx, room.ID, name+" slips away into the sprawl.", playerID)
	}
}

// guideName resolves a live guide's display name from the store, falling back to a
// generic noun when the id no longer resolves (store drift). Never returns "" so
// the lifecycle lines always read.
func (m *Manager) guideName(id entities.EntityID) string {
	if n := m.mobName(id); n != "" {
		return n
	}
	return "your guide"
}

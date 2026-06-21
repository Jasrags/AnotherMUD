// Package action is the engine's generic per-actor timed-action / busy-state
// substrate (action-economy.md). It records at most one in-flight occupation per
// actor — "busy doing Kind until tick ReadyAt" — and provides the begin /
// query / complete / interrupt operations the command layer and the completion
// sweep drive. It is a leaf: it imports nothing from combat, items, session, or
// any consumer, so reload / don / future action-economy features ride it without
// a dependency cycle.
//
// The state is transient and never persisted (action-economy.md §6): an in-flight
// action at logout / crash is simply dropped, and because consumers reserve
// nothing at begin (the lazy-completion model, mirroring crafting's PendingCraft),
// nothing is lost with it. The shape unions the two existing occupation trackers —
// tick-stamped completion like crafting.PendingCraft, central per-entity storage +
// optional interruption like progression.CastTracker.
package action

import (
	"strings"
	"sync"
)

// Kind is an opaque, consumer-owned tag the completion sweep routes on. Each
// consumer (the crossbow reload, the armor don) declares its own kind; the
// substrate never interprets it.
type Kind string

// Action is the small transient record of one actor's in-flight occupation.
// Payload is opaque consumer data captured at begin (e.g. which item, which
// slot) so completion need not recompute world-coupled state off the tick
// goroutine — mirroring how PendingCraft captures StationTier.
type Action struct {
	// Kind routes the completed action to its consumer (action-economy.md §3).
	Kind Kind
	// ReadyAt is the engine tick the action completes on (begin tick + duration).
	ReadyAt uint64
	// Interruptible reports whether the movement / manual-cancel path aborts this
	// action. A forced Drop (logout, death) clears it regardless (§5).
	Interruptible bool
	// Label is the human phrase for the "You are busy <label>." refusal and the
	// completion / interrupt notices (e.g. "reloading").
	Label string
	// Payload is opaque consumer data for the completion router. The substrate
	// stores and returns it untouched.
	Payload any
}

// Tracker records each entity's single in-flight timed action. Process-wide;
// per-entity state lives in one id-keyed map guarded by a mutex, mirroring
// progression.CastTracker. An entity has at most one action in flight — Begin
// refuses a second rather than overwriting.
type Tracker struct {
	mu     sync.RWMutex
	active map[string]Action // entityID -> in-flight action
}

// NewTracker returns an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{active: make(map[string]Action)}
}

func normID(entityID string) string {
	return strings.ToLower(strings.TrimSpace(entityID))
}

// Begin records a as entityID's in-flight action and returns true. It returns
// false (recording nothing) when an action is already in flight: the caller is
// expected to gate on IsBusy first, but the refusal is the safe degenerate
// (mirrors crafting SetPendingCraft returning false). A blank entityID is a
// no-op returning false.
func (t *Tracker) Begin(entityID string, a Action) bool {
	eid := normID(entityID)
	if eid == "" || t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, busy := t.active[eid]; busy {
		return false
	}
	t.active[eid] = a
	return true
}

// Active returns entityID's in-flight action without mutating it. The second
// return is false when the entity is idle.
func (t *Tracker) Active(entityID string) (Action, bool) {
	eid := normID(entityID)
	if eid == "" || t == nil {
		return Action{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	a, ok := t.active[eid]
	return a, ok
}

// IsBusy reports whether entityID has a timed action in flight — the dispatch
// gate (action-economy.md §4).
func (t *Tracker) IsBusy(entityID string) bool {
	_, ok := t.Active(entityID)
	return ok
}

// CompleteReady claims entityID's in-flight action if it has come due (now >=
// ReadyAt), returning it for routing and true. It clears the action before
// returning (single-winner: a second caller gets false), so a failed completion
// never loops — mirroring crafting CompleteReady. Returns (_, false) when the
// entity is idle or its action is not yet due.
func (t *Tracker) CompleteReady(entityID string, now uint64) (Action, bool) {
	eid := normID(entityID)
	if eid == "" || t == nil {
		return Action{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.active[eid]
	if !ok || now < a.ReadyAt {
		return Action{}, false
	}
	delete(t.active, eid)
	return a, true
}

// Interrupt clears entityID's in-flight action and returns it ONLY when one is
// in flight and it is Interruptible — the movement / manual-cancel path (§5). It
// returns (_, false) both when the entity is idle and when its action is
// non-interruptible (the caller distinguishes via Active for the "can't be
// stopped" message). Use Drop to clear regardless of the flag.
func (t *Tracker) Interrupt(entityID string) (Action, bool) {
	eid := normID(entityID)
	if eid == "" || t == nil {
		return Action{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.active[eid]
	if !ok || !a.Interruptible {
		return Action{}, false
	}
	delete(t.active, eid)
	return a, true
}

// Drop unconditionally removes entityID's in-flight action and returns it,
// reporting whether one was present — the forced-abort path (logout, death) that
// ignores the Interruptible flag (§5). No-op returning false when idle.
func (t *Tracker) Drop(entityID string) (Action, bool) {
	eid := normID(entityID)
	if eid == "" || t == nil {
		return Action{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.active[eid]
	if ok {
		delete(t.active, eid)
	}
	return a, ok
}

// BusyEntities returns the ids of every entity with an action in flight, so the
// completion sweep iterates only the occupied (action-economy.md §3). Order is
// unspecified; returns nil when nobody is busy.
func (t *Tracker) BusyEntities() []string {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.active) == 0 {
		return nil
	}
	ids := make([]string, 0, len(t.active))
	for id := range t.active {
		ids = append(ids, id)
	}
	return ids
}

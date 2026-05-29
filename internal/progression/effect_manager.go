package progression

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/srckey"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// EffectTarget is the per-entity surface the EffectManager needs
// to apply stat modifiers and identify the target. Players
// (session.connActor) and, in M9.4, mobs (entities.MobInstance
// once the M8.1 StatBlock wiring lands) both implement this. The
// interface stays small (1-3 methods per project style) so test
// fakes are cheap.
//
// All methods MUST be safe for concurrent access; the
// EffectManager holds no per-entity locks of its own beyond its
// own map mutex.
type EffectTarget interface {
	// EntityID returns the stable id under which the manager
	// keys this target. For players this is loaded.Player.ID;
	// for mobs (M9.4) it will be the MobInstance id.
	EntityID() string
	// AddModifiers installs mods under src on the target's
	// stat block. Mirrors progression.StatBlock.AddModifiers —
	// the effect manager calls with src = EffectSourceKey(id)
	// so removal can later target the exact set.
	AddModifiers(src StatModifierSource, mods []stats.Modifier)
	// RemoveBySource drops the modifier set under src; reports
	// whether anything was removed.
	RemoveBySource(src StatModifierSource) bool
}

// StatModifierSource is the source-key alias the EffectTarget surface
// uses — a progression-package name for srckey.SourceKey. Using the
// leaf type (rather than entities.SourceKey) is what lets entities
// import progression without a cycle; it is an alias rather than a
// fresh type so casts to/from srckey.SourceKey (and the
// entities.SourceKey alias) are zero-cost. Concrete value is the same
// key EffectSourceKey produces.
type StatModifierSource = srckey.SourceKey

// EffectSink is the optional event-emission seam the
// EffectManager uses when applied/removed/expired transitions
// occur (spec abilities-and-effects §7). Implementations adapt to
// the eventbus.Bus in production; tests use a recording fake.
// nil-safe: a manager constructed without a sink runs silently.
type EffectSink interface {
	EffectApplied(ctx context.Context, ev EffectAppliedEvent)
	EffectRemoved(ctx context.Context, ev EffectRemovedEvent)
	EffectExpired(ctx context.Context, ev EffectExpiredEvent)
}

// EffectAppliedEvent is the payload published on a successful
// Apply (spec §7 "effect applied"). Mirrors the shape of the
// eventbus.EffectApplied event so a bus-bridging sink is a
// straight pass-through.
type EffectAppliedEvent struct {
	EntityID        string
	EffectID        string
	SourceAbilityID string
	Duration        int
}

// EffectRemovedEvent is the payload published on a successful
// non-expiration removal (RemoveByID, RemoveByFlag, external
// dispel). Spec §7.
type EffectRemovedEvent struct {
	EntityID        string
	EffectID        string
	SourceAbilityID string
}

// EffectExpiredEvent is the payload published when Tick's batch
// expiration sweeps a zero-counter effect. Spec §7.
type EffectExpiredEvent struct {
	EntityID        string
	EffectID        string
	SourceAbilityID string
}

// EffectManager tracks active effects per entity and owns the
// per-target effect-flag index (spec abilities-and-effects §5).
// The manager is process-wide; per-entity state lives in two
// id-keyed maps guarded by a single mutex.
//
// Targets are looked up via a TargetResolver supplied at
// construction so the manager doesn't depend on the session layer
// directly. Apply/Remove/Tick fail soft when the resolver returns
// (nil, false) — the entity is gone (logged out, despawned) and
// the manager simply drops its state.
type EffectManager struct {
	resolver TargetResolver
	sink     EffectSink

	mu      sync.RWMutex
	effects map[string][]*Effect // entityID -> active effects (insertion order)
}

// TargetResolver maps an entity id to its live EffectTarget. The
// resolver is the seam between the manager (which keys on stable
// ids) and the session layer (which owns target instances by
// connection). nil-safe: a manager with no resolver returns
// false from every lookup, which collapses Apply/Remove/Tick to
// metadata-only operations (useful in tests that exercise the
// active-list bookkeeping without a real stat block).
type TargetResolver interface {
	ResolveTarget(entityID string) (EffectTarget, bool)
}

// TargetResolverFunc lets callers pass a closure where a
// TargetResolver is expected. Mirrors http.HandlerFunc shape.
type TargetResolverFunc func(entityID string) (EffectTarget, bool)

// ResolveTarget implements TargetResolver.
func (f TargetResolverFunc) ResolveTarget(entityID string) (EffectTarget, bool) {
	return f(entityID)
}

// NewEffectManager returns a manager bound to resolver and sink.
// Both may be nil — see TargetResolver / EffectSink documentation
// for nil-safe semantics.
func NewEffectManager(resolver TargetResolver, sink EffectSink) *EffectManager {
	return &EffectManager{
		resolver: resolver,
		sink:     sink,
		effects:  make(map[string][]*Effect),
	}
}

// Apply installs an effect built from tpl on entityID, with the
// supplied source attribution. Returns true on success, false
// when the single-instance rule refuses the application (spec
// §5.2 step 2: a target already carrying an effect with the same
// id rejects the new one cleanly — no event, no mutation).
//
// Empty tpl.ID is rejected as a no-op (returns false). The
// supplied sourceEntityID/sourceAbilityID may be empty when the
// effect lacks an explicit source (admin grant, world hook).
//
// Successful application:
//  1. Resolves the target via TargetResolver. If the target is
//     gone (resolver returns false), the manager records the
//     effect in its active-list so Tick / RemoveByID still see
//     it but no stat modifiers are written. This matches the
//     spec's "ephemeral effect-list state" model — the stat
//     block has already been persisted by the equipment path,
//     and a target that comes back online will rehydrate via
//     Restore (M9.4-era extension).
//  2. Writes stat modifiers under EffectSourceKey(tpl.ID).
//  3. Appends the runtime Effect to the entity's active list.
//  4. Emits EffectApplied via the sink.
func (m *EffectManager) Apply(ctx context.Context, entityID string, tpl EffectTemplate, sourceEntityID, sourceAbilityID string) bool {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	id := strings.ToLower(strings.TrimSpace(tpl.ID))
	if eid == "" || id == "" {
		return false
	}

	m.mu.Lock()
	if m.hasLocked(eid, id) {
		m.mu.Unlock()
		return false
	}
	eff := newEffectFromTemplate(tpl, eid, sourceEntityID, sourceAbilityID)
	m.effects[eid] = append(m.effects[eid], eff)
	m.mu.Unlock()

	// Stat modifiers go through the target outside the manager
	// lock so the target's own mutex never nests inside ours.
	if m.resolver != nil && len(eff.Modifiers) > 0 {
		if target, ok := m.resolver.ResolveTarget(eid); ok {
			target.AddModifiers(EffectSourceKey(id), eff.Modifiers)
		}
	}

	if m.sink != nil {
		m.sink.EffectApplied(ctx, EffectAppliedEvent{
			EntityID:        eid,
			EffectID:        id,
			SourceAbilityID: eff.SourceAbilityID,
			Duration:        eff.Remaining,
		})
	}
	return true
}

// Has reports whether entityID currently carries an active
// effect with the supplied id. Spec §5.6 first query.
func (m *EffectManager) Has(entityID, effectID string) bool {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	id := strings.ToLower(strings.TrimSpace(effectID))
	if eid == "" || id == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hasLocked(eid, id)
}

// hasLocked checks the per-entity list for an effect with id.
// Caller MUST hold m.mu (R or W).
func (m *EffectManager) hasLocked(entityID, effectID string) bool {
	for _, e := range m.effects[entityID] {
		if e.ID == effectID {
			return true
		}
	}
	return false
}

// Effects returns a snapshot of every active effect on entityID,
// sorted by id (deterministic order for tests + renderers). The
// returned slice + each Effect value are fresh copies; mutation
// does not affect manager state. Spec §5.6 second query.
func (m *EffectManager) Effects(entityID string) []Effect {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	src := m.effects[eid]
	if len(src) == 0 {
		return nil
	}
	out := make([]Effect, 0, len(src))
	for _, e := range src {
		out = append(out, copyEffect(e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// copyEffect returns a deep copy of e safe for caller mutation.
func copyEffect(e *Effect) Effect {
	out := *e
	if len(e.Modifiers) > 0 {
		out.Modifiers = make([]stats.Modifier, len(e.Modifiers))
		copy(out.Modifiers, e.Modifiers)
	}
	if len(e.Flags) > 0 {
		out.Flags = make([]string, len(e.Flags))
		copy(out.Flags, e.Flags)
	}
	return out
}

// Flags returns a sorted snapshot of every distinct flag carried
// by entityID's active effects. Used by passive matchers (M9.5)
// and by tests asserting flag-driven removals.
func (m *EffectManager) Flags(entityID string) []string {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := make(map[string]struct{})
	for _, e := range m.effects[eid] {
		for _, f := range e.Flags {
			seen[f] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// HasFlag reports whether any active effect on entityID carries
// flag. Case-insensitive on input. Convenience wrapper around
// Flags-iteration for hot-path passive checks where allocating a
// snapshot slice would be wasteful.
func (m *EffectManager) HasFlag(entityID, flag string) bool {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	target := strings.ToLower(strings.TrimSpace(flag))
	if eid == "" || target == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.effects[eid] {
		for _, f := range e.Flags {
			if f == target {
				return true
			}
		}
	}
	return false
}

// RemoveByID reverses the effect with id on entityID: removes its
// stat modifiers, drops the entry, emits EffectRemoved. Returns
// true if an effect was removed, false if no matching id was
// found (silent no-op per spec §5.3 "Removal by unknown id ... is
// a silent no-op"). Case-insensitive on ids.
func (m *EffectManager) RemoveByID(ctx context.Context, entityID, effectID string) bool {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	id := strings.ToLower(strings.TrimSpace(effectID))
	if eid == "" || id == "" {
		return false
	}
	removed := m.removeMatching(eid, func(e *Effect) bool { return e.ID == id })
	if len(removed) == 0 {
		return false
	}
	m.reverseAndEmit(ctx, eid, removed, false)
	return true
}

// RemoveByFlag removes EVERY active effect on entityID whose flag
// list contains flag (spec §5.3 batch behavior). Returns the
// count of effects removed. Case-insensitive on flag.
func (m *EffectManager) RemoveByFlag(ctx context.Context, entityID, flag string) int {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	target := strings.ToLower(strings.TrimSpace(flag))
	if eid == "" || target == "" {
		return 0
	}
	removed := m.removeMatching(eid, func(e *Effect) bool {
		for _, f := range e.Flags {
			if f == target {
				return true
			}
		}
		return false
	})
	if len(removed) == 0 {
		return 0
	}
	m.reverseAndEmit(ctx, eid, removed, false)
	return len(removed)
}

// removeMatching extracts every entry in entityID's active list
// for which pred returns true, in stable order. Returns the
// removed entries so the caller can reverse stat mods and emit
// events outside the manager lock (avoids nesting target locks
// inside ours).
func (m *EffectManager) removeMatching(entityID string, pred func(*Effect) bool) []*Effect {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.effects[entityID]
	if len(list) == 0 {
		return nil
	}
	kept := list[:0]
	var removed []*Effect
	for _, e := range list {
		if pred(e) {
			removed = append(removed, e)
			continue
		}
		kept = append(kept, e)
	}
	if len(removed) == 0 {
		return nil
	}
	if len(kept) == 0 {
		delete(m.effects, entityID)
	} else {
		m.effects[entityID] = kept
	}
	return removed
}

// reverseAndEmit reverses stat modifiers for each removed effect
// and publishes the appropriate event. `expired` selects between
// the EffectExpired (true) and EffectRemoved (false) sink hooks
// so a single helper serves both removal paths.
func (m *EffectManager) reverseAndEmit(ctx context.Context, entityID string, removed []*Effect, expired bool) {
	target, hasTarget := (EffectTarget)(nil), false
	if m.resolver != nil {
		target, hasTarget = m.resolver.ResolveTarget(entityID)
	}
	for _, e := range removed {
		if hasTarget && len(e.Modifiers) > 0 {
			target.RemoveBySource(EffectSourceKey(e.ID))
		}
		if m.sink == nil {
			continue
		}
		if expired {
			m.sink.EffectExpired(ctx, EffectExpiredEvent{
				EntityID:        entityID,
				EffectID:        e.ID,
				SourceAbilityID: e.SourceAbilityID,
			})
		} else {
			m.sink.EffectRemoved(ctx, EffectRemovedEvent{
				EntityID:        entityID,
				EffectID:        e.ID,
				SourceAbilityID: e.SourceAbilityID,
			})
		}
	}
}

// Tick decrements every non-permanent effect's remaining counter
// by one and batches expirations (spec §5.4). Iteration is
// snapshot-style — the per-entity slices are walked under the
// manager lock to identify expirations, but the actual removal +
// stat reversal + event emission happens after the lock is
// released so the active list is never mutated mid-iteration
// (spec §5.4 last paragraph).
//
// Permanent effects (Remaining < 0) are skipped entirely.
func (m *EffectManager) Tick(ctx context.Context) {
	type pending struct {
		entityID string
		removed  []*Effect
	}
	var batch []pending

	m.mu.Lock()
	for eid, list := range m.effects {
		var expired []*Effect
		kept := list[:0]
		for _, e := range list {
			if e.IsPermanent() {
				kept = append(kept, e)
				continue
			}
			e.Remaining--
			if e.Remaining <= 0 {
				expired = append(expired, e)
				continue
			}
			kept = append(kept, e)
		}
		if len(expired) == 0 {
			// Mutated slice in place; either keep as-is or
			// re-assign for clarity. The decrement above
			// already mutated *Effect entries in `kept`.
			continue
		}
		if len(kept) == 0 {
			delete(m.effects, eid)
		} else {
			m.effects[eid] = kept
		}
		batch = append(batch, pending{entityID: eid, removed: expired})
	}
	m.mu.Unlock()

	for _, p := range batch {
		m.reverseAndEmit(ctx, p.entityID, p.removed, true)
	}
}

// Drop removes every active effect for entityID without emitting
// any event or reversing stat modifiers — the session layer
// calls this at logout, where the actor is going away and its
// stat block won't be observed again. Returns the count of
// effects dropped.
//
// Distinct from RemoveByID/RemoveByFlag because logout does NOT
// emit "the spell wore off" notifications: the player is gone.
// Distinct from Tick because no time passed.
func (m *EffectManager) Drop(entityID string) int {
	eid := strings.ToLower(strings.TrimSpace(entityID))
	if eid == "" {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.effects[eid])
	if n > 0 {
		delete(m.effects, eid)
	}
	return n
}

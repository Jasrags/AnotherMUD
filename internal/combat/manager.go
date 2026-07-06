package combat

import (
	"context"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Manager owns the engage/disengage state described in spec
// combat §2: per-entity combat lists, primary-target promotion, and
// the engagement / combat-ended events.
//
// M7.2 scope: bookkeeping only. The round loop (heartbeat-bucket
// tick handler) lands in M7.3 and consumes Manager's queries; the
// auto-attack swing land M7.4; the cancellable death flow lands
// M7.5. Tag checks (safe-room, no-kill, flee-cooldown) deferred to
// M7.6 — today's Engage refuses only the already-engaged case (which
// is a no-op per §2.1, not an error).
//
// Internal storage is map[CombatantID][]CombatantID — IDs only, never
// live Combatant pointers. The Locator resolves IDs to live
// combatants when subscribers (or future round-loop code) need richer
// state. This choice is deliberate: a logged-out player whose ID
// remains in someone's combat list will fail the Locator on the next
// round-loop pre-flight (spec §4.1 "missing target → disengage"),
// which is the spec-aligned cleanup path. The alternative (storing
// live Combatant pointers) would require session teardown to call
// Manager.DisengageAll explicitly, which is a cross-package contract
// the four existing session teardown paths would be easy to break.
//
// Concurrency: a single RWMutex serializes all map mutations; query
// methods take the read lock; mutation methods (Engage, Disengage,
// DisengageAll, PromoteTarget) take the write lock. Events are
// published OUTSIDE the lock — collected during the locked section,
// emitted after the unlock — so a subscriber that re-enters Manager
// from its handler cannot deadlock. The Locator MUST NOT call back
// into Manager from the handler path; the contract is one-way (combat
// publishes; nothing about combat consumes those events to mutate
// itself).
//
// Sink may be nil — tests that don't assert on events pass nil and a
// no-op sink is substituted so emission paths don't have to nil-guard.
type Manager struct {
	mu      sync.RWMutex
	lists   map[CombatantID][]CombatantID
	locator Locator
	sink    EventSink

	// tags resolves the room / entity tags consulted by Engage's
	// refusal checks (safe-room, no-kill, no-flee). Optional: a nil
	// source means "tag refusals are skipped", which is the M7.2
	// behavior preserved for tests that don't wire tags.
	tags TagSource

	// cooldowns is the §5.3 flee-cooldown gate: an attacker whose
	// CombatantID has a future expiry tick value cannot Engage.
	// nil clock + nil cooldowns means "no cooldown gating" so M7.2
	// tests continue to pass with default Manager construction.
	cooldowns *FleeCooldowns

	// bands holds the per-pairing range band (ranged-combat §5), keyed on the
	// order-independent bandKey. An absent entry reads as the melee band (0),
	// so melee fights need no entry and pre-ranged behavior is unchanged. Set
	// on a ranged-initiated engage, cleared on disengage. Guarded by m.mu.
	bands map[bandKey]int

	// charged records pending charges as DIRECTIONAL (charger→victim) entries: a
	// combatant that CLOSED a band toward another and has not yet been answered.
	// A set-weapon victim consumes its charger's entry on its next swing to land
	// the braced "set vs a charge" blow (special-weapons §4). The key is
	// directional (unlike the order-independent bandKey) so mutual charges — both
	// combatants closing in the same round — coexist instead of overwriting. Set
	// by the round-loop auto-close + the advance verb, consumed by ConsumeCharge,
	// cleared on disengage. Guarded by m.mu.
	charged map[chargeKey]bool
}

// TagSource is the read surface Manager consults for §2.1 tag-based
// engagement refusals. The two predicates are separate because room
// tags (world side) and entity tags (entities + session side) live in
// different stores; bundling them into one interface keeps the
// Manager-side touch points to one field.
//
// Both methods MUST be safe for concurrent reads from the tick
// goroutine. Implementations resolve through whatever index they
// own — a session.Manager-backed implementation reads connActor tags;
// an entities.Store-backed implementation reads MobInstance tags.
type TagSource interface {
	RoomHasTag(roomID world.RoomID, tag string) bool
	EntityHasTag(id CombatantID, tag string) bool
}

// Tag constants consulted by Engage / flee. Spec text uses dashed
// names ("safe-room", "no-kill", "no-flee"); the constants carry the
// exact strings publishers and content packs declare in YAML.
const (
	TagSafeRoom = "safe-room"
	TagNoKill   = "no-kill"
	TagNoFlee   = "no-flee"
)

// NewManager returns an empty Manager. A nil locator is replaced by an
// empty MapLocator (every Name lookup misses and events carry empty
// names — useful for tests that don't wire a real locator). A nil sink
// is replaced by a no-op so the mutation path always has a non-nil
// dispatch target.
func NewManager(locator Locator, sink EventSink) *Manager {
	return NewManagerWith(ManagerConfig{Locator: locator, Sink: sink})
}

// ManagerConfig bundles Manager's constructor inputs. Locator and
// Sink behave the same as in NewManager (nil tolerated). Tags and
// Cooldowns are new in M7.6: both optional. A nil TagSource keeps
// the M7.2 "no tag refusals" behavior; nil Cooldowns disables flee-
// cooldown gating. Production wiring supplies both; tests pass only
// what they exercise.
type ManagerConfig struct {
	Locator   Locator
	Sink      EventSink
	Tags      TagSource
	Cooldowns *FleeCooldowns
}

// NewManagerWith is the option-shaped constructor introduced in
// M7.6. NewManager wraps it for backwards-compatible call sites.
func NewManagerWith(cfg ManagerConfig) *Manager {
	if cfg.Locator == nil {
		cfg.Locator = MapLocator{}
	}
	if cfg.Sink == nil {
		cfg.Sink = nopSink{}
	}
	return &Manager{
		lists:     make(map[CombatantID][]CombatantID),
		locator:   cfg.Locator,
		sink:      cfg.Sink,
		tags:      cfg.Tags,
		cooldowns: cfg.Cooldowns,
		bands:     make(map[bandKey]int),
	}
}

// Engage adds target to attacker's combat list and attacker to
// target's combat list. Spec §2.1.
//
// Refusals (§2.1):
//   - attacker == target → false, no event (caller bug; not a
//     no-op the spec considers; surface to handlers as "you can't
//     fight yourself" via the verb layer, not here).
//   - already engaged (target in attacker's list) → false, no event,
//     no error per §2.1 "already engaged is a no-op, not an error".
//
// Tag-based refusals (safe-room, no-kill, flee-cooldown) defer to
// M7.6. M7.2 always engages if not already engaged and not self-
// targeting.
//
// Returns true if a fresh engagement happened (and Engagement event
// was published); false if refused. RoomID is the shared room at
// engagement time, carried through to the event payload — combat
// itself does not consult it today, but listeners (UI, future quest
// hooks) want it.
func (m *Manager) Engage(ctx context.Context, attacker, target CombatantID, roomID world.RoomID) bool {
	_, ok := m.engageWithReason(ctx, attacker, target, roomID)
	return ok
}

// EngageRefusal explains why a non-successful Engage returned false.
// Callers (the `kill` verb today, future verb layers) use this to
// surface a precise message to the player rather than a generic
// "you can't attack that".
//
// EngageRefusalNone is the success value; the matching boolean from
// EngageWithReason is true. The other values are spec §2.1 refusals
// in the order Engage evaluates them.
type EngageRefusal int

const (
	EngageRefusalNone EngageRefusal = iota
	EngageRefusalSelfTarget
	EngageRefusalEmptyID
	EngageRefusalAlreadyEngaged
	EngageRefusalSafeRoom
	EngageRefusalNoKill
	EngageRefusalFleeCooldown
)

// EngageWithReason is the explicit-refusal variant of Engage. Returns
// the refusal code and a boolean that's true iff the engagement
// happened (refusal == EngageRefusalNone). Verbs that want to map a
// refusal back to a message use this; callers that only care
// about success/failure use Engage.
func (m *Manager) EngageWithReason(ctx context.Context, attacker, target CombatantID, roomID world.RoomID) (EngageRefusal, bool) {
	return m.engageWithReason(ctx, attacker, target, roomID)
}

func (m *Manager) engageWithReason(ctx context.Context, attacker, target CombatantID, roomID world.RoomID) (EngageRefusal, bool) {
	if attacker == target {
		return EngageRefusalSelfTarget, false
	}
	if attacker == "" || target == "" {
		return EngageRefusalEmptyID, false
	}

	// Tag-based refusals (spec §2.1). Evaluated BEFORE the
	// already-engaged check because the spec lists them first; a
	// fresh engage that would have been refused by a tag should not
	// silently succeed just because the pair happens to already be
	// engaged from an earlier round. The tag source is optional —
	// nil means "no tag gating" which preserves M7.2 test ergonomics.
	if m.tags != nil {
		if m.tags.RoomHasTag(roomID, TagSafeRoom) {
			return EngageRefusalSafeRoom, false
		}
		if m.tags.EntityHasTag(target, TagNoKill) {
			return EngageRefusalNoKill, false
		}
	}

	// Flee-cooldown refusal (§5.3): an attacker whose cooldown has
	// not yet expired cannot Engage but can be Engaged. Asymmetric on
	// purpose — fleers don't get hidden from pursuit, they get a
	// breather before they can re-initiate.
	if m.cooldowns != nil && m.cooldowns.Active(attacker) {
		return EngageRefusalFleeCooldown, false
	}

	// ranged-combat §5.2: the opening band depends on the initiator's wielded
	// weapon — a ranged opener (bow/thrown) opens at far, a melee opener at
	// melee. Resolved BEFORE m.mu because openingBand calls the locator (which
	// takes session/entities locks); m.mu must stay inner (see name resolution
	// below). A melee opener (meleeBand) needs no entry — absent reads as melee.
	openBand := m.openingBand(attacker)

	m.mu.Lock()
	if contains(m.lists[attacker], target) {
		m.mu.Unlock()
		return EngageRefusalAlreadyEngaged, false
	}
	m.lists[attacker] = append(m.lists[attacker], target)
	// Symmetric insertion: §2.1 step 2 guarantees both sides hold each
	// other after a successful engage. The second contains check
	// handles the (degenerate) case where target already had attacker
	// in its list but attacker did not — should not happen given the
	// first check, but cheap insurance against a future bug that
	// breaks symmetry elsewhere.
	if !contains(m.lists[target], attacker) {
		m.lists[target] = append(m.lists[target], attacker)
	}
	if openBand != meleeBand {
		m.setBandLocked(attacker, target, openBand)
	}
	m.mu.Unlock()

	// Resolve names OUTSIDE m.mu (lock-order narrowing, M7.2 review).
	// The production Locator takes session.Manager.mu and entities.
	// Store.mu; holding m.mu across those acquisitions would put
	// combat.Manager.mu at the root of a multi-package lock chain
	// that any future "session caller takes its lock then enters
	// combat" path would invert into a deadlock. Resolving outside
	// the lock keeps m.mu strictly inner.
	attackerName := m.lookupName(attacker)
	targetName := m.lookupName(target)

	m.sink.OnEngagement(ctx, Engagement{
		AttackerID:   attacker,
		TargetID:     target,
		AttackerName: attackerName,
		TargetName:   targetName,
		RoomID:       roomID,
	})
	return EngageRefusalNone, true
}

// Disengage removes each from the other's combat list (spec §2.2).
// If either side's list becomes empty as a result, a CombatEnded
// event is published for that side. Pairwise — the two-side cleanup
// of disengage-all lives in DisengageAll.
//
// Returns true if the pair was actually engaged (and at least one
// list was modified); false if either side was missing or didn't
// hold the other. The boolean is mostly for caller assertions; the
// state mutation itself is idempotent.
func (m *Manager) Disengage(ctx context.Context, a, b CombatantID, roomID world.RoomID) bool {
	if a == b || a == "" || b == "" {
		return false
	}

	var endedIDs []CombatantID

	m.mu.Lock()
	removedA := m.removeFromListLocked(a, b)
	removedB := m.removeFromListLocked(b, a)
	if !removedA && !removedB {
		m.mu.Unlock()
		return false
	}
	// The pairing's range band (ranged-combat §5) ends with the engagement.
	delete(m.bands, makeBandKey(a, b))
	// A pending charge (either direction) dies with the fight.
	delete(m.charged, chargeKey{a, b})
	delete(m.charged, chargeKey{b, a})

	if removedA && len(m.lists[a]) == 0 {
		delete(m.lists, a)
		endedIDs = append(endedIDs, a)
	}
	if removedB && len(m.lists[b]) == 0 {
		delete(m.lists, b)
		endedIDs = append(endedIDs, b)
	}
	m.mu.Unlock()

	// Name resolution outside m.mu — see Engage for the lock-order
	// rationale.
	for _, id := range endedIDs {
		m.sink.OnCombatEnded(ctx, CombatEnded{
			CombatantID:   id,
			CombatantName: m.lookupName(id),
			RoomID:        roomID,
		})
	}
	return true
}

// DisengageAll removes c from every opponent's list and clears c's
// own list (spec §2.3). One CombatEnded fires per opponent whose
// list becomes empty PLUS one for c itself (always — c's list is
// always emptied here, even if it was already empty).
//
// Used by death (M7.5) and flee (M7.6). Today no caller exists in
// the engine; combat tests cover the path.
//
// roomID carries through to the event payloads the same way
// Disengage does — combat itself does not consult it.
func (m *Manager) DisengageAll(ctx context.Context, c CombatantID, roomID world.RoomID) {
	if c == "" {
		return
	}

	var endedIDs []CombatantID

	m.mu.Lock()
	// Snapshot opponents before mutating: removing entries from each
	// opponent's list invalidates any range over m.lists[c], so we
	// copy first. This is the "snapshot and iterate" pattern §2.3
	// explicitly calls out.
	opponents := append([]CombatantID(nil), m.lists[c]...)
	for _, opp := range opponents {
		// The pairing's range band ends with the engagement (ranged-combat §5).
		delete(m.bands, makeBandKey(c, opp))
		delete(m.charged, chargeKey{c, opp})
		delete(m.charged, chargeKey{opp, c})
		if m.removeFromListLocked(opp, c) && len(m.lists[opp]) == 0 {
			delete(m.lists, opp)
			endedIDs = append(endedIDs, opp)
		}
	}
	delete(m.lists, c)
	// c always emits CombatEnded — even an entity that was not in
	// combat. The spec phrasing ("emit combat ended for the entity
	// itself") is unconditional. Callers that want a guard ("only emit
	// if it was in combat") can wrap with InCombat first; the engine
	// callers (death, flee) always know c was engaged when they invoke.
	endedIDs = append(endedIDs, c)
	m.mu.Unlock()

	// Name resolution outside m.mu — see Engage for the lock-order
	// rationale.
	for _, id := range endedIDs {
		m.sink.OnCombatEnded(ctx, CombatEnded{
			CombatantID:   id,
			CombatantName: m.lookupName(id),
			RoomID:        roomID,
		})
	}
}

// PromoteTarget moves opponent to the head of c's combat list, making
// it the primary target. Returns false if opponent is not already in
// c's list — the spec §2.4 rule is "MUST already be in the list, no
// silent insertion." Not symmetric: promoting on one side does not
// reorder the other side.
//
// Used by taunt, rescue, threat mechanics — none of which exist yet
// in M7.2, but the primitive lands here so abilities (M9) can use
// it without combat needing a second pass.
func (m *Manager) PromoteTarget(c, opponent CombatantID) bool {
	if c == "" || opponent == "" || c == opponent {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.lists[c]
	for i, id := range list {
		if id != opponent {
			continue
		}
		if i == 0 {
			return true // already primary; idempotent success.
		}
		// Move to head, preserving relative order of the rest. In-place reorder
		// of c's backing array — safe under the same no-slice-escapes
		// invariant documented on removeFromListLocked.
		moved := list[i]
		copy(list[1:i+1], list[0:i])
		list[0] = moved
		m.lists[c] = list
		return true
	}
	return false
}

// InCombat reports whether c has at least one opponent.
func (m *Manager) InCombat(c CombatantID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.lists[c]) > 0
}

// PrimaryTargetOf returns the head of c's combat list, or
// ("", false) if c is not in combat. Returns a copy of the ID
// (CombatantID is a value-typed string).
func (m *Manager) PrimaryTargetOf(c CombatantID) (CombatantID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := m.lists[c]
	if len(list) == 0 {
		return "", false
	}
	return list[0], true
}

// OpponentsOf returns a snapshot copy of c's combat list. Callers
// may iterate freely without affecting Manager state — required by
// spec §2.5 ("Combat list of entity X — a snapshot copy (not a live
// reference), so callers may iterate without affecting state").
// Empty slice (not nil) when c is not in combat, so range-over-nil
// edge cases don't surprise callers; len == 0 is the canonical check.
func (m *Manager) OpponentsOf(c CombatantID) []CombatantID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := m.lists[c]
	if len(list) == 0 {
		return []CombatantID{}
	}
	out := make([]CombatantID, len(list))
	copy(out, list)
	return out
}

// AllCombatants returns a snapshot of every CombatantID currently in
// combat (i.e. every key in the lists map). Spec §2.5: "All current
// combatants — iteration over every entity currently in combat.
// Order is unspecified to callers but MUST be stable during a single
// tick." Stability within a tick is provided by the snapshot — the
// returned slice is a copy, so a concurrent Engage during iteration
// cannot tear the result.
func (m *Manager) AllCombatants() []CombatantID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]CombatantID, 0, len(m.lists))
	for id := range m.lists {
		out = append(out, id)
	}
	return out
}

// removeFromListLocked drops the first occurrence of opp from c's
// list. Returns true if anything was removed. Caller MUST hold m.mu.
// Does NOT delete the map entry on emptying — that's the caller's
// responsibility (so Disengage / DisengageAll can decide whether to
// emit CombatEnded before the delete).
//
// INVARIANT: the append below shifts c's backing array in place, which is
// safe ONLY because no un-copied m.lists slice ever escapes m.mu — every
// accessor snapshots (OpponentsOf/AllCombatants copy; PrimaryTargetOf returns
// a value; DisengageAll copies before its mutation loop). If a future accessor
// returns the raw slice, this in-place mutation would corrupt that holder's
// view — copy on the way out, or switch this to a fresh-slice build.
func (m *Manager) removeFromListLocked(c, opp CombatantID) bool {
	list := m.lists[c]
	for i, id := range list {
		if id == opp {
			m.lists[c] = append(list[:i], list[i+1:]...)
			return true
		}
	}
	return false
}

// lookupName resolves id → Name via the locator for event payloads.
// Returns "" if the locator misses (e.g. a logged-out player whose
// CombatantID still sits in someone's list during the disengage
// that's about to clean it up).
//
// LOCK CONTRACT: caller MUST NOT hold m.mu. The production locator
// (combatLocator in cmd/anothermud) acquires session.Manager.mu and
// then connActor.mu via Name(); holding m.mu across those would
// extend the lock-chain root to combat.Manager and any future
// path that takes session.Manager.mu before entering combat.Manager
// would deadlock. Resolving outside the mutation lock keeps
// combat.Manager.mu strictly inner relative to session/entities
// locks (it never holds them; it never has them held while it runs).
func (m *Manager) lookupName(id CombatantID) string {
	if m.locator == nil {
		return ""
	}
	c, ok := m.locator.LookupCombatant(id)
	if !ok {
		return ""
	}
	return c.Name()
}

// contains is a tiny linear scan. Combat lists are short (single-
// digit typically), so a slice + scan beats a map per-combatant on
// memory and on the iteration path that the round loop will use.
func contains(list []CombatantID, id CombatantID) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

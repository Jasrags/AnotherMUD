package ai

import (
	"context"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// PlayerView is the read-only projection of a player that the
// disposition evaluator needs (spec mobs-ai-spawning §5.3 rule
// matching). It is intentionally tiny: ID for caching and dispatch,
// Name for log/event payloads, Tags for `has_tag` rules.
//
// Alignment fields land alongside M8 progression. The evaluator
// already accepts and stores rules referencing alignment so content
// can be authored ahead of the runtime support; they just never
// match until that data exists.
type PlayerView struct {
	ID   string
	Name string
	Tags []string
}

// PlayerLookup is the seam the evaluator uses to find players. The
// session manager is the production implementation; tests pass an
// in-memory stub. The interface lives here (consumer-side) per the
// project's "accept interfaces, return structs" convention.
type PlayerLookup interface {
	// PlayersInRoom returns every connected player currently in the
	// given room. Order is unspecified.
	PlayersInRoom(ctx context.Context, room world.RoomID) []PlayerView

	// PlayerByID returns the view for a specific player. ok is false
	// when the player is not connected (typical for the
	// OnPlayerEnteredDeferred path is "always ok"; mob-entered-room
	// can race against a quit).
	PlayerByID(ctx context.Context, id string) (PlayerView, bool)
}

// TemplateLookup resolves a mob instance back to the template that
// produced it. mob.Templates satisfies this trivially via a small
// adapter; tests build a map-backed stub.
type TemplateLookup interface {
	Get(id mob.TemplateID) (*mob.Template, error)
}

// Mode picks between full evaluation and the aggro-only short
// path. Spec §5.4: the immediate room-entry hook uses
// ModeAggroOnly so non-hostile reactions don't fire before the
// room description renders.
type Mode int

const (
	ModeFull Mode = iota
	ModeAggroOnly
)

// pairKey is the per-tick dedup key (spec §5.2). Identical across
// the lifetime of one tick; cleared by ResetTick.
type pairKey struct {
	mob    entities.EntityID
	player string
}

// roomPlayerKey is the per-room reaction-state key (spec §5.2).
// Cleared for a player when they leave a room (subscription to
// EventPlayerMoved).
type roomPlayerKey struct {
	room   world.RoomID
	player string
}

// Evaluator owns disposition reactions (spec mobs-ai-spawning §5).
// Safe for concurrent use; AI tick and command goroutines call into
// it in parallel.
type Evaluator struct {
	templates TemplateLookup
	players   PlayerLookup
	bus       *eventbus.Bus
	placement *entities.Placement
	store     *entities.Store

	mu sync.Mutex
	// tickPair tracks (mob, player) pairs already evaluated this
	// tick. Cleared by ResetTick at the top of each AI tick.
	tickPair map[pairKey]struct{}
	// roomState maps (room, player) to the set of non-hostile
	// reactions already dispatched while that player has been in
	// the room. Cleared per-(room,player) on EventPlayerMoved.
	// Hostile reactions are NEVER recorded here (§5.2 — they bypass
	// the suppression).
	roomState map[roomPlayerKey]map[mob.Reaction]struct{}
}

// EvaluatorConfig bundles the evaluator's constructor inputs. nil
// fields are tolerated by the evaluator but degrade behavior:
//
//   - nil Templates: no mob has rules, nothing dispatches
//   - nil Players: nothing dispatches (no targets)
//   - nil Placement: room-scoped helpers return empty lists
//   - nil Bus: events are skipped (used by tests)
type EvaluatorConfig struct {
	Templates TemplateLookup
	Players   PlayerLookup
	Placement *entities.Placement
	Store     *entities.Store
	Bus       *eventbus.Bus
}

// NewEvaluator returns a configured evaluator and subscribes it to
// EventPlayerMoved so per-room reaction state is cleared when a
// player leaves a room. Caller owns lifecycle; the subscription is
// process-lifetime (no Unsubscribe today, mirroring other bus
// subscribers).
func NewEvaluator(cfg EvaluatorConfig) *Evaluator {
	e := &Evaluator{
		templates: cfg.Templates,
		players:   cfg.Players,
		placement: cfg.Placement,
		store:     cfg.Store,
		bus:       cfg.Bus,
		tickPair:  make(map[pairKey]struct{}),
		roomState: make(map[roomPlayerKey]map[mob.Reaction]struct{}),
	}
	if cfg.Bus != nil {
		cfg.Bus.Subscribe(eventbus.EventPlayerMoved, func(_ context.Context, ev eventbus.Event) {
			m, ok := ev.(eventbus.PlayerMoved)
			if !ok {
				return
			}
			e.clearRoomState(m.From, m.PlayerID)
		})
	}
	return e
}

// ResetTick clears the per-tick dedup cache (spec §5.2). Called by
// Dispatcher.Tick at the top of every AI tick.
func (e *Evaluator) ResetTick() {
	e.mu.Lock()
	e.tickPair = make(map[pairKey]struct{})
	e.mu.Unlock()
}

// clearRoomState forgets every recorded non-hostile reaction for
// (room, player). A no-op when the room is empty (login spawn).
func (e *Evaluator) clearRoomState(room world.RoomID, player string) {
	if player == "" {
		return
	}
	e.mu.Lock()
	delete(e.roomState, roomPlayerKey{room: room, player: player})
	e.mu.Unlock()
}

// OnPlayerEnteredImmediate runs the aggro-only sweep before the
// room description renders (spec §4 "Player entering room
// (immediate)"). Only hostile reactions can dispatch from this
// path.
func (e *Evaluator) OnPlayerEnteredImmediate(ctx context.Context, player PlayerView, room world.RoomID) {
	e.sweepRoom(ctx, player, room, ModeAggroOnly)
}

// OnPlayerEnteredDeferred runs after the room description renders
// (spec §4 "Player entered room (deferred)"). Full evaluator —
// any reaction may dispatch.
func (e *Evaluator) OnPlayerEnteredDeferred(ctx context.Context, player PlayerView, room world.RoomID) {
	e.sweepRoom(ctx, player, room, ModeFull)
}

// OnMobEntered fires when a mob moves into a room. Evaluates the
// arriving mob against every player currently in the room (spec
// §4 "Mob entered room"). Full evaluator — there's no room
// description racing the dispatch.
func (e *Evaluator) OnMobEntered(ctx context.Context, m *entities.MobInstance, room world.RoomID) {
	if e.players == nil || m == nil {
		return
	}
	for _, p := range e.players.PlayersInRoom(ctx, room) {
		e.Evaluate(ctx, m, p, room, ModeFull)
	}
}

// sweepRoom iterates every mob placed in the room and evaluates
// each one against the given player. Used by both room-entry
// hooks; mode picks aggro-only vs. full.
func (e *Evaluator) sweepRoom(ctx context.Context, player PlayerView, room world.RoomID, mode Mode) {
	if e.placement == nil || e.store == nil {
		return
	}
	for _, id := range e.placement.InRoom(room) {
		ent, ok := e.store.GetByID(id)
		if !ok {
			continue
		}
		m, ok := ent.(*entities.MobInstance)
		if !ok {
			// Placement holds items too; only MobInstance carries
			// the disposition surface.
			continue
		}
		e.Evaluate(ctx, m, player, room, mode)
	}
}

// Evaluate runs the §5.3 evaluation algorithm for one (mob,
// player) pair in the given room and dispatches the resulting
// event if appropriate. Public so callers that already hold a
// MobInstance (the AI tick, OnMobEntered) can avoid a redundant
// Placement scan.
func (e *Evaluator) Evaluate(ctx context.Context, m *entities.MobInstance, player PlayerView, room world.RoomID, mode Mode) {
	if m == nil || player.ID == "" {
		return
	}
	// Resolve template + decide reaction WITHOUT holding the
	// evaluator mutex. Two concurrent callers (AI tick + command
	// goroutine) might both reach this point with the same pair
	// before the dedup commit below — the atomic check-and-set
	// at line // 'commit dedup' resolves the race so only one
	// dispatches.
	tpl, ok := e.template(m)
	if !ok {
		return
	}
	reaction, ok := decideReaction(tpl, player)
	if !ok {
		return
	}

	// Aggro-only mode short-circuits BEFORE the per-tick cache is
	// populated (§5.4: "not cached, not dispatched. Leaving it
	// uncached preserves the option for the deferred call to
	// evaluate and dispatch it normally"). No cache write means no
	// race to worry about here.
	if mode == ModeAggroOnly && reaction != mob.ReactionHostile {
		return
	}

	// Commit dedup atomically: a single locked check-and-set so two
	// concurrent callers can't both observe "unseen" and both
	// dispatch. The cost is recomputing template+reaction on the
	// losing side; cheap compared to a second dispatch.
	key := pairKey{mob: m.ID(), player: player.ID}
	e.mu.Lock()
	if _, seen := e.tickPair[key]; seen {
		e.mu.Unlock()
		return
	}
	e.tickPair[key] = struct{}{}
	e.mu.Unlock()

	// Hostile bypasses the per-room suppression (§5.2). Combat's
	// engage handler is idempotent so duplicate emissions are
	// harmless.
	if reaction == mob.ReactionHostile {
		e.dispatch(ctx, reaction, m, player, room)
		return
	}

	// Non-hostile: suppress repeat dispatches inside the same
	// room (§5.2). Record the reaction so subsequent ticks in
	// this room with this player don't re-greet.
	rpKey := roomPlayerKey{room: room, player: player.ID}
	e.mu.Lock()
	state, ok := e.roomState[rpKey]
	if !ok {
		state = make(map[mob.Reaction]struct{})
		e.roomState[rpKey] = state
	}
	if _, repeat := state[reaction]; repeat {
		e.mu.Unlock()
		return
	}
	state[reaction] = struct{}{}
	e.mu.Unlock()

	e.dispatch(ctx, reaction, m, player, room)
}

// decideReaction implements the §5.3 algorithm against a single
// template + player. ok=false means "no reaction at all" (no rules,
// no base disposition) — the caller should not dispatch anything.
func decideReaction(tpl *mob.Template, player PlayerView) (mob.Reaction, bool) {
	// Step 3: static hostile is unconditional.
	if tpl.BaseDisposition == mob.ReactionHostile {
		return mob.ReactionHostile, true
	}
	// Step 4: walk the rule list when present.
	if tpl.DispositionRules != nil {
		for _, r := range tpl.DispositionRules.Rules {
			if ruleMatches(r, player) {
				return r.Reaction, true
			}
		}
		// No rule matched → default. Empty default returns
		// no-dispatch instead of synthesizing neutral: synthesizing
		// would waste a per-room state slot on a reaction that
		// emits no event today (dispatch's neutral branch is a
		// no-op).
		if tpl.DispositionRules.Default != "" {
			return tpl.DispositionRules.Default, true
		}
		return "", false
	}
	// Non-hostile static reaction with no rule list: single fixed
	// reaction. Spec §5.1 treats this as the legacy "base
	// disposition" path.
	if tpl.BaseDisposition != "" {
		return tpl.BaseDisposition, true
	}
	return "", false
}

// ruleMatches reports whether r's conditions all match player. A
// rule with no conditions matches anything (spec §5.3). Alignment
// conditions are accepted but never match today — they'll start
// matching when PlayerView grows the alignment field in M8.
func ruleMatches(r mob.Rule, player PlayerView) bool {
	if !r.HasConditions() {
		return true
	}
	if r.HasTag != "" {
		if !playerHasTag(player, r.HasTag) {
			return false
		}
	}
	if r.HasMinAlignment || r.HasMaxAlignment || len(r.Buckets) > 0 {
		// Alignment data does not exist yet; treat as "this
		// condition cannot be satisfied" so rules that depend on
		// alignment never match prematurely. When the field
		// arrives this branch becomes a real comparison.
		return false
	}
	return true
}

func playerHasTag(p PlayerView, tag string) bool {
	for _, t := range p.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// template resolves m's template via the configured lookup. Returns
// ok=false when templates is nil (test setups) or the template id
// is missing from the registry (content was reloaded under a live
// instance — shouldn't happen at M6 but guard anyway).
func (e *Evaluator) template(m *entities.MobInstance) (*mob.Template, bool) {
	if e.templates == nil {
		return nil, false
	}
	raw, ok := m.Properties()[entities.PropTemplateID].(string)
	if !ok || raw == "" {
		return nil, false
	}
	tpl, err := e.templates.Get(mob.TemplateID(raw))
	if err != nil {
		return nil, false
	}
	return tpl, true
}

// dispatch publishes the appropriate event for reaction. Unknown
// reactions emit nothing (spec §5.5 "the engine treats unknown
// reactions as no-op").
func (e *Evaluator) dispatch(ctx context.Context, reaction mob.Reaction, m *entities.MobInstance, player PlayerView, room world.RoomID) {
	if e.bus == nil {
		return
	}
	switch reaction {
	case mob.ReactionHostile:
		e.bus.Publish(ctx, eventbus.MobAggro{
			MobID:    m.ID(),
			MobName:  m.Name(),
			PlayerID: player.ID,
			RoomID:   room,
		})
	case mob.ReactionWary:
		e.bus.Publish(ctx, eventbus.MobWary{
			MobID:    m.ID(),
			MobName:  m.Name(),
			PlayerID: player.ID,
			RoomID:   room,
		})
	case mob.ReactionFriendly:
		e.bus.Publish(ctx, eventbus.MobFriendly{
			MobID:    m.ID(),
			MobName:  m.Name(),
			PlayerID: player.ID,
			RoomID:   room,
		})
	default:
		// neutral and unknown reactions intentionally emit
		// nothing (§5.5).
	}
}


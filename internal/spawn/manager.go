package spawn

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Tag values inspected by the reset algorithm. Spec
// mobs-ai-spawning §3.6: "Read the `persistent` flag from the rule's
// tag list. If persistent and the count is at or above the target,
// skip this rule."
const TagPersistent = "persistent"

// Spawner is the boundary the manager calls to mint a new mob
// instance. The production implementation lives in cmd/anothermud
// (bootSpawner) so the spawn package doesn't pull in entities/mob
// concretely. Returning the new EntityID lets the manager record it
// against the producing rule (spec §3.6 last step).
type Spawner interface {
	Spawn(ctx context.Context, templateID string, roomID world.RoomID) (entities.EntityID, error)
}

// Store is the subset of entities.Store the manager needs to decide
// whether a tracked entity is still alive (spec §3.6 step 1).
// Defined as a tiny interface so tests can pass a map-backed stub.
type Store interface {
	GetByID(entities.EntityID) (entities.Entity, bool)
}

// Manager subscribes to area.tick events and runs the §3.6 reset
// algorithm. Safe for concurrent use; the bus delivers events
// serialized today but the manager doesn't rely on that — its only
// mutable state is the Tracker and a sync.Mutex around the RNG.
type Manager struct {
	world   *world.World
	tracker *Tracker
	spawner Spawner
	store   Store
	bus     *eventbus.Bus

	rngMu sync.Mutex
	rng   *rand.Rand
}

// Config bundles Manager's constructor inputs. Rng is optional; the
// constructor supplies a wall-clock-seeded PCG default when nil so
// independent processes don't share a sequence.
type Config struct {
	World   *world.World
	Tracker *Tracker
	Spawner Spawner
	Store   Store
	Bus     *eventbus.Bus
	Rng     *rand.Rand
}

// NewManager wires the manager and subscribes it to area-tick.
// Subscription is process-lifetime (no Close today) — same
// convention as the disposition evaluator. The returned manager
// owns its Tracker only by reference; callers can share one
// tracker across managers if a future use case ever needs it.
func NewManager(cfg Config) *Manager {
	m := &Manager{
		world:   cfg.World,
		tracker: cfg.Tracker,
		spawner: cfg.Spawner,
		store:   cfg.Store,
		bus:     cfg.Bus,
		rng:     cfg.Rng,
	}
	if m.rng == nil {
		// Seed exactly as ai.Dispatcher does so the two engines
		// don't share a sequence by accident.
		m.rng = rand.New(rand.NewPCG(seedNanos(), seedNanos()^0x9e3779b97f4a7c15))
	}
	if cfg.Bus != nil {
		cfg.Bus.Subscribe(eventbus.EventAreaTick, func(ctx context.Context, ev eventbus.Event) {
			t, ok := ev.(eventbus.AreaTick)
			if !ok {
				return
			}
			m.Reset(ctx, t.AreaID)
		})
	}
	return m
}

// Reset runs the §3.6 area-reset algorithm for areaID. Exposed
// directly so tests can drive it without publishing events; the
// production path comes via the area.tick subscription installed in
// NewManager.
//
// Errors are logged at the per-rule level; one bad rule does not
// abort the area. Mirrors the AI tick's "behavior failure is a
// warning, not a fatal" contract.
func (m *Manager) Reset(ctx context.Context, areaID world.AreaID) {
	if m.world == nil || m.tracker == nil || m.spawner == nil {
		return
	}
	area, err := m.world.Area(areaID)
	if err != nil {
		return
	}
	logger := logging.From(ctx).With(
		slog.String("event", "area.reset"),
		slog.String("area_id", string(areaID)),
	)

	for i, rule := range area.SpawnRules {
		m.applyRule(ctx, logger, areaID, i, rule)
	}
}

// applyRule processes one rule's reset step. Extracted so each
// rule's per-step logging carries a stable structured prefix.
func (m *Manager) applyRule(ctx context.Context, logger *slog.Logger, areaID world.AreaID, ruleIdx int, rule world.SpawnRule) {
	ruleLog := logger.With(
		slog.Int("rule_idx", ruleIdx),
		slog.String("room_id", string(rule.RoomID)),
		// One of mob/node is set per rule; log both so node rules are
		// identifiable (mob="" alone would be ambiguous).
		slog.String("mob", rule.MobTemplateID),
		slog.String("node", rule.NodeTemplateID),
	)

	// §3.6 step "Resolve the room; skip the rule if the room does
	// not exist." Pack-load validation catches this at boot, but
	// guard at runtime too against post-boot world mutation.
	if _, err := m.world.Room(rule.RoomID); err != nil {
		ruleLog.Warn("spawn rule skipped: room missing")
		return
	}

	// Step 1: purge dead tracking BEFORE counting.
	m.tracker.Purge(areaID, ruleIdx, m.alive)

	// Step "count living + persistent check".
	living := m.tracker.Count(areaID, ruleIdx)
	if rule.HasTag(TagPersistent) && living >= rule.Count {
		return
	}

	missing := rule.Count - living
	if missing <= 0 {
		return
	}

	// Step "for each missing slot: choose template + spawn + track".
	for range missing {
		templateID := m.chooseTemplate(rule)
		id, err := m.spawner.Spawn(ctx, templateID, rule.RoomID)
		if err != nil {
			ruleLog.Warn("spawn failed",
				slog.String("template", templateID),
				slog.Any("err", err))
			continue
		}
		m.tracker.Track(areaID, ruleIdx, id)
		ruleLog.Info("spawn placed",
			slog.String("entity_id", string(id)),
			slog.String("template", templateID))
	}
}

// chooseTemplate implements the rare-swap roll (spec §3.6
// "if the rule has a rare-swap and the rare chance roll succeeds,
// use the rare template instead"). Rolls are independent per
// missing slot.
func (m *Manager) chooseTemplate(rule world.SpawnRule) string {
	// Resource-node rules carry a node template id and never rare-swap
	// (gathering.md §3.1). The spawner disambiguates node vs mob by
	// registry lookup, so returning the node id here is sufficient.
	if rule.NodeTemplateID != "" {
		return rule.NodeTemplateID
	}
	if rule.Rare == "" || rule.RareChance <= 0 {
		return rule.MobTemplateID
	}
	m.rngMu.Lock()
	roll := m.rng.Float64()
	m.rngMu.Unlock()
	if roll < rule.RareChance {
		return rule.Rare
	}
	return rule.MobTemplateID
}

// alive is the predicate Tracker.Purge calls to test each tracked
// id. Lives on Manager so the Tracker stays free of any Store
// dependency.
func (m *Manager) alive(id entities.EntityID) bool {
	if m.store == nil {
		// No store wired (test fixtures) → treat every tracked id
		// as still alive so the test path doesn't accidentally
		// purge everything.
		return true
	}
	_, ok := m.store.GetByID(id)
	return ok
}

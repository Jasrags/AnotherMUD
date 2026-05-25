// Command anothermud is the MUD server entrypoint.
//
// M3 scope: configure logging, install signal-cancelled root ctx, load
// content packs into a world, open the account+player stores, start the
// tick loop with an autosave handler, open a TCP listener, hand it to
// server.Serve with the session handler (which now runs login first).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/account"
	"github.com/Jasrags/AnotherMUD/internal/ai"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/pack"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/server"
	"github.com/Jasrags/AnotherMUD/internal/session"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/spawn"
	"github.com/Jasrags/AnotherMUD/internal/tick"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// version is set via -ldflags "-X main.version=..." by the Makefile.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "anothermud: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()

	logger := newLogger(cfg)
	slog.SetDefault(logger)
	logging.Default = logger

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctx = logging.With(ctx,
		slog.String("version", version),
		slog.String("component", "server"),
	)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}

	registries := pack.NewRegistries()
	// Engine baseline slots register before pack loading so packs that
	// try to redefine baseline slots fail loudly at boot rather than
	// silently overriding (spec inventory-equipment-items §3.1).
	if err := slot.RegisterEngineBaseline(registries.Slots); err != nil {
		return fmt.Errorf("register engine baseline slots: %w", err)
	}

	// entityStore + placement are constructed at boot so the tag-swap
	// tick handler can be wired immediately and the session layer can
	// pass both into command dispatch for get/drop.
	//
	// They are also passed to pack.Load via the bootSpawner adapter so
	// room YAML `items:` lists spawn-and-place at load time (spec
	// world-rooms-movement §2.2).
	entityStore := entities.NewStore()
	placement := entities.NewPlacement()
	contents := entities.NewContents()
	bus := eventbus.New()

	spawner := &bootSpawner{
		store:        entityStore,
		placement:    placement,
		templates:    registries.Items,
		mobTemplates: registries.Mobs,
		bus:          bus,
	}
	if err := pack.Load(ctx, cfg.ContentDir, nil, registries, spawner, spawner); err != nil {
		return fmt.Errorf("loading content from %s: %w", cfg.ContentDir, err)
	}
	w := registries.World
	if _, err := w.Room(cfg.StartRoom); err != nil {
		return fmt.Errorf("starting room %q not in loaded world: %w", cfg.StartRoom, err)
	}

	accounts, err := account.NewService(cfg.SaveDir)
	if err != nil {
		return fmt.Errorf("open account store: %w", err)
	}
	players, err := player.NewStore(cfg.SaveDir)
	if err != nil {
		return fmt.Errorf("open player store: %w", err)
	}

	cmds := command.New()
	if err := command.RegisterBuiltins(cmds); err != nil {
		return fmt.Errorf("register builtins: %w", err)
	}

	mgr := session.NewManager()

	// Store.SetRoomScan is intentionally NOT wired today: every spawned
	// item goes through Store.Spawn which auto-tracks, so the by-id
	// index is always the source of truth. The §4.2 step-2 fallback
	// becomes relevant only once items can enter the world without
	// passing through Spawn (e.g. external loader); add the bridge
	// when that path lands rather than fabricating one now.

	clk := clock.RealClock{}
	loop := tick.New(clk, cfg.TickInterval)
	if err := entities.RegisterTagSwap(loop, entityStore); err != nil {
		return fmt.Errorf("register entities tag-swap: %w", err)
	}
	autosaveInterval := autosaveTicks(cfg.TickInterval, cfg.AutosaveInterval)
	if err := loop.Register("autosave", autosaveInterval, func(ctx context.Context, n uint64) {
		mgr.SaveAll(ctx)
	}); err != nil {
		return fmt.Errorf("register autosave tick: %w", err)
	}

	idleCfg := session.DefaultIdleConfig()
	idleSweepCadence := cadenceTicks(cfg.TickInterval, cfg.IdleSweepInterval)
	if err := loop.Register("idle-sweep", idleSweepCadence, func(ctx context.Context, n uint64) {
		mgr.IdleSweep(ctx, idleCfg, clk)
	}); err != nil {
		return fmt.Errorf("register idle-sweep tick: %w", err)
	}

	// AI tick (spec mobs-ai-spawning §4). Registers AFTER the
	// tag-swap handler so the first dispatch sees the read-side tag
	// index populated by the pack.Load placements. Cadence one
	// second (10 ticks at the 100ms default) — fast enough that
	// wander feels alive, slow enough not to dominate the loop.
	aiReg := ai.NewRegistry()
	if err := ai.RegisterEngineBaseline(aiReg); err != nil {
		return fmt.Errorf("register ai baseline: %w", err)
	}
	// Disposition evaluator (spec mobs-ai-spawning §5). Constructed
	// before the AI dispatcher so it can be passed in via Deps, and
	// before session.Handler so the room-entry hook surface is
	// available at first login.
	evaluator := ai.NewEvaluator(ai.EvaluatorConfig{
		Templates: registries.Mobs,
		Players:   playerLookup{mgr: mgr},
		Placement: placement,
		Store:     entityStore,
		Bus:       bus,
	})

	aiDispatcher := ai.NewDispatcher(aiReg, ai.Deps{
		World:       w,
		Placement:   placement,
		Store:       entityStore,
		Bus:         bus,
		Broadcaster: mgr,
		Clock:       clk,
		Evaluator:   evaluator,
		// Rand left nil — Dispatcher.Tick supplies a default.
	})
	aiDispatcher.AttachEvaluator(evaluator)

	aiCadence := cadenceTicks(cfg.TickInterval, time.Second)
	if err := loop.Register("ai-tick", aiCadence, aiDispatcher.Tick); err != nil {
		return fmt.Errorf("register ai tick: %w", err)
	}

	// Area-driven respawn (spec mobs-ai-spawning §3.5–3.7). The
	// tracker holds per-rule live-instance counts; the manager runs
	// the §3.6 reset algorithm on each area.tick event; the
	// scheduler emits those events at per-area cadence
	// (base × occupiedModifier). First tick fires on the scheduler
	// step boundary, populating areas from zero — no boot-time
	// placement needed for mobs that live in areas with spawn_rules.
	spawnTracker := spawn.NewTracker()
	spawnManager := spawn.NewManager(spawn.Config{
		World:   w,
		Tracker: spawnTracker,
		Spawner: &bootSpawnerAdapter{inner: spawner},
		Store:   entityStore,
		Bus:     bus,
	})
	_ = spawnManager // retained only for documentation of bus subscription

	scheduler := spawn.NewScheduler(spawn.SchedulerConfig{
		World:            w,
		Bus:              bus,
		Presence:         presenceSource{mgr: mgr, world: w},
		DefaultReset:     cadenceTicks(cfg.TickInterval, 30*time.Second),
		OccupiedModifier: 1.0,
	})
	// Register the scheduler at the same 1-second cadence the AI
	// dispatcher uses, but pass the cadence in as deltaTicks so the
	// scheduler advances its per-area accumulators in game-tick
	// units (the same units Area.ResetInterval is authored in).
	if err := loop.Register("area-tick", aiCadence, func(ctx context.Context, _ uint64) {
		scheduler.Step(ctx, aiCadence)
	}); err != nil {
		return fmt.Errorf("register area tick: %w", err)
	}

	linkDeadCfg := cfg.LinkDead
	if linkDeadCfg.Enabled {
		linkDeadCadence := cadenceTicks(cfg.TickInterval, cfg.LinkDeadSweepInterval)
		if err := loop.Register("linkdead-cleanup", linkDeadCadence, func(ctx context.Context, n uint64) {
			mgr.LinkDeadCleanup(ctx, linkDeadCfg, clk)
		}); err != nil {
			return fmt.Errorf("register linkdead-cleanup tick: %w", err)
		}
	}

	// Combat manager (spec combat §2, M7.2). Locator dispatches on the
	// CombatantID prefix: mob: → entities.Store, player: →
	// session.Manager. Sink is log-only today; M7.5 (mob.killed →
	// spawn untrack) and M7.6 (combat.ended → flee cooldown clear)
	// will replace it with a real eventbus-backed adapter when those
	// subscribers exist.
	combatLocator := combatLocator{entities: entityStore, sessions: mgr}
	combatSink := loggingCombatSink{logger: logging.From(ctx)}
	combatMgr := combat.NewManager(combatLocator, combatSink)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := loop.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logging.From(ctx).Warn("tick loop exited with error", slog.Any("err", err))
		}
	}()

	logging.From(ctx).Info("anothermud starting",
		slog.String("addr", ln.Addr().String()),
		slog.String("log_format", cfg.LogFormat),
		slog.String("log_level", cfg.LogLevel),
		slog.Duration("tick_interval", cfg.TickInterval),
		slog.Duration("autosave_interval", cfg.AutosaveInterval),
		slog.String("content_dir", cfg.ContentDir),
		slog.String("save_dir", cfg.SaveDir),
		slog.String("start_room", string(cfg.StartRoom)),
		slog.Bool("color_default", cfg.ColorDefault),
	)

	handler := session.Handler(session.Config{
		World:        w,
		Commands:     cmds,
		Players:      players,
		Manager:      mgr,
		Items:        entityStore,
		Placement:    placement,
		Contents:     contents,
		Templates:    registries.Items,
		Slots:        registries.Slots,
		Bus:          bus,
		Disposition:  dispositionHook{e: evaluator},
		Combat:       combatMgr,
		StartID:      cfg.StartRoom,
		ColorEnabled: cfg.ColorDefault,
		Clock:        clk,
		Flood:        session.DefaultFloodConfig(),
		LinkDead:     linkDeadCfg,
		Login: login.Config{
			Accounts:        accounts,
			Players:         players,
			DefaultLocation: string(cfg.StartRoom),
		},
	})
	srv := &server.Server{Handler: handler}
	serveErr := srv.Serve(ctx, ln)

	// Final flush so anyone still in-world has their state committed
	// even if they didn't disconnect cleanly. Uses a fresh ctx that is
	// not already cancelled.
	flushCtx, cancelFlush := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	mgr.SaveAll(flushCtx)
	cancelFlush()

	wg.Wait()
	if serveErr != nil && !errors.Is(serveErr, server.ErrServerClosed) {
		return fmt.Errorf("serve: %w", serveErr)
	}
	logging.From(ctx).Info("anothermud stopped cleanly")
	return nil
}

// autosaveTicks converts a wall-clock interval into a tick cadence,
// honoring the tick interval. Returns at least 1 so a misconfigured
// interval doesn't trip tick.Register's > 0 check.
func autosaveTicks(tickInterval, autosaveInterval time.Duration) uint64 {
	return cadenceTicks(tickInterval, autosaveInterval)
}

// cadenceTicks is the generic wall-clock → tick conversion used by
// any handler that wants to fire on a real-time cadence rather than
// every tick.
func cadenceTicks(tickInterval, cadence time.Duration) uint64 {
	if tickInterval <= 0 || cadence <= 0 {
		return 1
	}
	n := uint64(cadence / tickInterval)
	if n == 0 {
		return 1
	}
	return n
}

// config is the M3 config knobs — env-only until we have more than
// ~5 of them per the ROADMAP "not front-loaded" list.
type config struct {
	Addr                  string
	LogLevel              string
	LogFormat             string
	TickInterval          time.Duration
	AutosaveInterval      time.Duration
	IdleSweepInterval     time.Duration
	LinkDeadSweepInterval time.Duration
	ContentDir            string
	SaveDir               string
	StartRoom             world.RoomID
	ColorDefault          bool
	LinkDead              session.LinkDeadConfig
}

func loadConfig() config {
	ld := session.DefaultLinkDeadConfig()
	if v, ok := os.LookupEnv("ANOTHERMUD_LINKDEAD_ENABLED"); ok && v != "" {
		ld.Enabled = !(strings.EqualFold(v, "0") || strings.EqualFold(v, "false") || strings.EqualFold(v, "off"))
	}
	if d := envDurationOr("ANOTHERMUD_LINKDEAD_TIMEOUT", 0); d > 0 {
		ld.TimeoutSeconds = int(d / time.Second)
	}
	return config{
		Addr:                  envOr("ANOTHERMUD_ADDR", ":4000"),
		LogLevel:              strings.ToLower(envOr("ANOTHERMUD_LOG_LEVEL", "info")),
		LogFormat:             strings.ToLower(envOr("ANOTHERMUD_LOG_FORMAT", "text")),
		TickInterval:          envDurationOr("ANOTHERMUD_TICK_INTERVAL", 100*time.Millisecond),
		AutosaveInterval:      envDurationOr("ANOTHERMUD_AUTOSAVE_INTERVAL", 30*time.Second),
		IdleSweepInterval:     envDurationOr("ANOTHERMUD_IDLE_SWEEP_INTERVAL", 30*time.Second),
		LinkDeadSweepInterval: envDurationOr("ANOTHERMUD_LINKDEAD_SWEEP_INTERVAL", 30*time.Second),
		ContentDir:            envOr("ANOTHERMUD_CONTENT_DIR", "./content"),
		SaveDir:               envOr("ANOTHERMUD_SAVE_DIR", "./saves"),
		StartRoom:             world.RoomID(envOr("ANOTHERMUD_START_ROOM", "tapestry-core:town-square")),
		ColorDefault:          colorDefault(),
		LinkDead:              ld,
	}
}

// colorDefault honors the NO_COLOR convention.
func colorDefault() bool {
	if v, ok := os.LookupEnv("NO_COLOR"); ok && v != "" {
		return false
	}
	return true
}

func envDurationOr(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// bootSpawner adapts the runtime entity store + placement index to
// the pack.Spawner / pack.MobSpawner interfaces, letting room YAMLs
// declare items AND mobs that should exist at boot. Implemented here
// rather than in internal/ because it crosses package boundaries
// that don't otherwise meet (entities + item + mob + eventbus + pack);
// keeping the adapter at the composition root avoids inventing a
// shared adapter package for two users.
//
// Specs: world-rooms-movement §2.2 (room item placement),
// mobs-ai-spawning §3.1 (spawn placement + §3.1 step 10 event).
type bootSpawner struct {
	store        *entities.Store
	placement    *entities.Placement
	templates    *item.Templates
	mobTemplates *mob.Templates
	bus          *eventbus.Bus
}

// SpawnAndPlace looks up the item template, mints an instance via the
// store, and records its placement in roomID. Returns an error rather
// than panicking on a missing template, even though the loader's
// pre-validation should make that impossible — defense against a
// future caller bypassing the validation.
func (b *bootSpawner) SpawnAndPlace(_ context.Context, templateID string, roomID world.RoomID) error {
	tpl, err := b.templates.Get(item.TemplateID(templateID))
	if err != nil {
		return fmt.Errorf("template lookup: %w", err)
	}
	inst, err := b.store.Spawn(tpl)
	if err != nil {
		return fmt.Errorf("spawn: %w", err)
	}
	b.placement.Place(inst.ID(), roomID)
	return nil
}

// SpawnAndPlaceMob implements §3.1's spawn-placement pipeline for
// boot-time mob placement: resolve template, instantiate, place in
// room, emit the mob.spawned event. Steps mapped to spec:
//
//   - §3.1 step 1 + 3 (resolve template, instantiate) → mobTemplates
//     lookup + Store.SpawnMob
//   - §3.1 step 2 (resolve room) — implicit; the loader validates the
//     room exists before reaching here. A nil/missing room would
//     surface from Placement.Place, but Placement is a forgiving map
//     so we don't need a second world lookup here.
//   - §3.1 step 5 (set entity location + add to room) → Placement.Place
//   - §3.1 step 6 (track in entity store) → already done by SpawnMob
//   - §3.1 step 10 (emit mob.spawned event)
//
// Deferred (no consumer yet): step 4 stat derivation, step 7 equipment
// instantiation/equip, step 8 loot generation, step 9 ability
// proficiencies — all tracked under M6 follow-on slices.
func (b *bootSpawner) SpawnAndPlaceMob(ctx context.Context, templateID string, roomID world.RoomID) error {
	_, err := b.spawnMob(ctx, templateID, roomID)
	return err
}

// spawnMob is the shared implementation behind SpawnAndPlaceMob and
// the spawn.Spawner adapter. Returns the new entity id so the area
// reset manager can record the spawn against its rule.
func (b *bootSpawner) spawnMob(ctx context.Context, templateID string, roomID world.RoomID) (entities.EntityID, error) {
	tpl, err := b.mobTemplates.Get(mob.TemplateID(templateID))
	if err != nil {
		return "", fmt.Errorf("mob template lookup: %w", err)
	}
	inst, err := b.store.SpawnMob(tpl)
	if err != nil {
		return "", fmt.Errorf("spawn mob: %w", err)
	}
	b.placement.Place(inst.ID(), roomID)
	if b.bus != nil {
		b.bus.Publish(ctx, eventbus.MobSpawned{
			EntityID:   inst.ID(),
			RoomID:     roomID,
			TemplateID: templateID,
		})
	}
	return inst.ID(), nil
}

// bootSpawnerAdapter wraps *bootSpawner to satisfy spawn.Spawner.
// The adapter exists because spawn.Spawner's signature returns the
// new entity id (the spawn manager records it against the rule)
// while pack.MobSpawner just returns error (the pack loader doesn't
// care about the id). Keeping two adapter methods avoids forcing
// the pack interface to grow a return value it has no use for.
type bootSpawnerAdapter struct{ inner *bootSpawner }

func (a *bootSpawnerAdapter) Spawn(ctx context.Context, templateID string, roomID world.RoomID) (entities.EntityID, error) {
	return a.inner.spawnMob(ctx, templateID, roomID)
}

// presenceSource adapts *session.Manager + *world.World to
// spawn.PresenceSource. Per-area player count is derived by summing
// per-room occupancy across the rooms in the area; the manager
// has no native byArea index today and rebuilding one for one
// consumer would be premature optimization. With ≤10 rooms per area
// and once-per-second sampling the cost is negligible.
type presenceSource struct {
	mgr   *session.Manager
	world *world.World
}

func (p presenceSource) PlayerCountInArea(areaID world.AreaID) int {
	total := 0
	for _, r := range p.world.RoomsInArea(areaID) {
		total += len(p.mgr.PlayersInRoom(r.ID))
	}
	return total
}

// playerLookup adapts *session.Manager to ai.PlayerLookup. The
// adapter lives at the composition root for the same reason
// bootSpawner does: ai and session don't directly depend on each
// other, and stitching them here avoids inventing a shared package
// just to host the bridge.
type playerLookup struct{ mgr *session.Manager }

func (p playerLookup) PlayersInRoom(_ context.Context, room world.RoomID) []ai.PlayerView {
	pairs := p.mgr.PlayersInRoom(room)
	out := make([]ai.PlayerView, 0, len(pairs))
	for _, pr := range pairs {
		out = append(out, ai.PlayerView{ID: pr.ID, Name: pr.Name})
	}
	return out
}

func (p playerLookup) PlayerByID(_ context.Context, id string) (ai.PlayerView, bool) {
	a, ok := p.mgr.GetByPlayerID(id)
	if !ok {
		return ai.PlayerView{}, false
	}
	return ai.PlayerView{ID: a.PlayerID(), Name: a.PlayerName()}, true
}

// dispositionHook adapts *ai.Evaluator to command.DispositionHook
// (primitive-typed) so the command package doesn't have to import
// ai. Constructs ai.PlayerView from the primitives on each call.
type dispositionHook struct{ e *ai.Evaluator }

func (d dispositionHook) OnPlayerEnteredImmediate(ctx context.Context, playerID, playerName string, tags []string, room world.RoomID) {
	d.e.OnPlayerEnteredImmediate(ctx, ai.PlayerView{ID: playerID, Name: playerName, Tags: tags}, room)
}

func (d dispositionHook) OnPlayerEnteredDeferred(ctx context.Context, playerID, playerName string, tags []string, room world.RoomID) {
	d.e.OnPlayerEnteredDeferred(ctx, ai.PlayerView{ID: playerID, Name: playerName, Tags: tags}, room)
}

// combatLocator implements combat.Locator. Dispatches on the prefix
// embedded in every CombatantID: "mob:" → entities.Store, "player:"
// → session.Manager. A CombatantID with neither prefix (or an
// unknown one) misses — same as a missing combatant, which the round
// loop's §4.1 pre-flight handles by disengaging.
type combatLocator struct {
	entities *entities.Store
	sessions *session.Manager
}

func (l combatLocator) LookupCombatant(id combat.CombatantID) (combat.Combatant, bool) {
	s := string(id)
	switch {
	case strings.HasPrefix(s, combat.MobPrefix):
		entityID := entities.EntityID(s[len(combat.MobPrefix):])
		e, ok := l.entities.GetByID(entityID)
		if !ok {
			return nil, false
		}
		mob, ok := e.(*entities.MobInstance)
		if !ok {
			return nil, false
		}
		return mob, true
	case strings.HasPrefix(s, combat.PlayerPrefix):
		playerID := s[len(combat.PlayerPrefix):]
		return l.sessions.CombatantByPlayerID(playerID)
	}
	return nil, false
}

// loggingCombatSink is the M7.2 production sink — logs every combat
// event at Info so the engage/disengage path is observable in a
// default-configured server. M7.5 (mob.killed → spawn untrack) and
// M7.6 (combat.ended → flee cooldown clear) replace this with a real
// eventbus-backed adapter when those subscribers exist; at that point
// Info-level logging can drop to Debug since the event path itself
// becomes the observability surface.
type loggingCombatSink struct{ logger *slog.Logger }

func (s loggingCombatSink) OnEngagement(_ context.Context, e combat.Engagement) {
	s.logger.Info("combat.engagement",
		slog.String("attacker", string(e.AttackerID)),
		slog.String("target", string(e.TargetID)),
		slog.String("room", string(e.RoomID)))
}

func (s loggingCombatSink) OnCombatEnded(_ context.Context, e combat.CombatEnded) {
	s.logger.Info("combat.ended",
		slog.String("combatant", string(e.CombatantID)),
		slog.String("room", string(e.RoomID)))
}

func newLogger(cfg config) *slog.Logger {
	var lvl slog.Level
	switch cfg.LogLevel {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}

	var h slog.Handler
	if cfg.LogFormat == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h)
}

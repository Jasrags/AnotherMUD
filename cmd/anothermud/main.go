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
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/account"
	"github.com/Jasrags/AnotherMUD/internal/ai"
	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/conn/telnet"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/emote"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/mssp"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
	"github.com/Jasrags/AnotherMUD/internal/pack"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/portal"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/queststore"
	"github.com/Jasrags/AnotherMUD/internal/questwatch"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/scripting"
	"github.com/Jasrags/AnotherMUD/internal/server"
	"github.com/Jasrags/AnotherMUD/internal/session"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/spawn"
	"github.com/Jasrags/AnotherMUD/internal/tick"
	"github.com/Jasrags/AnotherMUD/internal/weather"
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
	// M14.5: engine baseline property registrations. The room loader
	// (and any future entity-side property validator) consults this
	// registry. Pack-scoped property registrations belong in their
	// owning feature's init code; the engine declares only what every
	// pack can rely on existing.
	if err := registerEngineBaselineProperties(registries.Properties); err != nil {
		return fmt.Errorf("register engine baseline properties: %w", err)
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
		races:        registries.Races,
		classes:      registries.Classes,
		bus:          bus,
	}
	// M17.1b: a sandboxed scripting.Engine is the ScriptCompiler the
	// pack loader uses to syntax-check each pack-supplied Lua file
	// at boot. M17.1c reuses the same Engine via a Runtime that
	// installs long-lived Sandboxes + bus subscriptions.
	scriptEngine := scripting.New(scripting.Options{})
	if err := pack.Load(ctx, cfg.ContentDir, nil, registries, spawner, spawner, scriptEngine); err != nil {
		return fmt.Errorf("loading content from %s: %w", cfg.ContentDir, err)
	}
	// M17.1c: bring scripts online. The Runtime spins up one Sandbox
	// per discovered script, installs the engine.* API on its LState,
	// and runs the script body to register handlers. Bus
	// subscriptions become live at the first engine.subscribe call.
	scriptRuntime := scripting.NewRuntime(scriptEngine, bus)
	if err := scriptRuntime.LoadRegistry(ctx, registries.Scripts); err != nil {
		return fmt.Errorf("loading scripts: %w", err)
	}
	defer scriptRuntime.Close()
	w := registries.World
	if _, err := w.Room(cfg.StartRoom); err != nil {
		return fmt.Errorf("starting room %q not in loaded world: %w", cfg.StartRoom, err)
	}

	// M10.2: compile the pack-loaded theme once and bind a shared,
	// read-only color renderer. connActor.Write routes every outbound
	// line through it (RenderAnsi/RenderPlain by the session color
	// flag). Compiling after Load means the renderer sees every pack's
	// theme overrides; no recompile happens at runtime.
	registries.Theme.Compile()
	colorRenderer := render.NewColorRenderer(registries.Theme)

	// M10.8: quest persistence store (writes players/<name>/quests.yaml,
	// loads on login). The quest service itself is constructed later
	// (after the progression / proficiency managers it grants rewards
	// through exist).
	questStore := queststore.NewStore(cfg.SaveDir, registries.Quests)

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
	// M9.6: register one enqueue verb per ACTIVE ability id so a
	// player can invoke an ability by its own name ("kick goblin",
	// "bless"). Passive abilities are hook-driven (§6) and never
	// queued, so they get no verb. A collision with an existing
	// builtin (e.g. a content ability literally named "cast" or a
	// movement direction) is skipped with a warning rather than
	// aborting boot — the generic `cast <ability>` verb still reaches
	// the shadowed ability.
	for _, ab := range registries.Abilities.All() {
		if ab.Type != progression.AbilityActive {
			continue
		}
		if err := cmds.Register(ab.ID, command.AbilityVerb(ab.ID)); err != nil {
			logging.From(ctx).Warn("skill-named verb skipped (keyword taken)",
				slog.String("ability", ab.ID), slog.Any("err", err))
		}
	}
	// Backfill help topics from command registration metadata (spec
	// commands-and-dispatch §8). Runs after builtins + ability verbs are
	// registered and after pack help loaded (pack.Load above), so authored
	// topics shadow generated ones and `help commands` lists every verb.
	command.GenerateHelpTopics(cmds, registries.Help)

	mgr := session.NewManager()

	// Store.SetRoomScan is intentionally NOT wired today: every spawned
	// item goes through Store.Spawn which auto-tracks, so the by-id
	// index is always the source of truth. The §4.2 step-2 fallback
	// becomes relevant only once items can enter the world without
	// passing through Spawn (e.g. external loader); add the bridge
	// when that path lands rather than fabricating one now.

	clk := clock.RealClock{}

	// M13.1c: notification queue substrate. Per spec §10 the
	// per-entity cap is 50 (matches tells inbox). The Manager and
	// Store mirror the queststore convention so future notifications
	// readers (M13.5 tells, M13.6 channels) see one shape.
	const notifQueueCap = 50
	notifStore := notifications.NewStore(cfg.SaveDir, notifQueueCap)
	notifMgr := notifications.NewManager(notifStore, notifQueueCap, clk)
	mgr.SetOnRemove(func(playerID string) {
		// Bridge session.Manager.Remove → notifications.Manager.Unregister.
		// Background ctx because session.Manager has no per-call ctx;
		// errors are logged inside Unregister via the notif store's
		// own logger.
		_ = notifMgr.Unregister(context.Background(), playerID)
	})

	// M13.6: channel registry + engine baseline. Baseline channels
	// are registered programmatically in v1; the pack-loaded
	// content path (content/core/channels/*.yaml) lands in M13.6b.
	// Per-channel scrollback is in-memory only for now.
	chatRegistry := chat.NewRegistry()
	chatScrollbacks := make(map[string]*chat.Scrollback)
	registerBaselineChannels(chatRegistry, chatScrollbacks)
	subscribers := chatSubscribers{mgr: mgr}
	scrollbackLookup := chatScrollbackLookup{m: chatScrollbacks}
	// Per-channel verbs are registered now so chat.Registry.All()
	// drives the command surface; missing here would mean a
	// configured channel has no verb.
	for _, ch := range chatRegistry.All() {
		if err := cmds.Register(ch.DisplayName, command.MakeChannelHandler(ch)); err != nil {
			return fmt.Errorf("register channel verb %q: %w", ch.DisplayName, err)
		}
	}

	// M13.7: emote registry + engine baseline. Like channels, the
	// pack-loaded YAML path is M13.7b. Per-emote verbs (and any
	// declared aliases) register into the command registry now.
	emoteRegistry := emote.NewRegistry()
	registerBaselineEmotes(emoteRegistry)
	for _, e := range emoteRegistry.All() {
		handler := command.MakeEmoteHandler(e)
		if err := cmds.Register(e.DisplayName, handler); err != nil {
			return fmt.Errorf("register emote verb %q: %w", e.DisplayName, err)
		}
		for _, alias := range e.Aliases {
			if err := cmds.Register(alias, handler); err != nil {
				return fmt.Errorf("register emote alias %q: %w", alias, err)
			}
		}
	}

	loop := tick.New(clk, cfg.TickInterval)
	if err := entities.RegisterTagSwap(loop, entityStore); err != nil {
		return fmt.Errorf("register entities tag-swap: %w", err)
	}
	autosaveInterval := autosaveTicks(cfg.TickInterval, cfg.AutosaveInterval)
	if err := loop.Register("autosave", autosaveInterval, func(ctx context.Context, n uint64) {
		mgr.SaveAll(ctx)
		// M13.1c: flush dirty notification queues alongside player
		// state so a crash loses at most one autosave-interval of
		// queued tells / channel posts (spec §6.3).
		notifMgr.SaveAll(ctx)
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

	// Prompt flush (ui-rendering-help §7.3 / session-lifecycle §3.5).
	// Every tick, render a fresh prompt for any session that has had
	// content sent since its last prompt, so the prompt settles after
	// output rather than mid-stream. Cadence 1 = end of every tick.
	if err := loop.Register("prompt-flush", 1, func(ctx context.Context, _ uint64) {
		mgr.FlushPrompts(ctx)
	}); err != nil {
		return fmt.Errorf("register prompt-flush tick: %w", err)
	}

	// M16.4a: GMCP Char.Vitals flush. Cadence 1 = every tick (100ms
	// at the default). Per-actor poll-and-diff inside FlushGmcpVitals
	// means a no-op when nothing changed; a real wire frame goes out
	// only when the snapshot differs. PD-3: at most one Char.Vitals
	// frame per session per tick.
	if err := loop.Register("gmcp-vitals-flush", 1, func(ctx context.Context, _ uint64) {
		mgr.FlushGmcpVitals(ctx)
	}); err != nil {
		return fmt.Errorf("register gmcp-vitals-flush tick: %w", err)
	}

	// M16.4c: GMCP Char.Items flush. Same cadence-1 poll-and-diff
	// shape as the vitals flusher. Per-location (inv / wear) shadows
	// inside FlushGmcpItems so a get/drop emits only the inv frame
	// and an equip emits only the wear frame.
	if err := loop.Register("gmcp-items-flush", 1, func(ctx context.Context, _ uint64) {
		mgr.FlushGmcpItems(ctx)
	}); err != nil {
		return fmt.Errorf("register gmcp-items-flush tick: %w", err)
	}

	// M16.4d: GMCP Char.Combat flush. Same cadence-1 shape; one
	// frame per session per tick when the combat snapshot (in-
	// combat flag + primary target name/HP) changes.
	if err := loop.Register("gmcp-combat-flush", 1, func(ctx context.Context, _ uint64) {
		mgr.FlushGmcpCombat(ctx)
	}); err != nil {
		return fmt.Errorf("register gmcp-combat-flush tick: %w", err)
	}

	// M16.4e: GMCP Char.Effects flush. Same cadence-1 shape; one
	// full Char.Effects list per session per tick when the active-
	// effect snapshot (id + remaining + flags + source) changes.
	if err := loop.Register("gmcp-effects-flush", 1, func(ctx context.Context, _ uint64) {
		mgr.FlushGmcpEffects(ctx)
	}); err != nil {
		return fmt.Errorf("register gmcp-effects-flush tick: %w", err)
	}

	// M16.4f: GMCP Char.Experience flush. Same cadence-1 shape; one
	// full per-track list per session per tick when any track's
	// (level, xp, xpnext) tuple changes.
	if err := loop.Register("gmcp-experience-flush", 1, func(ctx context.Context, _ uint64) {
		mgr.FlushGmcpExperience(ctx)
	}); err != nil {
		return fmt.Errorf("register gmcp-experience-flush tick: %w", err)
	}

	// M16.4h: GMCP Char.Login + Char.StatusVars (emit-once-per-
	// activation) + Char.Status (poll-and-diff). One tick handler
	// covers the three boot-identity packages — login + vars fire
	// on the first GMCP-active flush per session, Char.Status
	// follows the same per-tick diff cadence as Vitals.
	if err := loop.Register("gmcp-charstatus-flush", 1, func(ctx context.Context, _ uint64) {
		mgr.FlushGmcpCharStatus(ctx)
	}); err != nil {
		return fmt.Errorf("register gmcp-charstatus-flush tick: %w", err)
	}

	// M11.3: sustenance drain (spec economy-survival §4.4). The service
	// owns the value semantics + tier/multiplier helpers; the world-tick
	// handler decrements every logged-in player's pool at DrainCadence
	// and emits throttled hunger reminders. Constructed here so both the
	// handler and the session.Config seed path (below) share one
	// instance. Sustenance emits no bus events (§7), so unlike currency
	// it needs no sink bridge.
	sustenanceSvc := economy.NewSustenanceService(economy.DefaultSustenanceConfig())
	if err := loop.Register("sustenance-drain", sustenanceSvc.Config().DrainCadence, func(ctx context.Context, n uint64) {
		mgr.DrainSustenance(ctx, sustenanceSvc, n)
	}); err != nil {
		return fmt.Errorf("register sustenance-drain tick: %w", err)
	}

	// M11.4: rest service (spec economy-survival §5). The cancellable
	// change event bridges to the bus; loop.TickCount stamps sleep-start
	// for well-rested credit (consumed by the M11.5 regen heartbeat).
	// Wired into session.Config for the rest/sleep/wake verbs and into
	// the combat sink for combat-wake (set after combatSink is built).
	restSvc := economy.NewRestService(economy.DefaultRestConfig(), &restSink{bus: bus}, loop.TickCount)

	// M11.5: consumable service (spec economy-survival §6) — eat/drink/
	// use over the entity store, replenishing sustenance and emitting
	// item.consuming/consumed. Effect application is a decoupled
	// subscriber (§6.3); none is wired yet (no effect-id registry).
	consumableSvc := economy.NewConsumableService(entityStore, sustenanceSvc, &consumableSink{bus: bus})

	// M11.5: vitals-regen heartbeat (spec §4.3 × §5.5 + §5.7). Composes
	// the sustenance and rest multipliers + room healing_rate to heal
	// living, out-of-combat players below max HP. Pays the M9 "real
	// pools + regen" deferral.
	regenCfg := economy.DefaultRegenConfig()
	if err := loop.Register("vitals-regen", regenCfg.Cadence, func(ctx context.Context, _ uint64) {
		mgr.RegenTick(ctx, sustenanceSvc, restSvc, regenCfg)
	}); err != nil {
		return fmt.Errorf("register vitals-regen tick: %w", err)
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

	// M17.4: pump pack-script engine.schedule callbacks. Cadence one so
	// a delay of N ticks fires N ticks later; the Runtime's queue is
	// empty on most ticks (idle fast path).
	if err := loop.Register("scripting-schedule", 1, scriptRuntime.Tick); err != nil {
		return fmt.Errorf("register scripting schedule: %w", err)
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

	// M15.1c: door reset on area.tick (spec
	// world-rooms-movement §5.4). Mirrors the spawn manager's
	// area-tick subscriber; restores every door in the area to
	// its (DefaultClosed, DefaultLocked) state with paired
	// reverse-side sync. Logged at debug so a busy reset window
	// does not flood production output.
	bus.Subscribe(eventbus.EventAreaTick, func(ctx context.Context, ev eventbus.Event) {
		t, ok := ev.(eventbus.AreaTick)
		if !ok {
			return
		}
		if n := w.ResetDoorsInArea(t.AreaID); n > 0 {
			logging.From(ctx).Debug("doors reset on area tick",
				slog.String("event", "door.reset"),
				slog.String("area", string(t.AreaID)),
				slog.Int("transitions", n))
		}
	})

	// M15.2: portal service + auto-expiry on area.tick (spec §5.6).
	// The portal sink bridges service-level lifecycle hooks to the
	// engine bus so subscribers (renderer, AI, future scripting)
	// see portal.opened / portal.closed events. ExpireUpTo runs on
	// every area-tick using the event's monotonic TickCount as the
	// authoritative clock.
	portalSvc := portal.NewService(w, &portalBusSink{bus: bus})
	bus.Subscribe(eventbus.EventAreaTick, func(ctx context.Context, ev eventbus.Event) {
		t, ok := ev.(eventbus.AreaTick)
		if !ok {
			return
		}
		if n := portalSvc.ExpireUpTo(t.AreaID, t.TickCount); n > 0 {
			logging.From(ctx).Debug("portals expired on area tick",
				slog.String("event", "portal.expired"),
				slog.String("area", string(t.AreaID)),
				slog.Int("count", n))
		}
	})
	_ = portalSvc // retained for future verb wiring (M15.2b admin verb)

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

	// M15.4b: in-game clock + weather service. The clock ticks at
	// cadence 1 (every simulation tick); on every TicksPerGameHour
	// boundary it publishes time.period.change (if the period
	// transitioned) then time.hour.change. The weather service
	// subscribes to both, rolls per-area weather on the configured
	// hour interval, and broadcasts start/end ambience to eligible
	// rooms (spec world-rooms-movement §6). Spec defaults
	// (TicksPerGameHour=600 at the 100ms default tick = 1 in-game
	// hour per real minute) apply; both are knobs M15.4b₂c could
	// expose via cfg if needed.
	//
	// Dedicated RNG so the weather goroutine (the time.hour.change
	// subscriber path) doesn't share state with the combat RNG —
	// math/rand/v2.Rand is not concurrent-safe.
	weatherRNG := rand.New(rand.NewPCG(uint64(clk.Now().UnixNano()), 1))
	weatherSvc := weather.New(weather.Config{
		Registry:    registries.Weather,
		World:       w,
		Bus:         bus,
		Broadcaster: mgr,
		Roller:      weatherRNG,
	})
	bus.Subscribe(eventbus.EventTimeHourChange, func(ctx context.Context, ev eventbus.Event) {
		hc, ok := ev.(eventbus.TimeHourChange)
		if !ok {
			return
		}
		weatherSvc.HourChanged(ctx, hc.Hour)
	})
	bus.Subscribe(eventbus.EventTimePeriodChange, func(ctx context.Context, ev eventbus.Event) {
		pc, ok := ev.(eventbus.TimePeriodChange)
		if !ok {
			return
		}
		weatherSvc.PeriodChanged(ctx, pc.Period)
	})
	gameClock := gameclock.New(gameclock.Config{Bus: bus})
	if err := loop.Register("game-clock", 1, func(ctx context.Context, _ uint64) {
		gameClock.Tick(ctx)
	}); err != nil {
		return fmt.Errorf("register game-clock tick: %w", err)
	}

	// Combat manager (spec combat §2, M7.2). Locator dispatches on the
	// CombatantID prefix: mob: → entities.Store, player: →
	// session.Manager. Sink is log-only today; M7.5 (mob.killed →
	// spawn untrack) and M7.6 (combat.ended → flee cooldown clear)
	// will replace it with a real eventbus-backed adapter when those
	// subscribers exist.
	combatLocator := combatLocator{entities: entityStore, sessions: mgr, placement: placement}

	// combatantName resolves a bare entity id (player.Save.ID or mob
	// store id) to its live display name, trying the mob then player
	// namespace. The ability events carry the bare TargetID and a
	// placeholder TargetName (the resolver has no name registry —
	// "id doubles as name", M9.4a), so the ability renderers + side-
	// effect handler use this to show a real name. Falls back to the
	// id when the target has despawned/logged out mid-pulse.
	combatantName := func(bareID string) string {
		if bareID == "" {
			return ""
		}
		// Defensive: the ability events carry bare ids by contract
		// (connActor.EntityID + CurrentTarget both return prefix-
		// stripped ids), but EntityIDOf is idempotent on an already-
		// bare id, so normalizing first means a future prefixed id
		// can't silently miss both namespace probes.
		bareID = combat.EntityIDOf(combat.CombatantID(bareID))
		if c, ok := combatLocator.LookupCombatant(combat.NewMobCombatantID(bareID)); ok {
			return c.Name()
		}
		if c, ok := combatLocator.LookupCombatant(combat.NewPlayerCombatantID(bareID)); ok {
			return c.Name()
		}
		return bareID
	}

	// combatSink wires the production death flow (spec combat §6).
	// OnVitalDepleted publishes the cancellable death.check; if not
	// cancelled, publishes kill (+ mob.killed for mob victims) and
	// calls combatMgr.DisengageAll. combatMgr is back-pointed via a
	// setter after construction so the sink and manager can reference
	// each other without an init cycle.
	combatSink := &productionCombatSink{
		logger:   logging.From(ctx),
		bus:      bus,
		locator:  combatLocator,
		entities: entityStore,
		sessions: mgr,
		nameOf:   combatantName,
	}
	combatTags := combatTagSource{
		entities: entityStore,
		world:    w,
		mgr:      mgr,
	}
	// M8.2: progression manager backed by the pack-loaded track
	// registry. The bus-backed sink bridges progression.EventSink
	// to eventbus.Publish — keeps progression itself free of the
	// eventbus → entities import edge that would close a cycle.
	progressionMgr := progression.NewManager(registries.Tracks, &progressionSink{bus: bus})

	// M9.1: proficiency manager — per-entity ability prof+cap maps
	// (spec abilities-and-effects §3). Bound to the AbilityRegistry
	// so AbilityName lookups resolve and DefaultCap per ability is
	// honored at Learn time. Satisfies the AbilityProficiency seam
	// the TrainingManager declares (training.go), making the M8.6
	// train + practice paths functional end-to-end.
	proficiencyMgr := progression.NewProficiencyManager(
		registries.Abilities,
		progression.DefaultProficiencyConfig(),
	)

	// M11.1: currency service (spec economy-survival §2). Bus-bridging
	// sink mirrors alignmentSink so economy stays free of an eventbus
	// import. Constructed before the quest service so the gold reward
	// granter can route through it.
	currencySvc := economy.NewCurrencyService(&currencySink{bus: bus})

	// M11.2: shop service over the item registry + entity store +
	// currency, with the global economy defaults and a bus-bridging
	// cancellable sink for shop.buy/shop.sell.
	shopSvc := economy.NewShopService(registries.Items, entityStore, currencySvc, economy.DefaultEconomyConfig(), &shopSink{bus: bus})

	// M10.7-M10.10: quest service, now that the reward dependencies
	// (manager, progression, proficiency, item templates, entity store,
	// currency) all exist. Rewards grant XP / abilities / items / gold on
	// completion; the event sink logs the lifecycle (a typed event-bus
	// bridge can land when a consumer needs it). The watcher routes
	// mob-killed / item-picked-up / item-given / player-moved into
	// objective progress.
	questSvc := quest.NewService(quest.Config{
		Registry: registries.Quests,
		Persist:  questStore,
		Rewards:  session.NewQuestRewards(mgr, progressionMgr, proficiencyMgr, registries.Items, entityStore, currencySvc),
		Events:   questLogSink{logger: logging.From(ctx)},
	})
	questWatcher := questwatch.New(questSvc, entityStore)
	// §7.2 quest_grant item side channel: resolve a picker's id to a
	// quest.Player (connActor) so picking up a quest_grant item
	// auto-accepts its quest.
	questWatcher.SetItemGrant(func(playerID string) (quest.Player, bool) {
		a, ok := mgr.GetByPlayerID(playerID)
		if !ok {
			return nil, false
		}
		return a, true
	})
	// M14.6: room-side quest_grant. Reads the destination room's
	// quest_grant property and auto-accepts. Reuses the item-grant
	// player resolver above.
	questWatcher.SetRoomGrant(func(roomID world.RoomID) string {
		r, err := registries.World.Room(roomID)
		if err != nil {
			return ""
		}
		s, _ := r.PropertyString("quest_grant")
		return s
	})
	questWatcher.Subscribe(bus)

	// M9.2: effect manager — per-entity active effects (spec
	// abilities-and-effects §5). Resolver walks the session
	// manager's playerID index; sink bridges the applied /
	// removed / expired transitions onto the eventbus. Manager
	// is constructed before this block so the resolver can
	// capture it directly.
	effectMgr := progression.NewEffectManager(
		session.NewEffectTargetResolver(mgr, entityStore),
		&effectSink{bus: bus},
	)

	// M14.2: item.consumed → effect.Registry lookup → effectMgr.Apply.
	// Closes the m11-5 deferral. Items declaring effect_id (e.g. the
	// bless-potion) get their effect applied to the consumer after the
	// consume path emits ItemConsumed. Empty effect_id is a legacy /
	// non-effect consumable (e.g., trail-ration) — silently skipped.
	bus.Subscribe(eventbus.EventItemConsumed, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.ItemConsumed)
		if !ok || e.EffectID == "" {
			return
		}
		tpl, ok := registries.Effects.Get(e.EffectID)
		if !ok {
			logging.From(ctx).Warn("item.consumed: unknown effect_id",
				slog.String("event", "effect.consumed.unknown"),
				slog.String("actor", string(e.ActorID)),
				slog.String("item", e.ItemName),
				slog.String("effect_id", e.EffectID))
			return
		}
		// Per-event duration override: a potion can declare a longer
		// or shorter duration than the template default by setting
		// effect_duration in its properties. 0 (the default) means
		// "use template duration".
		if e.EffectDuration != 0 {
			tpl.Duration = e.EffectDuration
		}
		effectMgr.Apply(ctx, string(e.ActorID), tpl, string(e.ActorID), "")
	})

	// M9.3/M9.4: per-entity action queue + pulse-delay cooldown
	// tracker. The M9.4 ability phase pops from the queue each pulse
	// and records cooldowns into the tracker; the validation pipeline
	// reads the tracker. Both are handed to the session Config so
	// logout drops the entity's working set. No enqueue path exists
	// until the M9.6 verb surface, so the queue stays empty in
	// production today — the wiring is live but dormant.
	actionQueueMgr := progression.NewActionQueueManager(progression.ActionQueueConfig{})
	pulseDelayTracker := progression.NewPulseDelayTracker()

	// M8.5: alignment manager + bus-bridging sink. Config uses
	// the engine defaults (-1000/+1000 bounds, ±500 bucket
	// thresholds, history capacity 20) per the M8.5 ROADMAP
	// choice. NewAlignmentManager panics on a malformed config so
	// the composition root catches misconfiguration at boot.
	alignmentMgr := progression.NewAlignmentManager(
		progression.DefaultAlignmentConfig(),
		&alignmentSink{bus: bus},
		clk.Now,
	)

	// M8.4: class-side level-up subscribers. Path processor grants
	// abilities (logs as unknown until M9 abilities ship). Stat
	// growth subscriber rolls per-level dice + credits trains.
	//
	// Both use a Roller backed by math/rand/v2's default source so
	// every server process gets independent randomness — combat
	// already follows the same convention for hit rolls. The
	// progression.Roller is structurally the same as combat.Roller
	// (IntN(int) int); we pass a closure wrapper since rand.IntN
	// is a package-level function.
	classPath := &progression.ClassPathProcessor{
		Classes:  registries.Classes,
		Granter:  proficiencyMgr,
		Notifier: notifierAdapter{mgr: mgr},
	}
	trainsCrediter := trainsAdapter{mgr: mgr}
	growthRoller := stdRoller{}

	bus.Subscribe(eventbus.EventLevelUp, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.LevelUp)
		if !ok {
			return
		}
		actor, ok := mgr.GetByPlayerID(e.EntityID)
		if !ok {
			return // player logged out between cascade emit and dispatch
		}
		classID := actor.ClassID()
		if classID == "" {
			return
		}
		classPath.Apply(ctx, e.EntityID, classID, e.Track, e.NewLevel)
		if cls, ok := registries.Classes.Get(classID); ok {
			// Spec §4.6 step 2: no track gate — stat growth runs on
			// every level-up regardless of track. (M8.4 ROADMAP
			// acceptance criterion: documented decision.)
			progression.ApplyStatGrowth(ctx, cls, actor.StatBlock(), growthRoller, trainsCrediter, e.EntityID)
		}
	})

	bus.Subscribe(eventbus.EventCharacterCreated, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.CharacterCreated)
		if !ok {
			return
		}
		if e.ClassID == "" {
			return
		}
		// Spec §4.5 step 3: character-created is treated as level 1
		// with no track gate. Pass empty trackName so Apply
		// short-circuits the gate check.
		classPath.Apply(ctx, e.EntityID, e.ClassID, "", 1)
	})

	// M9.6: render ability resolution outcomes to players. The
	// ability phase + resolver emit ability.used / .missed / .fizzled
	// into the bus (M9.4b) but nothing surfaced them to the caster
	// until the verb path landed. SourceID is the bare player entity
	// id; only players have an enqueue path today, so a GetByPlayerID
	// miss means the source was a mob/logged-out and we stay silent.
	bus.Subscribe(eventbus.EventAbilityUsed, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.AbilityUsed)
		if !ok {
			return
		}
		actor, ok := mgr.GetByPlayerID(e.SourceID)
		if !ok {
			return
		}
		verb, verbThird := abilityVerbWords(e.Category)
		selfMsg := fmt.Sprintf("You %s %s", verb, e.AbilityName)
		roomMsg := fmt.Sprintf("%s %s %s", actor.Name(), verbThird, e.AbilityName)
		// TargetID == "" is a self-cast (buff/heal-self); only name a
		// target when the ability resolved against another entity.
		if e.TargetID != "" && e.TargetID != e.SourceID {
			tn := combatantName(e.TargetID)
			selfMsg += " on " + tn
			roomMsg += " on " + tn
		}
		_ = actor.Write(ctx, selfMsg+".")
		if room := actor.Room(); room != nil {
			mgr.SendToRoom(ctx, room.ID, roomMsg+".", actor.PlayerID())
		}
	})
	bus.Subscribe(eventbus.EventAbilityMissed, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.AbilityMissed)
		if !ok {
			return
		}
		actor, ok := mgr.GetByPlayerID(e.SourceID)
		if !ok {
			return
		}
		selfMsg := fmt.Sprintf("You try %s", e.AbilityName)
		roomMsg := fmt.Sprintf("%s tries %s", actor.Name(), e.AbilityName)
		if e.TargetID != "" && e.TargetID != e.SourceID {
			tn := combatantName(e.TargetID)
			selfMsg += " on " + tn
			roomMsg += " on " + tn
		}
		_ = actor.Write(ctx, selfMsg+", but misses.")
		if room := actor.Room(); room != nil {
			mgr.SendToRoom(ctx, room.ID, roomMsg+", but misses.", actor.PlayerID())
		}
	})
	bus.Subscribe(eventbus.EventAbilityFizzled, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.AbilityFizzled)
		if !ok {
			return
		}
		actor, ok := mgr.GetByPlayerID(e.SourceID)
		if !ok {
			return
		}
		// Fizzles are a private validation failure — caster only, no
		// room broadcast.
		_ = actor.Write(ctx, fizzleMessage(e.AbilityName, e.Reason))
	})

	combatCooldowns := combat.NewFleeCooldowns()
	combatMgr := combat.NewManagerWith(combat.ManagerConfig{
		Locator:   combatLocator,
		Sink:      combatSink,
		Tags:      combatTags,
		Cooldowns: combatCooldowns,
	})
	combatSink.mgr = combatMgr
	combatSink.rest = restSvc
	// Wire the AI dispatcher's combat-state gate now that combatMgr
	// exists. Without this the wander behavior keeps moving mobs
	// between combat rounds and the auto-attack pre-flight then
	// disengages on different-room — see ai/dispatcher.go gate.
	aiDispatcher.AttachCombat(combatMgr)

	// M7.5: mob.aggro → combat.Engage closes the M6.5 deferred. The
	// disposition evaluator emits MobAggro for every fresh hostile
	// reaction; combat.Manager.Engage is idempotent on duplicates
	// (already-engaged returns false silently per §2.1), so a
	// re-triggered aggro after a brief disengage simply re-engages.
	bus.Subscribe(eventbus.EventMobAggro, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.MobAggro)
		if !ok {
			return
		}
		attacker := combat.NewMobCombatantID(string(e.MobID))
		target := combat.NewPlayerCombatantID(e.PlayerID)
		combatMgr.Engage(ctx, attacker, target, e.RoomID)
	})

	// M7.5: mob.killed → entity untrack closes M6.6's deferred death-
	// driven purge. The spawn tracker's Purge predicate calls
	// entities.Store.GetByID; an untracked mob fails that check on the
	// next area-tick reset and the rule's missing count is recomputed,
	// so the kill drives a respawn without spawn needing a direct
	// mob.killed subscription. Subscription is process-lifetime, same
	// convention as the spawn manager's own area.tick subscription.
	bus.Subscribe(eventbus.EventMobKilled, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.MobKilled)
		if !ok {
			return
		}
		if err := entityStore.Untrack(e.MobID); err != nil {
			// Already untracked — benign (a future caller may have
			// untracked the mob via a corpse pipeline). Logged at
			// debug so the kill path stays quiet on the happy path.
			logging.From(ctx).Debug("mob.killed untrack noop",
				slog.String("mob_id", string(e.MobID)),
				slog.Any("err", err))
		}
		// Also clear the mob's room placement so a subsequent
		// look/exits in the room doesn't list the corpse.
		placement.Remove(e.MobID)
	})

	// M7.6 follow-up: combat.kill → player respawn closes M7.5
	// deferred #1 ("dead-but-walking player"). Combat spec §6.4 says
	// player death recovery is owned by a separate feature
	// subscribing to vital-depleted / kill. This subscriber heals
	// the player to 1 HP, moves them to the configured start room,
	// announces the death in the room they fell + the respawn in
	// the room they wake in, and emits player.respawned for any
	// downstream listener (renderers, XP-loss policies).
	//
	// Today's policy (chosen explicitly, see m7-6 review responses):
	//   - Respawn room: ANOTHERMUD_START_ROOM (same as new-character).
	//   - Respawn HP: 1 (penalty for dying, still playable).
	//   - Gear: kept on the corpse-less respawn for now. Gear-loss
	//     is a policy decision that lands with the corpse pipeline.
	bus.Subscribe(eventbus.EventKill, func(ctx context.Context, ev eventbus.Event) {
		k, ok := ev.(eventbus.Kill)
		if !ok {
			return
		}
		if !strings.HasPrefix(k.VictimID, combat.PlayerPrefix) {
			return // mob deaths handled by the mob.killed subscriber above.
		}
		playerID := k.VictimID[len(combat.PlayerPrefix):]
		actor, ok := mgr.GetByPlayerID(playerID)
		if !ok {
			// Player disconnected between vital-depleted and kill —
			// nothing to respawn into; restorePlayerVitals on next
			// login will floor HP to 1.
			return
		}
		// Heal FIRST so a failed start-room lookup below still leaves
		// the player alive (in their death room, at 1 HP) instead of
		// stuck at HP=0 with no teleport. Vitals.Heal(1) on a HP=0
		// combatant yields HP=1 (Heal clamps to [0, max]).
		actor.Vitals().Heal(1)

		dst, err := w.Room(cfg.StartRoom)
		if err != nil {
			logging.From(ctx).Error("player respawn: start room missing — player stays in death room at 1 HP",
				slog.String("player", actor.PlayerName()),
				slog.String("start_room", string(cfg.StartRoom)),
				slog.Any("err", err))
			_ = actor.Write(ctx, "You stir back to life where you fell.")
			return
		}

		// Death-room broadcast — visible to anyone still standing
		// over the corpse. Sent BEFORE SetRoom so the room id we
		// announce in is the room they actually died in.
		if actor.PlayerName() != "" {
			mgr.SendToRoom(ctx, k.RoomID,
				actor.PlayerName()+"'s body fades from the world.",
				playerID)
		}

		// Tell the dying player what happened, then move them. The
		// two writes flank SetRoom so the messages render in the
		// correct order even if RenderRoom races a re-prompt.
		_ = actor.Write(ctx, "Everything goes black...")
		actor.SetRoom(dst)
		_ = actor.Write(ctx, "...and you wake, dazed, in another place.")

		// Respawn-room broadcast.
		if actor.PlayerName() != "" {
			mgr.SendToRoom(ctx, dst.ID,
				actor.PlayerName()+" appears, dazed and gasping.",
				playerID)
		}

		// Publish player.respawned for any future listener. From is
		// the room they died in; To is the respawn room. MaxHP is
		// snapshotted at publish time so listeners can compute the
		// post-respawn fraction without a follow-up Vitals lookup.
		_, maxHP := actor.Vitals().Snapshot()
		bus.Publish(ctx, eventbus.PlayerRespawned{
			PlayerID:   playerID,
			PlayerName: actor.PlayerName(),
			From:       k.RoomID,
			To:         dst.ID,
			RespawnHP:  1,
			MaxHP:      maxHP,
		})
	})

	// Combat heartbeat (spec combat §3, M7.3). Round skeleton at the
	// configured cadence. Phases (ability / auto-attack / effects /
	// wimpy) wire in M7.4-M7.6 + M9; the bucket lands now so those
	// milestones drop into a stable shape rather than ship the round
	// loop and the first phase in the same commit.
	// M7.4 wires AutoAttack; M7.5/M7.6 (Effects/Wimpy) and M9
	// (Ability) follow. The RNG is process-lifetime, seeded once at
	// boot. *math/rand/v2.Rand satisfies combat.Roller via its IntN
	// method; the locator already implements both combat.Locator and
	// combat.RoomLocator from the §4.1 RoomOf addition above.
	//
	// CONCURRENCY: *math/rand/v2.Rand is NOT safe for concurrent use
	// (see Roller doc in internal/combat/damage.go). Heartbeat.runPhase
	// serializes every phase callback on the tick goroutine, so the
	// shared RNG is safe today. A future phase that rolls from a
	// separate goroutine MUST either pass its own Roller or wrap this
	// one in a mutex; do not silently share the Rand pointer.
	combatRNG := rand.New(rand.NewPCG(uint64(clk.Now().UnixNano()), 0))
	// M9.5: passive-ability evaluator for the auto-attack §4.2/§4.3
	// hooks. Shares combatRNG (same single-goroutine tick context as
	// the swing rolls) and the proficiency manager (read + §6.3 gain).
	// *progression.PassiveResolver satisfies combat.PassiveEvaluator
	// structurally — no adapter needed.
	passiveResolver := progression.NewPassiveResolver(
		registries.Abilities, proficiencyMgr, proficiencyMgr, combatRNG,
	)
	autoAttackPhase := combat.NewAutoAttack(combat.AutoAttackConfig{
		Locator:     combatLocator,
		RoomLocator: combatLocator,
		Sink:        combatSink,
		Roller:      combatRNG,
		Passives:    passiveResolver,
	})
	// M7.6 wimpy phase — fires §5.2 flee when a combatant's HP%
	// drops to or below its WimpyThreshold property. Shares the same
	// FleeConfig the M7.6d `flee` verb uses; the verb path constructs
	// its own config from the same dependencies.
	fleeMover := combatMover{sessions: mgr, placement: placement, world: w, broadcaster: mgr, bus: bus}
	fleeBusAdapter := combatFleeBus{bus: bus}
	fleeCfg := combat.FleeConfig{
		Mgr:           combatMgr,
		Locator:       combatLocator,
		RoomLocator:   combatLocator,
		Rooms:         w,
		Mover:         fleeMover,
		Sink:          combatSink,
		Bus:           fleeBusAdapter,
		Cooldowns:     combatCooldowns,
		Tags:          combatTags,
		Rand:          combatRNG,
		CooldownTicks: cadenceTicks(cfg.TickInterval, cfg.FleeCooldown),
	}
	wimpyPhase := combat.NewWimpy(fleeCfg)

	// M9.4: ability-resolution phase. The driver pops each
	// combatant's queued actions, runs the §4.3 validation pipeline,
	// and resolves the first valid entry per pulse (§4.5).
	//
	// Three host seams bridge the bare entity ids the progression
	// layer uses to the combat-side combatant namespace:
	//   - abilitySources: combatant id → ResolutionSource. Players
	//     resolve to their connActor; mobs have no queue/AI enqueue
	//     path in M9.4 so they are never ability sources yet.
	//   - abilityTargets: bare entity id → exists? Resolves players
	//     and mobs via the shared combat locator (try both prefixes).
	//   - abilityTargetHP: bare entity id → current HP, for the §4.5
	//     post-hit death check. Works for mobs (they carry Vitals)
	//     even though effect-stat installation on mobs is deferred
	//     (the stats↔entities import-cycle, m8-1 #1).
	abilitySources := progression.ResolutionSourceLookupFunc(func(combatantID string) (progression.ResolutionSource, bool) {
		if !strings.HasPrefix(combatantID, combat.PlayerPrefix) {
			return nil, false // only players queue abilities in M9.4.
		}
		a, ok := mgr.GetByPlayerID(combat.EntityIDOf(combat.CombatantID(combatantID)))
		if !ok {
			return nil, false
		}
		return a, true
	})
	abilityTargets := progression.TargetLookupFunc(func(entityID string) bool {
		if _, ok := combatLocator.LookupCombatant(combat.NewPlayerCombatantID(entityID)); ok {
			return true
		}
		_, ok := combatLocator.LookupCombatant(combat.NewMobCombatantID(entityID))
		return ok
	})
	abilityTargetHP := progression.TargetHPLookupFunc(func(entityID string) (int, bool) {
		if c, ok := combatLocator.LookupCombatant(combat.NewPlayerCombatantID(entityID)); ok {
			return c.Vitals().Current(), true
		}
		if c, ok := combatLocator.LookupCombatant(combat.NewMobCombatantID(entityID)); ok {
			return c.Vitals().Current(), true
		}
		return 0, false
	})
	abilitySink := &abilitySink{bus: bus}

	// M9.6b: ability side-effect handler (damage / heal) + the
	// ability-death bridge.
	//
	// Pre-parse each ability's dice once at boot so the hot path (an
	// ability hit) parses nothing and a malformed NdM±K surfaces here
	// as a warning instead of silently dealing no damage.
	type abilityDice struct {
		damage    combat.DiceExpr
		hasDamage bool
		heal      combat.DiceExpr
		hasHeal   bool
	}
	abilityDiceByID := make(map[string]abilityDice)
	for _, ab := range registries.Abilities.All() {
		var d abilityDice
		if ab.DamageDice != "" {
			if expr, err := combat.ParseDice(ab.DamageDice); err != nil {
				logging.From(ctx).Warn("ability damage dice unparsable; ability deals no damage",
					slog.String("ability", ab.ID), slog.String("dice", ab.DamageDice), slog.Any("err", err))
			} else {
				d.damage, d.hasDamage = expr, true
			}
		}
		if ab.HealDice != "" {
			if expr, err := combat.ParseDice(ab.HealDice); err != nil {
				logging.From(ctx).Warn("ability heal dice unparsable; ability heals nothing",
					slog.String("ability", ab.ID), slog.String("dice", ab.HealDice), slog.Any("err", err))
			} else {
				d.heal, d.hasHeal = expr, true
			}
		}
		if d.hasDamage || d.hasHeal {
			abilityDiceByID[ab.ID] = d
		}
	}

	// resolveCombatant maps a bare entity id to its live Combatant +
	// prefixed CombatantID (mob namespace first, then player), the
	// same try-both shape the ability target lookups use.
	//
	// INVARIANT: the ability events carry bare ids — SourceID/TargetID
	// from the resolver (connActor.EntityID + CurrentTarget are
	// prefix-stripped), VictimID/KillerID likewise. EntityIDOf is
	// idempotent on a bare id, so the leading normalize is a cheap
	// guard that keeps the two namespace probes correct even if a
	// future ResolutionSource handed us a prefixed id.
	resolveCombatant := func(bareID string) (combat.Combatant, combat.CombatantID, bool) {
		bareID = combat.EntityIDOf(combat.CombatantID(bareID))
		mobID := combat.NewMobCombatantID(bareID)
		if c, ok := combatLocator.LookupCombatant(mobID); ok {
			return c, mobID, true
		}
		playerID := combat.NewPlayerCombatantID(bareID)
		if c, ok := combatLocator.LookupCombatant(playerID); ok {
			return c, playerID, true
		}
		return nil, "", false
	}

	// ability.used side-effect handler. Runs synchronously inside the
	// resolver's §4.5 step-8 emit (tick goroutine), BEFORE the
	// resolver's step-9 HP probe — so damage it applies is visible to
	// the post-hit death check, which then emits ability.vital_depleted
	// (bridged below). Shares combatRNG + combatSink with auto-attack
	// on the same goroutine, so no new concurrency is introduced.
	//
	// Dispatch is by the §4.5 handler token: "damage" rolls DamageDice
	// onto the target's HP and emits a combat.Hit (so ability damage
	// flows through the same sink/renderer as a weapon swing); "heal"
	// rolls HealDice onto the target-or-self HP. An empty/unknown token
	// is a no-op — effect-only abilities (bless) already had their
	// payload applied by the resolver in step 7.
	bus.Subscribe(eventbus.EventAbilityUsed, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.AbilityUsed)
		if !ok {
			return
		}
		dice := abilityDiceByID[e.AbilityID]
		switch e.HandlerToken {
		case "damage":
			if !dice.hasDamage || e.TargetID == "" {
				return
			}
			target, targetCID, ok := resolveCombatant(e.TargetID)
			if !ok {
				return
			}
			amount := dice.damage.Roll(combatRNG)
			if amount < 1 {
				amount = 1
			}
			if _, wasAlive := target.Vitals().ApplyDamageIfAlive(amount); !wasAlive {
				return // already a corpse; another source owns the death emit.
			}
			// Source is a player by construction (only players enqueue);
			// resolveCombatant normalizes + probes both namespaces. The
			// fallback below uses the normalized bare id so a logged-out
			// source can't produce a double-prefixed CombatantID.
			attackerBare := combat.EntityIDOf(combat.CombatantID(e.SourceID))
			attackerCID := combat.NewPlayerCombatantID(attackerBare)
			attackerName := attackerBare
			if c, acid, ok := resolveCombatant(e.SourceID); ok {
				attackerCID, attackerName = acid, c.Name()
			}
			room, _ := combatLocator.RoomOf(targetCID)
			// Emit through the combat sink so ability damage shares the
			// hit log (and any future hit renderer) with weapon swings.
			// The §4.5 step-9 probe owns the death emit, so we do NOT
			// emit VitalDepleted here (avoids a double death signal).
			combatSink.OnHit(ctx, combat.Hit{
				AttackerID:   attackerCID,
				TargetID:     targetCID,
				AttackerName: attackerName,
				TargetName:   target.Name(),
				WeaponName:   e.AbilityName,
				Damage:       amount,
				DamageType:   combat.DamageTypePhysical,
				RoomID:       room,
			})
		case "heal":
			if !dice.hasHeal {
				return
			}
			targetID := e.TargetID
			if targetID == "" {
				targetID = e.SourceID // self-heal
			}
			target, _, ok := resolveCombatant(targetID)
			if !ok {
				return
			}
			amount := dice.heal.Roll(combatRNG)
			if amount < 1 {
				amount = 1
			}
			target.Vitals().Heal(amount)
			logging.From(ctx).Info("ability.heal",
				slog.String("source", e.SourceID),
				slog.String("target", targetID),
				slog.String("ability", e.AbilityID),
				slog.Int("amount", amount))
		}
	})

	// ability.vital_depleted → combat death bridge. The resolver emits
	// this on its own topic (M9.4b) to avoid a progression→combat edge;
	// nothing consumed it until ability damage existed. Translate the
	// bare ids back to prefixed CombatantIDs and feed the SAME
	// cancellable death-check/Kill flow auto-attack uses, so an ability
	// kill respawns players / untracks mobs identically.
	bus.Subscribe(eventbus.EventAbilityVitalDepleted, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.AbilityVitalDepleted)
		if !ok {
			return
		}
		victim, victimCID, ok := resolveCombatant(e.VictimID)
		if !ok {
			return // victim gone between the probe and now; nothing to kill.
		}
		var killerCID combat.CombatantID
		if _, kcid, ok := resolveCombatant(e.KillerID); ok {
			killerCID = kcid
		}
		room, _ := combatLocator.RoomOf(victimCID)
		combatSink.OnVitalDepleted(ctx, combat.VitalDepleted{
			VictimID:   victimCID,
			VictimName: victim.Name(),
			AttackerID: killerCID,
			Vital:      combat.VitalHP,
			RoomID:     room,
		})
	})

	abilityPipeline := progression.NewValidationPipeline(
		registries.Abilities, proficiencyMgr, effectMgr, pulseDelayTracker, abilityTargets,
	)
	abilityResolver := progression.NewAbilityResolver(
		progression.DefaultResolutionConfig(),
		proficiencyMgr, // ProficiencyReader (hit chance + cap)
		proficiencyMgr, // ProficiencyMutator (gain)
		pulseDelayTracker,
		effectMgr,
		abilityTargetHP,
		abilitySink,
		combatRNG, // shared single-goroutine RNG (see CONCURRENCY note above)
	)
	abilityPhase := progression.NewAbilityPhaseDriver(
		actionQueueMgr, abilityPipeline, abilityResolver, abilitySources, abilitySink,
	)

	combatHeartbeat := combat.NewHeartbeat(combatMgr, combat.Phases{
		Ability:    abilityPhase,
		AutoAttack: autoAttackPhase,
		Wimpy:      wimpyPhase,
	})
	combatCadence := cadenceTicks(cfg.TickInterval, cfg.CombatCadence)
	// M9.2 #1 / spec abilities-and-effects §5.4: advance active-effect
	// durations one pulse per combat round. EffectManager.Tick is
	// global (decrements every entity's effects + expires the
	// zero-counter ones, reversing their stat mods) so it runs as its
	// own loop handler rather than the per-combatant heartbeat Effects
	// slot — which stays reserved for future per-entity DoT/HoT that
	// needs in-round death checks. Registered BEFORE combat-tick so an
	// effect applied during this round's ability phase is not
	// decremented in the same pulse. Runs at the combat cadence
	// regardless of whether anyone is engaged, so a buff cast in combat
	// still expires after combat ends.
	if err := loop.Register("effect-tick", combatCadence, func(ctx context.Context, _ uint64) {
		effectMgr.Tick(ctx)
	}); err != nil {
		return fmt.Errorf("register effect tick: %w", err)
	}
	if err := loop.Register("combat-tick", combatCadence, combatHeartbeat.Tick); err != nil {
		return fmt.Errorf("register combat tick: %w", err)
	}

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
		slog.Duration("combat_cadence", cfg.CombatCadence),
		slog.Duration("autosave_interval", cfg.AutosaveInterval),
		slog.String("content_dir", cfg.ContentDir),
		slog.String("save_dir", cfg.SaveDir),
		slog.String("start_room", string(cfg.StartRoom)),
		slog.Bool("color_default", cfg.ColorDefault),
	)

	handler := session.Handler(session.Config{
		World:         w,
		Commands:      cmds,
		Players:       players,
		Manager:       mgr,
		Items:         entityStore,
		Placement:     placement,
		Contents:      contents,
		Templates:     registries.Items,
		Slots:         registries.Slots,
		Bus:           bus,
		Disposition:   dispositionHook{e: evaluator},
		Combat:        combatMgr,
		CombatLocator: combatLocator,
		Flee: func(ctx context.Context, c combat.CombatantID) combat.FleeOutcome {
			return combat.Flee(ctx, c, fleeCfg)
		},
		// M17.3: the `reload` verb re-discovers pack Lua from disk and
		// hot-swaps the scripting runtime — script-only, so world.World
		// and the content registries are untouched. DiscoverScripts'
		// compile-check rejects a syntax-broken edit before Reload tears
		// the live scripts down.
		ReloadScripts: func(ctx context.Context) (int, error) {
			fresh, err := pack.DiscoverScripts(ctx, cfg.ContentDir, nil, scriptEngine)
			if err != nil {
				return 0, err
			}
			if err := scriptRuntime.Reload(ctx, fresh); err != nil {
				return 0, err
			}
			return fresh.Len(), nil
		},
		Progression: progressionMgr,
		Training: progression.NewTrainingManager(
			progression.DefaultTrainingConfig(),
			registries.Races,
			session.NewTrainerSource(mgr, placement, entityStore),
			proficiencyMgr,
		),
		Proficiency:     proficiencyMgr,
		Abilities:       registries.Abilities,
		Effects:         effectMgr,
		ActionQueue:     actionQueueMgr,
		PulseDelay:      pulseDelayTracker,
		Races:           registries.Races,
		Classes:         registries.Classes,
		Alignment:       alignmentMgr,
		DefaultRace:     cfg.DefaultRace,
		StartID:         cfg.StartRoom,
		ColorEnabled:    cfg.ColorDefault,
		Render:          colorRenderer,
		Help:            registries.Help,
		Quests:          questSvc,
		QuestStore:      questStore,
		Notifications:   notifMgr,
		TellResolver:    session.TellResolver{Manager: mgr, Players: players},
		ChatRegistry:    chatRegistry,
		ChatSubscribers: subscribers,
		ChatScrollbacks: scrollbackLookup,
		Currency:        currencySvc,
		Shop:            shopSvc,
		Sustenance:      sustenanceSvc,
		Rest:            restSvc,
		Consumable:      consumableSvc,
		// M12.3: interactive character-creation wizard built from the
		// race/class registries. Nil when neither is populated → the §2
		// "no flow → immediate commit" path.
		CreationFlow: session.NewCreationFlow(registries.Races, registries.Classes),
		Clock:        clk,
		Flood:        session.DefaultFloodConfig(),
		LinkDead:     linkDeadCfg,
		// M15.4b₂b: per-look weather ambience. The closure binds
		// weather.Service.Ambience; RenderRoom appends its output
		// after the room description in eligible rooms.
		Ambience: weatherSvc.Ambience,
		Login: login.Config{
			Accounts:        accounts,
			Players:         players,
			DefaultLocation: string(cfg.StartRoom),
		},
	})
	// M16.2: MSSP variable table for MUD-listing crawlers. Static
	// fields describe the server identity; PLAYERS and UPTIME are
	// resolved through closures so each crawler request sees live
	// state. GMCP stays false until M16.3 lands the transport.
	startTime := clk.Now()
	hostname, port := splitHostPort(cfg.Addr)
	msspCfg := &mssp.Config{
		Name:      "AnotherMUD",
		Codebase:  "AnotherMUD/dev",
		Contact:   "https://github.com/Jasrags/AnotherMUD",
		Hostname:  hostname,
		Port:      port,
		Language:  "English",
		Family:    "Custom",
		Gameplay:  []string{"Hack and Slash", "Roleplaying"},
		Classes:   true,
		Races:     true,
		Levels:    true,
		Equipment: true,
		ANSI:      true,
		UTF8:      true,
		Players:   func() int { return mgr.Count() },
		Uptime:    func() int64 { return int64(clk.Now().Sub(startTime).Seconds()) },
	}
	srv := &server.Server{
		Handler:       handler,
		TelnetOptions: []telnet.Option{telnet.WithMssp(msspCfg)},
	}

	// M16.5: optional parallel WebSocket listener. ANOTHERMUD_WS_ADDR
	// empty disables the listener entirely (telnet-only deployment).
	// The HTTP server runs in its own goroutine and shuts down via
	// ctx cancellation; Server.wg drains both telnet and websocket
	// handlers before Serve returns.
	var wsHTTP *http.Server
	if cfg.WsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle(cfg.WsPath, server.NewWebSocketHandler(srv, server.WebSocketOptions{
			OriginPatterns:     cfg.WsOriginPatterns,
			InsecureSkipVerify: cfg.WsInsecureSkipVerify,
		}))
		// ReadHeaderTimeout bounds the upgrade-handshake header
		// read so a Slowloris-style stall can't pin the listener.
		// ReadTimeout / WriteTimeout are deliberately unset:
		// coder/websocket's Accept clears the conn deadline after
		// the upgrade, so a 30s ReadTimeout would silently kill
		// the long-lived post-upgrade session. The websocket
		// library handles its own ping/pong keepalive.
		wsHTTP = &http.Server{
			Addr:              cfg.WsAddr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			logging.From(ctx).Info("ws listener starting",
				slog.String("addr", cfg.WsAddr),
				slog.String("path", cfg.WsPath))
			if err := wsHTTP.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logging.From(ctx).Warn("ws listener exited",
					slog.Any("err", err))
			}
		}()
		go func() {
			<-ctx.Done()
			shutdownCtx, cancelSD := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancelSD()
			_ = wsHTTP.Shutdown(shutdownCtx)
		}()
	}

	serveErr := srv.Serve(ctx, ln)

	// Final flush so anyone still in-world has their state committed
	// even if they didn't disconnect cleanly. Uses a fresh ctx that is
	// not already cancelled.
	flushCtx, cancelFlush := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	mgr.SaveAll(flushCtx)
	notifMgr.SaveAll(flushCtx)
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
	WsAddr                string
	WsPath                string
	WsOriginPatterns      []string
	WsInsecureSkipVerify  bool
	LogLevel              string
	LogFormat             string
	TickInterval          time.Duration
	CombatCadence         time.Duration
	FleeCooldown          time.Duration
	AutosaveInterval      time.Duration
	IdleSweepInterval     time.Duration
	LinkDeadSweepInterval time.Duration
	ContentDir            string
	SaveDir               string
	StartRoom             world.RoomID
	DefaultRace           string
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
	wsOrigins := []string{}
	if v := envOr("ANOTHERMUD_WS_ORIGINS", ""); v != "" {
		for _, part := range strings.Split(v, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				wsOrigins = append(wsOrigins, trimmed)
			}
		}
	}
	wsInsecure := false
	if v, ok := os.LookupEnv("ANOTHERMUD_WS_INSECURE_SKIP_VERIFY"); ok && v != "" {
		wsInsecure = !(strings.EqualFold(v, "0") || strings.EqualFold(v, "false") || strings.EqualFold(v, "off"))
	}
	return config{
		Addr:                  envOr("ANOTHERMUD_ADDR", ":4000"),
		WsAddr:                envOr("ANOTHERMUD_WS_ADDR", ""),
		WsPath:                envOr("ANOTHERMUD_WS_PATH", "/mud"),
		WsOriginPatterns:      wsOrigins,
		WsInsecureSkipVerify:  wsInsecure,
		LogLevel:              strings.ToLower(envOr("ANOTHERMUD_LOG_LEVEL", "info")),
		LogFormat:             strings.ToLower(envOr("ANOTHERMUD_LOG_FORMAT", "text")),
		TickInterval:          envDurationOr("ANOTHERMUD_TICK_INTERVAL", 100*time.Millisecond),
		CombatCadence:         envDurationOr("ANOTHERMUD_COMBAT_CADENCE", 3*time.Second),
		FleeCooldown:          envDurationOr("ANOTHERMUD_FLEE_COOLDOWN", 15*time.Second),
		AutosaveInterval:      envDurationOr("ANOTHERMUD_AUTOSAVE_INTERVAL", 30*time.Second),
		IdleSweepInterval:     envDurationOr("ANOTHERMUD_IDLE_SWEEP_INTERVAL", 30*time.Second),
		LinkDeadSweepInterval: envDurationOr("ANOTHERMUD_LINKDEAD_SWEEP_INTERVAL", 30*time.Second),
		ContentDir:            envOr("ANOTHERMUD_CONTENT_DIR", "./content"),
		SaveDir:               envOr("ANOTHERMUD_SAVE_DIR", "./saves"),
		StartRoom:             world.RoomID(envOr("ANOTHERMUD_START_ROOM", "tapestry-core:town-square")),
		DefaultRace:           envOr("ANOTHERMUD_DEFAULT_RACE", "human"),
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
	races        *progression.RaceRegistry
	classes      *progression.ClassRegistry
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
	// M8.3: apply racial flags + starting alignment from the
	// race registry. Unknown race id is a no-op (spec §3.1 mob
	// spawn fail-silent) — the content author will notice via the
	// missing tags rather than a boot failure.
	if rid := inst.RaceID(); rid != "" && b.races != nil {
		if race, ok := b.races.Get(rid); ok {
			inst.ApplyRacialFlags(race.RacialFlags, race.StartingAlignment)
			// Republish to the tag index now that racial flags have
			// been merged into the mob's tag slice. SpawnMob's Track
			// call captured only the template's tags; without this
			// Retag, GetByTag("common-tongue") on a freshly-spawned
			// dwarf would miss. Cheap O(num_tags) sweep.
			if err := b.store.Retag(inst.ID()); err != nil {
				logging.From(ctx).Warn("mob spawn: retag after racial flags failed",
					slog.String("mob", string(inst.ID())),
					slog.Any("err", err))
			}
		} else {
			// Warn (not debug): a template referencing a missing
			// race id is almost always an authoring error — a
			// rename that didn't propagate, content removed
			// between releases. Spec §3.1 mandates fail-silent on
			// missing mob *templates*, but races aren't templates;
			// the diagnostic should be loud enough that a content
			// author running with default log level (info) sees
			// it without combing through debug output.
			logging.From(ctx).Warn("mob spawn: unknown race id; mob spawned without racial flags",
				slog.String("mob", string(inst.ID())),
				slog.String("template", templateID),
				slog.String("race", rid))
		}
	}

	// M14.3: class-bound stat growth (mobs-ai-spawning §3.2). If
	// the template declares class + non-zero level, apply
	// averageDice(growth) × level to each stat-growth entry under
	// the srckey.ClassGrowth source key. Vitals.SetMax fires
	// automatically through the M14.1 listener; spec §3.2 then
	// requires current vitals reset to max so the level-applied HP
	// is immediately available — Heal-to-full handles that.
	if tpl.Class != "" && tpl.Level > 0 && b.classes != nil {
		if cls, ok := b.classes.Get(tpl.Class); ok {
			if progression.ApplyMobClassGrowth(inst.StatBlock(), cls, tpl.Level) {
				if v := inst.Vitals(); v != nil {
					_ = v.Heal(v.Max()) // top up to whatever the new max is
				}
			}
		} else {
			logging.From(ctx).Warn("mob spawn: unknown class id; mob spawned without growth",
				slog.String("mob", string(inst.ID())),
				slog.String("template", templateID),
				slog.String("class", tpl.Class),
				slog.Int("level", tpl.Level))
		}
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
	infos := p.mgr.PlayersInRoom(room)
	out := make([]ai.PlayerView, 0, len(infos))
	for _, info := range infos {
		out = append(out, ai.PlayerView{
			ID:           info.ID,
			Name:         info.Name,
			Tags:         info.Tags,
			Alignment:    info.Alignment,
			Bucket:       info.Bucket,
			HasAlignment: info.Bucket != "",
		})
	}
	return out
}

func (p playerLookup) PlayerByID(_ context.Context, id string) (ai.PlayerView, bool) {
	a, ok := p.mgr.GetByPlayerID(id)
	if !ok {
		return ai.PlayerView{}, false
	}
	tag := a.AlignmentTag()
	view := ai.PlayerView{
		ID:        a.PlayerID(),
		Name:      a.PlayerName(),
		Tags:      a.Tags(),
		Alignment: a.Alignment(),
	}
	switch tag {
	case progression.TagAlignmentEvil:
		view.Bucket = string(progression.BucketEvil)
		view.HasAlignment = true
	case progression.TagAlignmentGood:
		view.Bucket = string(progression.BucketGood)
		view.HasAlignment = true
	case progression.TagAlignmentNeutral:
		view.Bucket = string(progression.BucketNeutral)
		view.HasAlignment = true
	}
	return view, ok
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
	entities  *entities.Store
	sessions  *session.Manager
	placement *entities.Placement
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

// RoomOf implements combat.RoomLocator. Mob rooms come from the
// Placement index (the authority for "where is this entity right
// now"); player rooms come from session.Manager.RoomOfPlayer
// (authoritative for online players, returns false for offline or
// mid-login). Both branches return ("", false) for unknown ids,
// which the auto-attack pre-flight (§4.1) treats as "different
// room" — pairwise-disengage and skip.
func (l combatLocator) RoomOf(id combat.CombatantID) (world.RoomID, bool) {
	s := string(id)
	switch {
	case strings.HasPrefix(s, combat.MobPrefix):
		entityID := entities.EntityID(s[len(combat.MobPrefix):])
		return l.placement.RoomOf(entityID)
	case strings.HasPrefix(s, combat.PlayerPrefix):
		playerID := s[len(combat.PlayerPrefix):]
		return l.sessions.RoomOfPlayer(playerID)
	}
	return "", false
}

// productionCombatSink is the runtime combat.EventSink. M7.2 shipped
// this as a log-only sink (loggingCombatSink) so the engage/disengage
// path was observable in the default-configured server; M7.5 promotes
// it to the canonical death-flow handler per spec combat §6.
//
// OnVitalDepleted is the entry point for the death pipeline:
//
//  1. Resolve killer attribution (§6.2: explicit attacker > victim's
//     primary target > empty).
//  2. Resolve victim shape (mob vs player; mob carries template id for
//     mob.killed payload + tracker untrack).
//  3. Publish the cancellable DeathCheck (§6.1). If a listener cancels,
//     stop — the canceller is responsible for restoring HP.
//  4. Publish Kill, and MobKilled if the victim is a mob (§6.3).
//  5. Call combatMgr.DisengageAll(victim) to clean both sides of every
//     engagement (§6.3 step 3).
//
// All other event methods stay log-only; M7.6 will use OnCombatEnded
// to clear flee cooldowns.
type productionCombatSink struct {
	logger   *slog.Logger
	bus      *eventbus.Bus
	locator  combatLocator
	entities *entities.Store
	mgr      *combat.Manager // back-pointer set after construction
	// M10 combat rendering: sessions sends combat messages to player
	// participants; nameOf resolves a combatant id to a display name.
	sessions *session.Manager
	nameOf   func(bareID string) string
	// rest is the M11.4 rest service. OnEngagement forcibly wakes a
	// resting/sleeping target (spec §5.4). nil disables combat wake.
	rest *economy.RestService
}

// tell sends msg to the combatant if it is an online player (M10 combat
// rendering). Mobs and offline players are skipped.
func (s *productionCombatSink) tell(ctx context.Context, id combat.CombatantID, msg string) {
	if s.sessions == nil || !strings.HasPrefix(string(id), combat.PlayerPrefix) {
		return
	}
	if a, ok := s.sessions.GetByPlayerID(combat.EntityIDOf(id)); ok {
		_ = a.Write(ctx, msg)
	}
}

// announce sends msg to everyone in room except the listed combatants
// (the second-person participants, who get their own tell). Mob ids in
// exclude are harmless — they never match a session.
func (s *productionCombatSink) announce(ctx context.Context, room world.RoomID, msg string, exclude ...combat.CombatantID) {
	if s.sessions == nil {
		return
	}
	ex := make([]string, 0, len(exclude))
	for _, c := range exclude {
		ex = append(ex, combat.EntityIDOf(c))
	}
	s.sessions.SendToRoom(ctx, room, msg, ex...)
}

func (s *productionCombatSink) OnEngagement(ctx context.Context, e combat.Engagement) {
	s.logger.Info("combat.engagement",
		slog.String("attacker", string(e.AttackerID)),
		slog.String("target", string(e.TargetID)),
		slog.String("room", string(e.RoomID)))

	// M11.4 combat wake (spec §5.4): a resting/sleeping target is
	// forcibly woken when engaged, bypassing the cancellable check. Only
	// players carry rest state; a non-player target (mob) resolves to no
	// session and is skipped. ForceAwake returns true only if the target
	// was actually resting/sleeping, so the jolt message fires once.
	if s.rest != nil && s.sessions != nil && strings.HasPrefix(string(e.TargetID), combat.PlayerPrefix) {
		if a, ok := s.sessions.GetByPlayerID(combat.EntityIDOf(e.TargetID)); ok {
			if s.rest.ForceAwake(ctx, a, "combat") {
				_ = a.Write(ctx, "You are jolted awake as combat begins!")
			}
		}
	}
}

func (s *productionCombatSink) OnCombatEnded(_ context.Context, e combat.CombatEnded) {
	s.logger.Info("combat.ended",
		slog.String("combatant", string(e.CombatantID)),
		slog.String("room", string(e.RoomID)))
}

func (s *productionCombatSink) OnHit(ctx context.Context, e combat.Hit) {
	s.logger.Info("combat.hit",
		slog.String("attacker", string(e.AttackerID)),
		slog.String("target", string(e.TargetID)),
		slog.String("weapon", e.WeaponName),
		slog.Int("damage", e.Damage),
		slog.String("damage_type", e.DamageType),
		slog.Bool("critical", e.IsCritical),
		slog.String("room", string(e.RoomID)))

	// M10 rendering: second-person to each player participant, third to
	// the room. damage numbers are now visible (closes m9-6 #2).
	an := s.nameOf(string(e.AttackerID))
	tn := s.nameOf(string(e.TargetID))
	crit := ""
	if e.IsCritical {
		crit = " <danger>A critical hit!</danger>"
	}
	s.tell(ctx, e.AttackerID, fmt.Sprintf("<good>You hit %s for %d damage.</good>%s", tn, e.Damage, crit))
	s.tell(ctx, e.TargetID, fmt.Sprintf("<danger>%s hits you for %d damage.</danger>", an, e.Damage))
	s.announce(ctx, e.RoomID, fmt.Sprintf("%s hits %s.", an, tn), e.AttackerID, e.TargetID)
}

func (s *productionCombatSink) OnMiss(ctx context.Context, e combat.Miss) {
	s.logger.Info("combat.miss",
		slog.String("attacker", string(e.AttackerID)),
		slog.String("target", string(e.TargetID)),
		slog.String("weapon", e.WeaponName),
		slog.Bool("fumble", e.IsFumble),
		slog.String("room", string(e.RoomID)))

	an := s.nameOf(string(e.AttackerID))
	tn := s.nameOf(string(e.TargetID))
	second, third := "miss", "misses" // attacker-perspective, observer-perspective
	if e.IsFumble {
		second, third = "fumble and miss", "fumbles and misses"
	}
	s.tell(ctx, e.AttackerID, fmt.Sprintf("You %s %s.", second, tn))
	s.tell(ctx, e.TargetID, fmt.Sprintf("%s %s you.", an, third))
	s.announce(ctx, e.RoomID, fmt.Sprintf("%s %s %s.", an, third, tn), e.AttackerID, e.TargetID)
}

func (s *productionCombatSink) OnEvade(_ context.Context, e combat.Evade) {
	s.logger.Info("combat.evade",
		slog.String("attacker", string(e.AttackerID)),
		slog.String("target", string(e.TargetID)),
		slog.String("ability", e.AbilityName),
		slog.String("room", string(e.RoomID)))
}

func (s *productionCombatSink) OnVitalDepleted(ctx context.Context, e combat.VitalDepleted) {
	s.logger.Info("combat.vital_depleted",
		slog.String("victim", string(e.VictimID)),
		slog.String("attacker", string(e.AttackerID)),
		slog.String("vital", e.Vital),
		slog.String("room", string(e.RoomID)))

	// Only HP-depletion is a death today. A future stamina/mana
	// depletion event would land here as a separate code path.
	if e.Vital != combat.VitalHP {
		return
	}

	// §6.2 killer attribution: explicit attacker on the depletion
	// event wins; otherwise fall back to the victim's primary
	// target at the moment of death; otherwise empty.
	killerID := e.AttackerID
	if killerID == "" && s.mgr != nil {
		if pt, ok := s.mgr.PrimaryTargetOf(e.VictimID); ok {
			killerID = pt
		}
	}
	killerName := ""
	if killerID != "" {
		if c, ok := s.locator.LookupCombatant(killerID); ok {
			killerName = c.Name()
		}
	}

	// Victim resolution. A player victim is never untracked here
	// (player teardown owns its own lifecycle); a mob victim drives
	// MobKilled emission so the spawn manager's untrack subscriber
	// fires.
	isMob := strings.HasPrefix(string(e.VictimID), combat.MobPrefix)
	var (
		mobEntityID entities.EntityID
		mobTemplate string
	)
	if isMob {
		mobEntityID = entities.EntityID(string(e.VictimID)[len(combat.MobPrefix):])
		if ent, ok := s.entities.GetByID(mobEntityID); ok {
			if mi, ok := ent.(*entities.MobInstance); ok {
				mobTemplate = string(mi.TemplateID())
			}
		}
	}

	// §6.1 cancellable death-check. A listener that cancels MUST
	// restore the victim to a non-dead state (heal to 1 HP) before
	// the next round — the engine does not undo damage on cancel.
	check := eventbus.NewDeathCheck(
		string(e.VictimID),
		e.VictimName,
		string(killerID),
		killerName,
		e.RoomID,
		isMob,
		mobTemplate,
	)
	if s.bus != nil && s.bus.PublishCancellable(ctx, check) {
		s.logger.Info("combat.death_cancelled",
			slog.String("victim", string(e.VictimID)),
			slog.String("killer", string(killerID)))
		// Spec §6.1 places the heal-to-non-dead obligation on the
		// canceller. If the listener cancelled but did NOT restore
		// HP, the victim is now permanently stuck: still in combat
		// lists, HP=0, ApplyDamageIfAlive returns wasAlive=false on
		// every future swing so VitalDepleted never re-emits and
		// this death pipeline never re-runs. The engine can't undo
		// damage on the canceller's behalf (we don't know what HP
		// to restore to), but we can surface evidence so operators
		// chase the buggy listener instead of the symptom.
		if c, ok := s.locator.LookupCombatant(e.VictimID); ok && c.Vitals().IsDead() {
			s.logger.Warn("combat.death_cancel_left_corpse",
				slog.String("victim", string(e.VictimID)),
				slog.String("killer", string(killerID)),
				slog.String("hint", "cancelling listener must heal victim to >0 HP per combat spec §6.1"))
		}
		return
	}

	// §6.3 emit kill (+ mob.killed) and disengage-all.
	if s.bus != nil {
		s.bus.Publish(ctx, eventbus.Kill{
			VictimID:   string(e.VictimID),
			VictimName: e.VictimName,
			KillerID:   string(killerID),
			KillerName: killerName,
			RoomID:     e.RoomID,
		})
		if isMob {
			s.bus.Publish(ctx, eventbus.MobKilled{
				MobID:      mobEntityID,
				MobName:    e.VictimName,
				TemplateID: mobTemplate,
				KillerID:   string(killerID),
				KillerName: killerName,
				RoomID:     e.RoomID,
			})
		}
	}

	// M10 rendering: announce the kill. The slayer (if a player) gets a
	// second-person line; the room gets the death. A player victim's own
	// "you wake, dazed" message comes from the respawn flow, and a mob
	// killer never matches a session, so neither is double-messaged.
	if killerID != "" {
		s.tell(ctx, killerID, fmt.Sprintf("<good>You have slain %s!</good>", e.VictimName))
	}
	s.announce(ctx, e.RoomID, fmt.Sprintf("<danger>%s is dead!</danger>", e.VictimName), killerID)

	if s.mgr != nil {
		s.mgr.DisengageAll(ctx, e.VictimID, e.RoomID)
	}
}

// combatTagSource implements combat.TagSource (cluster 2). Room tags
// come from world.Room.Tags (the §2.1 safe-room engage refusal); entity
// tags from the entities.Store (mob side) or the session manager's
// connActor.Tags() (player side — racial flags + alignment bucket).
type combatTagSource struct {
	entities *entities.Store
	world    *world.World
	mgr      *session.Manager
}

func (t combatTagSource) RoomHasTag(roomID world.RoomID, tag string) bool {
	if t.world == nil {
		return false
	}
	r, err := t.world.Room(roomID)
	if err != nil {
		return false
	}
	return r.HasTag(tag)
}

func (t combatTagSource) EntityHasTag(id combat.CombatantID, tag string) bool {
	s := string(id)
	switch {
	case strings.HasPrefix(s, combat.MobPrefix):
		entityID := entities.EntityID(s[len(combat.MobPrefix):])
		e, ok := t.entities.GetByID(entityID)
		if !ok {
			return false
		}
		return hasTag(e.Tags(), tag)
	case strings.HasPrefix(s, combat.PlayerPrefix):
		// Player tags are the connActor's Tags() surface (racial flags
		// + alignment bucket). A content author can thus mark a race
		// no-flee / no-kill; a general persisted player-tag slice +
		// admin grant remains deferred to the role system (m7-6 #2).
		if t.mgr == nil {
			return false
		}
		a, ok := t.mgr.GetByPlayerID(s[len(combat.PlayerPrefix):])
		if !ok {
			return false
		}
		return hasTag(a.Tags(), tag)
	}
	return false
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// combatMover implements combat.Mover. Players move via
// connActor.SetRoom (which keeps session presence indexes in sync);
// mobs move via the placement index. Both paths announce the
// arrival/departure in the rooms so other occupants see "X flees!"
// rather than "X disappears."
type combatMover struct {
	sessions    *session.Manager
	placement   *entities.Placement
	world       *world.World
	broadcaster *session.Manager // session.Manager implements SendToRoom
	bus         *eventbus.Bus
}

func (m combatMover) Move(ctx context.Context, id combat.CombatantID, dst *world.Room) bool {
	if dst == nil {
		return false
	}
	s := string(id)
	switch {
	case strings.HasPrefix(s, combat.MobPrefix):
		entityID := entities.EntityID(s[len(combat.MobPrefix):])
		from, ok := m.placement.RoomOf(entityID)
		if !ok {
			return false
		}
		m.placement.Place(entityID, dst.ID)
		// Announce the flee to both rooms so witnesses see it.
		if m.broadcaster != nil {
			m.broadcaster.SendToRoom(ctx, from, "Something bolts away!", "")
			m.broadcaster.SendToRoom(ctx, dst.ID, "Something bolts in, panting.", "")
		}
		return true
	case strings.HasPrefix(s, combat.PlayerPrefix):
		playerID := s[len(combat.PlayerPrefix):]
		actor, ok := m.sessions.GetByPlayerID(playerID)
		if !ok {
			return false
		}
		from, _ := m.sessions.RoomOfPlayer(playerID)
		if m.broadcaster != nil && actor.PlayerName() != "" {
			m.broadcaster.SendToRoom(ctx, from,
				actor.PlayerName()+" flees!", playerID)
		}
		actor.SetRoom(dst)
		if m.broadcaster != nil && actor.PlayerName() != "" {
			m.broadcaster.SendToRoom(ctx, dst.ID,
				actor.PlayerName()+" arrives, panting.", playerID)
		}
		// Publish player.moved so disposition + future hooks fire on
		// flee-driven moves the same way they do on verb-driven ones.
		if m.bus != nil {
			m.bus.Publish(ctx, eventbus.PlayerMoved{
				PlayerID: playerID,
				From:     from,
				To:       dst.ID,
			})
		}
		return true
	}
	return false
}

// combatFleeBus implements combat.FleeBus by translating the three
// narrow emission methods into eventbus.Publish calls. Keeps the
// combat package free of the eventbus import.
type combatFleeBus struct {
	bus *eventbus.Bus
}

func (b combatFleeBus) EmitFlee(ctx context.Context, id combat.CombatantID, name string, from, to world.RoomID, dir string) {
	if b.bus == nil {
		return
	}
	b.bus.Publish(ctx, eventbus.Flee{
		EntityID: string(id), EntityName: name,
		From: from, To: to, Direction: dir,
	})
}

func (b combatFleeBus) EmitFleePrevented(ctx context.Context, id combat.CombatantID, name string, room world.RoomID) {
	if b.bus == nil {
		return
	}
	b.bus.Publish(ctx, eventbus.FleePrevented{
		EntityID: string(id), EntityName: name, RoomID: room,
	})
}

func (b combatFleeBus) EmitFleeFailed(ctx context.Context, id combat.CombatantID, name string, room world.RoomID, reason string) {
	if b.bus == nil {
		return
	}
	// Translate combat's reason strings to eventbus's. They are
	// the same values today, but the indirection insulates against
	// either side renaming independently.
	switch reason {
	case combat.FleeReasonNoExits:
		reason = eventbus.FleeFailedNoExits
	case combat.FleeReasonUnknownRoom:
		reason = eventbus.FleeFailedUnknownRoom
	}
	b.bus.Publish(ctx, eventbus.FleeFailed{
		EntityID: string(id), EntityName: name, RoomID: room, Reason: reason,
	})
}

// progressionSink bridges progression.EventSink to eventbus.Bus.
// Lives in the composition root for the same reason
// productionCombatSink does — progression itself does not import
// eventbus (closing progression → eventbus → entities → progression
// otherwise). Each method propagates the granter's ctx through to
// bus.Publish so subscribers see the request-scoped logger fields
// and respect cancellation.
type progressionSink struct {
	bus *eventbus.Bus
}

func (s *progressionSink) OnXPGained(ctx context.Context, entityID, track string, amount, newTotal int64, source string) {
	s.bus.Publish(ctx, eventbus.XPGained{
		EntityID: entityID,
		Track:    track,
		Amount:   amount,
		NewTotal: newTotal,
		Source:   source,
	})
}

func (s *progressionSink) OnLevelUp(ctx context.Context, entityID, track string, oldLevel, newLevel int) {
	s.bus.Publish(ctx, eventbus.LevelUp{
		EntityID: entityID,
		Track:    track,
		OldLevel: oldLevel,
		NewLevel: newLevel,
	})
}

func (s *progressionSink) OnXPLost(ctx context.Context, entityID, track string, amount, newTotal int64) {
	s.bus.Publish(ctx, eventbus.XPLost{
		EntityID: entityID,
		Track:    track,
		Amount:   amount,
		NewTotal: newTotal,
	})
}

// alignmentSink bridges progression.AlignmentSink to eventbus.Bus.
// Lives in the composition root for the same reason
// progressionSink does — progression must not import eventbus.
//
// OnAlignmentShiftCheck constructs the cancellable event, publishes
// it via PublishCancellable, then reads the (possibly rewritten)
// SuggestedDelta off the event after dispatch. Listeners that flip
// the cancel flag short-circuit the manager; a non-cancel listener
// can call RewriteDelta to change the applied magnitude.
type alignmentSink struct {
	bus *eventbus.Bus
}

func (s *alignmentSink) OnAlignmentShiftCheck(ctx context.Context, entityID, reason string, suggested int) (int, bool) {
	if s.bus == nil {
		return suggested, false
	}
	ev := eventbus.NewAlignmentShiftCheck(entityID, reason, suggested)
	cancelled := s.bus.PublishCancellable(ctx, ev)
	return ev.SuggestedDelta(), cancelled
}

func (s *alignmentSink) OnAlignmentShifted(ctx context.Context, entityID, reason string, oldValue, newValue, actualDelta int, bucketChanged bool) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.AlignmentShifted{
		EntityID:      entityID,
		Reason:        reason,
		OldValue:      oldValue,
		NewValue:      newValue,
		ActualDelta:   actualDelta,
		BucketChanged: bucketChanged,
	})
}

// effectSink bridges progression.EffectSink to eventbus.Bus.
// Same composition-root pattern as alignmentSink / progressionSink
// (progression must not import eventbus). Each callback maps 1:1
// to an eventbus payload; nil bus is a silent no-op so tests that
// wire the manager without a bus still exercise the rest of the
// pipeline.
// questLogSink is the M10.10b quest.EventSink: it logs the quest
// lifecycle so completions/abandons are observable. A typed event-bus
// bridge (quests.md §9) can replace this when a consumer (achievement /
// GMCP) needs the events; for now there is none.
type questLogSink struct {
	logger *slog.Logger
}

func (s questLogSink) Started(e quest.StartedEvent) {
	s.logger.Info("quest started", slog.String("event", "quest.started"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID))
}

func (s questLogSink) ObjectiveAdvanced(e quest.ObjectiveAdvancedEvent) {
	s.logger.Debug("quest objective advanced", slog.String("event", "quest.objective_advanced"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID),
		slog.String("objective", e.ObjectiveID), slog.Int("current", e.Current), slog.Int("required", e.Required))
}

func (s questLogSink) StageAdvanced(e quest.StageAdvancedEvent) {
	s.logger.Debug("quest stage advanced", slog.String("event", "quest.stage_advanced"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID), slog.Int("stage", e.StageIndex))
}

func (s questLogSink) Completed(e quest.CompletedEvent) {
	s.logger.Info("quest completed", slog.String("event", "quest.completed"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID),
		slog.Int64("xp", e.XP), slog.Int("gold", e.Gold))
}

func (s questLogSink) Abandoned(e quest.AbandonedEvent) {
	s.logger.Info("quest abandoned", slog.String("event", "quest.abandoned"),
		slog.String("entity_id", e.PlayerID), slog.String("quest", e.QuestID))
}

type effectSink struct {
	bus *eventbus.Bus
}

func (s *effectSink) EffectApplied(ctx context.Context, ev progression.EffectAppliedEvent) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.EffectApplied{
		EntityID:        ev.EntityID,
		EffectID:        ev.EffectID,
		SourceAbilityID: ev.SourceAbilityID,
		Duration:        ev.Duration,
	})
}

func (s *effectSink) EffectRemoved(ctx context.Context, ev progression.EffectRemovedEvent) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.EffectRemoved{
		EntityID:        ev.EntityID,
		EffectID:        ev.EffectID,
		SourceAbilityID: ev.SourceAbilityID,
	})
}

func (s *effectSink) EffectExpired(ctx context.Context, ev progression.EffectExpiredEvent) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.EffectExpired{
		EntityID:        ev.EntityID,
		EffectID:        ev.EffectID,
		SourceAbilityID: ev.SourceAbilityID,
	})
}

// abilityVerbWords returns the (second-person, third-person) verb a
// renderer uses for an ability of the given category. Spells are
// "cast / casts"; everything else (skills, unknown) is "use / uses".
func abilityVerbWords(category string) (string, string) {
	if category == string(progression.AbilitySpell) {
		return "cast", "casts"
	}
	return "use", "uses"
}

// fizzleMessage renders a §4.8 fizzle reason into a caster-facing
// line. The reason is the lower-case keyword from the bus event; an
// unrecognized keyword falls back to a generic line so a future
// engine/content reason doesn't render as a blank message.
func fizzleMessage(abilityName, reason string) string {
	switch reason {
	case "unknown_ability":
		return "You don't know how to do that."
	case "asleep":
		return "You can't do that right now."
	case "alignment_restricted":
		return fmt.Sprintf("Your conscience won't let you use %s.", abilityName)
	case "no_proficiency":
		return fmt.Sprintf("You haven't learned %s.", abilityName)
	case "equipment_required":
		return fmt.Sprintf("You aren't wielding the right equipment for %s.", abilityName)
	case "initiate_only":
		return fmt.Sprintf("You can only use %s to start a fight.", abilityName)
	case "invalid_target":
		return "You don't see your target here."
	case "not_in_combat":
		return fmt.Sprintf("You have to be fighting to use %s.", abilityName)
	case "effect_present":
		return fmt.Sprintf("%s is already in effect.", abilityName)
	case "pulse_delay":
		return fmt.Sprintf("You can't use %s again so soon.", abilityName)
	case "insufficient_resources":
		return fmt.Sprintf("You're too exhausted to use %s.", abilityName)
	default:
		return fmt.Sprintf("Your %s fizzles.", abilityName)
	}
}

// abilitySink bridges progression.AbilitySink to eventbus.Bus
// (M9.4). Same composition-root pattern as effectSink: progression
// must not import eventbus, so each resolution-phase callback maps
// 1:1 to a bus payload. nil bus is a silent no-op.
//
// The vital-depleted callback publishes AbilityVitalDepleted on its
// own topic rather than reaching into combat's death flow directly —
// a future subscriber bridges it to the cancellable death check
// (combat §6.1). Today no ability applies damage, so the callback
// stays dark in production.
type abilitySink struct {
	bus *eventbus.Bus
}

func (s *abilitySink) OnAbilityUsed(ctx context.Context, ev progression.AbilityUsedEvent) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.AbilityUsed{
		SourceID:     ev.SourceID,
		AbilityID:    ev.AbilityID,
		AbilityName:  ev.AbilityName,
		Category:     string(ev.Category),
		HandlerToken: ev.HandlerToken,
		TargetID:     ev.TargetID,
		TargetName:   ev.TargetName,
	})
}

func (s *abilitySink) OnAbilityMissed(ctx context.Context, ev progression.AbilityMissedEvent) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.AbilityMissed{
		SourceID:    ev.SourceID,
		AbilityID:   ev.AbilityID,
		AbilityName: ev.AbilityName,
		TargetID:    ev.TargetID,
		TargetName:  ev.TargetName,
	})
}

func (s *abilitySink) OnAbilityFizzled(ctx context.Context, ev progression.AbilityFizzledEvent) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.AbilityFizzled{
		SourceID:    ev.SourceID,
		AbilityID:   ev.AbilityID,
		AbilityName: ev.AbilityName,
		Reason:      string(ev.Reason),
	})
}

func (s *abilitySink) OnVitalDepleted(ctx context.Context, ev progression.VitalDepletedEvent) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.AbilityVitalDepleted{
		VictimID: ev.VictimID,
		KillerID: ev.KillerID,
		Vital:    ev.Vital,
	})
}

func (s *alignmentSink) OnAlignmentBucketChanged(ctx context.Context, entityID string, oldBucket, newBucket progression.Bucket) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.AlignmentBucketChanged{
		EntityID:  entityID,
		OldBucket: string(oldBucket),
		NewBucket: string(newBucket),
	})
}

func (s *progressionSink) OnTrackReset(ctx context.Context, entityID, track string) {
	s.bus.Publish(ctx, eventbus.TrackReset{
		EntityID: entityID,
		Track:    track,
	})
}

// currencySink bridges economy.Sink to eventbus.Bus (M11.1 — spec
// economy-survival §2.2). Same composition-root pattern as
// alignmentSink: the economy package must not import eventbus, so the
// service reports through this adapter and we map 1:1 to the bus.
type currencySink struct {
	bus *eventbus.Bus
}

func (s *currencySink) OnGoldCredited(ctx context.Context, entityID string, amount int, reason string, newTotal int) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.CurrencyCredited{
		EntityID: entityID,
		Amount:   amount,
		Reason:   reason,
		NewTotal: newTotal,
	})
}

func (s *currencySink) OnGoldDebited(ctx context.Context, entityID string, amount int, reason string, newTotal int) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.CurrencyDebited{
		EntityID: entityID,
		Amount:   amount,
		Reason:   reason,
		NewTotal: newTotal,
	})
}

// shopSink bridges economy.ShopSink to the bus's cancellable publish
// (M11.2 — spec §3.10). Returns whether a listener vetoed; a nil bus
// never cancels.
type shopSink struct {
	bus *eventbus.Bus
}

func (s *shopSink) OnShopBuy(ctx context.Context, actorID, npcID, templateID string, price int64) bool {
	if s.bus == nil {
		return false
	}
	return s.bus.PublishCancellable(ctx, eventbus.NewShopBuy(actorID, npcID, templateID, price))
}

func (s *shopSink) OnShopSell(ctx context.Context, actorID, npcID, templateID string, price int64) bool {
	if s.bus == nil {
		return false
	}
	return s.bus.PublishCancellable(ctx, eventbus.NewShopSell(actorID, npcID, templateID, price))
}

// restSink bridges economy.RestSink to the bus's cancellable publish
// (M11.4 — economy-survival §5.3/§5.4). Returns the cancelled flag so
// the rest service can veto a player-initiated transition; the
// combat-wake path calls through here too but discards the result.
type restSink struct {
	bus *eventbus.Bus
}

func (s *restSink) OnRestStateChange(ctx context.Context, entityID string, oldState, newState economy.RestState, reason string) bool {
	if s.bus == nil {
		return false
	}
	return s.bus.PublishCancellable(ctx, eventbus.NewRestStateChanged(entityID, string(oldState), string(newState), reason))
}

// consumableSink bridges economy.ConsumableSink to the bus (M11.5 —
// economy-survival §6.2). OnItemConsuming is the cancellable pre-event;
// OnItemConsumed is the post notification carrying the effect params for
// the (future) effects subscriber.
type consumableSink struct {
	bus *eventbus.Bus
}

func (s *consumableSink) OnItemConsuming(ctx context.Context, actorID, itemID entities.EntityID, method string) bool {
	if s.bus == nil {
		return false
	}
	return s.bus.PublishCancellable(ctx, eventbus.NewItemConsuming(actorID, itemID, method))
}

func (s *consumableSink) OnItemConsumed(ctx context.Context, actorID entities.EntityID, r economy.ConsumeResult) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, eventbus.ItemConsumed{
		ActorID:         actorID,
		ItemID:          r.ItemID,
		ItemName:        r.ItemName,
		Method:          r.Method,
		EffectID:        r.EffectID,
		EffectDuration:  r.EffectDuration,
		EffectData:      r.EffectData,
		SustenanceValue: r.SustenanceValue,
	})
}

// notifierAdapter bridges progression.Notifier to session.Manager.
// Look up the actor by entity id (which is the player id at M8.4
// — progression keys players by bare id, not the player: prefix)
// and Write the message. A missing actor silently drops the
// notification: a player disconnecting between level-up and notify
// is a routine race, not an error.
type notifierAdapter struct{ mgr *session.Manager }

func (n notifierAdapter) Notify(ctx context.Context, entityID, msg string) {
	a, ok := n.mgr.GetByPlayerID(entityID)
	if !ok {
		return
	}
	_ = a.Write(ctx, msg)
}

// trainsAdapter routes CreditTrains calls to the live actor. The
// bus dispatches synchronously on the publishing goroutine (the
// player session goroutine that called GrantExperience), so a
// missing actor is impossible during a normal level-up cascade —
// the actor cannot disconnect between Publish and the subscriber
// returning. The lookup-miss branch is defensive for the future
// case where progression XP grants come from a non-session origin
// (admin tooling, async event source); we drop the credit
// silently rather than fault.
type trainsAdapter struct{ mgr *session.Manager }

func (t trainsAdapter) CreditTrains(ctx context.Context, entityID string, n int) {
	a, ok := t.mgr.GetByPlayerID(entityID)
	if !ok {
		return
	}
	a.CreditTrains(ctx, entityID, n)
}

// stdRoller satisfies combat.Roller using math/rand/v2's package-
// level IntN, which is documented safe for concurrent use. The
// level-up subscribers fire from whatever goroutine published
// progression.level.up (the player session goroutine that called
// GrantExperience), so a shared *rand.Rand would need its own
// mutex; the package-level functions avoid that complexity. Not
// used by combat itself — combat keeps a dedicated *rand.Rand
// because its phase callbacks already serialize on the tick.
type stdRoller struct{}

func (stdRoller) IntN(n int) int { return rand.IntN(n) }

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

// registerBaselineChannels populates the engine baseline channel
// set. v1 ships ooc only; admin lands when the role-tag system
// is fully real (M10+). Per spec §3.1 (resolved 2026-05-30) the
// channels live under the engine namespace; the YAML loader that
// would also place admin/etc. lands in M13.6b.
func registerBaselineChannels(reg *chat.Registry, scrollbacks map[string]*chat.Scrollback) {
	baseline := []chat.Channel{
		{
			ID:          "tapestry-core:ooc",
			DisplayName: "ooc",
			Kind:        chat.KindPublic,
			DefaultOn:   true,
			Persisted:   true,
			BufferCap:   100,
		},
	}
	for _, ch := range baseline {
		if err := reg.Register(ch); err != nil {
			// Baseline collisions would be a coding bug, not a
			// runtime condition. Panic at boot makes the failure
			// loud rather than silently dropping a channel.
			panic(fmt.Sprintf("baseline channel %q: %v", ch.ID, err))
		}
		cap := ch.BufferCap
		if cap <= 0 {
			cap = chat.DefaultBufferCap
		}
		scrollbacks[ch.ID] = chat.NewScrollback(cap)
	}
}

// chatSubscribers adapts session.Manager to command.ChatSubscribers.
// v1: every online player is subscribed to every channel — the
// channelID argument is intentionally ignored. M13.6b will read
// the player's subscription set off player.yaml and filter by
// channelID at that point. Returns a fresh map per call per the
// ChatSubscribers contract (session.Manager.OnlinePlayers does).
type chatSubscribers struct{ mgr *session.Manager }

func (cs chatSubscribers) Subscribers(channelID string) map[string]string {
	_ = channelID // ignored in v1; see type comment
	if cs.mgr == nil {
		return nil
	}
	return cs.mgr.OnlinePlayers()
}

// chatScrollbackLookup adapts the composition-root scrollback map to
// command.ChatScrollbacks.
type chatScrollbackLookup struct{ m map[string]*chat.Scrollback }

func (l chatScrollbackLookup) Scrollback(channelID string) *chat.Scrollback {
	return l.m[channelID]
}

// portalBusSink bridges portal.Service lifecycle hooks to the
// engine event bus. M15.2 — subscribes are anything that wants to
// hear portal.opened / portal.closed (renderer, AI hooks, future
// scripting).
type portalBusSink struct{ bus *eventbus.Bus }

func (s *portalBusSink) OnPortalOpened(p portal.Portal) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(context.Background(), eventbus.PortalOpened{
		PortalEvent: portalEventOf(p),
	})
}

func (s *portalBusSink) OnPortalClosed(p portal.Portal) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(context.Background(), eventbus.PortalClosed{
		PortalEvent: portalEventOf(p),
	})
}

func portalEventOf(p portal.Portal) eventbus.PortalEvent {
	return eventbus.PortalEvent{
		PortalID:    p.ID,
		SourceRoom:  p.SourceRoom,
		TargetRoom:  p.TargetRoom,
		Keyword:     p.Keyword,
		DisplayName: p.DisplayName,
		ExpiryTick:  p.ExpiryTick,
		PairedID:    p.PairedID,
	}
}

// registerEngineBaselineProperties seeds the engine-known property
// keys. Today's set is small — quest_grant on rooms is the only new
// consumer in M14.5; the registration shape is the seam that future
// engine-side properties (door state, weather override, etc.) will
// extend. Pack-scoped properties belong in their owning feature's
// boot code via Registry.RegisterPack.
func registerEngineBaselineProperties(reg *property.Registry) error {
	if reg == nil {
		return nil
	}
	baseline := []property.Entry{
		{
			Name:        "quest_grant",
			Type:        property.TypeString,
			Description: "Quest id auto-accepted on item pickup or room entry (spec quests §7.2).",
			AppliesTo:   []string{"item", "room"},
		},
		{
			Name:        "key_for",
			Type:        property.TypeString,
			Description: "Door id this item unlocks (spec world-rooms-movement §5.3 + PD-4).",
			AppliesTo:   []string{"item"},
		},
	}
	for _, e := range baseline {
		if err := reg.RegisterEngine(e); err != nil {
			return fmt.Errorf("baseline property %q: %w", e.Name, err)
		}
	}
	return nil
}

// registerBaselineEmotes seeds the engine emote set. Per spec §8 the
// minimum baseline is smile/nod/wave/bow/grin/shrug/laugh; v1 ships
// the full set here. M13.7b moves these into pack content YAML.
func registerBaselineEmotes(reg *emote.Registry) {
	baseline := []emote.Emote{
		{
			ID: "tapestry-core:smile", DisplayName: "smile",
			NoTarget: emote.View{
				ActorView: "You smile.",
				RoomView:  "$n smiles.",
			},
			Targeted: emote.View{
				ActorView:  "You smile at $N.",
				TargetView: "$n smiles at you.",
				RoomView:   "$n smiles at $N.",
			},
		},
		{
			ID: "tapestry-core:nod", DisplayName: "nod",
			NoTarget: emote.View{
				ActorView: "You nod.",
				RoomView:  "$n nods.",
			},
			Targeted: emote.View{
				ActorView:  "You nod to $N.",
				TargetView: "$n nods to you.",
				RoomView:   "$n nods to $N.",
			},
		},
		{
			ID: "tapestry-core:wave", DisplayName: "wave",
			NoTarget: emote.View{
				ActorView: "You wave.",
				RoomView:  "$n waves.",
			},
			Targeted: emote.View{
				ActorView:  "You wave to $N.",
				TargetView: "$n waves to you.",
				RoomView:   "$n waves to $N.",
			},
		},
		{
			ID: "tapestry-core:bow", DisplayName: "bow",
			NoTarget: emote.View{
				ActorView: "You bow deeply.",
				RoomView:  "$n bows deeply.",
			},
			Targeted: emote.View{
				ActorView:  "You bow to $N.",
				TargetView: "$n bows to you.",
				RoomView:   "$n bows to $N.",
			},
		},
		{
			ID: "tapestry-core:grin", DisplayName: "grin",
			NoTarget: emote.View{
				ActorView: "You grin.",
				RoomView:  "$n grins.",
			},
			Targeted: emote.View{
				ActorView:  "You grin at $N.",
				TargetView: "$n grins at you.",
				RoomView:   "$n grins at $N.",
			},
		},
		{
			ID: "tapestry-core:shrug", DisplayName: "shrug",
			NoTarget: emote.View{
				ActorView: "You shrug.",
				RoomView:  "$n shrugs.",
			},
			Targeted: emote.View{
				ActorView:  "You shrug at $N.",
				TargetView: "$n shrugs at you.",
				RoomView:   "$n shrugs at $N.",
			},
		},
		{
			ID: "tapestry-core:laugh", DisplayName: "laugh",
			NoTarget: emote.View{
				ActorView: "You laugh.",
				RoomView:  "$n laughs.",
			},
			Targeted: emote.View{
				ActorView:  "You laugh at $N.",
				TargetView: "$n laughs at you.",
				RoomView:   "$n laughs at $N.",
			},
		},
	}
	for _, e := range baseline {
		if err := reg.Register(e); err != nil {
			panic(fmt.Sprintf("baseline emote %q: %v", e.ID, err))
		}
	}
}

// splitHostPort parses an addr like ":4000" or "0.0.0.0:4000" into
// a (host, port) pair suitable for the MSSP NAME / PORT fields.
// Empty host (the ":4000" form) returns "" for hostname; the
// composition root leaves that field for the operator to override
// via a deployment-specific config layer when one lands.
func splitHostPort(addr string) (host, port string) {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", addr
	}
	return h, p
}

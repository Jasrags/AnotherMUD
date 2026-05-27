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
	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/pack"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
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
		races:        registries.Races,
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
	combatLocator := combatLocator{entities: entityStore, sessions: mgr, placement: placement}
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
	}
	combatTags := combatTagSource{
		entities: entityStore,
		// Rooms have no tag surface yet (spec §2.1 safe-room is
		// deferred content work — see m7-6-deferred-fixes.md). The
		// TagSource plumbing is in place so the moment rooms grow
		// a Tags slice the lookup wires through with no Engage-side
		// changes.
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

	// M9.2: effect manager — per-entity active effects (spec
	// abilities-and-effects §5). Resolver walks the session
	// manager's playerID index; sink bridges the applied /
	// removed / expired transitions onto the eventbus. Manager
	// is constructed before this block so the resolver can
	// capture it directly.
	effectMgr := progression.NewEffectManager(
		session.NewEffectTargetResolver(mgr),
		&effectSink{bus: bus},
	)

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

	combatCooldowns := combat.NewFleeCooldowns()
	combatMgr := combat.NewManagerWith(combat.ManagerConfig{
		Locator:   combatLocator,
		Sink:      combatSink,
		Tags:      combatTags,
		Cooldowns: combatCooldowns,
	})
	combatSink.mgr = combatMgr
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
	autoAttackPhase := combat.NewAutoAttack(combat.AutoAttackConfig{
		Locator:     combatLocator,
		RoomLocator: combatLocator,
		Sink:        combatSink,
		Roller:      combatRNG,
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
	combatHeartbeat := combat.NewHeartbeat(combatMgr, combat.Phases{
		AutoAttack: autoAttackPhase,
		Wimpy:      wimpyPhase,
	})
	combatCadence := cadenceTicks(cfg.TickInterval, cfg.CombatCadence)
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
		World:       w,
		Commands:    cmds,
		Players:     players,
		Manager:     mgr,
		Items:       entityStore,
		Placement:   placement,
		Contents:    contents,
		Templates:   registries.Items,
		Slots:       registries.Slots,
		Bus:         bus,
		Disposition: dispositionHook{e: evaluator},
		Combat:      combatMgr,
		Flee: func(ctx context.Context, c combat.CombatantID) combat.FleeOutcome {
			return combat.Flee(ctx, c, fleeCfg)
		},
		Progression:  progressionMgr,
		Training: progression.NewTrainingManager(
			progression.DefaultTrainingConfig(),
			registries.Races,
			session.NewTrainerSource(mgr, placement, entityStore),
			proficiencyMgr,
		),
		Proficiency: proficiencyMgr,
		Abilities:   registries.Abilities,
		Effects:     effectMgr,
		Races:        registries.Races,
		Classes:      registries.Classes,
		Alignment:    alignmentMgr,
		DefaultRace:  cfg.DefaultRace,
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
	return config{
		Addr:                  envOr("ANOTHERMUD_ADDR", ":4000"),
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
}

func (s *productionCombatSink) OnEngagement(_ context.Context, e combat.Engagement) {
	s.logger.Info("combat.engagement",
		slog.String("attacker", string(e.AttackerID)),
		slog.String("target", string(e.TargetID)),
		slog.String("room", string(e.RoomID)))
}

func (s *productionCombatSink) OnCombatEnded(_ context.Context, e combat.CombatEnded) {
	s.logger.Info("combat.ended",
		slog.String("combatant", string(e.CombatantID)),
		slog.String("room", string(e.RoomID)))
}

func (s *productionCombatSink) OnHit(_ context.Context, e combat.Hit) {
	s.logger.Info("combat.hit",
		slog.String("attacker", string(e.AttackerID)),
		slog.String("target", string(e.TargetID)),
		slog.String("weapon", e.WeaponName),
		slog.Int("damage", e.Damage),
		slog.String("damage_type", e.DamageType),
		slog.Bool("critical", e.IsCritical),
		slog.String("room", string(e.RoomID)))
}

func (s *productionCombatSink) OnMiss(_ context.Context, e combat.Miss) {
	s.logger.Info("combat.miss",
		slog.String("attacker", string(e.AttackerID)),
		slog.String("target", string(e.TargetID)),
		slog.String("weapon", e.WeaponName),
		slog.Bool("fumble", e.IsFumble),
		slog.String("room", string(e.RoomID)))
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
	if s.mgr != nil {
		s.mgr.DisengageAll(ctx, e.VictimID, e.RoomID)
	}
}

// combatTagSource implements combat.TagSource. Reads entity tags
// from the entities.Store (mob side) or returns false for players
// (no Tags surface today — tracked in M6.5 deferred fixes). Room
// tags also return false until rooms grow a Tags field; the §2.1
// safe-room refusal therefore never fires today, but the plumbing
// is in place so it activates the moment content authors a tagged
// room.
type combatTagSource struct {
	entities *entities.Store
}

func (t combatTagSource) RoomHasTag(_ world.RoomID, _ string) bool {
	// Deferred until rooms expose tags. See m7-6-deferred-fixes.md.
	return false
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
		for _, t := range e.Tags() {
			if t == tag {
				return true
			}
		}
		return false
	case strings.HasPrefix(s, combat.PlayerPrefix):
		// Players have no Tags surface yet (M6.5 deferred). The
		// no-kill / no-flee refusals therefore never apply to a
		// player combatant today. Wires through cleanly the moment
		// connActor grows a Tags field.
		return false
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

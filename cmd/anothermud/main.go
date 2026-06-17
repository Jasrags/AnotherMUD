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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jasrags/AnotherMUD/internal/account"
	"github.com/Jasrags/AnotherMUD/internal/ai"
	"github.com/Jasrags/AnotherMUD/internal/biome"
	"github.com/Jasrags/AnotherMUD/internal/campfire"
	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/combat"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/condition"
	"github.com/Jasrags/AnotherMUD/internal/conn/telnet"
	"github.com/Jasrags/AnotherMUD/internal/corpse"
	"github.com/Jasrags/AnotherMUD/internal/crafting"
	"github.com/Jasrags/AnotherMUD/internal/economy"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/feat"
	"github.com/Jasrags/AnotherMUD/internal/gameclock"
	"github.com/Jasrags/AnotherMUD/internal/gathering"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/light"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/loot"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/mssp"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
	"github.com/Jasrags/AnotherMUD/internal/pack"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/portal"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/queststore"
	"github.com/Jasrags/AnotherMUD/internal/questwatch"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/scripting"
	"github.com/Jasrags/AnotherMUD/internal/server"
	"github.com/Jasrags/AnotherMUD/internal/session"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/spawn"
	"github.com/Jasrags/AnotherMUD/internal/stacking"
	"github.com/Jasrags/AnotherMUD/internal/tick"
	"github.com/Jasrags/AnotherMUD/internal/visibility"
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
	if err := pack.RegisterEngineBaselineProperties(registries.Properties); err != nil {
		return fmt.Errorf("register engine baseline properties: %w", err)
	}
	// Engine baseline biomes (outdoors/indoors/underground) register before
	// pack loading so packs can't shadow them and existing bare `terrain`
	// strings resolve (biomes.md §3 / PD-2). indoors/underground carry the
	// shielding flags that A2 will read in place of the hardcoded list.
	if err := biome.RegisterEngineBaseline(registries.Biomes); err != nil {
		return fmt.Errorf("register engine baseline biomes: %w", err)
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

	// M22.1: a dedicated RNG drives loot-table rolls. Spawning runs on
	// the area-tick (single tick goroutine) and at boot — never
	// concurrently — so a non-shared *rand.Rand needs no extra locking,
	// mirroring the combat/weather RNG pattern. Seeded from wall-clock
	// nanos — an F3-accepted exception for RNG seeding (clk is not yet
	// constructed at this point in boot).
	lootRNG := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 2))

	spawner := &bootSpawner{
		store:        entityStore,
		placement:    placement,
		contents:     contents,
		templates:    registries.Items,
		slots:        registries.Slots,
		mobTemplates: registries.Mobs,
		races:        registries.Races,
		classes:      registries.Classes,
		lootTables:   registries.Loot,
		lootRoller:   lootRNG,
		bus:          bus,
		nodes:        registries.Nodes,
	}
	// M17.1b: a sandboxed scripting.Engine is the ScriptCompiler the
	// pack loader uses to syntax-check each pack-supplied Lua file
	// at boot. M17.1c reuses the same Engine via a Runtime that
	// installs long-lived Sandboxes + bus subscriptions.
	scriptEngine := scripting.New(scripting.Options{})
	if err := pack.Load(ctx, cfg.ContentDir, cfg.Packs, registries, spawner, spawner, scriptEngine); err != nil {
		return fmt.Errorf("loading content from %s: %w", cfg.ContentDir, err)
	}
	// Character-identity §2: a server must host at least one world (a
	// `kind: world` pack) — characters are stamped to and gated by the
	// active world set. A pack set that is all libraries can host no one;
	// fail loudly at boot rather than silently disabling the world gate.
	if len(registries.Worlds) == 0 {
		return fmt.Errorf("no active world pack (kind: world) among packs %v — a server must host at least one world", cfg.Packs)
	}
	// Channel layer (docs/themes/channel-vocabulary.md): build the active
	// ruleset's stat→combat-channel derivation from the loaded packs
	// (later-wins per channel). Built AFTER Load because the mapping is
	// pack content; an empty registry (content-less boot) falls back to the
	// Go baseline so derivation still works. SetChannelMap stamps the store
	// AND retro-stamps any mob spawned during Load; the same mapping feeds
	// session.Config for players.
	channelMap := channel.BaselineMapping()
	if registries.ChannelMap.Len() > 0 {
		built, err := registries.ChannelMap.Build()
		if err != nil {
			return fmt.Errorf("building combat-channel map: %w", err)
		}
		channelMap = built
	}
	entityStore.SetChannelMap(channelMap)
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
	// Gathering (gathering.md §3.1): generate per-room resource-node spawn
	// rules from each node-bearing biome's spawn table, appended to the
	// room's area BEFORE the spawn scheduler starts. The shared §3.6 reset
	// algorithm then spawns + respawns nodes exactly like mobs.
	if n := wireBiomeNodeSpawnRules(w, registries.Biomes, registries.Nodes); n > 0 {
		logging.From(ctx).Info("biome node spawn rules wired",
			slog.String("event", "gathering.node_rules"), slog.Int("rules", n))
	}

	// M10.2: compile the pack-loaded theme once and bind a shared,
	// read-only color renderer. connActor.Write routes every outbound
	// line through it (RenderAnsi/RenderPlain by the session color
	// flag). Compiling after Load means the renderer sees every pack's
	// theme overrides; no recompile happens at runtime.
	// M20.4: seed item.<key> / essence.<key> theme entries from the
	// loaded decoration vocabularies before Compile. Register-if-absent, so
	// a pack theme file's explicit decoration color overrides the tier's
	// built-in default (item-decorations §4 — the theme owns the color).
	registries.Rarity.RegisterTheme(registries.Theme)
	registries.Essence.RegisterTheme(registries.Theme)
	registries.Theme.Compile()
	colorRenderer := render.NewColorRenderer(registries.Theme)

	// M21: the inventory stack-grouping service. Engine default (template +
	// essence keys); pack-registered extra keys (AddKey) would wire here
	// once a manifest surface for them exists.
	stackingSvc := stacking.NewService()

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

	// Help visibility through HasRole (M19.4f — ui-rendering-help §9.5):
	// resolve a requester's help tier from their live role set so admins see
	// admin-tier topics (the admin verbs GenerateHelpTopics marks RoleAdmin)
	// and players don't. Closes the M19.3 "hidden-from-all" cap. Set once
	// here at boot, before the listeners start serving help queries.
	registries.Help.SetRoleResolver(func(entityID string) help.Role {
		if a, ok := mgr.GetByPlayerID(entityID); ok && a.HasRole(cfg.AdminRole) {
			return help.RoleAdmin
		}
		return help.RolePlayer
	})

	// Store.SetRoomScan is intentionally NOT wired today: every spawned
	// item goes through Store.Spawn which auto-tracks, so the by-id
	// index is always the source of truth. The §4.2 step-2 fallback
	// becomes relevant only once items can enter the world without
	// passing through Spawn (e.g. external loader); add the bridge
	// when that path lands rather than fabricating one now.

	clk := clock.RealClock{}

	// Bad-input tracker (commands-and-dispatch §6): records unknown player
	// verbs for operator triage. Shared by the dispatcher (records) and the
	// `badinput` admin verb (reads); threaded through session.Config.
	badInput := command.NewBadInputTracker(clk)

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
	// Channels are pack content (loaded by pack.Load above into
	// registries.Channels; the engine baseline `ooc` ships in the core
	// pack). Per-channel scrollbacks are derived here from the registry;
	// scrollback persistence is in-memory only for now.
	chatRegistry := registries.Channels
	chatScrollbacks := make(map[string]*chat.Scrollback)
	subscribers := chatSubscribers{mgr: mgr}
	scrollbackLookup := chatScrollbackLookup{m: chatScrollbacks}
	// Per-channel verbs + scrollbacks are registered now so chat.Registry.All()
	// drives the command surface; missing here would mean a configured channel
	// has no verb.
	for _, ch := range chatRegistry.All() {
		cap := ch.BufferCap
		if cap <= 0 {
			cap = chat.DefaultBufferCap
		}
		chatScrollbacks[ch.ID] = chat.NewScrollback(cap)
		if err := cmds.Register(ch.DisplayName, command.MakeChannelHandler(ch)); err != nil {
			return fmt.Errorf("register channel verb %q: %w", ch.DisplayName, err)
		}
	}

	// M13.7: emote registry + engine baseline. Like channels, the
	// pack-loaded YAML path is M13.7b. Per-emote verbs (and any
	// declared aliases) register into the command registry now.
	// Emotes are pack content too (registries.Emotes; the smile/nod/…
	// baseline ships in the core pack). Per-emote verbs (+ aliases) register
	// into the command registry now.
	emoteRegistry := registries.Emotes
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
	// Slow-tick observability (time-and-clock §5): warn when a tick
	// overruns its budget. Default threshold is the tick interval itself
	// — a tick that takes longer than its own period means the loop is
	// falling behind. Operators tune via ANOTHERMUD_SLOW_TICK_THRESHOLD
	// (set very high to effectively silence it).
	slowThreshold := cfg.SlowTickThreshold
	if slowThreshold <= 0 {
		slowThreshold = cfg.TickInterval
	}
	loop.SetSlowTickObserver(slowThreshold, func(n uint64, total, handlers time.Duration) {
		slog.Warn("slow tick",
			slog.Uint64("tick", n),
			slog.Duration("total", total),
			slog.Duration("handlers", handlers),
			slog.Duration("threshold", slowThreshold),
		)
	})
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
	idleCfg.AdminRole = cfg.AdminRole // admins are exempt from idle (session-lifecycle §5.2)
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
	// Override the §4.4 drain knobs from env (testing/tuning) while
	// keeping the tier thresholds + regen multipliers at their defaults.
	// The tick handler below reads Config().DrainCadence, so the override
	// flows through to the registration cadence automatically.
	susCfg := economy.DefaultSustenanceConfig()
	susCfg.DrainCadence = cadenceTicks(cfg.TickInterval, cfg.SustenanceDrainInterval)
	if cfg.SustenanceDrainAmount > 0 {
		susCfg.DrainAmount = cfg.SustenanceDrainAmount
	}
	sustenanceSvc := economy.NewSustenanceService(susCfg)
	if err := loop.Register("sustenance-drain", sustenanceSvc.Config().DrainCadence, func(ctx context.Context, n uint64) {
		mgr.DrainSustenance(ctx, sustenanceSvc, cfg.AdminRole, n)
	}); err != nil {
		return fmt.Errorf("register sustenance-drain tick: %w", err)
	}

	// Light-source fuel burn (light-and-darkness §3.2). Same drain shape
	// as sustenance: a lit fuel-burning source held by a logged-in actor
	// loses fuel on this cadence and gutters out at zero.
	fuelCfg := light.DefaultFuelConfig()
	if err := loop.Register("fuel-burn", fuelCfg.BurnCadence, func(ctx context.Context, _ uint64) {
		mgr.BurnFuel(ctx, fuelCfg, entityStore, bus)
	}); err != nil {
		return fmt.Errorf("register fuel-burn tick: %w", err)
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
		// Biome-driven weather/time shielding (biomes.md §3): the §6.4
		// eligibility check reads the room biome's flags. Unregistered
		// terrain → ok=false → weather falls back to the hardcoded
		// indoors/underground set, so existing content is unchanged (the
		// baseline indoors/underground biomes carry identical flags).
		Shielding: func(terrain string) (weatherShielded, timeShielded bool, ok bool) {
			// Resolve (not Get) so the closure self-normalizes empty →
			// outdoors, staying robust if a future caller passes a raw
			// room.Terrain and symmetric with the ambience path (also
			// Resolve). Unregistered terrain → ok=false → hardcoded fallback.
			b, found := registries.Biomes.Resolve(terrain)
			if !found {
				return false, false, false
			}
			return b.WeatherShielded, b.TimeShielded, true
		},
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

	// Biome ambience (biomes.md §4): idle ecological flavor delivered to
	// occupied rooms of a biome on its own cadence. Runs on the (single)
	// tick goroutine, so its RNG needs no extra synchronization. Unlike
	// weather/time ambience it is NOT shielding-gated and emits no event.
	// PCG stream 4 (distinct from combat=0, weather=1, loot=2, corpse=3).
	biomeRNG := rand.New(rand.NewPCG(uint64(clk.Now().UnixNano()), 4))
	biomeAmbience := biome.NewAmbienceService(
		registries.Biomes, w,
		func(roomID world.RoomID) bool { return len(mgr.PlayersInRoom(roomID)) > 0 },
		mgr, biomeRNG,
	)
	if err := loop.Register("biome-ambience", cadenceTicks(cfg.TickInterval, cfg.BiomeAmbienceInterval), func(ctx context.Context, _ uint64) {
		biomeAmbience.Tick(ctx)
	}); err != nil {
		return fmt.Errorf("register biome-ambience tick: %w", err)
	}

	// In-game time persistence (light-and-darkness §7, resolving
	// time-and-clock §3.6): seed the clock from the global saved time
	// when present, else cold-start at the documented initial state.
	// Because darkness now gates gameplay, a restart must not reset the
	// world to night.
	clockStore := gameclock.NewStore(cfg.SaveDir)
	clockCfg := gameclock.Config{Bus: bus}
	if saved, ok := clockStore.Load(ctx); ok {
		clockCfg.InitialHour = saved.CurrentHour
		clockCfg.InitialDay = saved.DayCount
	}
	gameClock := gameclock.New(clockCfg)
	if err := loop.Register("game-clock", 1, func(ctx context.Context, _ uint64) {
		gameClock.Tick(ctx)
	}); err != nil {
		return fmt.Errorf("register game-clock tick: %w", err)
	}
	// Flush the clock on every hour advance — the saved-time write
	// cadence (§7). This bounds loss on an unclean shutdown to at most
	// one in-game hour; a clean shutdown flushes the current time
	// below. Sub-hour position is intentionally not persisted.
	bus.Subscribe(eventbus.EventTimeHourChange, func(ctx context.Context, ev eventbus.Event) {
		if _, ok := ev.(eventbus.TimeHourChange); !ok {
			return
		}
		if err := clockStore.Save(gameClock.Snapshot()); err != nil {
			logging.From(ctx).Warn("gameclock.save: hour-advance flush failed",
				slog.String("err", err.Error()))
		}
	})

	// Light-and-darkness resolver (light §2): default policy paired with
	// the in-game clock as its period source. Threaded into the session
	// config so command.Env carries it.
	lightResolver := light.NewResolver(light.DefaultConfig(), gameClock)
	// Light transitions (§6): on a period boundary, notify players whose
	// effective light level crosses (per-viewer, so torch-bearers and
	// darkvision viewers feel only their own change). Shares the
	// time.period.change seam with the weather time-ambience above.
	bus.Subscribe(eventbus.EventTimePeriodChange, func(ctx context.Context, ev eventbus.Event) {
		pc, ok := ev.(eventbus.TimePeriodChange)
		if !ok {
			return
		}
		mgr.LightTransitions(ctx, lightResolver, w, entityStore, placement, pc.PreviousPeriod, pc.Period)
	})

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

	// Crafting Phase 1: per-character known-recipe manager. Holds the
	// recipe registry so it can drop ids removed from content on restore
	// (crafting-and-cooking §9) and resolve a discipline's baseline
	// recipes on learn (§2).
	knownRecipesMgr := recipe.NewKnownManager(registries.Recipes)

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
	// completion; the watcher routes mob-killed / item-picked-up /
	// item-given / player-moved into objective progress.
	//
	// The event sink is the session questNotifier: it both logs the
	// lifecycle and writes progress / stage / ready-to-turn-in / completion
	// messages to the acting player (the old log-only sink left the player
	// blind). The giver/item name resolvers turn template ids into display
	// names for the turn-in prompt and the reward banner.
	questGiverName := func(tid string) string {
		if t, err := registries.Mobs.Get(mob.TemplateID(tid)); err == nil {
			return t.Name
		}
		return ""
	}
	questItemNameFn := func(tid string) string {
		if t, err := registries.Items.Get(item.TemplateID(tid)); err == nil {
			return t.Name
		}
		return ""
	}
	questSvc := quest.NewService(quest.Config{
		Registry: registries.Quests,
		Persist:  questStore,
		Rewards:  session.NewQuestRewards(mgr, progressionMgr, proficiencyMgr, registries.Items, entityStore, currencySvc, knownRecipesMgr, cfg.DefaultXPTrack),
		Events:   session.NewQuestNotifier(mgr, registries.Quests, questGiverName, questItemNameFn, logging.From(ctx)),
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

	// conditions §6: condition apply/clear messaging. Active effects only
	// feed GMCP today; this renders a player-facing line for the Core 5 so
	// "You are stunned!" (to the target) and "The bandit is knocked prone."
	// (to the room) read in-game. Resisting is narrated by the SaveResolved
	// event; prone's clear is owned by the `stand` verb (omitted here to
	// avoid a double message). Second-person + third-person-suffix per id.
	conditionApplyMsg := map[string][2]string{
		"stunned":    {"You are stunned!", "is stunned"},
		"prone":      {"You are knocked prone!", "is knocked prone"},
		"blinded":    {"You are blinded!", "is blinded"},
		"frightened": {"You are gripped by fear!", "is gripped by fear"},
		"fatigued":   {"You feel fatigued.", "looks fatigued"},
	}
	conditionClearMsg := map[string][2]string{
		"stunned":    {"You shake off the stun.", "shakes off the stun"},
		"blinded":    {"Your sight returns.", "can see again"},
		"frightened": {"You master your fear.", "masters their fear"},
		"fatigued":   {"Your fatigue passes.", "looks rested"},
	}
	notifyCondition := func(ctx context.Context, entityID, effectID string, msgs map[string][2]string) {
		m, ok := msgs[effectID]
		if !ok {
			return
		}
		// Tell the target if it is an online player (second person).
		if a, ok := mgr.GetByPlayerID(entityID); ok {
			_ = a.Write(ctx, "<warning>"+m[0]+"</warning>")
		}
		// Announce to the room in third person (the attacker/admin sees this
		// even when the target is a mob). Resolve the room from the player
		// table first, then the entity placement. The exclude arg is the
		// effect's entityID, which for a player IS its PlayerID (the key
		// SendToRoom excludes on) so the target doesn't get the room copy on
		// top of the direct tell above; for a mob it matches no session.
		roomID, found := mgr.RoomOfPlayer(entityID)
		if !found {
			roomID, found = placement.RoomOf(entities.EntityID(entityID))
		}
		if found {
			mgr.SendToRoom(ctx, roomID, combatantName(entityID)+" "+m[1]+".", entityID)
		}
	}
	bus.Subscribe(eventbus.EventEffectApplied, func(ctx context.Context, ev eventbus.Event) {
		if e, ok := ev.(eventbus.EffectApplied); ok {
			notifyCondition(ctx, e.EntityID, e.EffectID, conditionApplyMsg)
		}
	})
	bus.Subscribe(eventbus.EventEffectRemoved, func(ctx context.Context, ev eventbus.Event) {
		if e, ok := ev.(eventbus.EffectRemoved); ok {
			notifyCondition(ctx, e.EntityID, e.EffectID, conditionClearMsg)
		}
	})
	bus.Subscribe(eventbus.EventEffectExpired, func(ctx context.Context, ev eventbus.Event) {
		if e, ok := ev.(eventbus.EffectExpired); ok {
			notifyCondition(ctx, e.EntityID, e.EffectID, conditionClearMsg)
		}
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
	// WoT S2 — the channel interrupt game. Tracks each entity's single
	// in-flight timed weave (warmup countdown). Behavior-neutral until content
	// authors a `cast_time` > 0 ability: with no timed casts the ability phase
	// resolves everything instantly, exactly as before. Logout drops state.
	castTracker := progression.NewCastTracker()

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
		// Walk every class (one today; wot-character-model D1 multiclass
		// seam). ClassPathProcessor.Apply gates on the class's BoundTrack, so
		// only the class bound to this level-up's track grants its Path.
		for _, classID := range actor.ClassIDs() {
			classPath.Apply(ctx, e.EntityID, classID, e.Track, e.NewLevel)
			if cls, ok := registries.Classes.Get(classID); ok {
				// Spec §4.6 step 2: no track gate — stat growth runs on
				// every level-up regardless of track. (M8.4 ROADMAP
				// acceptance criterion: documented decision.) Per-track
				// stat-growth gating is a future multiclass-tuning concern.
				progression.ApplyStatGrowth(ctx, cls, actor.StatBlock(), growthRoller, trainsCrediter, e.EntityID)
			}
		}
		// feats §2.2 (EPIC S4 Phase 2): bank a feat slot for every 3rd
		// character level crossed (3/6/9/…). Each level-up step is +1, so for a
		// single-class character the track level == character level and this is
		// correct. MULTICLASS FIX-BY: with two bound tracks this over-credits —
		// each track independently crossing its own 3rd level grants a slot, so
		// the error scales with the number of tracks. When multiclass content
		// can advance a second track, feed CreditsForLevelChange the actor's
		// TOTAL character level transition instead of this track's e.Old/NewLevel
		// (that is the only change — the helper stays the same).
		if n := feat.CreditsForLevelChange(e.OldLevel, e.NewLevel); n > 0 {
			actor.CreditFeats(n)
		}
	})

	// backgrounds §4: the one-time starting-package granter (skills/items/gold).
	// Fired only on character.created, so it never re-applies on login.
	backgroundGranter := session.NewBackgroundGranter(mgr, proficiencyMgr, registries.Items, entityStore, currencySvc)

	bus.Subscribe(eventbus.EventCharacterCreated, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.CharacterCreated)
		if !ok {
			return
		}
		// Resolve the actor and grant every class's level-1 Path (one class
		// today; wot-character-model D1 seam). The event payload carries the
		// primary class, but we walk the live list so a multiclass character
		// gets all its starting features.
		actor, ok := mgr.GetByPlayerID(e.EntityID)
		if !ok {
			return
		}
		for _, classID := range actor.ClassIDs() {
			// Spec §4.5 step 3: character-created is treated as level 1
			// with no track gate. Pass empty trackName so Apply
			// short-circuits the gate check.
			classPath.Apply(ctx, e.EntityID, classID, "", 1)
			// WoT S2 Phase 1: a flat level-1 base-stat endowment (the
			// channeler's resource_max One Power pool). Applied additively
			// via AdjustBase so it composes across a multiclass character;
			// it persists into the base snapshot and the OnMaxChange listener
			// (wired in seedResourcePools at login) propagates a resource_max
			// bump straight to the live mana pool. Fires once — this event
			// never re-fires on relogin, where RestoreBase carries the value.
			if cls, ok := registries.Classes.Get(classID); ok {
				actor.ApplyStartingStats(cls.StartingStats)
			}
		}
		// A freshly created character starts with full resource pools: the
		// StartingStats above raised a channeler's resource_max via OnMaxChange,
		// but SetMax leaves current at 0 (level-up semantics), so fill once here.
		actor.FillResourcePools()
		// backgrounds §4: grant the chosen background's starting package once.
		if bgID := actor.BackgroundID(); bgID != "" {
			if bg, ok := registries.Backgrounds.Get(bgID); ok {
				backgroundGranter.Grant(ctx, e.EntityID, bg)
			}
		}
		// feats §2.2 (EPIC S4 Phase 2): the base feat slot granted at creation
		// (1 feat at character creation). Per-3-levels slots accrue from the
		// level-up subscriber; background/class feat grants are Phase 5.
		actor.CreditFeats(1)
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

	// combat.flee → room announcements (with direction). The Mover does
	// the move but not the messaging; this subscriber owns the
	// presentation because the Flee event carries the fled direction
	// ("X flees to the north!"), mirroring normal-move phrasing. Handles
	// both players (excluded from their own departure line — they get
	// "You flee in panic!" from the verb) and mobs.
	bus.Subscribe(eventbus.EventFlee, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.Flee)
		if !ok {
			return
		}
		exclude := ""
		if pid, isPlayer := strings.CutPrefix(e.EntityID, combat.PlayerPrefix); isPlayer {
			exclude = pid
		}
		name := upperFirst(e.EntityName)
		departure := name + " flees!"
		if e.Direction != "" {
			departure = fmt.Sprintf("%s flees to the %s!", name, e.Direction)
		}
		mgr.SendToRoom(ctx, e.From, departure, exclude)
		mgr.SendToRoom(ctx, e.To, name+" arrives, panting.", exclude)
	})

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

	// M22.2: mob.killed → corpse creation (loot-and-corpses §2). Wired
	// BEFORE the untrack subscriber below so the corpse is minted while
	// the mob is still tracked (spec §2.1 ordering). It reads the mob's
	// spawn-time loot from the shared Contents index (which survives the
	// later Untrack regardless) and the room from the event.
	//
	// corpseRNG is its own *rand.Rand. All mob deaths flow through the
	// combat heartbeat (auto-attack or ability phase), which is the
	// combat-tick handler; the tick loop runs handlers sequentially on a
	// single goroutine and the bus dispatches synchronously, so the coin
	// roll is tick-goroutine-confined — same safety basis as combatRNG /
	// weatherRNG. (If a death is ever signalled off a non-tick goroutine,
	// this RNG and lootRNG would each need a lock — tracked in
	// m22-deferred-fixes.)
	corpseRNG := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 3))
	corpseSvc := corpse.New(corpse.Config{
		Store:     entityStore,
		Contents:  contents,
		Placement: placement,
		Bus:       bus,
		Mobs:      registries.Mobs,
		Loot:      registries.Loot,
		Roller:    corpseRNG,
		Now:       loop.TickCount,
	})
	bus.Subscribe(eventbus.EventMobKilled, corpseSvc.OnMobKilled)

	// M22.5: corpse-decay sweep (loot-and-corpses §7; time-and-clock §3
	// reserved handler). Removes corpses past their lifetime, destroying
	// any unlooted contents — this is what bounds corpse growth on a
	// live server. Lifetime + sweep cadence are wall-clock knobs → ticks.
	corpseLifetimeTicks := cadenceTicks(cfg.TickInterval, cfg.CorpseLifetime)
	if err := loop.Register("corpse-decay", cadenceTicks(cfg.TickInterval, cfg.CorpseDecayInterval), func(ctx context.Context, n uint64) {
		corpseSvc.DecaySweep(ctx, n, corpseLifetimeTicks)
	}); err != nil {
		return fmt.Errorf("register corpse decay: %w", err)
	}

	// Crafting Phase 5: campfire-decay sweep (crafting-and-cooking §4). A
	// built campfire is a temporary Tier-1 cooking station; this removes
	// fires past their lifetime and announces the burn-out to the room.
	campfireSvc := campfire.NewService(entityStore, placement)
	campfireLifetimeTicks := cadenceTicks(cfg.TickInterval, cfg.CampfireLifetime)
	if err := loop.Register("campfire-decay", cadenceTicks(cfg.TickInterval, cfg.CampfireDecayInterval), func(ctx context.Context, n uint64) {
		for _, roomID := range campfireSvc.DecaySweep(ctx, n, campfireLifetimeTicks) {
			mgr.SendToRoom(ctx, roomID, "The campfire burns down to cold ashes.")
		}
	}); err != nil {
		return fmt.Errorf("register campfire decay: %w", err)
	}

	// M22.4: autoloot (loot-and-corpses §6). On corpse.created, if the
	// killer is an online player with autoloot ON who is present in the
	// corpse's room, loot it on their behalf immediately. Rights are
	// trivially satisfied (the killer owns their own fresh kill), so this
	// reuses command.TransferCorpse without the §4 gate. Scoped to the
	// killer's own kills, loots everything (items + coins) — narrower
	// scopes are future refinements (§10).
	//
	// Goroutine safety: this runs on the tick goroutine (the mob.killed →
	// corpse.created chain dispatches synchronously from the combat
	// heartbeat). TransferCorpse mutates the killer's inventory
	// (AddToInventory) and gold (Currency.AddGold) — both fully a.mu-
	// guarded on connActor, the same discipline the effect-tick already
	// uses to mutate live actors (AddModifiers/RemoveBySource). The
	// contents/placement/coin claims are independent single-winner ops
	// with no nested lock-order inversion, so a concurrent player
	// loot/get/drop on the session goroutine is serialized, not raced.
	bus.Subscribe(eventbus.EventCorpseCreated, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.CorpseCreated)
		if !ok || e.KillerID == "" {
			return
		}
		pid, ok := strings.CutPrefix(e.KillerID, combat.PlayerPrefix)
		if !ok {
			return // killer is not a player (mob/scripted) — no autoloot
		}
		actor, ok := mgr.GetByPlayerID(pid)
		if !ok || !actor.Autoloot() {
			return
		}
		if room := actor.Room(); room == nil || room.ID != e.RoomID {
			return // killer not present in the corpse's room
		}
		ent, ok := entityStore.GetByID(e.CorpseID)
		if !ok {
			return
		}
		target, ok := ent.(*entities.ItemInstance)
		if !ok {
			return
		}
		taken, coins := command.TransferCorpse(ctx, command.LootGrant{
			Items:     entityStore,
			Contents:  contents,
			Placement: placement,
			Currency:  currencySvc,
			Bus:       bus,
		}, actor, target, e.RoomID, e.KillerID)
		if len(taken) == 0 && coins == 0 {
			return
		}
		_ = actor.Write(ctx, fmt.Sprintf("You quickly loot %s from %s.",
			autolootSummary(taken, coins), target.Name()))
		mgr.SendToRoom(ctx, e.RoomID,
			fmt.Sprintf("%s quickly loots %s.", actor.Name(), target.Name()),
			actor.PlayerID())
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
	// the swing rolls). The proficiency surface is the M9.5 #3 composite
	// so a MOB's content-defined passives (e.g. a guard's second-attack)
	// fire alongside players' — players resolve through the persistent
	// manager, mobs through the entity store; mob gain is a no-op.
	// *progression.PassiveResolver satisfies combat.PassiveEvaluator
	// structurally — no adapter needed.
	passiveProficiency := session.NewPassiveProficiency(proficiencyMgr, entityStore)
	// §3.5 step-3 gain stat factor: a passive's training rate scales with
	// its declared gain stat (parry/second-attack scale on dex). The reader
	// resolves player-or-mob to an effective stat value; mob gain is a
	// no-op regardless, so this only speeds player passive training.
	passiveStatReader := session.NewPassiveStatReader(mgr, entityStore)

	// Crafting Phase 2 service (quality roll + atomic consume/produce,
	// §3/§5). Built here so it can share passiveStatReader for the §3.5
	// craft skill-up gain-stat scaling. stdRoller{} (math/rand/v2 package
	// source, concurrent-safe) since crafts arrive on per-session goroutines.
	craftSvc := crafting.NewService(
		registries.Items, entityStore, registries.Recipes, knownRecipesMgr,
		proficiencyMgr, registries.Rarity, stdRoller{}, crafting.DefaultConfig(),
		passiveStatReader,
	)

	// Gathering (gathering.md §2): the `forage` verb rolls the room biome's
	// forage table. The Service owns the quality roll + item spawn + the
	// gathering-proficiency use-gain; the per-character forage cooldown is
	// config-driven (§5).
	gatheringCfg := gathering.DefaultConfig()
	gatheringCfg.ForageCooldownTicks = cadenceTicks(cfg.TickInterval, cfg.ForageCooldown)
	gatheringSvc := gathering.NewService(
		registries.Rarity, proficiencyMgr, stdRoller{}, gatheringCfg,
		passiveStatReader, entityStore, registries.Items,
	)

	// B3 timed crafting (crafting-and-cooking §3): a recipe's time_pulses
	// occupies the player. The `craft` verb arms a per-actor timer; this
	// tick finishes any craft that has come due. Cadence 1 (every tick) so
	// a craft completes on exactly its ReadyAt tick — the sweep is cheap (a
	// per-player int compare). Movement and combat cancel an in-flight
	// craft (SetRoom + the engagement sink); nothing here reserves inputs,
	// so a logout/crash mid-craft loses only the work, never the materials.
	if err := loop.Register("craft-complete", 1, func(ctx context.Context, n uint64) {
		mgr.CompleteReadyCrafts(ctx, n, craftSvc)
	}); err != nil {
		return fmt.Errorf("register craft-complete tick: %w", err)
	}

	passiveResolver := progression.NewPassiveResolver(
		registries.Abilities, passiveProficiency, passiveProficiency, passiveStatReader, combatRNG,
	)
	// §5.3 darkness to-hit penalty: resolve the attacker to its room +
	// per-viewer light (held source, room luminous items, darkvision)
	// and turn the effective level into a negative hit-mod delta. Players
	// and mobs both flow through the shared EffectiveLight gather; a mob
	// has no light slot, so it darkens by room light alone. Lit ⇒ 0.
	attackerDarknessPenalty := func(id combat.CombatantID) int {
		roomID, ok := combatLocator.RoomOf(id)
		if !ok {
			return 0
		}
		room, err := w.Room(roomID)
		if err != nil {
			return 0
		}
		viewer, ok := combatLocator.LookupCombatant(id)
		if !ok {
			return 0
		}
		lvl := command.EffectiveLight(lightResolver, room, viewer, entityStore, placement)
		return -lightResolver.Config().HitPenalty(lvl)
	}
	// weapon-identity §3: a player wielding a weapon outside their class
	// proficiency set takes a flat to-hit penalty. Mobs have no class and
	// are always proficient (short-circuit on the non-player prefix). The
	// proficiency result is computed on the connActor (resolving its class
	// live by id). Composes additively with the darkness penalty through
	// the same HitModAdjust seam.
	attackerProficiencyPenalty := func(id combat.CombatantID) int {
		if !strings.HasPrefix(string(id), combat.PlayerPrefix) {
			return 0
		}
		a, ok := mgr.GetByPlayerID(string(id)[len(combat.PlayerPrefix):])
		if !ok || a.IsWeaponProficient() {
			return 0
		}
		return -cfg.NonProficientPenalty
	}
	// conditions §3/§5: a bare entity id → the aggregate combat/save impact
	// of its active condition flags (prone/stunned/blinded/frightened/…).
	// Reads the live effect-flag set; the leaf `condition` package does the
	// pure fold under the engine-default magnitudes.
	condCfg := condition.DefaultConfig()
	conditionImpact := func(bareID string) condition.Impact {
		return condition.Resolve(effectMgr.Flags(bareID), condCfg)
	}
	hitModAdjust := func(id combat.CombatantID) int {
		// conditions §3 — the attacker-penalty half (prone/blinded/fear)
		// composes additively here alongside darkness + proficiency.
		condPenalty := -conditionImpact(combat.EntityIDOf(id)).AttackerHitPenalty
		return attackerDarknessPenalty(id) + attackerProficiencyPenalty(id) + condPenalty
	}

	// baseSaveBonus resolves a bare entity's class+ability save on an axis,
	// player or mob, BEFORE condition penalties. Players compose class base
	// saves + the live ability modifier (connActor.Saves); classless mobs
	// save on the ability modifier alone (base zero). Tries the player table
	// first, then the entity store. 0 when unresolvable (the save still
	// rolls, just with no bonus).
	baseSaveBonus := func(bareID string, axis progression.SaveType) int {
		if a, ok := mgr.GetByPlayerID(bareID); ok {
			return a.Saves().Get(axis)
		}
		if ent, ok := entityStore.GetByID(entities.EntityID(bareID)); ok {
			if m, ok := ent.(*entities.MobInstance); ok {
				return progression.DeriveSaves(progression.Saves{}, m.StatBlock().Effective).Get(axis)
			}
		}
		return 0
	}
	// effectiveSaveBonus is the single save-bonus surface every save consumer
	// reads (saves §4, conditions §4): the base save minus any morale
	// penalty from an active fear condition. Used by both the massive-damage
	// Fortitude save and the condition shake-off saves so fear consistently
	// makes a target worse at every save.
	effectiveSaveBonus := func(bareID string, axis progression.SaveType) int {
		return baseSaveBonus(bareID, axis) - conditionImpact(bareID).SavePenalty
	}
	// conditions §4: the effect manager rolls per-tick shake-off saves
	// through this bridge — combat.ResolveSave over the effective save bonus.
	// Recurring saves do not emit a SaveResolved event per tick (that would
	// spam a failed shake-off every round); the shake-off is narrated by the
	// effect-removed message when a save finally lands.
	effectMgr.SetSaveResolver(progression.SaveResolverFunc(
		func(_ context.Context, entityID string, axis progression.SaveType, dc int, _ string) bool {
			return combat.ResolveSave(combatRNG, effectiveSaveBonus(entityID, axis), dc).Success
		}))
	victimFortBonus := func(id combat.CombatantID) int {
		return effectiveSaveBonus(combat.EntityIDOf(id), progression.SaveFortitude)
	}
	// conditions §4: the EMITTING entry-save resolver the ability resolver
	// uses (trip/bash). Unlike the silent shake-off resolver above, this
	// emits a SaveResolved event so a resisted condition reads in-game
	// ("X resists!"). Resolves the bare entity id to a combatant + room for
	// the event payload (player first, then mob).
	combatantIDOf := func(bareID string) (combat.CombatantID, bool) {
		if _, ok := mgr.GetByPlayerID(bareID); ok {
			return combat.NewPlayerCombatantID(bareID), true
		}
		if _, ok := entityStore.GetByID(entities.EntityID(bareID)); ok {
			return combat.NewMobCombatantID(bareID), true
		}
		return "", false
	}
	entrySaveResolver := progression.SaveResolverFunc(
		func(ctx context.Context, entityID string, axis progression.SaveType, dc int, cause string) bool {
			outcome := combat.ResolveSave(combatRNG, effectiveSaveBonus(entityID, axis), dc)
			// Emit SaveResolved only on a MADE save (the resist) — that is the
			// line worth showing ("X resists!"). A failed entry save is
			// narrated by the condition apply-message instead ("X is stunned!"),
			// so emitting here too would double-narrate the failure.
			if outcome.Success {
				if cid, ok := combatantIDOf(entityID); ok {
					room, _ := combatLocator.RoomOf(cid)
					combatSink.OnSaveResolved(ctx, combat.SaveResolved{
						CreatureID: cid,
						SaveType:   string(axis),
						Cause:      cause,
						Outcome:    outcome,
						RoomID:     room,
					})
				}
			}
			return outcome.Success
		})
	// A non-positive threshold means "every hit" at the combat layer (a
	// test-only option); in production that is never intended and almost
	// always a misconfigured env, so coerce it back to the engine default
	// with a warning rather than silently forcing a save on every swing.
	massiveThreshold := cfg.MassiveDamageThreshold
	if massiveThreshold <= 0 {
		logging.From(ctx).Warn("massive-damage threshold non-positive; using engine default",
			slog.Int("configured", cfg.MassiveDamageThreshold),
			slog.Int("default", combat.DefaultMassiveDamageThreshold))
		massiveThreshold = combat.DefaultMassiveDamageThreshold
	}
	autoAttackPhase := combat.NewAutoAttack(combat.AutoAttackConfig{
		Locator:        combatLocator,
		RoomLocator:    combatLocator,
		Sink:           combatSink,
		Roller:         combatRNG,
		Passives:       passiveResolver,
		CritMultiplier: cfg.CritMultiplier,
		HitModAdjust:   hitModAdjust,
		// conditions §3: a stunned attacker skips its swings; a prone/
		// stunned/blinded defender is easier to hit. Both read the target's
		// live condition flags through the shared conditionImpact fold.
		Incapacitated: func(id combat.CombatantID) bool {
			return conditionImpact(combat.EntityIDOf(id)).Incapacitated
		},
		DefenderHitAdjust: func(id combat.CombatantID) int {
			return conditionImpact(combat.EntityIDOf(id)).DefenderVulnerability
		},
		// ranged-combat §3: a projectile swing spends one matching ammo unit.
		// Resolve the attacker → its ammo kind (from its wielded weapon's
		// combat.Stats) → spend one via the session-side AmmoConsumer, mapping
		// the consumed unit's masterwork grade to a to-hit bonus here (the
		// grade registry lives in the binary, keeping session decoupled). A
		// combatant without inventory (a mob) fires freely — mob ranged AI is
		// deferred (ranged-combat §9).
		// ranged-combat §5.3: a projectile's per-band to-hit falloff (per band
		// of distance) and the point-blank penalty firing at the melee band.
		RangeFalloff:      envIntOr("ANOTHERMUD_RANGE_FALLOFF", 2),
		PointBlankPenalty: envIntOr("ANOTHERMUD_POINT_BLANK_PENALTY", 4),
		AmmoFor: func(attackerID combat.CombatantID) (bool, int) {
			c, ok := combatLocator.LookupCombatant(attackerID)
			if !ok {
				return false, 0
			}
			consumer, ok := c.(session.AmmoConsumer)
			if !ok {
				return true, 0 // no inventory (mob) — fire freely
			}
			gradeKey, consumed := consumer.ConsumeAmmo(c.Stats().AmmoKind)
			if !consumed {
				return false, 0
			}
			bonus := 0
			if registries.Grades != nil {
				if g, gok := registries.Grades.Get(gradeKey); gok {
					bonus = g.WeaponToHit
				}
			}
			return true, bonus
		},
		MassiveDamage: &combat.MassiveDamageConfig{
			Threshold: massiveThreshold,
			DC:        cfg.MassiveDamageDC,
			FortBonus: victimFortBonus,
		},
	})
	// M7.6 wimpy phase — fires §5.2 flee when a combatant's HP%
	// drops to or below its WimpyThreshold property. Shares the same
	// FleeConfig the M7.6d `flee` verb uses; the verb path constructs
	// its own config from the same dependencies.
	fleeMover := combatMover{sessions: mgr, placement: placement, bus: bus}
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
		// conditions §5: a frightened combatant flees each round regardless
		// of HP. The wimpy phase honors this before its HP-threshold check.
		ForceFlee: func(id combat.CombatantID) bool {
			return conditionImpact(combat.EntityIDOf(id)).ForcesFlee
		},
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

	// WoT S2 Phase 3 — affinity potency. A channeler weaving a spell whose
	// Five-Power elements lie outside their gender-derived affinity does so at
	// reduced magnitude (soft scaling, never a hard gate). Inert outside the
	// WoT pack: an ability with no `elements` (every starter-world ability)
	// returns 1.0, as does a caster with no gender. The weak factor is tunable;
	// the default halves a weak weave's payload.
	affinityWeakFactor := envFloatOr("ANOTHERMUD_AFFINITY_WEAK_FACTOR", 0.5)
	if affinityWeakFactor <= 0 || affinityWeakFactor > 1.0 {
		// A weak factor must be in (0, 1]: 1.0 = no penalty, smaller = harsher.
		// Out of range is nonsense (zero/negative would zero the weave, >1 would
		// be a bonus the scaler ignores) — warn and fall back to the default
		// rather than ship a silently-broken tuning.
		slog.Warn("ANOTHERMUD_AFFINITY_WEAK_FACTOR out of range (0,1]; using default 0.5",
			slog.Float64("got", affinityWeakFactor))
		affinityWeakFactor = 0.5
	}
	casterAffinityPotency := func(sourceID, abilityID string) float64 {
		ab, ok := registries.Abilities.Get(abilityID)
		if !ok || len(ab.Elements) == 0 {
			return 1.0
		}
		gender := ""
		if a, ok := mgr.GetByPlayerID(sourceID); ok {
			gender = a.Gender()
		}
		return affinityPotency(gender, ab.Elements, affinityWeakFactor)
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
			amount = scaleByPotency(amount, casterAffinityPotency(e.SourceID, e.AbilityID))
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
			amount = scaleByPotency(amount, casterAffinityPotency(e.SourceID, e.AbilityID))
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
	// WoT S2 channeling knobs (default off → fantasy behavior unchanged): the
	// reserve-to-begin gate and spend-on-success. A channeling ruleset boot
	// sets ANOTHERMUD_CHANNEL_RESERVE_MULTIPLE (e.g. 2) and
	// ANOTHERMUD_SPEND_ON_SUCCESS=true; both apply server-wide.
	resolutionCfg := progression.DefaultResolutionConfig()
	resolutionCfg.SpendOnSuccess = envBoolOr("ANOTHERMUD_SPEND_ON_SUCCESS", false)
	reserveMult := envIntOr("ANOTHERMUD_CHANNEL_RESERVE_MULTIPLE", 1)
	if reserveMult < 1 {
		// SetReserveMultiple clamps to 1, but surface the misconfiguration so
		// an operator who typed 0 / a negative isn't silently ignored.
		slog.Warn("ANOTHERMUD_CHANNEL_RESERVE_MULTIPLE < 1; clamped to 1 (gate disabled)",
			slog.String("component", "server"), slog.Int("configured", reserveMult))
	}
	abilityPipeline.SetReserveMultiple(reserveMult)
	// WoT S2 Phase 2b: a stilled channeler is cut off from the Source and
	// cannot weave (spell-category abilities fizzle). The "stilled" effect is
	// WoT content; in non-WoT boots no one ever carries it, so the gate is
	// inert. Hardcoded id (the engine's one stilling concept), not env-tuned.
	abilityPipeline.SetChannelBlockEffect("stilled")
	abilityResolver := progression.NewAbilityResolver(
		resolutionCfg,
		proficiencyMgr, // ProficiencyReader (hit chance + cap)
		proficiencyMgr, // ProficiencyMutator (gain)
		pulseDelayTracker,
		effectMgr,
		abilityTargetHP,
		abilitySink,
		combatRNG, // shared single-goroutine RNG (see CONCURRENCY note above)
	)
	// conditions §4: save-gated ability effects (trip/bash) roll their entry
	// save through the emitting resolver so a resisted condition reads in-game.
	abilityResolver.SetSaveResolver(entrySaveResolver)

	// WoT S2 Phase 4 — affinity potency on the effect path. The same affinity
	// factor that scales weave damage/heal in the ability.used handler now also
	// scales a landed effect's entry-save DC, recurring-save DC, and modifier
	// magnitudes: bonds-of-air is easier to resist and warding's ac/hit buff is
	// smaller when woven outside the channeler's gender-derived affinity. Inert
	// outside the WoT pack (no `elements` ⇒ casterAffinityPotency returns 1.0).
	abilityResolver.SetPotencyProvider(casterAffinityPotency)

	// WoT S2 Phase 2 — the overchannel consequence. After a deliberately
	// overdrawn weave resolves, the channeler makes a Fortitude save whose DC
	// rises with how far past the safe reserve they reached; on a failure the
	// Power scours them — fatigued for a mild miss, stunned for a bad one,
	// scaling with the save margin. (The catastrophic "stilled" tier — losing
	// the Power outright — is Phase 2b.) Runs on the tick goroutine inside the
	// driver loop, so combatRNG stays single-goroutine like the other saves.
	const (
		overchannelBaseDC       = 12
		overchannelDeficitScale = 1  // +1 DC per point of mana drawn past the reserve
		overchannelStunMargin   = 5  // fail by ≥ this → stunned, else fatigued
		overchannelStillMargin  = 15 // fail by ≥ this → stilled (reached catastrophically far)
	)
	overchannelHandler := func(ctx context.Context, entityID, abilityID string, deficit int) {
		actor, ok := mgr.GetByPlayerID(entityID)
		if !ok {
			return // logged out between resolve and consequence
		}
		dc := overchannelBaseDC + overchannelDeficitScale*deficit
		out := combat.ResolveSave(combatRNG, actor.Saves().Fortitude, dc)
		if out.Success {
			_ = actor.Write(ctx, "You draw far more of the One Power than is safe — and hold it, the Source raging through you and away.")
			return
		}
		// Cascade by the miss margin: tired → stunned → stilled (cut off from
		// the Source). The stilled effect is WoT content (absent in non-WoT
		// boots, where channelers don't exist); the channel-block gate keys on it.
		margin := out.DC - out.Total
		effectID, msg := "fatigued", "The Power scours you as it tears free; you sag, wrung out and shaking."
		switch {
		case margin >= overchannelStillMargin:
			effectID, msg = "stilled", "You reach catastrophically too far. The Source rips away — and is simply GONE. You are stilled."
		case margin >= overchannelStunMargin:
			effectID, msg = "stunned", "You reach too far. The One Power turns on you — the world whites out and your knees give way."
		}
		if tpl, ok := registries.Effects.Get(effectID); ok {
			effectMgr.Apply(ctx, entityID, tpl, entityID, "overchannel")
		} else {
			// The cascade message implies a mechanical bite; if the content
			// pack is missing the effect the player would be misled into
			// thinking nothing happened. Surface it rather than swallow.
			logging.From(ctx).Warn("overchannel cascade effect missing from content",
				slog.String("event", "overchannel.cascade_missing"),
				slog.String("effect_id", effectID), slog.String("entity_id", entityID))
		}
		_ = actor.Write(ctx, msg)
	}

	// WoT S2 — the channel interrupt game. The cast notifier messages the
	// caster when a timed weave begins its warmup and when one is disrupted.
	// Both look the caster up by entity id (== player id) and write directly,
	// mirroring the overchannel handler. nil-safe inside the driver; supplied
	// unconditionally because it is inert until a `cast_time` weave is woven.
	castNotifier := castMessenger{mgr: mgr}
	abilityPhase := progression.NewAbilityPhaseDriver(
		actionQueueMgr, abilityPipeline, abilityResolver, abilitySources, abilitySink,
		overchannelHandler, castTracker, castNotifier,
	)
	// WoT S2 interrupt game: let the combat sink abort a mid-cast target's
	// weave when a blow lands. Set here (post-construction, like mgr/rest)
	// now that the tracker + notifier exist.
	combatSink.casts = castTracker
	combatSink.castNotify = castNotifier
	combatSink.incapacitated = func(bareID string) bool {
		return conditionImpact(bareID).Incapacitated
	}

	// Slice 3: moving rooms also disrupts a weave — you can't walk (or be
	// teleported/recalled) away mid-channel and keep the weave. Reuses the
	// sink's interrupt+notify path (it owns the tracker + notifier refs). Fires
	// on the connection goroutine that moved the player, concurrent with the
	// tick goroutine's cast advance; CastTracker.Interrupt is lock-guarded, so
	// the cast resolves OR interrupts exactly once. Skips presence-only moves
	// (From == To: link-dead reconnect) — only a real room change breaks focus.
	bus.Subscribe(eventbus.EventPlayerMoved, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.PlayerMoved)
		if !ok || e.From == e.To {
			return
		}
		combatSink.interruptCast(ctx, combat.NewPlayerCombatantID(e.PlayerID), "moved")
	})

	// Moving rooms drops hide concealment (visibility §3.1): you cannot stay
	// hidden while walking into a new room — only sneak (a later slice) is the
	// moving concealment that survives a move. Reuses the player.moved seam;
	// Reveal is lock-guarded and reports whether it actually dropped a hide,
	// so the entity.revealed (reason = moved) fires only for a hidden mover.
	// Skips presence-only moves (From == To: link-dead reconnect).
	bus.Subscribe(eventbus.EventPlayerMoved, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.PlayerMoved)
		// Skip same-room (link-dead reconnect) and login spawn (From == ""):
		// neither is a real room-to-room move that should break hide.
		if !ok || e.From == "" || e.From == e.To {
			return
		}
		actor, ok := mgr.GetByPlayerID(e.PlayerID)
		if !ok {
			return
		}
		if actor.Reveal() {
			bus.Publish(ctx, eventbus.EntityRevealed{
				EntityID:   e.PlayerID,
				SourceType: string(visibility.SourceHide),
				Reason:     "moved",
				Room:       e.To,
			})
		}
	})

	// Magical invisibility ends through the effect lifecycle (visibility §3.4):
	// when an `invisible`-flagged effect expires or is dispelled, surface an
	// entity.revealed(magical-invis) so visibility-aware subscribers learn the
	// bearer is back in view. The predicate already re-shows them on the next
	// render (it reads the flag live), so this is the explicit signal, not the
	// mechanism. The expired/removed event carries only the effect id, so the
	// invisible flag is recovered from the effect template registry.
	revealOnInvisEffectEnd := func(ctx context.Context, entityID, effectID, reason string) {
		tpl, ok := registries.Effects.Get(effectID)
		if !ok {
			return
		}
		invis := false
		for _, f := range tpl.Flags {
			if strings.EqualFold(f, command.InvisibleFlag) {
				invis = true
				break
			}
		}
		if !invis {
			return
		}
		var room world.RoomID
		if a, ok := mgr.GetByPlayerID(entityID); ok {
			if r := a.Room(); r != nil {
				room = r.ID
			}
		}
		bus.Publish(ctx, eventbus.EntityRevealed{
			EntityID:   entityID,
			SourceType: string(visibility.SourceMagicalInvis),
			Reason:     reason,
			Room:       room,
		})
	}
	bus.Subscribe(eventbus.EventEffectExpired, func(ctx context.Context, ev eventbus.Event) {
		if e, ok := ev.(eventbus.EffectExpired); ok {
			revealOnInvisEffectEnd(ctx, e.EntityID, e.EffectID, "expired")
		}
	})
	bus.Subscribe(eventbus.EventEffectRemoved, func(ctx context.Context, ev eventbus.Event) {
		if e, ok := ev.(eventbus.EffectRemoved); ok {
			revealOnInvisEffectEnd(ctx, e.EntityID, e.EffectID, "dispelled")
		}
	})

	// Slice 3: being incapacitated (stunned) mid-weave drops it — a control
	// weave like bonds-of-air deals no damage, so the hit path never sees it;
	// this effect-apply seam catches the disable. onEffectApplied gates on the
	// post-apply condition state, so only an incapacitating effect interrupts.
	bus.Subscribe(eventbus.EventEffectApplied, func(ctx context.Context, ev eventbus.Event) {
		e, ok := ev.(eventbus.EffectApplied)
		if !ok {
			return
		}
		combatSink.onEffectApplied(ctx, e.EntityID)
	})

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
	// Out-of-combat ability drain: the combat heartbeat's ability phase
	// only services engaged combatants, so ANY ability queued while idle
	// would otherwise queue forever — most commonly a self-buff/heal
	// between fights (`cast bless` / `cast heal`), but also an offensive
	// cast at a target that isn't yet an active combatant. This runs the
	// same ability phase driver, at the same cadence, for casters that
	// are NOT in combat — engaged ones stay the heartbeat's job, so no
	// double-drain. Registered after effect-tick so an effect applied
	// here isn't decremented in the same pulse. Only players queue
	// abilities (M9.4), so the bare queue key maps back to a player
	// combatant id.
	if err := loop.Register("ability-idle-tick", combatCadence, func(ctx context.Context, n uint64) {
		// Union the queued-action set with the in-flight-cast set: once a timed
		// weave (WoT S2) begins, its queue entry is popped, so a casting-but-
		// idle channeler would vanish from PendingEntities and its warmup would
		// never advance. Dedupe so an entity that is both queued and casting is
		// driven once per round. Both sets are bare PLAYER ids: only players
		// queue abilities or channel today (the same M9.4 invariant the queue
		// loop already relies on), so NewPlayerCombatantID is correct. A future
		// mob weave (cast_time on a mob ability) would need prefix-aware routing
		// here — guard with that, not silently, when mob casting lands.
		seen := make(map[string]struct{})
		drive := func(id string) {
			if _, dup := seen[id]; dup {
				return
			}
			seen[id] = struct{}{}
			cid := combat.NewPlayerCombatantID(id)
			if combatMgr.InCombat(cid) {
				return // engaged casters are the heartbeat's job (no double-advance)
			}
			abilityPhase(ctx, cid, combatMgr, n)
		}
		for _, id := range actionQueueMgr.PendingEntities() {
			drive(id)
		}
		for _, id := range castTracker.CastingEntities() {
			drive(id)
		}
	}); err != nil {
		return fmt.Errorf("register ability idle tick: %w", err)
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
		ChannelMap:    channelMap,
		Commands:      cmds,
		Players:       players,
		Manager:       mgr,
		Items:         entityStore,
		Placement:     placement,
		Contents:      contents,
		Templates:     registries.Items,
		Slots:         registries.Slots,
		Bus:           bus,
		Properties:    registries.Properties,
		Rarity:        registries.Rarity,
		Essence:       registries.Essence,
		Stacking:      stackingSvc,
		Disposition:   dispositionHook{e: evaluator},
		Combat:        combatMgr,
		CombatLocator: combatLocator,
		Flee: func(ctx context.Context, c combat.CombatantID) combat.FleeOutcome {
			return combat.Flee(ctx, c, fleeCfg)
		},
		// ranged-combat §3: the `throw` verb resolves one out-of-loop swing
		// with the wielded thrown weapon (full-STR via combat.Stats), reusing
		// the same hit/damage/death path as a normal swing.
		ResolveAttack: func(ctx context.Context, attacker, target combat.CombatantID, room world.RoomID) bool {
			return combatMgr.ResolveSingleAttack(ctx, attacker, target, room, combatRNG, cfg.CritMultiplier)
		},
		// M17.3: the `reload` verb re-discovers pack Lua from disk and
		// hot-swaps the scripting runtime — script-only, so world.World
		// and the content registries are untouched. DiscoverScripts'
		// compile-check rejects a syntax-broken edit before Reload tears
		// the live scripts down.
		ReloadScripts: func(ctx context.Context) (int, error) {
			fresh, err := pack.DiscoverScripts(ctx, cfg.ContentDir, cfg.Packs, scriptEngine)
			if err != nil {
				return 0, err
			}
			if err := scriptRuntime.Reload(ctx, fresh); err != nil {
				return 0, err
			}
			// Server-side confirmation so an operator watching the log sees
			// the reload land (the count itself also returns to the admin's
			// client via the reload verb). DiscoverScripts/Reload are
			// otherwise silent.
			n := fresh.Len()
			logging.From(ctx).Info("scripts reloaded",
				slog.String("event", "scripting.reload"),
				slog.Int("count", n))
			return n, nil
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
		Recipes:         registries.Recipes,
		Known:           knownRecipesMgr,
		Craft:           craftSvc,
		Gathering:       gatheringSvc,
		Biomes:          registries.Biomes,
		Grades:          registries.Grades,
		ForageTables:    registries.ForageTables,
		Effects:         effectMgr,
		EffectTemplates: registries.Effects,
		// skills §3: the pick verb runs on the command goroutine, so use the
		// concurrency-safe package-level roller (combat keeps its own).
		SkillRoller:     stdRoller{},
		ActionQueue:     actionQueueMgr,
		PulseDelay:      pulseDelayTracker,
		Casts:           castTracker,
		Races:           registries.Races,
		Classes:         registries.Classes,
		Feats:           registries.Feats,
		Alignment:       alignmentMgr,
		DefaultRace:     cfg.DefaultRace,
		RoleSeed:        cfg.RoleSeed,
		StartID:         cfg.StartRoom,
		ColorEnabled:    cfg.ColorDefault,
		Render:          colorRenderer,
		Help:            registries.Help,
		Quests:          questSvc,
		QuestStore:      questStore,
		Notifications:   notifMgr,
		TellResolver:    session.TellResolver{Manager: mgr, Players: players},
		RoleTargets:     session.RoleTargetResolver{Manager: mgr},
		GrantingRole:    cfg.GrantingRole,
		AdminRole:       cfg.AdminRole,
		DefaultXPTrack:  cfg.DefaultXPTrack,
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
		CreationFlow: session.NewCreationFlow(registries.Races, registries.Classes, registries.Backgrounds),
		Clock:        clk,
		Flood:        session.DefaultFloodConfig(),
		// Raising ChainCap multiplies a client's effective command throughput:
		// the flood gate counts one token per submitted LINE, and each line can
		// expand to ChainCap commands. Bump with that coupling in mind.
		ChainCap: envIntOr("ANOTHERMUD_CHAIN_CAP", command.DefaultChainCap),
		BadInput: badInput,
		LinkDead: linkDeadCfg,
		// M15.4b₂b: per-look weather ambience. The closure binds
		// weather.Service.Ambience; RenderRoom appends its output
		// after the room description in eligible rooms.
		Ambience: weatherSvc.Ambience,
		// Crafting Phase 5: weather-state query for the build verb's
		// wet-weather gate (crafting-and-cooking §4).
		WeatherState: weatherSvc.CurrentWeather,
		// Light-and-darkness resolver (light §2): pairs the default
		// light policy with the in-game clock so render/combat/movement
		// can gate on effective light. Friction wiring lands in Phase 5;
		// the verbs read its auto-light policy now.
		Light: lightResolver,
		// M22.3: loot ownership-window seam. NowTick reads the live tick
		// for the §4 window comparison against a corpse's creation tick;
		// the window itself is a wall-clock knob converted to ticks.
		NowTick:               loop.TickCount,
		CorpseOwnershipWindow: cadenceTicks(cfg.TickInterval, cfg.CorpseOwnershipWindow),
		DefaultMoveCost:       cfg.DefaultMoveCost,
		Login: login.Config{
			Accounts:        accounts,
			Players:         players,
			DefaultLocation: string(cfg.StartRoom),
			// World gate (character-identity §5): the active world set
			// derived by pack.Load. A returning character whose WorldID
			// isn't here is refused login.
			ActiveWorlds: registries.Worlds,
			// Per-phase idle timeout (login §6.1): bound interactive
			// reads so a connection that never responds is reaped. Driven
			// off the engine Clock (F3) so it's testable.
			Clock:       clk,
			IdleTimeout: cfg.LoginIdleTimeout,
			// Per-phase overrides (login §6.1): each absent phase falls
			// back to IdleTimeout above.
			PhaseIdleTimeouts: cfg.LoginPhaseIdle,
			// Name-gates (login §3): refuse reserved/system names at
			// character creation. nil NameGates → the default
			// reserved-names gate over this list.
			ReservedNames: cfg.ReservedNames,
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
	// Commit the current in-game time so a clean shutdown loses no
	// sub-hour remainder beyond the start of the current hour (§7).
	if err := clockStore.Save(gameClock.Snapshot()); err != nil {
		logging.From(flushCtx).Warn("gameclock.save: shutdown flush failed",
			slog.String("err", err.Error()))
	}
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
	Addr                   string
	WsAddr                 string
	WsPath                 string
	WsOriginPatterns       []string
	WsInsecureSkipVerify   bool
	LogLevel               string
	LogFormat              string
	TickInterval           time.Duration
	CombatCadence          time.Duration
	FleeCooldown           time.Duration
	CritMultiplier         int
	NonProficientPenalty   int
	MassiveDamageThreshold int
	MassiveDamageDC        int
	DefaultMoveCost        int
	CorpseOwnershipWindow  time.Duration
	CorpseLifetime         time.Duration
	CorpseDecayInterval    time.Duration
	CampfireLifetime       time.Duration
	CampfireDecayInterval  time.Duration
	BiomeAmbienceInterval  time.Duration
	ForageCooldown         time.Duration
	AutosaveInterval       time.Duration
	IdleSweepInterval      time.Duration
	LinkDeadSweepInterval  time.Duration
	// LoginIdleTimeout bounds every interactive login/creation read
	// (login spec §6.1). Zero disables it; the default closes a peer
	// that opens a connection but never responds.
	LoginIdleTimeout time.Duration
	// LoginPhaseIdle overrides LoginIdleTimeout for individual login
	// phases (login spec §6.1). nil — or a phase absent from it — uses
	// the global LoginIdleTimeout fallback.
	LoginPhaseIdle map[login.Phase]time.Duration
	// SlowTickThreshold warns when a tick's duration exceeds it
	// (time-and-clock §5). Zero falls back to the tick interval.
	SlowTickThreshold time.Duration
	// ReservedNames is the case-insensitive blocklist the default
	// name-gate refuses at character creation (login §3).
	ReservedNames []string
	// SustenanceDrainInterval / SustenanceDrainAmount tune the §4.4
	// hunger drain (economy-survival). Interval is how often the pool
	// drops; Amount is how much it drops each time. Defaults reproduce
	// DefaultSustenanceConfig (−1 every 30s at a 100ms tick = cadence 300).
	SustenanceDrainInterval time.Duration
	SustenanceDrainAmount   int
	ContentDir              string
	Packs                   []string
	SaveDir                 string
	StartRoom               world.RoomID
	DefaultRace             string
	DefaultXPTrack          string
	RoleSeed                map[string][]string
	GrantingRole            string
	AdminRole               string
	ColorDefault            bool
	LinkDead                session.LinkDeadConfig
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
		Addr:                    envOr("ANOTHERMUD_ADDR", ":4000"),
		WsAddr:                  envOr("ANOTHERMUD_WS_ADDR", ""),
		WsPath:                  envOr("ANOTHERMUD_WS_PATH", "/mud"),
		WsOriginPatterns:        wsOrigins,
		WsInsecureSkipVerify:    wsInsecure,
		LogLevel:                strings.ToLower(envOr("ANOTHERMUD_LOG_LEVEL", "info")),
		LogFormat:               strings.ToLower(envOr("ANOTHERMUD_LOG_FORMAT", "text")),
		TickInterval:            envDurationOr("ANOTHERMUD_TICK_INTERVAL", 100*time.Millisecond),
		CombatCadence:           envDurationOr("ANOTHERMUD_COMBAT_CADENCE", 3*time.Second),
		FleeCooldown:            envDurationOr("ANOTHERMUD_FLEE_COOLDOWN", 15*time.Second),
		CritMultiplier:          envIntOr("ANOTHERMUD_CRIT_MULTIPLIER", combat.DefaultCritMultiplier),
		NonProficientPenalty:    envIntOr("ANOTHERMUD_NONPROFICIENT_PENALTY", combat.DefaultNonProficientPenalty),
		MassiveDamageThreshold:  envIntOr("ANOTHERMUD_MASSIVE_DAMAGE_THRESHOLD", combat.DefaultMassiveDamageThreshold),
		MassiveDamageDC:         envIntOr("ANOTHERMUD_MASSIVE_DAMAGE_DC", combat.DefaultMassiveDamageDC),
		DefaultMoveCost:         envIntOr("ANOTHERMUD_MOVE_COST", 2),
		CorpseOwnershipWindow:   envDurationOr("ANOTHERMUD_CORPSE_OWNERSHIP_WINDOW", 60*time.Second),
		CorpseLifetime:          envDurationOr("ANOTHERMUD_CORPSE_LIFETIME", 5*time.Minute),
		CorpseDecayInterval:     envDurationOr("ANOTHERMUD_CORPSE_DECAY_INTERVAL", 3*time.Second),
		CampfireLifetime:        envDurationOr("ANOTHERMUD_CAMPFIRE_LIFETIME", 10*time.Minute),
		CampfireDecayInterval:   envDurationOr("ANOTHERMUD_CAMPFIRE_DECAY_INTERVAL", 5*time.Second),
		BiomeAmbienceInterval:   envDurationOr("ANOTHERMUD_BIOME_AMBIENCE_INTERVAL", 90*time.Second),
		ForageCooldown:          envDurationOr("ANOTHERMUD_FORAGE_COOLDOWN", 30*time.Second),
		AutosaveInterval:        envDurationOr("ANOTHERMUD_AUTOSAVE_INTERVAL", 30*time.Second),
		IdleSweepInterval:       envDurationOr("ANOTHERMUD_IDLE_SWEEP_INTERVAL", 30*time.Second),
		LinkDeadSweepInterval:   envDurationOr("ANOTHERMUD_LINKDEAD_SWEEP_INTERVAL", 30*time.Second),
		LoginIdleTimeout:        envDurationOr("ANOTHERMUD_LOGIN_IDLE_TIMEOUT", 60*time.Second),
		LoginPhaseIdle:          loginPhaseIdleFromEnv(),
		SlowTickThreshold:       envDurationOr("ANOTHERMUD_SLOW_TICK_THRESHOLD", 0),
		ReservedNames:           envCSVOr("ANOTHERMUD_RESERVED_NAMES", defaultReservedNames),
		SustenanceDrainInterval: envDurationOr("ANOTHERMUD_SUSTENANCE_DRAIN_INTERVAL", 30*time.Second),
		SustenanceDrainAmount:   envIntOr("ANOTHERMUD_SUSTENANCE_DRAIN_AMOUNT", 1),
		ContentDir:              envOr("ANOTHERMUD_CONTENT_DIR", "./content"),
		// Pack allowlist. Names the world pack(s) to boot; the loader pulls in
		// their dependency closure, so `ANOTHERMUD_PACKS=wot` also loads
		// tapestry-core. Defaults to the demo world (matching the default start
		// room). A boot selects ONE world: two world packs share bare-global
		// terrain/biome ids, so loading all at once collides — pick one
		// (e.g. `ANOTHERMUD_PACKS=wot ANOTHERMUD_START_ROOM=wot:the-green`).
		Packs:          envCSVOr("ANOTHERMUD_PACKS", []string{"starter-world"}),
		SaveDir:        envOr("ANOTHERMUD_SAVE_DIR", "./saves"),
		StartRoom:      world.RoomID(envOr("ANOTHERMUD_START_ROOM", "starter-world:town-square")),
		DefaultRace:    envOr("ANOTHERMUD_DEFAULT_RACE", "human"),
		DefaultXPTrack: envOr("ANOTHERMUD_DEFAULT_XP_TRACK", command.DefaultXPTrack),
		RoleSeed:       parseRoleSeed(envOr("ANOTHERMUD_ROLE_SEED", "")),
		GrantingRole:   strings.ToLower(strings.TrimSpace(envOr("ANOTHERMUD_GRANTING_ROLE", "admin"))),
		AdminRole:      strings.ToLower(strings.TrimSpace(envOr("ANOTHERMUD_ADMIN_ROLE", "admin"))),
		ColorDefault:   colorDefault(),
		LinkDead:       ld,
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

// loginPhaseIdleFromEnv builds the per-phase login idle-timeout overrides
// (login §6.1) from ANOTHERMUD_LOGIN_IDLE_TIMEOUT_{NAME,EMAIL,PASSWORD}.
// A phase whose env var is unset is omitted, so it falls back to the global
// ANOTHERMUD_LOGIN_IDLE_TIMEOUT. A set-but-invalid value (unparseable or
// non-positive) is warned about rather than silently ignored — a mistyped
// timeout is a misconfiguration the operator should hear about, since the
// silent fallback could leave a phase far more (or less) permissive than
// intended. Returns nil when no override is configured (the common case).
func loginPhaseIdleFromEnv() map[login.Phase]time.Duration {
	overrides := []struct {
		env   string
		phase login.Phase
	}{
		{"ANOTHERMUD_LOGIN_IDLE_TIMEOUT_NAME", login.PhaseName},
		{"ANOTHERMUD_LOGIN_IDLE_TIMEOUT_EMAIL", login.PhaseEmail},
		{"ANOTHERMUD_LOGIN_IDLE_TIMEOUT_PASSWORD", login.PhasePassword},
	}
	var m map[login.Phase]time.Duration
	for _, o := range overrides {
		v, ok := os.LookupEnv(o.env)
		if !ok || v == "" {
			continue
		}
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			slog.Warn("ignoring invalid login phase idle timeout; falling back to global",
				slog.String("component", "server"),
				slog.String("env", o.env), slog.String("value", v), slog.Any("err", err))
			continue
		}
		if m == nil {
			m = make(map[login.Phase]time.Duration)
		}
		m[o.phase] = d
	}
	return m
}

// defaultReservedNames seeds the login name-gate (login §3): privileged
// and system identities a new character must not impersonate. Matching is
// case-insensitive. Override the whole set via ANOTHERMUD_RESERVED_NAMES.
var defaultReservedNames = []string{
	"admin", "administrator", "root", "system", "console", "server",
	"god", "immortal", "staff", "gm", "dm", "guard", "guardian",
	"anothermud", "newbie",
}

// envCSVOr returns the comma-separated values of key (trimmed, empties
// dropped), or def when unset/blank.
func envCSVOr(key string, def []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

// envIntOr returns the integer value of key, or def when unset or
// unparseable.
func envIntOr(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

// envBoolOr reads a boolean env knob, falling back to def when unset or
// unparseable. Accepts the strconv.ParseBool vocabulary (1/t/true/0/f/false,
// case-insensitive).
func envBoolOr(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b
		}
	}
	return def
}

// envFloatOr reads a float env knob, falling back to def when unset or
// unparseable.
func envFloatOr(key string, def float64) float64 {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return def
}

// parseRoleSeed parses the ANOTHERMUD_ROLE_SEED operator config
// (roles-and-permissions §5) into a name→roles map. Format:
// "name:role,role;name:role" — entries separated by ';', a name and its
// comma-separated roles separated by ':'. Names and roles are lowercased
// and trimmed (the session layer normalizes again, but doing it here keeps
// the map keys canonical). Malformed entries are skipped, not fatal — a
// typo in the bootstrap config should not crash the server. Empty input
// yields a nil map (seeding disabled).
func parseRoleSeed(s string) map[string][]string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	out := make(map[string][]string)
	for _, entry := range strings.Split(s, ";") {
		name, roleList, ok := strings.Cut(entry, ":")
		name = strings.ToLower(strings.TrimSpace(name))
		if !ok || name == "" {
			continue
		}
		var roles []string
		for _, r := range strings.Split(roleList, ",") {
			if r = strings.ToLower(strings.TrimSpace(r)); r != "" {
				roles = append(roles, r)
			}
		}
		if len(roles) > 0 {
			out[name] = append(out[name], roles...)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// autolootSummary describes what an autoloot transferred — item names
// plus coins — as "a, b and 5 gold", so the killer sees their haul
// rather than a bare "you loot the corpse" (M22.4). Item names are
// undecorated here (autoloot is a convenience path off the tick
// goroutine, with no per-actor render Context); the manual loot verb
// keeps decoration.
func autolootSummary(items []*entities.ItemInstance, coins int) string {
	parts := make([]string, 0, len(items)+1)
	for _, it := range items {
		parts = append(parts, it.Name())
	}
	if coins > 0 {
		parts = append(parts, fmt.Sprintf("%d gold", coins))
	}
	switch len(parts) {
	case 0:
		return "nothing"
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
}

// upperFirst capitalizes the first rune of s so a flee announcement
// starting with a lowercased mob name ("a road bandit") reads as a
// sentence ("A road bandit flees…"). Rune-aware so a pack-authored
// UTF-8 name isn't corrupted; idempotent for already-capitalized names
// (players).
func upperFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
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
	contents     *entities.Contents
	templates    *item.Templates
	slots        *slot.Registry
	mobTemplates *mob.Templates
	races        *progression.RaceRegistry
	classes      *progression.ClassRegistry
	lootTables   *loot.Registry
	lootRoller   loot.Roller
	bus          *eventbus.Bus
	nodes        *gathering.NodeRegistry
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
// Steps 4 (stat derivation, M14.3 class growth), 7 (equipment
// instantiation/equip, §3.3), 8 (loot generation, M22.1), and 9 (ability
// proficiencies, copied at instantiation) are all wired in spawnMob.
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

	// M22.1: loot generation at spawn (mobs-ai-spawning §6.3 / §3.1
	// step 8). Roll the mob's loot table (if any) and file each rolled
	// item instance under the mob's id in the shared Contents index, so
	// the mob carries its loot from the moment it appears and any
	// observer can inspect a kill's drops beforehand. Unknown table or
	// item ids fail silently — consistent with the race/class spawn
	// convention.
	if tblID := tpl.LootTable; tblID != "" && b.lootTables != nil && b.contents != nil {
		if tbl, ok := b.lootTables.Get(tblID); ok {
			ids := loot.RollItems(tbl, b.lootRoller)
			generated := 0
			for _, itemID := range ids {
				itpl, terr := b.templates.Get(item.TemplateID(itemID))
				if terr != nil {
					logging.From(ctx).Debug("mob loot: unknown item template; skipped",
						slog.String("mob", string(inst.ID())),
						slog.String("loot_table", tblID),
						slog.String("item", itemID))
					continue
				}
				it, serr := b.store.Spawn(itpl)
				if serr != nil {
					logging.From(ctx).Warn("mob loot: item spawn failed",
						slog.String("mob", string(inst.ID())),
						slog.String("item", itemID),
						slog.Any("err", serr))
					continue
				}
				b.contents.Put(inst.ID(), it.ID())
				generated++
			}
			if b.bus != nil {
				b.bus.Publish(ctx, eventbus.MobLootGenerated{
					EntityID:   inst.ID(),
					RoomID:     roomID,
					TemplateID: templateID,
					Count:      generated,
				})
			}
		} else {
			logging.From(ctx).Warn("mob spawn: unknown loot table id; mob spawned without loot",
				slog.String("mob", string(inst.ID())),
				slog.String("template", templateID),
				slog.String("loot_table", tblID))
		}
	}

	// §3.3 / §3.1 step 7: equipment instantiation. Spawn each item on
	// the template's equipment list, apply its modifiers to the mob's
	// stat block under a per-item source key, and file it in the mob's
	// Contents so it drops into the corpse on death like loot. Missing
	// item templates are skipped silently (§3.3 step 1) — logged at Debug
	// for the content author, consistent with the loot convention above.
	if len(tpl.Equipment) > 0 && b.templates != nil {
		res, eerr := b.store.EquipMobAtSpawn(inst, tpl.Equipment, b.templates, b.contents, b.slots)
		if eerr != nil {
			// Spawn failure means a broken id generator — fail the spawn
			// rather than place a half-equipped mob.
			return "", fmt.Errorf("equip mob: %w", eerr)
		}
		for _, missing := range res.Missing {
			logging.From(ctx).Debug("mob equip: unknown item template; skipped",
				slog.String("mob", string(inst.ID())),
				slog.String("template", templateID),
				slog.String("item", missing))
		}
		// §3.7: items carried but not slot-equipped (no eligible free slot)
		// — their modifiers were not applied. Logged so a content author
		// notices a too-crowded equipment list.
		for _, skipped := range res.Skipped {
			logging.From(ctx).Debug("mob equip: no free slot; carried but not equipped",
				slog.String("mob", string(inst.ID())),
				slog.String("template", templateID),
				slog.String("item", skipped))
		}
		// §2.3 vitals-at-full: equipment may raise hp_max (the M14.1
		// OnMaxChange listener bumps Vitals.Max but never raises current),
		// so top the mob up once after equipping. A no-op for the common
		// case where gear only touches non-vital stats (str, hit_mod).
		if res.Equipped > 0 {
			if v := inst.Vitals(); v != nil {
				_ = v.Heal(v.Max())
			}
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
	// Resource-node rules (gathering.md §3.1) carry a node template id;
	// disambiguate node vs mob by registry lookup so the existing mob path
	// is untouched. A node spawns as a tagged placed entity, not a mob.
	if a.inner.nodes != nil {
		if _, ok := a.inner.nodes.Node(templateID); ok {
			return a.inner.spawnNode(ctx, templateID, roomID)
		}
	}
	return a.inner.spawnMob(ctx, templateID, roomID)
}

// wireBiomeNodeSpawnRules generates per-room resource-node spawn rules from
// each node-bearing biome's spawn table (gathering.md §3.1, biomes.md §2):
// for every room whose biome declares a node_spawn_table, it appends a
// world.SpawnRule (room, node template, count, reset interval) to that
// room's area. Runs once at boot before the scheduler starts, so the
// slice append needs no synchronization. Returns the rule count (logging).
func wireBiomeNodeSpawnRules(w *world.World, biomes *biome.Registry, nodes *gathering.NodeRegistry) int {
	if w == nil || biomes == nil || nodes == nil {
		return 0
	}
	count := 0
	for _, room := range w.Rooms() {
		if room == nil {
			continue
		}
		b, ok := biomes.Resolve(room.Terrain)
		if !ok || b.NodeSpawnTable == "" {
			continue
		}
		tbl, ok := nodes.SpawnTable(b.NodeSpawnTable)
		if !ok {
			continue
		}
		area, err := w.Area(room.AreaID)
		if err != nil {
			continue
		}
		for _, e := range tbl.Entries {
			area.SpawnRules = append(area.SpawnRules, world.SpawnRule{
				RoomID:         room.ID,
				NodeTemplateID: e.Node,
				Count:          e.Count,
				ResetInterval:  e.ResetInterval,
			})
		}
		count += len(tbl.Entries)
	}
	return count
}

// spawnNode mints a harvestable resource node (gathering.md §3.1) as a
// tagged placed ItemInstance carrying its harvest state (charges, yield
// table, required tool) as properties — the same placed-entity shape the
// campfire station uses. The §3.6 reset algorithm respawns a depleted node
// (removed from the store on its last harvest) exactly as it respawns a
// killed mob.
func (b *bootSpawner) spawnNode(_ context.Context, templateID string, roomID world.RoomID) (entities.EntityID, error) {
	n, ok := b.nodes.Node(templateID)
	if !ok {
		return "", fmt.Errorf("node template lookup: %q", templateID)
	}
	inst, err := b.store.SpawnContainer(
		n.Name,
		[]string{gathering.NodeTag, gathering.NoGetTag},
		n.Keywords,
		map[string]any{
			gathering.PropNodeTemplate:     n.ID,
			gathering.PropNodeCharges:      n.Charges,
			gathering.PropNodeYieldTable:   n.YieldTable,
			gathering.PropNodeRequiredTool: n.RequiredTool,
		},
	)
	if err != nil {
		return "", fmt.Errorf("spawn node: %w", err)
	}
	b.placement.Place(inst.ID(), roomID)
	return inst.ID(), nil
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

// Hostile is the read-only disposition query RenderRoom uses to redden
// hostile mobs. It dispatches nothing and writes no cache — just runs
// the §5.3 reaction algorithm against the (immutable) template and the
// supplied view.
func (d dispositionHook) Hostile(m *entities.MobInstance, playerID, playerName string, tags []string) bool {
	r, ok := d.e.ReactionFor(m, ai.PlayerView{ID: playerID, Name: playerName, Tags: tags})
	return ok && r == mob.ReactionHostile
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
	// casts + castNotify drive the WoT S2 interrupt game: a hit on a
	// mid-cast target aborts its weave (cost was tempo, not Power — under
	// spend-on-success an unresolved cast never spent). Both nil-safe; set
	// after construction once the tracker + notifier exist.
	casts      *progression.CastTracker
	castNotify progression.CastNotifier
	// incapacitated reports whether an entity is currently too disabled to act
	// (stunned). onEffectApplied uses it to break a weave when an incapacitating
	// condition lands. nil ⇒ the stun-interrupt path is inert.
	incapacitated func(bareID string) bool
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

	// Crafting interrupt (crafting-and-cooking §3): being drawn into a fight
	// breaks an in-flight timed craft for either participant. A mob id
	// resolves to no session and is skipped.
	if s.sessions != nil {
		for _, id := range []combat.CombatantID{e.AttackerID, e.TargetID} {
			if !strings.HasPrefix(string(id), combat.PlayerPrefix) {
				continue
			}
			if a, ok := s.sessions.GetByPlayerID(combat.EntityIDOf(id)); ok {
				a.CancelCraft(ctx)
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

	// WoT S2 — the channel interrupt game. A landed blow aborts the victim's
	// in-flight weave. A miss or evade does not (only OnHit fires here): a
	// dodged blow doesn't break concentration.
	s.interruptCast(ctx, e.TargetID, "hit")
}

// interruptCast aborts victimCID's in-flight weave, if any, and tells the
// caster. cause is the disruption keyword (today only "hit"; slice 3 adds
// "stunned"/"moved"). No Power is refunded because none was spent — a timed
// weave only draws the One Power when it RESOLVES, and an interrupted weave
// never does (the cost was tempo). casts + castNotify are an all-or-nothing
// feature pair (both set together, or both nil outside the WoT pack), so the
// outer nil-guard covers both; the inner guard is belt-and-suspenders.
func (s *productionCombatSink) interruptCast(ctx context.Context, victimCID combat.CombatantID, cause string) {
	if s.casts == nil {
		return
	}
	victimID := combat.EntityIDOf(victimCID)
	cast, ok := s.casts.Interrupt(victimID)
	if !ok || s.castNotify == nil {
		return
	}
	s.castNotify.OnCastInterrupted(ctx, progression.CastInterruptedEvent{
		SourceID:    victimID,
		AbilityID:   cast.AbilityID,
		AbilityName: cast.AbilityName,
		Cause:       cause,
	})
}

// onEffectApplied breaks entityID's in-flight weave when the just-applied
// condition leaves them incapacitated (WoT S2 interrupt game, slice 3). The
// rule is principled rather than a hand-listed set: a condition that stops you
// ACTING (stunned) drops your weave; one that merely hampers you (blinded,
// frightened, fatigued) does not — so any future incapacitating condition
// interrupts automatically. interruptCast no-ops when entityID isn't casting,
// so this is safe to call for every effect application.
func (s *productionCombatSink) onEffectApplied(ctx context.Context, entityID string) {
	if s.incapacitated == nil || !s.incapacitated(entityID) {
		return
	}
	s.interruptCast(ctx, combat.NewPlayerCombatantID(entityID), "stunned")
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

// OnRangedDry narrates a projectile swing skipped for want of ammo
// (ranged-combat §3): the wielder gets a "click — out of <ammo>" line and
// the room sees them grasping for ammunition. The attacker stays engaged, so
// a re-supply resumes fire on the next round with no re-engage.
func (s *productionCombatSink) OnRangedDry(ctx context.Context, e combat.RangedDry) {
	s.logger.Info("combat.ranged_dry",
		slog.String("attacker", string(e.AttackerID)),
		slog.String("weapon", e.WeaponName),
		slog.String("ammo_kind", e.AmmoKind),
		slog.String("room", string(e.RoomID)))

	an := s.nameOf(string(e.AttackerID))
	ammo := e.AmmoKind
	if ammo == "" {
		ammo = "ammunition"
	}
	s.tell(ctx, e.AttackerID, fmt.Sprintf("*click* — you are out of %s!", ammo))
	s.announce(ctx, e.RoomID, fmt.Sprintf("%s grasps for %s that isn't there.", an, ammo), e.AttackerID)
}

// OnBandChange narrates a range-band move (ranged-combat §5.2/§5.4): a melee
// foe auto-closing the distance, or a manual advance/withdraw. The subject sees
// it second-person, the room third-person, both keyed on the new band name.
func (s *productionCombatSink) OnBandChange(ctx context.Context, e combat.BandChange) {
	s.logger.Info("combat.band_change",
		slog.String("subject", string(e.SubjectID)),
		slog.String("opponent", string(e.OpponentID)),
		slog.String("band", e.NewBandName),
		slog.Bool("closing", e.Closing),
		slog.String("room", string(e.RoomID)))

	sn := s.nameOf(string(e.SubjectID))
	on := s.nameOf(string(e.OpponentID))
	if e.Closing {
		s.tell(ctx, e.SubjectID, fmt.Sprintf("You close on %s — now at %s range.", on, e.NewBandName))
		s.announce(ctx, e.RoomID, fmt.Sprintf("%s closes on %s, now at %s range.", sn, on, e.NewBandName), e.SubjectID)
	} else {
		s.tell(ctx, e.SubjectID, fmt.Sprintf("You open the distance from %s — now at %s range.", on, e.NewBandName))
		s.announce(ctx, e.RoomID, fmt.Sprintf("%s opens the distance from %s, now at %s range.", sn, on, e.NewBandName), e.SubjectID)
	}
}

// OnSaveResolved renders a saving-throw resolution to the creature
// (second person) and the room (third person), per saves §3/§4. The event
// is informational — the consumer that forced the save owns whatever
// consequence a failure brings; this method only narrates the roll.
func (s *productionCombatSink) OnSaveResolved(ctx context.Context, e combat.SaveResolved) {
	s.logger.Info("combat.save_resolved",
		slog.String("creature", string(e.CreatureID)),
		slog.String("save", e.SaveType),
		slog.String("cause", e.Cause),
		slog.Int("roll", e.Outcome.Roll),
		slog.Int("total", e.Outcome.Total),
		slog.Int("dc", e.Outcome.DC),
		slog.Bool("success", e.Outcome.Success),
		slog.String("room", string(e.RoomID)))

	cn := s.nameOf(string(e.CreatureID))
	save := upperFirst(e.SaveType)
	// Only the saving creature is excluded from the room announce — the
	// attacker who forced the save is a deliberate observer of the result
	// ("X resists."). The massive-damage consumer (saves §4) emits no
	// separate attacker line for the save, so there is no double-narration.
	if e.Outcome.Success {
		s.tell(ctx, e.CreatureID, fmt.Sprintf("<good>You resist! (%s save)</good>", save))
		s.announce(ctx, e.RoomID, fmt.Sprintf("%s resists.", cn), e.CreatureID)
		return
	}
	s.tell(ctx, e.CreatureID, fmt.Sprintf("<danger>You fail to resist! (%s save)</danger>", save))
	s.announce(ctx, e.RoomID, fmt.Sprintf("%s fails to resist.", cn), e.CreatureID)
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
	sessions  *session.Manager
	placement *entities.Placement
	bus       *eventbus.Bus
}

func (m combatMover) Move(ctx context.Context, id combat.CombatantID, dst *world.Room) bool {
	if dst == nil {
		return false
	}
	s := string(id)
	switch {
	case strings.HasPrefix(s, combat.MobPrefix):
		entityID := entities.EntityID(s[len(combat.MobPrefix):])
		if _, ok := m.placement.RoomOf(entityID); !ok {
			return false
		}
		m.placement.Place(entityID, dst.ID)
		// The flee announcement (with direction) is broadcast by the
		// combat.flee event subscriber, which carries the fled direction
		// the Mover doesn't receive.
		return true
	case strings.HasPrefix(s, combat.PlayerPrefix):
		playerID := s[len(combat.PlayerPrefix):]
		actor, ok := m.sessions.GetByPlayerID(playerID)
		if !ok {
			return false
		}
		from, _ := m.sessions.RoomOfPlayer(playerID)
		actor.SetRoom(dst)
		// Departure/arrival announcements are handled by the combat.flee
		// subscriber (it has the direction). Publish player.moved so
		// disposition + future hooks fire on flee-driven moves the same
		// way they do on verb-driven ones.
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
	case "stilled":
		return "You reach for the Source and find nothing — you are stilled, cut off from the One Power."
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

// castMessenger bridges progression.CastNotifier to session.Manager (WoT S2 —
// the channel interrupt game). It messages the caster when a timed weave begins
// its warmup and when an in-flight weave is disrupted, looking the actor up by
// entity id (== player id) and writing directly, like notifierAdapter. A
// missing actor (disconnected mid-cast) silently drops the message.
type castMessenger struct{ mgr *session.Manager }

func (m castMessenger) OnCastBegan(ctx context.Context, ev progression.CastBeganEvent) {
	a, ok := m.mgr.GetByPlayerID(ev.SourceID)
	if !ok {
		return
	}
	_ = a.Write(ctx, fmt.Sprintf("You begin to weave %s...", ev.AbilityName))
}

func (m castMessenger) OnCastInterrupted(ctx context.Context, ev progression.CastInterruptedEvent) {
	a, ok := m.mgr.GetByPlayerID(ev.SourceID)
	if !ok {
		return
	}
	_ = a.Write(ctx, fmt.Sprintf("Your weave of %s is disrupted!", ev.AbilityName))
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

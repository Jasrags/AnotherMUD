# ROADMAP

How we get from an empty scaffold to a playable MUD without building the
wrong thing first.

The approach is **bottom-up but in thin vertical slices**: each milestone
pulls just enough of every spec layer to exercise the substrate it adds.
Layers stay deliberately under-built between milestones; gaps are tracked
in each milestone's "Known gaps" list rather than papered over.

Milestone exit criteria are written as boxes so they double as a "are we
done" gate. Where a box maps to a spec's acceptance criteria, the spec
section is cited.

---

## Foundations

These are **cross-cutting conventions** that are painful to retrofit once
the codebase has any size. They're decided up front so every line written
from M0 onward follows them.

### Adopted from day one (binding conventions)

These are not code — they are rules every change must follow.

#### F1. `context.Context` is the first parameter

Any function that does I/O, runs on a tick, can be cancelled, or might
need a deadline takes `ctx context.Context` as its first parameter.

```go
// good
func (r *RoomRepo) Load(ctx context.Context, id RoomID) (*Room, error)

// bad — will hurt to retrofit
func (r *RoomRepo) Load(id RoomID) (*Room, error)
```

**Why now:** every cancellation, deadline, request-scoped logger, and
trace span flows through `ctx`. Adding it later means changing every
signature *and* every call site in the dependency chain.

**Exception:** pure functions (no I/O, no side effects, no time) don't
need it. When in doubt, take it.

#### F2. Structured logging via `log/slog`, logger lives on context

We use the stdlib `log/slog` package. Loggers are attached to and
retrieved from `context.Context` so log records automatically carry
the right scope (session id, entity id, tick number, etc.) without
threading the logger through every signature.

```go
// at session start
ctx = logging.With(ctx, slog.String("session_id", sess.ID))

// deep in the call stack
logging.From(ctx).Info("entered room", slog.String("room_id", string(rid)))
```

**Why now:** retrofitting structured logging means rewriting every
`fmt.Printf` and every log call site, plus inventing field names twice
(once ad-hoc, once "properly"). Pick the field vocabulary now and stick
with it.

**Field naming conventions** (lowercase, snake_case, stable):

| Field | Meaning |
|---|---|
| `session_id` | PlayerSession id |
| `account_id` | Account id |
| `entity_id` | Any entity (player, mob, item instance) |
| `room_id`, `area_id` | World identifiers |
| `tick` | Tick counter at log time |
| `event` | Bus event name when logging from a handler |
| `pack` | Pack name when logging from pack load/exec |
| `err` | Use `slog.Any("err", err)` not string-formatted |

#### F4. Error wrapping convention

Wrap with `fmt.Errorf("doing X: %w", err)`. Define sentinel errors as
package-level `var Err... = errors.New(...)`. Use `errors.Is` and
`errors.As` at the boundary that decides how to react.

```go
var ErrRoomNotFound = errors.New("room not found")

func (w *World) Room(id RoomID) (*Room, error) {
    r, ok := w.rooms[id]
    if !ok {
        return nil, fmt.Errorf("world.Room(%q): %w", id, ErrRoomNotFound)
    }
    return r, nil
}
```

**Why now:** "could not load room" with no chain and no sentinel is the
default if there's no convention. Once it's everywhere, normalizing it
is a slog.

### Deferred but committed (introduce when first consumer lands)

We've agreed on the shape — these get built when the first thing that
actually uses them is written, not before.

#### F3. `Clock` interface for testable time

The `time-and-clock` spec needs a tick loop driven by something other
than real wall clock so tests can advance time deterministically. We
will introduce a `Clock` interface (`Now() time.Time`, plus whatever
tick driver shape we settle on) **when the tick loop lands in M1**,
not before.

**Why deferred:** an abstraction designed before its first real
consumer is almost always the wrong shape. The convention is locked
in (no direct `time.Now()` calls inside the engine once `Clock`
exists); the implementation waits.

### Not front-loaded (revisit when needed)

- **Metrics / Prometheus** — a no-op interface might be fine but real
  wiring is premature.
- **Event bus implementation** — the specs need it but its shape will
  be wrong without a real subscriber. Lands with M1 or M2.
- **Full config loader** — env vars + a small struct until there are
  more than ~5 knobs.

---

## Milestones

Each milestone is small enough that a developer can hold the whole
slice in their head and explain what runs when a player connects.

### M0 — Echo telnet

**Slice:** raw TCP listener accepts a connection, reads input lines,
echoes them back. No world, no commands, no characters. A single
process you can `telnet localhost 4000` into.

**Why this first:** forces us to pick the connection seam from
`networking-protocols` and the session-vs-connection split from
`session-lifecycle` and *only* that. Establishes the F1/F2/F4
conventions in the smallest possible codebase.

**Exit criteria:**
- [x] `cmd/anothermud` listens on a configurable TCP port (`ANOTHERMUD_ADDR`)
- [x] Multiple concurrent telnet connections work (each in its own goroutine)
- [x] Each connection gets a logger with `session_id` field attached to ctx
- [x] Server handles `SIGINT`/`SIGTERM` by cancelling root ctx and closing listeners cleanly
- [x] An `IConnection`-ish interface exists with at least `Read`, `Write`, `Close`, `ID()` — even if telnet is the only implementation (`internal/conn.Connection`, name dropped `I` prefix per Go convention)
- [x] One integration test exercises connect → send line → receive echo → disconnect

**Status:** ✅ complete.

**Touches specs:** `networking-protocols` §IConnection, `session-lifecycle` §PlayerSession (minimal).

**Known gaps after M0** (deferred to later milestones, do not paper over):
- No telnet IAC option negotiation, no GMCP, no MSSP, no WebSocket.
- No tick loop, no `Clock` interface (deferred to M1 per F3).
- No real `PlayerSession` — the connection plays that role; M4 splits them.
- No flood protection, idle timeout, or link-dead detection (M4).
- `EchoHandler` lives in `internal/server/` for convenience; when M1 lands the command dispatcher, the handler likely moves to its own package.

---

### M1 — Two rooms

**Slice:** a hardcoded `World` with two rooms and one exit. Commands
`look`, `n`, `s`. A tick loop running at 100ms that does nothing
useful yet but exists so future milestones have somewhere to attach.

**Why this:** smallest possible exercise of `world-rooms-movement`
and `commands-and-dispatch`, and the first place `time-and-clock`'s
tick loop is real. This is also where F3 (`Clock` interface) lands.

**Exit criteria:**
- [x] `Room`, `Exit`, `World` types exist with the minimum fields needed (`internal/world`)
- [x] `Clock` interface introduced; `time.Now()` not called directly in engine packages (`internal/clock`; only `clock.RealClock` calls `time.Now`, engine packages take a `Clock`)
- [x] Tick loop runs at 100 ms cadence, driven by `Clock`, cancellable via ctx (`internal/tick`; cadence configurable via `ANOTHERMUD_TICK_INTERVAL`)
- [x] At least one tick-handler registration mechanism exists, even if empty (`tick.Loop.Register` + `SetPreTick`; main wires a no-op handler so the seam is exercised)
- [x] Command dispatcher parses input and routes to a `Command` handler (`internal/command`; exact → prefix resolution, case-insensitive)
- [x] `look`, `n`, `s`, `quit` work; unknown commands produce a clear message (`"Huh?"`)
- [x] A test advances the `Clock` and verifies tick handlers fire on schedule (`internal/tick/loop_test.go::TestLoop_HandlerFiresOnCadence`)

**Status:** ✅ complete.

**Touches specs:** `time-and-clock` §2-3, `world-rooms-movement` §2, `commands-and-dispatch` §2.

**Known gaps after M1:** no pack loading (world is hardcoded in `cmd/anothermud/world_seed.go`), no persistence (state dies on restart), no real entity model (a "player" is just a `connActor` in `internal/session`), no inventory, no other players visible to each other. Command registry is minimal: no aliases, priority, roles, arg types, chain (`;`), or repeat (`Nverb`) — these land when a real consumer needs them. Tick loop has no consumers besides a no-op; the in-game `GameClock` (time-and-clock §3) is not wired yet because no feature subscribes to `time.hour.change` yet.

---

### M2 — Load from disk

**Slice:** replace the hardcoded world with a content pack loader.
Packs are **data-only** at this point — YAML/JSON files describing
rooms, exits, areas. No scripting language picked yet.

**Why this:** confronts `scripting-and-packs` two-phase loading
and the registry pattern without committing to a script runtime.
Forces the pack-discovery and validation pipeline to exist.

**Exit criteria:**
- [x] Pack discovery walks a configurable content directory (`ANOTHERMUD_CONTENT_DIR`, default `./content`)
- [x] Pack manifest format defined (name, version, depends-on)
- [x] Two-phase load: phase 1 registers tags/properties, phase 2 instantiates content *(phase 1 is a manifest-list stub in M2 — tags/properties registries arrive with the milestone that needs them)*
- [x] Validation errors abort the load with actionable messages (which pack, which file, which field)
- [x] World, Area, Room registries are pack-populated
- [x] At least 3-4 rooms across 2 areas defined in pack files (`content/core/` ships town + wilderness, 4 rooms)
- [x] Reload story documented (even if "restart the server" for now) — see **Reload story** below
- [x] ANSI 16-color SGR support: small `internal/ansi` package (or equivalent) provides color helpers + a markup-or-helper format usable in pack-authored room text. Renderer applies it; per-session "color enabled?" flag exists (default on; off when `NO_COLOR` is set in the environment or the player runs `color off`). Plain-text fallback verified by integration test.

**Reload story (M2):** restart the process. Hot reload is deliberately
deferred — the loader is single-shot at boot, and `world.World` is not
designed for concurrent mutation while sessions are live. Operators
edit pack files, then `kill && go run ./cmd/anothermud` (or the
equivalent for their deploy harness). The reload primitive lands when
something forces the issue — most likely with the scripting runtime in
M2.5+ or with admin-level commands later.

**Touches specs:** `scripting-and-packs` §pack-discovery, §two-phase loading; `world-rooms-movement` §2.4 areas; `ui-rendering-help` (color subset only — themes, 256/truecolor, structured markup remain deferred).

**Known gaps after M2:** no scripting runtime (data-only packs), no cross-pack reference resolution beyond what alphabetical order gives us, no hot reload. Color is ANSI-16 only; no telnet IAC capability negotiation (assume-on with opt-out), no themes, no 256/truecolor, no per-pack palettes — those land with the full `ui-rendering-help` slice in M10.

**Decision point:** at the end of M2 we know whether packs really need
a scripting language, or whether data + Go-side handlers cover the
needed extensibility. Pick the runtime (or defer further) based on
real evidence.

---

### M3 — Save me

**Slice:** account file + player file persistence. Login flow:
name → email → password → enter game. Same character survives
server restart.

**Why this:** pulls `persistence` and `login` end-to-end. First
time we deal with atomic file I/O and the account/player split.

**Exit criteria:**
- [x] Account file shape matches `persistence` save/load surface (`internal/account`; minimum subset — verification fields present but workflow deferred)
- [x] Player file shape matches `persistence` save/load surface (`internal/player`; minimum subset — version, ids, name, location)
- [x] Password hashing uses a vetted algorithm (bcrypt via `golang.org/x/crypto/bcrypt`, default cost; documented in `internal/account`)
- [x] File writes are atomic (write-tmp → rotate-to-.bak → rename → drop-.bak in `internal/persistence`)
- [x] Login state machine matches `login` spec stages (`internal/login`; Name → returning Password | new Email → Password → confirmation → handoff)
- [x] Character location persists across restart (verified by `session.TestSessionPersistsLocationAcrossRestart`)
- [x] An autosave tick handler runs at a configurable cadence (`ANOTHERMUD_AUTOSAVE_INTERVAL`, default 30s; wired in `cmd/anothermud/main.go`)
- [x] Integration test: create account, log in, walk to room B, restart server, log in, verify in room B (`internal/session/session_test.go::TestSessionPersistsLocationAcrossRestart`)

**Status:** ✅ complete.

**Touches specs:** `persistence`, `login`, `character-creation` (minimal — pre-existing test character is fine until full wizard lands).

**Known gaps after M3** (carried forward into later milestones, do not paper over):
- No character creation wizard — name + email + password is the full new-player flow (spec §5.4 entity baseline is a single-room placement).
- No quest file, no inventory file, no stats block on the player save — those land with M5/M8 when there's live state worth serializing. The migration table is scaffolded empty; bump `player.CurrentVersion` and append a migration when the shape changes.
- No tagged-value envelope (`persistence` spec §4.4). Properties don't exist on the entity yet; the property registry (spec §2) lands when a feature needs typed props.
- No session takeover, no link-dead reconnect, no per-account concurrency cap — M4 territory. The session.Manager exists but only tracks live actors for autosave.
- No per-phase idle timeout in login (spec §6.1). `conn.Read` doesn't take a deadline yet; lands with M4's session-lifecycle work.
- No name-gates (spec §3). Name validation is hardcoded ASCII letters + length; the pluggable gate list is deferred.
- No email verification, no password reset / change command — out of scope per spec §1 non-goals.
- Telnet echo suppression during password entry is a bare `IAC WILL/WONT ECHO` byte write; full IAC negotiation lands with the networking-protocols slice.
- Autosave is single-shot synchronous (no snapshot-then-write split per spec §6.2). On a busy server this would stall the tick loop; revisit when M4 splits Session from Connection and a real entity model exists.
- `account.Service` uses a single `sync.Mutex` (bcrypt runs outside it, but `LoadByID` / `AddCharacter` still do file I/O under the lock). Latency-bounded by account count today; revisit with per-account locking when load justifies it.
- `player.Store` uses a single `sync.Mutex` across all character saves. Per-name locking would be more efficient; the simple cut is fine until concurrent autosave + disconnect-flush on many players is measurable.
- Account email rename has no path (spec §10 open question).

---

### M4 — Another player

**Slice:** multi-session correctness. Two players in the same room
see each other. Flood protection, idle timeout, link-dead detection
from `session-lifecycle`. First time concurrency really bites.

**Why this:** every system from here on assumes multiple sessions
and that entities can observe each other. Getting concurrency right
on a small surface now beats debugging races in M6 combat.

**Exit criteria:**
- [x] `SessionManager` tracks active sessions with concurrency-safe access (M4.1: multi-index byConn/byPlayerID/byName/byAccount/byRoom under RWMutex)
- [x] Player A sees "B has arrived" when B enters their room (M4.1)
- [x] Flood protection rejects abusive input rates per `session-lifecycle` §flood (M4.2: per-session token bucket, strike threshold disconnects)
- [x] Idle timeout disconnects per `session-lifecycle` §5 (M4.3: per-session lastInputAt + warned latch + tick handler at cadence 300; admin-tag exemption deferred until a role system exists)
- [x] Link-dead detection per `session-lifecycle` §7 (M4.4: phase enum on connActor, RemoveConnectionOnly / ReRegisterConnectionForSession on Manager, linkdead-cleanup tick handler at cadence 300, login reconnect path; Disabled fallback path preserved for tests)
- [x] Takeover (same account logs in twice) handled per `session-lifecycle` spec (M4.5: yes/no prompt on new conn when existing session is in Playing phase; performTakeover notifies + Removes + closes old conn; takenOver latch short-circuits the stale dispatchTeardown so the old conn's eventual EOF cannot tear down indices the new session now owns)
- [x] Race detector clean: `make test` (which uses `-race`) stays green under load (M4.6: stress tests in internal/session/stress_test.go exercise concurrent Add/Remove/SetRoom/SendToRoom/SendToAll/GetBy* across goroutines for a wall-budgeted run, plus 200×8-goroutine takeover-claim contention; both gated behind testing.Short() so the default suite stays fast)

**Touches specs:** `session-lifecycle` substantially, `world-rooms-movement` (entity tracking layer).

**Known gaps after M4:** no chat channels, no tells, no who list.

---

### M5 — Inventory & items

**Slice:** items exist as first-class entities. A player can pick up
a sword in town-square, look at their inventory, drop it, give it to
another player, equip it, and have all of that survive logout. First
content registry past rooms/areas, first mutable non-room entity, first
player-save migration.

**Why this:** every later milestone assumes items exist. Mobs (M6)
drop loot, combat (M7) consumes weapons and armor, abilities (M9)
target items, economy (M11) moves currency items. Keyword resolution
(`get 2.sword`, `look red potion`) is the parser every later command
will reuse for targeting. Getting the entity-instance model right
here sets the shape M6 mob instances will follow.

**Exit criteria:**
- [ ] Item template registry loads from packs alongside rooms/areas; templates carry id, name, type, tags, keywords, property bag, modifier list per `inventory-equipment-items` §2.1–§2.2. `content/core/` ships at least one weapon, one wearable, one container, and one stackable consumable.
- [ ] Item instantiation produces fresh entities with runtime ids distinct from template ids, transient `modifiers` rebuilt from template on load, `room_id` filtered out, modifier source keys tagged by entity id per §2.3. Two instances of the same template never collide.
- [ ] Item instances are tracked in a global entity index per `world-rooms-movement` §4: Track/Untrack on instantiation/destruction, Get-by-id resolves tracked → room-scan fallback, Get-by-tag uses the read/write double-buffer with a swap at the tick boundary, Get-by-type filters the tracked set.
- [ ] Slot registry accepts engine-baseline and pack-defined slots; snake_case enforced at registration; multi-cap slots use `name:index` keys packed from zero per §3.1–§3.2.
- [ ] `equip` / `unequip` move items between holder contents and equipment, apply/reverse stat modifiers by `equipment:<entity id>` source key, auto-swap on full slot, emit `entity equipped` / `entity unequipped` with base slot name per §3.3–§3.4.
- [ ] Inventory operations `get` / `drop` / `give` / `put` / `fill` validate atomically, emit one observable event each, return structured failure reasons per §4. Two-actor transfers (give, put) hold session locks in a consistent order — no deadlocks under race detector.
- [ ] Stacking service groups contents read-only (no entity merging), respects extension-key registration order, preserves first-seen position per §5.1–§5.2. Look-at-inventory renders "3 healing potions" instead of three lines.
- [ ] Keyword resolver handles `sword`, `red potion`, `2.ring`, `all`, `all.gem` with exact → prefix → substring precedence per §6; out-of-range ordinals return none; empty input never matches. Shared by every command that takes an item argument.
- [ ] Player save shape adds `inventory` (item entity list) and `equipment` (slot key → item entity) blocks. `player.CurrentVersion` bumps 1 → 2 and the first real entry lands in the migration table; v1 saves load cleanly (empty inventory, empty equipment).
- [ ] Race detector clean: `make test` stays green with stress tests covering concurrent get/drop/give between sessions in the same room.

**Touches specs:** `inventory-equipment-items` substantially, `commands-and-dispatch` (new builtins + keyword resolver as shared infrastructure), `persistence` (save shape v2 + migration), `world-rooms-movement` (rooms hold item ids).

**Known gaps after M5:** no shops, no currency auto-conversion (that's M11 — the `try-auto-convert` hook in §4.1 is a no-op stub for now), no rarity/essence colorization beyond plain text, no container weight limits enforced at runtime.

**M5.10 — Room item placement (post-M5.9c).** Adds an `items:` list
to room YAML and a `Spawner` interface to `pack.Load`; the loader
spawns and places one instance per id at boot, validating template
existence cross-pack. Closes the "no fillable sources placed in
content" gap (town-square now seeds its well via content, not via
a temp hook in `main.go`). Spec: world-rooms-movement §2.2.

**Decision — entity storage:** item instances live in a new `internal/entities` package, not on `world.World`. The package owns the tracking surface required by `world-rooms-movement` §4 (Track/Untrack, Get-by-id, Get-by-tag, Get-by-type, with the read/write double-buffer and a tick-handler swap at cadence 1). `world.World` keeps its boot-only-mutation invariant and stays a pure registry. `session.Manager` keeps its session indices. The three locks own disjoint state and do not nest.

Rationale:
- The tracking primitives are a coherent unit; bolting them onto `World` would give `World` two unrelated responsibilities.
- M6 mobs need the same machinery — `MobInstance` slots into the same `Store` as another `Entity` implementation with no refactor.
- Mirrors the `docs/specs/README.md` taxonomy that already separates registries from tracked entities.

Minimal initial shape:
```
internal/entities/
  entity.go   // Entity interface (ID, Type, Tags) + ItemInstance struct
  store.go    // Store with byID + byTag (read/write double-buffer) + byType
  tick.go     // SwapTagIndex tick handler at cadence 1
```

---

### Beyond M5

The exact ordering past M5 is less load-bearing because the substrate
is now real. Sketch of remaining vertical slices:

- **M6 — Mobs walking around:** `mobs-ai-spawning`, mob templates,
  area-driven spawning, the AI tick.
  - **M6.1 (landed):** mob template loader. New `internal/mob`
    package with `Template` + `Templates` registry; `MobFile` YAML
    schema with required (id/name/behavior) + scoped optionals
    (type defaults to "npc", disposition, tags, keywords,
    properties, stats, equipment ids). Pack loader decodes mob
    files via `content.mobs` glob and registers them. Equipment
    template ids are NOT validated at load (spec §3.1 specifies
    fail-silent-at-spawn).
  - **M6.4 (landed):** AI tick + first behaviors. New `internal/ai`
    package with `Registry`, `Dispatcher`, and built-in
    `stationary` + `wander` behaviors (spec §4.2-4.3). Dispatcher
    iterates `Store.GetByTag("mob")` each second and invokes each
    mob's behavior; errors are logged and skipped, never fatal.
    Wander picks a random exit on a 5-second interval, updates
    Placement, and broadcasts departure/arrival using the same
    phrasing as player movement. `MobInstance` gains the synthetic
    `mob` tag at instantiation so the dispatcher can iterate live
    mobs without parallel state. The town-square village-guard's
    behavior flipped from `stationary` to `wander` — guard now
    walks the four-room loop on its own. Active-area filter (§4.1)
    deferred: no perf issue at single-digit mob counts.
  - **M6.3 (landed):** room renderer surfaces placed entities. The
    `RenderRoom` signature widens to take optional `*Placement` +
    `*Store`; when both are supplied, a "You see here: a, b, c."
    line is inserted between description and exits, listing Placement-
    tracked entities (items + mobs) in insertion order. Nil-safe so
    existing tests and call sites that don't care about placement
    pass nil. Updates the three live-game call sites (look, movement
    arrival, link-dead reconnect). First user-visible payoff of M6:
    the village guard actually shows up in town-square.
  - **M6.2 (landed):** mob instantiation + boot-time spawn placement.
    `entities.MobInstance` (Entity impl) and `Store.SpawnMob` perform
    §2.3 instantiation steps 1-5 (build entity, drop implicit
    type-tag, copy stats+behavior+templateID into properties).
    `RoomFile.Mobs []string` parallels `Items`; loader gains
    `MobSpawner` interface and `applyMobPlacements` post-pass
    (validation + invocation). `bootSpawner` in `main.go` implements
    both interfaces and publishes `mob.spawned` on placement
    (§3.1 step 10). First content: `tapestry-core:village-guard`
    placed in `town-square`. Deferred (no consumer yet): §3.1
    step 4 stat derivation (M8), step 7 equipment instantiation,
    step 8 loot generation, step 9 ability proficiencies; §2.3
    steps 6-8 (patrol/idle/battle/disposition/scripts).
  - **M6.6 (landed):** area-driven respawn (spec §3.5–3.7). New
    `internal/spawn` package: `Tracker` indexes live mob instances
    by `(area, ruleIdx)`; `Manager` subscribes to `area.tick` and
    runs the §3.6 reset algorithm (purge dead → count → persistent
    ceiling → per-slot rare-swap → spawn-and-track); `Scheduler`
    accumulates game-tick deltas per area and emits `area.tick`
    events at `base × occupiedModifier` cadence (per-area override
    supported). `world.Area` gains `SpawnRules + ResetInterval`;
    pack loader decodes `spawn_rules:` + `reset_interval:` YAML
    and validates referenced rooms/templates at boot.
    `cmd/anothermud/main.go` wires `spawn.Manager` +
    `spawn.Scheduler`; new adapters `bootSpawnerAdapter`
    (entity-id-returning Spawner) and `presenceSource`
    (per-area player count via `world.RoomsInArea` +
    `session.Manager.PlayersInRoom`). New error sentinels:
    `ErrMissingSpawnRoom`. New events: `area.tick`. Sample
    content migrated: `tapestry-core:town` ships
    `spawn_rules: [{room: town-square, mob: village-guard,
    count: 1, tags: [persistent]}]` with a 30s reset; the
    village-guard now respawns automatically rather than being
    hardcoded at boot via `room.mobs:`. Mobs are NOT persisted
    across restart by design (spec §3.5: "tracking is purely
    runtime state"). Deferred (no consumer yet): death-driven
    purge (M7 combat — today's `alive` predicate only catches
    explicit untracks); per-area runtime modifier overrides via
    admin command (M10+).
  - **M6.5 (landed):** disposition reactions (spec §5). New
    `internal/ai/disposition.go` Evaluator with per-tick dedup +
    per-room reaction state caches, three hook points
    (OnPlayerEnteredImmediate aggro-only, OnPlayerEnteredDeferred
    full, OnMobEntered full). `mob.Template` gains
    `BaseDisposition` (static reaction string) and
    `DispositionRules` (default + ordered rule list); pack
    decoder accepts `base_disposition:` and `disposition_rules:`
    YAML blocks. New bus events: `player.moved` (clears per-room
    state on subscription), `mob.aggro`, `mob.wary`,
    `mob.friendly`. Movement command, login spawn, and link-dead
    reconnect publish player.moved and bracket RenderRoom with
    the immediate/deferred hooks. AI dispatcher resets the
    per-tick cache at the top of every tick; wander calls
    OnMobEntered after a successful move. `command.DispositionHook`
    interface keeps the command package free of an `ai` import;
    adapters in `cmd/anothermud/main.go` (`playerLookup`,
    `dispositionHook`) bridge `session.Manager` ↔ `ai.Evaluator`.
    Content: `village-guard.yaml` ships a sample rule set
    (default friendly; players tagged `outlaw` draw hostile).
    Deferred: alignment-based rules (M8 — accepted by the
    decoder but the evaluator currently treats alignment
    conditions as unmatchable); `mob.aggro` has no engine
    subscriber until M7 combat lands.
- **M7 — Hit something:** `combat`, engage/disengage, the heartbeat
  bucket, death.
  - **M7.1 (landed):** Combatant + vitals — the prerequisite slice.
    New `internal/combat` package: `Combatant` interface
    (`CombatantID`, `Name`, `Vitals`, `Stats`), mutex-protected
    `Vitals` type (HP/MaxHP with `ApplyDamage`/`Heal`/`SetMax`/
    `Percent`/`IsDead`/`Snapshot`), value-typed `Stats` block
    (`HitMod`, `AC`, `STR`), and `FromTemplateStats` helper that
    lifts the combat-relevant keys out of a mob template's Stats
    map with engine defaults (DefaultMobMaxHP=10, DefaultAC=10,
    DefaultSTR=10). `MobInstance` implements `Combatant`: vitals
    initialized from `stats.hp_max` at spawn, stat block derived
    from the template's `Stats` map. `connActor` implements
    `Combatant`: vitals from `DefaultPlayerMaxHP=20`, stats from
    `DefaultPlayerStats()` — both hardcoded until M8 progression
    lands real derivation. CombatantID namespaces are kept disjoint
    by prefix (`mob:<entityID>`, `player:<playerID>`) so a future
    unified Locator cannot cross-hit. New `consider <target>`
    command (alias `con`) is the end-to-end check: resolves self
    via name/me/self aliases, mobs via Placement + keyword
    resolver, and players via the existing Locator surface;
    surfaces HP/MaxHP, a coarse descriptor (uninjured → lightly →
    moderately → badly wounded → near death → dead), and AC.
    Player vitals are NOT persisted yet — every login starts at
    full HP. Persistence (player.Save schema bump) ships with the
    M7.5 death flow when there's something meaningful to save.
  - **M7.2 (landed):** CombatManager primitives. New
    `combat.Manager` owns `map[CombatantID][]CombatantID` combat
    lists under a single RWMutex; engage/disengage/disengage-all/
    primary-target promotion + queries (InCombat, PrimaryTargetOf,
    OpponentsOf snapshot copy, AllCombatants). Engage is symmetric
    + idempotent (already-engaged is the spec §2.1 no-op; tag
    refusals — safe-room, no-kill, flee-cooldown — deferred to
    M7.6). DisengageAll snapshots opponents before mutating per
    spec §2.3 and unconditionally emits CombatEnded for the
    target. Events dispatch through a small `combat.EventSink`
    interface, not directly through eventbus.Bus, because
    eventbus imports entities and entities imports combat
    (MobInstance carries Vitals fields from M7.1) — a combat →
    eventbus edge would close that cycle. cmd/anothermud wires
    a log-only sink today; M7.5/M7.6 swap in a real bus-backed
    adapter when there's a subscriber. New `combat.Locator`
    interface + `MapLocator` test helper resolves CombatantIDs
    back to live Combatants by prefix (`mob:` → entities.Store,
    `player:` → session.Manager via new
    `Manager.CombatantByPlayerID`); a logged-out player drops
    out of combat naturally via the §4.1 "missing target →
    disengage" branch — no cross-package teardown contract.
    `findMobByKeyword` from consider.go was promoted to a shared
    `findCombatantInRoom` (mob via Placement + keyword resolver,
    player via Locator) — closes M7.1 deferred #3. New
    `kill <target>` command resolves a Combatant in the room,
    refuses self / missing target / already-engaged / no-Combat-
    env, and calls Engage; emits attacker-first-person and
    room-broadcast lines.
  - **M7.3 (landed):** Heartbeat bucket + round skeleton. New
    `combat.Heartbeat` with four optional `PhaseFunc` slots
    (Ability, AutoAttack, Effects, Wimpy) registered as the
    `combat-tick` handler at `cfg.CombatCadence` (env
    `ANOTHERMUD_COMBAT_CADENCE`, default 3s). Each round snapshots
    `AllCombatants` once, then runs each non-nil phase over the
    snapshot in fixed spec-§3 priority order. Per-step
    `InCombat` re-check skips combatants that disengaged or died
    mid-round; mid-round Engage is NOT picked up until the next
    round. Phase panics are isolated per combatant so one bad
    callback can't abort the round. All four phases are nil in the
    production wiring today — M7.4 fills AutoAttack, M7.5/M7.6
    fill Effects/Wimpy, M9 fills Ability.
  - **M7.4 (landed):** Auto-attack swings. New `combat.NewAutoAttack`
    factory returns a `PhaseFunc` that the boot path slots into
    `combat.Phases.AutoAttack`. §4.1 pre-flight: skip on no target,
    pairwise-disengage on missing/dead/different-room target. §4.2
    swing count = `1 + extraAttackCount` (M9 stub returns 0). §4.3
    per-swing: defensive evade hook (M9 stub returns false), §4.4
    d20 with nat-1 fumble / nat-20 critical overrides, §4.5 dice
    damage via new `combat.DiceExpr` (NdM±K parser, `Roller`
    interface satisfied by `*math/rand/v2.Rand`) + `STRBonus`
    policy `(STR-10)/2`, clamped to ≥1, applied to `Vitals.
    ApplyDamage`. Emits `hit` / `miss` / `evade` events; emits
    `vital.depleted{hp}` and stops further swings when HP reaches
    0. New `combat.RoomLocator` interface bridges to placement +
    session.Manager.RoomOfPlayer (also new). Heartbeat snapshot
    now sorts players-first via `SortPlayersFirst` for §4.1's
    tie-break preference. Default unarmed damage `1d3` / weapon
    name `"fists"` ship in `combat.Stats.EffectiveDamage` and
    `EffectiveWeaponName`; real weapon plumbing arrives with
    equipment-stat work post-M8.
  - **M7.5 (landed):** Death flow + downstream wiring. New
    `combat.Vitals.ApplyDamageIfAlive` atomic primitive (closes
    M7.4 review obligation: single-lock liveness+damage so a
    future DoT/ability can't race the killing blow into a double
    VitalDepleted). New eventbus types `DeathCheck` (cancellable,
    §6.1), `Kill` (§6.3 step 1), `MobKilled` (§6.3 step 2);
    `productionCombatSink` (replacing M7.2's `loggingCombatSink`)
    owns `OnVitalDepleted` as the death entry — killer attribution
    per §6.2 (explicit attacker > victim's primary target > empty),
    cancellable death-check publish, kill/mob.killed emission,
    `combatMgr.DisengageAll(victim)`. Boot-time bus subscribers
    wire `mob.aggro` → `combat.Engage` (closes M6.5 deferred) and
    `mob.killed` → `entities.Store.Untrack` + `placement.Remove`
    (closes M6.6 deferred #1 — area respawn now fires on combat
    deaths). Player vitals persist via `player.Save` v5 + new
    `VitalsState` field; `Persist` syncs HP from the combat tick
    before the dirty check so damage taken between autosaves
    round-trips through disk.
  - **M7.6 (landed):** Flee + wimpy. New combat.TagSource
    (room + entity tag predicates) and combat.FleeCooldowns
    (tick-stamped, asymmetric: gates Engage as attacker not as
    target) plug into the M7.2 Manager via ManagerConfig; Engage
    now refuses safe-room attackers, no-kill targets, and flee-
    cooldown attackers per §2.1, and EngageWithReason surfaces the
    refusal code to the verb layer. New combat.Flee primitive runs
    the §5.2 sequence (no-flee tag check, exit enumeration,
    deterministic random-pick via supplied Roller, DisengageAll
    before Move, cooldown stamp after Move) with three new
    eventbus types: Flee, FleePrevented, FleeFailed. combat.Mover
    is the move seam (combat doesn't import session/entities for
    movement); cmd/anothermud wires connActor.SetRoom for players
    and placement.Place for mobs, plus broadcast announcements.
    combat.NewWimpy is the §5.1 heartbeat phase that triggers
    Flee on combatants whose Vitals.Percent() drops to or below
    their WimpyHolder threshold. connActor and MobInstance both
    satisfy WimpyHolder (player save grows a wimpy field without
    a schema bump since zero=disabled is indistinguishable from
    absent). New `flee` and `wimpy [<pct>|off]` commands.
    Heartbeat.Tick advances the cooldown tracker's "now" at the
    top of every round.
- **M8 — Get better:** `progression`, stats, levels, races, classes,
  tracks, alignment, training. Six substrates from `docs/specs/
  progression.md`. Slices land independently; later slices depend
  on earlier ones (classes need tracks + races + stat block;
  training needs races for caps).
  - **M8.1 (landed):** Stat block. New `internal/progression`
    package with `StatBlock` — string-keyed `StatType` base
    attributes (six classics + `hp_max`/`resource_max`/`movement_max`
    + the combat-derived `hit_mod`/`ac` slots carried alongside
    until M8.4 derives them properly), composes the existing
    `*stats.Block` for the sourced modifier set, and caches
    effective values behind a dirty flag under a RWMutex.
    Mutation surface: `SetBase`/`AdjustBase`/`AddModifier`/
    `AddModifiers`/`RemoveBySource`/`RebindSource`/`Invalidate`;
    read surface: `Base`/`Effective`/`AllEffective`/`HasSource`.
    Persistence shape splits into two snapshots: `BaseSnapshot`
    (new, ordered `[]{stat, value}`) and `ModifiersSnapshot` (the
    existing M5.6 `stats.Snapshot` round-tripped unchanged). New
    `StatDisplayNames` registry maps lowercased stat names to
    display strings with overrides → defaults → raw-name
    fallthrough; default set covers the canonical attributes plus
    the legacy `hit_mod`/`ac`/`damage` combat surface. `connActor`
    swaps its `*stats.Block` for a `*progression.StatBlock` seeded
    with `progression.DefaultPlayerBase()`; `Stats()` now derives
    `combat.Stats{HitMod, AC, STR}` from `Effective(...)` so
    equipment modifiers flow into auto-attack and consider
    without a separate sync step. Equip/unequip route through
    `AddModifiers`/`RemoveBySource`; respawn rebinding uses the
    new `RebindSource` wrapper. Player save v6: `stats_base`
    block added carrying `progression.BaseSnapshot`; v5 → v6
    migration is a no-op on dict content (absent block ⇒ engine
    defaults at construction). M8.1 scope-bound: `MobInstance`
    keeps its `combat.FromTemplateStats`-derived static block —
    wiring mobs to a StatBlock would create an `entities →
    progression → stats → entities` import cycle, deferred to
    M8.3 when races become the natural reason to inject base
    attributes into mobs (move `SourceKey` out of `entities`
    then). Vital clamping under max-affecting recompute is also
    deferred: current HP lives in `combat.Vitals` with its own
    internal clamp at M8.1 — StatBlock holds `hp_max` but does
    not own the current-vital integer that the spec §2.3
    re-clamp rule cares about. See m8-1-deferred-fixes.md.
  - **M8.2 (landed):** Tracks + XP/level engine. New
    `progression.TrackDef` + `progression.TrackRegistry` (priority-
    based override semantics, case-sensitive lookups); new
    `progression.ProgressionState` (per-entity level/XP maps with
    internal mutex + ordered Snapshot/Restore); new
    `progression.Manager` operating on State with
    `GrantExperience` (cascading through crossed thresholds),
    `DeductExperience` (floors at current-level threshold —
    cannot de-level, spec §5.5 open question recorded), `ResetTrack`,
    and the structured `GetTrackInfo` view (XpToNext / Overflow /
    CurrentLevelThreshold). Lazy init seeds `(level=1, xp=0)` on
    first interaction. `progression.EventSink` interface keeps
    progression free of an eventbus import (same pattern as
    combat.EventSink); cmd/anothermud wires a `progressionSink`
    adapter to `bus.Publish`. New eventbus types `XPGained`,
    `XPLost`, `LevelUp`, `TrackReset` plus matching constants.
    Pack loader gains `tracks` in ContentPaths + decode for
    `tracks/*.yaml` (M8.2 supports XPTable form only; XPFormula
    is reserved for Go-side construction until scripting lands).
    `content/core/tracks/adventurer.yaml` ships a 10-level
    triangular-curve track. Player save v7: `progression` block
    (ordered `[]{name, level, xp}`); v6 → v7 migration is a
    no-op. `connActor` holds `*ProgressionState` and exposes
    `GrantXP` / `DeductXP` / `TrackInfo` wrapper methods that flip
    the dirty bit so autosave commits the new state. New admin
    `xp [<amount> [<track>]]` verb: no-args lists every track's
    TrackInfo; arg form self-grants for end-to-end probing. Role
    gate + target-by-name form land with the role system (M10+).
    Deferred: class subscriber for `LevelUp` (M8.4 — the
    StatGrowthSubscriber and ClassPathProcessor land then);
    `OnLevelUp` per-track callback exists but no track wires it
    until M8.4; M8.2 cannot de-level via `DeductExperience` and
    `XPLost` callers don't yet test that branch end-to-end (the
    spec open question is unresolved — see
    m8-2-deferred-fixes.md).
  - **M8.3 (landed):** Races. New `progression.Race` +
    `RaceRegistry` with priority-based override semantics and
    case-insensitive lookups (id lowercased at registration);
    StatCaps and RacialFlags are deep-cloned on Register so
    caller-side post-registration mutation can't bleed through.
    `cost.AdjustCost(base, race)` lives next door and returns
    `max(0, base + race.CastCostModifier)` with nil-race
    pass-through (consumed by M9 abilities). Pack loader gains
    `races` in ContentPaths + `decodeRace` reading
    `races/*.yaml` (validates non-empty id, rejects negative
    stat caps, normalizes stat-cap keys to lowercase StatType).
    `content/core/races/{human,dwarf}.yaml` ship with distinct
    stat-caps, categories, and racial-flag sets. `mob.Template`
    gains an optional `race` string; `decodeMob` lowercases on
    decode. New `MobInstance.RaceID()` + `MobInstance.
    ApplyRacialFlags(flags, alignment)` (primitive-typed to
    avoid an entities → progression import cycle); the boot
    spawner resolves the race registry after `Store.SpawnMob`
    and applies flags + seeds the `alignment` reserved property
    key. Unknown race id is a fail-silent debug log per spec
    §3.1 mob-spawn convention. Player save v8: `race` string;
    v7 → v8 migration is a no-op. `session.Config` grows
    `Races` + `DefaultRace`; new `applyRace` resolution
    function: saved id wins, empty falls through to
    `cfg.DefaultRace` (configured via `ANOTHERMUD_DEFAULT_RACE`,
    defaulting to "human"), unknown id leaves the actor
    raceless (raceID="", no tags) rather than erroring. The
    resolved id round-trips back to the save so the default
    sticks on the next Persist. `connActor.Tags()` surfaces
    racial flags to the disposition evaluator via the new
    `session.PlayerInfo` projection (closes the M6.5 deferred
    "players have no Tags field yet" note). M8.1's deferred
    MobInstance-StatBlock + SourceKey extraction did NOT land
    this slice — race contributes tags + alignment + cast-cost
    + (training-time) caps, none of which require live
    derivation through a per-mob StatBlock. The cycle break is
    re-targeted to whenever a consumer actually needs live
    stat derivation on mobs (M9 effects, or mob equipment if
    that ever lands).
  - **M8.4 (landed) — Classes (path + growth).** New `progression.Class` +
    `ClassRegistry` (spec §4). Class carries stat-growth map
    (StatType → dice expression — reuses M7.4's `combat.DiceExpr`
    parser), growth-bonuses map (StatType → source StatType),
    bound track name, `path` list of `(level, abilityId,
    unlockedVia)` entries, trains-per-level integer, allowed-
    categories / allowed-genders lists. Two new subscribers on
    `progression.level.up`: `ClassPathProcessor` (also listens
    to `character.created` and treats it as level 1 — see open
    question on character-created event source; for M8.4 wire it
    to a one-shot publish at character-creation handoff so the
    plumbing exists before M12) and `StatGrowthSubscriber`
    (rolls dice + applies growth-bonus, increments base
    attributes via StatBlock, credits trains-per-level). Path
    entries with non-empty `unlockedVia` are skipped — those are
    quest/script-owned and land later. Ability grants in the
    path call out to abilities (M9) via a thin interface;
    pre-M9 wiring logs the grant and queues a "you have
    learned" notification without actually teaching anything
    (M9 fills the proficiency side). Eligibility query
    `GetEligibleClasses(raceCategory, gender)` lands but is
    consumed only by the M12 character-creation wizard;
    M8.4 ships it with a unit test.
    - [x] Classes load from packs into `ClassRegistry`; case-
          insensitive id lookup; priority overrides; get / get-all /
          has / get-eligible-classes queries.
    - [x] `ClassPathProcessor` runs at level 1 on a
          `character.created` event AND on every
          `progression.level.up` whose track equals the class's
          bound track (case-insensitive); skips entries with
          non-empty `unlockedVia`; logs and skips unknown ability
          ids without raising.
    - [x] `StatGrowthSubscriber` rolls dice for each entry, adds
          `max(0, (sourceStatValue - 10) / 2)` from the
          *effective* source-stat value when growth-bonus declares
          a source, increments base attributes via StatBlock,
          credits `trainsPerLevel` to `trains_available`.
    - [x] Stat-growth handler's track-gating behavior is
          documented (open question §10 — picked "no gate" today;
          recorded in `level_up.go` ApplyStatGrowth doc + the
          bus-wiring comment in `cmd/anothermud/main.go`).
    - [x] `content/core/` ships at least one class with a non-
          empty path, growth map, and bound track; an
          end-to-end integration test grants enough XP to that
          class's bound track to cross 2-3 thresholds and
          asserts both subscribers fired.
    - [x] Player save v9 carries `class` id + `trains_available`
          integer; v8 saves load cleanly (no class, zero
          trains).
  - **M8.5 — Alignment.** New `progression.AlignmentManager`
    backing the alignment integer property on entities per spec
    §6. Bounded by configured min/max (defaults -1000 / +1000),
    bucketed by configured evil/good thresholds. `Bucket` /
    `Set` / `Shift` operations; `Set` is silent (admin path),
    `Shift` is the gameplay path with the cancellable
    `alignment.shift.check` event (listeners may set `cancel`
    or rewrite `suggestedDelta`). On apply: write value, sync
    `alignment_evil` / `alignment_neutral` / `alignment_good`
    tag (exactly one present at a time), append to bounded
    `alignment_history` list (capacity 20), emit
    `alignment.shifted` and — when bucket changed — also
    `alignment.bucket.changed`. `ResolveBuckets(set)` helper
    translates `{evil, good, neutral}` set names to `(min, max)`
    range for disposition rules and ability gates. Admin entities
    bypass Shift entirely. Closes the M6.5 disposition deferral:
    the AI `Evaluator` consumes the helper and starts matching
    alignment conditions instead of treating them as unmatchable.
    Player save grows `alignment` integer + `alignment_history`
    (or the latter stays runtime-only — recorded in the spec's
    open questions; pick one here).
    - [ ] Alignment integer clamped to configured `[min, max]`
          on every write.
    - [ ] Bucket tag is present and unique on every entity the
          manager has touched; idempotent re-sync on every
          `Bucket` call.
    - [ ] `Set` emits no events and does not append history;
          `Shift` emits `alignment.shift.check` (cancellable +
          rewritable delta), then on actual change emits
          `alignment.shifted` and conditionally
          `alignment.bucket.changed`.
    - [ ] `Shift` is a no-op for entities carrying the `admin`
          role tag.
    - [ ] `ResolveBuckets` returns the correct `(min, max)`
          range for every subset of `{evil, neutral, good}`
          including degenerate cases per spec §6.6.
    - [ ] `village-guard.yaml` (or a sibling) gains a sample
          alignment-conditioned disposition rule that fires
          end-to-end; the M6.5 deferral is closed.
    - [ ] Player save v10 carries `alignment`; v9 saves load
          cleanly (alignment = 0 / neutral bucket).
  - **M8.6 — Training.** New `progression.TrainingManager` with
    both operations from spec §7: `TryPractice(entityId,
    abilityId)` (cap-tier ladder Novice/Apprentice/Journeyman/
    Master at 25/50/75/100, exact-next-tier-only, catch-up
    boost when proficiency < prior cap) and `TryTrain(entityId,
    stat)` (safe-room gate optional, trainable-list gate, race
    cap gate, decrement `trains_available`, increment base
    attribute, invalidate StatBlock). Trainers are mobs carrying
    the `skill_trainer` tag + a `TrainerConfig` (tier +
    teachable ability ids). New commands `practice <ability>`
    and `train <stat>` that resolve via TrainingManager and
    render result messages. `TrainerConfig` decoder added to
    pack mob loader. Practice is a no-op on the proficiency
    side until M9 (logs the would-have-trained ability + tier);
    stat training is fully wired against the StatBlock from
    M8.1 + race caps from M8.3.
    - [ ] Trains-available defaults to 0 and increases only via
          class level-up credits (M8.4).
    - [ ] Practice requires a learned ability (or a deferred-to-
          M9 stub) AND a matching in-room trainer; cannot skip
          tiers; does not consume a train.
    - [ ] Stat training enforces safe-room rule only when
          `RequireSafeRoomForStats` config is true; honors
          per-race stat cap (default 25 when the race doesn't
          declare); fails with structured result for
          NotTrainable / UnsafeRoom / NoTrains / AtRaceCap.
    - [ ] Catch-up boost bumps proficiency toward *prior* cap
          (clamped), not the new cap.
    - [ ] `content/core/` ships at least one trainer mob with
          `TrainerConfig` so `practice` and `train` commands are
          end-to-end testable.
    - [ ] A handful of integration tests exercise: grant XP →
          level up → trains credited → `train str` succeeds →
          base STR increases → effective combat hit reflects it.
- **M9 — Do something:** `abilities-and-effects`, the action queue,
  effects.
- **M10 — Quests & UI polish:** `quests`, `ui-rendering-help` themes,
  panels, 256/truecolor, and telnet capability negotiation. (Basic
  ANSI-16 color already landed in M2.)
- **M11 — Survive:** `economy-survival`, currency, shops, sustenance.
- **M12 — Character creation wizard:** the full `character-creation`
  flow now that the systems it touches exist.

Each of these will get its own M2-style exit-criteria section when it's
the next milestone in flight.

---

## How to use this document

- The **current milestone** is whichever section above has unchecked
  boxes and no later milestone has been started.
- When `orient me` is requested (see CLAUDE.md Developer Learning
  Protocol), reflect the actual state of the codebase against this
  doc's milestones.
- When a milestone's exit criteria are all checked and the slice runs,
  the milestone is done. Update "Known gaps" lists in later milestones
  if implementation revealed something the spec didn't anticipate.
- When code starts to diverge from a spec, surface it explicitly per
  the Drift detection section of the Developer Learning Protocol.

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
- **M7 — Hit something:** `combat`, engage/disengage, the heartbeat
  bucket, death.
- **M8 — Get better:** `progression`, stats, levels, races, classes,
  tracks.
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

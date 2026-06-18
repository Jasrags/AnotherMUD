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

**Companion docs.** This ROADMAP is the **done-log + active milestone**.
For **open work and candidate next themes**, see [`BACKLOG.md`](./BACKLOG.md).
For **behavior contracts**, see `docs/specs/`. The old `TAPESTRY-GAP-MATRIX.md`
and `THEME-AXIS-PLAN.md` are superseded by `BACKLOG.md` and now live under
`docs/archive/` (the five themes they framed have all shipped — M13–M17).

## Status (as of 2026-06-16)

- **Done:** **M0–M28.** The five original themes — A/M13 (Social), B/M16
  (Modern-Client + GMCP), C/M15 (World-Depth), D/M17 (Content-Authoring +
  Lua scripting), E/M14 (Engine-Debt) — plus **M18** (Command & UI polish —
  prompt, who, bad-input tracker, chaining/repeat, auto-help),
  **M19** (Roles & Administration), **M20** (Item Decorations — rarity &
  essence), **M21** (Item Stacking), **M22** (Loot and Corpses —
  spawn loot → corpse → loot / get-from / look-in / autoloot → decay), and
  **M23** (Room Coordinates — derive-by-default area-local (x,y,z) from the
  exit graph + pins, GMCP exposure, and a builder `roomdata` look view), and
  **M24** (Player Maps — persisted fog-of-war visited set, the shared
  `world.LocalWindow` query, the active `minimap` toggle + the `map` verb,
  and the Mudlet GMCP surface), **M25** (Equipment slots — eligibility,
  footprint, contention, mob capacity), **M26** (Engine Debt II —
  door-key boot validation, passive gain stat factor, GMCP wizard panel),
  and **M27** (Crafting & Cooking MVP — recipes, crafting-skill proficiency,
  the quality roll, cooking→well-fed, fixed/portable/campfire stations), and
  **M28** (Visibility + Hidden exits — hide/sneak/wizinvis/magical-invis, the
  per-observer can-see predicate, `search` + secret-exit discovery). Since M28,
  three orthogonal features shipped 2026-06-16: **movement cost / encumbrance**
  (biome-weighted step cost + carry-from-Strength, save v24), **account-first
  login + character roster** (login by account username), and **character
  world-locking** (a `WorldID` stamp gating login to a server running that
  world, save v23).
  The core loop, world, combat, progression, economy, quests, scripting,
  sessions, modern-client, roles/admin, decorations, stacking, loot, room
  coordinates, player maps, equipment slots, crafting, visibility, and
  movement cost all work.
- **Active — M29 Player Trade** (specced, `BACKLOG.md` §1 → in build). The
  escrow primitive + direct trade + buyout auction house. **Phase 0 (the
  escrow/atomic-transaction primitive, `internal/escrow`) + Phase 1 (the audit
  log) + Phase 2 (direct trade — `internal/trade`, the verbs, teardown hooks)
  shipped — Ship line A (player-to-player trade) is complete.** Next is Ship
  line B, the auction house (Phases 3–8). Plan: `docs/plans/trade-plan.md`;
  details in the M29 section below.
- **Active arc — WoT Mechanics EPIC** (post-M27, additive sub-epics; Decision 0
  = posture A, translate onto tick/chance). Shipped so far: **S1 weapon-identity**,
  **S1.H masterwork** (item quality grades — masterwork/power-wrought, delivered
  through the to-hit / damage / check-penalty / skill-check seams, `internal/grade`),
  **S2 The One Power** (Phases 0–4 — pool substrate, channeler class + weaves +
  `channel`, overchannel cascade, affinities/gender, the interrupt game +
  Initiate/Wilder split, save v21/v22),
  **S3 skills** (use-based proficiency + skill-check primitive + lockpicking),
  **S5 conditions** (Core 5), **S6 saves** (Fort/Reflex/Will), the **multiclass
  seam** (class widened to a list, save v18), **S9 backgrounds** (the
  creation-origin starting package — skills/items/gold, save v19), and **S4 feats**
  (the player-choice feat selection engine — slots, the `feat`/`feats` verbs, all
  six grant kinds, authored background feats, save v20). The per-sub-epic done-log
  + remaining S2-Phase-4+/S7/S8/S10/S11 candidates live in the EPIC tracker
  `docs/themes/wot-mechanics-epic.md`; the arc is summarized below after M27.
- **Specs ahead of code.** Behavior contracts written without
  implementation, still awaiting a milestone: `tag-observers`, `faction`,
  the trade trio (`trade-escrow` / `direct-trade` / `auction-house`), and
  the remaining WoT EPIC S1 increments (`armor-depth`, `size-and-wielding`,
  `ranged-combat`). They sit in `BACKLOG.md` §1. The earlier
  `roles-and-permissions` / `admin-verbs` / `item-decorations` contracts
  have since shipped (M19 / M20), as have `crafting-and-cooking` (M27) and
  `visibility` / `hidden-exits` (M28).

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
- [x] Item template registry loads from packs alongside rooms/areas; templates carry id, name, type, tags, keywords, property bag, modifier list per `inventory-equipment-items` §2.1–§2.2. `content/core/` ships at least one weapon, one wearable, one container, and one stackable consumable.
- [x] Item instantiation produces fresh entities with runtime ids distinct from template ids, transient `modifiers` rebuilt from template on load, `room_id` filtered out, modifier source keys tagged by entity id per §2.3. Two instances of the same template never collide.
- [x] Item instances are tracked in a global entity index per `world-rooms-movement` §4: Track/Untrack on instantiation/destruction, Get-by-id resolves tracked → room-scan fallback, Get-by-tag uses the read/write double-buffer with a swap at the tick boundary, Get-by-type filters the tracked set.
- [x] Slot registry accepts engine-baseline and pack-defined slots; snake_case enforced at registration; multi-cap slots use `name:index` keys packed from zero per §3.1–§3.2.
- [x] `equip` / `unequip` move items between holder contents and equipment, apply/reverse stat modifiers by `equipment:<entity id>` source key, auto-swap on full slot, emit `entity equipped` / `entity unequipped` with base slot name per §3.3–§3.4.
- [x] Inventory operations `get` / `drop` / `give` / `put` / `fill` validate atomically, emit one observable event each, return structured failure reasons per §4. Two-actor transfers (give, put) hold session locks in a consistent order — no deadlocks under race detector.
- [x] Stacking service groups contents read-only (no entity merging), respects extension-key registration order, preserves first-seen position per §5.1–§5.2. Look-at-inventory renders "3 healing potions" instead of three lines.
- [x] Keyword resolver handles `sword`, `red potion`, `2.ring`, `all`, `all.gem` with exact → prefix → substring precedence per §6; out-of-range ordinals return none; empty input never matches. Shared by every command that takes an item argument.
- [x] Player save shape adds `inventory` (item entity list) and `equipment` (slot key → item entity) blocks. `player.CurrentVersion` bumps 1 → 2 and the first real entry lands in the migration table; v1 saves load cleanly (empty inventory, empty equipment).
- [x] Race detector clean: `make test` stays green with stress tests covering concurrent get/drop/give between sessions in the same room.

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
    `EffectiveWeaponName`; real weapon plumbing (wielded/equipped/
    natural-weapon damage dice feeding `Stats.Damage`) landed later as
    a post-roadmap warmup — see combat §4.5.
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
  - **M8.5 (landed) — Alignment.** New `progression.AlignmentManager`
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
    - [x] Alignment integer clamped to configured `[min, max]`
          on every write.
    - [x] Bucket tag is present and unique on every entity the
          manager has touched; idempotent re-sync on every
          `Bucket` call.
    - [x] `Set` emits no events and does not append history;
          `Shift` emits `alignment.shift.check` (cancellable +
          rewritable delta), then on actual change emits
          `alignment.shifted` and conditionally
          `alignment.bucket.changed`.
    - [x] `Shift` is a no-op for entities carrying the `admin`
          role tag.
    - [x] `ResolveBuckets` returns the correct `(min, max)`
          range for every subset of `{evil, neutral, good}`
          including degenerate cases per spec §6.6.
    - [x] `village-guard.yaml` (or a sibling) gains a sample
          alignment-conditioned disposition rule that fires
          end-to-end; the M6.5 deferral is closed (AI evaluator
          now consumes `PlayerView.Alignment` + `PlayerView.Bucket`
          and matches min/max/buckets conditions).
    - [x] Player save v10 carries `alignment`; v9 saves load
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
    - [x] Trains-available defaults to 0 and increases only via
          class level-up credits (M8.4).
    - [x] Practice requires a learned ability (or a deferred-to-
          M9 stub) AND a matching in-room trainer; cannot skip
          tiers; does not consume a train.
    - [x] Stat training enforces safe-room rule only when
          `RequireSafeRoomForStats` config is true; honors
          per-race stat cap (default 25 when the race doesn't
          declare); fails with structured result for
          NotTrainable / UnsafeRoom / NoTrains / AtRaceCap.
    - [x] Catch-up boost bumps proficiency toward *prior* cap
          (clamped), not the new cap.
    - [x] `content/core/` ships at least one trainer mob with
          `TrainerConfig` so `practice` and `train` commands are
          end-to-end testable.
    - [x] A handful of integration tests exercise: grant XP →
          level up → trains credited → `train str` succeeds →
          base STR increases → effective combat hit reflects it.
- **M9 — Abilities & effects:** `abilities-and-effects` — registry,
  proficiency, action queue, validation pipeline, hit/miss
  resolution, active effects, passive abilities. Six slices:

  - **M9.1 (landed) — AbilityRegistry + ProficiencyManager.** New
    `progression.Ability` + `AbilityRegistry` (id-keyed, priority
    override, mirrors `ClassRegistry` shape — abilities are NOT
    namespaced, matching the slot registry). New
    `progression.ProficiencyManager` (per-entity prof + cap maps
    with `[1, min(cap, 100)]` clamp on every mutation). Manager
    satisfies the M8.6 `AbilityProficiency` seam so the existing
    `train` / `practice` verbs become functional end-to-end
    (closes the m8-6 "proficiency seam nop" deferral). Pack
    loader gains `content.abilities: [...]` globs. Player save
    bumps to v11 with an `abilities:` block (parallel proficiency
    + cap maps); v10 migration is a no-op. `cmd/anothermud`
    wires the manager + replaces `NewNopGranter` with the
    proficiency-backed `Teach`. `content/core/abilities/`
    ships `slash`, `parry`, and `basic-strike` so the M8.6
    practice path and the M8.4 fighter level-1 grant land as
    real proficiency entries.

    - [x] `progression.AbilityRegistry` is id-keyed,
          case-insensitive, with priority override semantics
          mirroring `ClassRegistry`.
    - [x] `progression.ProficiencyManager` exposes Learn,
          Forget, Has, Proficiency, Cap, SetCap, AddProficiency,
          LearnedAbilities, Snapshot, Restore, Drop.
    - [x] Manager satisfies `progression.AbilityProficiency` so
          `TryPractice` reports `PracticeSuccess` end-to-end
          (closes m8-6 #1).
    - [x] Manager satisfies `progression.AbilityGranter` so the
          `ClassPathProcessor`'s level-up grants land as real
          proficiency entries (replaces `NewNopGranter`).
    - [x] Pack loader accepts `content.abilities` globs,
          validates type (active/passive) + category
          (skill/spell), rejects malformed entries at boot.
    - [x] Player save v11 round-trips `abilities:` block;
          `Persist` calls a `syncAbilitiesToSaveLocked` helper
          that diffs against the previous snapshot so
          training-driven mutations (which bypass the actor's
          dirty bit) still autosave.
    - [x] Session teardown calls `Proficiency.Drop` so the
          manager's working set stays bounded to currently-
          connected players.
    - [x] `content/core/abilities/` ships ≥ 3 baseline
          abilities; `Maerys`'s teach list resolves through the
          registry; fighter's path entry teaches `basic-strike`
          at level 1.

  - **M9.2 (landed) — EffectManager (apply / tick / remove /
    expire).** New `progression.EffectTemplate` + `Effect`
    types; `progression.EffectManager` owns per-entity active-
    effect lists, applies / ticks / removes effects with the
    single-instance rule (spec §5.2), batches expirations
    safely against mid-iteration mutation (spec §5.4), and
    emits applied / removed / expired events. Stat modifiers
    flow through a small `EffectTarget` interface implemented
    directly by `connActor` (EntityID + AddModifiers +
    RemoveBySource); the modifier set is keyed under
    `EffectSourceKey(effectID)` so removal reverses the exact
    set. Effect flags are owned by the manager (per-entity
    snapshot via `Flags` / `HasFlag`) — no entity-side Tags
    surface in M9.2. `cmd/anothermud` wires the manager with
    a session-Manager-backed `EffectTargetResolver` and a
    bus-bridging `effectSink`. Logout calls `Effects.Drop`.

    - [x] `progression.EffectManager` exposes Apply, Has,
          Effects, Flags, HasFlag, RemoveByID, RemoveByFlag,
          Tick, Drop.
    - [x] Apply refuses single-instance re-application
          cleanly: no event, no stat mutation (spec §5.2
          step 2). Pinned by
          `TestEffectManager_SingleInstanceRefusesReapply`.
    - [x] Stat modifiers install + reverse under
          `EffectSourceKey(id)`; modifier removal goes through
          the same source-key path equipment uses, so M5.6's
          dedup invariant carries.
    - [x] RemoveByFlag batches every match in one pass; flag
          and id lookups are case-insensitive. Permanent
          effects (`Duration < 0`) survive every Tick until
          explicitly removed.
    - [x] Tick is safe against mid-tick removal — expirations
          are batched, stat reversal + event emission happens
          outside the manager lock. Pinned by
          `TestEffectManager_TickConcurrentRemoveIsSafe`
          under `-race`.
    - [x] Three new eventbus payloads (`EffectApplied`,
          `EffectRemoved`, `EffectExpired`); production
          `effectSink` in the composition root bridges
          manager → bus.
    - [x] `session.connActor` satisfies `EffectTarget`
          directly (no adapter); `session.EffectTargetResolver`
          maps playerID → connActor via the existing manager
          index. Mob targets land in M9.4.
    - [x] Active-effect state is ephemeral per spec §5.5;
          logout drops it. Stat modifiers persist with the
          entity's stat block by the same source-key path
          equipment uses.
  - **M9.3 (landed) — Action queue + validation pipeline.**
    `progression.Ability` grew the validation surface (cost,
    pulse-delay, initiate-only, target-types, equipment slot+tag,
    alignment range, optional effect template) and the pack
    AbilityFile schema accepts the corresponding YAML fields plus
    a nested `effect:` block. New `progression.ActionQueueManager`
    is per-entity ordered + bounded (16-default, configurable),
    snapshot-deep-copies, drops on logout. New
    `progression.PulseDelayTracker` records next-ready pulse per
    `(entity, ability)` and is consulted by the validator
    (records land in M9.4 on resolution per spec §4.5 step 3).
    New `progression.ValidationPipeline` runs the §4.3 nine-step
    pipeline against a small `ValidationEntity` seam
    (EntityID/IsResting/Alignment/EquippedTags/InCombat/
    CurrentTarget/Movement/Mana/Race) and a `TargetLookup` seam.
    Returns a `ValidationResult{Reason, Ability, ResolvedTarget}`
    with first-failure-wins ordering. New `FizzleReason` typed
    constants enumerate the §4.8 keyword set. Helpers:
    `IsOffensive` (§4.6, conservatively returns false for spells
    until M9.4 metadata lands) and `ResourceFor` (§4.7
    skill→movement, spell→mana). No driver wired — the resolution
    phase consumes the pipeline in M9.4.

    - [x] `Ability` carries Cost, PulseDelay, InitiateOnly,
          TargetTypes, EquipmentSlot, EquipmentTag,
          HasAlignmentRange + AlignmentMin/Max, Effect template.
          Registry normalizes (lowercase slot/tag, dedup target
          types, defensive copy of Effect).
    - [x] Pack `AbilityFile` decodes the new fields incl. a
          nested `effect:` block with modifiers and flags;
          missing `effect.id` is an `ErrInvalidContent` at load.
          `content/core/abilities/slash.yaml` exercises the new
          surface end-to-end.
    - [x] `ActionQueueManager.Push` rejects empty ids and
          over-cap pushes; `Pop` deletes the map slot when the
          queue empties (spec §4.2 "If the queue ends up empty
          the property is cleared"); `Snapshot` returns a deep
          copy; `Drop` clears on logout.
    - [x] `PulseDelayTracker.IsCoolingDown` returns true only
          when `readyAt > currentPulse` (so a recorded readyAt
          ==currentPulse means "ready THIS pulse"); `Sweep`
          evicts stale entries; `Drop` on logout.
    - [x] `ValidationPipeline.Validate` enforces §4.3 ordering:
          rest → alignment → proficiency → equipment slot+tag →
          initiate-only → target validity → offensive in-combat
          → effect-present → pulse-delay → resource. Each step
          pinned by a dedicated test.
    - [x] Target resolution §4.4 covers explicit-id-resolves,
          explicit-id-unresolvable→`invalid_target`, offensive
          fallback to current combat target, and self-target for
          non-offensive abilities.
    - [x] `IsOffensive` returns true for skills and false for
          spells (with or without effect) until M9.4 metadata
          enables damage-spell detection. Resource selection
          maps skill→movement, spell→mana; race-adjusted cost
          via existing `AdjustCost`.
  - **M9.4 — Resolution (hit/miss roll, resource deduct, pulse
    delay, effect application, vital-depleted emit).** Wires
    into the combat round's ability-resolution phase. Split into
    two slices: M9.4a (resolver core) + M9.4b (per-pulse driver +
    wiring + mob targets).

    - **M9.4a (landed) — AbilityResolver core.** New
      `progression.AbilityResolver` executes spec §4.5 for one
      validated invocation: deduct race-adjusted resource (§4.7),
      record last-used, roll hit/miss (§4.5 step 4), on hit record
      pulse delay + apply effect template + emit `ability used`, on
      miss emit `ability missed`, roll proficiency gain on both
      paths (§3.5), and run the post-hit `vital-depleted` death
      check (§4.5 step 9). New seams: `Roller` (mirrors
      combat.Roller so production shares one `*rand.Rand`),
      `ResolutionSource` (embeds `ValidationEntity` + DeductMovement
      / DeductMana / SetLastAbility / StatValue), `TargetHPLookup`,
      `ProficiencyMutator`, and `AbilitySink` (used / missed /
      fizzled / vital-depleted event family). `Ability` grew
      `Variance` + `MaxHitChance`; pack `AbilityFile` decodes both.
      No driver wired — M9.4b consumes the resolver in the combat
      `Ability` phase.

      - [x] Pulse delay recorded on hit only (spec §4.8 acceptance
            criterion overrides the §4.5 step-3 narrative ordering).
      - [x] Variance 0 ⇒ always hits (no roll); otherwise
            `chance = clamp(prof × variance / 100, 1,
            MaxHitChance|default)` vs uniform 1..100.
      - [x] Resource deduction uses the race-adjusted cost; skills
            draw movement, spells draw mana.
      - [x] Proficiency gain rolled on hit AND miss with the §3.5
            taper `(1 - prof/100)`, optional stat factor, and
            failure multiplier; no gain at prof 100.
      - [x] `vital-depleted` emitted only when the resolved target
            is non-self and probes HP ≤ 0; self-cast never
            death-checks. Emit-only plumbing until M9.6 lands
            damage-bearing abilities.
      - [x] `progression.VitalDepletedEvent` is distinct from
            `combat.VitalDepleted` to avoid a progression → combat
            edge; the production bus-bridge forwards both.

    - **M9.4b (landed) — Per-pulse driver + wiring + mob
      targeting.** New `progression.AbilityPhaseDriver` implements
      the §4.2 loop (peek → validate → fizzle-drop-continue OR
      resolve-drop-stop; at most one valid execution per entity per
      pulse) and returns a `combat.PhaseFunc`, wired as
      `combat.Phases.Ability` in `cmd/anothermud`. `combat.PhaseFunc`
      gained a `pulse uint64` param (the round's tick count, threaded
      from `Heartbeat.Tick`) so the resolver records pulse-delay
      cooldowns against it; auto-attack + wimpy ignore it.
      `session.connActor` now satisfies `progression.ResolutionSource`
      (the validation + resolution seam). New
      `eventbus.Ability{Used,Missed,Fizzled,VitalDepleted}` events +
      a bus-bridging `abilitySink`. Logout drops the action queue +
      pulse-delay tracker. New `combat.EntityIDOf` strips the
      combatant prefix.

      - [x] Driver enforces §4.2: invalid entries fizzle + drop
            without consuming the pulse's single execution slot;
            first valid entry resolves + drops + stops. Pinned by
            `ability_phase_test.go` (fizzle-continue, one-per-pulse,
            unknown-ability) + a real-heartbeat integration test.
      - [x] `connActor` ResolutionSource: InCombat / CurrentTarget
            via the combat manager (prefix-stripped target id),
            EquippedTags via the equipment map + item store (lock
            held across the store lookup), Alignment / Race / StatValue
            wired. Pinned by `TestConnActor_SatisfiesResolutionSource`.
      - [x] Mob *targeting* + *death*: the cmd TargetLookup resolves
            mob existence and TargetHPLookup reads mob `combat.Vitals`
            so a queued ability can target a mob and the post-hit
            death check fires for mob victims.

      Known gaps (carried as deferrals):
      - **THIN POOLS:** players have no current movement/mana pool
        yet. `connActor.Movement()/Mana()` report the `movement_max`
        / `resource_max` stat; `DeductMovement/Mana` are no-ops.
        Real pools + regen land with economy-survival (M11).
      - **Mob effect-stat install is NOT delivered** (revises the
        original "mob targets" plan). `MobInstance` can't implement
        `progression.EffectTarget` because `stats` imports
        `entities` (cycle), and it holds a flat `combat.Stats`
        snapshot rather than a source-keyed block. Effects applied
        to a mob are tracked but install no modifiers. This is the
        m8-1 #1 SourceKey-extraction slice.
      - The whole ability path is **dormant until M9.6** — no verb
        enqueues actions yet.
  - **M9.5 — Passive abilities (binary check, scaling bonus,
    hook discovery).** Replaces combat §4.2 extra-attack and §4.3
    defensive-check stubs with real passive rolls. Split into two
    slices: M9.5a (progression primitives) + M9.5b (combat seam +
    content).

    - **M9.5a (landed) — Passive building blocks + resolver.**
      `Ability` gained `Hook` (the §6.3 discovery key) + `MaxBonus`
      (the §6.2 scaling ceiling). `AbilityRegistry.ByHook(hook)`
      returns the PASSIVE abilities for a hook, id-sorted, matched by
      metadata not hardcoded id. New `internal/progression/passive.go`:
      - `PassiveBinaryCheck(prof, variance, maxChance, roller)` —
        §6.1 (`prof×variance/100`, or `prof×maxChance/100` when
        variance ≥ 100; roll 1..100).
      - `PassiveScalingBonus(maxBonus, prof)` — §6.2
        (`maxBonus×prof/100`).
      - `PassiveResolver.ExtraAttacks(entityID)` — binary-checks each
        `extra_attack` passive, +1 swing per success (the chosen
        model; §6.1 "does it fire on this opportunity").
      - `PassiveResolver.DefensiveEvade(defenderID)` — first
        `defensive` passive (id-order) that wins its binary check
        evades.
      - Both roll a §6.3 proficiency gain on a firing passive, via a
        shared `gainThreshold` extracted from the active resolver's
        `rollGain` (DRY; behavior-preserving — the §3.5 stat factor /
        failure-multiplier / cap-guard now live in one helper, with
        `proficiencyValueOf` / `effectiveCapValueOf` as free funcs).
      - [x] Primitives + `ByHook` + resolver pinned by
            `passive_test.go`; unlearned passives never fire or roll
            (prof-0 short-circuit); resolver refactor verified by the
            existing resolution tests.

      Known gaps (carried to M9.5b / deferrals):
      - **Stat factor omitted from passive gain.** The §3.5 step-3
        gain stat factor (e.g. parry's `gain_stat: dex`) needs an
        entity-stat-by-id host seam that doesn't exist; passive gain
        uses base × taper × failure-mult only for now.
      - No combat wiring yet — the auto-attack `extraAttackCount` /
        `defensiveEvade` stubs still return zero. M9.5b adds the
        combat `PassiveEvaluator` seam + the host adapter + content
        (`second-attack`, `parry` hook) + fighter grants.
      - Hook YAML surface (`hook` / `max_bonus` on `AbilityFile`) +
        content land in M9.5b.

    - **M9.5b (landed) — Combat seam + content.** A combat-defined
      `PassiveEvaluator` interface (`ExtraAttacks` / `DefensiveEvade`,
      bare-id keyed) added to `AutoAttackConfig` (nil-safe → pre-M9.5
      behavior). The auto-attack `extraAttackCount` / `defensiveEvade`
      helpers now delegate through it, prefix-stripping the combatant
      id via `EntityIDOf`. `*progression.PassiveResolver` satisfies the
      interface structurally — no adapter; `cmd/anothermud` builds one
      (sharing `combatRNG` + the proficiency manager) and passes it in.
      Pack `AbilityFile` gained `hook` / `max_bonus`; `second-attack`
      (extra_attack) + `parry` (defensive, now hooked) content; fighter
      L1 grants both.

      - [x] Extra-attack raises swing count; defensive evade pre-empts
            a swing + emits `combat.Evade` without consuming a hit
            roll. Pinned by `autoattack_test.go` (fake evaluator).
      - [x] `hook` / `max_bonus` decode + `ByHook` discovery pinned by
            `loader_test.go`.

      Known gaps (deferrals):
      - Passive gain still omits the §3.5 stat factor (carried from
        M9.5a — needs an entity-stat-by-id seam).
      - `PassiveScalingBonus` (§6.2) has no wired consumer yet — the
        two hooks use the §6.1 binary check. It ships as a tested
        building block for future scaling passives (extra-damage,
        crit-chance).
      - Mob passives: mobs never enqueue and have no proficiency map,
        so they grant no extra attacks / evades. Player-only until a
        mob proficiency surface lands.
  - **M9.6 — Content + verb surface.** Player-facing
    `abilities` / `cast` / skill-named verbs; baseline content
    (kick, heal, bless). Split into two slices: M9.6a (verb
    surface + effect-only content) + M9.6b (ability.used
    damage/heal handler + offensive/heal content).

    - **M9.6a (landed) — Verb surface + bless.** The dormant
      M9.4 ability path is now player-driven. `command.Env` /
      `Context` carry the M9.1/M9.3 managers (`Abilities`,
      `Proficiency`, `ActionQueue`); session threads them from
      `Config`. New verbs in `internal/command/abilities.go`:
      `abilities` (+ `abi`) lists the actor's learned set with
      proficiency/cap + skill/spell classification;
      `cast <ability> [on] <target>` enqueues a `QueuedAction`;
      `AbilityVerb(id)` is the skill-named-verb factory, and
      `cmd/anothermud` registers one verb per **active** ability id
      at boot (collision with a builtin is skipped with a warning).
      A new `ability.{used,missed,fizzled}` bus subscriber renders
      resolution outcomes to the caster (+ room for used/missed);
      `fizzleMessage` maps each §4.8 reason to a player line.
      Content: `abilities/bless.yaml` — an effect-only spell
      (hit_mod +2 / ac +1 for 12 pulses, variance 0) granted on the
      fighter path at level 1 so a fresh fighter casts it
      end-to-end. Resolution stays **combat-only** (the ability
      phase iterates `AllCombatants`); a queued buff sits until the
      caster is in a round.

      - [x] `abilities` lists learned set; empty + unregistered
            (declarative-grant) cases handled. Pinned by
            `abilities_test.go`.
      - [x] `cast` / skill-named verbs enqueue with optional
            target resolution (Locator/Placement via
            `findCombatantInRoom`, prefix-stripped to the bare
            entity id); unknown ability, missing target, and
            queue-full refusals covered.
      - [x] bless resolves through the existing resolver's
            on-hit effect application (connActor EffectTarget);
            no damage handler needed.

      Known gaps (carried to M9.6b / deferrals):
      - **No damage/heal yet.** basic-strike (granted, active
        skill) and any offensive ability resolve hit/miss +
        proficiency gain but apply no HP change — the resolver
        emits `ability.used` and leaves damage to a handler
        (spec §4.5 step 9). kick/heal content + the handler are
        M9.6b. The `ability.vital_depleted` → combat-death bridge
        also lands in M9.6b (no ability kills today).
      - **AbilityUsedEvent has no handler token** (m9-4 deferral
        #2) — added with the M9.6b handler-dispatch path.

    - **M9.6b (landed) — Damage/heal handler + content.**
      Abilities now change HP. `Ability` gained `HandlerToken` +
      `DamageDice`/`HealDice` (YAML `handler`/`damage`/`heal`);
      `IsOffensive` (§4.6) now classifies a no-effect spell with
      damage dice as offensive. `AbilityUsedEvent` +
      `eventbus.AbilityUsed` carry the handler token (closes m9-4
      deferral #2). Two `cmd/anothermud` bus subscribers:
      - `ability.used` side-effect handler dispatches on the token —
        `damage` rolls `DamageDice` via `ApplyDamageIfAlive` and
        emits a `combat.Hit` (so ability damage shares the combat
        sink/log + future renderer); `heal` rolls `HealDice` onto
        the target-or-self. Dice are pre-parsed at boot
        (`combat.ParseDice`; a bad expr warns + disables that
        ability's effect). It runs synchronously inside the
        resolver's §4.5 step-8 emit, so the damage is committed
        before the resolver's step-9 HP probe.
      - `ability.vital_depleted` bridge re-prefixes the bare ids and
        calls `productionCombatSink.OnVitalDepleted`, reusing the
        cancellable death-check/Kill flow auto-attack uses (player
        respawn + mob untrack fire identically). The handler never
        emits the death itself — step 9 owns it, so there is no
        double signal.
      - Content: `kick.yaml` (skill, 1d6), `heal.yaml` (spell, 2d4),
        and `basic-strike.yaml` upgraded to deal 1d4 (it was a
        granted no-op skill). Fighter L1 path grants kick/heal/bless.
      - Renderer fix: the M9.6a `ability.used`/`.missed` renderers
        resolved a placeholder target name (the resolver has no name
        registry — "id doubles as name"); they now resolve the live
        name from `TargetID` via a `combatantName` host helper.

      - [x] `IsOffensive` damage-spell case + `HandlerToken`
            propagation + `decodeAbility` handler/dice pinned by
            unit tests (`validation_test.go`, `resolution_test.go`,
            `loader_test.go`).
      - [x] Bare-id contract for the bridge hardened: the host
            helpers normalize via `EntityIDOf` before re-prefixing
            (idempotent on bare ids), so a future prefixed
            `ResolutionSource` id can't misroute the death bridge.

      Known gaps (deferrals):
      - The damage/heal handler + death bridge live in the
        composition root and are integration-only (no unit test),
        same as the M9.6a renderers. Player-facing damage/heal
        NUMBERS are still invisible — combat hits are log-only until
        the M10 ui-rendering pass.
      - Mob effect-stat install still blocked by the stats↔entities
        cycle (m8-1 #1): mobs are damageable + killable by abilities,
        but a debuff effect applied to a mob remains inert.
- **M10 — Quests & UI polish:** `ui-rendering-help` + `quests`. Basic
  ANSI-16 brace color already landed in M2 (`internal/ansi`); M10
  builds the full rendering surface (themes, semantic/literal tags,
  prompt, panel, help) and then the quest system on top. Two tracks;
  UI ships first because quest banners/journals render through the UI
  primitives and because it pays the deferred "combat damage/heal
  numbers are log-only / invisible" debt (m9-6).

  **Deviation note (decorator → send seam):** the spec models color as
  a `ColorRenderingConnection` IConnection decorator (§5). This repo's
  `conn.Connection` is byte-only (ID/Read/Write/Close) with no
  `SupportsAnsi`/`SendLine`; color is already applied at the
  `connActor.Write` seam via `ansi.Render(msg, ColorEnabled())`. M10
  keeps rendering at that seam (swapping the minimal renderer for the
  full one) rather than introducing a transport decorator — the seam
  already satisfies "features emit tags without per-call capability
  checks." `ColorEnabled()` plays the role of `SupportsAnsi`.

  UI track:

  - **M10.1 (planned) — Theme registry + full color renderer.** Grow
    `internal/ansi` (or a sibling `internal/render`) into the spec's
    pipeline: a `ThemeRegistry` mapping semantic tag → `{fg,bg,html}`
    with a `Compile()` step producing `AnsiPair(open,close)` lookups;
    a `ColorRenderer` with `RenderAnsi`/`RenderPlain` recognizing
    semantic tags (`<highlight>`), literal color tags
    (`<color fg=".." bg="..">`), and brace shorthand (`{yellow}` /
    existing `{r}` codes), each with identical structural scanning and
    a per-mode input→output cache; static `ResolveFgColor`/
    `ResolveBgColor`; and a `TagStripper` (`StripTags`,
    `VisibleLength`). Theme entries load from pack content
    (`theme.yaml`).

    - [x] Semantic, literal, and brace forms all recognized;
          case-insensitive tag/color names.
    - [x] `Compile()` idempotent; produces an open/close pair only
          when fg or bg resolves; `IsKnown` true for declared-but-
          colorless; `Resolve` null for same.
    - [x] Unknown opening tags pass through as literals; known closing
          tags consumed, unknown closing tags pass through.
    - [x] Plain and ANSI modes recognize the same constructs; cache
          never re-parses identical input.
    - [x] `TagStripper.VisibleLength(s) == len(StripTags(s))` for every
          input; a `<` with no `>` consumes the rest.

  - **M10.2 (landed) — Wire the renderer into the send seam.**
    `connActor` gains a `renderer *render.ColorRenderer` (from a new
    `session.Config.Render`), and `connActor.Write` routes every line
    through `connActor.render` — `RenderAnsi` when `ColorEnabled()`,
    `RenderPlain` otherwise; nil renderer falls back to the minimal M2
    `ansi.Render` so tests need no wiring. Theme loads from pack
    content: `content.theme: [...]` globs → `ThemeFile`/`decodeTheme`
    → `Registries.Theme` (global, later packs override). The
    composition root compiles the theme once after `pack.Load` and
    binds a shared read-only `ColorRenderer`. `content/core/theme/
    theme.yaml` ships the starter semantic palette (hp/mana/mv,
    highlight/subtle/title/danger, damage/heal, frame, item.*).

    - [x] `connActor.Write` renders through the themed renderer;
          color-disabled sessions get `RenderPlain` (no ANSI).
    - [x] Existing M2 brace codes still render (back-compat via the
          renderer's ROM token table + the nil-renderer fallback).
    - [x] Theme compiled exactly once at boot; renderer shared
          read-only across sessions; cross-pack tag override resolves
          by load order. Pinned by pack + session seam tests; boot
          smoke shows `theme=1` in the pack-content log.

  - **M10.3 — Prompt renderer.** Two slices: the pure renderer, then
    the session flush wiring (the prompt-refresh state machine lands
    with it since session-lifecycle §2.5/§3.5 doesn't exist yet).

    - **M10.3a (landed) — PromptRenderer (pure).** `render.RenderPrompt`
      + `PromptVitals` + `DefaultPromptTemplate`: a `{token}`
      substituter over the fixed table (`{hp}`,`{maxhp}`,`{mana}`,
      `{maxmana}`,`{mv}`,`{maxmv}`,`{gold}`), case-insensitive,
      unknown-letters-token→empty (§7.2 typo tolerance). Only
      `{letters}` shapes are treated as tokens; other braces (`{1}`,
      lone `{`, unterminated) are left verbatim so brace color shorthand
      survives to the M10.1 renderer. Default template uses `<hp>`/
      `<mana>`/`<mv>` semantic tags (§7.1) and composes with the color
      renderer downstream.

      - [x] Default template used when the template arg is empty.
      - [x] All listed tokens substituted; unknown tokens → empty
            (not literal `{x}`); case-insensitive. Composition with the
            color renderer pinned by test.

    - **M10.3b (landed) — Prompt flush wiring.** connActor gains the
      §2.5 prompt-refresh flags (`promptDisplayed`/`receivedInput`/
      `needsPromptRefresh`, under a.mu). `Write` is the content-send
      half of §3.5: it breaks the line ahead of content when a prompt
      is displayed and unanswered, then arms the refresh. `noteInput`
      sets `receivedInput`. `connActor.flushPrompt` renders the
      template (default or per-player) through `RenderPrompt` + the
      color renderer and writes it on its own line (no trailing
      newline); `Manager.FlushPrompts` drives it for every non-
      link-dead session, registered as a cadence-1 `prompt-flush`
      tick handler. Player save gains an optional `prompt_template`
      (no schema bump; absent → default). `promptVitals` reads HP from
      Vitals and mana/movement from their max stats (thin pools).

      - [x] Prompt renders after content arrives, on its own line
            (verified live + unit tests).
      - [x] Refresh flags suppress prompt/content collisions; link-dead
            sessions skipped. Flow / prompt-mode skips are N/A until
            character-creation (M12) adds those input modes.

      Deferrals: no `prompt` verb sets the per-player template yet
      (lands with the M10 command surface); mana/movement render 0/0
      until new-character stat seeding populates resource/movement max;
      gold is 0 until economy-survival (M11).

  - **M10.4 (landed) — Panel renderer.** `render.Panel`/`Section`/`Row`
    (EmptyRow/TitleRow/TextRow/CellRow/FooterRow constructors) /`Cell`
    (Fixed `Width` or `Fill`, `Align`, progress via `Progress`+
    `Value`/`Max`). `Panel.Render() (string, error)` emits a `<frame>`-
    bordered multi-line string (\r\n) with all width math through
    `VisibleLength` so colored cells align with plain ones; Major/Minor/
    None section separators (first suppressed, top+bottom always Major);
    title left-truncation+ellipsis with `ErrPanelTitleOverflow` when the
    right side alone overflows; TextRow word-wrap; Fill cells split
    leftover width; ASCII progress bars (keep VisibleLength byte-correct).

    - [x] All output lines equal visible width; width math uses visible
          length (colored-cell-aligns-with-plain test).
    - [x] Section separators honored; first suppressed; top+bottom
          Major regardless of config.
    - [x] Title right side over inner width raises
          (`ErrPanelTitleOverflow`); combined over triggers left
          truncation+ellipsis.

    Scope note: cells render single-line (truncate to width); per-cell
    multi-line wrap (different-height cells in one CellRow) is deferred
    — TextRow covers the multi-line body-text case. Spec §13's
    "raise on title overflow" is followed (vs clamp+log).

  - **M10.5 — Help service + `help` command + renderer.** Two slices:
    the pure `internal/help` package, then the pack/command wiring.

    - **M10.5a (landed) — Help package.** `internal/help`: `Service`
      (byNS canonical set + byID incl. namespaced / byTitle indices,
      `putIfHigher` load-order precedence, integer role tiers
      none<player<builder<admin with the §9.5 placeholder
      `requesterTier`, `Query` exact-id→exact-title→fuzzy, `List`,
      `Categories`); `Topic` + `Summary` + `ParseRole`; `RenderTopic`/
      `RenderDisambiguation`/`RenderNoMatch` emitting `<title>`/`<subtle>`
      tags for the M10.1 renderer (term sanitized against tag injection).

      - [x] Topics index by id, namespaced id, title, category; dup
            registrations resolve by load-order (higher wins, equal
            keeps newest).
      - [x] Query precedence exact-id → exact-title → fuzzy; role gate
            on query/list/categories (admin hidden from player; player
            hidden pre-login).
      - [x] Renderers: topic (Syntax/See-also sections iff present),
            disambiguation (id column), no-match (term sanitized). 92%
            coverage.

    - **M10.5b (landed) — Pack loading + `help` command + wiring.**
      Per-pack `<pack>/help/*.yaml` loading via `content.help` globs
      → `HelpFile`/`decodeHelp` → `Registries.Help.AddTopic` at the
      pack's load order (PackName = pack namespace); topics missing
      id/title are skipped with a `pack.help.skip` warn. The `help`
      command (wired through `command.Env`/`Context` + `session.Config`)
      runs `Query` and renders topic / disambiguation / no-match by
      status, plus a no-arg category index. `content/core/help/
      commands.yaml` ships help/look/movement/combat topics. The §9.2
      command-help generator is N/A (the registry has no typed arg
      definitions) and is deferred.

      - [x] `help <topic>` renders topic; ambiguous term renders
            disambiguation; miss renders no-match (handler tests +
            live smoke).
      - [x] Missing-field topics skipped with a load warn; help loads
            at boot (smoke shows `help=4`).

  Quest track (after UI):

  - **M10.6 (landed) — Quest registry + definitions + loader.**
    `internal/quest`: `Definition`/`Stage`/`Objective`/`Prerequisite`/
    `Reward` model + `Registry` (id-keyed `Register` that validates
    [non-empty id, ≥1 stage each with ≥1 objective] + normalizes
    objectives [count→≥1, generated stable ids `stageKey-type-index`
    when absent], `Lookup`/`All`/`Len`). Pack loader: `content.quests`
    globs → `QuestFile`/`decodeQuest` → `Registries.Quests`;
    `decodeQuest` namespace-qualifies giver / objective target+npc /
    prereq quest ids / reward item ids (qualified `pack:id` passes
    through), defaults `abandonable` to true, and sets `PackDir`.
    `content/core/quests/patrol.yaml` ships a sample (gate-patrol).

    - [x] Definitions register by id; later replace earlier; objective
          ids generated when absent + stable across reloads (registry
          tests).
    - [x] Missing reward/prereq/flag values default without error;
          abandonable defaults true; loader namespaces ids (loader
          tests + boot smoke `quests=1`).

  - **M10.7 (landed) — QuestService accept/advance/abandon + rewards.**
    `quest.Service` (single-mutex state machine over an in-memory
    per-player `State`): `Accept` (six `AcceptStatus` outcomes, §3.2
    prereq gates, §3.3 abandonable-only cap, §3.4 banner honoring
    secret/silent), `AdvanceObjective` → `advanceStage` → `complete`,
    `AdvanceMatching` (snapshot-then-advance, §4.4), `Abandon`. Reward
    `Dispatcher` over four replaceable interfaces (`ExperienceGranter`/
    `GoldGranter`/`AbilityTeacher`/`ItemGranter`) each with a no-op
    default + functional options; class/race unlock via `Player`
    setters. `EventSink` (Nop default) emits Started/ObjectiveAdvanced/
    StageAdvanced/Completed/Abandoned; `Persister` (Nop default) saves
    on every mutation. `LoadState`/`DropState`/`Snapshot` expose the
    repo for the M10.8 persistence + M10.10 commands. Player cache
    populated on Accept (§4.3). Package-only this slice — composition-
    root wiring lands with M10.8+.

    - [x] Six acceptance outcomes distinguishable; cap counts only
          abandonable + bypassed for non-abandonable; banner honors
          secret/silent.
    - [x] Advance no-ops on missing/complete; progress clamped; stage
          seeds at zero; completion only on final-stage all-complete.
    - [x] Reward steps independent + silently no-op on nop service;
          cache miss skips reward but still emits completed.
    - [x] Abandon silently rejected for non-abandonable. 96.3% cov.

  - **M10.8 (landed) — Quest persistence + wiring.** `internal/
    queststore.Store` implements `quest.Persister` (Save writes
    `players/<lowercase name>/quests.yaml` via `AtomicWrite`; path
    resolved from an id→name cache populated by Load) and `Load`
    (reads + orphan-filters + caches the name; side-effect-only on
    missing/unreadable). A `questFile` DTO keeps the pure quest package
    free of YAML. Orphan filter drops active+completed entries unknown
    to the registry, skipped when the registry is empty (§6.4). The
    composition root builds the store + `quest.Service` (nop events/
    rewards for now) and the session login path calls `Load` →
    `LoadState`; teardown calls `DropState` + `Forget`.

    Deviation (noted): load is a direct synchronous session-config call,
    not a bus event (spec §6.3) — consistent with the Effects/
    Proficiency wiring and because load must finish before the player
    issues commands (spec §11 flags the event load as racy).

    - [x] Every mutating op writes (service calls Persist.Save); load on
          login → LoadState; orphan filter gated on non-empty registry
          (queststore tests). 86.8% store coverage; boot+login smoke
          clean.

  - **M10.9 (landed) — Watcher + markers.** `internal/questwatch.Watcher`
    subscribes to mob-killed/item-picked-up/item-given/player-moved and
    routes each to `Service.AdvanceMatching` for the source player
    (`kill`/`collect`/`deliver`/`visit`); collect/deliver resolve the
    instance id → template id through the entity store, and missing
    source ids / missing entities are tolerated (§7.4). Markers live in
    the pure quest package as `Service.HasMarker`/`MarkedTemplates`:
    per-definition giver (always) + current-stage deliver-npc /
    collect-target, excluding kill, with secret quests contributing
    none. Watcher wired at the composition root (dormant until a player
    accepts a quest).

    - [x] Watcher maps exactly the four events via AdvanceMatching;
          non-canonical types only advance explicitly; missing payload/
          entities don't raise (watcher tests + bus-routing test).
    - [x] Markers: per-definition giver + current-stage deliver/collect;
          kill excluded; secret contributes none; bulk ≤1 per entity
          (marker tests).

    Deferred (§7.2/§7.3 side channels): `quest_grant` on an item
    template or destination room, and `quest_advance` on the pickup
    payload — room has no property bag, the pickup event has no
    `quest_advance` field, and the grant path needs the M10.10 accept
    Player adapter. Recorded for M10.10/a later content slice.

  - **M10.10a (landed) — Commands + journal rendering.** connActor
    satisfies `quest.Player` (EntityID/Level/Class/SetClass/SetRace);
    `quest.Service` flows through `command.Env`/`Context`. `accept`/
    `abandon`/`quests` (+`journal`) verbs: accept resolves a term to a
    quest id via `Registry.ResolveID` (bare id / namespaced / name) and
    surfaces all six outcomes (banner on success); abandon checks the
    quest is active+abandonable for precise feedback; `quests` renders
    the active-quest journal through the M10.4 panel + M10.1 color
    (title/classification, current-stage description, `[x]`/`[ ]`
    objective rows with progress).

    - [x] `accept`/`abandon`/`quests` map to service ops with the spec
          outcomes surfaced (handler tests).
    - [x] Banner + journal render through panel/color. Verified live:
          accept → journal → move (watcher advances visit → stage
          advance) → abandon all work end-to-end.

  - **M10.10b (landed) — Reward adapters + event sink + markers in
    look.** `session.NewQuestRewards` builds the reward dispatcher:
    XP→`connActor.GrantXP` via the progression manager, abilities→the
    proficiency manager directly (its `Learn` matches `AbilityTeacher`),
    items→`entities.Store.Spawn` + `AddToInventory` + `MarkContentsDirty`
    (gold stays nop until M11). `quest.Service` is now constructed after
    those managers exist, with a `questLogSink` logging the lifecycle.
    `RenderRoom` gained a marker checker; `look`/movement/login/reconnect
    pass one bound to `Service.HasMarker`, so quest givers / current-stage
    deliver-npcs / collect-targets show a `(!)` glyph.

    - [x] Completing a quest grants its XP/abilities/items (reward
          adapters wired; dispatch unit-tested in the service).
    - [x] Quest-relevant entities show a marker in look output (render
          test + verified live: the giver shows `(!)` after accept).

    Deferred (still tracked in m10-9-deferred-fixes): the typed
    event-bus bridge (no consumer yet — logging sink for now) and the
    `quest_grant` item/room side channels.

- **M11 — Survive:** `economy-survival`. Four small, loosely-coupled
  subsystems that share the same shape (a service over an entity
  property, integrating through events): currency, shops, sustenance,
  rest, and the consumable pipeline that feeds the first three. This
  milestone also pays the M9 deferral "real pools + regen land with
  M11" — sustenance and rest only expose regen *multipliers*; the
  vitals-regen heartbeat that composes them is built in the last slice.

  Sliced bottom-up so each lands behind the last:

  - **M11.1 (landed) — Currency core.** A single integer `gold`
    property on the player, plus the `CurrencyService` that mutates it
    (spec §2). New `internal/economy` package mirroring the
    `AlignmentManager` seam: an `Entity` interface (`ID`/`Gold`/
    `SetGold`) the `connActor` satisfies, a `Sink` bridged to the bus
    at the composition root, and `AddGold`/`SetGold`/`Read`. Gold
    floors at zero on every mutation; `AddGold` fires
    `currency.credited` on non-negative deltas and `currency.debited`
    on negative; `SetGold` rejects negative input. `gold` persists on
    the player save (v11→v12, no-op migration). The quest reward
    `GoldGranter` nop is replaced with a real adapter resolving
    entityId→actor→service (closes the M10.10b "gold stays nop"
    note). The `TryAutoConvert` pickup hook (spec §2.3, referenced by
    inventory §4.1) lands in the `get`/`give` paths: a `currency`-
    tagged item with a positive `value` is credited as gold and
    untracked instead of entering inventory, suppressing the normal
    pickup/give event. A `gold` verb reads the balance. A
    `gold-coins` currency template ships in `content/core` and is
    placed in town-square for live testing.

    - [x] Gold floors at zero; `AddGold` fires credited/debited by
          delta sign; `SetGold` rejects negative (economy unit tests).
    - [x] `gold` persists across save/load (v12 round-trip + v11→v12
          migration tests).
    - [x] Auto-convert credits gold + untracks the item for a
          `currency`-tagged positive-value item on `get`/`give`,
          only for player destinations, suppressing the pickup/give
          event (command tests cover convert / zero-value / no-service
          / give-to-recipient).
    - [x] Quest gold reward credits the player through the service
          (wiring test; nil-service stays a no-op).

  - **M11.2 — Shops.** A shop NPC carries the `shop` tag and a config
    record (sells list + optional per-shop buy markup / sell discount,
    falling back to the global economy defaults 1.2 / 0.5). The
    `ShopService` (spec §3) prices items (`max(1, round(V×mult))`,
    int64), resolves stock by partial name with ambiguity guarding
    (§3.7) and inventory by first-match (§3.8), lists stock (§3.4), and
    runs buy/sell/value through the currency service with cancellable
    `shop.buy`/`shop.sell` pre-events. Sliced a/b:

    - **M11.2a (landed) — Service core.** `internal/economy/shop.go`:
      `EconomyConfig` + `ShopConfig`, pricing, stock/inventory
      resolution, listings, and `Buy`/`Sell`/`Value` over a `Shopper`
      interface (the connActor satisfies it) + `ShopSink` for the
      cancellable events. Buy charges before item creation with no
      refund on spawn failure (spec §9 open question, kept as-is); sell
      auto-unequips silently and rejects `no_sell` / zero-value items.

      - [x] Pricing floors at 1; per-shop multipliers override the
            global default only when positive (unit tests).
      - [x] Stock resolves by partial name; a prefix matching two
            sells entries is ambiguous → no sale. Inventory resolves
            first-match (unit tests).
      - [x] Buy fires the cancellable `shop.buy` before charging;
            `InsufficientGold` returns the price; sell auto-unequips and
            rejects `no_sell` (unit tests).
      - [x] Value returns the inventory (sell) price first, then the
            stock (buy) price (unit tests).

    - **M11.2b (landed) — Verbs + content + wiring.** `buy`/`sell`/
      `value`/`list` verbs in `command/shop.go` (find the first
      `shop`-tagged mob in the room, parse `ShopConfig` from its
      properties); `shop.buy`/`shop.sell` bus events + a main-side sink
      bridge; `ShopService` wired through `session.Config`/`Env`. A
      `merchant` mob (sells healing-draught + leather-cap) spawns in
      Market Row; healing-draught / leather-cap gained `value` props.

      - [x] Verbs route through the service and render each outcome
            (command tests: list / buy / insufficient / sell / value /
            no-shop).
      - [x] Live-verified the full loop: `get coins` (+25) → `buy
            healing` (−18) → `sell healing` (+8 = value 15×0.5) → 15
            gold, with correct list pricing (15×1.2=18, 12×1.2=14).

      Known limitation (spec §3.8, kept): shop name resolution is a
      prefix on the full item name, so `sell sword` won't match "a
      short sword" (only `sell short` does) — unlike `get`'s keyword
      match. Recorded in m11-2-deferred-fixes.
  - **M11.3 (landed) — Sustenance.** Persisted `sustenance` pool
    (spec §4) on the player, the `SustenanceService` over a small
    `SustenanceEntity` interface (the connActor satisfies it), tier
    derivation (`TierOf`: full/hungry/famished at 67/34) +
    `GetRegenMultiplier` (1.0/0.5/0.0), and the drain world-tick
    subscriber with throttled hunger reminders. New `internal/economy/
    sustenance.go`: `SustenanceConfig` (thresholds, multipliers, drain
    amount/cadence, reminder interval) + `DefaultSustenanceConfig`,
    `Set`/`Add`/`Drain`/`Read` all clamped to `[0, MaxSustenance=100]`.
    Sustenance emits NO bus events (spec §7 — value + helpers only), so
    unlike currency it carries no Sink. `sustenance` persists on the
    player save (v12→v13); the v12→v13 migration is the first
    value-injecting migration — it seeds legacy characters to full so
    they don't load famished. A fresh character is seeded to 100 inline
    in the session load path (mirroring the alignment seed, NOT a
    `character.created` bus subscriber, because the actor is not yet
    registered with the Manager at publish time). `Manager.DrainSustenance`
    is the drain body — registered in `cmd/anothermud` at `DrainCadence`
    (300 ticks), it decrements every logged-in actor and emits one
    hunger reminder per `ReminderIntervalTicks` (3000) per player.

    - [x] Tiers honor configured thresholds (full ≥ 67 / hungry ≥ 34 /
          famished < 34); regen multiplier is 1.0 / 0.5 / 0.0 (unit
          tests).
    - [x] Set / Add / Drain clamp to `[0, 100]` and Drain floors at zero
          (unit tests); sustenance never modifies vitals directly — only
          the multiplier is exposed.
    - [x] `sustenance` persists across save/load (v13 round-trip,
          including the famished-zero omitempty round-trip + v12→v13
          seeds-full migration tests).
    - [x] Drain decrements every logged-in actor and emits a throttled
          below-Full reminder; Full tier and nil service are silent
          (session tests).
  - **M11.4 (landed) — Rest.** Transient rest-state machine (spec §5)
    on the player, the `RestService` over a small `RestEntity`
    interface (the connActor satisfies it), the combat-engage wake, and
    rest/sleep/wake verbs. New `internal/economy/rest.go`: `RestState`
    (awake/resting/sleeping), `RestConfig` (multipliers 2.0/3.0,
    `MinSleepTicksForWellRested` 120) + `GetRestMultiplier`,
    `SetRestState` (cancellable `entity.rest_state.changed` pre-event
    via a `RestSink`, returns `(ok, reason)`), and `ForceAwake` (combat
    wake — same event with reason `combat`, veto ignored). Rest state is
    TRANSIENT: stored as zero-value-awake fields on the connActor whose
    setters never mark the save dirty, so a disconnect while
    resting/sleeping restores as awake — no persistence change, no
    schema bump. Sleep-start tick is stamped from `loop.TickCount` for
    the M11.5 well-rested credit. The combat-wake lives at the
    composition root (`productionCombatSink.OnEngagement` → `ForceAwake`
    on the target), not in a verb. `rest`/`sleep`/`wake` (+`stand`
    alias) verbs route through the service.

    - [x] Rest state defaults to awake when unset; SetRestState fails on
          same-state (`already_in_state`) and cancelled events
          (`cancelled`) (economy unit tests).
    - [x] Transition to sleeping records the start tick; transition to
          awake clears the rest target (unit tests).
    - [x] Combat-engage forces a resting/sleeping target back to awake
          and emits `entity.rest_state.changed` with reason `combat`,
          bypassing the cancellable check (ForceAwake unit test; wired
          through OnEngagement at the composition root).
    - [x] Multipliers are 2.0 (resting) / 3.0 (sleeping), 1.0 otherwise
          (unit tests).
    - [x] rest/sleep/wake verbs route through the service and render
          each outcome (command tests).

    Deferred to M11.5 (see m11-4-deferred-fixes): the `healing_rate`
    room property (spec §5.7). `world.Room` has no property bag and the
    only consumer is the M11.5 regen heartbeat, so it lands with that
    consumer rather than shipping a field nothing reads.
  - **M11.5 (landed) — Consumables + regen.** The eat/drink/use
    pipeline (spec §6), the `healing_rate` room property (§5.7), and the
    vitals-regen heartbeat that composes the sustenance × rest × room
    multipliers — paying the M9 "real pools + regen" obligation and
    closing M11. New `internal/economy/consumable.go`:
    `ConsumableService.Consume` over the entity store runs the §6.2
    pipeline — top-level-only resolution (§6.5), charge gate
    (`NoCharges` before the pre-event), cancellable `item.consuming`,
    sustenance replenish via the M11.3 service (clamped at 100),
    `item.consumed` emitted BEFORE destruction so the effects subscriber
    can read the item, then destroy/untrack (single-use, or charged with
    `destroy_on_empty`). Effect application is decoupled (§6.3): the
    event carries `effect_id`/duration/data but no subscriber applies it
    yet (no effect-id registry — recorded in m11-5-deferred-fixes).
    `internal/economy/regen.go`: `RegenConfig` + `RegenAmount` (base ×
    sustMult × restMult, + room healing_rate additive, famished → 0).
    `Manager.RegenTick` heals living, out-of-combat, below-max players;
    registered as the `vitals-regen` world-tick handler. `world.Room`
    gained a typed `HealingRate` field (loader + RoomFile). eat/drink/use
    verbs gate on the item's `consume_method`. Content: a `trail-ration`
    food item, `consume_method: drink` on the healing-draught, and
    `healing_rate: 1` on town-square.

    - [x] Consume requires a top-level item; zero-charge items fail
          `NoCharges` without firing the pre-event; cancel keeps the
          charge + item; single-use destroys; `destroy_on_empty=false`
          survives empty; sustenance clamps at 100; `item.consumed` fires
          before destruction (economy unit tests).
    - [x] eat/drink/use route through the service and gate on
          consume_method (command tests; live-verified `get ration` →
          `eat ration`).
    - [x] Regen composes sustenance × rest + room healing_rate; famished
          heals nothing; full HP / in-combat / nil-service are skipped
          (session tests).
    - [x] `healing_rate` loads from room YAML onto `world.Room`
          (pack tests).

- **M12 — Character creation wizard:** the full `character-creation`
  flow now that the systems it touches exist. Sliced bottom-up:

  - **M12.1 (landed) — Wizard primitive.** New `internal/wizard`
    package: the engine-side flow primitive (spec §3-§5), with NO
    session/login/telnet dependency. `Flow` (ordered `Step`s + trigger +
    cancellable + `OnComplete` validation handler + optional
    wizard-progress labels); `Instance` state machine driven one input
    line at a time (`Start`/`Input` → `StatusAwaitingInput` /
    `StatusCompleted`); the four step types (`InfoStep` auto-advances,
    `ChoiceStep` resolves 1-based index OR unique case-insensitive label
    prefix, `TextStep` with optional validation + secret-echo toggle,
    `ConfirmStep` y/yes/n/no); skip predicates evaluated before
    rendering; and the structured `StepEvent` sink (the §5 seam — the
    plain-text path is real, the GMCP wizard-panel renderer is deferred,
    no negotiated client channel yet). Operates over an opaque `Entity`
    (handlers are content closures) and an `IO` interface (the session
    wires the real conn in M12.2). The completion pipeline (§6),
    restart (§7), and login handoff are M12.2 — the Instance only
    sequences steps and reports completion + exposes the assembled
    entity.

    - [x] Info auto-advances; choice accepts index + unique prefix and
          repeats on invalid/ambiguous; text runs validation; secret
          text toggles echo off-at-render / on-before-next-output; confirm
          treats y/n variants and rejects everything else; skip predicates
          bypass render + handlers (unit tests, 88% cov).
    - [x] Every rendered step emits a StepEvent with type + prompt
          (+ options for choice), the §5 structured seam (unit tests).

  - **M12.2 (landed) — Creating phase + commit pipeline.** The
    spec-faithful persistence reshape: `login` now BUILDS a new
    character's baseline entity but does NOT persist it
    (`buildNewCharacter` drops the old inline `Players.Save` +
    `AddCharacter` + welcome), so a mid-creation disconnect leaves
    nothing on disk (§8). The session owns the §6.4 commit: a new
    character's entity is assembled in `run()` (race/class/alignment/
    sustenance seeded), then `commitCreation` — under a process-wide
    creation mutex — re-checks the canonical name is free
    (`ErrNameConflict` last-chance → message + close, nothing written),
    persists the save, and links it to the account. `phaseCreating`
    added; set during the (synchronous, immediate-commit) creation
    window and flipped to `phasePlaying` at commit. `character.created`
    moved to AFTER commit + `Manager.Add` (§6.4 step 6) so the class-path
    level-1 grant never fires for a name-conflict loser and the notifier
    can resolve the now-registered actor. Returning players skip the
    pipeline entirely. M12.2 takes the §2 "no flow registered → immediate
    commit" path; the interactive wizard, input routing (§4), restart
    (§7), and the live mid-creation-disconnect window move to M12.3.

    - [x] New characters are not persisted at login; the commit pipeline
          is the first disk write (live-verified: new player reaches the
          world and `players/<name>/player.yaml` appears only after
          commit).
    - [x] Commit is mutually exclusive; a persisted name collision at
          commit returns `ErrNameConflict`, the session closes with a
          message, and the winner's record is untouched (session unit
          tests).
    - [x] Returning players are unaffected (end-to-end + takeover/
          link-dead tests stay green).
    - [x] `character.created` publishes after commit + placement.

    Deferred (see m12-2-deferred-fixes): MOTD enqueue (§6.4 step 9 — no
    MOTD content/command exists; welcome+look only); trigger-keyed
    multi-flow resolution (§2 — single nil-able flow seam suffices,
    lands with M12.3's flow); §8 disconnect-during-await cleanup +
    spawn-room "any room" last resort (exercised once M12.3's interactive
    flow holds the actor in Creating).

  - **M12.3 (landed) — Interactive creation flow.** `NewCreationFlow`
    builds the engine-default flow from the race + class registries
    (intro → race choice → class choice → confirm); `runCreation` drives
    the wizard primitive over the connection as a post-login, pre-actor
    phase (spec §3-§7), so the chosen race/class land on the baseline
    save and the existing M12.2 build/seed/commit path consumes them
    unchanged. A `creationIO` renders step text through the session's
    themed renderer and toggles telnet echo for secret steps; input
    routes only to the wizard with §4 help passthrough (`?` / `help` via
    the help service, without advancing the step); a confirm "no" fails
    validation and restarts against a fresh pending entity (§7); a
    disconnect mid-creation returns before any build/commit, so nothing
    is persisted (§8 — the real Creating window the M12.2 no-content path
    couldn't open). `CreationFlow` wired through `session.Config` from the
    composition root; nil keeps the §2 immediate-commit path.

    - [x] A new player chooses race/class interactively; the choices
          persist (live-verified: Dwarf/Fighter round-trips to disk, and
          the class-path level-1 ability grants now deliver because
          character.created publishes post-Add).
    - [x] Help passthrough answers `?`/`help` without advancing the step;
          non-help input never reaches the command router (the actor/
          command loop doesn't exist yet during creation).
    - [x] Confirm "no" restarts the flow against a fresh baseline; a
          mid-creation disconnect persists nothing (session unit tests
          with a scripted connection).
    - [x] Choice steps accept index or unique prefix (inherited from the
          M12.1 primitive); no-content registries yield a nil flow → §2
          immediate commit.

    Deferred (m12-3-deferred-fixes): §5 structured flow-step events /
    GMCP wizard-progress panel (nil sink — no negotiated client channel;
    plain clients get the prompt + numbered options the spec specifies);
    Option.Description carried but not surfaced in the menu (needs an
    inspect/help-on-race step); trigger-keyed multi-flow registry (still
    a single nil-able CreationFlow). **M12 complete.**

Each of these will get its own M2-style exit-criteria section when it's
the next milestone in flight.

---

### M13 — Social MUD

**Slice:** players can talk to each other across the world, not just in
their current room. Notification queue substrate, then tells, then
multi-recipient channels, then emotes. First themed milestone driven by
`docs/archive/THEME-AXIS-PLAN.md` (Theme A).

**Why this:** the world is real but socially flat — players in different
rooms have no way to interact. This is the single highest-leverage
product addition for a single-developer MUD; everything else (combat,
quests, training) gains weight once players can actually coordinate.

**Live plan + current step:** `docs/archive/themes/social-mud-plan.md`.

**Pre-decisions locked (2026-05-30):**
- Channels: hybrid (engine baseline + pack-defined additions)
- History: per-channel global ring buffer + per-player persisted tell inbox
- Ignore/block: deferred to a follow-up after channels land
- GMCP: plain telnet only (GMCP `Comm.Channel` is Theme B's job)

**Sub-milestones (exit criteria filled in during spec phase):**
- [x] **M13.1 — Notification queue.** Per-entity priority queue substrate.
      Spec + impl. Smallest, isolated.
- [x] **M13.2 — Tells.** Per-player `tells.yaml` inbox. Offline tells
      deliver on next login. `tell` + `reply` verbs.
- [x] **M13.3 — Channels.** Hybrid ownership. Global per-channel ring
      buffer in `saves/channels/<id>.yaml`. Engine baseline `ooc` +
      `admin`; pack-channel YAML schema. Verbs per channel.
- [x] **M13.4 — Emotes.** Registry-driven emote table with actor/target/
      room pronoun substitution. `smile`, `nod`, etc.

**Touches specs:** new `social-and-notifications.md` spec (or extension
to `session-lifecycle` + `commands-and-dispatch`; decided in M13.1).
`persistence` (new `saves/channels/` dir + `tells.yaml` shape + player-
save version bump for inbox pointer if needed).

**Demo target:** Two players in different rooms chat over `ooc`; one
tells the other privately; one emotes; both see channel history on
reconnect; the offline tell delivers when the recipient logs back in.

---

### M14 — Engine Debt

**Slice:** close the half-wired deferrals that accumulated across
M8-M11. The import cycle is already resolved (cluster 1, `af94b0c`);
this milestone finishes the consumers. No user-visible demo by
design — Theme E is internal cleanup that unblocks Themes B / C / D.

**Why this:** several real bugs hide behind these gaps today. A
potion's `effect_id` is published but never applied. A stat-change
event that raises max-HP doesn't bump current-HP. A mob's declared
race+class never shapes its actual stats. Each is a small piece;
together they're the engine substrate showing through.

**Live plan + current step:** `docs/archive/themes/engine-debt-plan.md`.

**Sub-milestones (order: independent block first, then chained):**
- [x] **M14.1 — Vital re-clamp on max-affecting stat recompute.**
      Listener seam on the stats recompute path; max changes flow
      to `combat.Vitals.SetMax` with current clamped as needed.
- [x] **M14.2 — Consumable EffectTemplate registry.** New
      `internal/effect.Registry` + pack-loaded `effects/*.yaml` +
      subscriber on `item.consumed` that resolves `effect_id` and
      applies via `effectMgr.Apply`.
- [x] **M14.3 — Mob stat derivation from race + class.** Wire the
      race / class lookups into `Store.SpawnMob`; apply modifiers
      to `MobInstance.StatBlock` under `race:<id>` and `class:<id>`
      source keys at spawn.
- [x] **M14.4 — Property registry on persistence.** New
      `internal/property.Registry` + tagged-value envelope codec;
      integrate with player save and entity instance properties.
- [x] **M14.5 — `world.Room.Property` bag.** Depends on M14.4.
      Adds the property bag to `world.Room`; pack-loadable.
- [x] **M14.6 — `quest_grant` on room.** Depends on M14.5. Quest
      watcher extends its existing item-side grant handler to
      read room properties on room-entry.

**Touches specs:** `progression` (vital re-clamp), `mobs-ai-spawning`
§3.2 (stat derivation), `economy-survival` (consumables),
`persistence` §2/§4.4 (property registry), `world-rooms-movement`
§2.2 (room property bag), `quests` (room-side grant).

**Pre-decisions (see plan doc):** PD-1 through PD-6 — package
location for property + effect registries, tagged-value envelope
type system, vital re-clamp mechanism, mob derivation timing,
pack-file glob additions.

**Closes from memory:** `m8-1` (vital re-clamp + mob StatBlock
consumer), `m11-5` (item.consumed effect application), `m10-9`
(quest_grant on room), `m6-2` (mob stat derivation).

---

### M15 — World Depth

**Slice:** the world has state beyond rooms and exits. Doors that
open + lock + need keys; portals that expire on a tick; weather
that shifts per area; recall as a saved return point. Closes
gap-matrix §1.8 (doors+locks) and §3 (portals / weather / recall).

**Why this:** the engine substrate is real now; the four-room town
feels flat without environmental state. Each item is contained
(no cross-cutting substrate work) and ships visible texture for
playtesting.

**Live plan + current step:** `docs/archive/themes/world-depth-plan.md`.

**Sub-milestones (order: doors → portals → recall → weather):**
- [x] **M15.1 — Doors + locks.** Per-exit state with paired
      reverse-side sync; open/close/lock/unlock verbs; key items;
      area-reset restoration. Spec §5.1-§5.5 already complete.
- [x] **M15.2 — Portals (temporary keyword exits).** Runtime
      keyword exits with TTL; cleanup tick handler; observable
      creation/expiry events. Spec §5.6 complete.
- [x] **M15.3 — Recall / return-home.** Per-character return
      address + `set recall` + `recall` verbs. Spec written
      (`docs/specs/recall.md`); player-save v14 carries the
      `recall` field; cancellable `recall.before` + post-fact
      `recall.after` events let content layers gate or react.
- [x] **M15.4 — Weather.** Area-scoped weather zones; hour-driven
      rolls subscribing to the in-game clock; per-state message
      tables; weather-exposed rooms render current state. Spec §6
      complete. **Theme C done.**
  - [x] **M15.4a — Substrate.** `internal/weather` package
        (Zone, Registry, Service with HourChanged / PeriodChanged
        seams, weighted-pick transition, message cascade,
        eligibility gate); `world.Room.Terrain` /
        `WeatherExposed` / `TimeExposed`; `world.Area.WeatherZone`;
        `weather.changed` bus event.
  - [x] **M15.4b — Wiring.**
    - [x] **M15.4b₁ — In-game clock.** `internal/gameclock`
          implementing time-and-clock §3 (CurrentHour, DayCount,
          TicksPerGameHour cadence, period boundary lookup,
          `time.hour.change` + `time.period.change` events).
    - [x] **M15.4b₂a — Loader + composition wiring.** Pack
          loader extensions (`weather_zones/*.yaml` schema,
          area `weather_zone`, room `terrain` /
          `weather_exposed` / `time_exposed`); composition-
          root binding (`game-clock` tick handler;
          `time.hour.change` → `Service.HourChanged`;
          `time.period.change` → `Service.PeriodChanged`);
          starter `temperate` zone shipped in `content/core`.
    - [x] **M15.4b₂b — Render integration.** `Service.Ambience`
          + `RenderRoom` ambience callback. `look` and movement
          renders show the current state's `ongoing` message
          in eligible rooms.

**Touches specs:** `world-rooms-movement` §5 (doors + portals), §6
(weather); new `recall.md` (or §7 section) for M15.3.

**Pre-decisions (see plan doc):** PD-1 (door state home — exit
field vs. service), PD-2 (closed: spec already picks per-area
weather), PD-3 (recall scope — verb-only vs. cooldown/cost/hooks),
PD-4 (door key entities — tag vs. property), PD-5 (portal creator
surface — admin / content / scripting).

**Order rationale:** Doors smallest and well-spec'd; portals reuse
the per-exit pattern; recall is tiny once its spec lands; weather
last because it crosses into the render path and the in-game clock.

---

### M16 — Modern Client

**Slice:** Mudlet / MUSHclient / Blightmud / browser clients see
real HUDs and panels instead of just scrolling text. Closes
gap-matrix §1.2 (GMCP), §1.3 (telnet IAC negotiation), §1.4
(WebSocket), §2 networking-protocols MSSP variables, and §2
ui-rendering-help 256/truecolor.

**Why this:** Theme C closed the world-state work; the next
user-visible payoff is what the *client* sees. `internal/conn` is
already well-abstracted so the blast radius is bounded.

**Live plan + current step:** `docs/archive/themes/modern-client-plan.md`.

**Sub-milestones (order: IAC+TTYPE+NAWS → MSSP → GMCP transport
→ GMCP packages → WebSocket → 256/truecolor):**

- [x] **M16.1 — Telnet IAC + TTYPE + NAWS.** Per-connection
      IAC subnegotiation state machine driven from Read.
      Server-initiated `IAC DO TTYPE` / `IAC DO NAWS` on first
      Read. TTYPE rotation captured per PD-5 (stop when name
      already seen). NAWS width/height tracked per re-emit.
      Capabilities exposed via `telnet.Conn.Capabilities()`.
      Spec §3.3-§3.4 + §4.1-§4.4.
- [x] **M16.2 — MSSP.** Server-discovery variables; crawlers
      list us. New `internal/mssp` package (Config + Encode →
      VAR/VAL payload); negotiator handles `IAC DO MSSP` →
      `IAC SB MSSP ... IAC SE`; `server.Server.TelnetOptions` +
      `telnet.WithMssp` thread the config through to every
      accepted conn. PLAYERS / UPTIME via dynamic factories
      against `session.Manager.Count()` and server start time.
      Spec §8.
- [x] **M16.3 — GMCP option negotiation + envelope.** Wire
      format + Core.Supports state machine. WILL GMCP added to
      initial offers; `IAC DO GMCP` activates; `IAC SB GMCP
      <pkg> SPACE <json> IAC SE` framing with IAC-doubled
      payload bytes; `Conn.SendGmcp / SupportsPackage /
      GmcpActive / SetGmcpHandler` surface; Core.Supports.Set
      / Add / Remove handled inside the negotiator (permissive
      default, prefix match per spec §5.3); other inbound
      packages dispatch to the engine-installed callback.
      Spec §5.1, §5.3, §5.5.
- [x] **M16.4 — GMCP packages.** Char.Vitals → Room.Info →
      Char.Items → Char.Combat → Char.Effects →
      Char.Experience → Comm.Channel → Char.Login. Spec §7.
  - [x] **M16.4a — Char.Vitals.** New `internal/gmcp` package
        (Tapestry-shape payload types per PD-2); per-actor
        last-sent shadow + `Manager.FlushGmcpVitals` walker
        registered as the cadence-1 `gmcp-vitals-flush` tick
        handler. Poll-and-diff implementation of PD-3:
        zero frames when nothing changed, one frame per session
        per tick max, no instrumentation across Vitals
        mutators.
  - [x] **M16.4b — Room.Info.** `gmcp.RoomInfo` payload
        (num/name/area/exits/keywords/terrain/details).
        Event-driven (no shadow): emitted from `connActor.SetRoom`
        on every transition + login spawn render + link-dead
        reattach. Cardinals flatten to short-form direction
        codes (n/s/e/w/u/d); M15.2 keyword exits land under
        their own keys.
  - [x] **M16.4c — Char.Items.** `gmcp.CharItem` +
        `gmcp.CharItemsList` (location-keyed: `inv` and
        `wear`). Poll-and-diff like Vitals with per-LOCATION
        shadows so an inventory change skips the wear frame
        and vice versa. Registered as `gmcp-items-flush`
        cadence-1 tick handler; link-dead reattach resets
        both shadows for a baseline frame on the new peer.
  - [x] **M16.4d — Char.Combat.** `gmcp.CharCombat`
        (in_combat + primary target name / id / HP /
        hp_percent). Poll-and-diff per actor; resolves the
        target via the same combat.Locator the combat package
        uses (threaded through Session.Config.CombatLocator).
        Registered as `gmcp-combat-flush` cadence-1 tick
        handler. Nil-locator path emits just the in_combat
        flag + TargetID so the wiring is opt-in for tests
        and future non-combat transports.
  - [x] **M16.4e — Char.Effects.** `gmcp.CharEffect` +
        `gmcp.CharEffectsList` (id + remaining + permanent
        flag + per-effect flags + source ability). Poll-and-
        diff per actor sourcing `progression.EffectManager.
        Effects(playerID)`; manager already returns a deep
        copy sorted by id so the shadow compare is stable.
        Permanent effects (negative duration) set
        `permanent:true` and drop `remaining`; time-bounded
        effects emit the live pulse counter. Registered as
        `gmcp-effects-flush` cadence-1 tick handler; link-
        dead reattach resets the shadow for a baseline frame
        on the new peer. Effects manager is wired onto
        connActor at construction so the flusher doesn't
        cross the cfg boundary.
  - [x] **M16.4f — Char.Experience.** `gmcp.CharExperience` +
        `gmcp.CharExperienceTrack` (track + display name +
        level + xp + xpnext + maxlevel + at_max + overflow).
        Multi-track shape — one entry per registered track via
        `TrackRegistry.All`. Poll-and-diff per actor sourcing
        `progression.Manager.GetTrackInfo` per track; lazy-init
        seeds (level=1, xp=0) for never-touched tracks so the
        baseline frame is stable. Max-level tracks set
        `at_max:true` + `overflow` and drop `xpnext`. Display
        name omits when equal to track id (saves wire bytes
        for content that doesn't configure separate labels).
        Registered as `gmcp-experience-flush` cadence-1 tick
        handler; link-dead reattach resets the shadow.
        Progression manager wired onto connActor at
        construction.
  - [x] **M16.4g — Comm.Channel.** `gmcp.CommChannelText`
        (channel + talker + text). Event-driven, NOT poll-
        and-diff: parallel-emit from `actorSink.Deliver` on
        every `Kind=="channel"` notification, alongside the
        plain-text write that ships to the main window. Routes
        the FULL rendered line (`[ooc] Alice: hello`) so
        bundled Mudlet chat plugins compatible with Tapestry's
        Comm.Channel.Text shape strip the prefix client-side.
        System messages (empty Sender) emit with `talker`
        omitted via omitempty. Required new field:
        `notifications.Notification.Channel` (populated by the
        chat verb in `command/chat.go`). Empty Channel id on a
        channel-kind notification silently skips the GMCP emit
        — main-window text still ships. GMCP send failures
        log at Debug rather than bubbling (would otherwise
        trigger notification re-enqueue and double-write the
        text line).
  - [x] **M16.4h — Char.Login + Char.StatusVars + Char.Status.**
        `gmcp.CharLogin` (name + fullname + account; all-emit),
        `gmcp.CharStatusVars` (static var→caption catalogue,
        `{vars:{…}}` envelope), `gmcp.CharStatus` (race + class +
        alignment + alignment_tag; alignment always-emits since 0
        is meaningful "neutral", others omitempty). Char.Login +
        Char.StatusVars are emit-once-per-activation (sent flags
        on the actor); Char.Status is poll-and-diff per tick.
        Static catalogue lives at package scope (every session
        sees the same map). One tick handler
        `gmcp-charstatus-flush` cadence-1 covers all three;
        link-dead reattach resets all three flags so the new
        peer's panels get fresh baseline identity frames.
        Closes M16.4.
- [x] **M16.5 — WebSocket transport.** Parallel-shippable;
      same package payloads, JSON envelope. Spec §6. New
      `internal/conn/ws` package implements `conn.Connection` +
      `gmcpSender` over a coder/websocket socket — Read pulls
      `{type:"command", data:"…"}` envelopes (skipping text /
      gmcp / unknown / malformed); Write emits one
      `{type:"text"}` text frame per call; SendGmcp emits one
      `{type:"gmcp", package, data}` envelope with the GMCP
      encoder's pre-marshalled JSON as the `data` raw value.
      WebSocket Conn reports `GmcpActive()=true` +
      `SupportsPackage(_)=true` unconditionally (§5.2/§6.5 —
      no negotiation; every package on for every session).
      Inbound size cap = 64 KiB (§6.3) via SetReadLimit; clean
      peer close maps to io.EOF for the session loop's existing
      EOF handler. New `server.NewWebSocketHandler` returns an
      `http.Handler` that upgrades each request, wraps in a
      ws.Conn, and dispatches through the shared `Server.Handler`
      — connection ids reuse `Server.nextID` so telnet + ws
      sessions share one numbering space. Composition root in
      `cmd/anothermud/main.go` starts an optional parallel
      `http.Server` when `ANOTHERMUD_WS_ADDR` is set; new env
      vars `ANOTHERMUD_WS_PATH` (default `/mud`),
      `ANOTHERMUD_WS_ORIGINS` (comma-separated origin patterns),
      `ANOTHERMUD_WS_INSECURE_SKIP_VERIFY` (dev only) tune the
      upgrade options. Empty `ANOTHERMUD_WS_ADDR` disables the
      listener entirely (telnet-only deployment unchanged). New
      dep: `github.com/coder/websocket` v1.8.14 — zero non-stdlib
      transitive deps, aligned with the repo's minimalist
      posture. 10 ws.Conn tests (ID, Write envelope shape,
      SendGmcp envelope shape, Read returns command data, skips
      unknown / text / gmcp / malformed JSON, returns EOF on
      normal close, returns error on ctx cancel,
      SupportsPackage always true, Connection-interface compile
      assertion) + 2 server-package integration tests (accept +
      round-trip; no-handler 500). `go test -race ./...` clean.
- [x] **M16.6 — 256 / truecolor.** Per-session render tier
      selection driven by captured TTYPE.
  - [x] **M16.6a — Capability detection.** New
        `render.ColorTier` enum (None / Basic / Extended /
        TrueColor) defined in `internal/render` so the future
        renderer dispatch consumer (M16.6b) doesn't drag a
        telnet import. `telnet.Capabilities` extended with
        `ClientName`, `IsMudClient`, and `ColorSupport`.
        `internal/conn/telnet/capabilities.go` houses the
        known-MUD-client allowlist (Mudlet, MUSHclient, TinTin++,
        ZMud/CMud, Atlantis, Potato, BlowTorch, KildClient,
        BeIP, GnomeMUD — case-insensitive substring match) +
        `deriveColorTier` (TRUECOLOR > 256COLOR > known-mud-
        client > basic > none). `telnet.Conn.ColorTier()` +
        `ws.Conn.ColorTier()` (always TrueColor per §6.5)
        expose the tier through the conn abstraction; session
        captures it once at construction via a `colorTierSource`
        interface (test-fake fallback = Basic to preserve M0-era
        behavior). `connActor.ColorTier()` accessor +
        Debug-level `session.color_tier` log at session-add.
        M16.6b will wire tier-aware ANSI emission.
  - [x] **M16.6b — Tier-aware ANSI emission.** `ThemeRegistry.
        Compile()` now produces three per-tier compiled maps
        (Basic/Extended/TrueColor) keyed by tag; the existing
        Basic map remains as the back-compat target for
        `Resolve(tag)`. New `ResolveForTier(tag, tier)`
        returns the per-tier pair, returning (zero, false)
        for ColorTierNone. Hex→SGR helpers (`hexToTrueColorSGR`,
        `hexTo256SGR`, `nearestXterm256`) live in `render/hex.
        go`; Extended uses the xterm-256 6×6×6 cube + grayscale
        ramp, TrueColor emits `\x1b[38;2;R;G;Bm`. ColorRenderer
        gains `RenderAnsiForTier(s, tier)` with a tier-aware
        cache key `(s, tier)`; `RenderAnsi(s)` stays as an
        alias for ColorTierBasic for back-compat. connActor's
        `render()` dispatches off the captured `colorTier`
        when ColorEnabled (the admin override still wins, even
        for TrueColor-capable clients). Theme entries without
        HTML degrade gracefully — Extended/TrueColor fall
        through to the ANSI-16 FG/BG path. Content theme
        updates deliberately scoped out per the M16.6 plan.
        Closes M16.

**Touches specs:** `networking-protocols` end-to-end; eventually
`ui-rendering-help` §3 for the color tier follow-up.

**Pre-decisions (see plan doc):** PD-1 (per-client subscribe
model — defer to M16.3), PD-2 (payload shape — defer to M16.4a),
PD-3 (vitals batching — defer to M16.4a), PD-4 (HTTP listener
— defer to M16.5), PD-5 (TTYPE rotation policy — closed in M16.1
as "stop when name already seen").

**Order rationale:** TTYPE+NAWS is cheapest and gives immediate
panel-width data even before GMCP. MSSP follows naturally (same
IAC machinery, tiny payload). GMCP transport is the headline;
packages are small individually but plural. WebSocket runs
parallel because internal/conn is clean. Truecolor closes the
loop once clients have advertised their tier via TTYPE.

---

### M17 — Content Authoring (Theme D)

**Why this:** M13/M14/M15/M16 closed the engine surface. The
remaining theme-axis lane is content authoring — give pack
authors a scripting runtime so they don't need to recompile the
engine to add behavior. Largest theme on the plan; scripting is
the keystone.

**Pre-decision (locked):** scripting language = **gopher-lua**
(user choice, M17.1a). The Engine type is the abstraction
boundary so a future runtime swap (goja, Starlark) is feasible
without rewriting scripts.

- [x] **M17.1 — Scripting runtime.** Sandbox + bus bridge +
      minimal engine API.
  - [x] **M17.1a — Sandbox substrate.** New
        `internal/scripting` package; `Engine.Run(ctx, packID,
        scriptPath, script)` constructs a fresh sandboxed
        `*lua.LState` per call (concurrent-safe, no internal
        locking). Sandbox: `os`/`io`/`debug`/`package` libs NOT
        loaded; base lib loaded but `dofile`/`loadfile`/`load`/
        `loadstring`/`collectgarbage`/`getfenv`/`setfenv`/
        `module`/`require`/`newproxy`/`print`/`_printregs`
        stripped; `table`/`string`/`math` loaded as safe
        pure-data libs. Limits: per-Run timeout (default 50ms,
        context-cancel based — works against the parent ctx
        too); `MaxRegistrySize` caps the VM register stack;
        `MaxCallStackSize` caps recursion depth. Errors wrap
        into `*scripting.Error` carrying `PackID`, `ScriptPath`,
        and the underlying lua error; context cancellation
        passes through verbatim so `errors.Is(err,
        context.DeadlineExceeded)` works. Known gap: gopher-lua
        has no clean way to cap the Go-side heap behind Lua
        tables — RegistryMaxSize doesn't reach it and SetMx
        calls os.Exit on overflow (unacceptable for a server).
        Wall-clock timeout is the load-bearing memory defense
        until a real allocation counter lands. 14 tests cover
        the happy path, every denied global, every
        unloaded namespace, sandbox attribution on require,
        timeout (infinite loop), parent-ctx cancel,
        call-stack overflow, allocation-storm bounded by
        timeout, error format / Unwrap. New dep:
        `github.com/yuin/gopher-lua` v1.1.2.
  - [x] **M17.1b — Pack discovery.** Manifest `content.scripts:
        ["scripts/*.lua"]` glob picks up Lua files alongside
        existing YAML. New `internal/script` package owns the
        boot-time `Registry` (PackID, Path, Source, LoadOrder
        tuples; deterministic All() sort by LoadOrder then
        Path; rejects (PackID, Path) duplicates). Pack loader
        reads each script, calls `scriptCompiler.Compile`
        (new method on `*scripting.Engine` — LoadString-only,
        no execution, no Timeout) to surface syntax errors with
        pack + path attribution at boot, then registers the
        Entry. New `pack.ScriptCompiler` interface (defined at
        the use site so the pack package doesn't import
        internal/scripting directly) threaded through
        `pack.Load`; nil-safe so tests can skip the compile
        check. Composition root constructs a sandboxed
        `*scripting.Engine` and passes it. 7 registry-level
        tests (round-trip, sort stability, duplicate rejection,
        cross-pack same-path allowed, concurrent registration,
        snapshot semantics) + 3 Compile tests (valid,
        attributed syntax error, no side-effects) + 4
        integration tests (discovery, syntax-error attribution
        through Load, empty-glob clean load, nil-compiler skips
        check). `go test -race ./...` clean.
  - [x] **M17.1c — Bus bridge + minimal API.** New
        `*scripting.Sandbox` (long-lived LState with per-instance
        mutex; `Run` for the boot registration pass, `Call` for
        per-event handler invocation, both honor the per-call
        timeout + `recover()` wrap so a Go-side panic from an
        LGFunction surfaces as `*Error` rather than crashing the
        caller — closes the M17.1a-deferred MEDIUM). New
        `*scripting.Runtime` wires Sandboxes to the eventbus.Bus:
        `LoadRegistry` constructs one Sandbox per `script.Entry`,
        installs the `engine` global (`subscribe(name, fn)` +
        `log(msg)`), runs the script body once to register
        handlers. Lazy bus subscription per event name; one
        Go-side handler fans out to all Lua subscribers under
        each Sandbox's lock. New `marshal.go` reflectively walks
        event struct fields and presents them as a snake_case
        Lua table (`MobID` → `mob_id`, `XMLParser` → `xml_parser`).
        Composition root wires `scripting.NewRuntime(engine, bus)`
        + `LoadRegistry(registries.Scripts)` after pack load with
        `defer runtime.Close()`. Demo target landed:
        `content/core/scripts/track_kills.lua` subscribes to
        `mob.killed` and logs every kill via `engine.log`. 9
        runtime tests (register-dispatch, multi-pack same event,
        argument validation rejects non-string name, handler
        error doesn't affect siblings, Close unsubscribes +
        idempotent, concurrent dispatch -race-clean, nil
        registry no-op, log binding callable, snake_case
        marshalling).
- [x] **M17.2 — Arg typing.** `commands-and-dispatch §5`.
      Independent of the scripting runtime; landed in parallel.
      Substrate (M17.2a-c) + dispatch integration and handler
      migration (M17.2d) all shipped.
  - [x] **M17.2d — Handler migration + production adapter.**
        **CLOSES M17.2.** Wired the §5 arg-typing pipeline into
        live dispatch (Option A: dispatcher pre-resolves declared
        `Command.Args`, empty = legacy passthrough) and migrated
        every verb that fits. **Migrated (14):** drop, get, put,
        equip, give, consider, kill (+ cast targeting), eat,
        drink, use, open, close, lock, unlock — exercising every
        arg shape (inventory, room_item, container, player,
        entity, keyword, door, bulk, ordinals). **Left
        hand-parsed as documented non-fits:** `unequip` (no
        `equipped` arg type in §5.2 — needs a spec decision),
        `fill` (source-scope analysis pending), and
        `buy`/`sell`/`value` (resolve inside `economy.ShopService`
        against shop-stock / dual-scope, which no engine arg type
        covers — a text-arg migration would be cargo-cult). See
        `memory/m17-2d3-deferred-fixes.md` for the non-fit
        rationale and the dispatch-model decision record. Standard
        behavior changes across migrated verbs: single-token item
        references and §5.4-standardized error copy.
    - [x] **M17.2d₆ — Door verbs (open / close / lock / unlock).**
          First production consumer of the M17.2c `door` resolver +
          M17.2d₁ `worldDoorScope` (built but previously unused).
          Each verb declares one `door` arg; `doorOpHandler` reads
          the resolved `DoorRef`, parses its short direction back to
          a `world.Direction`, and re-fetches the live `DoorState`
          via `GetDoor` (the per-op checks want current state — same
          as the old GetDoor-before-switch). The four
          `handleOpen/Close/Lock/Unlock` functions are unchanged.
          `world.ResolveDoorTarget` is still exercised — now via the
          door scope adapter rather than the handler. **Behavior:**
          single-token/direction/ordinal forms identical; `open iron
          gate` now resolves via the "iron" token (the old
          `strings.Join` produced "iron gate", which never matched a
          single-token keyword anyway). Standardized copy: missing
          arg → "What door?", ambiguous → "Which door do you mean?",
          not found → "You don't see a door like that here." Updated
          2 message tests. Coverage 81.9%.
    - [x] **M17.2d₅ — Consumable verbs (eat / drink / use).**
          All three route through `consumeVerb`, which now reads a
          single declared `inventory` arg (Option A) via
          `resolvedItemInstance` instead of hand-parsing. §6.5
          (nested-in-container items aren't consumable) is
          preserved automatically — the inventory scope is built
          from `Actor.Inventory()` only. **Behavior change:** the
          missing-arg prompt is the §5.4 `"What item?"` instead of
          the verb-specific `"Eat what?"` / `"Drink what?"` /
          `"Use what?"`. Rewrote the consume tests to dispatch
          through the registry (the old direct-handler calls left
          `c.Resolved` nil); nil-service and wrong-method guards
          still verified. Coverage 81.9%.
    - [x] **M17.2d₄b — consider / kill via the entity resolver
          (Option B).** The self-referencing combat verbs can't
          pre-resolve under Option A (the `entity` arg excludes
          self, so `consider self` would fail before the handler
          runs). Instead the shared `findCombatantInRoom` helper
          was rewritten to delegate to the §5 `entity` resolver
          (via a new `Context.ArgResolver` the dispatcher injects,
          with a fresh-registry fallback for direct-call tests) +
          a `resolveCombatantRef` re-fetch (mob → Store, player →
          Locator). consider/kill keep their existing self-checks
          before the call; `cast` (abilities) self-targets via the
          empty-target path — all three now share uniform combat
          targeting. **Behavior changes (consistent with d₄a):**
          players are keyword/partial-matchable (`kill bo` → Bob)
          and ordinals (`kill 2.rat`) work for both kinds; a named
          self-target is now excluded (self-cast uses the no-target
          form). `findMobByKeyword` retained (emote uses it
          directly). Updated 3 player-via-locator fixtures to the
          enumerating stubLocator; added a partial-name test.
          Coverage 81.9%.
    - [x] **M17.2d₄a — Player enumeration + give.** Closes the
          M17.2d₁ player gap. Added `Locator.PlayersInRoom(roomID)
          []Actor` (session `managerLocator` impl over a new
          `Manager.roomConnActors` snapshot; 3 command test fakes
          get nil/real stubs). New `playerCandidate` adapter
          (nil Keywords → name-substring matching) and
          BuildResolveContext appends room players to RoomEntities
          AFTER mobs (mobs win exact-keyword ties), excluding the
          actor itself (§5.2 self-exclusion). Migrated `give` →
          `inventory` + `player` (prep `to`, Option A); handler
          re-fetches the recipient Actor by the resolved name.
          Removed dead `parseGiveArgs` + its internal test.
          **Behavior changes:** players are now keyword/partial-
          matchable (`give sword al` → Alice) — the old exact-
          name-only rule is gone; self-give and missing-target
          now report the player not-found sentinel / §5.4 prompt.
          Coverage 82.7%.
    - [x] **M17.2d₃ — Item-verb batch (get / put / equip).**
          Migrated the three item verbs that need only the
          existing inventory / room_item / container / keyword
          scopes. `get` → one `room_item` arg; `put` →
          `inventory` + `container` (preps `in`/`into`); `equip`
          → `inventory` + `keyword` slot. New shared
          `resolvedItemInstance(c, name)` helper re-fetches the
          live `*ItemInstance` from a resolved `ItemRef.ID`
          (TOCTOU guard); `drop` refactored onto it too.
          Removed now-dead `parsePutArgs` + `accessibleContainers`
          (+ the `public` placeholder) — the `container` resolver
          subsumes both, and put's §4.5 step-1 type check is now a
          resolution invariant. **Accepted behavior changes:**
          single-token item references only (multi-word phrases
          like `put short sword in bag` regress — use a single
          keyword) and §5.4-standard error copy (`What item?` /
          `What container?` / `What slot?`, container/room
          not-found sentinels). Deferred (genuinely blocked):
          `give` (player target — Locator can't enumerate room
          players), `unequip` (no `equipped` arg type in §5.2),
          `fill` (source scope analysis pending). Updated 4
          existing tests for the new copy; added ordinal-through-
          dispatch + drop coverage. Coverage 82.8%.
    - [x] **M17.2d₂ — Dispatch integration (Option A) + first
          handler.** `Command.Args []ArgDefinition` +
          `Context.Resolved map[string]any`; the `Registry`
          owns an `*ArgResolverRegistry` (seeded in `New()`,
          exposed via `ArgResolvers()` for pack types). New
          `resolveRegistration` returns the matched
          registration (handler + Args); `Resolve` now delegates
          to it (one routing rule). `Dispatch` gained the §5
          step: when a command declares Args, build the
          ResolveContext, resolve against `c.Args`, log warnings
          at Debug, write the resolver error + skip the handler
          on failure, else store the map in `c.Resolved`. Empty
          Args = legacy raw-`c.Args` passthrough (un-migrated
          handlers untouched). First migrated verb: `drop`
          (declares one `inventory` arg; re-fetches the live
          instance by the resolved `ItemRef.ID`, which doubles
          as the TOCTOU guard). **Behavior change:** drop's
          missing-arg / not-carried copy is now the §5.4-
          standard resolver text ("What item?" / "You aren't
          carrying that.") instead of the hand-rolled "Drop
          what?" / "You aren't carrying anything." 7 dispatch +
          drop tests; coverage 82.7%. No composition-root change
          (registry self-seeds via New()).
    - [x] **M17.2d₁ — Production ResolveContext adapter.**
          New `internal/command/argcontext.go`: candidate
          adapters bridging the M17.2b/c resolver interfaces
          to runtime types — `itemCandidate` over
          `*entities.ItemInstance` (also satisfies
          `ContainerCandidate`; `IsContainer` mirrors the put
          verb's `Type()==container` test), `mobCandidate`
          over `*entities.MobInstance` (EntityType constant
          `mob`), and `worldDoorScope` over `*world.World`
          (wraps `ResolveDoorTarget`→`GetDoor`, **single-token
          door addressing** — resolves the M17.2c door
          deferral). `(*Context).BuildResolveContext()`
          assembles inventory / room-item / room-mob / door
          scopes + actor self-tag from a live Context, nil-safe
          for partial test contexts. Killed the `player`/`mob`
          magic strings in the resolvers with
          `entityType{Player,Mob}` consts. 10 tests resolve
          every scope end-to-end through the real registry;
          coverage 83.0%. **KNOWN GAP:** room players aren't
          enumerated (Locator is name-only) so RoomEntities
          carries mobs only — entity/player/visible won't
          surface other players until the Locator gains room
          enumeration (M17.2d₂). No handler migration yet.
  - [x] **M17.2c — Ordinals, bulk, and the door resolver.**
        Closes the §5.5/§5.6 selection surface. Ordinal
        selection (`<n>.<keyword>`) already worked across all
        eight selecting types because every entity resolver
        calls the ordinal-aware `keyword.Resolve`; M17.2c
        pins it with boundary tests. Bulk (`all` /
        `all.<keyword>`) added to the two bulk-capable types
        (`inventory`, `room_item`): when `ArgDefinition.Bulk`
        is set AND the token is a bulk token, the resolver
        runs `keyword.ResolveAll` and returns `[]ItemRef`;
        a bare keyword on a bulk arg still yields a single
        `ItemRef` (the bulk variant is specifically the `all`
        form), and ordinal vs. bulk are trivially mutually
        exclusive per token. Zero-match bulk surfaces the
        type's not-found sentinel so a successful bulk resolve
        always carries ≥1 element. New `door` resolver returns
        `DoorRef{Direction, Door DoorInfo{Name, Closed,
        Locked, KeyID}}` (§5.6) via a new `DoorScope` lookup
        interface on `ResolveContext` (doors resolve by
        direction OR keyword against the room graph, so the
        seam is an interface, not a pre-filtered slice; the
        production adapter over `world.World` lands M17.2d).
        Three door outcomes map to value / `ErrDoorAmbiguous`
        / `ErrNoSuchDoor`; nil scope → `ErrNoSuchDoor`. 13
        new tests; package coverage 82.7%.
  - [x] **M17.2b — Entity / inventory / room resolvers.** New
        `ResolveContext{Inventory, RoomItems, RoomEntities,
        ActorName, ActorID}` threaded through `ResolverInput`
        via a new `ResolveArgsWithContext(defs, tokens, ctx)`
        driver entry point (existing `ResolveArgs` keeps its
        zero-context signature). Eight new resolvers wired
        into the registry: `inventory`, `room_item`, `entity`,
        `player`, `npc`, `container` (inventory-first then
        room, filters via `ContainerCandidate.IsContainer`),
        `visible` (scans self → inventory → room → entities,
        returns VisibleRef carrying the §5.6 `Source`
        discriminator), `findable` (inventory-first, no
        container filter). New result shapes per §5.6:
        `ItemRef{ID, Name, Keyword, TemplateID}`,
        `EntityRef{ID, Name, Type}`, `VisibleRef` (embeds
        EntityRef + Source). New candidate interfaces
        (`ItemCandidate`, `EntityCandidate`,
        `ContainerCandidate`) extend `keyword.Named` so
        resolvers stay decoupled from concrete entity types.
        19 tests cover every resolver path. Production wiring
        (concrete adapter from connActor + room entities)
        lands in M17.2d.
  - [x] **M17.2a — ArgDefinition substrate + keyword/text/
        number.** New `command.ArgType` + 12 engine-baseline
        constants (keyword/text/number/inventory/room_item/
        entity/player/npc/container/visible/findable/door —
        the latter 9 land in M17.2b+ but are reserved here so
        a pack can't squat the names). `ArgDefinition`
        struct (Name, Type, Optional, Bulk, Prepositions,
        BypassVisibility) — Optional is the zero-value
        idiomatic inverse of the spec's "required default
        true". New `ArgResolverRegistry` seeded with the
        keyword/text/number resolvers; pack-Register rejects
        engine-type names via `ErrEngineTypeImmutable`
        sentinel, pack-name re-register is last-wins (§5.3).
        `ResolveArgs(defs, tokens) → (map, warnings, rest,
        err)` driver implements §5.4 step-by-step: §5.4 step 1
        preposition skip (case-insensitive, immediately-
        before-only), step 2 text early-out (slurps remainder
        joined by spaces), step 3 missing-required short-
        circuit, step 4 resolver dispatch with unknown-type
        keyword fallback + warning collection, step 5 cursor
        advance by resolver-reported Consumed count. New
        `ArgResolveError` carries ArgName + Cause; the spec's
        "What &lt;name&gt;?" string is formatted when Cause is
        `ErrMissingRequired`, otherwise Cause.Error()
        surfaces verbatim. 22 tests cover every §5.4
        acceptance criterion plus the engine-immutability +
        last-wins + unknown-fallback + optional-miss-nil
        surface. No handler migration yet — M17.2d wires
        existing handlers to the new path.
- [x] **M17.3 — Hot reload.** Script-only reload, shipped across
      M17.3a (primitives) + M17.3b (verb + wiring + spec). Rebuilds
      the scripting sandboxes + bus subscriptions without touching
      `world.World` or the content registries. Amended the
      `scripting-and-packs` §1 non-goal: content hot-reload stays
      out (live world mutation unsafe), script hot-reload is now in.
  - [x] **M17.3b — `reload` verb + composition wiring + spec.**
        New `command.ReloadHandler` registered bare (ungated, like
        `xp`, until the role system can gate it — M10+/M12); it
        calls the `Context.ReloadScripts` closure and reports the
        loaded count, or surfaces the underlying error verbatim
        (the `*scripting.Error` carries pack/script/Lua attribution
        the operator needs). Threaded `ReloadScripts func(ctx)
        (int, error)` through `session.Config` → `command.Env` →
        `Context` (mirrors the `Flee` closure). Composition root
        wires it to `pack.DiscoverScripts(cfg.ContentDir, …,
        scriptEngine)` → `scriptRuntime.Reload` → `fresh.Len()`.
        Amended `scripting-and-packs` §1: content reload remains a
        non-goal, script reload is supported (trigger surface is a
        host concern). 3 verb tests (disabled-when-unwired, count,
        error-surface). Full suite 45 pkgs, -race clean.
  - [x] **M17.3a — Reload primitives.** `pack.DiscoverScripts`
        (discovery + script-glob + compile-check ONLY — no
        content parse, no spawning, so it is safe on a live
        server; reuses pack discovery/ordering so reloaded
        LoadOrder matches boot) and `scripting.Runtime.Reload`
        (reloadMu-serialized: detach bus subs + Close old
        sandboxes under the swap lock, then re-run the new
        registry). Call-after-Close is already safe (Sandbox
        returns a "closed" error, not a panic), so an in-flight
        dispatch racing a reload degrades to a logged handler
        error. `LoadRegistry` refactored to append sandboxes
        under `mu` (lock-safe); `Close` reshaped to the same
        detach-then-teardown lock order. `loadScripts`
        re-pointed at a `*script.Registry`. Known edge: a script
        that compiles but errors during its registration pass
        yields a partial reload (DiscoverScripts' compile-check
        catches syntax errors before teardown). 6 new tests.
        M17.3b wires the `reload` verb + composition-root trigger
        + the spec amendment.
- [x] **M17.4 — Schedule primitive.** **CLOSES M17 / Theme D.**
      New `engine.schedule(delayTicks, fn)` Lua binding: a one-shot
      callback fired `delayTicks` engine ticks later (tick = 100ms),
      fire-and-forget (no cancel handle in v1; pending callbacks are
      dropped on Reload/Close). The `Runtime` keeps a
      `{dueTick, sandbox, fn}` queue under a dedicated `schedMu`;
      `Runtime.Tick(ctx, tickCount)` (a `tick.Handler`) sets
      `lastTick`, collects due entries under the lock, and fires
      them OUTSIDE it via the same sandbox-locked path as bus
      dispatch (a re-arming callback lands on a future tick; a
      closed-sandbox call after a concurrent reload logs-and-
      continues). Idle fast path when nothing is queued. Composition
      root registers `scriptRuntime.Tick` at cadence one. Demo:
      `track_kills.lua` schedules a follow-up log ~3s after each
      kill. 5 tests (due-tick, one-shot, relative-to-current-tick,
      reject delay < 1, dropped-on-reload). Full suite 45 pkgs,
      -race clean.

**Touches specs:** `scripting-and-packs` (the substrate);
`commands-and-dispatch §5` (arg typing).

---

### M18 — Command & UI polish

**Slice:** the small, already-specced command/UI items that polish the
play experience — set your prompt, list who's online, command chaining
and repeat, bad-input escalation, and help-syntax auto-generation. Pulled
from `BACKLOG.md` §1 "Command & UI polish" cluster; each item is verified
against code and reads its cited spec section first.

**Live list:** `BACKLOG.md` §1 (delete an item there as it ships here).

**Sub-milestones:**

- [x] **M18.1 — `prompt` verb.** Show / set / reset the player's status
      prompt template (ui-rendering-help §7.4, written first this slice —
      the verb behavior was unspecced; §7 only covered rendering). New
      `command.promptController` (`PromptTemplate` / `SetPromptTemplate`)
      satisfied by `connActor`; bare `prompt` verb stores the rest-of-line
      verbatim (internal spacing + color tags preserved), `prompt default`
      / `prompt reset` clears to the default, length-capped at
      `MaxPromptTemplateLen` (240). Set/clear marks the save dirty +
      flags a prompt refresh so the change shows next flush. 10 tests
      (8 command + 2 session), -race clean.
- [x] **M18.2 — `who` verb.** World-wide online roster (who §2–§4): one
      alphabetical line each + summary count, `[Admin]` role marker, `(idle)`
      marker (reuses the idle-sweep `lastInputAt`). New `command.Roster` seam +
      value `WhoEntry` snapshots; session `managerRoster` adapter with the
      `PlayersInRoom` lock discipline. v1 shows everyone; per-viewer hiding
      attaches at the adapter when visibility lands. `-race` clean.
- [x] **M18.3 — Bad-input tracker.** commands-and-dispatch §6 — informational,
      NOT escalation (the router never changes routing). `command.BadInput
      Tracker` (count/first/last per verb, atomic Record, count-desc Snapshot,
      Clear); dispatcher records + logs `event=command.unknown` at the unknown-
      verb miss (not the admin-gate "Huh?"); `badinput` admin verb renders the
      ranked snapshot. Mob verbs excluded by construction. OTel counter
      deferred to Ops. `-race` clean.
- [x] **M18.4 — Command chaining `;` + repeat `3n`.** commands-and-dispatch §4.
      `command.ParseInput` splits/expands (cap-bounded, repeat `3n`/`12east`/
      `2pick item`, pure-digit passthrough, overflow-clamped); pump dispatches
      each segment in order. Synchronous v1 (per-tick pacing out of scope per
      §4.4). `ANOTHERMUD_CHAIN_CAP` (default 10).
- [x] **M18.5 — Auto-help synthesis from arg defs.** commands-and-dispatch §8.
      `GenerateHelpTopics` now synthesizes the syntax line from a command's
      `ArgDefinition`s (`[req]`, `([opt])`, `[x | all | all.x]`, prepositions
      in position) via `synthesizeSyntax`; untyped commands keep hand-authored
      Syntax; pack help still overrides. Deliberate superset (a topic per every
      command).

**M18 complete** — all five sub-milestones shipped.

**Touches specs:** `ui-rendering-help §7.4` (new), `commands-and-dispatch
§4/§6/§8`, `who §2–§4` (new).

---

### M19 — Roles & Administration

**Slice:** the keystone authorization system — a per-character role set with
a flat `HasRole` check — and the admin verb surface built on it. Implements
the two specced contracts `roles-and-permissions` + `admin-verbs`. Finally
gates the standing ungated `reload` / `xp` verbs, and unblocks the auction
house's admin moderation (`auction-house.md` §11).

**Live list:** `BACKLOG.md` §1 (Roles & permissions, Admin verbs).

**Sub-milestones:**

- [x] **M19.1 — Role-set substrate + persistence + `HasRole` + config seed.**
      `player.Save.Roles` (v14→v15 migration; legacy saves load as the empty
      set). `connActor` carries a live, normalized role set — a **separate
      namespace from gameplay tags** (it never crosses into `Tags()`) — with
      `HasRole` (read-only, case-insensitive), `Roles()`, and idempotent
      `grant`/`revoke` mutators. `applyRoles` (a construction hook beside
      `applyRace`/`applyClass`) restores saved roles, then applies the config
      seed additively + idempotently, persisting + dirtying so the bootstrap
      admin survives first login. Operator config `ANOTHERMUD_ROLE_SEED`
      (`name:role,role;name:role`). 18 tests (2 player + 9 session + the
      parseRoleSeed table), full suite 46 pkgs, -race clean. roles-and-
      permissions §2/§3/§5/§6.
- [x] **M19.2 — `grant` / `revoke` verbs + `role.granted` / `role.revoked`
      events.** `grant <role> to <player>` / `revoke <role> from <player>`
      (lenient on the preposition). Self-gated on the granting role
      (`ANOTHERMUD_GRANTING_ROLE`, default `admin`) with a refusal that does
      not disclose the gating role (§3); **self-grant blocked** (§1.1);
      target resolved to an online player via `Manager.GetByName` (v1
      online-only, §9). Reuses the M19.1 idempotent mutators and emits the
      non-cancellable fact event **only on an actual change** (§2/§7). New
      `command.RoleController` + `RoleTargetResolver` seams + a session
      adapter. 9 command tests (refuse/grant/revoke/idempotent/self-block/
      offline/disabled/usage). Full suite 46 pkgs, -race clean. §4/§7.
- [x] **M19.3 — The admin gate.** A command `Admin` flag the dispatcher
      refuses unless `HasRole`, refusal indistinguishable from an unknown
      verb; gates today's ungated `reload` / `xp`. admin-verbs §2.
      Flag flows through cmdMeta + every registration (primary + alias) into
      CommandInfo; minimal `RoleHolder` (HasRole) check at dispatch before
      the Context is built; admin verbs take the admin help tier (hidden
      from non-admins); `Env.AdminRole` from `ANOTHERMUD_ADMIN_ROLE`. `grant`
      / `revoke` / `xp` / `reload` admin-marked; grant/revoke keep their
      granting-role self-gate. da96420.
- [x] **M19.4a — The `admin.action` audit primitive + `announce`.** The
      accountability choke point every admin verb shares (admin-verbs §6):
      a non-cancellable `admin.action` fact (`eventbus.AdminAction` =
      actor/verb/target/args) routed to the operator log via the shared
      `auditAdmin` helper, proven end-to-end with `announce` (§5) — the
      simplest baseline verb (server-wide broadcast, no target resolution,
      no mutation). New `command.Announcer` seam (all-sessions `SendToAll`,
      satisfied by `session.Manager`); `announce` admin-marked so the M19.3
      gate refuses non-admins with the indistinguishable "Huh?". 4 command
      tests (broadcast+audit / usage / disabled / non-admin refusal). Full
      suite -race clean. admin-verbs §5/§6.
- [x] **M19.4b — Administrative target resolution + `inspect`.** The
      read-only diagnostic verb (admin-verbs §5) and the §3 resolution it
      proves out. `inspect [<target>]`: no arg → self; otherwise resolves a
      player or mob in the room via the shared §5 entity path
      (`findCombatantInRoom`). Renders a multi-line dump via capability
      type-asserts — identity/vitals/stats always, plus roles/levels/
      equipment (player) and tags/properties (mob), degrading by kind. §3
      visibility bypass is a documented no-op today (hide/sneak rules are
      still greenfield — BACKLOG; the bypass attaches here when they land).
      Audited via the M19.4a `auditAdmin` choke point. 4 command tests
      (self+audit / non-admin refusal / room-mob dump+audit / not-found).
      Full -race suite green. admin-verbs §3/§5.
- [x] **M19.4c — The `set` field-write framework + the `vital` kind.** The
      general-purpose admin write (admin-verbs §4): `set <kind> <type>
      <target> <value>` with an extensible field catalogue, parse/validate/
      usage-panel, room/self target resolution (reused from inspect), and
      audit. Ships the `vital` kind (`set vital hp <target> N`, clamped to
      max via the new `Vitals.SetCurrent`); `property` + `tag` kinds land as
      incremental catalogue entries next. Roles are deliberately absent from
      the catalogue (privilege goes through grant/revoke, §4). **`set`
      reclaimed for admin** — the former player `set recall` relocated to
      `recall set` (sub-form of `recall`); admin `set` is admin-marked so
      the M19.3 gate hides it + refuses non-admins. recall.md §7 updated.
      9 tests (vital live+audit / clamp / non-numeric / unknown-vital /
      unknown-kind / bare-usage / non-admin refusal + SetCurrent unit +
      recall-set migration). Full -race suite green. admin-verbs §4.
- [x] **M19.4d — `restore` + `teleport`.** Two baseline mutating verbs
      (admin-verbs §5). `restore [<target>]`: the mercy verb — set a
      target's vitals to full via `Vitals.SetCurrent(max)`; no arg restores
      self; room target otherwise (reuses M19.4c `resolveSetTarget`).
      `teleport <room-id|player>` (alias `goto`): move the actor to a room
      by id or to the room of a named online player — the §3 **world-scoped
      resolution** via the new `command.PlayerRoomResolver` seam
      (`session.PlayerRoomResolver` over `Manager.GetByName`+`RoomOfPlayer`).
      Reuses `SetRoom`'s room-change events + the recall vanish/appear
      broadcast pattern; same-room is a no-op. Both audited via auditAdmin.
      12 tests (restore: full/not-found/non-admin; teleport: room/goto-alias/
      to-player/unknown/same-room/non-admin). Full -race suite green.
      admin-verbs §3/§5.
- [x] **M19.4e — `purge`.** Completes the §5 baseline verb set. `purge
      <target>` removes a non-player entity — a mob or a room item — from
      the world: `Placement.Remove` + `Store.Untrack`, mirroring the
      canonical death-cleanup path (so a purged mob's spawn slot respawns on
      the next sweep). Resolves players+mobs first (a player match is
      refused — §5/§9 never targets a player), else a room item via the §5
      `room_item` arg path. Audited via auditAdmin. Container/mob-carried
      contents are not recursively cleaned (the death path doesn't either —
      accepted v1 limitation, recorded). 5 tests (mob / room-item / player-
      refusal / not-found / non-admin gate). Full -race suite green.
      admin-verbs §5.
- [x] **M19.4f — Help-visibility through `HasRole`.** Closes the M19.3
      "hidden-from-ALL" cap (ui-rendering-help §9.5): the help `Service`
      gains an injected `RoleResolver` (entityID → tier), wired at the
      composition root to the session `Manager` + `cfg.AdminRole`, so an
      admin sees the admin-tier topics `GenerateHelpTopics` marks `RoleAdmin`
      and a player does not. The help package stays role-agnostic (the
      resolver is the only seam); `entityID==""` (pre-login) still
      short-circuits to `RoleNone` before the resolver. Default (no resolver)
      is unchanged → no caller/test churn. 3 help tests (admin-sees /
      category+list / pre-login-stays-none). Full -race suite green.
- [x] **M19.4g — idle-sweep admin exemption.** session-lifecycle §5.1/§5.2
      step 2: `IdleConfig` gains an `AdminRole` field, wired at the
      composition root from `cfg.AdminRole` (the same `ANOTHERMUD_ADMIN_ROLE`
      the dispatch gate + help visibility use). In `idleDecision` — already
      holding `a.mu` — an actor holding the admin role short-circuits to
      `idleQuiet` before any idle computation, so admins are never warned or
      timed out. The check reuses `HasRole`'s core via a new `hasRoleLocked`
      (the public `HasRole` re-locks `a.mu` and would self-deadlock there).
      Empty `AdminRole` disables the exemption (defensive). 4 tests
      (admin-exempt-warn+timeout / non-admin-still-times-out /
      case-insensitive / empty-role-disables). Full -race suite green.
- [x] **M19.4h — `set property` (room mobs/items).** admin-verbs §4. The
      property half of the `set` surface, scoped to targets that already
      carry a property bag: a room **mob** or **item** (both have
      `SetProperty`). `property.Entry` gains an **`AdminSettable`** flag
      (default false — a property can't be `set` unless its registration
      opts in), and the engine `property.Registry` is threaded root →
      `session.Config` → `command.Env` → `command.Context` (mirrors how
      `Items` flows). `set property <name> <target> <value>` validates the
      name against the registry (exists → admin-settable → value coerces to
      the declared `ValueType`, else nothing is written), refuses the
      reserved keys (`template_id`/`room_id`) and collection types, and
      writes the live bag via the existing `SetProperty`. Target resolution
      gains a room-item fall-through (`findRoomItem`, reusing the
      `roomItems`+`keyword.Resolve` chain) after the player/mob lookup. The
      two engine baseline string properties (`quest_grant`, `key_for`) are
      flagged `AdminSettable` so it's live-exercisable. Audited via
      `auditAdmin`. **Live-only write** — room mobs/items are transient, so
      no save-integration (clean for this slice). 9 tests (item write+audit /
      int coercion / type-mismatch refused / not-admin-settable / unknown /
      reserved-key / player-target refused / usage-panel lists property /
      reuse of vital cases). Full -race suite green.
- [ ] **M19.4i+ — `set property` on players + the `tag` kind.** Still
      **substrate-blocked**: player property needs `connActor.Properties()`/
      `SetProperty` + a property bag on the save + save-integration; the
      `tag` kind needs a runtime tag mutator (mobs have only
      `SetAlignmentTag`; players have no mutable gameplay-tag set, and player
      tags are PARTIAL). admin-verbs §4.

**Touches specs:** `roles-and-permissions`, `admin-verbs`,
`session-lifecycle §5`, `ui-rendering-help §9.5`, `commands-and-dispatch`.

(M19.4i+ — `set property` on players + the `set tag` kind — is left as an
open, substrate-blocked tail; see `m19-4h-deferred-fixes`.)

---

### M20 — Item Decorations (Rarity & Essence)

**Slice:** the warmup theme before Loot. Implements the specced contract
`item-decorations` — two parallel item-marker systems: an **ordered rarity
tier ladder** (decorated, colored, padded for column alignment) and a flat
**essence** glyph that participates in stack identity. Both are
content-registered, theme-colored presentation markers attached to items
via reserved properties. Render-and-attach only — no rarity *generation*
(that's a Loot concern, item-decorations §8). Chosen as a warmup because
it's Small, and it feeds the next theme: Loot's rare-bonus pools and the
deferred rarity-filter autoloot key on the rarity ladder this builds.

**Live list:** `BACKLOG.md` §1 (Rarity tiers, Essence).

**Sub-milestones:**

- [x] **M20.1 — Rarity registry.** item-decorations §2. An ordered tier
      registry: each tier carries key / order / display text / left+right
      decorators / color / visible flag. Sort by order; case-insensitive
      unique keys; idempotent registration (later-wins, pack convention). A
      tier that is invisible, or lacks display text or decorators, is a
      "renders-as-nothing" baseline (e.g. `common` carries order+color for
      logic without cluttering display).
- [x] **M20.2 — Essence registry.** item-decorations §3. A flat set: each
      essence carries key / glyph / color. Case-insensitive unique keys;
      idempotent later-wins. No order, no decorators; at most one essence
      per item (multi-essence deferred, §8).
- [x] **M20.3 — Themed rendering.** item-decorations §4. Rarity inline
      (decorator-wrapped display text in the tier's themed color) and padded
      (centered to the registry's max visible tag width so list columns
      align; unset → blank padding of that width); essence as a themed
      glyph-in-parens. Colors resolve through the theme registry via
      semantic tags (`item.<key>` / `essence.<key>`); registering a visible
      marker registers its theme entry. Plain mode strips to visible
      text/glyph (ui-rendering-help §2/§3).
- [x] **M20.4 — Pack content loading.** item-decorations §2/§3 (content).
      `RarityFile`/`EssenceFile` YAML shapes + `rarity:`/`essence:` manifest
      globs + `Registries.Rarity`/`.Essence` (in `NewRegistries`), loaded via
      `pack.Load` (`decodeRarity`/`decodeEssence`) mirroring the theme path.
      Marker **keys are validated at the load boundary** (new
      `decoration.ValidateKey` + `ErrInvalidKey`: rejects empty, `<`/`>`/`{`/`}`,
      and whitespace — closes the M20.3 review LOW; `Register` also self-rejects).
      `RegisterTheme` is **register-if-absent**, so a theme file's explicit
      `item.<key>`/`essence.<key>` wins (item-decorations §4); the composition
      root seeds + Compiles after Load. `core` ships a tier ladder
      (common/uncommon/rare/legendary, colors via the existing theme
      `item.*` entries) + fire/frost essences. Boots clean. 3 pack tests +
      4 decoration tests; full -race suite green.
- [x] **M20.5 — Attachment + rendering + persistence.** item-decorations
      §5/§6 (the buildable half; stacking split to M20.6). Reserved item
      properties (`rarity`/`essence`) hold the marker key, settable on a
      template (copied to the instance bag at spawn → all instances) or an
      instance (one drop, via M19.4h `set property`). The registries thread
      into the command `Context`; item display reads an item's key and
      resolves it through them, splicing the M20.3 markup into the line —
      the **inventory** list renders rarity + essence **trailing inline**
      after the name ("a short sword [RARE] (✦)"; chosen 2026-06-02 over the
      padded-column form, which stays available via `PaddedMarkup`/
      `MaxVisibleWidth` for a future column surface like shop lists).
      An instance value round-trips
      across logout (existing instance-property persistence); an unknown or
      unset key renders nothing, never an error (§6). An item with neither
      renders exactly as today (§1.1).
- [x] **M20.6 — Essence as stack identity** *(closed by M21.1)*.
      item-decorations §5: two same-template items with different essence
      keys must not stack; same/both-unset stack; rarity does not split
      stacks (§5/§9 default). Delivered by the M21.1 stacking service, whose
      stack key is `<templateId>|<essence>|<extra…>` — essence is part of
      the key, so different-essence items land in separate stacks
      (`TestStack_DifferentEssenceSplits`) and rarity is carried but does not
      split (`TestStack_RarityDoesNotSplit`).

**Touches specs:** `item-decorations` §2–§6, `ui-rendering-help §2/§3`
(theme + plain-strip), `inventory-equipment-items §2/§5` (item properties +
stacking), `persistence` (instance-property round-trip).

---

### M21 — Item Stacking

**Slice:** the read-only contents-grouping the inventory listing wants
("3 healing potions" instead of three lines) — and the thing that unblocks
**M20.6** (essence as stack identity). Implements `inventory-equipment-items
§5`. Stacking is **presentation-only**: items are never merged — each keeps
its id, contents position, and per-instance state; the service just groups
them into stack entries for a UI to render. No inventory-model change, no
count field, no merge/split, no persistence change.

**Live list:** `BACKLOG.md` — folds in the M20.6 stack-identity tail.

**Sub-milestones:**

- [x] **M21.1 — The stacking service.** inventory-equipment-items §5.1/§5.2.
      A new `internal/stacking` package: `StackEntry` (template id, display
      name, quantity, rarity key, essence key, contained ids in stack order)
      + a `Service` with `AddKey(propertyName)` (the §5.1 extensibility hook)
      and `Stack(items) []StackEntry`. Stack key: template-less items are
      singletons keyed `notemplate:<id>`; templated items key on
      `<templateId>|<essence>|<extra…>` (empty string for an absent
      property), extras in registration order. First-seen position fixes a
      stack's place. Read-only — iterating stacks mutates nothing.
      **Closes M20.6** — essence is part of the stack key, so two
      same-template items with different essence keys land in separate
      stacks. Leaf package: `StackEntry` carries key strings; the display
      layer formats them via the M20.3 decoration registries.
- [x] **M21.2 — Inventory display uses stacks.** `InventoryHandler` groups
      the actor's items through the threaded `stacking.Service` and renders
      one line per stack as `decoratedName(first) (xN)` (trailing `(xN)`
      suffix, chosen 2026-06-02), reusing the M20.5 decoration rendering
      unchanged. Singletons carry no count and still expand container
      contents; a qty>1 stack shows only the count (instances differ). A nil
      service degrades to one line per item (tests unchanged). Service built
      at the composition root + threaded root → `session.Config` →
      `command.Env` → `command.Context`. 2 display tests (stacks-to-(x3),
      singleton-no-count); full -race suite green. (Room/container listings +
      quantity-aware `get`/`drop N` — keyword §6 — remain follow-ons.)

**Touches specs:** `inventory-equipment-items §5` (stacking),
`item-decorations §5` (closes the M20.6 stack-identity tail).

---

### M22 — Loot and Corpses

**Slice:** turn a mob death into lootable drops. Two specced-but-uncoded
halves join here: (a) loot **generation** at spawn (`mobs-ai-spawning §6.3`)
— a loot table rolled into the mob's contents the moment it appears; and
(b) **death → drop** (`loot-and-corpses`) — a corpse-as-container created on
the `mob.killed` event, coins rolled into it, a killer-first ownership
window, the `loot` / `get … from <corpse>` verbs, a per-player autoloot
toggle, and a corpse-decay sweep. Unblocks group loot-sharing (the §4
rights seam) and the autoloot rarity-floor filter (item-decorations).

**Specs:** `loot-and-corpses` (corpse half) + `mobs-ai-spawning §6.3`
(generation half). **Live list:** `BACKLOG.md` §1 "Loot drops + corpses
+ autoloot".

**Sub-milestones:**

- [x] **M22.1 — Loot table + generation at spawn.** `mobs-ai-spawning §6.3`.
      New `internal/loot` leaf package: `Table` (guaranteed pool, weighted
      pool with `PoolRolls`, optional `RareBonus`), a `Roller` interface
      (mirrors combat/progression), `RollItems(table, roller) []string`
      (guaranteed first → N weighted rolls → one rare-bonus roll, each
      independent), and an id-keyed `Registry` with priority override.
      `mob.Template` gains a `loot_table` id; `internal/pack` decodes
      `loot_tables/*.yaml` into the registry and validates the mob's
      reference. `bootSpawner.spawnMob` (the single boot+respawn chokepoint)
      rolls the table after class growth and before placement, instantiates
      each item via `Store.Spawn`, files it under the mob's id in
      `entities.Contents`, and publishes `mob.loot.generated` (mob id, room,
      template, count). The coin block (`loot-and-corpses §3`) — both the
      `Table` field and its decode — lands with M22.2, where it is rolled at
      corpse creation (no declared-but-unused field in M22.1).
- [x] **M22.2 — Corpse creation on death + coins.** `loot-and-corpses §2–§3`.
      On `mob.killed`, publish a cancellable `corpse.creating`; unless
      cancelled, mint a corpse container entity in the room, **move** the
      mob's contents into it (instance identity preserved), roll the loot
      table's coin block into it as a coin pile, record killer id + creation
      tick + owner set (today just the killer), and emit `corpse.created`.
      No corpse when the mob carried nothing and rolled no coins. Corpse is
      read-only via the existing look-in/container path; refuses `put`; the
      corpse item itself is no-get.
- [x] **M22.3a — Looting rights + the `loot` verb.** `loot-and-corpses §4`
      + §5.1. The ownership-window rights check (`corpse.MayLoot`: owner-set
      member during the window; open after; empty killer / zero window ⇒
      open immediately; non-owner refusal does not name the owner) and the
      `loot [<corpse>]` verb — takes every fitting item (no carry cap yet,
      so all) plus all coins (credited to currency, not inventory), removes
      the corpse + emits `corpse.looted` when emptied, no-arg picks the
      most-recent lootable corpse. Ownership window is a config knob
      (`ANOTHERMUD_CORPSE_OWNERSHIP_WINDOW`, default 60s) measured in ticks
      against the corpse's creation tick.
- [x] **M22.3b — `get … from <corpse>` + look-in display.** `loot-and-corpses §5.2`
      + §2.2. Built the general container-access verbs the engine lacked.
      **b₁:** `GetHandler` hand-parses on `from` (the room form still
      resolves via `resolveRoomItem` so ordinals/messages are unchanged);
      `get <item> from <container>` resolves the container inventory-first
      then room and keyword-matches the item within its contents; corpses
      enforce the §4 window (refusal unnamed); `get coins from <corpse>`
      claims the coin pile to currency; a corpse drained empty is removed +
      emits `corpse.looted` (single-winner); plain containers get no gate
      and aren't auto-removed. New `container.item_removing` (cancellable) +
      `container.item_removed` events mirror put. **b₂:** `look [in|at]
      <target>` lists a container/corpse's contents + coin amount (gated by
      presence only, not §4 — anyone present may look), or shows a plain
      item's name; bare `look` still renders the room.
- [x] **M22.4 — Autoloot.** `loot-and-corpses §6`. Per-character autoloot
      preference (`player.Save.Autoloot`, off by default, no schema bump —
      omitempty bool like wimpy) + `autoloot [on|off]` verb (reports/toggles
      via a lock-free `connActor.autoloot` atomic). A `corpse.created`
      subscriber auto-loots when the killer is an online player with
      autoloot on present in the corpse's room, reusing the shared
      `command.TransferCorpse` primitive (rights trivially satisfied — own
      kill). Scoped to the killer's own kills, loots everything (§10 leaves
      coins-only / rarity-floor scopes as future refinements).
- [x] **M22.5 — Corpse decay.** `loot-and-corpses §7`. The `corpse-decay`
      tick sweep (`Service.DecaySweep`, reserved in `time-and-clock §3`):
      each corpse past its creation-tick + lifetime is removed with all
      remaining contents (destroyed, not spilled), emitting `corpse.decayed`.
      `Placement.Remove` single-winner guard makes it race-safe against a
      concurrent `loot`; `Contents.Take` leaves a just-looted item in the
      looter's hands. Lifetime (`ANOTHERMUD_CORPSE_LIFETIME`, default 5m) +
      sweep cadence (`ANOTHERMUD_CORPSE_DECAY_INTERVAL`, default 3s) are
      config knobs → ticks. Closes the unbounded-corpse-growth debt;
      corpses are not persisted (a restart removes them too).

**Touches specs:** `loot-and-corpses` (substantially),
`mobs-ai-spawning §6.3` (loot generation), `combat §6.3` (the `mob.killed`
event consumed), `inventory-equipment-items §2.5/§4` (corpse-as-container),
`economy-survival §2.1` (coin credit), `time-and-clock §3` (decay handler).

---

### M23 — Room Coordinates (+ builder room-data view)

**Slice:** the area-local coordinate substrate a client mapper and a future
telnet `map` verb need — **derived from the exit graph at load, not
authored** — plus a small builder-visibility QoL: an admin overlay on `look`
that exposes the room's metadata (now including those coordinates). Implements
`room-coordinates` (the whole substrate) and extends the look render with an
admin-only block. **Unblocks player-maps** (`BACKLOG.md` §2): this stops at
the coordinate substrate + GMCP exposure and leaves rendering to that slice.

**Specs:** `room-coordinates` (substrate). **Live list:** `BACKLOG.md` §1
"Room coordinates" → shipped; player-maps remains open on top of it.

**Sub-milestones:**

- [x] **M23.1 — Coordinate derivation substrate.** `room-coordinates §2–§5,§8`.
      New `internal/world/coords.go`: `Coord{x,y,z}`, the §2.3 direction-delta
      table, and `(*World).DeriveCoordinates` — a deterministic per-area BFS
      (seeds-first / ascending-id / canonical-direction order, PD-6) walking
      intra-area directional exits from a seed, with an authored per-room
      `coord:` **pin** (§3.5) that seeds the walk and is never overwritten.
      Conflicts (collision, non-square re-reach, unplaced room,
      two-pins-one-cell, malformed pin) emit typed load-time warnings and
      never abort (PD-4). `Room.Pin` (authored) + `Room.Coord` (derived;
      **nil = unplaced**, never (0,0,0) — the origin is a valid placed value).
      Pin decode + warn-and-fallback in `internal/pack` (`decodeRoom`);
      derivation runs after `validateExits` in `Load` and logs via slog. GMCP
      `Room.Info` gains optional `x`/`y`/`z` (`*int`, omitted when unplaced) —
      a flat **placeholder** pending validation against a live Mudlet mapper
      (§5 wire-shape caveat). No persistence, no migration (§8). The walk is
      factored into an `areaDeriver` (seed → walk → unplaced). All
      §2.4/§3.4/§3.6/§4.5/§7.1 acceptance checklists covered; 100% coverage on
      `coords.go`. The derived `Coord` lives on `world.Room` (not a separate
      package) to avoid an import cycle with the field.
- [x] **M23.2 — Admin/builder `roomdata` look view.** Extends the room render
      (`ui-rendering-help`). A `roomdata` admin verb toggles a **persisted**
      per-character preference (`player.Save.ShowRoomData`, no schema bump —
      omitempty bool like Autoloot) to append a builder metadata block to the
      room view: room/area ids, the area-local coordinate + source, terrain +
      exposure flags, tags, properties, healing rate, and per-exit targets with
      door state. **Double-gated** — renders only when the viewer holds the
      admin role **and** has the toggle on (role is the gate, toggle is the
      preference). The shared `command.AppendRoomData` is the single seam every
      "you see the room" render routes through — `look`, movement, recall,
      teleport, flee, the login spawn, and link-dead reattach — so the block
      shows on **room entry**, not only an explicit `look`. A distinct
      **builder** role (separate from `admin`) is deferred to OLC, where it is
      first needed (`BACKLOG.md` OLC pre-decision 0).

**Touches specs:** `room-coordinates` (substantially),
`networking-protocols §7` (the Room.Info coordinate fields),
`world-rooms-movement §3.2` (the exit graph the walk projects),
`ui-rendering-help` (the `roomdata` look overlay),
`roles-and-permissions` (the admin gate the view reuses).

---

### M24 — Player Maps (active)

**Slice:** turn the M23 coordinate substrate into something players see —
an **active, toggleable minimap** (a small local window auto-appended to
the room view) and a **`map` verb** (the full discovered map of the current
area), both behind a **persisted fog of war** (only rooms the character has
entered are drawn), plus the **Mudlet GMCP** map surface. One shared
local-window query feeds every surface. Implements the new
`player-maps` spec (born from `docs/proposals/player-maps.md`).

**Spec:** `player-maps` (whole feature). **Live list:** `BACKLOG.md` §2
"Player maps" — unblocked by M23.

**Sub-milestones:**

- [x] **M24.1 — Fog-of-war visited-set substrate.** `player-maps §3,§8`.
      The persistence + exploration tracking, the proposal's recommended
      first slice (the only piece that touches the save). `player.Save`
      gains `VisitedRooms []string`; `CurrentVersion` 15→16 with an
      append-only no-op `migrateV15toV16` (a legacy save migrates to an
      empty set — a returning character re-explores; fog of war is not
      back-filled). `connActor` carries an O(1) in-memory visited index
      (`internal/session/visited.go`) lazily seeded from the save; the
      exploration hook rides **`SetRoom`** — the single room-change
      chokepoint — so **every** arrival (move, recall, teleport, login
      spawn, link-dead reattach) marks the destination visited (PD-5),
      deduped, dirtying the save for the normal autosave path.
      `HasVisited` / `VisitedRooms` accessors are the fog gate the later
      renderers read. Hooked at `SetRoom` rather than the `player.moved`
      event because that event isn't emitted uniformly (recall skips it),
      so the chokepoint is what makes "entering is entering" hold.
- [x] **M24.2 — The shared local-window query.** `player-maps §2`.
      `internal/world/window.go`: `(*World).LocalWindow(origin, radius)`
      → `Window{Origin, Area, Rooms []WindowRoom}` (+ `Contains` and
      `OriginCoord` helpers). A bounded BFS from the origin over
      **intra-area directional** exits, collecting **placed** rooms with
      their stable coordinates (copied, never aliasing `Room.Coord`).
      `radius` is a STEP bound (depth); a negative radius is unbounded =
      the whole area (the `map` form). Crosses doored exits normally,
      stops at the area boundary, never traverses keyword/portal exits,
      and surfaces a placed neighbour even from an unplaced origin (which
      `OriginCoord` then reports as un-centerable). Applies **no** fog —
      callers (the renderers, the GMCP path) filter against the M24.1
      visited set. Net-new (the coords walk in `world` is boot-only); the
      seam both ASCII renderers and the area query share. ~97% cov.
- [x] **M24.3 — ASCII renderers: minimap toggle + `map` verb.**
      `player-maps §4–§6`. Net-new grid renderer (`internal/command/
      minimap.go` + `mapcanvas.go`): a sparse char canvas, render-time
      recenter (viewer at `@`), terrain→glyph mapping, connectors between
      drawn rooms, **stub connectors** for cardinal exits to non-rendered
      (unvisited / off-window / cross-area) rooms (§6.4), single z-plane
      with up/down + keyword-exit annotations (§6.5, PD-7/PD-8). The
      active **minimap** renders **beside** the room view (to its right,
      not below — `internal/command/sidebyside.go`: markup-aware wrap +
      column join with a `{x}` boundary reset so room color can't bleed
      into the map) and is **enclosed in a `<frame>` border** bounding the
      fog-of-war window, via the shared `AppendMinimap` seam — wired into
      `renderRoomWithData` (look, movement, recall, teleport, flee) and
      the session spawn + reattach renders — behind a persisted,
      non-admin `minimap [on|off]` toggle (`player.Save.MinimapEnabled`,
      omitempty, no bump). The **`map`** verb renders the full discovered
      current area on demand (unbounded `LocalWindow`) with a **legend**
      (viewer marker / connectors / terrain glyphs / stub convention),
      reporting cleanly from an unplaced room. Both fog-filtered against
      the M24.1 visited set (`MapViewer.HasVisited`). Renderer/canvas/
      sidebyside/verbs covered; 51 pkgs green -race.
- [x] **M24.4 — Mudlet GMCP surface (verify + lock).** `player-maps §7`.
      Mostly confirmation: the `Room.Info` coordinate fields already ship
      (M23.1), and emission is **current-room-only** — the three
      `sendGmcpRoomInfo` senders (SetRoom, login spawn, reattach) each
      send a single room; no bulk/multi-room sender exists, so the server
      never bulk-reveals an unexplored area and Mudlet fills in as the
      player walks (fog of war stays honest with no second persistence
      layer). Added tests locking the placeholder wire shape:
      `TestBuildRoomInfoPayload_CarriesCoordinatesWhenPlaced` (fresh ints,
      no aliasing) and `_OmitsCoordinatesWhenUnplaced` (no `x/y/z` keys at
      all, not `x:0`). **Deferred (human-in-the-loop):** validating the
      flat `x/y/z` layout against a **live Mudlet mapper** before shipping
      graphical-mapper support (PD-9) — recorded in the
      `room-coordinates-gmcp-wireshape` memory; the shape may need
      reshaping to Mudlet's `coords`/area convention.

**Touches specs:** `player-maps` (substantially),
`room-coordinates` (the coordinate source it reads),
`persistence §3` (the v16 visited-set save field + migration),
`world-rooms-movement §3` (the exit graph the window walks),
`ui-rendering-help` (the minimap/`map` rendering + the toggle),
`networking-protocols §7` (the Mudlet GMCP surface).

---

### M25 — Equipment slots: eligibility, footprint, contention, mob capacity

**Slice:** close the four `inventory-equipment-items §3` slot gaps the spec
left open. Items now declare which slots they are eligible for and which
extra slots they span; equip enforces eligibility, computes a footprint,
auto-swaps every conflicting occupant, and exposes a cancellable veto seam;
mobs equip through the same rules so their stat budgets stay bounded.
Driven by the spec gap analysis (not the BACKLOG arc); the spec's §3 was
rewritten first, then implemented phase by phase.

**Spec:** `inventory-equipment-items §2.2, §3.3–§3.8, §7, §8, §9`
(rewritten ahead of code). **Decisions (spec §9):** ambiguous-slot →
**ask-which**; structural auto-swap → **respect no-remove, fail the equip**.

**Sub-milestones:**

- [x] **M25.1 — Data model + slot substrate.** `§2.2, §3.3`. `item.Template`
      + `ItemFile` gain `EligibleSlots` / `CompanionSlots`; `decodeItem`
      decodes them with a legacy `properties.slot` → one-element bridge
      (`item.LegacySlotName`, shared with the instance builder) so existing
      content needs no edits. Boot post-pass `validateItemSlots`
      (`ErrItemUnknownSlot`) rejects unknown slot names with file
      attribution, mirroring `validateSpawnRules`. `offhand` added to the
      engine slot baseline (the second hand a two-hander spans).
- [x] **M25.2 — Item↔slot eligibility (gap 1).** `§3.4 step 3`. `EquipHandler`
      rejects an item whose eligible set excludes the target slot, with a
      reason distinct from "no such slot" — closing the equip-anything-
      anywhere exploit. Eligible/companion slots are lifted onto
      `entities.ItemInstance` at spawn (R5 — like `WeaponDamage`), so the
      handler and mob path read them with no template-registry wiring.
      `slot.IsEligible` is the shared membership helper.
- [x] **M25.3 — Footprint, contention, cancellable veto (gaps 2 & 3).**
      `§3.4–§3.6`. `connActor` gains a `footprints` index (a spanning item
      appears under several equipment keys but one footprints entry — one
      modifier set, one save entry, whole-footprint unequip). `Actor.Equip`
      takes a footprint slice; `Unequip` frees the whole footprint.
      `slot.FreeKey`/`slot.Footprint` are the shared expansion helpers.
      Equip computes the footprint, displaces **all** conflicting occupants
      (a single equip may evict several), guards no-remove items, then
      publishes the cancellable `eventbus.EntityEquipping` (the policy/
      feature-module seam) before any mutation. Decision A makes the slot
      arg optional (sole-eligible auto-targets, multi-eligible asks which).
      **No save-version bump** — only the target key persists (§3.8);
      respawn re-derives companions from the template.
- [x] **M25.4 — Mob slot capacity (gap 4).** `§3.7`. `EquipMobAtSpawn`
      routes each item through the shared `slot.Footprint`/eligibility
      helpers (no auto-swap — a conflict skips the later item), so a
      template that lists two head items no longer double-stacks their
      modifiers. Non-fitting items are still **carried as loot** but not
      slot-equipped (modifiers skipped), recorded in `EquipResult.Skipped`
      and logged. `nil` registry falls back to legacy apply-all.
- [x] **M25.5 — Content migration + demo.** Starter equippable items
      migrated to explicit `eligible_slots`; a two-handed `iron-greatsword`
      (`companion_slots: [offhand]`) and a `wooden-shield` (`offhand`)
      added to the town square so spanning + contention are demonstrable
      live (`equip greatsword` fills both hands; `equip shield` displaces
      it; `equip greatsword` displaces the shield). 51 pkgs green -race.

**Open / deferred:** the `no_remove` auto-swap guard is built and tested but
the tag name is hardcoded pending externalization to the §8 config surface
(no content uses it yet). Multi-cap companion slots and a spanning robe
(`body`/`legs`) are expressible but unshipped (spec §9 edges).

**Touches specs:** `inventory-equipment-items` (substantially — §3
rewritten), plus the README cancellable-events table (`entity.equipping`).
**Touches code:** `internal/slot` (FreeKey/Footprint/IsEligible),
`internal/item` (template fields + legacy bridge), `internal/pack` (decode +
validate), `internal/entities` (instance lift + mob equip), `internal/command`
(equip/unequip handlers + Actor interface), `internal/session` (footprint
representation + persistence), `internal/eventbus` (the cancellable event).

---

### M26 — Engine Debt II

**Slice:** a tight debt-paydown pass closing three genuine specced gaps
that accreted across earlier milestones. Scoped to *real wins* — the
substrate-without-consumer items (§6.2 scaling-bonus consumer,
property-registry save-pipeline, §3.4 tag-indexed movement reads) were
deliberately left deferred because their triggers haven't fired (YAGNI).
The fourth candidate, death-driven purge (`mobs-ai-spawning §3.5`), was
found **already done** (M7.5 wired `mob.killed → Untrack`); its stale
BACKLOG line was deleted as drift.

**Exit criteria:**

- [x] **M26.1 — Cross-pack door-key boot validation.** `world-rooms-movement
      §5.3`. A keyed door's `KeyID` is an item template id, qualified at
      decode but never checked — a typo'd key produced a permanently-
      unlockable door, fail-silent until unlock. `validateDoorKeys` (loader
      post-pass, `ErrMissingDoorKey`) now fails the load, mirroring
      `validateItemSlots`/`validateSpawnRules`. Runs after all packs load so
      cross-pack keys resolve.
- [x] **M26.2 — Passive gain stat factor.** `abilities-and-effects §3.5`
      step 3 (closes m9-5 #1). Passive proficiency gain hardcoded
      `statFactor=1.0` because a passive fires off a bare entity id with no
      stat-by-id seam. New `progression.StatReader` (nil-safe) threads into
      `PassiveResolver.rollGain`; `session.PassiveStatReader` implements it
      with the player-then-mob fallback shared with `EffectTargetResolver`.
      Effect: `parry`/`second-attack` (`gain_stat: dex`) train faster for
      high-dex characters. Mob gain stays a no-op.
- [x] **M26.3 — GMCP wizard panel.** `character-creation §5` (closes m12-3).
      `runCreation` discarded the wizard's structured `StepEvent` (nil sink)
      because M12.3 predated a structured-data channel; GMCP landed in M16.
      New `Char.Wizard` package + `CharWizardStep` payload + `wizardGmcpSink`
      bridge each rendered step to a GMCP frame for a rich client's in-place
      creation panel. Plain/not-yet-negotiated clients get a nil sink and the
      unchanged text path, so creation works on every client.

**Open / deferred (triggers unfired):** §6.2 scaling-bonus consumer (needs a
scaling passive in content), property-registry save-pipeline (no save grows a
content property yet), §3.4 tag-indexed movement reads (marginal at current
world scale). Recorded in `[[m26-deferred-fixes]]`.

**Touches code:** `internal/pack` (door-key validator), `internal/progression`
(StatReader + passive rollGain), `internal/session` (PassiveStatReader,
wizardGmcpSink, runCreation), `internal/gmcp` (Char.Wizard payload),
`cmd/anothermud` (passiveStatReader wiring). 53 pkgs green -race.

---

### M27 — Crafting & Cooking (MVP)

**Theme:** the full `crafting-and-cooking.md` MVP (plan
`themes/crafting-cooking-plan.md` Phases 0–5), built per-slice. Almost all
of it is wiring to existing systems — rarity tiers ARE the quality tiers,
crafting skill IS a proficiency, the well-fed buff IS an EffectTemplate via
the consumable pipeline, cooking feeds the existing sustenance pool. Two
new pieces: the recipe registry and the campfire temp-entity (reusing the
M15.2 decay shape). Reviewed twice (foundation + final); the atomic
consume/produce was independently verified free of item dup/loss.

**Exit criteria:**

- [x] **Phase 0 — Recipe substrate.** `internal/recipe` (Recipe +
      namespaced Registry); pack `RecipeFile`/`decodeRecipe` + `recipes:`
      glob; player save **v17** `known_recipes` + migrateV16toV17.
- [x] **Phase 1 — Crafting skills.** `recipe.KnownManager` (per-character
      known set, session-persisted, §9 drop-unknown-on-restore); `learn
      <discipline>` verb (trainer-gated acquire → seed proficiency + grant
      baseline recipes). A discipline = any ability a recipe references.
      Decision: acquisition = learn-at-trainer; one mob can be trainer +
      shopkeeper (Brandr the blacksmith).
- [x] **Phase 2 — Tier-0 craft + quality roll.** `internal/crafting`
      Service: `craft <recipe>` verb; the §5 weighted roll (skill + tool +
      ingredient → RNG band → station hard ceiling + ingredient soft
      ceiling → rarity-tier instance property); atomic input
      consume/produce with rollback (no item loss).
      `ProficiencyManager.RollUseGain` shares the §3.5 gain path.
- [x] **Phase 3 — Cooking + well-fed.** Craft stamps a quality-scaled
      `effect_id` on output (`quality_effects` map); the existing eat
      pipeline applies it (no-stack by id). Marta the cook; cooked-meal;
      well-fed-minor/well-fed effects. Common = cold ration (no buff).
- [x] **Phase 4 — Stations + gate + portable tools.** Present station tier
      (room `craft_stations` ∪ carried tool) gates the attempt + sets the
      ceiling. Forge = Tier-2 smithing, market = Tier-2 cooking; cook's kit
      = Tier-1 field tool. Registered `craft_stations` room property
      (TypeMapInt; fixed the validator to accept the YAML map shape).
- [x] **Phase 5 — Buildable campfire.** `internal/campfire` temp Tier-1
      cooking station (M15.2 decay reuse): `build campfire` (terrain +
      weather + fuel gates), decay tick, room-placed station read by the
      craft path. Firewood fuel.

**The MVP loop:** learn a discipline at a trainer → buy ingredients (vendor
stopgap; gathering replaces it later) → craft Tier 0 anywhere → build a
campfire for Tier-1 field cooking → use the forge/kitchen for Tier 2 →
meals grant well-fed, quality renders via rarity, skill grows through use.

**Post-MVP follow-ups (shipped after the Phase 0–5 MVP):**

- [x] **Cleanup pass (C/B1/B2).** `learn` now requires a registered
      discipline ability — refuses instead of seeding a default-cap-100
      proficiency that would inflate the §5 ceiling (`d626bef`). **B1 — tool
      quality (§5):** the best inventory tool matching a recipe's `tool` tag
      now weights the roll by its rarity, separate from skill; tools read,
      never consumed (`de57e5f`; iron-hammer content). **B2 — gain-stat on
      craft skill-up (§3.5):** the crafting service shares the M26
      `StatReader` so dex/wis scale craft proficiency gain (`6ab13b1`).
- [x] **Multi-trainer resolution fix (`715eee6`).** Surfaced by in-game
      verification: with two trainers in one room (the forge holds both
      Maerys the combat master and Brandr the blacksmith), `learn`/`practice`
      consulted only the first trainer found, so `learn smithing` failed
      with "no one here can teach you." `TrainerSource.TrainerInRoom` now
      takes the target ability and **prefers the trainer who can teach it**
      (pure `selectTrainer` + regression test); threaded through
      `TryPractice` + the learn verb. A pre-existing first-match limitation
      the M27 two-trainer forge exposed; also fixes `practice` in any
      multi-trainer room.
- **In-game verified** via a live telnet playthrough: creation → learn at
      trainer → buy → station-gated craft (quality roll + atomic
      consume/produce, no dup/loss) → cook → eat; tool quality lifted a
      crafted sword to `[UNCOMMON]` live.

**Open / deferred (post-MVP, plan Phases 6–8):** recipe-acquisition breadth
(common/uncommon/rare/regional via shops/quests/loot), regional sets +
guided discovery, and **gathering** (the real ingredient source replacing
vendor stock). **B3 — craft time** (`time_pulses` declared but unused; needs
a tick-based delay slice) deferred. Remaining LOW review items in
`[[crafting-deferred-fixes]]`.

**Touches code:** `internal/recipe` + `internal/crafting` + `internal/campfire`
(new), `internal/command` (learn/craft/build verbs + station tier),
`internal/progression` (RollUseGain), `internal/pack` (recipe loader +
craft_stations property + TypeMapInt fix), `internal/player` (save v17),
`internal/session` + `cmd/anothermud` (wiring), `content/core` (cooking +
smithing disciplines, trainers, recipes, food, effects, tools, stations).
54 pkgs green -race.

---

### WoT Mechanics EPIC (post-M27, additive sub-epics)

**Theme:** the mechanical-fidelity program for the Wheel-of-Time setting,
decomposed in [`themes/wot-mechanics-epic.md`](themes/wot-mechanics-epic.md)
(the canonical per-sub-epic tracker). **Decision 0 = posture A** (2026-06-10):
translate WoT's flavor + meaningful choices onto the engine's tick/chance
model; **no d20 substrate rewrite** (S12 shelved). Every sub-epic is therefore
an *additive* feature on the working engine, shipped as its own spec slice +
content. Unlike M0–M27, these are tracked as EPIC sub-epics rather than
numbered milestones — this section is the ROADMAP-side index; the EPIC doc
holds the detail and the open candidates.

**Shipped slices (all 2026-06-10 → 2026-06-11, each reviewed):**

- [x] **S1 — Weapon identity (A+B+C).** Category + proficiency-tier + damage-type
      metadata, class-granted weapon proficiency + non-proficient penalty,
      per-weapon crit threat/multiplier. Spec `weapon-identity.md`. (Remaining
      S1: ranged (G), armor (E), size-wield (F) — separate themes.)
- [x] **S6 — Saves (Fort/Reflex/Will).** Three derived saves (class strong/weak
      base + ability mod), the `combat.ResolveSave` d20 primitive + `SaveResolved`
      event, massive-damage Fortitude consumer, score row. Spec `saves.md`.
- [x] **S5 — Conditions (Core 5).** Stunned/prone/blinded/frightened/fatigued as
      flagged effects + combat hooks, entry + per-tick shake-off saves (consumes
      S6), `afflict`/`cure` + save-gated `trip`/`bash`, `affects` listing. Spec
      `conditions.md`.
- [x] **S3 — Skills (substrate).** Skill = use-based proficiency (the convention
      crafting proved) + the `ResolveSkillCheck` primitive (`d20 + bonus vs DC`)
      + lockpicking (`pick`) as the first consumer + a `skills` listing. Spec
      `skills.md`. The primitive is the seam backgrounds (S9) and visibility call.
- [x] **Multiclass seam (S9 prep).** Player class widened `string → []string`;
      **player save v18** (append-only migration). The engine was already
      multi-track, so multiclass became ~80% content + this one scalar→list change.
- [x] **S9 — Backgrounds (creation-origin half).** The third creation axis after
      race + class: a content-defined background grants a one-time starting
      package — skill proficiencies + items + gold — applied once at creation,
      never re-applied. `progression.BackgroundRegistry` (mirrors race/class),
      `session.BackgroundGranter` (fired from `character.created`), pack
      `BackgroundFile`/`decodeBackground` + `backgrounds:` glob (item ids
      namespace-qualified at decode), a creation-wizard background step, the
      `score` row, and **player save v19** (append-only; pre-migration saves load
      background-less). Content: a core `Commoner` (skill+gold-only, engine
      baseline) + four starter-world demo backgrounds (soldier/thief/smith/
      wanderer) whose item grants keep the core→world dependency direction
      intact. Spec `backgrounds.md`. go-reviewer APPROVE; M1/M3/M4/L4 fixed
      in-pass, one MEDIUM deferred (`[[backgrounds-deferred-fixes]]` — wizard
      doesn't yet call `GetEligible`, the same accepted gap as `classOptions`).
- [x] **S4 — Feats (the player-choice perk engine).** The full feat selection
      engine over 8 per-slice commits, each go-reviewed: `internal/feat` (registry
      + prereq evaluator + grant/bonus aggregator), banked feat-credit slots
      (1 at creation + 1 per 3 character levels, reusing the trains-crediter
      pattern), the `feat <name> [target]` / `feats` verbs, all **six grant kinds**
      — save_bonus (derived saves), max_hp (via `srckey.Feat` → the hp_max →
      vitals pipeline, no compounding), hit_bonus + crit_threat (per weapon
      category, folded into `connActor.Stats()` lock-free), skill_bonus (at the
      check site), and ability (teaches an ability, idempotently) — plus authored
      background/class feat grants (closing S9's deferred item). **Player save
      v20** (append-only; `feat_credits` + `known_feats`). Content: the v1
      static-bonus family (Great Fortitude / Iron Will / Lightning Reflexes /
      Toughness / Weapon Focus / Improved Critical / Skill Emphasis / Power
      Attack) in `tapestry-core`. Spec/scope `proposals/wot-feats.md`. Every
      phase live-verified; the action-economy/ranged/armor/channeling/reputation/
      lost-ability feats are deferred to their owning sub-epics. Open polish:
      creation-wizard feat-pick step, the Power Attack ability's combat effect,
      choose-a-feat-from-a-pool for backgrounds.
- [x] **S2 — The One Power (Phases 0–4).** The marquee channeling arc, built on
      the generalized-pool substrate: a channeler class with a One Power pool, the
      classic-four weaves + the `channel` verb, the `overchannel` → Fortitude-save
      fatigued/stunned/stilled cascade, Five-Powers affinities + gender-derived
      saidin/saidar potency, the cast-time **interrupt game** (warmup + hit / move /
      stun interrupts), and the content-only **Initiate/Wilder split**. **Player
      saves v21** (pool currents) **+ v22** (gender). Phase-4+ depth — taint/madness,
      angreal, linking, stilling-restore, the Wilder Block — remains open. Scope
      `proposals/wot-the-one-power.md`.
- [x] **S1.H — Masterwork (item quality grades).** Grade-scaled bonuses
      (masterwork / power-wrought) delivered through existing seams — weapon to-hit,
      power-wrought damage, armor check-penalty, tool skill-check — plus the
      power-wrought unbreakable flag and boot-time grade-key validation;
      `internal/grade`, spec `masterwork.md`. Mechanically independent of the
      cosmetic rarity/essence decoration.

**Open / next candidates:** S2 Phase 4+ depth (taint/madness, angreal, linking),
S7 survival v2, S8 reputation, S10 travel/planes, S11 Shadowspawn, plus the
separate ranged (G) / armor (E) / size-wield (F) S1 follow-ons, and the
S4 polish items above. See `BACKLOG.md` §2 + the EPIC doc for sequencing.

---

### M28 — Visibility (Gameplay Systems keystone) — COMPLETE

The per-observer "can X see Y?" model (`docs/specs/visibility.md`), the
keystone that unblocked hidden exits, `who` per-viewer hiding, and admin
wizinvis. Built as a multi-slice arc (S1–S6); each slice its own commit +
review. All six slices shipped; the only open items are the recorded S6b
refinements (secret-door op gating, flee gate, GMCP exit filter, passive-find,
active entity-search) — none block the headline behavior.

- [x] **S1 — the filter primitive** (`internal/visibility`): `CanSee` /
      `Visible[T]`, layer AND-composition (§2.2), self-always-visible + bypass
      (§2.1), per-source pierce dispatch (flag-gated darkness/invis/admin vs
      roll-gated detect→sticky→contest). Decoupled from the engine via small
      Observer/Target interfaces; unknown source fails open (§1.2). 100% cov,
      behavior-neutral (no sources wired yet).
- [x] **S2 — command-resolver seam + darkness targeting gate** — `ResolveContext.CanSee`
      predicate (nil = permissive/legacy), honored by the room-scoped entity/item
      resolvers (entity/player/npc/room_item/container/visible/findable) unless the
      arg sets `BypassVisibility`. `BuildResolveContext` populates it via the
      `visibility` filter: in a pitch-black room (effective light Black) a
      non-luminous occupant can't be targeted (`kill`/`get`), a luminous item
      (a dropped lit torch) still can. Render already gates occupants by light
      tier, and `who`/GMCP are invis concerns — so those consumers get wired when
      their source lands (S3–S5) rather than a behavior-neutral wire-everything pass.
- [x] **S3a — hide mechanic foundation** — ephemeral concealment state on
      `connActor` (hidden + score + instance, never persisted), the `hide`/`unhide`
      verbs, the `concealment.before` (cancellable) + `entity.concealed` +
      `entity.revealed` events, and move-drops-hide (a `player.moved` subscriber
      reveals a hidden mover). Hide score is `10 + DEX mod` for v1. Unit-tested;
      not yet observable to others — the filter wiring is S3b.
- [x] **S3b — light up hide in the filter** — the observer side on `connActor`
      (`PerceptionBonus` = WIS mod, sticky detection set cleared on room change,
      the contest via `ResolveSkillCheck`); the real `visObserver`/`visTarget`
      adapters (Contest + sticky AlreadyPierced); the visibility predicate now
      composes darkness AND hide, so a hidden player can't be `kill`ed/`look at`
      unless the viewer wins the contest (reuses the S2 resolver seam); and the
      **room render** occupant list filters through the SAME predicate, so a
      hidden player vanishes from `look`. Optional `hideable`/`perceiver`
      capability interfaces keep the Actor interface unchanged. Unit-tested
      (contest win/lose/sticky/no-perception) + live verb smoke; the 2-player
      "can't see" outcome rides the contest RNG so it's unit-covered, not live.
- [x] **S3c — reveal-on-action** — `Command.BreaksConcealment` registry flag +
      a dispatch hook (`breakConcealmentOnAction`) that drops a hidden actor's
      concealment and emits `entity.revealed(acted)` before the handler runs
      (§4.5). Flagged the loud verbs: kill, cast/channel, get/take, drop, give,
      open/close. Fires after a successful typed-arg resolution (a failed target
      doesn't reveal); hand-parsed loud verbs reveal on attempt. Magical/admin
      invis (S5) is exempt. Unit-tested + live (hide → loud action → revealed).
      Closes the "hide is permanent" gap — hide (S3) is now complete.
- [x] **S4 — sneak** + per-observer movement-broadcast filtering (§3.2). The
      `sneak` verb toggles a MOVING concealment: ephemeral `connActor` state
      (sneaking/score/instance, like hide but it SURVIVES a room change), the
      cancellable `concealment.before` on toggle-on + `entity.concealed/revealed`
      (source = sneak). `movementHandler` now filters its two enter/leave
      `SendToRoom` lines through `sneakUnseenBy` — a fresh per-move perception
      contest per occupant, reusing the existing `excludePlayerIDs` param (no
      Broadcaster change): a piercer sees the line, a failer sees nothing. Sneak
      does NOT feed the stationary `CanSee` render/resolve predicate (§3.2: hide
      governs stationary, sneak governs movement). `breakConcealmentOnAction`
      drops both hide and sneak. Unit-tested (state machine, verb toggle/cancel,
      drops-both-layers, end-to-end movement filter) + live (sneak on → look →
      sneak off).
- [x] **S5 — magical + admin invisibility** + detect traits; closes `who §4`
      and `admin-verbs §3` wizinvis forward-refs.
  - [x] **S5a — admin invisibility (wizinvis).** `wizinvis` admin-gated verb
        toggles an ephemeral, flag-gated concealment (atomic toggle on
        `connActor`, never persisted). The predicate adds a `SourceAdminInvis`
        layer and sets the observer's admin rank (flat roles → binary), so the
        render occupant list, the §5.4 resolvers, AND `who` all hide a wizinvis
        admin from lower ranks per-viewer — the viewer always sees themselves.
        Does NOT break on action (§3.4). Unit + live verified.
  - [x] **S5b — magical invisibility** — an `invisible` effect flag pierced only
        by the `see_invisible` counter (honored as a racial/ability tag OR an
        active effect flag); reuses the S5a predicate/who seams. An effect-
        lifecycle bridge emits `entity.revealed` when the effect ends. v1 is
        players-only (invisible mobs are a §9 extension); no content grants it
        yet, so the in-game trigger (a potion/spell) is a follow-up authoring
        task. Engine mechanic unit-verified.
- [x] **S6 — the `search` verb + hidden exits** (`hidden-exits.md`) — the
      original ask that started this arc. `world.Exit` gains `Hidden` +
      `SearchDifficulty` (a `hidden_exits:` room block); `search` runs the
      perception contest (with the active-search bonus, or auto-find via
      detect_hidden/admin) and records discovery in a per-observer, ephemeral,
      direction-keyed set cleared on room change. An undiscovered hidden exit
      is unlisted (exits-line filter) AND unwalkable (movement gate fails
      identically to no exit, PD-4); the move primitive stays unconditional
      (§4.1). Emits `exit.discovered`. A secret alcove off the starter-world
      forge makes it playable; unit + live verified. S6b refinements (secret-
      door op gating, flee gate, GMCP exits, passive-find) deferred + recorded.

Key finding (S1): light/darkness is ~80% shipped (`internal/light` per-viewer
resolver), so the spec's §3.3 darkness layer mostly *integrates* rather than
gets built — the darkness work is extending light-awareness to the non-render
consumers, not re-gating render.

---

### M29 — Player Trade (active)

The escrow/atomic-transaction substrate plus its two consumers — direct
player-to-player trade and a buyout auction house. Built once, consumed twice:
both consumers commit through the **one** escrow primitive
(`docs/specs/trade-escrow.md`), the engine's first true all-or-nothing
multi-actor value mover (`give` today is a documented racy two-step). Plan +
phase order in `docs/plans/trade-plan.md`. Two ship lines: A = Direct Trade
(Phases 0–2), B = Auction House (Phases 3–8).

- [x] **Phase 0 — escrow / atomic-transaction primitive** (`internal/escrow`,
      `trade-escrow.md` §2–§4). The `Transaction` (stage/withdraw/commit/
      rollback) over two injected seams — `Custodian` (the value mover) and
      `Bus` (the publish seam) — so the package stays free of session/entities/
      economy deps. Commit publishes the cancellable `trade.committing`
      pre-event (the validation/veto seam), moves every leg as one unit on
      no-veto, fires the non-cancellable `trade.committed`, and rolls everyone
      whole on any veto/withdraw/failure. Coin is debit-at-stage / credit-at-
      deliver (no `CurrencyService` change); cross-txn double-staging is
      structurally impossible (staging removes the item from the owner).
      Delivery is items-first / coin-last so a mid-commit failure never has to
      reverse a coin credit. Unit + `-race` tested (concurrent withdraw-vs-
      commit asserts value conservation), 86% cov; two review passes (APPROVE).
      Events: `trade.committing` / `trade.committed` (`internal/eventbus`).
- [x] **Phase 1 — audit log** (`trade-escrow.md` §5): append-only, versioned,
      persisted (`saves/trade-audit.yaml` via `persistence.AtomicWrite`); one
      record per commit (parties, item instances, coin, source, time), enough
      to reconstruct and reverse. Lands with Phase 0.
- [x] **Phase 2 — direct trade** (`direct-trade.md`) — **Ship line A done.**
      `internal/trade` (Manager/Session/custodian) over a `Party` interface the
      connActor satisfies; the symmetric `trade <player>` handshake, offer/
      rescind/offergold/rescindgold/confirm/decline verbs, the confirm +
      total-reset rule, the atomic swap via Phase 0, and graceful teardown
      (cancel on disconnect / link-death / room-separation, wired in session.go
      outside `a.mu`). **Stage model is remove-at-stage** (a staged item leaves
      inventory, coin is debited) — chosen over lock-in-place so no verb in any
      package (incl. the cross-package shop `sell`) can touch staged value, at
      the cost of a documented §6 deviation (a hard crash mid-trade loses staged
      value, matching the engine's existing give/buy window; graceful teardown
      is lossless). The `no_trade` tag gates tradability; capacity/weight veto
      deferred (the seam exists). Unit-tested over the real escrow + currency
      (incl. veto-make-whole, retry-after-veto, confirm-reset, rescind-by-query)
      + a live two-client smoke (`cmd/telnet-smoke`); two review passes.
- [ ] **Phase 3 — pickup delivery** (`auction-house.md` §7): escrow-holds-until-
      collected + the `collect` verb + notification notice (text only).
- [ ] **Phase 4 — listing + persisted store** (`auction-house.md` §3–§4): the
      `list` verb, the new versioned `internal/auction` store with restart
      reconciliation, the non-refundable listing fee, the per-player cap.
- [ ] **Phase 5 — browse / search** (`auction-house.md` §5): category/name
      filter, price/time sort, pagination, stable per-listing reference.
- [ ] **Phase 6 — buyout purchase** (`auction-house.md` §6): atomic buy through
      Phase 0 (coin to seller minus cut, item to buyer pickup).
- [ ] **Phase 7 — tick expiry + returns** (`auction-house.md` §8): recurring
      expiry handler, idempotent + expire-on-load for lapsed-while-down.
- [ ] **Phase 8 — fees as the gold sink** (`auction-house.md` §9): listing fee +
      sale cut wired as config; zero disables the sink. **Ship line B done.**

Deferred (post-MVP): bidding (Phase 9 — needs anti-sniping policy),
push-delivery/mail-attachments (Phase 10 — greenfield, shared with Mail),
location-scoped markets (Phase 11), admin moderation (role-gated, dependency
already shipped). See `docs/plans/trade-plan.md`.

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

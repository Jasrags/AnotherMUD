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
- [ ] `Room`, `Exit`, `World` types exist with the minimum fields needed
- [ ] `Clock` interface introduced; `time.Now()` not called directly in engine packages
- [ ] Tick loop runs at 100 ms cadence, driven by `Clock`, cancellable via ctx
- [ ] At least one tick-handler registration mechanism exists, even if empty
- [ ] Command dispatcher parses input and routes to a `Command` handler
- [ ] `look`, `n`, `s`, `quit` work; unknown commands produce a clear message
- [ ] A test advances the `Clock` and verifies tick handlers fire on schedule

**Touches specs:** `time-and-clock` §2-3, `world-rooms-movement` §2, `commands-and-dispatch` §2.

**Known gaps after M1:** no pack loading (world is hardcoded), no persistence (state dies on restart), no real entity model, no inventory, no other players visible to each other.

---

### M2 — Load from disk

**Slice:** replace the hardcoded world with a content pack loader.
Packs are **data-only** at this point — YAML/JSON files describing
rooms, exits, areas. No scripting language picked yet.

**Why this:** confronts `scripting-and-packs` two-phase loading
and the registry pattern without committing to a script runtime.
Forces the pack-discovery and validation pipeline to exist.

**Exit criteria:**
- [ ] Pack discovery walks a configurable content directory
- [ ] Pack manifest format defined (name, version, depends-on)
- [ ] Two-phase load: phase 1 registers tags/properties, phase 2 instantiates content
- [ ] Validation errors abort the load with actionable messages (which pack, which file, which field)
- [ ] World, Area, Room registries are pack-populated
- [ ] At least 3-4 rooms across 2 areas defined in pack files
- [ ] Reload story documented (even if "restart the server" for now)

**Touches specs:** `scripting-and-packs` §pack-discovery, §two-phase loading; `world-rooms-movement` §2.4 areas.

**Known gaps after M2:** no scripting runtime (data-only packs), no cross-pack reference resolution beyond what alphabetical order gives us, no hot reload.

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
- [ ] Account file shape matches `persistence` save/load surface
- [ ] Player file shape matches `persistence` save/load surface
- [ ] Password hashing uses a vetted algorithm (bcrypt or argon2; pick and document)
- [ ] File writes are atomic (write-temp-then-rename) so a crash mid-write doesn't corrupt
- [ ] Login state machine matches `login` spec stages
- [ ] Character location persists across restart
- [ ] An autosave tick handler runs at a configurable cadence
- [ ] Integration test: create account, log in, walk to room B, restart server, log in, verify in room B

**Touches specs:** `persistence`, `login`, `character-creation` (minimal — pre-existing test character is fine until full wizard lands).

**Known gaps after M3:** no character creation wizard, no quest file, no email verification, no link-dead recovery across restart.

---

### M4 — Another player

**Slice:** multi-session correctness. Two players in the same room
see each other. Flood protection, idle timeout, link-dead detection
from `session-lifecycle`. First time concurrency really bites.

**Why this:** every system from here on assumes multiple sessions
and that entities can observe each other. Getting concurrency right
on a small surface now beats debugging races in M6 combat.

**Exit criteria:**
- [ ] `SessionManager` tracks active sessions with concurrency-safe access
- [ ] Player A sees "B has arrived" when B enters their room
- [ ] Flood protection rejects abusive input rates per `session-lifecycle` §flood
- [ ] Idle timeout disconnects per `session-lifecycle` §5
- [ ] Link-dead detection per `session-lifecycle` §7
- [ ] Takeover (same account logs in twice) handled per `session-lifecycle` spec
- [ ] Race detector clean: `make test` (which uses `-race`) stays green under load

**Touches specs:** `session-lifecycle` substantially, `world-rooms-movement` (entity tracking layer).

**Known gaps after M4:** no chat channels, no tells, no who list.

---

### Beyond M4

The exact ordering past M4 is less load-bearing because the substrate
is now real. Sketch of remaining vertical slices:

- **M5 — Inventory & items:** `inventory-equipment-items`, slot system,
  keyword resolution.
- **M6 — Mobs walking around:** `mobs-ai-spawning`, mob templates,
  area-driven spawning, the AI tick.
- **M7 — Hit something:** `combat`, engage/disengage, the heartbeat
  bucket, death.
- **M8 — Get better:** `progression`, stats, levels, races, classes,
  tracks.
- **M9 — Do something:** `abilities-and-effects`, the action queue,
  effects.
- **M10 — Quests & UI polish:** `quests`, `ui-rendering-help` themes
  and panels.
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

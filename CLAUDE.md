# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository status

The repo is the **M3 slice (persistence + login)** of the engine: content packs loaded from `content/core/` populate a `world.World` at boot, a tick loop runs (with an autosave handler), accounts and player saves live under `ANOTHERMUD_SAVE_DIR`, and a telnet connection runs through the login state machine before entering the game loop. The 17 behavior specs (under `docs/specs/`) are language-agnostic and remain the source of truth for behavior; the Go layout is filling in milestone by milestone (see `docs/ROADMAP.md`).

- **Language:** Go (module `github.com/Jasrags/AnotherMUD`, `go 1.26`)
- **Entrypoint:** `cmd/anothermud/main.go` — opens the account + player stores under `ANOTHERMUD_SAVE_DIR` (default `./saves`), loads content via `pack.Load` against `ANOTHERMUD_CONTENT_DIR` (default `./content`), starts the tick loop with an autosave handler at `ANOTHERMUD_AUTOSAVE_INTERVAL` (default `30s`), runs `server.Serve` with `session.Handler`. Starting room for new characters is `ANOTHERMUD_START_ROOM` (default `tapestry-core:town-square`).
- **Packages in play:** `internal/clock` (Clock interface + Real/Manual), `internal/tick` (game loop + handler registration), `internal/world` (Direction, Room, Area, World registry + move primitive), `internal/pack` (manifest, discovery, dep-ordering, two-phase content loader), `internal/command` (registry + dispatcher + builtins), `internal/persistence` (atomic file I/O + path safety), `internal/account` (account file store + bcrypt service), `internal/player` (player save + migration scaffold), `internal/login` (name/email/password state machine), `internal/session` (per-connection actor + Manager + read→dispatch loop), `internal/conn[/telnet]`, `internal/server`, `internal/logging`.
- **Content packs:** `content/core/` ships the engine-namespace (`tapestry-core`) starter pack — two areas (town, wilderness) and four rooms (town-square, forge, market, village-gate). All room/area ids are namespaced (`tapestry-core:town-square`); unqualified ids in YAML resolve against the current pack's namespace, qualified ids (`other-pack:foo`) cross packs.
- **Saves on disk:** `<save-dir>/accounts/index.yaml` (email → account id), `<save-dir>/accounts/<id>/account.yaml`, `<save-dir>/players/<lowercased-name>/player.yaml`. Writes use the tmp→bak→rename rotation in `internal/persistence`. Player saves carry a `version` field; `player.CurrentVersion` is `1` today, migration table empty but scaffolded.
- **Login flow:** Name prompt → if character file exists → Password (returning); else Email → Password (with confirmation for net-new accounts) → handoff. New characters start at `ANOTHERMUD_START_ROOM`. Returning characters land in their persisted `location` (falling back to start if the saved room is no longer in content).
- **Autosave:** `session.Manager` tracks logged-in actors. The autosave tick handler calls `Manager.SaveAll`, which writes any actor whose `dirty` bit is set. `SetRoom` flips the bit. Final flush on shutdown so SIGINT commits live state. Per-player errors are isolated (one bad save does not abort the batch).
- **F3 status:** the `Clock` interface exists. `time.Now()` is only called inside `clock.RealClock`, `internal/account` (account `created_at`), and the `cmd/anothermud` binary; engine packages take a `Clock`.
- **Scripting language:** undecided. The previous incarnation used Lua. The `scripting-and-packs` spec is written language-agnostically; M2 is intentionally data-only so the runtime choice (Lua via gopher-lua, JS via goja, Starlark, Wasm, etc.) can be made on real evidence after content authoring exposes the gaps.

### Commands

```
go build ./...              # build everything
go run ./cmd/anothermud     # run the server (telnet localhost 4000)
go test -race ./...         # run tests (race detector mandatory)
```

When asked to implement features, **read the relevant spec first** — they are the source of truth for behavior. The specs reference some Tapestry-specific names (e.g. `tapestry-core` engine namespace); treat those as placeholder strings unless/until renamed.

### Roadmap and foundations

- `docs/ROADMAP.md` — the milestone plan (M0 echo telnet → M1 two rooms → M2 pack loader → M3 persistence → M4 multi-session → …). Each milestone has exit-criteria checkboxes. Treat the current milestone as the one with unchecked boxes that no later milestone has started.
- `docs/ROADMAP.md#foundations` — binding conventions adopted from day one. The short version, in case ROADMAP isn't loaded:
  - **F1**: `ctx context.Context` is the first parameter on anything that does I/O, ticks, or is cancellable.
  - **F2**: structured logging is `log/slog` with the logger carried on `ctx`. Field names follow the table in the ROADMAP foundations section (`session_id`, `entity_id`, `room_id`, `tick`, `event`, `pack`, `err`, …).
  - **F4**: errors wrap with `fmt.Errorf("doing X: %w", err)` and use package-level sentinel `var Err... = errors.New(...)`.
  - **F3 (deferred)**: a `Clock` interface lands in M1 with the tick loop. Once it exists, no direct `time.Now()` calls inside engine packages.

## Spec architecture

Specs are layered bottom-up. The reading order in `docs/specs/README.md` is canonical:

1. **Substrate** — `time-and-clock`, `persistence`, `scripting-and-packs`, `networking-protocols`
2. **World/entities** — `world-rooms-movement`, `progression`, `inventory-equipment-items`, `mobs-ai-spawning`
3. **Action/interaction** — `commands-and-dispatch`, `abilities-and-effects`, `combat`, `quests`, `economy-survival`
4. **Player lifecycle** — `login`, `character-creation`, `session-lifecycle`
5. **Presentation** — `ui-rendering-help`

`docs/specs/README.md` also holds the cross-cutting indexes that span specs:

- **Cancellable events table** — which specs emit which cancellable bus events.
- **Registry table** — pack-loaded content registries in roughly the order packs touch them at load time.
- **Save/load surface** — what's in account vs. player vs. quest files; what is deliberately *not* persisted (sessions, in-game time, weather, link-dead state, mob spawn tracking, temporary exits, active effects, rest state).
- **Tick handlers table** — canonical scheduler entries with cadences (tick = 100 ms by default; cadence 10 = 1 s, 300 = 30 s).

When touching any spec, check whether these tables in the README also need updating.

## Spec conventions (locked)

Every spec follows the same shape — keep it:

- Overview (concepts, goals/non-goals)
- Narrative sections organized around operations
- Per-section **acceptance criteria** checklists (designed to read as tests)
- **Configuration surface** table (everything externally configurable)
- **Open questions** (preserves design tensions, do not delete to "clean up")

Specs are **behavior-only**: no specific numeric values, library names, or implementation language. Values that matter for interoperability (telnet option codes, IAC bytes) are called out explicitly; everything else numeric goes in the configuration-surface table.

## Cross-cutting themes to watch

The README's open-question summary tracks issues that recur across specs — flag these when relevant:

- Hardcoded magic values not yet externalized (cap tiers, flee cooldown, sustenance cap, JS sandbox limits, engine namespace)
- Persistence gaps (in-game time, weather, link-dead-across-restart, active effects, temporary exits, rest state)
- Pack load order relies on alphabetical discovery — no topological sort over declared dependencies yet
- Ad-hoc staleness guards (session takeover, combat death) rather than a general event-versioning primitive
- Role tier hierarchy exists in help-service but doesn't actually elevate non-admin roles
- Unbounded growth in render cache, bad-input tracker, notification queues

## Developer Learning Protocol

The goal of this protocol is to keep the developer connected to the codebase
as it evolves — not just to produce correct code, but to ensure the developer
understands what was built and why.

### Before every change

Before writing any code, briefly explain:
1. **What files/packages will be touched** and why
2. **What invariant or contract is being extended or relied on** (reference
   architecture.md / CONVENTIONS.md where relevant)
3. **How this fits the tick/event/layer model** — which layer owns this,
   which bucket or event is involved, what the data flow is

Keep this to 3-5 sentences. Don't skip it even for small changes.

### While writing code

- **Name the pattern** when you use one: "this follows the same repo seam as
  MobInstance," "this subscribes to the tick bucket the same way Affects does"
- **Flag deviations** immediately: if the cleanest implementation would violate
  a convention or cross a dependency boundary, say so before writing it, not
  after
- **Don't bury decisions**: if a non-obvious design choice is made (e.g. why
  a field lives on creature.Core vs. a separate table), add a one-line comment
  in the code and explain it in the response

### After every non-trivial change

Provide a short **"what just happened" summary**:
- What was added/changed in plain language
- What a developer should check or test to verify it works
- Whether any spec doc (architecture.md, ROADMAP.md, PLAN.md) needs updating
  as a result

### Periodic orientation (ask on request: "orient me")

When asked to "orient me" on a system or the whole codebase, produce:
1. **The current state** — what exists, where it lives, what it does
2. **The active seams** — where the system connects to others right now
3. **The open edges** — what's stubbed, partial, or known to be incomplete
4. **The next decision** — the one architectural or design choice that will
   most shape what comes next

This is distinct from summarizing the spec docs. It should reflect actual
code state, not design intent.

### Drift detection

If a request would cause the implementation to diverge from the spec docs,
say so explicitly: "This would deviate from [doc] because [reason]. Here are
two ways to stay aligned: ..."

Never silently implement something that conflicts with an existing convention
or architectural boundary.

## Deferred work tracking

When a code review (or any other moment) surfaces issues that won't be
addressed in the current commit, they must be persisted to memory so they
resurface in a future session. Otherwise they become invisible debt.

### When to defer vs. fix now

Fix in the current change when:
- The finding is CRITICAL or a security issue.
- The fix is small (≤ a few lines) and lives in code already being
  touched by the change.
- Shipping without the fix would leave a known bug in a path users hit.

Defer (and record) when:
- The finding is HIGH/MEDIUM and lives outside the diff's natural scope.
- The fix needs design work or coordinates with an upcoming milestone.
- Bundling it would obscure the current commit's intent.

A HIGH item should still come with a "fix-by" trigger (e.g. "before M6
multi-session ships"), not just an open-ended deferral.

### How to record a deferral

Write a memory file named `m<N>-deferred-fixes.md` under
`~/.claude/projects/-Users-jrags-Code-Jasrags-AnotherMUD/memory/`, then
add a one-line entry to `MEMORY.md` so the index loads it. Existing
examples to mirror: `m0-deferred-fixes.md`, `m1-deferred-fixes.md`,
`m5-deferred-fixes.md`.

Each entry in the file should include:
1. **Severity** (HIGH/MEDIUM/LOW) and short title.
2. **File:line** pointer to the code in question.
3. **What's wrong** in one or two sentences.
4. **Suggested fix** — concrete enough that future-you doesn't have to
   re-derive the analysis.
5. **When to fix** — a specific trigger ("next time `session.go` is
   touched", "before M6", "if the test flakes").

Link related deferral files with `[[m1-deferred-fixes]]` so the memory
graph stays connected.

### When to surface deferrals

- At the start of a new milestone — read the relevant `m<N>-deferred-fixes.md` files first.
- When editing a file named in a deferral entry — address that entry in the same change if it fits.
- During "orient me" — call out outstanding deferrals as part of the open edges.

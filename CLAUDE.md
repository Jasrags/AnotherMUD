# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository status

The repo is **well past prototype.** Milestones **M0–M17 are complete** — including all five cross-cutting themes (Social/M13, Engine-Debt/M14, World-Depth/M15, Modern-Client+GMCP/M16, Content-Authoring+scripting/M17) — and **M18** (small command/UI polish) is in progress. What works today: the tick loop + event bus; rooms/areas/exits with **doors, locks, temporary portals, weather, and an in-game clock**; the entity store (items + mobs, double-buffered tag index); inventory/equipment/slots/stacking/keyword resolution; progression (stats/races/classes/tracks/alignment/training/**proficiency with use-based gain**/**abilities**/**effects**); combat (engage/round/hit-miss-damage/flee/death); mobs + area spawning + AI + loot; economy (currency/shops/**sustenance**/**rest**/**consumables**); quests; the command registry with **typed arguments** (M17.2); a **sandboxed Lua scripting runtime** (gopher-lua) with hot reload; social (notifications/tells/channels/emotes); accounts + versioned player saves + an interactive **character-creation wizard** + sessions (flood/idle/link-dead/takeover); **telnet with full IAC negotiation + WebSocket + GMCP packages + tiered ANSI color**.

The **39 behavior specs** under `docs/specs/` are the source of truth; the Go layout fills them in milestone by milestone. **Several specs are contracts written ahead of code** (tag-observers, visibility, hidden-exits, faction, biomes, gathering, room-coordinates, crafting-and-cooking, and the trade trio trade-escrow/direct-trade/auction-house) — `docs/specs/README.md`'s footer + tables mark which are spec-only. (Since-shipped, formerly on that list: roles-and-permissions, admin-verbs, item-decorations, who, loot-and-corpses, tab-completion, light-and-darkness.) Companion docs: `docs/ROADMAP.md` (done-log + active milestone), `docs/BACKLOG.md` (open work + greenfield design items), `docs/PRIMER.md` (a pasteable orientation for external design work). For a fast token-lean orientation to the code layout, `docs/CODEMAPS/` (architecture, backend/engine-flow, presentation, data, dependencies) is a derived navigation aid — keep it subordinate to the specs and regenerate via `/update-codemaps` after large changes rather than hand-editing.

- **Language:** Go (module `github.com/Jasrags/AnotherMUD`, `go 1.26`)
- **Entrypoint:** `cmd/anothermud/main.go` — the composition root. Opens the account + player stores under `ANOTHERMUD_SAVE_DIR` (default `./saves`), loads content (including pack Lua scripts) via `pack.Load` against `ANOTHERMUD_CONTENT_DIR` (default `./content`), wires every service (combat, progression, effects, quests, economy, scripting runtime, GMCP flushers, …), registers the tick handlers, and runs the telnet listener on `ANOTHERMUD_ADDR` plus an optional WebSocket listener on `ANOTHERMUD_WS_ADDR`. Starting room is `ANOTHERMUD_START_ROOM` (default `tapestry-core:town-square`). Many knobs are env-configurable (`ANOTHERMUD_TICK_INTERVAL`, `_AUTOSAVE_INTERVAL`, `_COMBAT_CADENCE`, `_FLEE_COOLDOWN`, `_IDLE_SWEEP_INTERVAL`, `_SUSTENANCE_DRAIN_INTERVAL`/`_AMOUNT`, `_LINKDEAD_*`, `_LOG_FORMAT`/`_LEVEL`, `_WS_*`, …).
- **Packages in play** (`internal/…`, grouped by layer):
  - *Foundations:* `tick` (game loop + handler registration), `eventbus` (typed cancellable + non-cancellable bus), `clock` (wall Clock) + `gameclock` (in-game hour/day clock), `logging`, `persistence` (atomic tmp→bak→rename file I/O + path safety), `srckey` (modifier-source leaf, breaks the entities↔stats cycle).
  - *World + things:* `world` (rooms/areas/exits/doors), `entities` (Store, MobInstance, ItemInstance, Placement, Contents, tag index), `item`/`mob`/`slot` (templates + registries), `keyword`, `spawn`, `ai`, `portal` (temporary exits), `weather`, `property` (registry + tagged-value envelope).
  - *Character mechanics:* `stats`, `progression` (tracks/proficiency/abilities/training/alignment), `combat`, `effect`.
  - *Action + interaction:* `command` (registry + dispatcher + typed-arg resolvers + builtins), `economy` (currency/shops/sustenance/rest/consumables), `quest`/`queststore`/`questwatch`.
  - *Player lifecycle:* `account` (bcrypt store), `player` (versioned save), `login`, `session` (actor + Manager + flood/idle/link-dead/takeover), `wizard` (creation flow).
  - *Social:* `chat`, `notifications`, `emote`.
  - *Presentation:* `render`, `ansi`, `help`.
  - *Networking:* `conn`/`conn/telnet`/`conn/ws`, `server`, `gmcp`, `mssp`.
  - *Content + scripting:* `pack` (manifest/discovery/dep-order/two-phase loader), `script` (registry), `scripting` (sandboxed gopher-lua runtime + bus bridge).
- **Content packs:** `content/core/` ships the engine-namespace (`tapestry-core`) starter pack. It now spans `areas` (town, wilderness), `rooms` (town-square, forge, market, village-gate), `items`, `mobs`, `abilities`, `classes`, `races`, `tracks`, `effects`, `slots`, `quests`, `weather_zones`, `theme`, `help`, and `scripts` (Lua). The setting is a **placeholder** — content names like `tapestry-core` are not a committed setting; specs stay setting-agnostic. All room/area ids are namespaced (`tapestry-core:town-square`); unqualified ids in YAML resolve against the current pack's namespace, qualified ids (`other-pack:foo`) cross packs.
- **Saves on disk:** `<save-dir>/accounts/index.yaml` (email → account id), `<save-dir>/accounts/<id>/account.yaml`, `<save-dir>/players/<lowercased-name>/player.yaml` (+ sibling `quest.yaml`, `notifications.yaml`, chat-subscriptions file), and `<save-dir>/channels/<id>.yaml` (global channel scrollback). Writes use the tmp→bak→rename rotation in `internal/persistence`. Player saves carry a `version` field; `player.CurrentVersion` is **17** with a populated migration chain (each migration is append-only — never edit an old one). The player save now carries tags, roles, stats, properties, equipment/inventory, abilities + proficiencies, recall address, prompt template, visited-room fog-of-war set, and known crafting recipes.
- **Login flow:** Name prompt → if character file exists → Password (returning); else Email → Password (with confirmation for net-new accounts) → handoff. New characters start at `ANOTHERMUD_START_ROOM`. Returning characters land in their persisted `location` (falling back to start if the saved room is no longer in content).
- **Autosave:** `session.Manager` tracks logged-in actors. The autosave tick handler calls `Manager.SaveAll`, which writes any actor whose `dirty` bit is set. `SetRoom` flips the bit. Final flush on shutdown so SIGINT commits live state. Per-player errors are isolated (one bad save does not abort the batch).
- **F3 status:** the `Clock` interface exists and is honored — engine packages read *simulation/wall* time through a `Clock`, never `time.Now()` directly (the in-game `gameclock` is tick-driven, not wall-clock). Direct `time.Now()` survives only in `clock.RealClock`, `internal/account` (`created_at`), the `cmd/anothermud` binary, and as an **accepted exception for RNG seeding** (`internal/ai`, `internal/spawn` seed a PRNG from wall-clock nanos — seeding randomness, not reading time).
- **Scripting language: decided — Lua (gopher-lua), landed in M17.** `internal/scripting` is a sandboxed per-instance LState (base/table/string/math only; no os/io/debug/package), with a bus bridge (`engine.subscribe`/`log`/`schedule`), pack script discovery + a script registry, and hot reload. Pack Lua lives under `content/<pack>/scripts/*.lua`. The `scripting-and-packs` spec remains language-agnostic; the Go side committed to gopher-lua.

### Commands

```
go build ./...              # build everything
go run ./cmd/anothermud     # run the server (telnet localhost 4000)
go test -race ./...         # run tests (race detector mandatory)
```

When asked to implement features, **read the relevant spec first** — they are the source of truth for behavior. The specs reference some Tapestry-specific names (e.g. `tapestry-core` engine namespace); treat those as placeholder strings unless/until renamed.

### Git workflow

**Work directly on `main`. Do NOT create feature branches.** This is a solo
project with no PR-gating workflow — when asked to commit/push, stage and
commit on `main` and `git push origin main` directly; never `git checkout -b`.
This intentionally **overrides** any default "branch off the default branch
first" rule. Per-slice commits on `main` are the rhythm; code review still
runs before a phase is called complete (that gate is independent of branching).

### Roadmap and foundations

- `docs/ROADMAP.md` — the milestone **done-log** (M0 echo telnet → … → M17 + all five themes) plus the active milestone (M18). It is now mostly history; for **what to build next**, read `docs/BACKLOG.md` (open §1 specced-ready items, §2 greenfield design items, candidate themes). The old "current milestone = the section with unchecked boxes" heuristic no longer applies — the planned arc shipped, so new work is scoped from the BACKLOG.
- `docs/ROADMAP.md#foundations` — binding conventions adopted from day one. The short version, in case ROADMAP isn't loaded:
  - **F1**: `ctx context.Context` is the first parameter on anything that does I/O, ticks, or is cancellable.
  - **F2**: structured logging is `log/slog` with the logger carried on `ctx`. Field names follow the table in the ROADMAP foundations section (`session_id`, `entity_id`, `room_id`, `tick`, `event`, `pack`, `err`, …).
  - **F4**: errors wrap with `fmt.Errorf("doing X: %w", err)` and use package-level sentinel `var Err... = errors.New(...)`.
  - **F3 (deferred)**: a `Clock` interface lands in M1 with the tick loop. Once it exists, no direct `time.Now()` calls inside engine packages.

## Spec architecture

Specs are layered bottom-up. **The reading order in `docs/specs/README.md` is canonical and kept current** — consult it rather than this sketch when it matters. The layers (with the specs added since the early set in **bold**):

1. **Substrate** — `time-and-clock`, `persistence`, `scripting-and-packs`, `networking-protocols`, **`notifications`**
2. **World/entities** — `world-rooms-movement`, **`tag-observers`**, `progression`, `inventory-equipment-items`, `mobs-ai-spawning`
3. **Action/interaction** — `commands-and-dispatch`, `abilities-and-effects`, `combat`, `quests`, `economy-survival`, **`crafting-and-cooking`**, **`trade-escrow`/`direct-trade`/`auction-house`**, **`chat-channels-and-tells`**, **`emotes`**, **`recall`**, **`admin-verbs`**, **`who`**
4. **Player lifecycle** — `login`, `character-creation`, `session-lifecycle`, **`roles-and-permissions`**
5. **Presentation** — `ui-rendering-help`, **`item-decorations`**

`docs/specs/README.md` also holds the cross-cutting indexes that span specs:

- **Cancellable events table** — which specs emit which cancellable bus events.
- **Registry table** — pack-loaded content registries in roughly the order packs touch them at load time.
- **Save/load surface** — what's in account vs. player vs. sibling files (quest, notifications, chat-subscriptions) vs. global stores (channel scrollback, **in-game time** at `saves/clock.yaml`; spec-pending: the auction listing store + the trade audit log); what is deliberately *not* persisted (sessions, weather, link-dead state, mob spawn tracking, temporary exits, active effects, rest state, direct-trade sessions).
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

- Hardcoded magic values not yet externalized (cap tiers, flee cooldown, sustenance cap, Lua sandbox limits, engine namespace)
- Persistence gaps — still unbuilt in code: weather, link-dead-across-restart, active effects, temporary exits, rest state. (In-game time **now persists** — `internal/gameclock/store.go` → `saves/clock.yaml`, seeded at boot, flushed on hour advance + clean shutdown; light-and-darkness §7, resolving time-and-clock §3.6.)
- Pack load order is dependency-ordered: `internal/pack/order.go` runs Kahn's-algorithm topological sort (alphabetical tie-breaks, `ErrCycle`/`ErrUnknownDep`), wired into `pack.Load` (loader.go). (Was an open gap; resolved — verified 2026-06-06.)
- Ad-hoc staleness guards (session takeover, combat death) rather than a general event-versioning primitive
- **Role enforcement — LANDED.** `roles-and-permissions` (flat `HasRole`) + `admin-verbs` are implemented and enforcing: admin commands gate on `HasRole(adminRole)` in the command registry, with live `grant`/`revoke` verbs and role-change events. No longer a gap — downstream gates (e.g. auction moderation) can rely on it.
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

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository status

The repo is **well past prototype.** Milestones **M0–M29 are complete** — the five original cross-cutting themes (Social/M13, Engine-Debt/M14, World-Depth/M15, Modern-Client+GMCP/M16, Content-Authoring+scripting/M17), then M18 (command/UI polish), M19 (roles & admin), M20 (item decorations), M21 (item stacking), M22 (loot & corpses + decay), M23 (room coordinates), M24 (player maps), M25 (equipment slots), M26 (engine debt II), M27 (crafting & cooking), M28 (visibility + hidden exits), M29 (player trade — escrow/atomic-transaction + direct trade + buyout auction house) — **plus light & darkness, biomes/gathering, movement cost/encumbrance, account-first login + character roster + world-locking, mounts (ride + mounted travel), and the in-progress WoT mechanics EPIC (weapon-identity, masterwork item grades, saves, conditions, skills, feats, the One Power)**. What works today: the tick loop + event bus; rooms/areas/exits with **doors, locks, temporary portals, weather, an in-game clock, and per-viewer light/darkness**; the entity store (items + mobs, double-buffered tag index); inventory/equipment/slots/stacking/keyword resolution; progression (stats/races/classes/tracks/alignment/training/**proficiency with use-based gain**/**abilities**/**effects**); combat (engage/round/hit-miss-damage/flee/death/**corpses+looting+decay**); mobs + area spawning + AI + loot; economy (currency/shops/**sustenance**/**rest**/**consumables**); **crafting & cooking + gathering (forage/harvest) over biome resource tables**; **room coordinates + an ASCII `map`/minimap with persisted fog-of-war**; quests; the command registry with **typed arguments** (M17.2); a **sandboxed Lua scripting runtime** (gopher-lua) with hot reload; social (notifications/tells/channels/emotes); accounts + versioned player saves + an interactive **character-creation wizard** + sessions (flood/idle/link-dead/takeover); **telnet with full IAC negotiation + WebSocket + GMCP packages + tiered ANSI color**.

The **57 behavior specs** under `docs/specs/` are the source of truth; the Go layout fills them in milestone by milestone. **A few specs are contracts still written ahead of code** (tag-observers, faction) — `docs/specs/README.md`'s footer + tables mark which are spec-only. (Since-shipped, formerly on that list: roles-and-permissions, admin-verbs, item-decorations, who, loot-and-corpses, tab-completion, light-and-darkness, **biomes, gathering, room-coordinates, crafting-and-cooking, visibility, hidden-exits** (M28), the **trade trio** trade-escrow/direct-trade/auction-house (M29), `weapon-identity` + `masterwork` + `armor-depth` + `size-and-wielding` + `ranged-combat` + `two-weapon-fighting` (WoT EPIC S1 — the last is increment K complete: off-hand attack, the feats, Improved TWF, mob dual-wield), **movement-cost** (encumbrance + biome-weighted step cost), **character-select** (account-first login), **character-identity** (world-locking, save v23), **mounts** (core-v1: ride + mounted travel, save v26).) **The active arc is the Wheel-of-Time setting** — a content track (`docs/themes/wot-world-plan.md`, geography) and a mechanics program (`docs/themes/wot-mechanics-epic.md`, the EPIC; Decision 0 resolved to translate WoT onto the tick/chance model, not a d20 rewrite). Companion docs: `docs/ROADMAP.md` (done-log), `docs/BACKLOG.md` (open work + greenfield design items), `docs/DEFERRED-BACKLOG.md` (deferred-fix index), `docs/PRIMER.md` (a pasteable orientation for external design work). For a fast token-lean orientation to the code layout, `docs/CODEMAPS/` (architecture, backend/engine-flow, presentation, data, dependencies) is a derived navigation aid — keep it subordinate to the specs and regenerate via `/update-codemaps` after large changes rather than hand-editing.

- **Language:** Go (module `github.com/Jasrags/AnotherMUD`, `go 1.26`)
- **Entrypoint:** `cmd/anothermud/main.go` — the composition root. Opens the account + player stores under `ANOTHERMUD_SAVE_DIR` (default `./saves`), loads content (including pack Lua scripts) via `pack.Load` against `ANOTHERMUD_CONTENT_DIR` (default `./content`), wires every service (combat, progression, effects, quests, economy, scripting runtime, GMCP flushers, …), registers the tick handlers, and runs the telnet listener on `ANOTHERMUD_ADDR` plus an optional WebSocket listener on `ANOTHERMUD_WS_ADDR`. Active packs are `ANOTHERMUD_PACKS` (default `starter-world`, + its dep closure; `=wot` boots the WoT pack), and the starting room is `ANOTHERMUD_START_ROOM` (default `starter-world:town-square`). Many knobs are env-configurable (`ANOTHERMUD_TICK_INTERVAL`, `_AUTOSAVE_INTERVAL`, `_COMBAT_CADENCE`, `_FLEE_COOLDOWN`, `_IDLE_SWEEP_INTERVAL`, `_SUSTENANCE_DRAIN_INTERVAL`/`_AMOUNT`, `_LINKDEAD_*`, `_LOG_FORMAT`/`_LEVEL`, `_WS_*`, …).
- **Packages in play** (`internal/…`, grouped by layer):
  - *Foundations:* `tick` (game loop + handler registration), `eventbus` (typed cancellable + non-cancellable bus), `clock` (wall Clock) + `gameclock` (in-game hour/day clock), `logging`, `persistence` (atomic tmp→bak→rename file I/O + path safety), `srckey` (modifier-source leaf, breaks the entities↔stats cycle).
  - *World + things:* `world` (rooms/areas/exits/doors + derived coordinates), `entities` (Store, MobInstance, ItemInstance, Placement, Contents, tag index), `item`/`mob`/`slot` (templates + registries), `keyword`, `spawn`, `ai`, `portal` (temporary exits), `weather`, `biome` (terrain definitions + ambience), `light` (per-viewer effective light + sources/fuel), `visibility` (per-observer can-see: hide/sneak/invis/search), `property` (registry + tagged-value envelope).
  - *Character mechanics:* `stats`, `progression` (tracks/proficiency/abilities/training/alignment), `combat`, `effect`, `condition` (status conditions), `feat` (player-chosen perks), `pool` (generalized resource pools), `channel` (derived-stat formula layer).
  - *Action + interaction:* `command` (registry + dispatcher + typed-arg resolvers + builtins), `economy` (currency/shops/sustenance/rest/consumables), `crafting`/`recipe` (+ `campfire` station), `gathering` (forage + harvest nodes), `grade` (item quality grades / masterwork), `loot`/`corpse` (loot tables + corpse creation/decay), `quest`/`queststore`/`questwatch`.
  - *Player lifecycle:* `account` (bcrypt store), `player` (versioned save), `login`, `session` (actor + Manager + flood/idle/link-dead/takeover), `wizard` (creation flow).
  - *Social:* `chat`, `notifications`, `emote`.
  - *Presentation:* `render`, `ansi`, `help`, `decoration`, `stacking`.
  - *Networking:* `conn`/`conn/telnet`/`conn/ws`, `server`, `gmcp`, `mssp`.
  - *Content + scripting:* `pack` (manifest/discovery/dep-order/two-phase loader), `script` (registry), `scripting` (sandboxed gopher-lua runtime + bus bridge).
- **Content packs:** three ship today — `content/core/` (the engine-namespace `tapestry-core` baseline: slots/races/classes/tracks/abilities/effects/rarity/essence/biomes/weather/theme/help — **no world**), `content/starter-world/` (the demo village; the default boot, depends on core), and `content/wot/` (a Wheel-of-Time content pack in progress, depends on core). A boot selects active packs via `ANOTHERMUD_PACKS` (default `starter-world`; the dependency closure is pulled in, so `=wot` also loads `tapestry-core`). The `tapestry-core` name is a **placeholder** — specs stay setting-agnostic. All room/area ids are namespaced (`starter-world:town-square`); unqualified ids in YAML resolve against the current pack's namespace, qualified ids (`other-pack:foo`) cross packs. See `docs/ENGINE-VOCABULARY.md` for the content↔engine contract and `make run` / `make run-wot` for the two boots.
- **Saves on disk:** `<save-dir>/accounts/index.yaml` (an email→id `entries` map plus a username→id `usernames` map; login now authenticates by username, the email map is retained for back-compat and backfilled on load), `<save-dir>/accounts/<id>/account.yaml`, `<save-dir>/players/<lowercased-name>/player.yaml` (+ sibling `quest.yaml`, `notifications.yaml`, chat-subscriptions file), and `<save-dir>/channels/<id>.yaml` (global channel scrollback). Writes use the tmp→bak→rename rotation in `internal/persistence`. Player saves carry a `version` field; `player.CurrentVersion` is **27** with a populated migration chain (each migration is append-only — never edit an old one). The player save now carries tags, roles, stats (incl. persisted base stats), properties, equipment/inventory, abilities + proficiencies, recall address, prompt template, visited-room fog-of-war set, known crafting recipes, feat credits + known feats (v20), the generalized resource-pool currents (mana/movement/the One Power — v21), the character's gender (v22 — a general attribute that fills the engine's `AllowedGenders` contract and derives a WoT channeler's saidin/saidar affinity), the character's **`WorldID`** (v23 — world-locking: the leaf ruleset pack the character belongs to, backfilled from the `Location` namespace; a returning character is gated at login to a server running its world — `character-identity.md`, `internal/pack` manifest `kind: world|library`), and a **`movement_max`** base stat (v24 — backfills the movement-points pool onto pre-feature saves whose persisted base predates the movement-cost gate; the at-login `RestoreBase` merge already supplies it from `DefaultPlayerBase`, so the migration just makes the on-disk shape explicit), and the character's **`madness`** — a male channeler's accumulated saidin taint (v25 — WoT S2 Phase 4+; rises per saidin weave + overchannel, decays when abstaining, above a threshold a ~10s tick rolls a chance to inflict a Core-5 condition; cured by the Heal-the-Mind weave; the migration is a no-op, 0 for every pre-v25/non-channeler save), and the character's **owned `mounts`** — a list of `MountRecord`s, the durable resting form of a rideable mount (v26 — mounts.md §10; each carries the mount's template identity, with barding/saddlebag/upkeep additive in later slices; the live ride relationship is never persisted; the migration is a no-op, empty for every pre-v26 save and every mountless character), and the character's **`PowerAttackActive`** — whether the Power Attack combat stance is on (v27 — feats Bucket C; the melee accuracy-for-power trade toggled by `powerattack on|off`; a persistent posture, not a per-swing choice; the migration is a no-op, off for every pre-v27 save).
- **Login flow (account-first — `character-select.md`):** **Account username** prompt → if the account exists → Password → **character roster** (the account's characters, each shown with its world + availability); else (new username) → choose+confirm Password (email dropped/deprecated) → account created → empty roster. From the roster the player **selects** an available character (→ Playing) or **creates** a new one (character name → wizard, stamped with the active world). An out-of-world character (its world not in the active set) is listed **greyed/unselectable** (the `character-identity` world gate). New characters start at `ANOTHERMUD_START_ROOM`; returning characters land in their persisted `location` (falling back to start). Accounts carry a unique username (login key) + many characters; the prior character-name-first / email flow is superseded.
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

### Model selection

Pick the cheaper model that can do the job; escalate only when the work
actually demands deeper reasoning. Default to **`claude-sonnet-4-6`** and
reach for **`claude-opus-4-8`** for the hard, high-stakes work.

**Use `claude-sonnet-4-6` (default) for:**
- Implementing a specced slice where the design is already settled.
- Single-file or small multi-file edits, bug fixes, and refactors with a clear shape.
- Writing tests, docs, commit messages, and routine YAML/content authoring.
- Mechanical follow-ups from a review (applying agreed fixes).
- Tracing/answering "where does X live" and other code-navigation questions.

**Use `claude-opus-4-8` for:**
- Architectural decisions that shape multiple packages or cross a spec boundary
  (e.g. a new substrate seam, an event-versioning primitive, a save-version bump
  whose migration chain is non-trivial).
- New spec authoring or resolving an Open Question / Decision-0-style fork.
- Cross-cutting changes that touch the tick/event/layer model or a dependency
  boundary, where getting the seam wrong is expensive to unwind.
- Debugging subtle concurrency / ordering / race issues (the `-race`-class bugs).
- Multi-step work spanning many files where the plan itself is uncertain.

**Escalate Sonnet → Opus mid-task when:**
- Sonnet has tried twice and the fix isn't converging (thrashing, re-introducing
  the same bug, or fighting the type/borrow/lock model).
- The change turns out to deviate from a spec or violate a convention, and the
  right call needs design judgment (see "Drift detection" below).
- The blast radius grew past the original estimate — what looked like a one-file
  edit now reshapes a package contract or a persisted save shape.
- A code review surfaces a CRITICAL/architectural finding rather than a mechanical one.

When escalating, hand off with the current state captured (what was tried, what
failed, the relevant file:line) so Opus isn't re-deriving from scratch.

### Roadmap and foundations

- `docs/ROADMAP.md` — the milestone **done-log** (M0 echo telnet → … → M27 + all five themes + light/dark + biomes/gathering). It is now mostly history; for **what to build next**, read `docs/BACKLOG.md` (open §1 specced-ready items, §2 greenfield design items, candidate themes) and — for the next planned arc — `docs/themes/wot-mechanics-epic.md` + `docs/themes/wot-world-plan.md`. The old "current milestone = the section with unchecked boxes" heuristic no longer applies — the planned arc shipped, so new work is scoped from the BACKLOG.
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

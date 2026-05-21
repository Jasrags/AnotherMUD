# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository status

The repo is a **Go scaffold plus 17 behavior specs**. The specs (under `docs/specs/`) are language-agnostic and remain the source of truth for behavior; the Go layout is empty wiring waiting to be filled in.

- **Language:** Go (module `github.com/Jasrags/AnotherMUD`, `go 1.26`)
- **Entrypoint:** `cmd/anothermud/main.go` — currently a no-op
- **Scripting language:** undecided. The previous incarnation used Lua. The `scripting-and-packs` spec is written language-agnostically; the runtime choice (Lua via gopher-lua, JS via goja, Starlark, Wasm, etc.) is open and should be picked deliberately when pack loading lands.

### Commands

```
go build ./...              # build everything
go run ./cmd/anothermud     # run the scaffold entrypoint
go test ./...               # run tests (none yet)
```

When asked to implement features, **read the relevant spec first** — they are the source of truth for behavior. The specs reference some Tapestry-specific names (e.g. `tapestry-core` engine namespace); treat those as placeholder strings unless/until renamed.

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

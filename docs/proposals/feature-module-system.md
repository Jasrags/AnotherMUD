# Proposal: Feature-Module System (code-level feature packaging)

**Status:** Draft / for discussion · **Type:** Architecture proposal (pre-spec) · **Audience:** engine
**Feeds into:** a future `feature-modules.md` spec + plan, and reshapes how the §2 gameplay-module cluster ships
**Builds on:** the existing runtime substrate — `command.Registry`, `eventbus.Bus`, the `scripting` runtime, the `pack` loader, and the `pack.Registries` set. No new substrate is required; this is a *packaging seam* over what already exists.
**Reference:** GoMud's plugin system (`GoMud/internal/plugins`) — the inspiration and the cautionary tale (see [[gomud-module-reference]]). BACKLOG §2 "Feature-module system".

## 1. Problem / motivation

Every gameplay feature's code today is woven through `internal/…` and wired by hand at the composition root. `cmd/anothermud/main.go` is ~470 lines that construct each service, register each command, attach each tick handler, and bridge each event sink inline — channels register their verbs in one loop (`main.go:284`), emotes in another (`main.go:295`), ability verbs in a third (`main.go:214`), the quest store, notification manager, chat registry, and a dozen services each get their own bespoke block. This works, but it has three costs:

1. **No feature boundary.** "The auction house" is not a thing you can point at — its commands, listeners, data, and persistence would be scattered across `internal/command`, `internal/economy`, a new store, and main.go wiring. Adding or removing a feature means surgery in multiple packages plus the boot sequence.
2. **The composition root is the bottleneck.** Every new feature grows `main.go`. There is no way for a feature to declare "here is everything I need wired" and have the boot sequence honor it; a human transcribes that wiring each time.
3. **No enable/disable.** A feature is either compiled-and-wired or absent. There is no operator switch to run a server *without* gambling, or a test build with only the core verbs.

We already solved the **data half** of this: a content pack (`content/<pack>/`) is a self-contained, dependency-ordered, hot-reloadable bundle of data + Lua. What's missing is the **code half** — a way to package a feature's *Go code* (commands + listeners + script functions + lifecycle) as one self-describing, toggleable unit that wires *itself* in, instead of being hand-grafted. GoMud demonstrates the value of the idea; this proposal adapts it to AnotherMUD's conventions without copying its mistakes.

## 2. Goals & non-goals

**Goals.** A `Module` contract: a self-contained Go type that declares its name/version/dependencies and, given an injected context of engine seams, registers everything it owns (commands, event listeners, scripting functions, tick handlers, engine-baseline content) in one `Register` call. A **module registry + loader** that orders modules by declared dependency (reusing the `pack.Order` topological-sort pattern), honors an **enable/disable manifest**, and invokes each module's `Register` at a single, well-defined point in boot — collapsing the per-feature wiring blocks in `main.go` into one loop. A demonstration migration of one or two existing self-contained features (chat channels, emotes) to prove the seam carries real weight. The seam that **every §2 gameplay system is then delivered through** (gambling, leaderboards, alt-characters, …) rather than another graft into `internal/`.

**Non-goals (rule out now).**
- **Not runtime plugin loading.** No `plugin.Open` / `.so` files, no `go-plugin` RPC, no WASM. Modules are **compiled into the one static binary**; the toggle is configuration, not dynamic linking (§6 explains why, and why this still beats GoMud's recompile-to-enable).
- **Not hot-swap.** A module is enabled/disabled at boot; toggling it live is out of scope (content packs already cover the hot-reloadable *data* case; Lua covers live behavior tweaks).
- **Not sandboxed third-party code.** Modules are first-party Go with full engine access. Untrusted/author-supplied behavior is what the **Lua scripting runtime** is for — that boundary stays.
- **Not a rewrite of the engine into modules.** Core systems (tick loop, event bus, world, entities, sessions, networking) stay as engine packages. Modules are the *optional/feature* layer on top. The migration of existing features is opt-in and incremental, not a big-bang.
- **Not a web admin layer.** That's a sibling BACKLOG §2 item; it composes naturally *after* this (a per-module admin page is the payoff of the seam) but is its own slice.

## 3. Proposed approach (the shape)

**A `Module` contract, a dependency-ordered registry, and one boot-time `Register` loop.**

A module is a Go type satisfying a small interface (final shape is §5/open):

```go
// internal/module (new package — the contract + registry + loader)
type Module interface {
    Meta() Meta                       // name, version, declared deps
    Register(ctx context.Context, mc *Context) error
}

type Meta struct {
    Name     string
    Version  string
    Requires []Dependency             // other modules this one needs
}
```

The injected `*module.Context` is the **dependency-injection seam** — it bundles the engine handles a feature wires into, the same ones `main.go` threads by hand today:

```go
type Context struct {
    Commands   *command.Registry      // AddUserCommand analogue: Register / RegisterCommand
    Bus        *eventbus.Bus          // Subscribe(name, handler) → unsubscribe
    Scripts    ScriptRegistrar        // expose module funcs to Lua (modules.<name>.<fn>)
    Loop       *tick.Loop             // Register(name, cadence, handler) for scheduled work
    Registries *pack.Registries       // engine-baseline content registration (slots, properties, …)
    Clock      clock.Clock            // F3 — never time.Now()
    // … the handful of other shared services a feature needs (sessions, persistence dir)
}
```

This is **constructor-injected, not package-global** — the central correction to GoMud's `init()`-with-globals style (§8). A module receives its dependencies; it does not reach into a global registry from an `init()` side-effect.

**The loader** (`module.Loader`) is the new code path in boot:

1. Collect the **compiled-in module set** (a slice assembled in `main.go` or a small `modules/` package — explicit Go, no code generation).
2. Apply the **enable/disable manifest** (config: which modules are on). Disabled modules are dropped here.
3. **Order by dependency** — reuse the exact pattern in `internal/pack/order.go` (topological sort, `ErrCycle`, `ErrUnknownDep`). A module requiring a disabled/absent module is a **loud boot error**, mirroring pack dependency validation.
4. Invoke each module's `Register(ctx, mc)` in order. A module wires its commands/listeners/script-fns/tick-handlers through `mc`.

The result: the per-feature blocks scattered through `main.go` (channels loop, emotes loop, future auction block, …) collapse into one `loader.RegisterAll(ctx, mc)` call. `main.go` still owns the *engine* construction (stores, bus, tick loop, listener); modules own the *feature* wiring.

## 4. What a module owns (the extension surface)

A module wires into the seams that already exist — this is deliberately the union of what `main.go` does by hand today, nothing new:

| Capability | Mechanism (existing) | Today's hand-wired analogue |
|---|---|---|
| Player/admin commands | `mc.Commands.Register` / `.RegisterCommand` (typed args, M17.2) | `cmds.Register(...)` loops in `main.go` |
| Event reactions | `mc.Bus.Subscribe(name, handler)` → unsubscribe | scattered `bus.Subscribe` calls in services |
| Cancellable-event vetoes | `mc.Bus` + `PublishCancellable` | combat/container adders |
| Scheduled work | `mc.Loop.Register(name, cadence, fn)` | the autosave / idle-sweep / gmcp-flush blocks |
| Scripting hooks | `mc.Scripts` → `modules.<name>.<fn>` in Lua | (new — no per-feature analogue yet) |
| Engine-baseline content | `mc.Registries.*` register-if-absent | `slot.RegisterEngineBaseline`, `pack.RegisterEngineBaselineProperties` |
| Lifecycle | the `Register` call itself + a returned teardown / `Close` (open, §7) | `defer scriptRuntime.Close()`, shutdown flush |

**Persistence** is the one seam that needs a decision (§7). v1 default: a module uses the existing stores (player save bag, a feature store under `saves/` mirroring `queststore`/`notifications`). A *module-owned, auto-discovered* save surface is a later increment, not v1.

**Content pairing.** A module is **code-only in v1**. If a feature needs data, that data lives in a normal content pack (`content/<pack>/`), loaded by the existing pack loader; the module registers the *engine-baseline* registrations its data relies on (the `RegisterEngineBaseline` pattern). Fusing a module with an embedded content sub-pack — GoMud's one-unit model — is an explicit non-goal for v1 (keeps the data path single and hot-reloadable).

## 5. The central fork: enable/disable model

This is the decision that shapes everything. Three options:

- **(A) Always-on registered modules.** Modules exist purely for *code organization* — every compiled-in module always registers. No manifest, no toggle. Simplest; delivers goals 1–2 (feature boundary, no main.go bottleneck) but **not goal 3** (operator enable/disable).
- **(B) Compiled-in, manifest-gated at boot.** *(recommended)* All modules compile into one binary, but a config manifest decides which `Register` at boot. One static artifact; the toggle is configuration. Delivers all three goals. Strictly better than GoMud's model: GoMud toggles features by **editing source + `go generate` + recompile**; (B) toggles by **editing config + restart**, no rebuild.
- **(C) True runtime plugins.** Dynamically loaded code (`plugin`, `go-plugin`, WASM). Maximal flexibility; **rejected for v1** — Go's `plugin` package is fragile (exact-toolchain/version coupling, no unload), `go-plugin` adds RPC overhead and process management, WASM adds a heavy host boundary and loses direct engine access. The thing people *want* from "plugins" (don't recompile to toggle a feature) is delivered by (B) without any of this cost.

**Recommendation: (B).** It hits all three goals with one static binary, respects Go's strengths, and is a clean superset of (A) (an empty/all-on manifest *is* always-on). The manifest is small: a list of enabled module names, validated against the compiled-in set + the dependency graph at boot.

## 6. Why compiled-in still beats recompile-to-enable

Worth stating plainly because it's the crux of "is this even worth it vs. GoMud." GoMud's modules are also compiled in — the difference is the **toggle mechanism**. GoMud: a `go generate` step rewrites a blank-import file and you rebuild the binary to add/remove a feature. AnotherMUD (B): the binary ships with all modules; an operator flips a config entry and restarts. No toolchain on the server, no rebuild, reproducible artifact, and the same binary can run different feature sets per deployment. We also already beat GoMud on the *data* half (our packs hot-reload; their module data is embedded and needs a rebuild). So the combined story — hot-reloadable content packs + config-gated code modules in one static binary — is more operable than GoMud's, while borrowing its best idea (the self-describing feature unit).

## 7. Open questions (for sign-off before the spec)

- **`Module` interface shape.** Method set beyond `Meta()` + `Register()`? Specifically: does a module return a **teardown** func (for clean shutdown / the eventual hot-disable), or is process lifetime assumed (matching most current subscriptions, which live for the process)? *Leaning: `Register` returns an optional `io.Closer`-like teardown; v1 modules mostly return nil.*
- **`module.Context` membership.** Exactly which engine handles go in the injected context? Too many and it's a god-object; too few and modules can't do their job. *Leaning: start minimal (Commands, Bus, Loop, Scripts, Registries, Clock) and grow by need, the way `main.go` reveals what each feature actually touches.*
- **Persistence ownership.** Does a module get an **auto-wired, namespaced save surface** (`saves/modules/<name>.yaml` with load-on-boot / flush-on-autosave), or does it use existing stores by hand in v1? *Leaning: by-hand in v1 (reuse the `queststore`/`notifications` store shape); auto-wired module-save is a later increment once 2–3 modules show the common shape.*
- **Enable/disable manifest location + format.** A top-level config block (`ANOTHERMUD_MODULES` / a `modules.yaml`)? How does it interact with the dependency graph (auto-enable deps, or error if a dep is disabled)? *Leaning: explicit list + hard error on a missing/disabled dependency, mirroring pack dep validation — no silent auto-enable.*
- **Compiled-in set assembly.** A hand-maintained slice in a `modules/` package (explicit, greppable) vs. a registration-on-import pattern (closer to GoMud, but reintroduces `init()` side-effects). *Leaning: explicit slice — the import side-effect pattern is exactly the convention break we're avoiding.*
- **Migration scope.** Which existing features migrate as proof? *Leaning: chat channels + emotes (already self-contained, already loop-registered — a clean before/after) — but NOT the engine core.*
- **Inter-module communication.** GoMud has `ExportFunction`/`GetExportedFunction` for module-to-module calls. Do we need it, or do modules coordinate purely through the event bus? *Leaning: bus-only in v1 (typed events are the existing seam); add an export registry only if a concrete need appears.*
- **Ordering vs. the engine.** Modules register *after* engine construction and pack load (they depend on registries being populated). Is a single "after everything, before serving" phase sufficient, or do some modules need an earlier hook (e.g. to register engine-baseline content before pack validation)? *Leaning: two phases — a `RegisterBaseline` (pre-pack-load, for slot/property baselines) and a `Register` (post-load, for commands/listeners), mirroring how `slot.RegisterEngineBaseline` runs before `pack.Load` today.*

## 8. Alternatives considered & rejected

- **GoMud's model verbatim** (`init()` + package globals + `go generate` blank imports + recompile-to-toggle) — rejected on two counts: the `init()`-with-globals style fights our ctx-first + immutability conventions and our event-versioning discipline, and recompile-to-toggle is worse than config-gating (§6). We keep the *idea* (self-describing feature unit, one registration call, declared dependencies) and drop the *mechanism*.
- **True runtime plugins (`.so`/`go-plugin`/WASM)** — rejected for v1 (§5C): cost far exceeds the benefit, and the benefit people actually want is delivered by config-gating.
- **Do nothing / keep hand-wiring** — the honest baseline. Defensible while the feature count is small, but every §2 gameplay system makes `main.go` longer and the "where is feature X" problem worse. The seam pays for itself at roughly the second or third new optional feature; gambling/leaderboards/alt-characters are exactly that horizon.
- **A pure code-organization convention (no registry, just packages)** — i.e. option (A) without even a loader. Rejected as insufficient: it doesn't collapse the main.go wiring (a human still calls each feature's setup) and gives no enable/disable. The registry + manifest is what turns "tidy packages" into a real feature seam.

## 9. Dependencies & risks

Enabling substrate **all already exists** — this is the proposal's central claim and its main de-risking: `command.Registry` (typed-arg commands), `eventbus.Bus.Subscribe` (string-keyed, returns unsubscribe — clean teardown primitive already present), the `scripting.Runtime` (module→Lua exposure), `tick.Loop.Register`, the `pack.Registries` set, and crucially `internal/pack/order.go` (a working topological dependency sort with cycle detection to mirror, not reinvent). No greenfield engine system is required.

Risks worth naming:

- **God-object `module.Context`.** The injected context could accrete every service in the engine. Mitigation: start minimal, grow by demonstrated need; if it gets large, split into role-specific sub-contexts.
- **Registration order subtleties.** Some features need engine-baseline content registered *before* pack load (slots/properties) and commands registered *after*. The two-phase split (§7) addresses it but must be specced precisely or modules will race the pack loader.
- **Teardown correctness.** If v1 ships a teardown story (even unused), it must actually unsubscribe bus handlers and deregister commands or a future hot-disable leaks. Mitigation: lean on `Bus.Subscribe`'s existing unsubscribe return; defer command-deregistration until hot-disable is real.
- **Scope creep into web admin / runtime plugins.** Both are tempting adjacencies. Keep them out of v1 (explicit non-goals §2); the seam is valuable on its own.
- **Migration churn.** Moving chat/emotes to modules touches working code for an organizational win. Mitigation: migrate exactly one or two as proof; do not mass-migrate until the contract is proven.

## 10. Rough sizing & phasing

The contract + loader is **small and self-contained** — a `Module` interface, a `Context` struct, and a loader that filters by manifest then orders via the existing topo-sort pattern. The risk and the thinking are in the **contract design** (§7), not the code volume. Suggested phasing:

1. **Contract + loader + manifest** — `internal/module` package: the interface, the injected context (minimal membership), the dependency-ordered loader reusing `pack.Order`'s pattern, the enable/disable manifest + boot validation. No feature moves yet; the loader runs over an empty set.
2. **Proof migration** — move chat channels (and/or emotes) from their `main.go` loops into a module. Validates the seam carries commands + registry-driven wiring, and produces a real before/after on `main.go`.
3. **First greenfield module** — deliver one §2 gameplay feature (leaderboards is a good first: event-fed, modest persistence, no deep coupling) *as a module*, establishing the pattern the rest of the cluster follows.
4. **(Later, separate slices)** module-owned save surface; web admin auto-discovery; teardown/hot-disable — each its own increment, none required for the seam to deliver value.

---

*Acceptance criteria and the configuration-surface table are deliberately omitted — those are spec-level. The dominant fork is the **enable/disable model** (§5), recommended as (B) compiled-in + manifest-gated; the rest of §7 is contract-shape detail that sign-off can settle. The proposal's load-bearing claim is that **no new substrate is needed** — every seam a module wires into already exists and is clean — so the work is packaging and contract design, not engine-building. Ready to become a `docs/specs/feature-modules.md` once §5 and the §7 contract questions are signed off.*

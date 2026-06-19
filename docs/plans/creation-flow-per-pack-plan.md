# Plan: per-pack character-creation flows (`CreationFlowFor` selector + WoT channeling step)

> **Status: APPROVED, not yet implemented.** Scope locked = **Phase 1 + Phase 2
> option (a)**. This file is self-contained — a fresh context can execute it from
> here plus the cited file:line anchors. Delete this file once the work ships.

## Goal

Let each content pack (starter-world, WoT, later Shadowrun) have its own
character-creation flow, **without** touching the generic wizard engine and
**without** changing the default flow for non-customized worlds. This is
"Option A" from the design discussion: branch the flow builder by active world
in Go; the wizard engine (`internal/wizard`) is already generic and pack-flow
ready.

## Grounding facts (verified 2026-06-18)

- The wizard engine is generic: a `wizard.Flow` is an ordered `[]Step`
  (`InfoStep`, `ChoiceStep`, `ConfirmStep`) run by `wizard.Instance`. It already
  supports echo-off/secret steps, a GMCP sink, a restart loop, and help
  passthrough. Only the *assembly* is hardcoded.
- The single hardcoded assembly is **`session.NewCreationFlow(races, classes,
  backgrounds)`** in `internal/session/creation_flow.go` (builds intro → gender →
  race → class → background → confirm from the global registries).
- It is wired **once at boot** in `cmd/anothermud/main.go:2819`:
  `CreationFlow: session.NewCreationFlow(registries.Races, registries.Classes, registries.Backgrounds)`.
- The active world set is available at that point: **`registries.Worlds`**
  (`internal/pack/registries.go:145`, a `[]string` of active world namespaces;
  character-identity §2). One server boots ONE world today (co-host deferred), so
  the "primary world" is the single `kind: world` entry; empty set → default flow.
- **`wizard.ChoiceStep`** (`internal/wizard/steps.go:62`) has `ID`, `Prompt`
  (static string), `Options []Option` (**static slice**), `OnSelect func(e, value)`,
  and `Skip skipFn` (a per-entity skip predicate — `ShouldSkip(e)` at
  `steps.go:71`). **Options/prompt cannot vary by an earlier answer; only Skip is
  dynamic.** → gender-specific options would need TWO Skip-gated steps, not one.
- **Channeling is already a CLASS in WoT** — `content/wot/classes/initiate.yaml`
  and `wilder.yaml` (+ `content/wot/tracks/one-power.yaml`). Gender already
  derives saidin/saidar affinity (consumed downstream from `loaded.Player.Gender`).
  This is why the channeling-gift step is scoped to a **non-consequential
  placeholder** (option (a)) — a real gift mechanic overlaps the class choice and
  is a separate WoT-chargen design task.
- `creationEntity` (the pending character) lives in `creation_flow.go:52`
  (fields: raceID, classID, backgroundID, gender, rejected). Commit happens in
  `runCreation` (`creation_flow.go:317-336`), stamping ids onto `loaded.Player.*`.

## Locked decisions

- **Option (a)** for the channeling step: a gender-AGNOSTIC `ChoiceStep` that
  records onto a new `creationEntity.channelingGift` field but is **NOT
  persisted** (no save-version bump) and has **no downstream effect** yet. It
  exists to demonstrate the per-pack-step pattern end-to-end. Making it
  consequential (eligibility gate / persisted spark-vs-learned flag) is an
  explicit follow-up, NOT in this scope.
- **Default flow preserved byte-for-byte** for `""`/`starter-world`/unknown
  worlds. Existing creation tests + a new Phase-1 regression test lock this.
- No wizard-engine changes. No new content vocabulary. No save bump.

## Phase 1 — the selector (mechanical, zero behavior change)

1. **Refactor `NewCreationFlow` into reusable step builders.** Today its body
   inlines the assembly. Extract small unexported helpers in `creation_flow.go`
   so the WoT builder reuses them rather than duplicating — e.g.
   `introStep()`, `genderStep()`, `raceStep(opts []wizard.Option)`,
   `classStep(opts)`, `backgroundStep(opts)`, `confirmStep()`, plus the
   `Flow{...}` wrapper + `OnComplete`. `NewCreationFlow` must produce the EXACT
   same flow as today (same step IDs, order, prompts, the nil-return when no
   race/class/background content).
2. **Add `CreationFlowFor`** in `creation_flow.go`:
   ```go
   func CreationFlowFor(world string, races *progression.RaceRegistry,
       classes *progression.ClassRegistry, backgrounds *progression.BackgroundRegistry) *wizard.Flow {
       switch strings.ToLower(strings.TrimSpace(world)) {
       case "wot":
           return newWoTCreationFlow(races, classes, backgrounds)
       default:
           return NewCreationFlow(races, classes, backgrounds)
       }
   }
   ```
3. **main.go**: replace the `creation_flow.go:2819` wiring. Compute the primary
   active world from `registries.Worlds` (single element today; empty → `""`),
   pass it to `CreationFlowFor`. Suggested helper near the wiring:
   `primaryWorld := ""; if len(registries.Worlds) > 0 { primaryWorld = registries.Worlds[0] }`
   (registries.Worlds holds only `kind: world` namespaces; if that needs
   confirming, check `internal/pack/loader.go:154-167` where Worlds is populated).
4. **Tests** (`internal/session/creation_flow_test.go` or a new file):
   - `CreationFlowFor("")`, `("starter-world")`, `("nonsense")` → identical step
     IDs/order to `NewCreationFlow(...)` (regression lock).
   - `CreationFlowFor("wot")` → a flow whose step IDs include the channeling step
     (Phase 2) — assert it differs from the default.

## Phase 2 — the WoT builder + channeling step (option (a))

1. **Add `creationEntity.channelingGift string`** field (`creation_flow.go:52`).
   Leave the commit path (`runCreation`) UNCHANGED — the field is recorded but
   not stamped onto `loaded.Player` (non-persisted, per option (a)). Optionally
   add a one-line structured log of the choice for visibility; no save write.
2. **`newWoTCreationFlow(...)`** (new, `creation_flow.go`): build the SAME steps
   as the default via the Phase-1 helpers, but INSERT the channeling step after
   `genderStep()` (gender must precede — it's the saidin/saidar key). Reuse the
   default `raceStep/classStep/backgroundStep/confirmStep` + the same
   `Flow{ID:"character-creation", OnComplete:...}` wrapper.
3. **The channeling step** = a gender-agnostic `ChoiceStep`:
   ```go
   &wizard.ChoiceStep{
       ID:     "channeling",
       Prompt: "Your relationship to the One Power:",
       Options: []wizard.Option{
           {Label: "Born with the spark", Value: "spark",
            Description: "The Power came to you unbidden; you must learn control or die."},
           {Label: "Able to learn", Value: "learn",
            Description: "You could be taught to channel, given a teacher."},
           {Label: "Cannot channel", Value: "none",
            Description: "The True Source is closed to you."},
       },
       OnSelect: func(e wizard.Entity, v any) { e.(*creationEntity).channelingGift = v.(string) },
   }
   ```
   (Gender-agnostic on purpose — `ChoiceStep.Options` is static, so saidin/saidar
   wording would require two `Skip`-gated steps; out of scope. The downstream
   affinity already derives from `Gender`, so this stays flavor for now.)
4. **Tests**: the WoT flow contains step ID `channeling` positioned immediately
   after `gender` and before `class`/`race`; the default flow has no `channeling`
   step; selecting an option records it on the entity (drive the step's
   `OnSelect` directly, mirroring the existing wizard_test pattern).

## Verification gate (run before calling it done)

```sh
cd /Users/jrags/Code/Jasrags/AnotherMUD
go build ./...
go test -race ./internal/session/ ./internal/wizard/ ./cmd/...
go test -race ./...            # full suite green
gofmt -l internal/session/creation_flow.go cmd/anothermud/main.go   # empty
```
- Default flow unchanged (regression test green).
- WoT flow includes the channeling step in the right position.
- No save-version change; no wizard-engine change.

## Out of scope (explicit follow-ups, do NOT do here)

- Making the channeling choice consequential: gating initiate/wilder class
  eligibility on it (blocked — needs dynamic `ChoiceStep` options), or persisting
  a spark/learned flag (needs a save bump + overlaps the class model). Design WoT
  chargen properly first.
- A Shadowrun flow (`newShadowrunCreationFlow`) — the selector leaves a clean
  slot (`case "shadowrun":`) for it later.
- Declarative (manifest-YAML) or Lua-scripted flows (Options B/C from the
  discussion) — only revisit if content-authored flows are wanted.

## Pointers

- Selector + default + WoT builder: `internal/session/creation_flow.go`
- Boot wiring: `cmd/anothermud/main.go:2819` (+ `registries.Worlds`)
- Wizard step types: `internal/wizard/steps.go` (ChoiceStep `:62`, Skip `:71`)
- WoT channeling classes: `content/wot/classes/{initiate,wilder}.yaml`

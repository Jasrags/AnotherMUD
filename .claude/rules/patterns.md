---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
# Go Patterns — AnotherMUD

> Extends the global common patterns rules with this repo's Go conventions.
> The engine is a **tick / event / layer** model — know which layer owns a
> change, which bus event or tick bucket is involved, and what the data flow is
> before writing it (see the Developer Learning Protocol in `CLAUDE.md`).

## Composition-root dependency injection

Wiring happens in **one place**: `cmd/anothermud/main.go` (the composition
root). Services take their collaborators via constructor functions and are
assembled there — do not reach for globals or service locators.

```go
func NewCombatService(bus *eventbus.Bus, clock clock.Clock, log *slog.Logger) *CombatService {
    return &CombatService{bus: bus, clock: clock, log: log}
}
```

## Event bus

- Subscribe on the typed `eventbus` (cancellable vs non-cancellable buses).
- Cancellable events model pre-action hooks (a subscriber can veto); the
  README's "Cancellable events" table is the index of which specs emit which.
- Beware re-entrant dispatch (an event handler that triggers another event) —
  it's used deliberately (e.g. follow-chains) but guard against unbounded loops.

## Tick handlers

Register scheduler entries with the `tick` package; cadence is in ticks
(tick = 100 ms default; cadence 10 = 1 s). The README "Tick handlers" table is
canonical — update it when you add one.

## Repository seam

Data access goes through the `entities.Store` and the template registries
(`item`/`mob`/`slot`), not ad-hoc file reads. Persistence is atomic
(tmp→bak→rename) via `internal/persistence`.

## Breaking import cycles

When two packages need a shared leaf, extract it rather than creating a cycle —
e.g. `internal/srckey` exists solely to break the entities↔stats cycle. Follow
that precedent.

## Save migrations are append-only

`player.CurrentVersion` + a migration chain. **Never edit an existing
migration** — add a new one and bump the version. A save-shape change is a
deliberate, high-stakes commit.

## Idiomatic helpers

- **Functional options** for optional construction params.
- **Small interfaces** defined at the point of use.

## Reference

See skill: `golang-patterns` for concurrency, error handling, and package
organization.

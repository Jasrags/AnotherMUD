---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
# Go Coding Style — AnotherMUD

> Extends the global common coding-style rules with this repo's Go conventions.
> Module `github.com/Jasrags/AnotherMUD`, `go 1.26`. The `docs/specs/` behavior
> specs are the source of truth — **read the relevant spec before writing code.**

## Formatting

- **gofmt** and **goimports** are mandatory — no style debates.

## Foundations (binding since day one — see `docs/ROADMAP.md#foundations`)

- **F1 — context first.** `ctx context.Context` is the **first parameter** on
  anything that does I/O, ticks, or is cancellable.
- **F2 — structured logging.** Use `log/slog` with the logger carried **on
  `ctx`**, never a package global. Field names follow the ROADMAP table:
  `session_id`, `entity_id`, `room_id`, `tick`, `event`, `pack`, `err`.
- **F3 — no wall clock in engine code.** Engine packages read simulation/wall
  time through a `Clock`, **never `time.Now()` directly**. Accepted exceptions:
  `clock.RealClock`, `internal/account` (`created_at`), `cmd/anothermud`, and
  RNG seeding in `internal/ai` / `internal/spawn`.
- **F4 — error wrapping + sentinels.** Wrap with
  `fmt.Errorf("doing X: %w", err)` and use package-level sentinels
  (`var ErrFoo = errors.New("...")`) for comparable failures.

## Design Principles

- Accept interfaces, return structs.
- Keep interfaces small (1–3 methods); define them where they are **used**.
- Prefer immutable updates (return a new copy) over in-place mutation where the
  data is shared across ticks/goroutines.

## File Organization

- Many small, cohesive files: **200–400 lines typical, 800 max.** Extract
  utilities from large modules; organize by domain (`internal/<pkg>`), not type.

## Reference

See skill: `golang-patterns` for comprehensive Go idioms and patterns.

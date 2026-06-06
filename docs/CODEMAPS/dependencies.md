<!-- Generated: 2026-06-06 | External deps + conventions | Token estimate: ~550 -->

# Dependencies & Conventions

Deliberately minimal external surface — 4 third-party modules, no framework, no
network services. Everything else is stdlib.

## External modules (go.mod, go 1.26)
| Module | Use |
|---|---|
| `golang.org/x/crypto` | bcrypt password hashing (`internal/account`) |
| `gopkg.in/yaml.v3` | all save + content (de)serialization |
| `github.com/yuin/gopher-lua` v1.1.2 | sandboxed pack scripting (`internal/scripting`) |
| `github.com/coder/websocket` v1.8.14 | WS transport (`internal/conn/ws`) |

No DB driver, no HTTP framework, no cache, no message broker — state is files +
in-process tick loop / event bus.

## Internal "shared libraries" (leaf utilities)
- `internal/persistence` — atomic tmp→bak→rename file I/O + path safety.
- `internal/keyword` — shared item/entity match rules (resolver + completion).
- `internal/eventbus` — typed cancellable/non-cancellable bus.
- `internal/clock` (wall) + `internal/gameclock` (tick-driven in-game time +
  global `clock.yaml` persistence).
- `internal/srckey` — modifier-source leaf, breaks the entities↔stats cycle.
- `internal/property` — registry + tagged-value envelope.
- `internal/light` — pure per-viewer effective-light resolver (level/config/
  resolve/source/fuel/viewer); imports only `world` (terrain) + `gameclock`
  (period names), so call sites gather inputs and it stays a leaf.
- `internal/world` (terrain) — `TerrainOf`/`IsShielded` + terrain vocabulary,
  the shared sky-gate classifier both `weather` and `light` key off.

## Foundations (binding conventions, ROADMAP §foundations)
- **F1** `ctx context.Context` first param on anything doing I/O / ticks / cancellable.
- **F2** structured logging = `log/slog` carried on ctx; field names per ROADMAP table.
- **F3** `Clock` interface — engine pkgs never call `time.Now()` directly
  (exceptions: `clock.RealClock`, `account` created_at, the `cmd` binary, RNG
  seeding in `ai`/`spawn`).
- **F4** errors wrap `fmt.Errorf("doing X: %w", err)`; package sentinel `var Err…`.

## Config (env, all `ANOTHERMUD_*`)
`SAVE_DIR`, `CONTENT_DIR`, `ADDR`, `WS_ADDR`, `START_ROOM`, `TICK_INTERVAL`,
`AUTOSAVE_INTERVAL`, `COMBAT_CADENCE`, `FLEE_COOLDOWN`, `IDLE_SWEEP_INTERVAL`,
`SUSTENANCE_DRAIN_INTERVAL`/`_AMOUNT`, `LINKDEAD_*`,
`CORPSE_LIFETIME`/`_OWNERSHIP_WINDOW`, `LOG_FORMAT`/`_LEVEL`, `WS_*`.

## Build / test
`go build ./...` · `go run ./cmd/anothermud` (telnet :4000) ·
`go test -race ./...` (race detector mandatory).

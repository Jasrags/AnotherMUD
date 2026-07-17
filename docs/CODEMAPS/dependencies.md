<!-- Generated: 2026-07-17 | External deps + conventions | Token estimate: ~700 -->

# Dependencies & Conventions

Deliberately minimal external surface — 4 third-party modules, no framework, no
network services. Everything else is stdlib.

## External modules (go.mod, go 1.26.4)
| Module | Use |
|---|---|
| `golang.org/x/crypto` | bcrypt password hashing (`internal/account`) |
| `gopkg.in/yaml.v3` | all save + content (de)serialization |
| `github.com/yuin/gopher-lua` v1.1.2 | sandboxed pack scripting (`internal/scripting`) |
| `github.com/coder/websocket` v1.8.14 | WS transport (`internal/conn/ws`) |

No DB driver, no HTTP framework, no cache, no message broker — state is files +
in-process tick loop / event bus.

## Internal "shared libraries" (leaf utilities, 78 packages total)
- `internal/persistence` — atomic tmp→bak→rename file I/O + path safety.
- `internal/keyword` — shared item/entity match rules (resolver + completion).
- `internal/eventbus` — typed cancellable/non-cancellable bus.
- `internal/clock` (wall) + `internal/gameclock` (tick-driven in-game time +
  global `clock.yaml` persistence).
- `internal/srckey` — modifier-source leaf, breaks the entities↔stats cycle.
- `internal/logging` — `log/slog` setup carried on ctx (F2).
- `internal/pool` — generalized resource-pool primitive (vitals/mana/movement/
  stun + mob-seed pools); a Vitals facade fronts it, save-persisted
  currents only (v21+).
- `internal/channel` — derived-stat formula layer (hand-rolled eval, no
  code-exec); combat attack/defense/damage/mitigation derive via content map +
  mob spawn pools (shadowrun stun monitor, etc.).
- `internal/grade` — item quality-grade vocabulary (masterwork/power-wrought).
- `internal/condition` — status conditions (KO, sickened, unconscious, etc.) with combat hooks.
- `internal/feat` — player-chosen perks (known-feat + credit tracking, source-keyed
  bonuses, stackable/per-parameter variants).
- `internal/action` — busy-state tracker + don/doff timers + reload/load gate for
  action economy (movement/combat/reload blocking).
- `internal/size` — sized-weapon validation + wield mode (one-hand/two-hand/etc.).
- `internal/faction` — per-character standing map, rank tags, shift events (v31+).
- `internal/reputation` — single-axis renown score, tier tags (v32+).
- `internal/karma` — spendable-advancement balance (Current/Total); SR karma-ledger
  state machine, nil for level-track worlds (v39+).
- `internal/security` — per-player heat tracker, wanted-level patrol spawning,
  de-escalation verbs (v38+).
- `internal/guard` — per-actor state-machine supervisor (guard assignment + move-gate enforcement).
- `internal/questspawn` — quest-scoped runtime mob/item creation per-player,
  per-observer visibility filter (Phase 1+2).
- `internal/rangedflavor` — per-pack weapon-style flavor text (dry/unloaded/load/fire moments).
- `internal/scrap` — loot-table + pool utility functions (shared by multiple
  services).
- `internal/mount` — near-leaf mount vocabulary: the temperament ladder
  (war/steady/skittish danger-entry gate) + the `travel` pool-kind identity a
  ridden mount spends; imports only stdlib + leaf `pool`, so mob/entities/
  command/session share it without a cycle.
- `internal/visibility` — per-observer can-see predicate (decoupled via small
  Observer/Target interfaces; composes darkness + concealment + search).
- `internal/property` — registry + tagged-value envelope.
- `internal/light` — pure per-viewer effective-light resolver (level/config/
  resolve/source/fuel/viewer); imports only `world` (terrain) + `gameclock`
  (period names), so call sites gather inputs and it stays a leaf.
- `internal/world` (terrain) — `TerrainOf`/`IsShielded` + terrain vocabulary,
  the shared sky-gate classifier both `weather` and `light` key off.
- `internal/transit` — elevator/subway state machine: car position/door state,
  keyword-exit retarget, on-demand vs scheduled call policy.
- `internal/dotenv` — `.env` file loader, optional local config before any
  env-var read (gitignored; `.env.example` template is committed).

## Foundations (binding conventions, ROADMAP §foundations)
- **F1** `ctx context.Context` first param on anything doing I/O / ticks / cancellable.
- **F2** structured logging = `log/slog` carried on ctx; field names per ROADMAP table.
- **F3** `Clock` interface — engine pkgs never call `time.Now()` directly
  (exceptions: `clock.RealClock`, `account` created_at, the `cmd` binary, RNG
  seeding in `ai`/`spawn`).
- **F4** errors wrap `fmt.Errorf("doing X: %w", err)`; package sentinel `var Err…`.

## Config (env, all `ANOTHERMUD_*`)
`SAVE_DIR`, `CONTENT_DIR`, `ADDR`, `WS_ADDR`, `START_ROOM`, `START_HOUR`,
`PACKS`, `TICK_INTERVAL`, `AUTOSAVE_INTERVAL`, `COMBAT_CADENCE`, `FLEE_COOLDOWN`,
`IDLE_SWEEP_INTERVAL`, `SUSTENANCE_DRAIN_INTERVAL`/`_AMOUNT`, `LINKDEAD_*`,
`MOVE_COST`, `RANGE_FALLOFF`/`_POINT_BLANK_PENALTY`/`_KITE_CHANCE` (ranged bands),
`CORPSE_LIFETIME`/`_OWNERSHIP_WINDOW`, `LOG_FORMAT`/`_LEVEL`, `WS_*`,
`GUIDE_TEMPLATE` (onboarding context).

## Build / test
`go build ./...` · `go run ./cmd/anothermud` (telnet :4000) ·
`go test -race ./...` (race detector mandatory).

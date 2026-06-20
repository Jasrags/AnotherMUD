# AnotherMUD

A modern, spec-driven **MUD (Multi-User Dungeon) engine** written in Go.

AnotherMUD is a from-scratch text-game server: players connect over telnet or
WebSocket, share a tick-driven world of rooms and areas, fight mobs, gain
levels, trade with shops, complete quests, and loot corpses — all driven by
**data + Lua content packs** rather than hardcoded game logic. The engine is
built bottom-up in thin vertical slices against a set of behavior
specifications (`docs/specs/`), which are the source of truth for what each
system does.

> **Status:** well past prototype. Milestones **M0–M29** are complete —
> through **M28** (visibility + hidden exits) and **M29** (player trade:
> escrow/atomic-transaction primitive, direct player-to-player trade, and a
> buyout auction house) — plus movement cost/encumbrance, account-first login
> with a character roster and world-locking, **mounts** (ride/dismount +
> mounted travel), and the in-progress **Wheel-of-Time mechanics EPIC**
> (weapon identity, masterwork grades, ranged combat, two-weapon fighting,
> armor depth, saves, conditions, skills, feats, and The One Power /
> channeling). Content ships as three packs — `core` (engine-baseline
> `tapestry-core`), `starter-world` (the demo village, the default boot), and
> `wot` (a Wheel-of-Time content pack in progress). The engine and specs stay
> **setting-agnostic**; settings live entirely in packs.

---

## What works today

- **Core loop** — a 100 ms tick loop and a typed, cancellable event bus.
- **World** — rooms, areas, exits, **doors & locks**, temporary **portals**,
  **weather**, and an in-game **clock**.
- **Entities & items** — a double-buffered entity store; inventory, equipment
  slots, containers, **stacking**, and keyword resolution (`get 2.sword`).
- **Mobs & AI** — mob templates, area-driven spawning, wander/stationary AI,
  disposition reactions, and **loot tables rolled at spawn**.
- **Combat** — engage/round/hit-miss-damage, flee/wimpy, death, and
  **corpses + looting + autoloot + decay** (M22).
- **Progression** — stats, races, classes, tracks, alignment, training,
  use-based **proficiency**, **abilities**, and **effects**.
- **Crafting** — recipes, crafting proficiency, a quality roll, cooking →
  well-fed, and **gathering** (`forage` + respawning `harvest` nodes) feeding
  **biome**-driven resource tables.
- **Light & darkness** — per-viewer effective light, light sources + fuel,
  render/combat/movement friction, and a persisted in-game clock.
- **Maps** — an ASCII `map`/minimap verb with persisted fog-of-war, terrain
  coloring, and points-of-interest.
- **Economy** — currency, shops, sustenance, rest, and consumables.
- **Trade** — direct player-to-player trade, an escrow/atomic-transaction
  primitive, and a buyout **auction house** (M29).
- **Mounts** — rideable creatures you own; `ride`/`dismount` and mounted
  travel, where the mount becomes the metered mover.
- **Quests**, **social** (tells, channels, emotes, notifications), and a
  **roles & permissions** model with **admin verbs**.
- **Content authoring** — a manifest-driven pack loader and a **sandboxed Lua
  runtime** (gopher-lua) with hot reload.
- **Networking** — telnet with full IAC negotiation, **WebSocket**, **GMCP**
  packages, MSSP, and tiered ANSI color with client capability detection.
- **Accounts & saves** — bcrypt accounts, versioned & migrated player saves, an
  interactive character-creation wizard, and session lifecycle (flood control,
  idle timeout, link-dead reconnect, takeover).

---

## Quick start

**Prerequisites:** Go **1.26+**.

```sh
# clone, then from the repo root:
make run           # build + run the server (telnet on :4000)
# or:
go run ./cmd/anothermud
```

Connect from another terminal:

```sh
telnet localhost 4000
```

At the prompt, enter an **account username**. An existing account asks for
your password, then shows a **character roster** — the account's characters,
each with its world and availability — from which you select an available
character or create a new one (which runs the character-creation wizard). A new
username prompts you to choose and confirm a password (no email), creating an
account with an empty roster. New characters spawn at
`starter-world:town-square` (the demo pack); returning characters land where
they logged out. To boot the Wheel-of-Time pack instead: `make run-wot` (or
`ANOTHERMUD_PACKS=wot ANOTHERMUD_START_ROOM=wot:the-green`).

Try: `look`, `n`/`s`/`e`/`w`, `inventory`, `get <item>`, `consider <mob>`,
`kill <mob>`, `loot`, `help`.

State persists to `./saves/` and survives a restart.

---

## Connecting

| Transport | Default | Enable |
|---|---|---|
| Telnet | `:4000` | always on (`ANOTHERMUD_ADDR`) |
| WebSocket | disabled | set `ANOTHERMUD_WS_ADDR` (e.g. `:4001`), path `ANOTHERMUD_WS_PATH` (`/mud`) |

GMCP and tiered ANSI color negotiate automatically for clients that support
them; plain telnet clients get a clean text fallback.

---

## Configuration

Everything is configured by environment variable — there is no config file.
The most common knobs:

| Variable | Default | Meaning |
|---|---|---|
| `ANOTHERMUD_ADDR` | `:4000` | telnet listen address |
| `ANOTHERMUD_WS_ADDR` | _(empty)_ | WebSocket listen address (empty = off) |
| `ANOTHERMUD_WS_PATH` | `/mud` | WebSocket route |
| `ANOTHERMUD_CONTENT_DIR` | `./content` | content-pack root |
| `ANOTHERMUD_PACKS` | `starter-world` | active packs (+ their dep closure; e.g. `wot`) |
| `ANOTHERMUD_SAVE_DIR` | `./saves` | account/player save root |
| `ANOTHERMUD_START_ROOM` | `starter-world:town-square` | new-character spawn room |
| `ANOTHERMUD_TICK_INTERVAL` | `100ms` | game tick cadence |
| `ANOTHERMUD_AUTOSAVE_INTERVAL` | `30s` | autosave sweep cadence |
| `ANOTHERMUD_COMBAT_CADENCE` | `3s` | combat round interval |
| `ANOTHERMUD_CORPSE_OWNERSHIP_WINDOW` | `60s` | killer-first looting window |
| `ANOTHERMUD_CORPSE_LIFETIME` | `5m` | corpse decay deadline |
| `ANOTHERMUD_LOG_LEVEL` / `ANOTHERMUD_LOG_FORMAT` | `info` / `text` | `slog` level / `text`\|`json` |

`NO_COLOR` disables ANSI color by default. The full list (link-dead timing,
flee cooldown, role seed, default race, …) lives in `loadConfig` in
[`cmd/anothermud/main.go`](cmd/anothermud/main.go).

---

## Project layout

```
cmd/anothermud/      # composition root: wires every service + starts listeners
internal/            # the engine, ~55 focused packages (see below)
content/             # core (engine baseline) + starter-world (demo) + wot packs
docs/                # specs (source of truth), roadmap, backlog, primer
saves/               # runtime account + player saves (git-ignored in practice)
Makefile             # dev tasks
```

`internal/` is organized by layer:

- **Foundations** — `tick`, `eventbus`, `clock`/`gameclock`, `logging`,
  `persistence`, `srckey`
- **World & things** — `world`, `entities`, `item`/`mob`/`slot`, `keyword`,
  `spawn`, `ai`, `portal`, `weather`, `property`, `biome`, `light`
- **Character mechanics** — `stats`, `progression`, `combat`, `effect`
- **Action & interaction** — `command`, `economy`, `quest`/`queststore`/
  `questwatch`, `loot`, `corpse`, `crafting`/`recipe`, `gathering`, `campfire`
- **Player lifecycle** — `account`, `player`, `login`, `session`, `wizard`
- **Social** — `chat`, `notifications`, `emote`
- **Presentation** — `render`, `ansi`, `help`, `decoration`, `stacking`
- **Networking** — `conn` (`telnet`/`ws`), `server`, `gmcp`, `mssp`
- **Content & scripting** — `pack`, `script`, `scripting`

---

## Content packs

Game content is data, not code. A pack is a directory under
`content/` with a `pack.yaml` manifest pointing at YAML files for rooms, areas,
items, mobs, classes, races, abilities, loot tables, quests, help topics,
themes, weather zones, and Lua `scripts/`. The loader discovers packs,
resolves dependencies, validates references, and registers everything at boot.

Three packs ship today: `core` (the engine-baseline `tapestry-core` namespace —
slots, races, classes, abilities, effects, theme, help), `starter-world` (the
demo village, the default boot), and `wot` (a Wheel-of-Time content pack in
progress, depends on `core`). A boot selects active packs via `ANOTHERMUD_PACKS`
(their dependency closure is pulled in automatically). Ids are namespaced
(`starter-world:town-square`); unqualified ids in YAML resolve against the
current pack. Edit pack files and restart to see changes (Lua scripts also
support hot reload via the admin `reload` verb).

---

## Development

```sh
make build      # compile into ./bin/anothermud
make run        # build + run
make watch      # live-reload: rebuild + restart on save (needs air)
make test       # go test -race -count=1 ./...   (race detector is mandatory)
make cover      # coverage profile + summary
make check      # fmt + vet + test — the gate to run before committing
make worldmap   # render docs/maps/world.html (interactive content map)
make help       # list all targets
```

**Live reload (`make watch`).** Watches `.go` / `.yaml` / `.lua` and rebuilds +
restarts the server on save (~1s) so you don't manually stop/start during
playtesting. Go is compiled, so this is rebuild+restart, not in-process
hot-swap — connections drop, but player saves persist (the server flushes on
the graceful SIGINT), so just reconnect. Pack Lua can also be hot-reloaded
in-session via the admin `reload` verb (no restart). Requires
[air](https://github.com/air-verse/air): `go install github.com/air-verse/air@latest`.
Server env vars are inherited from your shell — export them before `make watch`.

Conventions the codebase follows (see [`docs/ROADMAP.md`](docs/ROADMAP.md)
"Foundations"):

- `context.Context` is the first parameter on anything that does I/O, ticks, or
  is cancellable.
- Structured logging via `log/slog`, logger carried on `ctx`.
- Errors wrap with `fmt.Errorf("doing X: %w", err)` + package-level sentinels.
- Engine packages never call `time.Now()` directly — they read a `Clock`.
- Tests run under `-race`; new work targets 80%+ coverage.

---

## Documentation

| Doc | What it is |
|---|---|
| [`docs/specs/`](docs/specs/) | **Behavior specifications — the source of truth** (59 specs; read `docs/specs/README.md` first) |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | Milestone done-log + foundations/conventions |
| [`docs/BACKLOG.md`](docs/BACKLOG.md) | Open work + candidate next themes |
| [`docs/DEFERRED-BACKLOG.md`](docs/DEFERRED-BACKLOG.md) | Index of deferred fixes across milestones |
| [`docs/PRIMER.md`](docs/PRIMER.md) | Pasteable orientation for external design work |
| [`CLAUDE.md`](CLAUDE.md) | Guidance for AI-assisted development in this repo |

When implementing a feature, **read the relevant spec first** — specs describe
behavior, the ROADMAP tracks status, and the BACKLOG tracks the gap.

---

## Architecture in one paragraph

A single **tick loop** (`internal/tick`) drives time; systems register handlers
at a cadence and communicate through a typed, cancellable **event bus**
(`internal/eventbus`) rather than calling each other directly. Content is loaded
once at boot into per-system **registries** (`internal/pack`); live state lives
in a double-buffered **entity store** (`internal/entities`) plus per-session
actors (`internal/session`). The **composition root** in `cmd/anothermud`
constructs and wires every service. This keeps systems decoupled, testable
under the race detector, and extensible by content packs.

---

## License

No license file is present yet. Until one is added, all rights are reserved —
add a `LICENSE` before any public or external use.

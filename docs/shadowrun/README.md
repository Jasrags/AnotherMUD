# ShadowMUD

A text-based, multiplayer MUD (Multi-User Dungeon) set in the world of
**Shadowrun**, written in Go and played over SSH. Players connect with any SSH
client, create a runner, and explore a shared world driven by Shadowrun's
tabletop rules for attributes, skills, magic, cyberware, and gear.

> ⚠️ **Work in progress.** ShadowMUD is an early-stage hobby project. APIs, data
> formats, and game systems are still changing frequently.

## Features

- **SSH-based gameplay** — connect with a standard SSH client; no custom client required.
- **User accounts** — registration, login, ban handling, and bcrypt-hashed passwords.
- **Character creation** — multiple runner roles (Face, Spellcaster, Decker, Technomancer, Rigger, Street Samurai) and metatypes (Human, Elf, Dwarf, Ork, Troll).
- **Shadowrun rule systems** — attributes, skills and skill groups, qualities, magic and traditions, cyberware/bioware, weapons and armor, contacts, and more.
- **Data-driven content** — characters, items, skills, qualities, and world geography are defined in editable YAML under `_data/`.
- **Hot-reloading data** — file watching via `fsnotify` for iterating on content.
- **Data tooling** — `cmd/validate` and `cmd/loaddata` for validating and loading game data.

## Game Systems & Roadmap

ShadowMUD models the Shadowrun tabletop ruleset. The systems below are tracked
against their current implementation state. Design notes for most of these live
in [`docs/`](docs/).

### ✅ Built (working)

- **SSH server & sessions** — connection handling, session management, idle timeout, and per-session message channels.
- **Account flow** — banner, registration, login, ban checks, password change, and a main menu (enter game, create / list / delete character).
- **Character creation flow** — guided steps: name → metatype → magic/resonance type → attribute point buy → skills → qualities → spells → adept powers → complex forms → nuyen spend.
- **Character model** — full character struct integrating attributes, skills, qualities, cyberware/bioware, weapons, armor, powers, spells, contacts, licenses, and martial arts.
- **Metatypes** — Human, Elf, Dwarf, Ork, Troll with attribute min/max/augmented-max, essence, and racial traits (YAML-loaded).
- **Skills & skill groups** — 100+ active and knowledge skills with linked attributes and specializations (`common/skill`).
- **Qualities** — 100+ positive/negative qualities with prerequisites, modifiers, and karma costs (`common/quality`).
- **Weapons** — extensive melee and ranged weapon catalog with mods and ammo (`common/weapon`).
- **Armor** — armor specs with rating, capacity, and modifications (`common/armor`).
- **Shared mechanics** — attributes with modifiers, dice rolling (`utils/dice`), bcrypt auth, YAML/JSON loaders, and terminal color/prompt helpers.

### 🚧 In progress / partially built

- **Cyberware & bioware** — type/grade/essence modeling exists; YAML content is still sparse.
- **Magic & resonance** — magic types, spells, traditions, metamagic, complex forms, adept powers, echoes, and paragons are modeled but not yet fully wired into play.
- **Rooms / areas / zones** — managers exist; loading world geography from YAML is being integrated.
- **Build/cost system** — `common/character/build.go` covers karma/point costs for skills and qualities; several rules are still commented out.
- **In-game commands** — a command dispatcher exists with early commands (e.g. `say`, `look`).
- **Contacts & vehicles** — data structures are defined; managers and content are minimal.

### 📋 Planned (designed, not yet implemented)

- **Game loop / tick system** — `core/tick.go` and `core/world.go` are drafted but not yet driving simulation.
- **Movement & navigation** — moving between rooms via exits/directions.
- **Combat** — initiative, attack/defense resolution, damage, condition monitors, firing modes, called shots, and melee/environmental modifiers (see [docs/TESTS.md](docs/TESTS.md), [docs/MODIFIERS.md](docs/MODIFIERS.md)).
- **Test mechanics** — opposed tests, composure, judge intentions, lifting/carrying, memory, and matrix/astral initiative (see [docs/ROLLS.md](docs/ROLLS.md), [docs/TESTS.md](docs/TESTS.md)).
- **Full magic system** — spellcasting, summoning, binding, alchemy, and spirit data.
- **Matrix / decking** — hacking, programs, and resonance gameplay beyond complex form definitions.
- **NPCs / critters** — critter categories are defined; behavior and AI are not yet implemented.
- **Equipment use** — equipping, wielding, and applying gear effects in play.
- **Expanded content** — full cyberware, bioware, vehicle/drone, and gear catalogs.
- **Custom character creation path** — alternative to the guided flow.

## Requirements

- [Go](https://go.dev/) 1.23 or newer
- An SSH client (e.g. OpenSSH `ssh`)

## Getting Started

Clone and build:

```bash
git clone https://github.com/Jasrags/ShadowMUD.git
cd ShadowMUD
go build ./...
```

Run the server:

```bash
go run .
```

By default the server listens on `localhost:33333` (see [Configuration](#configuration)).

Connect from another terminal:

```bash
ssh localhost -p 33333
```

## Configuration

Server settings are read from `config.yaml` in the project root. Key sections:

| Section      | Purpose                                                         |
|--------------|-----------------------------------------------------------------|
| `server`     | Host, port, log level, idle timeout, max connections            |
| `user`       | Registration/login toggles, username & password length rules    |
| `character`  | Character creation toggles, name length, max characters per user |
| `banned_names` | Reserved names that cannot be used                            |
| `terminal`   | Terminal display settings (e.g. word wrap)                      |
| `data_files` | Paths to areas, characters, and users data directories          |

Example (abbreviated):

```yaml
server:
    host: localhost
    port: 33333
    log_level: DEBUG
    idle_timeout: 30m
    max_connections: 100
```

## Project Layout

```
.
├── main.go              # Entry point: build world, load config/data, start SSH server
├── world.go             # World state, SSH server, session handling
├── commands.go          # In-game command handling
├── config.yaml          # Server configuration
├── cmd/
│   ├── validate/        # Tool to validate game data
│   └── loaddata/        # Tool to load game data
├── common/              # Domain packages (game systems)
│   ├── character/       #   character model & build rules
│   ├── skill/           #   skills & skill groups
│   ├── quality/         #   positive/negative qualities
│   ├── cyberware/       #   cyberware & modifications
│   ├── weapon/          #   weapons, ammo, modifications
│   ├── magic/ spell/    #   magic, spells, traditions
│   ├── room/ area/      #   world geography
│   ├── user/            #   user accounts
│   └── ...              #   contacts, vehicles, mentors, etc.
├── core/                # World tick / simulation loop
├── utils/               # Dice, color, YAML/JSON, auth, prompt helpers
├── docs/                # Game system design notes (rules, creation, gear)
└── _data/               # YAML game data (rooms, areas, skills, qualities, ...)
```

## Data Tools

The `cmd/` directory contains helper utilities for working with game data:

```bash
# Validate game data files
go run ./cmd/validate

# Load game data
go run ./cmd/loaddata
```

## Testing

```bash
go test ./...
```

## Documentation

Design notes for the game's rule systems live in [`docs/`](docs/), including
[character creation](docs/CREATION.md), [skills](docs/ACTIVE.md),
[cyberware](docs/CYBERWARE.md), [weapons](docs/WEAPONS.md),
[armor](docs/ARMOR.md), and more.

## License

See [LICENSE](LICENSE) if present; otherwise this project is currently
unlicensed and provided as-is.

## Disclaimer

Shadowrun is a trademark of The Topps Company, Inc. This is a non-commercial fan
project and is not affiliated with or endorsed by the rights holders.

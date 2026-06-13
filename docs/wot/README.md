# WheelMUD

A Wheel of Time MUD server written in Go. Telnet on TCP `:2323`, SQLite-backed
world + accounts, mode-stack command dispatcher, ANSI/cfmt rendering with
width-aware reflow.

> **Status: pre-alpha but broadly playable.** A character can be created
> through full WoT chargen, walk a hierarchical authored world, fight in
> d20 combat, level up, channel the One Power, shop, bank, talk to NPCs,
> run quests, and chat across channels. The systems breakdown below tracks
> what is built, what is actively in progress, and what is planned.
> [`ROADMAP.md`](ROADMAP.md) is the authoritative, line-by-line status;
> [`docs/PLAN.md`](docs/PLAN.md) is the sequenced plan of attack.

## Systems

Legend: ✅ built · 🚧 in progress / partial · 📋 planned

### ✅ Built

| System | What ships today |
| ------ | ---------------- |
| **Network & protocol** | TCP telnet listener, full IAC negotiation (`WILL/WONT/DO/DONT`, `SB…SE`, `IAC IAC` escape), `TERM_TYPE` (RFC 1091) + `NAWS` (RFC 1073) subnegotiation, **GMCP** (V1 Char/Room/Comm frames), **MSSP** crawler status, **CHARSET/UTF-8** negotiation (RFC 2066), drop-in **Mudlet client package** (`make mudlet-package`) |
| **Terminal & rendering** | ANSI SGR + `cfmt` styling, color-level detection (`None`/`16`/`256`/truecolor with downsampling), NAWS-driven width/height, ANSI-aware + width-aware word-wrap (CJK fullwidth, combining marks, long-token break), pager mode, per-character prompt templating (`%h/%H/%r/%g`) |
| **Input & line editing** | Byte-at-a-time read loop, backspace/DEL, verb + argument tab-completion (auth-filtered), password-mode echo masking, quoted-argument tokenization, command history (↑/↓), in-line cursor movement (Home/End/Ctrl-A/E/U/W/K), user aliases, `;` multi-command lines |
| **Command system** | Registry with aliases / prefix lookup / ambiguity detection, per-session mode-stack dispatcher, `AuthLevel` enforcement (non-enumerating denials), per-character persisted auth level, per-command argument completers |
| **Accounts, auth & characters** | Account model + bcrypt hashing, login mode with lockout, account-create mode, **post-login account menu** (play/select/new/delete, password change, settings, security/login-history + session kick), character select/create, **full WoT chargen** (abilities point-buy → background → class → channeler branch + affinities/weaves → heroic characteristics → feats/skills → starting equipment auto-equip), single-session-per-account registry, connect splash + per-character MOTD/news |
| **Persistence** | SQLite via pure-Go `modernc.org/sqlite`, embedded forward-only migrations (0001–0057), typed repos with sqlite + memory impls and shared contract tests, hierarchical YAML world loader with additive resync on boot, periodic + shutdown autosave, scheduled `VACUUM INTO` backup rotation with retention pruning |
| **Game loop & scheduling** | 1 Hz tick scheduler with named pulse buckets (Save/Combat/Regen/AreaReset/…), typed event bus (sync + async), delayed/scoped actions, graceful signal-driven shutdown with session drain |
| **World model** | Rooms (flags / sector / light / coords / extra-descs / day-night clock + ambient broadcasts), exits (closed/locked/pickable/hidden/keys/lock-difficulty), zones (level range + reset pipeline: mob/door/item respawn), full item taxonomy with typed polymorphic stats (weapon/armor/shield/container/consumable/light/key/tool…), recursive container nesting, equipment slots, four-denomination currency |
| **Movement & navigation** | `look` (room/items/mobs/exits + extra-desc nouns), cardinal + vertical + diagonal movement, doors/locks/keys (`open`/`close`/`lock`/`unlock`/`pick`), BFS minimap + zonemap, auto-coords, `track`, `time` |
| **Combat (Phases D + L)** | d20 initiative / hit-miss / damage / crit, threat tables, mob death + loot + gold + XP, player death / respawn with XP debt + bind rooms, PvP opt-in, parties + `follow` + shared XP split, evasion, combat feats, iterative attacks |
| **Progression & affects (Phase E)** | d20 XP curve + level-up math (to level 20), trainer NPCs, `learn`/`feat`/`bump`/`learn weave` spend verbs, mid-game weave teachers + practice points, channeler slot refresh + madness accrual, affects/buffs/DoT/HoT with stacking caps + multi-charge consumables, per-verb command lag + per-skill cooldowns |
| **Communication** | `say`, `tell`/`reply`, `shout`, `yell`, chat channels, social/emote catalog (`:` freeform + per-social verbs) |
| **Inventory & economy** | Inventory + encumbrance (transitive through container multipliers), wear/wield equipment, shops (`list`/`buy`/`sell`/`value` + restock), bankers (`balance`/`deposit`/`withdraw` with hour gate) |
| **Quests, scripts & NPC behavior (Phase F)** | NPC dialogue trees, per-character quest engine (talk/kill/reach/script steps with XP + coin rewards), declarative trigger system (`on_enter`/`on_say`/`on_attack`/`on_death`/`on_tick`), sandboxed embedded Lua (50 ms ctx cap, fault budget, say/emote/log/quest/push_mode/apply_affect/give_item/target/room/clock APIs), authored mob paths + pathfinding |
| **Admin & moderation** | `spawn`/`teleport`/`goto`/`transfer`/`summon`/`wizinvis`/`shutdown`/`reboot`/`affect`/`dispel`/`cooldown`/news authoring, per-zone builder grants (`grant`/`revoke`/`grants`), `redit` room editor, append-only `admin_audit` log (one row per successful privileged verb) |
| **Ops, CI & packaging (Phase J)** | YAML + env config loader, per-character command audit, Prometheus metrics + pprof + healthz on a private listener, GitHub Actions matrix + nightly fuzz (IAC parser / tokenizer), telnet integration smoke, goreleaser + multi-arch Docker image + hardened systemd unit |
| **Flow engine (Phase O.0–O.4)** | Generic multi-step interactive Flow runner (text/choice/confirm + number/multi-select/point-buy/conditional/action steps), YAML catalog with `FLOW_DIR` override, SQLite-persisted state with resume-on-reconnect, Go-only action/validator registry, `oedit` item-template editor as the first production consumer |

### 🚧 In progress / partial

- **Online creation (OLC) via the Flow engine** — `redit` (room) and
  `oedit` (item) shipped; `zedit` (O.5) and `medit` (O.6) are next.
- **Chargen + account-create migration to the Flow engine** (O.7 / O.8) —
  retiring the hand-rolled mode code in favor of YAML flows.
- **Flow hot-reload** — `reload flows` admin verb (O.9), joining the
  existing `reload socials` / `reload help`.
- **Equipment slots** — V1 wear/wield works; two-handed/double weapons,
  cloak/backpack/worn-misc/belt-pouch slots, and wear-requirements pending.
- **Hot-reload of area files** — additive resync on boot ships; `reload
  world` verb, in-place updates, soft-delete of vanished rows, and an
  `fsnotify` watcher are pending.
- **Channeling depth** — slot/refresh/madness/embrace/still landed;
  circle linking, a'dam bind/unbind, Warder bond, and angreal/sa'angreal
  boosts pending.
- **Login hardening** — per-account lockout works; per-connection rate
  limiting / exponential backoff pending.

### 📋 Planned

- **Network breadth** — MCCP2/3 compression, MSDP, MXP, a TLS listener,
  a WebSocket gateway for browser clients, and an optional SSH admin shell.
- **Mail / notes / bulletin boards** — plus the multi-line editor mode
  they share with OLC `desc` editing.
- **Email verification / password reset** — optional email column,
  single-use token flow, pluggable mail sender.
- **User-defined aliases persisted on the character** (promoting the
  in-memory alias table to a table).
- **Versioned area saves** — diff/preview before commit, revert, optional
  git export.
- **Crafting (Phase K)** — recipes, materials, stations, quality tiers
  (high-level design only today).
- **Strategic / Tapestry-parity items (Phase N)** — an uncommitted option
  set; promoted into a real phase only when chosen.

## Quick start

```bash
make build/server          # go build -o /tmp/bin/server cmd/server/main.go
make run/server            # build then run
make run/live/server       # hot reload via cosmtrek/air
docker compose up          # build + run, exposes :2323
go test -race ./...        # full test suite
```

Connect:

```bash
telnet localhost 2323
# or
nc localhost 2323
```

## Configuration

Two paths, both optional and stackable: pass `-config <path>` to load a
YAML file (see [`config.example.yaml`](config.example.yaml)) and/or
export environment variables (see [`.env.example`](.env.example)). Env
overrides file values; both fall back to package defaults.

| Var                        | Default            | Purpose                                                      |
| -------------------------- | ------------------ | ------------------------------------------------------------ |
| `LISTEN_ADDR`              | `:2323`            | TCP listen address                                           |
| `METRICS_ADDR`             | `127.0.0.1:9090`   | Prometheus + pprof + healthz HTTP bind; empty disables       |
| `DB_DSN`                   | `wheelmud.db`      | SQLite DSN; `:memory:` for ephemeral runs                    |
| `BACKUP_DIR`               | _(empty)_          | When set, scheduled `VACUUM INTO` snapshots land here        |
| `LOG_LEVEL`                | `debug`            | `debug` / `info` / `warn` / `error`                          |
| `WORLD_DIR`                | `./data/world`     | YAML zone tree the world loader syncs into the DB            |
| `AUDIT_COMMANDS_ENABLED`   | `false`            | Per-character command audit log to `character_audit` table   |
| `AUDIT_COMMANDS_EXCLUDE`   | _(empty)_          | Comma-separated verb filter when audit is enabled            |

Catalog dirs (`CHARGEN_DIR` / `QUEST_DIR` / `SCRIPT_DIR` / `EFFECTS_DIR`)
remain env-only and switch each embedded-FS catalog to an on-disk
override. Run `wheelmud-server -version` for the build triple stamped
by goreleaser ldflags.

## Layout

```
cmd/server/           entrypoint — wires repos, catalogs, registry, tickers, dispatcher
telnet/               protocol, session, registry, mode stack, color, wrap, alias, history
internal/cmd/         concrete verbs (look/move/say/tell/shout/channel/who/examine/door/
                      inventory/equipment/shop/banker/quaff/attack/flee/parry/pvp/group/
                      follow/score/xp/train/learn/feat/bump/embrace/release/affects/
                      cooldowns/talk/quest/spawn/teleport/goto/transfer/summon/wizinvis/
                      shutdown/reboot/map/zonemap/coords/track/time/news/whereami/zones/
                      affect/dispel/cooldown)
internal/mode/        login, character_select, character_create, account_menu, postauth,
                      game, dialogue
internal/repo/        account, character, room, exit, item, mob_template, mob_instance,
                      mob_trail, zone, channel, news, shop, banker, trainer, weave_teacher,
                      trigger, admin_audit, character_audit, account_login
internal/db/          sql.DB open + embedded migrations 0001–0057
internal/world/       YAML loader + sync to DB; Restocker, ZoneResetter, Clock, day/night
internal/chargen/     YAML content catalog (backgrounds/classes/feats/skills/weaves)
internal/news/        embedded MOTD/news catalog
internal/effects/     YAML affect catalog (HoT/DoT/buffs feeding §E #25)
internal/affects/     per-tick session driver, stacking, expiry events
internal/combat/      d20 hit/damage, initiative, threat, group XP split, death pipeline
internal/group/       in-memory party manager (invite/accept/leave/kick/disband)
internal/progression/ XP curve + per-class level-up math (pure functions)
internal/channeling/  slot refresh + madness tick (Phase E #27)
internal/dialogue/    NPC dialogue tree model + validator
internal/quest/       per-character quest engine (talk/kill/reach/script steps)
internal/flow/        generic multi-step Flow engine + YAML catalog (Phase O); oedit consumer
internal/trigger/     declarative event registry + dispatcher (on_enter/say/attack/death/tick)
internal/scripts/     embedded Lua script catalog (one *.lua per file)
internal/lua/         sandboxed gopher-lua runner (LState pool, 50ms ctx cap, V1+V2+V3 APIs)
internal/audit/       append-only admin/account audit row writer
internal/session/     single-session-per-account registry
internal/eventbus/    typed pub/sub
internal/persist/     periodic + shutdown autosave manager
internal/tick/        scheduler + named buckets (Save, Combat, Regen, Affects, AreaReset, ...)
internal/safego/      panic-recovery goroutine wrapper
internal/auth/        bcrypt password hashing
internal/config/      YAML + env config loader (Phase J slice J2)
internal/metrics/     Prometheus + pprof + healthz on a private HTTP listener (J5)
internal/backup/      scheduled VACUUM INTO snapshots + retention pruning (J4)
internal/creature/    Core stat block, Channeling weave model, Equipment slot map
internal/currency/    copper-piece amount type
test/integration/     subprocess-based telnet smoke (build-tag `integration`, J6)
data/world/           authored zone YAML — hierarchical (continent/nation/region/settlement/
                      building); Emond's Field is the reference. See data/world/README.md.
deploy/               systemd unit + deploy/README.md ops runbook (J7)
docs/CODEMAPS/        token-lean architecture maps for AI context
docs/PLAN.md          sequenced plan of attack across roadmap phases
docs/reference/       game-system reference docs (abilities, classes, ...)
```

## Documentation

- [`CLAUDE.md`](CLAUDE.md) — guidance for Claude Code agents
- [`ROADMAP.md`](ROADMAP.md) — feature punch list + status
- [`docs/PLAN.md`](docs/PLAN.md) — sequenced plan of attack across roadmap phases
- [`docs/CODEMAPS/`](docs/CODEMAPS/) — architecture, command catalog, data model, dependencies, telnet protocol
- [`docs/reference/`](docs/reference/) — game-system rules ported from the WoT RPG
- [`data/world/README.md`](data/world/README.md) — zone YAML schema, room ID conventions, currency format, item taxonomy
- [`deploy/README.md`](deploy/README.md) — Docker + systemd deployment runbook
- [`config.example.yaml`](config.example.yaml) / [`.env.example`](.env.example) — full configuration surface
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — dev workflow, testing, commit conventions

## License

See [`LICENSE`](LICENSE).

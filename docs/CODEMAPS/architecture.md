<!-- Generated: 2026-07-17 | Go files scanned: ~450 (+550 tests) | 78 internal pkgs | Token estimate: ~790 -->

# Architecture

Single Go binary (module `github.com/Jasrags/AnotherMUD`, go 1.26). Tick-driven
MUD engine: one game loop + a typed event bus; everything else is layered
`internal/` packages (78 today). **No web frontend / DB / HTTP routes today** — clients are
telnet/WebSocket line connections (a browser web UI over the existing WS+GMCP
channel is the recorded long-term rich-client direction — docs/BACKLOG.md);
state is YAML save files + content packs (4 packs: core, shadowrun, starter-world, wot).

## Entry point
`cmd/anothermud/main.go` — composition root. Opens account/player stores
(`ANOTHERMUD_SAVE_DIR`), loads content via `pack.Load` (`ANOTHERMUD_CONTENT_DIR`),
wires every service, registers tick handlers, runs the telnet listener
(`ANOTHERMUD_ADDR`) + optional WS (`ANOTHERMUD_WS_ADDR`). Start room
`ANOTHERMUD_START_ROOM` (default `starter-world:town-square`).

## Layer stack (bottom-up; deps point down)
```
Foundations   tick, eventbus, clock+gameclock, logging, persistence, srckey,
              pool (generalized resource pools), karma (spendable advancement),
              mount (near-leaf: temperament ladder + travel-pool identity)
World/things  world (rooms/exits/doors + load-time area-local room
              coordinate derivation), entities, item/mob/slot, keyword,
              spawn, ai, portal, weather, biome, property, corpse,
              light (per-viewer effective-light resolver: terrain sky-gate ·
              room override · lit sources · darkvision floor),
              visibility (per-observer can-see: hide/sneak/invis/search),
              transit (elevators/subways: conveyor machine + per-room door gate)
Mechanics     stats, progression, combat, effect, condition (status conditions),
              feat (player-chosen perks), channel (derived-stat formula layer),
              security (heat / wanted-level / patrol response),
              faction (per-character standing), reputation (renown axis)
Action        command (registry+dispatch+§5 typed args), economy,
              escrow (atomic-transaction primitive + trade audit log),
              trade (direct player-to-player swap), auction (async marketplace),
              crafting/recipe/campfire, gathering, grade (item quality grades),
              quest*, questspawn (quest-scoped mobs/items), loot, decoration,
              stacking, action (busy-state / don-doff / reload gate),
              guard (per-actor state-machine supervisor), size (wielding modes),
              visibility (sight lines), rangedflavor (weapon-style messaging),
              mounts (ride/stable verbs in command; service in cmd)
Lifecycle     account, player, login, session, wizard
Social        chat, notifications, emote
Presentation  render, ansi, help
Networking    conn{telnet,ws}, server, gmcp, mssp
Content       pack (manifest/loader), script, scripting (gopher-lua sandbox),
              scrap (loot-table & pool utilities)
Test infra    telnettest (send/expect telnet driver + GMCP frame capture)
```

## Core data flow
```
telnet/ws conn ─▶ session.Actor ─▶ command.Registry.Dispatch
                                        │ resolve verb (exact→prefix)
                                        │ §5 arg-typing (or HandParsed=raw)
                                        ▼
                                   Handler ──▶ services (combat, economy,
                                        │        progression, quest, karma …)
                                        ▼        └▶ entities.Store / world
                                   eventbus.Publish ─▶ subscribers
                                                       (questwatch, ai,
                                                        gmcp flushers, scripting)
tick.Loop (100ms) ─▶ registered handlers: combat round, autosave, idle-sweep,
                     effect ticks, gmcp flush, regen, fuel-burn, prompt render,
                     security patrols (heat decay / wanted spawn), transit moves,
                     action completion, mount travel regen
gameclock (tick-driven) ─▶ time.period.change ─▶ weather ambience +
                     light transitions (per-viewer darken/brighten)
```

## Companion docs
- `backend.md` — tick/event/command/service flow + key files.
- `presentation.md` — transports, GMCP, render/ansi tiers, web-client packages.
- `data.md` — save surface (v39) + content packs (registries, load order).
- `dependencies.md` — external libs + foundations conventions.
- Specs are the behavior source of truth: `docs/specs/README.md` (canonical order).
  Roadmap/backlog: `docs/ROADMAP.md` (done-log, M0–M30 + WoT EPIC), `docs/BACKLOG.md`.

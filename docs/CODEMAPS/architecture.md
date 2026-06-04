<!-- Generated: 2026-06-04 | Go files scanned: 254 (+237 tests) | ~56k LOC | 51 internal pkgs | Token estimate: ~700 -->

# Architecture

Single Go binary (module `github.com/Jasrags/AnotherMUD`, go 1.26). Tick-driven
MUD engine: one game loop + a typed event bus; everything else is layered
`internal/` packages. **No web frontend / DB / HTTP routes** — clients are
telnet/WebSocket line connections; state is YAML save files + content packs.

## Entry point
`cmd/anothermud/main.go` — composition root. Opens account/player stores
(`ANOTHERMUD_SAVE_DIR`), loads content via `pack.Load` (`ANOTHERMUD_CONTENT_DIR`),
wires every service, registers tick handlers, runs the telnet listener
(`ANOTHERMUD_ADDR`) + optional WS (`ANOTHERMUD_WS_ADDR`). Start room
`ANOTHERMUD_START_ROOM` (default `tapestry-core:town-square`).

## Layer stack (bottom-up; deps point down)
```
Foundations   tick, eventbus, clock+gameclock, logging, persistence, srckey
World/things  world, entities, item/mob/slot, keyword, spawn, ai, portal,
              weather, property, corpse
Mechanics     stats, progression, combat, effect
Action        command (registry+dispatch+§5 typed args), economy, quest*,
              loot, decoration, stacking
Lifecycle     account, player, login, session, wizard
Social        chat, notifications, emote
Presentation  render, ansi, help
Networking    conn{telnet,ws}, server, gmcp, mssp
Content       pack (manifest/loader), script, scripting (gopher-lua sandbox)
```

## Core data flow
```
telnet/ws conn ─▶ session.Actor ─▶ command.Registry.Dispatch
                                        │ resolve verb (exact→prefix)
                                        │ §5 arg-typing (or HandParsed=raw)
                                        ▼
                                   Handler ──▶ services (combat, economy,
                                        │        progression, quest, …)
                                        ▼        └▶ entities.Store / world
                                   eventbus.Publish ─▶ subscribers
                                                       (questwatch, ai,
                                                        gmcp flushers, scripting)
tick.Loop (100ms) ─▶ registered handlers: combat round, autosave, idle-sweep,
                     effect ticks, gmcp flush, regen, prompt render
```

## Companion docs
- `backend.md` — tick/event/command/service flow + key files.
- `presentation.md` — transports, GMCP, render/ansi tiers.
- `data.md` — save surface + content packs (registries, load order).
- `dependencies.md` — external libs + foundations conventions.
- Specs are the behavior source of truth: `docs/specs/README.md` (canonical order).
  Roadmap/backlog: `docs/ROADMAP.md` (done-log, M0–M22), `docs/BACKLOG.md`.

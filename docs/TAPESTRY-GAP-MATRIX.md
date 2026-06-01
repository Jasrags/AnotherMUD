# Tapestry → AnotherMUD Gap Matrix

A point-in-time audit of what Tapestry (the previous C#/.NET incarnation, symlinked at
`./tapestry`) implements vs. what our `docs/specs/` capture vs. what the Go codebase has
actually built. Use this as the **starting board** for "what should we build next?" or
"what should we spec next?" conversations.

**Snapshot:** 2026-05-29 (end-of-M11 + M12.3; cluster 1, cluster 2, ItemInstance mutex
landed; help-discoverability refactor landed today).

> **Stale — pending full re-survey.** Since this snapshot, all five themes have shipped:
> A (Social MUD / M13), B (Modern Client / M16), C (World Depth / M15),
> D (Content Authoring / M17), E (Engine Debt / M14). The per-item statuses below
> (e.g. §1.1 scripting "Blocked", §1.2 GMCP "zero packages built", §1.3 telnet
> negotiation "incomplete") are **superseded** — those systems now exist. Re-run the
> refresh procedure below to rebuild an accurate board before using it to pick the next
> theme. Until then, cross-check any item against `docs/ROADMAP.md` and the
> `m13`–`m17` `*-deferred-fixes.md` memory files.

**Refresh procedure:** Re-run two parallel surveys — one to inventory Tapestry's feature
surface (Engine subdirs + Scripting modules + Server modules + Networking), one to map
each `docs/specs/*.md` to its Go implementation status via ROADMAP checkboxes and the
`m<N>-deferred-fixes.md` memory files. Synthesize. The prompts for both agents are
preserved in this session's git log if needed.

**Companion docs:**
- `docs/DEFERRED-BACKLOG.md` — open intra-spec deferrals (M0→M12)
- `docs/ROADMAP.md` — milestone plan with `[x]/[ ]` acceptance boxes
- `docs/specs/README.md` — spec index + cross-cutting tables (events, registries,
  save/load surface, tick handlers)

---

## How to read this doc

The matrix is split into four buckets:

1. **Big-ticket missing systems** — the small set with the most leverage. Discussion
   should start here.
2. **Specced but unbuilt in Go** — finite, milestone-shaped, anchored in ROADMAP/specs.
3. **In Tapestry but not specced yet** — needs a spec slice before it can land.
4. **Operational / ops gaps** — observability, container build, repo hygiene.

For each item we include, where useful: a Tapestry source pointer (so we can study the
reference), the Go package it would land in (so we know where to put it), and the
nearest spec section (or "no spec" if §3 territory).

---

## 1. Big-Ticket Missing Systems

Ordered by leverage. These are the conversations to have before scoping anything else,
because choices here ripple through many smaller items.

### 1.1 Scripting runtime

| | |
|---|---|
| **Status** | Blocked — ROADMAP M2 explicitly defers the language choice |
| **Spec** | `scripting-and-packs.md` — language-agnostic; describes pack discovery, two-phase load, namespace, registry scoping, and engine API surface obligations |
| **Tapestry choice** | Jint (JavaScript) — see `src/Tapestry.Scripting/JintRuntime.cs`, `JintApiModule.cs`, ~48 modules under `src/Tapestry.Scripting/Modules/` |
| **Go state** | All packs are data-only YAML; no script execution path |
| **Unblocks** | quest hook scripts, ability resolver custom, mob AI override behaviors, flow extensions beyond new-player wizard, schedule primitive, admin verbs, content-driven combat phases, content-defined login gates |
| **Discussion frame** | Three real candidates: (a) `gopher-lua` — most mature, mature MUD-side history; (b) `goja` — JS, large mindshare, Tapestry-portable scripts; (c) Starlark — Python-ish, capability-safe by design, smaller ecosystem. Wasm is a fourth but high-friction for content authors. Pick by **content-author ergonomics**, not engine ergonomics — they'll write 100× more script than we write embeddings. |
| **Open Qs** | Sandbox model (memory cap, instruction cap, no I/O without engine API), error attribution to file:line, hot-reload story, how scripts subscribe to engine events (callbacks vs. coroutines), per-pack vs. per-script execution context |
| **Sequencing** | Don't pick until 2-3 concrete scripted features are listed and a pack author has reviewed the choice with them. Building a runtime in search of a use case is the trap. |

### 1.2 GMCP package layer

| | |
|---|---|
| **Status** | Transport seam specced (`networking-protocols §5`); zero packages built |
| **Spec** | `networking-protocols.md` explicitly non-goals "the shape and semantics of specific GMCP packages" |
| **Tapestry packages** | `Char.Status`, `Char.Vitals` (batched dirty updates), `Char.Combat`, `Char.Items`, `Char.Effects`, `Char.Experience`, `Char.Commands`, `Comm`, `Room`, `World`, `Display`, `Login`, `Quest`, `Notification` — handlers under `src/Tapestry.Server/Gmcp/Handlers/` |
| **Go state** | No GMCP at all (telnet only, no negotiation) |
| **Depends on** | Telnet IAC negotiation (item 1.3); MSDP/MNES we can skip |
| **Unblocks** | Modern MUD client HUDs (Mudlet, MUSHclient, Blightmud) — health bars, mini-maps, target panels, quest trackers, all live off GMCP |
| **Discussion frame** | Three slices: (a) negotiate GMCP option + envelope (one milestone); (b) emit `Char.Vitals` + `Room.Info` first because they replace the prompt + room render for capable clients; (c) the rest one package per milestone, driven by what the engine already emits. Build dirty-batching from the start — `Char.Vitals` per HP-tick is a stampede vector. |
| **Open Qs** | Per-client subscribe model (on by default vs. opt-in), payload shape (Tapestry JSON vs. our own), versioning |

### 1.3 Telnet IAC option negotiation

| | |
|---|---|
| **Status** | Bare `IAC WILL/WONT ECHO` byte writes; no real option negotiation; deferred from M0 |
| **Spec** | `networking-protocols §4` — TTYPE, NAWS, ECHO, MSSP, GMCP |
| **Tapestry source** | `src/Tapestry.Networking/TelnetNegotiator.cs`, `TelnetProtocolRouter.cs`, `TelnetProtocolConstants.cs`, `ClientCapabilities.cs` |
| **Go state** | `internal/conn/telnet` — incomplete negotiator |
| **Unblocks** | TTYPE → client identity / 256-color hint; NAWS → terminal width (panel rendering); MSSP → server listing on MUD portals; **GMCP transport** (item 1.2 depends on this) |
| **Sequencing** | Do TTYPE+NAWS first (cheap, immediate UX win on panel width); MSSP next (one milestone, then we appear on MUD listings); GMCP last because it gates §1.2 |

### 1.4 WebSocket transport

| | |
|---|---|
| **Status** | Specced, unbuilt |
| **Spec** | `networking-protocols §3` — WebSocket with JSON envelope |
| **Tapestry source** | `src/Tapestry.Networking/WebSocketConnection.cs`, `WebSocketGmcpHandler.cs` |
| **Go state** | `internal/server` is telnet-only; no HTTP/WS surface |
| **Unblocks** | Browser play (in-page client); easier mobile clients; web-based admin tools |
| **Depends on** | Connection abstraction is already clean (`internal/conn.Connection` interface), so this is a parallel transport, not a refactor |
| **Open Qs** | Where the HTTP listener lives (same process as telnet vs. separate); auth story for WS (same login flow vs. token); JSON envelope shape (Tapestry's vs. a thinner one) |

### 1.5 Arg typing + resolution

| | |
|---|---|
| **Status** | Spec §5 fully designed; not built; we hand-tokenize `c.Args` in every handler |
| **Spec** | `commands-and-dispatch §5` — ArgDefinition, engine arg type set, pack-registered arg types, resolution pipeline, ordinal selectors |
| **Tapestry source** | `src/Tapestry.Engine/ArgDefinition.cs`, `ArgResolver.cs`, used by every handler |
| **Go state** | `internal/command/builtins.go` handlers parse args by hand — `target := strings.Join(c.Args, " ")` patterns repeated everywhere |
| **Unblocks** | Real command-help auto-generation (§8) — the metadata fix shipped today is a bridge; synthesis from arg defs is the real thing. Cleaner handler bodies. Pack-registered commands with safe target resolution. |
| **Cost** | Touches every existing handler — staged migration, not a flag day |
| **Open Qs** | How handlers consume resolved values (typed struct vs. accessor on `Context`); multi-word entity names (spec §12 open) |

### 1.6 Chat channels / tells / notifications

| | |
|---|---|
| **Status** | **No spec written.** Tapestry ships `Comm` GMCP + `NotificationQueue` + `RespondModule` |
| **Tapestry source** | `src/Tapestry.Engine/NotificationQueue.cs`, scripting `NotificationsModule.cs`, GMCP `Comm` handler under `Tapestry.Server/Gmcp/Handlers/` |
| **Go state** | Room broadcast only; no inter-player messaging |
| **Unblocks** | Social play. This is the single largest "MUD doesn't feel like a MUD" gap. |
| **Discussion frame** | Spec slice would cover: (a) channel model (named, joinable, role-gated); (b) tell (private 1:1); (c) notification queue per session with priority + GMCP route; (d) chat history retention policy; (e) ignore/block. Pre-decisions needed before spec: are channels content-defined (in packs) or engine-fixed (newbie/ooc/admin)? |

### 1.7 Emotes

| | |
|---|---|
| **Status** | Sketched in `commands-and-dispatch §7`; not fully specced; not built |
| **Tapestry source** | `src/Tapestry.Engine/EmoteRegistry.cs` + scripting `EmotesModule.cs` |
| **Go state** | None |
| **Spec gap** | §7 talks about behavior; the **registry shape** and **actor/target/room template substitution model** aren't specced in detail |
| **Unblocks** | Roleplay surface; this is the second-biggest "MUD doesn't feel like a MUD" gap after chat |

### 1.8 Doors + locks

| | |
|---|---|
| **Status** | **No spec written.** Tapestry has full door+lock model |
| **Tapestry source** | `src/Tapestry.Engine/DoorService.cs`, `DoorState.cs` + scripting `DoorsModule.cs` |
| **Go state** | `world.Exit` has no door field; exits are always open |
| **Spec gap** | Door state synchronization across both rooms, lock state, key entity, open/close/lock/unlock verbs, observable events |
| **Open Qs** | Are doors a property of the exit, the room, or a separate entity? Tapestry's choice is "service over exit pairs" — worth borrowing |

---

## 2. Specced but Unbuilt in Go

Grouped by spec. Citations are ROADMAP unchecked items or memory `m<N>-deferred-fixes.md`.

### commands-and-dispatch
- §4 chaining (`;`) + repeat (`3n`) — deferred
- §5 arg typing — deferred (see 1.5)
- §6 bad-input tracker — deferred (Tapestry: `BadInputTracker.cs`)
- §7 emotes — deferred (see 1.7)
- §8 auto-help from arg defs — partial; we hand-author Syntax in the registration metadata (landed today). Real synthesis from arg defs waits on §5.

### networking-protocols
- §3 WebSocket transport — see 1.4
- §4 full IAC negotiation — see 1.3
- §5 GMCP envelope + every package — see 1.2
- §6 MSSP variables (uptime, players, codebase, contact) — deferred

### login
- §3 pluggable name-gates — only hardcoded ASCII-letter validator today (Tapestry: `LoginGateRegistry.cs`, `ILoginGate.cs`)
- §6.1 per-phase idle timeout — `conn.Read` has no deadline

### time-and-clock
- §3 in-game 24-hour clock + Dawn/Day/Dusk/Night buckets — deferred; no consumer
- §4 slow-tick observability — deferred

### session-lifecycle
- §5 admin-tag idle exemption — blocked on role system (M10+)

### world-rooms-movement
- §3.4 tag-indexed reads during movement — deferred

### inventory-equipment-items
- Container weight/volume caps — deferred
- Rarity/essence colorization — see also 3 (`EssenceRegistry`, `RarityModule` are unspecced in our world)

### mobs-ai-spawning
- §3.2 mob stat derivation from race+class — mobs use static `combat.Stats`, no `StatBlock`
- §3.3 mob equipment instantiation — deferred
- §3.4 mob ability proficiencies — no mob prof map
- §3.5 death-driven purge from a generic alive predicate — only explicit `Untrack` triggers respawn

### abilities-and-effects
- **Mob effect-stat install** — blocked by `entities ↔ progression ↔ stats ↔ entities` import cycle (m8-1 #1, m9-4 #2). Effects publish to mobs but modifiers don't take. Resolution: hoist a small interface to a leaf package (like we did with `internal/srckey`).
- Stat-factor passive gain (§3.5) — base × taper × failure-mult only
- Passive scaling bonuses (§6.2) — built but no wired consumer
- Mob passives — no proficiency map, no extra attacks/evades

### progression
- Vital re-clamp under max-affecting stat recompute — `combat.Vitals` and `StatBlock` max diverge silently
- De-level via `DeductExperience` — spec §10 open question unresolved
- Mob `StatBlock` consumer — `SourceKey` extraction deferred until a consumer needs it

### quests
- `quest_grant` on item or room property — no `world.Room.Property` bag (shares m7-6 gap)
- Effect/item-triggered quest advance — no event field for pickup payload (depends on scripting)

### economy-survival
- Effect-id registry for consumables — `item.consumed` event carries `effect_id` but nothing subscribes to apply it. Needs a small `EffectTemplate` registry distinct from `Ability.Effects`.

### ui-rendering-help
- `prompt` verb (§7.6) — schema accepts per-player template; no verb to set it. Small slice.
- 256-color / truecolor — deferred
- Per-pack custom palettes — deferred
- Command-help auto-generation (§9.2) — partial; see 1.5

### character-creation
- Generalized content-authored flows — only new-player wizard exists
- GMCP wizard-panel renderer (§5 seam) — plain text only (depends on 1.2)

### scripting-and-packs
- Entire runtime — see 1.1
- Hot reload — deferred
- Cross-pack reference validation at boot — deferred

### persistence
- Property registry (§2) + tagged-value envelope (§4.4) — flat save state only

---

## 3. In Tapestry but Not Specced Yet

These need a spec slice before they can land. Roughly ordered by "would make the MUD feel modern."

| Feature | Tapestry source | Spec gap |
|---|---|---|
| **Doors & state sync** | `DoorService.cs`, `DoorState.cs` | See 1.8 |
| **Portals / temporary exits** | `TemporaryExitService.cs`, `TemporaryExitRecord.cs`, scripting `PortalsModule.cs` | Tick-driven cleanup, observable events, who can create |
| **Notifications queue** | `NotificationQueue.cs`, scripting `NotificationsModule.cs` | Per-entity priority queue underpins 1.6 |
| **Channels, tells, who** | `Comm` GMCP, `RespondModule.cs` | See 1.6 |
| **Weather** | `WeatherService.cs`, `WeatherZoneDefinition.cs`, `WeatherZoneRegistry.cs`, scripting `WeatherModule.cs` | Per-zone state, tick-driven evolution, observable events |
| **Recall / return-home** | `ReturnAddressService.cs`, scripting `ReturnAddressModule.cs` | Tracked last-recall point per character |
| **Essence** | `EssenceRegistry.cs`, `EssenceDefinition.cs`, scripting `EssenceModule.cs` | First-class item property with glyph + color |
| **Rarity tiers** | scripting `RarityModule.cs` | Tier ladder (common/rare/epic) with colorization |
| **Faction / standing** | (alignment substitutes today) | Per-faction reputation distinct from alignment buckets |
| **Visibility / hidden** | `VisibilityFilter.cs` | LoS, hidden mobs, sneak skill |
| **MSSP advertised variables** | `MsspProtocolHandler.cs`, `MsspConfig.cs` | Which variables to advertise + their semantics |
| **Admin verbs** | scripting `AdminModule.cs` (warp, set, reload, announce) | Needs role system (M10+) first |
| **Schedule primitive** | scripting `ScheduleModule.cs` | Script-driven callback at future tick; depends on 1.1 |
| **Stacking service** | scripting `StackingModule.cs` | Item stack merge/split/count display |
| **Cross-cutting event catalog** | `EventBus.cs`, `SystemEventQueue.cs` | Per-spec event tables exist; no aggregated catalog |
| **Tag observers** | `ITagObserver` pattern | Reactive subscribers on tag mutations |

---

## 4. Operational / Ops Gaps

Tapestry ships a full observability stack; AnotherMUD ships none.

| | |
|---|---|
| Container build | `Dockerfile`, `.dockerignore`, `docker-compose.yml` |
| Logs | `loki-config.yml` |
| Metrics | `prometheus.yml`, `TapestryMetrics.cs` |
| Traces | `otel-collector-config.yaml`, `TapestryTracing.cs` |
| Dashboards | `grafana/` |
| Repo hygiene | `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md` |

Go has only `log/slog` for structured logs; no metrics export, no traces, no
dashboards, no container build, no repo policy docs.

This is parallel to game-logic work and can land any time without spec changes.

---

## Discussion-starter sequencing

When we pick this up, a reasonable conversation flow:

1. **Pick the social slice or the modern-client slice first?** Social = 1.6 + 1.7
   (chat, tells, emotes). Modern-client = 1.3 → 1.2 (telnet negotiation → GMCP). They
   share no dependencies; either can lead. Social makes the MUD feel like a MUD even
   on basic clients; modern-client makes the MUD feel modern even without social
   features.
2. **Decide the scripting question or defer it again?** 1.1 is the longest-lived
   choice. We can keep deferring as long as we're shipping engine-internal milestones,
   but the next quest content slice (post-M10) probably forces the issue.
3. **What's the cheapest "feels like progress" slice?** Likely the `prompt` verb
   (§2 ui-rendering-help, ~30 minutes), or doors (1.8, needs a small spec + ~3-file
   slice). Use these as warm-up before tackling 1.1-1.6.
4. **Ops stack as background work?** Section 4 can land in parallel without blocking
   anything. Worth landing before we have real players hitting the server.

## Pre-decisions that would unblock spec work

If we want to spec the unspecced items in §3, these are the choices we'd need to make
first:

- **Channels** — content-defined (per-pack) or engine-fixed (newbie/ooc/admin only)?
- **Doors** — property of exit, property of room, or separate entity?
- **Weather** — coarse (per-area enum) or fine (per-room with neighbor influence)?
- **Faction** — alignment-shaped (linear scale) or graph-shaped (per-faction matrix)?
- **Essence vs. rarity** — one tag system or two? Tapestry treats them as separate.

---

*End of matrix. Update this file when the situation shifts; archive sections as
items land by moving them into `docs/DEFERRED-BACKLOG.md` or deleting if obsoleted
by a spec amendment.*

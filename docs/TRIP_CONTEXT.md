# AnotherMUD — Portable Trip Context

> **Snapshot date:** 2026-06-22 · Branch `main` @ `4cbc222` · Engine: Go 1.26, module `github.com/Jasrags/AnotherMUD`.
> A self-contained restore point for working on AnotherMUD from the Claude.ai web UI without repo access.

## 0. How to use this document (read first)

Treat this file as **ground truth for current project state** for this conversation. Specifically:

- AnotherMUD is a **custom Go MUD engine** — not ROM/Circle/Coffee/Diku/Evennia. **Do not recommend, compare, or assume any off-the-shelf codebase or object model.** The codebase is decided and built.
- Specs are **behavior-only and setting-agnostic**: *what* a system does + *why*, never *how*. No code, no library names, no magic numbers in the spec body — every tunable goes in a **Configuration surface** table. Follow the house spec format (Overview → numbered operation sections each ending in an Acceptance-criteria checklist → Configuration surface → Open questions → Cross-references). Imitate `recall.md` (small) or `roles-and-permissions.md` (stateful).
- **Do not re-litigate settled decisions** (§3) or re-propose systems that already exist (§1). Check the built list before designing.
- **Pick up active work from the status in §2** — don't restart shipped slices.
- Flag genuine uncertainty rather than inventing detail. Some in-repo index docs (PRIMER.md, specs/README.md footer, several `docs/proposals/*`) are **older than this snapshot** and under-report shipped work; where they conflict, this document + `CLAUDE.md` win.

> ⚠️ No external Claude.ai conversation context was pasted into this generation, so §3 reflects only repo/memory state, not any in-flight web-UI discussion.

---

## 1. Engine snapshot

**Maturity:** well past prototype. Milestones **M0–M30 complete** + the five original cross-cutting themes. The active arc is the **Wheel of Time program**: a *mechanics* EPIC (`docs/themes/wot-mechanics-epic.md`) translating the WoT d20 RPG onto the engine's tick/chance model, and a *content* track (`docs/themes/wot-world-plan.md`). Recent post-M30 work (shipped directly on `main`, not numbered milestones): the **follow → grouping** gameplay-systems arc.

**Load-bearing primitives:** a fixed-cadence **tick loop** (~100 ms); a typed **event bus** (cancellable vetoes + non-cancellable facts); an **entity store** (by id/tag/type, double-buffered tag index); **sessions** (`connActor` + manager: flood/idle/link-dead/takeover); **content packs** (YAML + Lua, namespaced ids, loaded into registries at boot); **versioned player saves** (atomic tmp→bak→rename, migration chain — currently **v32**). Two clocks: a wall `Clock` driving ticks + an in-game hour/day game clock content reasons about.

### Fully built (integrate with these; do not re-propose)
Networking (telnet+IAC/GMCP/MSSP, WebSocket, tiered ANSI) · world (rooms/areas/exits, doors/locks, portals, weather, in-game clock, light & darkness, biomes, room coords) · entities/items (templates, slots, equip, containers, stacking, keyword resolution, decorations/rarity/essence) · progression (stats/races/classes/tracks/alignment/training/use-based proficiency/abilities/effects, **feats**, **skills**, generalized **resource pools**) · combat (engage/round/hit-miss-damage/flee/death, **corpses+loot+decay**, **conditions**, **saves**, **subdual/knock-out**, **ranged incl. cross-room**, **two-weapon**, **size-and-wielding**, **weapon-identity**, **masterwork**, **special weapons**, **action-economy/busy-state**) · the **One Power** (channeler initiate/wilder split, weaves, overchannel cascade, affinities/Five Powers, angreal, taint/madness) · economy (currency/shops/sustenance/rest/consumables, **crafting & cooking**, **gathering**) · **the player-trade trio** (escrow/atomic-transaction, direct trade, buyout auction house) · **faction/standing** + **reputation/renown** (+ consumers) · **movement cost/encumbrance** · **mounts** (ride + mounted travel, core v1) · quests · commands (registry + typed args, tab-completion phases 0–2) · sandboxed **Lua** scripting · social (notifications/tells/channels/emotes) · player lifecycle (accounts, **account-first login + character roster**, **character world-locking**, creation wizard, **languages** substrate, **gender** save v22) · visibility (hide/sneak/invis/search + hidden exits) · presentation (themes/prompts/panels/help, **player maps** + fog-of-war) · roles-and-permissions + admin verbs (live + enforcing).

### Spec-only — contract written, no Go code yet
| System | Spec | Note |
|---|---|---|
| Reactive tag observers | `docs/specs/tag-observers.md` | `entity.tag_added/removed` bus events; substrate ahead of any consumer. The only write-ahead contract left in BACKLOG §1. |
| Area effects (grenades + room hazards) | `docs/specs/area-effects.md` | The engine's first multi-target attack: shared area-effect primitive → thrown grenadelike weapons + placed room hazards. Adds the first persisted dynamic room state. Greenfield, specced 2026-06-17, build pending. |

### Confirmed greenfield — no spec, design freedom (call out as new substrate if needed)
Mail/parcels (attachment push-delivery escrow) · banking/vault + a **gold-at-risk** rule (banking is flavor-only until gold is at risk) · hireable mobs · onboarding-guide NPC · OLC (in-game world building — collides with the boot-immutable content model) · feature-module system (code-level packaging) + web admin console · GoMud-style gameplay cluster (gambling, fishing→gathering, leaderboards, AFK, alt-characters, elections/zone-tax) · procedural missions/boards · fast-travel waypoints · dropped-item decay + `trash`/`bury` · moon-cycle/weather-driven ambient light · survival v2 (split sustenance into hunger+thirst — single pool today) · the **Shadowrun pack** + the multi-ruleset framework it needs.

> The in-repo PRIMER.md §5 still lists faction/biomes/gathering/visibility/auction/trade as greenfield — **stale; all shipped.** Trust this section.

---

## 2. Active work in flight

| Item | Status | Open decision | Artifact |
|---|---|---|---|
| **Grouping arc (§9 tail)** | Core arc COMPLETE — roster, kill-XP, shared loot, assist, auto-assist, leadership succession, **master-looter loot policy** + **`promote` handoff** all shipped this session | Next slice: **auto-follow-on-join** (recommended), then XP-split shape, shared quest credit, round-robin/need-greed loot | `docs/specs/grouping.md`; memory `grouping-arc` |
| **Follow primitive** | Slice 1 shipped (the move-with-leader pull); mob-leader following + flee-drag deferred | Whether to couple follow into grouping (auto-follow) | `docs/specs/follow.md` |
| **Area effects** | Spec complete, **build pending** | Build sequencing only (primitive → grenades → hazards) | `docs/specs/area-effects.md` |
| **Tag-observers** | Spec complete, no consumer/code | Wire when a first reactor needs it | `docs/specs/tag-observers.md` |
| **WoT EPIC — One Power S2 depth** | Phases 0–4 + taint/madness/angreal/Mental-Stability shipped | Resume targets: **Wilder emotional Block, linking, stilling-restore path, save-DC/buff affinity scaling** | `docs/proposals/wot-the-one-power.md`; `docs/themes/wot-mechanics-epic.md` |
| **WoT EPIC — open sub-epics** | S1/S2/S3/S4/S5/S6/S8/S9(partial) shipped | **S7** survival v2 (hunger/thirst), **S9** more classes + the multiclass feat-credit fix, **S10** travel/planes, **S11** Shadowspawn mob mechanics | `docs/themes/wot-mechanics-epic.md` |
| **Reputation consumers** | Core + persistence (v32) + Fame/Infamy/Low-Profile feats + disposition reaction shipped | **R4 recognition consumer** + remaining earn sources (worn signifier, class-level increment, creation seed) deferred | `docs/specs/reputation.md`; memory `reputation-build-log` |
| **Mounts depth** | Core v1 shipped (ride + mounted travel, save v26) | Deferred slices: mounted combat (charge + Ride contest), barding/saddlebag/stabling economy, lead/transfer | `docs/specs/mounts.md`; memory `mounts-build-log` |
| **Multi-ruleset / Shadowrun** | Framework design draft + SR reference extracted; **no code** | Whether to pursue; needs the `channel-vocabulary` ruleset-translation framework first | `docs/themes/channel-vocabulary.md`, `docs/themes/shadowrun-pack-plan.md`, `docs/shadowrun/` |
| **Feature-module system** | Greenfield design draft | `Module` contract shape + enable/disable model (manifest-gated boot recommended) | `docs/proposals/feature-module-system.md` |
| **Player maps — Mudlet mapper** | ASCII map + fog shipped (M24); GMCP `Room.Info` x/y/z is a **placeholder wire-shape** | Must validate against a live Mudlet client before announcing mapper support (unconfirmed) | `docs/proposals/player-maps.md`; memory `room-coordinates-gmcp-wireshape` |

---

## 3. Key decisions made (rationale in one line)

> As of this snapshot all recent decisions are committed to the cited files; none are floating uncommitted. Listed so the web UI does **not re-litigate** them.

- **WoT = translate onto tick/chance, not a d20 rewrite** (EPIC "Decision 0", resolved). Rationale: the engine is real-time tick/chance; the d20 tabletop grid is scaffolding to deliberately *not* port. *(S12 d20-rewrite shelved.)*
- **Loot distribution v2 = master-looter** (not round-robin/need-greed). Rationale: the engine's loot model is **manual** (`get from corpse` with encumbrance/slot checks); leader-controlled/threshold policies fit the owner-set seam, auto-distribution policies would need new machinery. → `grouping.md §5/§9`.
- **Leader-named successor = immediate `promote` handoff** (old leader stays a member); unplanned departures still fall to longest-tenured succession. → `grouping.md §3`.
- **`tapestry-core` is a placeholder name** — specs stay setting-agnostic; the world is content packs, not specs.
- **Engine is ruleset-agnostic** — generic + WoT + (future) Shadowrun on one tick/chance kernel via a content-driven channel/formula layer. → `docs/themes/channel-vocabulary.md`.
- **Work directly on `main`, no feature branches** (solo project); per-slice commits, code review still gates a phase. *(Repo workflow; overrides any default branch-first rule.)*
- **Deliberately-transient state is not persisted** — sessions, weather, spawn tracking, temporary exits, active effects, rest state, corpses, concealment/detection, biome/gathering node state. (First dynamic room state to persist will be area-effects hazards.)

---

## 4. Open questions / decide next (rough priority)

1. **Next grouping slice.** Options: auto-follow-on-join · XP-split shape (level-weighted / group bonus) · shared quest credit · round-robin/need-greed loot. **Recommended default:** auto-follow-on-join (small, clean, decoupled today; the obvious cohesion win). Quest credit and auto-distribution loot need cross-spec/new-machinery design.
2. **Which WoT sub-epic next.** Options: S2 One Power depth (Wilder Block / linking / stilling-restore) · S7 survival v2 (hunger+thirst split) · S9 more classes + multiclass feat-credit fix. **Recommended default:** S2 depth (the marquee arc has the most momentum); S9 multiclass-credit is a known correctness bug worth folding in when a 2nd class lands.
3. **Reputation R4 recognition consumer** + remaining earn sources. Mostly mechanical; default: ship recognition-check at the look/who surface, then the worn-signifier earn path.
4. **Greenfield economy depth.** Mail (attachment push-delivery escrow) and/or banking — but **banking is flavor-only until a gold-at-risk rule exists** (death costs no gold today). Decide gold-at-risk first, or accept banking as convenience.
5. **Build area-effects** (spec ready) — grenades + room hazards; introduces the first persisted dynamic room state.
6. **XP de-level semantics** (`progression` open Q): may `DeductExperience` drop a level? Function clamps today; de-level behavior unresolved.
7. **Pursue multi-ruleset / Shadowrun?** Large; needs the `channel-vocabulary` framework before any SR code. Default: defer until the WoT arc settles.
8. **Feature-module system** (architectural) — reshapes how every greenfield gameplay system ships. Default: defer; decide the `Module` contract + manifest-gated enable/disable model before committing.
9. **OLC (in-game building)** — needs the source-of-truth pre-decision (write-back to pack YAML vs. a runtime overlay) + a `builder` role before any spec.

---

## 5. Research library

| Path | Contains | Useful for |
|---|---|---|
| `docs/mud-research/` | Cross-MUD feature research: per-MUD inventories (Aardwolf, Achaea, AwakeMUD, BatMUD, Discworld, NukeFire, WoTMUD) + a command taxonomy + a README cross-index | Borrowing proven feature shapes / verb surfaces before designing a greenfield system |
| `docs/wot/` | Wheel of Time d20 RPG sourcebook extracts (one-power, classes, feats, skills, combat, equipment, backgrounds, geography, etc.) | The canonical reference being ported by the WoT mechanics EPIC + world content |
| `docs/shadowrun/` | Shadowrun 5e rules extraction (character/creation/rolls/tests/armor/weapons/cyber-bioware/magic) | Source material for a future Shadowrun pack (blocked on the multi-ruleset framework) |
| `docs/themes/wot-mechanics-epic.md` | The 12-sub-epic WoT mechanics program + Decision 0 + per-sub-epic engine-seam mapping + status | The roadmap for all WoT mechanics work; check before starting any WoT sub-epic |
| `docs/themes/wot-world-plan.md` | WoT geography + content authoring track (companion to the mechanics EPIC) | Authoring WoT regions/areas |
| `docs/themes/channel-vocabulary.md` | Design for hosting multiple rulesets (WoT, Shadowrun) on one tick/chance kernel | The framework a Shadowrun pack depends on |
| `docs/themes/shadowrun-pack-plan.md` | Shadowrun content-pack planning | Scoping the SR pack |
| `docs/themes/crafting-cooking-plan.md`, `biomes-gathering-plan.md` | Program plans for crafting/cooking + biomes/gathering (both shipped) | Historical design rationale for those shipped systems |
| `docs/proposals/` | Pre-spec design drafts; most are resolved/shipped (character-identity, light-and-darkness, tab-completion, player-maps, wot-the-one-power, combat-equipment-depth, gender-definitions) — `feature-module-system.md` is the live greenfield one | Design rationale behind shipped specs; the open one is the module system |
| `docs/PRIMER.md` | Self-contained orientation for external spec design (engine model, what exists, house format) | Pasting as context when drafting a new spec — **but its §3/§4/§5 lists are M18-era and under-report shipped work; trust §1 here instead** |
| `docs/ENGINE-VOCABULARY.md` | The engine↔content contract: reserved tags, property keys, id-namespacing rules | Authoring content packs |
| `docs/specs/README.md` | Canonical spec reading order + cross-cutting tables (cancellable events, registries, save surface, tick handlers) | Finding which spec owns an event/registry/save field (footer is dated 2026-06-18; some "build pending" markers are stale) |
| `docs/ROADMAP.md` / `docs/BACKLOG.md` / `docs/DEFERRED-BACKLOG.md` | Done-log / open-work list / deferred-fix aggregate | "What's done" / "what's next" / "what was punted" |
| `docs/PLAYTEST.md` | Manual playtest walkthrough by system | Verifying live behavior section by section |
| `docs/world/<pack>/map.html`, `docs/mockups/map-color.html` | Interactive world map + terrain-color mockup (visual references) | Eyeballing geography / render palette |
| `docs/clients/tab-completion-gmcp.md` | GMCP tab-completion wire spec + Mudlet snippet | Client-side completion integration |
| `docs/archive/` | Superseded docs (Tapestry gap-matrix, theme-axis plan) | Historical only — not active |
| `docs/CODEMAPS/` | Token-lean derived architecture maps (regenerate via `/update-codemaps`) | Fast code-layout orientation (subordinate to specs) |

---

*Generated for offline web-UI continuity. If a statement here conflicts with `CLAUDE.md` or a `docs/specs/*` file you can see, prefer those — this is a snapshot, not the source of truth.*

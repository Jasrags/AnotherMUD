# Backlog

The single list of **open** work: what's left to build, what needs a design decision
first, and the candidate themes those items cluster into. Answers "what should we do
next?" Replaces the Tapestry gap-matrix (a one-time bootstrap audit, now archived) and
the theme-axis plan (its method survives below).

## How this doc stays honest

- **Open-only.** Every line here is *not done*. When an item ships, **delete its line** —
  do not strike it through or mark it `[x]`. The record of done work lives in
  `ROADMAP.md` (milestone boxes) + git history + the `m<N>-deferred-fixes.md` memories.
  A single source for "done" is what keeps this list from rotting the way the matrix did.
- **Specs are the source of truth for behavior; this doc never duplicates it.** A
  specced item links its `docs/specs/<file> §X` — the *what* lives there. An unspecced
  item's first deliverable *is* a new spec slice (the spec set grows as ideas get
  promoted: it went 17 → 21 as Themes A/C added `notifications`, `chat-channels-and-tells`,
  `emotes`, `recall`).
- **Verified against code.** Every item below was confirmed absent in the codebase as of
  2026-06-01, not trusted from the old matrix (which misreported several shipped systems).

## Status: the five original themes all shipped

Theme A (Social / M13), B (Modern Client / M16), C (World Depth / M15),
D (Content Authoring / M17), E (Engine Debt / M14) are **done** — see `ROADMAP.md`.
What remains is the tail those themes didn't claim, below.

---

## 1. Specced — ready to build

A spec already describes the behavior; only the Go implementation is missing. These can
go straight into a milestone.

| Item | Spec § | Gap (verified absent) |
|---|---|---|
| Command chaining `;` + repeat `3n` | commands-and-dispatch §4 | no chain/repeat parsing in dispatch |
| Bad-input tracker (escalation on repeated junk) | commands-and-dispatch §6 | only "Huh?" + `floodGate` rate-limit exist; no §6 tracker |
| Auto-help synthesis from arg defs | commands-and-dispatch §8 / ui §9.2 | Syntax is hand-authored; no synthesis from `ArgDefinition`s. **Unblocked** — arg typing shipped M17.2 |
| `who` verb | chat-channels-and-tells (§ "if/when that verb lands") | no player-list verb. **Needs a small spec slice first** (verb behavior unspecced) |
| Pluggable name-gates | login §3 | only the hardcoded ASCII-letter validator |
| Per-phase idle timeout | login §6.1 | `login.go` notes it as a known gap; no per-phase `Conn.Read` deadline set |
| Tag-indexed reads during movement | world-rooms-movement §3.4 | movement scans, no tag index |
| Container weight/volume caps | inventory-equipment-items | no cap enforcement at runtime |
| Mob equipment instantiation at spawn | mobs-ai-spawning §3.3 | `Template.Equipment` decoded but `Store.SpawnMob` never equips it |
| Death-driven purge from a generic alive predicate | mobs-ai-spawning §3.5 | only explicit `Untrack` triggers respawn |
| Passive gain stat-factor | abilities-and-effects §3.5 | passive gain omits the §3.5 stat factor (no entity-stat-by-id seam) — m9-5 #1 |
| Passive scaling-bonus consumer | abilities-and-effects §6.2 | `PassiveScalingBonus` built, no wired hook consumes it — m9-5 #2 |
| Effect/item-triggered quest advance | quests | no event field carries the pickup payload (scripting now exists to carry it) |
| GMCP wizard-panel renderer | character-creation §5 | creation flow emits plain text only (nil GMCP sink) — m12-3 |
| Generalized content-authored creation flows | character-creation | only the fixed new-player wizard exists |
| Cross-pack reference validation at boot | scripting-and-packs | no boot-time cross-pack ref check |
| Property-registry save-pipeline integration | persistence §2 / §4.4 | registry substrate exists (M14.4); not wired into the save pipeline — m14 |
| Slow-tick observability | time-and-clock §4 | no slow-tick instrumentation |
| **Roles & permissions** *(keystone)* | **roles-and-permissions §2–§8** (new) | no role system; help-tier is a no-op stub. Flat `HasRole` capability model (ported from Tapestry). Unblocks admin verbs, §5 idle exemption, verb gating |
| Admin-tag idle exemption | session-lifecycle §5 | gated on Roles above — build alongside it |
| **Admin verbs** | **admin-verbs §2–§8** (new) | admin gate (commands marked admin, `HasRole`-gated) + baseline verbs (inspect/set/teleport/announce/restore/purge/reload). Gates today's ungated `reload`/`xp`. Ported from Tapestry `AdminModule` |

---

## 2. Unspecced — needs a spec slice first

No spec exists yet. The first deliverable is a new `docs/specs/` file (and the
pre-decision it depends on). These are where genuinely-new *systems* live — the gap the
old five-theme partition left uncovered.

- **Faction / standing** — per-faction reputation distinct from alignment buckets.
  ⚠️ **No Tapestry reference — needs design help before a spec.** Tapestry has no
  faction/standing/reputation system (alignment substitutes there). This is greenfield:
  pre-decisions (linear scale vs. per-faction matrix; relation to alignment; whether it
  depends on Roles) need a design conversation, not a port. Park until then.
- **Visibility / hidden / sneak** — line-of-sight, hidden mobs, sneak skill. (Today only a
  `BypassVisibility` arg flag exists — no system.)
- **Essence** — first-class item property with glyph + color. Pre-decision: one tag
  system with Rarity or two.
- **Rarity tiers** — common/rare/epic ladder with colorization.
- **Cross-cutting event catalog** — per-spec event tables exist in `specs/README.md`;
  no aggregated catalog. (Docs/meta, not engine.)
- **Reactive tag observers** — subscribers on tag mutations. Partial substrate exists
  (`Store.Retag`); no observer registration surface.

---

## 3. Decisions owed (spec open questions)

Not build tasks — design tensions parked in specs' "Open questions" sections. Resolve
before the dependent build.

- **XP de-level semantics** — progression §10: can `DeductExperience` drop a level?
  Function exists and clamps; de-level behavior is the unresolved part.
- (Each spec's "Open questions" section is the feeder here — pull others in as they block work.)

---

## 4. Ops (background track)

Parallel to game-logic work; never blocks a theme; needs no spec. AnotherMUD ships none
of this today (`log/slog` only). Land before real players hit the server.

- Container build — `Dockerfile`, `.dockerignore`, `docker-compose.yml`
- Metrics — Prometheus export
- Traces — OpenTelemetry collector
- Dashboards — Grafana
- Repo hygiene — `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`

---

## Candidate next themes

"What could we do next" = the open items above, clustered. Pick one arc; don't
cherry-pick across them.

| Theme | Pulls in | Spec status | Rough size |
|---|---|---|---|
| **Roles & Administration** | Roles (§2) → admin verbs, session §5 idle exemption, verb gating | unspecced (keystone) | M — unblocks the most |
| **Gameplay Depth** | faction, visibility/sneak, essence, rarity (§2) | unspecced (4 spec slices) | L |
| **Command & UI polish** | chaining/repeat §4, bad-input §6, auto-help §8, prompt verb, `who` | specced | S — good warmup cluster |
| **Engine Debt II** | mob equipment §3.3, death-purge §3.5, passive gain/scaling, property-registry save wiring, tag-indexed reads, cross-pack validation, GMCP wizard panel | specced | S–M — clears the m9-x/m14 tail |
| **Ops** | §4 above | n/a | background only |

### Picking rubric (from the retired theme-axis method)

| If yes → | start with |
|---|---|
| Verbs/features are ungated and you want admin control or to gate them | Roles & Administration |
| The character sheet / world feels mechanically thin in playtest | Gameplay Depth |
| You want a fast, low-stakes win to re-enter the codebase | Command & UI polish (or a single warmup item) |
| Accreting code debt is blocking a feature you want | Engine Debt II |
| You're about to expose the server to real players | Ops (in background) |

Prefer the smallest scope that lands a real win before committing further. Engine Debt
should land at least once every two or three other themes.

### Parallelism rules

- **One main theme at a time.** Splitting attention across two stalls both.
- **Ops always runs in the background** — filler between theme commits, never foreground.
- **Warmups between themes.** Take one small specced item (§1) for 30–90 min to
  recalibrate before committing to the next arc.

### Anti-patterns

- **Cherry-picking across themes** — one chaining fix, one faction stub, one ops file —
  produces breadth without throughline. Pick a theme.
- **Spec'ing a system alone** — for player-facing systems (faction, visibility), get the
  pre-decision settled before writing the spec.
- **Letting `BACKLOG.md` accumulate done items** — delete shipped lines; never `[x]` here.

---

## When a theme starts

1. Add a `## M<N> — <Theme>` heading to `ROADMAP.md` with `[ ]` exit criteria (cite the
   spec §s).
2. For unspecced items, write the `docs/specs/` slice first; resolve its pre-decision.
3. As items ship, **delete them from this file** (the ROADMAP box is the record).

*Specs describe behavior. ROADMAP tracks status. This file tracks the gap. Keep them in
their lanes.*

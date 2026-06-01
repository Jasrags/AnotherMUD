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
| `who` verb | **who §2–§4** (new) | no player-list verb. Conventional roster (Tapestry has none); reads `Manager.OnlinePlayers`. Per-viewer hiding deferred to visibility rules |
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
| **Rarity tiers** | **item-decorations §2,§4,§5** (new) | ordered tier ladder → themed decorated marker (inline + column-padded). Ported from Tapestry `RarityRegistry` |
| **Essence** | **item-decorations §3,§4,§5** (new) | colored glyph item marker; participates in stack identity. Ported from Tapestry `EssenceRegistry` |
| Reactive tag observers | **tag-observers §2–§4** (new) | `entity.tag_added/removed` bus events for non-index reactors. Substrate ahead of a consumer. Ported from Tapestry `ITagObserver` |
| **Crafting & Cooking** | **crafting-and-cooking** (new) + plan `themes/crafting-cooking-plan.md` | recipes + crafting-skill proficiency + quality roll (output = rarity tier) + cooking→sustenance/well-fed. MVP = Tier 0 + Tier 1 campfire (temp entity, M15.2 reuse) + Tier 2 room-tag + mob-loot ingredients, all in `core` pack. Defers only gathering nodes (§2) |
| **Player trade** (escrow + direct trade + auction) | **trade-escrow / direct-trade / auction-house** (new) + plan `plans/trade-plan.md` | shared escrow/atomic-commit primitive (cancellable bus); sync zero-sum direct trade; async persisted buyout auction (global, pickup delivery, fee gold sink). Admin moderation blocked on roles/admin (spec-only). Push delivery deferred to Mail (§2) |

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
- **Visibility / hidden / sneak** — line-of-sight, hidden mobs, sneak skill.
  ⚠️ **Tapestry reference is a STUB — rules need design help.** `VisibilityFilter` exists
  but `CanSee` always returns true and `GetVisibleEntities` returns everything; the *seam*
  (filter + `BypassVisibility` arg, already in our M17.2a resolver) is real, but the
  *rules* (what hides an entity, sneak mechanics, see-invisible) are greenfield. The seam
  is captured wherever it's consumed (`admin-verbs §3`, `commands-and-dispatch §5`); the
  rules need a design conversation. Park the rules; the seam is already usable.
_(Crafting & Cooking moved to §1 — now specced: `crafting-and-cooking.md` (+ plan in
`docs/themes/crafting-cooking-plan.md`). Stations are **resolved** (no furniture system:
Tier 2 = room tag/property M14.5; Tier 1 campfire = temporary placed entity reusing the
M15.2 decay pattern — both in the crafting MVP). One greenfield **sub-dependency** stays
below: **gathering / resource nodes**, deferred past MVP, which sources ingredients from
mob loot + authored placement until then.)_
- **Gathering / resource nodes** — the non-vendor ingredient source crafting wants
  (`crafting-and-cooking §8`). Overlaps **Biomes** below (forage/harvest). Greenfield;
  until it lands, crafting sources ingredients from mob loot + authored placement.
- **Biomes** — ecological zones layered on rooms, shaping spawns / resources / ambience.
  ⚠️ **Greenfield system — no Tapestry reference.** BUT the substrate exists: rooms already
  carry a `terrain` property (outdoors/indoors/forest/mountain) used for weather gating
  (`world-rooms-movement §6.4`) and weather zones (`weather_zones/`). Pre-decision: is a
  "biome" just a richer alias/extension of `terrain`, or a new layer adding biome-specific
  spawn tables, foraging/harvest resource nodes, and ambience? Heavily interlocks with
  `mobs-ai-spawning` (spawns) and a future foraging/crafting loop. Needs a design
  conversation; decide the terrain-vs-new-layer question first.
_(Player market — direct trade + auction house — moved to §1: now specced as
`trade-escrow.md` (the shared escrow/atomic-transaction primitive), `direct-trade.md`,
`auction-house.md`, + plan `docs/plans/trade-plan.md`. v1 decisions made: buyout-only,
global market, pickup delivery. The only deferred greenfield piece is **push delivery
(mail attachments)**, shared with Mail below.)_
- **Mail / parcels (addressed items + gold)** — send a message *with attachments*
  (items and/or gold) to another player, claimed later. ⚠️ **Greenfield — no Tapestry
  reference.** Today: text-only **offline tells** (M13.2) on the notifications queue; no
  attachments. The *text/delivery* substrate exists (notifications.md anticipates mail);
  the new piece is **push-delivery escrow** — attached items/gold held out of the world
  until claimed. **Shared substrate with the auction house** (`auction-house.md` §11.2):
  the auction ships **pickup** in v1 to avoid this, so push-delivery is deferred and, when
  built, is the *one* attachment-delivery layer both player-mail and auction push-delivery
  consume. Note the atomic-transaction half is already specced (`trade-escrow.md`); mail
  adds the addressed-push-to-an-offline-player layer on top. Pre-decisions: read-anywhere
  vs. a post-office/mailbox room; postage + COD (gold sinks); mailbox cap; unclaimed-mail
  expiry/return-to-sender.
- **Banking (stored gold, maybe item vault)** — a deposit/withdraw balance separate from
  carried gold. ⚠️ **Greenfield — no Tapestry reference.** Today gold is a single integer
  carried **directly on the character** (`economy-survival §2.1`), persisted on the save —
  there is no banked balance, vault, or teller. ⚠️ **Note: banking has little mechanical
  purpose until carried gold is *at risk*.** Death currently costs no gold (combat death
  heals to 1 HP + teleports, m7-5); with no death-penalty/theft/PvP, a bank is
  convenience/flavor only. Spec a gold-at-risk rule alongside it, or accept it as a
  convenience verb. Substrate: currency (M11.1), the account store (for an account-shared
  bank across alts), the shop/NPC pattern (M11.2) for a teller, persistence (a banked
  balance on the player or account save). Pre-decisions: gold-only vs. gold + item vault;
  per-character vs. account-shared; teller/bank-room vs. access-anywhere; interest + fees
  (economic levers / gold sinks); is there a gold-at-risk mechanic to justify it.
- **Cross-cutting event catalog** — per-spec event tables exist in `specs/README.md`;
  no aggregated catalog. (Docs/meta, not engine — not a behavior spec.)

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

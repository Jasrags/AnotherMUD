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
  item's first deliverable *is* a new spec slice (the spec set has grown 17 → **30** as
  ideas get promoted; of the write-ahead batch, roles, admin-verbs, and item-decorations
  have since shipped (M19/M20), leaving `tag-observers`, `who`, `crafting-and-cooking`,
  and the trade trio as contracts still ahead of code in §1).
- **Verified against code.** Every item below was confirmed absent in the codebase as of
  2026-06-02, not trusted from the old matrix (which misreported several shipped systems).

## Status: M0–M22 shipped; specced + greenfield work remains

The five original themes — A (Social / M13), B (Modern Client / M16),
C (World Depth / M15), D (Content Authoring / M17), E (Engine Debt / M14) — are
**done**, and since then **M19** (Roles & Administration), **M20** (Item Decorations),
**M21** (Item Stacking), and **M22** (Loot & Corpses) have shipped (see `ROADMAP.md`).
**M18** (Command & UI polish) is **paused** mid-flight (only M18.1 `prompt` shipped; the
rest sit in §1). Behavior contracts still written-ahead-of-code: `tag-observers`, `who`,
`crafting-and-cooking`, and the trade trio (§1). What remains unspecced (§2) is the
greenfield gameplay/economy-depth tail the themes didn't claim.

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
| Reactive tag observers | **tag-observers §2–§4** (new) | `entity.tag_added/removed` bus events for non-index reactors. Substrate ahead of a consumer. Ported from Tapestry `ITagObserver` |
| **Crafting & Cooking** | **crafting-and-cooking** (new) + plan `themes/crafting-cooking-plan.md` | recipes + crafting-skill proficiency + quality roll (output = rarity tier) + cooking→sustenance/well-fed. MVP = Tier 0 + Tier 1 campfire (temp entity, M15.2 reuse) + Tier 2 room-tag + mob-loot ingredients, all in `core` pack. Defers only gathering nodes (§2) |
| **Player trade** (escrow + direct trade + auction) | **trade-escrow / direct-trade / auction-house** (new) + plan `plans/trade-plan.md` | shared escrow/atomic-commit primitive (cancellable bus); sync zero-sum direct trade; async persisted buyout auction (global, pickup delivery, fee gold sink). Admin moderation blocked on roles/admin (spec-only). Push delivery deferred to Mail (§2) |
| **Tab-completion — Phase 0** (enumeration substrate) | **tab-completion §2–§9** (new) + proposal `proposals/tab-completion.md` | transport-agnostic completion query (verb scan + §5 typed-arg scope enumeration), distinct-name/ordinal disambiguation, the visibility leak-guard, and a role-gated `complete` debug verb. No enumeration op exists today (resolvers are resolve-one-token). Surfaces (GMCP/char-mode) are Phase 1/2 → §2 below |

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
- **Hidden / secret doors** — exits concealed until discovered (e.g. via a `search`
  verb). ⚠️ **Greenfield — not covered by what exists.** Doors (`world.Exit.Door`,
  M15.1) and temporary keyword portals (`world-rooms-movement §5.6`, M15.2) both exist,
  but neither is hidden/discoverable — portals are *dynamic runtime exits*, not
  *concealed permanent ones*. The relevant seam is the `visibility` filter
  (`world-rooms-movement §visibility`), a permissive stub already designed to consult a
  `hidden` tag — but it filters *entities*, not exits. Overlaps **Visibility / hidden /
  sneak** above (shared discovery/reveal mechanics). Pre-decisions: hidden flag on the
  exit/door vs. a hidden-exit tag; the `search` mechanic (auto-on-enter vs. explicit
  verb; skill/level gate; per-character vs. per-room discovery state and whether "found"
  persists); reveal messaging.
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
- **Player grouping / party** — a party of players with combat assist plus
  **XP-sharing and loot-sharing options**. ⚠️ **Greenfield — no grouping exists.**
  Substrate that's already in place: combat keys kill credit off the **attacker
  id on `VitalDepleted`** (`combat.md` §10), XP is granted per-entity via
  `progression.Manager.GrantExperience(entityID, track, amount, source)`, and the
  room is the shared combat arena. The new system is the **party itself** and its
  reward rules. Pre-decisions: party model (leader + invite/accept vs. follow);
  **XP split rule** (even / level-weighted / proximity-gated); **loot rules**
  (free-for-all / round-robin / leader-only / need-greed) — these attach to the
  **loot-and-corpses §4 ownership-set seam** (shipped in M22: the corpse already
  records an owner set + `corpse.MayLoot` checks it; grouping fills the set with
  party members); **assist / auto-engage** (a party member's attack pulls the
  rest); party chat (overlaps `chat-channels-and-tells`); shared **quest credit**
  (overlaps `quests`). Needs a design conversation before a spec.
- **Hireable mobs (mercenaries, hirelings)** — NPCs a player hires to follow, fight,
  or guide. ⚠️ **Greenfield — nothing in code or specs.** Mobs have behavior +
  disposition + AI only; there is no owner/controller relationship and no `follow`
  verb. Effectively a single-player analog of **Player grouping / party** above and
  reuses its substrate: a hireling follows its owner, assists in combat, and plugs into
  the same kill-credit + **loot-and-corpses §4 owner-set** seam; it also touches
  `mobs-ai-spawning` (a `MobInstance` with an owner + a follow/guard behavior) and
  `economy-survival` (hire cost + upkeep as a gold sink). Pre-decisions: ownership +
  lifetime (permanent vs. timed contract vs. dismissable); command surface
  (`hire`/`dismiss`/`order`/`follow`); combat assist + XP/loot split (reuse grouping's
  rules); cap on simultaneous hirelings; persistence (does a hireling survive logout?).
  Best decided alongside or just after grouping.
- **Input tab-completion — polish only (feature complete)** — all surfaces are
  **LANDED**: Phase 0 substrate; presentation policy (`tab-completion §12`); the
  line-mode `suggest` stopgap; **Phase 1** GMCP `Input.Complete` request/response
  (`§13`, live-verified over WS); and **Phase 2** char-mode real TAB on raw telnet
  (`§14`, live-verified — `get sw`+TAB → `get sword`). **Remaining is polish, not
  surfaces:** (a) the GMCP *client* integration (bind Tab → `Input.Complete`,
  render reply — Mudlet/client-owned, guide in `docs/clients/tab-completion-gmcp.md`).
  (b) char-mode editor polish: cursor movement (arrows/Home/End), input history,
  and prompt redraw after the Tab candidate list (MVP is a single forward-typed
  buffer). (c) minor Phase 0 deferrals in `m-…`/`phase0-tabcomplete-deferred-fixes`.
- **Survival depth — split sustenance into hunger + thirst** — today sustenance
  is a **single pool** `[0,100]` (`economy-survival §4.2`, "a hunger-like pool");
  both `eat` (food) and `drink` refill the *same* value, and `consume_method`
  (`eat`/`drink`/`use`) is only verb-routing/flavor, not a second resource. ⚠️
  **Greenfield — deliberate single-pool design today; no thirst meter exists.**
  The desire: make **thirst a distinct survival pressure** — two pools (hunger fed
  by food, thirst fed by drink), each with its own drain rate, tiers, and
  regen/penalty hooks, surfaced in the prompt, persisted (player save version
  bump), and reflected by `restore`. Pre-decisions: do both gate regen or does
  thirst carry a different penalty (e.g. movement vs. HP regen); do drink items
  stop feeding hunger entirely or partially; new tier vocabulary
  (parched/dehydrated); whether this rides a broader survival slice (temperature,
  fatigue) — best decided as one "survival v2" design pass. Reshapes the single
  `sustenance` pool that `restore` and the drain knob currently operate on. Needs
  a spec slice on `economy-survival §4` before building.
- **Mana / Movement current pools + regen** — the prompt's `MA`/`MV` columns
  (`render.DefaultPromptTemplate`, ui-rendering-help §7.1) render **stat-derived
  MAX only**: `session/prompt.go` builds `PromptVitals` with `Mana == MaxMana` and
  `MV == MaxMV` because **there is no current-pool tracking** (the code's own
  "Thin pools (M9.4b)" note). So MA/MV always show `current == max` (e.g. `0/0`
  for a fighter whose `resource_max`/`movement_max` stats are 0), and nothing
  drains or refills them. ⚠️ **Greenfield — only the *max* side exists.** What's
  present: `resource_max`/`movement_max` stats, plus `ResourceMana` cost handling
  in ability validation/resolution (`progression/validation.go`, `resolution.go`)
  — abilities can *declare* a mana cost, but there's no live pool to spend from.
  What's missing: a **current pool** for mana and movement analogous to
  `combat.Vitals` (HP-only today) — spend-on-cast, drain-on-move, a regen tick,
  and `restore`/effects integration (admin `restore` is HP-only by design; see
  `restore.go`), plus persistence. Pre-decisions: a per-resource pool type (like
  `Vitals`) vs. a generic resource-pool model; whether this rides with a future
  economy/survival slice (the prompt comment anticipated M11, which shipped
  sustenance/rest but not these). Until then MA/MV are display-only and `restore`
  correctly touches HP only.
- **Completion args for the remaining hand-parsed verbs (M17.2d non-fits)** —
  a handful of verbs still hand-parse and declare no arg types, so tab-completion
  (`tab-completion §8`) returns nothing for their arguments. The easy ones —
  `get`/`take`/`kill` (typed-arg migration commit) and `look`/`consider` — now
  declare a completion arg via `Command.HandParsed`. The rest are the **documented
  M17.2d non-fits** (`m17-2d3-deferred-fixes`), each blocked on a **new engine arg
  type** that doesn't exist in `commands-and-dispatch §5.2`: `unequip` needs an
  `equipped` arg type (match against worn slots); `fill` needs a source-scope arg
  (the fill source isn't inventory/room/container as-is); `buy`/`sell`/`value`
  resolve against **shop stock**, which no arg type covers (the resolution lives in
  `ShopService`). Each is a small design decision (define the arg type) + a
  `HandParsed` declaration — not a scheduled phase; pick up opportunistically or
  when an arg-type sweep is worth it. Not blocking the tab-completion surfaces above.
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
cherry-pick across them. **The picture flipped this cycle:** after the spec batch, most
themes are now **specced and ready to build** — the constraint is no longer "write a
spec" but "pick what to build." Only the greenfield gameplay/economy-depth systems still
need a design pass first.

**Ready to build (specced — §1):**

| Theme | Pulls in | Size |
|---|---|---|
| **Command & UI polish** *(PAUSED — M18, resume)* | `who`, bad-input §6, chaining/repeat §4, auto-help §8 (prompt verb shipped) | S |
| **Crafting & Cooking** | `crafting-and-cooking` + plan; full Tier 0/1/2 MVP in the `core` pack | M |
| **Player trade** | trade-escrow + direct-trade + auction-house + plan; atomic escrow, sync trade, buyout auction | M |
| **Engine Debt II** | mob equip §3.3, death-purge §3.5, passive gain/scaling, property-save wiring, tag-indexed reads, cross-pack validation, GMCP wizard panel | S–M |

**Needs a design pass first (greenfield — §2):**

| Theme | Pulls in | Why design-first |
|---|---|---|
| **Gameplay Systems** | faction, visibility/sneak, hidden doors, biomes, gathering, hireable mobs | no port reference; each needs pre-decisions before a spec |
| **Player Economy depth** | mail (push delivery / attachment escrow), banking + a gold-at-risk rule | extends the now-specced trade; banking wants gold-at-risk to matter |

**Background:** **Ops** (§4) — container/metrics/traces/dashboards/repo-hygiene; never a foreground theme.

### Picking rubric (from the retired theme-axis method)

| If yes → | start with |
|---|---|
| You want a real item economy — players selling loot to each other | **Player trade** *(specced — ready)*; then Economy depth (mail/banking, greenfield) |
| You want a crafting/gathering loop | **Crafting & Cooking** *(specced — MVP ready)* |
| The world/character sheet feels mechanically thin | **Gameplay Systems** *(greenfield — design first)* |
| You want a fast, low-stakes win to re-enter the codebase | finish **M18 Command & UI polish** (or one §1 warmup) |
| Accreting code debt is blocking a feature you want | **Engine Debt II** *(specced)* |
| You're about to expose the server to real players | **Ops** (in background) |

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

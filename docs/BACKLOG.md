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
  item's first deliverable *is* a new spec slice (the spec set has grown 17 → **32** as
  ideas get promoted; of the write-ahead batch, roles, admin-verbs, and item-decorations
  have since shipped (M19/M20; `who` shipped too), leaving `tag-observers`,
  `crafting-and-cooking`, and the trade trio as contracts still ahead of code in §1).
- **Verified against code.** Every item below was confirmed absent in the codebase as of
  2026-06-02, not trusted from the old matrix (which misreported several shipped systems).

## Status: M0–M22 shipped; specced + greenfield work remains

The five original themes — A (Social / M13), B (Modern Client / M16),
C (World Depth / M15), D (Content Authoring / M17), E (Engine Debt / M14) — are
**done**, and since then **M19** (Roles & Administration), **M20** (Item Decorations),
**M21** (Item Stacking), and **M22** (Loot & Corpses) have shipped (see `ROADMAP.md`).
**M18** (Command & UI polish) is now **complete** — `prompt`, `who`, auto-help
synthesis, command chaining/repeat, and the bad-input tracker all shipped.
Behavior contracts still written-ahead-of-code: `tag-observers`,
`crafting-and-cooking`, and the trade trio (§1). What remains unspecced (§2) is the
greenfield gameplay/economy-depth tail the themes didn't claim.

---

## 1. Specced — ready to build

A spec already describes the behavior; only the Go implementation is missing. These can
go straight into a milestone.

| Item | Spec § | Gap (verified absent) |
|---|---|---|
| Per-phase idle-timeout *overrides* | login §6.1 | global idle timeout **shipped** (Clock-driven, `Config.IdleTimeout`, `ANOTHERMUD_LOGIN_IDLE_TIMEOUT`, default 60s); only *per-phase override values* remain (a thin add on the same read primitive) |
| Tag-indexed reads during movement | world-rooms-movement §3.4 | movement scans, no tag index |
| Container weight/volume caps | inventory-equipment-items | no cap enforcement at runtime |
| Death-driven purge from a generic alive predicate | mobs-ai-spawning §3.5 | only explicit `Untrack` triggers respawn |
| Passive gain stat-factor | abilities-and-effects §3.5 | passive gain omits the §3.5 stat factor (no entity-stat-by-id seam) — m9-5 #1 |
| Passive scaling-bonus consumer | abilities-and-effects §6.2 | `PassiveScalingBonus` built, no wired hook consumes it — m9-5 #2 |
| Effect/item-triggered quest advance | quests | no event field carries the pickup payload (scripting now exists to carry it) |
| GMCP wizard-panel renderer | character-creation §5 | creation flow emits plain text only (nil GMCP sink) — m12-3 |
| Generalized content-authored creation flows | character-creation | only the fixed new-player wizard exists |
| Cross-pack reference validation at boot | scripting-and-packs | no boot-time cross-pack ref check |
| Property-registry save-pipeline integration | persistence §2 / §4.4 | registry substrate exists (M14.4); not wired into the save pipeline — m14 |
| Slow-tick observability — full breakdown / routing | time-and-clock §5 | core **shipped**: `Loop.SetSlowTickObserver` times each tick, warns (`slog`) when it exceeds a threshold (`ANOTHERMUD_SLOW_TICK_THRESHOLD`, default = tick interval); reports total + handlers. Remaining: the §5 event-queue/command components (no such tick phases in this engine) + admin-channel / OTel routing (a consumer on the callback seam) |
| Reactive tag observers | **tag-observers §2–§4** (new) | `entity.tag_added/removed` bus events for non-index reactors. Substrate ahead of a consumer. Ported from Tapestry `ITagObserver` |
| **Crafting & Cooking** | **crafting-and-cooking** (new) + plan `themes/crafting-cooking-plan.md` | recipes + crafting-skill proficiency + quality roll (output = rarity tier) + cooking→sustenance/well-fed. MVP = Tier 0 + Tier 1 campfire (temp entity, M15.2 reuse) + Tier 2 room-tag + mob-loot ingredients, all in `core` pack. Its ideal ingredient source — **gathering** — is now specced (§1, this table) and can ship alongside or after |
| **Player trade** (escrow + direct trade + auction) | **trade-escrow / direct-trade / auction-house** (new) + plan `plans/trade-plan.md` | shared escrow/atomic-commit primitive (cancellable bus); sync zero-sum direct trade; async persisted buyout auction (global, pickup delivery, fee gold sink). Admin moderation blocked on roles/admin (spec-only). Push delivery deferred to Mail (§2) |
| **Visibility** (hide / sneak / darkness / invisibility) | **visibility §2–§7** (new) | the keystone of the Gameplay Systems cluster. Hybrid model: flag-gated darkness + magical/admin invis, opposed-contest hide/sneak. Four detection paths (passive sticky auto-detect, see-invisible/see-in-dark/detect traits, `search` verb, reveal-on-action). Fills the `world-rooms-movement §7` filter seam + `commands-and-dispatch §5.4` `BypassVisibility`; unblocks `who §4` per-viewer hiding, `admin-verbs §3` wizinvis, and hidden exits. All ephemeral (no save). v1 light model is minimal (static `dark` room prop + `light` item + `see_in_dark` trait) |
| **Hidden exits** (secret doors / passages) | **hidden-exits §2–§7** (new) | `hidden` + `search_difficulty` flag on the Exit (works with or without a door, mirrors door `pick-difficulty`). Discovery reuses visibility's `search` + sticky memory; search-only (passive off by default). **Knowledge-gated**: an undiscovered hidden exit is unwalkable + door un-operable, not just unlisted — gate lives in the player movement command + `flee`, NOT the unconditional move primitive (mob/scripted/admin moves ungated). Per-character ephemeral; no save change. Emits `exit.discovered` (quest hook). Depends on visibility |
| **Faction / standing** | **faction §2–§8** (new) | per-character signed standing per content-defined faction; generalizes alignment's architecture (`progression §6`) to N axes as a **parallel sibling** — alignment untouched, no v1 interaction. Linear per-player (no opposition ripple in v1). Named ranks → rank tags, bounded combined history, cancellable `faction.shift.check`→`shifted`→`rank.changed`, admin-immune shift, `ResolveRanks` gating helper. Earn via quest rewards + faction-mob kills. New Faction registry + player-save `faction_standing` bag (version bump). Consumers (disposition/abilities/rooms/shops/quests) adopt the helper as they're wired |
| **Biomes** | **biomes §2–§6** (new) + designed with gathering | **richer terrain, one axis**: promote the existing room `terrain` property into a registered Biome definition (backward-compatible — unregistered terrain = today's behavior). Generalizes `world-rooms-movement §6.4` hardcoded shielding into biome metadata; adds idle biome ambience (new tick), an optional mob spawn table (additive to area spawns), and the forage/node resource tables gathering consumes. New Biome registry; nothing persists |
| **Gathering** (forage + nodes) | **gathering §2–§8** (new) + designed with biomes | the non-vendor ingredient source `crafting §8` wants. Ships **both** models: ambient `forage` (rolls room biome's resource table) + discrete respawning `harvest` nodes (reuse spawn scheduler). Single gathering proficiency (use-based gain), rarity-tier quality roll (mirrors `crafting §5`). **Permissive** (friction lowers quality, only tool-gated nodes refuse) + **scarce** (forage cooldown, node charges+respawn) per `crafting §8`. Cancellable `resource.gathering`; `resource.gathered` quest hook. Node/forage state transient (no save); proficiency rides existing surface |
| **Room coordinates** | **room-coordinates §2–§10** (new) | area-local integer `(x,y,z)` **derived from the exit graph** at load, **derive-by-default with a per-room `coord:` override/pin** for non-Euclidean spaces (PD-1, hybrid). Per-area grid seeded from pins (or a default anchor); BFS over intra-area cardinal/vertical exits applies fixed deltas; pins are ground truth the walk derives around. Coordinates are **stable** (viewer-independent, PD-7) so Mudlet's persistent mapper sees one fixed cell per room; the ASCII map recenters at render time. Collisions / non-square loops / unreachable rooms are **non-fatal load warnings** (PD-4). Adds the coordinate to `Room.Info` (omitted when unplaced) — exact wire shape pinned against a live Mudlet client. No movement change (PD-3); pin is content, no save change. Substrate ahead of its consumers (Mudlet mapper, telnet `map` verb — see §2) |

---

## 2. Unspecced — needs a spec slice first

No spec exists yet. The first deliverable is a new `docs/specs/` file (and the
pre-decision it depends on). These are where genuinely-new *systems* live — the gap the
old five-theme partition left uncovered.

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
- **OLC — online creation (in-game world building)** — let a **builder** create
  and edit world content live from inside the game (rooms, exits, mobs, items,
  areas, resets/spawns; later shops/quests/scripts), the classic Diku/ROM
  `redit`/`medit`/`oedit`/`aedit` surface. ⚠️ **Greenfield — needs a real design
  pass before a spec; it collides head-on with the current content model.** Today
  content is **file-authored, git-versioned, spec-driven, loaded once at boot**;
  `world.World` and the per-system registries are documented as **boot-immutable**
  (mutations MUST happen before serving). OLC inverts that — runtime mutation of
  the live world that persists somewhere. Substrate that already leans this way:
  the **`builder`/`admin` role gate** (M19 roles-and-permissions), the **admin-verb
  framework** (M19.4) and especially **`set property`** on live room mobs/items
  (M19.4h) — a tiny precursor that already mutates a running entity; the **pack
  loader's decode + validation** logic (reusable to validate OLC edits); and the
  **atomic tmp→bak→rename persistence** (M-substrate) for writing changes back.
  Pre-decisions, in rough priority: **(1) source-of-truth model** — does OLC write
  back into the pack YAML files (world-is-source, but fights git/spec authoring and
  hand-edits) or into a separate runtime/world-overlay save layered over the
  loaded packs (packs stay pristine, but the world now has two sources)? **(2)
  runtime-mutable registries** — making `world.World` + registries safe to mutate
  while serving (they're RWMutex-guarded but write-at-boot by contract); what
  invariants break (the double-buffered tag index, namespaced-id resolution, live
  entity references into edited rooms). **(3) command surface** — a sub-mode editor
  (`redit` → `name`/`desc`/`exit north <room>`/`done`) vs. flat verbs; **(4)
  validation parity** with the loader (exit targets resolve, ids unique/namespaced);
  **(5) area ownership + concurrency** — which builder may edit which area, and two
  builders on the same room; **(6) scope/order** — almost certainly rooms+exits
  first, then mobs/items, then resets, then the rest. Big system; gate it behind a
  design conversation and a dedicated spec slice.
- **Telnet `map` verb (ASCII area minimap)** — render an in-game ASCII map of the
  current area from room coordinates. **Unblocked by `room-coordinates` (§1)**, which
  deliberately stops at the coordinate substrate + GMCP exposure and leaves rendering
  to a separate slice. Today there is **no `map`/automap verb at all** and the room
  render is line-oriented (`ui-rendering-help` non-goal: real-time UI). The new piece
  is the presentation layer: project the area's placed rooms onto a 2-D character grid
  (z handled as layers or a `<` / `>` glyph), mark the player, draw exit connectors,
  omit unplaced rooms. Pre-decisions: per-area vs. radius-around-player view; z-layer
  handling (separate layer vs. stacked glyph); legend/colour via the theme registry;
  whether overlapping cells (coordinate collisions, `room-coordinates §4.1`) get a
  visible marker. Pure presentation — reads coordinates, changes no world state.
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
| **Crafting & Cooking** | `crafting-and-cooking` + plan; full Tier 0/1/2 MVP in the `core` pack | M |
| **Player trade** | trade-escrow + direct-trade + auction-house + plan; atomic escrow, sync trade, buyout auction | M |
| **Engine Debt II** | death-purge §3.5, passive gain/scaling, property-save wiring, tag-indexed reads, cross-pack validation, GMCP wizard panel | S–M |

**Needs a design pass first (greenfield — §2):**

| Theme | Pulls in | Why design-first |
|---|---|---|
| **Gameplay Systems** | hireable mobs | no port reference; needs pre-decisions before a spec. (Visibility, hidden exits, faction, biomes, and gathering are now **specced** and moved to §1; hireable mobs is best designed alongside/after grouping.) |
| **Player Economy depth** | mail (push delivery / attachment escrow), banking + a gold-at-risk rule | extends the now-specced trade; banking wants gold-at-risk to matter |
| **OLC (online creation)** | in-game world building — `redit`/`medit`/`oedit`/`aedit` for builders | collides with the boot-immutable, file-authored content model; needs the source-of-truth + runtime-mutable-registry pre-decisions first |

**Background:** **Ops** (§4) — container/metrics/traces/dashboards/repo-hygiene; never a foreground theme.

### Picking rubric (from the retired theme-axis method)

| If yes → | start with |
|---|---|
| You want a real item economy — players selling loot to each other | **Player trade** *(specced — ready)*; then Economy depth (mail/banking, greenfield) |
| You want a crafting/gathering loop | **Crafting & Cooking** + **Gathering** + **Biomes** *(all specced — ready)* |
| The world/character sheet feels mechanically thin | **Gameplay Systems** *(greenfield — design first)* |
| You want a fast, low-stakes win to re-enter the codebase | take one **§1 warmup** (tag-indexed reads, container caps, …) |
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

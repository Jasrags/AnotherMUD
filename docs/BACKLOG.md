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
  have since shipped (M19/M20; `who` shipped too, and `crafting-and-cooking` at M27),
  leaving `tag-observers` and the trade trio as contracts still ahead of code in §1).
- **Verified against code.** Every item below was confirmed absent in the codebase as of
  2026-06-02, not trusted from the old matrix (which misreported several shipped systems).

## Status: M0–M27 shipped; specced + greenfield work remains

The five original themes — A (Social / M13), B (Modern Client / M16),
C (World Depth / M15), D (Content Authoring / M17), E (Engine Debt / M14) — are
**done**, and since then **M19** (Roles & Administration), **M20** (Item Decorations),
**M21** (Item Stacking), **M22** (Loot & Corpses), **M23** (Room Coordinates),
**M24** (Player Maps), **M25** (Equipment slots), **M26** (Engine Debt II —
door-key boot validation, passive gain stat factor, GMCP wizard panel), and
**M27** (Crafting & Cooking MVP — recipes, crafting proficiency, the quality
roll, cooking→well-fed, fixed/portable/campfire stations) have shipped (see
`ROADMAP.md`).
**Light & darkness** has also shipped (per-viewer effective light + sources/fuel +
render/combat/movement friction + period transitions + persisted in-game time).
**M18** (Command & UI polish) is now **complete** — `prompt`, `who`, auto-help
synthesis, command chaining/repeat, and the bad-input tracker all shipped.
Behavior contracts still written-ahead-of-code: `tag-observers` and the trade
trio (§1). What remains unspecced (§2) is the greenfield gameplay/economy-depth
tail the themes didn't claim.

---

## 1. Specced — ready to build

A spec already describes the behavior; only the Go implementation is missing. These can
go straight into a milestone.

| Item | Spec § | Gap (verified absent) |
|---|---|---|
| Per-phase idle-timeout *overrides* | login §6.1 | global idle timeout **shipped** (Clock-driven, `Config.IdleTimeout`, `ANOTHERMUD_LOGIN_IDLE_TIMEOUT`, default 60s); only *per-phase override values* remain (a thin add on the same read primitive) |
| Tag-indexed reads during movement | world-rooms-movement §3.4 | movement scans, no tag index |
| Container weight/volume caps | inventory-equipment-items | no cap enforcement at runtime |
| Passive scaling-bonus consumer | abilities-and-effects §6.2 | `PassiveScalingBonus` built, no wired hook consumes it — m9-5 #2 |
| Effect/item-triggered quest advance | quests | no event field carries the pickup payload (scripting now exists to carry it) |
| Generalized content-authored creation flows | character-creation | only the fixed new-player wizard exists |
| Property-registry save-pipeline integration | persistence §2 / §4.4 | registry substrate exists (M14.4); not wired into the save pipeline — m14 |
| Slow-tick observability — full breakdown / routing | time-and-clock §5 | core **shipped**: `Loop.SetSlowTickObserver` times each tick, warns (`slog`) when it exceeds a threshold (`ANOTHERMUD_SLOW_TICK_THRESHOLD`, default = tick interval); reports total + handlers. Remaining: the §5 event-queue/command components (no such tick phases in this engine) + admin-channel / OTel routing (a consumer on the callback seam) |
| Reactive tag observers | **tag-observers §2–§4** (new) | `entity.tag_added/removed` bus events for non-index reactors. Substrate ahead of a consumer. Ported from Tapestry `ITagObserver` |
| **Player trade** (escrow + direct trade + auction) | **trade-escrow / direct-trade / auction-house** (new) + plan `plans/trade-plan.md` | shared escrow/atomic-commit primitive (cancellable bus); sync zero-sum direct trade; async persisted buyout auction (global, pickup delivery, fee gold sink). Admin moderation gates on roles/admin (now shipped + enforcing). Push delivery deferred to Mail (§2) |
| **Visibility** (hide / sneak / darkness / invisibility) | **visibility §2–§7** (new) | the keystone of the Gameplay Systems cluster. Hybrid model: flag-gated darkness + magical/admin invis, opposed-contest hide/sneak. Four detection paths (passive sticky auto-detect, see-invisible/see-in-dark/detect traits, `search` verb, reveal-on-action). Fills the `world-rooms-movement §7` filter seam + `commands-and-dispatch §5.4` `BypassVisibility`; unblocks `who §4` per-viewer hiding, `admin-verbs §3` wizinvis, and hidden exits. All ephemeral (no save). The minimal light model this row once sketched is **superseded** — light-and-darkness shipped (per-viewer effective light, sources, darkvision); visibility must compose darkness (this) with concealment, pinning the precedence per `light-and-darkness §12` |
| **Hidden exits** (secret doors / passages) | **hidden-exits §2–§7** (new) | `hidden` + `search_difficulty` flag on the Exit (works with or without a door, mirrors door `pick-difficulty`). Discovery reuses visibility's `search` + sticky memory; search-only (passive off by default). **Knowledge-gated**: an undiscovered hidden exit is unwalkable + door un-operable, not just unlisted — gate lives in the player movement command + `flee`, NOT the unconditional move primitive (mob/scripted/admin moves ungated). Per-character ephemeral; no save change. Emits `exit.discovered` (quest hook). Depends on visibility |
| **Faction / standing** | **faction §2–§8** (new) | per-character signed standing per content-defined faction; generalizes alignment's architecture (`progression §6`) to N axes as a **parallel sibling** — alignment untouched, no v1 interaction. Linear per-player (no opposition ripple in v1). Named ranks → rank tags, bounded combined history, cancellable `faction.shift.check`→`shifted`→`rank.changed`, admin-immune shift, `ResolveRanks` gating helper. Earn via quest rewards + faction-mob kills. New Faction registry + player-save `faction_standing` bag (version bump). Consumers (disposition/abilities/rooms/shops/quests) adopt the helper as they're wired |
| **Biomes** | **biomes §2–§6** (new) + designed with gathering + **plan `docs/themes/biomes-gathering-plan.md`** | **richer terrain, one axis**: promote the existing room `terrain` property into a registered Biome definition (backward-compatible — unregistered terrain = today's behavior). Generalizes `world-rooms-movement §6.4` hardcoded shielding into biome metadata; adds idle biome ambience (new tick), an optional mob spawn table (additive to area spawns), and the forage/node resource tables gathering consumes. New Biome registry; nothing persists. **Plan-sequenced as Milestone A** (registry → §3 shielding generalization → ambience → resource-table fields); decision **D1 = full biomes first** (the §3 shielding refactor is the risk slice, splittable into its own reviewed commit) |
| **Gathering** (forage + nodes) | **gathering §2–§8** (new) + designed with biomes + **plan `docs/themes/biomes-gathering-plan.md`** | the non-vendor ingredient source `crafting §8` wants. Ships **both** models: ambient `forage` (rolls room biome's resource table) + discrete respawning `harvest` nodes (reuse spawn scheduler). Single gathering proficiency (use-based gain), rarity-tier quality roll (mirrors `crafting §5`). **Permissive** (friction lowers quality, only tool-gated nodes refuse) + **scarce** (forage cooldown, node charges+respawn) per `crafting §8`. Cancellable `resource.gathering`; `resource.gathered` quest hook. Node/forage state transient (no save); proficiency rides existing surface. **Plan: Milestone B** (after biomes), then **Milestone C** migrates recipes + closes §8. **Vendor/§8 policy locked (D2):** gather-primary; vendors may sell basic intermediates at a premium but crafting is always cheaper (content pricing discipline `output.value > Σ input.value`) and every finished recipe chain has ≥1 gather/loot-only input; introduces a **refining tier** (gather ore → refine ingot → craft dagger). Not geography-blocked (only regional recipes are) |
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
  (economic levers / gold sinks); is there a gold-at-risk mechanic to justify it. The
  **item-vault half** is exactly GoMud's `storage` module — a per-player item stash at
  rooms tagged `storage` (`storage add/remove [all|<n>]`, by-number item reference); spec
  the gold-bank and item-vault together or as two slices.
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
  the **role gate** (M19 roles-and-permissions) — though note **only the single
  configured `admin` role is enforced anywhere today**; roles are free-form strings
  but a distinct **`builder`** role does not yet gate anything (the `look` room-data
  block and every admin verb gate on `admin`). The **admin-verb
  framework** (M19.4) and especially **`set property`** on live room mobs/items
  (M19.4h) — a tiny precursor that already mutates a running entity; the **pack
  loader's decode + validation** logic (reusable to validate OLC edits); and the
  **atomic tmp→bak→rename persistence** (M-substrate) for writing changes back.
  Pre-decisions, in rough priority: **(0) builder role** — wire a distinct
  **`builder`** role (separate from `admin`) and gate the OLC verbs on it, so
  world-editing privileges can be granted without handing out full admin. This is
  the first thing OLC needs (a gate before any edit verb exists) and is deferred
  to here deliberately — a separate builder role has no consumer until OLC, so the
  role substrate stays single-`admin` until then. Touch points when it lands: a
  `BuilderRole` in the dispatcher `Env`/`Config` (mirroring `AdminRole`), a
  `Command.Builder` flag (or a required-role field) in the registry gate, and an
  `IsBuilder()` Context helper alongside `IsAdmin()`; the existing `look` room-data
  block (admin-gated today) is a natural thing to also unlock for builders.
  **(1) source-of-truth model** — does OLC write
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
- **Player maps (ASCII `map` verb + Mudlet/GMCP)** — the full mapping feature on top
  of the coordinate substrate; proposal at `docs/proposals/player-maps.md`. **Unblocked
  by `room-coordinates` (§1)**, which stops at the coordinate substrate + GMCP exposure
  and leaves rendering to this slice. Today there is **no `map`/automap verb at all** and
  the room render is line-oriented (`ui-rendering-help` non-goal: real-time UI). Shape:
  one shared **local-window query** (radius-N BFS over placed rooms) feeding **two
  renderers** — a server-rendered ASCII minimap (recenters the stable coordinates at draw
  time; works on raw telnet) and a `Room.Info`-extending GMCP feed for Mudlet's native
  mapper. **Decided:** geometry is settled in `room-coordinates` (derive-with-override,
  stable/viewer-independent); **fog of war is IN v1 and persisted** — a per-character
  visited-room set (player-save version bump + append-only migration), an exploration
  hook on the `player.moved`/`SetRoom` entry seam, and a render-time filter so the map
  shows only explored rooms. **So this is NOT pure presentation** — it adds the one new
  save-state field the maps feature needs. Open sub-decisions (proposal §7): exit-stubs
  to unvisited neighbors vs. fully hidden; teleport-counts-as-visited; visited-set
  pruning; the secret-exit-in-a-visited-room leak (coordinate with visibility/hidden-exits);
  the Mudlet GMCP wire shape (pin against a live client). Suggested phasing:
  `room-coordinates` → fog-of-war visited-set + hook → ASCII renderer → Mudlet GMCP.
- **Feature-module system (code-level feature packaging)** — a registration seam that
  lets a *gameplay feature's code* (its commands + event listeners + scripting functions
  + data + lifecycle hooks) live in one self-contained directory and wire itself in,
  instead of being woven through `internal/`. ⚠️ **Greenfield — architectural; needs a
  design pass before a spec.** **Design draft:** [`docs/proposals/feature-module-system.md`](proposals/feature-module-system.md)
  (recommends compiled-in + manifest-gated modules — config-toggle, one static binary —
  over GoMud's recompile-to-enable; central open fork is the enable/disable model).
  Reference: **GoMud's plugin system** (`internal/plugins`),
  which bundles each feature (auction, mail, fishing, …) as a Go package whose `init()`
  calls `plugins.New(name, ver)` then `AddUserCommand`/`RegisterListener`/
  `AddScriptingFunction`/`AttachFileSystem`/`Requires` — one seam, every extension point.
  Today AnotherMUD has the *data* half of this (content packs: `content/<pack>/`, data +
  Lua, hot-reloadable, dependency-aware-ish) but **no code half** — every Go feature is
  compiled into `internal/…` and wired by hand at the composition root (`cmd/anothermud/
  main.go` registers each command, tick handler, and service inline; ~470 lines of
  wiring). The substrate a module seam would compose is **all already present and clean**:
  `command.Registry.RegisterCommand` (typed-arg commands, M17.2), the cancellable
  `eventbus`, the sandboxed `scripting` runtime + registry, the `pack` loader (data +
  dependency order), and `srckey`/registries. The new piece is the **`Module` contract**
  itself — a thing with a name/version, declared dependencies, and a single `Register(deps)`
  method that owns its commands/listeners/script-fns/data — plus a registry that orders and
  wires modules at boot. **Do NOT copy GoMud's enable-by-recompile model** (`go generate`
  blank imports + rebuild is not runtime modularity; our packs already beat it for the data
  half) **nor its `init()`-with-package-globals style** (fights our ctx-first + immutability
  conventions and the event-versioning discipline). The interesting design question is
  whether modules are **compiled-in but config/manifest-gated** (one binary, a manifest
  enables/disables features at boot — realistic for Go) vs. a fuller plugin story.
  Pre-decisions: the `Module` interface shape (constructor-injected deps vs. a context
  object); enable/disable model (manifest-gated boot vs. always-on); does a module own a
  content sub-pack or stay code-only; inter-module dependency declaration + load order
  (we'd want the topological sort the pack loader still lacks — shared fix); how a module
  contributes to persistence (save-surface ownership) and to GMCP. Big seam; gate behind a
  design conversation. **If pursued, it reshapes how every §2 gameplay system below is
  delivered** — each becomes a module rather than another graft into `internal/`.
- **Web admin console + per-feature REST API** — a browser-facing admin/ops surface
  (config viewer/editor, live-state inspection, per-feature management pages) plus a small
  REST API, with role/permission gating. ⚠️ **Greenfield — no web admin layer exists.**
  Today the only HTTP in the tree is the **WebSocket game transport** (`internal/server/
  wshandler.go`, `internal/conn/ws`) — there is no admin web server, no HTML, no REST.
  Admin happens entirely **in-game** via the admin-verb framework (M19.4: `inspect`,
  `set property`, `restore`, `teleport`, `purge`, `announce`, `grant`/`revoke`) gated on
  `HasRole(adminRole)` (roles-and-permissions, landed + enforcing). Reference: every GoMud
  module ships its own admin page + `AdminAPIEndpoint(method, slug, handler, permKey)` with
  per-endpoint permission keys — a clean pattern, but it presumes their plugin seam. For us
  this is most coherent **after (or alongside) the feature-module system** above: an admin
  surface that auto-discovers per-module pages is the natural payoff of that seam, and the
  existing role gate is the authorization model to reuse. Pre-decisions: is this an
  operator tool (config/metrics/inspection — overlaps **Ops §4**) or a player-facing web
  surface (leaderboards, help — overlaps GoMud's `webhelp`/`leaderboards`); embed in the
  game binary vs. a sidecar; session/auth model for the web (reuse account bcrypt store?);
  and the CSP/headers/CSRF posture from the web security rules. Could start tiny —
  read-only config + live `who`/room inspection over the WS port's HTTP mux — and grow.
- **Gameplay modules ported from GoMud (greenfield feature cluster)** — GoMud's module
  catalog surfaces several **genuinely-new** gameplay systems we have no spec or code for.
  (Overlap already tracked/shipped: auction → `auction-house` spec §1; mail → Mail §2;
  in-game time → `gameclock` shipped. **Storage/banking, fast travel, missions, world
  cleanup, follow, and onboarding now have their own §2 entries** — below or above.) ⚠️
  **Each is greenfield and needs its own spec slice; listed here as a clustered candidate
  pool, not a committed slice.** Best delivered *as modules* if the feature-module seam
  above lands first.
    - **Minigames / gambling** (GoMud `gambling`) — room-tag-activated games
      (`slots`/`slot machine`/`claw machine` tags), each with a **per-play cost**, a
      **win chance**, and **weighted prize tables**, plus a **persistent jackpot** pool.
      A gold sink. Touches economy (currency, M11.1), room tags, and item scripting (Lua).
      Pre-decision: pure-chance vs. skill-influenced; jackpot funding (rake % of plays) +
      payout cap.
    - **Fishing / activity minigames** (GoMud `fishing`) — turn-based catch loop with
      **rod-item gating**, **catch tables**, and a fishing-skill modifier. **Strong
      overlap with the specced Gathering loop (§1)** — likely a *flavor variant of
      `harvest`* (water node + catch table + required tool) rather than a separate system.
      Fold into the gathering design rather than spec'ing standalone.
    - **Leaderboards** (GoMud `leaderboards`) — server-wide rankings across configurable
      categories (level, kills, gold, …), fed by existing bus events (`LevelUp`,
      `MobDeath`, XP-grant analogues all exist). In-game `leaderboard` verb (aliases
      `highscore`/`topscore`); a public web page wants the web-admin layer above.
      Pre-decision: which categories; persistence (a global store like channel scrollback);
      **reset/season semantics** (all-time vs. periodic).
    - **AFK automation** (GoMud `zombiemode`) — player-configured auto-play for idle
      characters: a **combat target list** (`*`/all), a **roam radius**, a **rest HP-floor
      threshold** (stop or flee below it), **loot rules**, **waypoints**, session stats, and
      **wake-on-any-input**. ⚠️ Design-sensitive: interacts with idle/link-dead handling
      (session-lifecycle), combat fairness, and the economy (unattended farming / gold
      inflation). Likely *not* desirable without a deliberate decision; recorded for
      completeness.
    - **Multiple characters per account** (GoMud `alt-characters`) — N characters under one
      account with a **slot cap**, a **switch/swap** flow, and **recreate-character**, gated
      to rooms tagged `character`. We have the account↔player split already (`account` store
      → `player` saves), so the substrate is close; the new pieces are an account→characters
      index, the slot cap, a room-gated switch flow, and a `CharacterChanged`-style event.
      Pairs naturally with **Banking (§2, account-shared vault)** and **party/grouping (§2)**.
    - **Player governance / elections** (GoMud `elections`) — zone-level campaigns + voting
      at **polling rooms** (room tag); the winner's **title is appended to their name**
      ("Sammy, Mayor of Frostfang"); a **zone coffer** fed by a **configurable % tax on
      every shop purchase in the zone** (a player-controlled gold sink + economic lever —
      *novel; we have nothing like it*); **elected-officials-only restricted areas** (the
      official + their party may enter). Large, setting-heavy; wants **faction (§1)** + roles
      (shipped) + the zone-tax economy hook as substrate. Long-tail candidate — but the
      **zone-tax→coffer gold-sink is worth extracting on its own**, even without the full
      elections system (pairs with Banking's gold-at-risk discussion).
- **Procedural missions / mission boards** (GoMud `automission`) — auto-generated,
  repeatable objectives drawn from **mission boards** (room-tagged) — distinct from the
  authored `quests` system (shipped), which is hand-written and narrative. ⚠️ **Greenfield
  extension to quests.** Mission types **kill / find / explore / escort**, in **easy/hard
  difficulty tiers** with scaled rewards. Mechanics with no current analog: **escort**
  (spawns an NPC on accept + a **time limit** + guide-it-to-a-destination-zone), a
  **restock period** per board, a **max-concurrent-missions** cap, and **turn-in must
  happen at the same board** that issued it. Substrate: quests (reward grant + objective
  tracking), room tags (boards), mob/item spawn (targets + escort NPC), the kill-credit
  seam (`combat §10`). Pre-decisions: generator inputs (board-local mob/item/room pools vs.
  global); reuse the quest store or a separate transient store; escort-NPC AI (follow +
  guard — overlaps **follow** / **hireable mobs**).
- **Fast travel — waypoint network** (GoMud `fasttravel`) — a network of
  **visit-to-unlock** waypoints (rooms tagged `fast travel`): visiting one permanently
  unlocks it for that character, and from any waypoint you may jump to any
  previously-visited one (`fasttravel`/`ft`). ⚠️ **Greenfield — distinct from `recall`
  (shipped, single fixed point) and temporary portals (M15.2, ephemeral admin/scripted
  exits).** Adds a per-character persisted **visited-waypoint set** (save version bump).
  Friction knobs from the module: **per-use gold cost**, a **required item**, and
  **disallowed-item-types** (can't fast-travel carrying contraband — a reusable
  transport-friction idea). Pre-decisions: unlock-on-visit vs. purchase; instant vs.
  travel-time; interaction with locked/hidden rooms; whether the visited-set can share the
  player-maps fog-of-war set (§2).
- **World cleanup — corpse & item decay** (GoMud `cleanup`) — scheduled removal of stale
  **corpses**, **dropped items**, and temporary objects, plus `trash`/`bury` verbs. ⚠️
  **Partial — corpse *creation* shipped (M22.2) but decay is the deferred M22.5 slice
  ([[m22-deferred-fixes]]); dropped-item decay is unbuilt.** Today nothing prunes corpses
  or ground items → unbounded growth (a known open edge). Substrate: the tick scheduler (a
  decay-sweep handler), the entity store + placement, corpse ownership / `MayLoot`.
  Pre-decisions: decay timers (per-corpse vs. global sweep, rarity-weighted?); does
  `bury`/`trash` accelerate it; do owned / quest items resist decay; a `*.decayed` event
  (quest / observer hook).
- **Player/NPC follow** (GoMud `follow`) — a `follow <name>` primitive: when the target
  leaves a room, the follower moves with them (`follow stop`/`unfollow`; `follow lose` to
  shake pursuers). ⚠️ **Greenfield — no follow verb exists.** It is the **shared movement
  primitive under three other §2 items**: party (auto-move together), hireable mobs (a
  hireling trails its owner), and the newbie-guide NPC. Substrate: the player-move seam
  (`player.moved` / `SetRoom`), the room graph. Pre-decisions: consent (auto vs.
  accept-invite), chains / loops (A→B→A), cross-area + locked/hidden-exit handling,
  mob-following-player. Best designed alongside **grouping** so they share the
  move-with-leader mechanic.
- **Onboarding guide NPC** (GoMud `newbieguide`) — a guide NPC that **follows a new player
  and walks them through first steps until a configurable level cap**, then departs. ⚠️
  **Greenfield — we have the creation wizard (M12) + MOTD but no in-world onboarding NPC.**
  Depends on **follow** (the NPC trails the newbie) and mob spawn/AI; cheap once those
  exist. Pre-decisions: dialogue source (pack Lua vs. config), trigger (spawn on first
  login under level N), one guide per newbie vs. shared, dismissal.
- **Moon cycles + weather-driven ambient light** — make night brightness depend on
  **moon phase** and **cloud cover**, not a flat `gloom`. ⚠️ **Greenfield — anticipated and
  deferred by `light-and-darkness §12` ("Moonlight and weather-driven ambient") + the
  non-goal at §1.** Today `ambientFor(period)` maps night → a flat `gloom` regardless of
  sky; yet a full moon on a clear night is navigable without a torch and a new-moon/overcast
  night is not. The slice makes night ambient `ambientFor(period, moonPhase, cloudCover)`:
  the moon lifts the night floor (gloom → dim on a bright clear night), clouds gate it (and
  also knock daylight down a level — **this subsumes the standalone "Weather dimming" §12
  item**, same machinery, opposite direction). **Composes with `light-and-darkness §2.4`
  `light_floor`**: a lamp-lit village keeps its floor regardless of moon, while hamlets and
  open wilds (no floor) gain moonlit navigability for free — which is exactly the
  village/hamlet split the WoT content wants. Moon phase is a **pure function of the in-game
  day** (`gameclock.DayCount`), so **no new persisted state** — like the period is a pure
  function of the hour. Touches three specs: **`time-and-clock`** (a lunar calendar +
  phase vocabulary — new/waxing/full/waning or a 0–1 illumination fraction — derived from
  the day counter), **`light-and-darkness`** (the `ambientFor` signature change + the night-
  floor lift), and **`weather`/`world-rooms-movement §6`** (cloud cover as the gate; the
  weather state already rides the area). Substrate present: `gameclock` (day counter +
  `time.period.change`), the light resolver (`internal/light`, already pure over gathered
  Inputs), the weather service (per-area zone state). Pre-decisions: phase representation
  (named phases vs. illumination fraction); cycle length (in-game days per lunation);
  one moon vs. WoT-flavored detail (the source has no special lunar lore — a standard cycle
  is fine); how much a full moon lifts (gloom → dim only, or → lit on the clearest nights);
  whether forest/canopy biomes shield moonlight the way they'd shield rain.
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
| **Player trade** | trade-escrow + direct-trade + auction-house + plan; atomic escrow, sync trade, buyout auction | M |
| **Engine Debt III** | §6.2 scaling-bonus consumer, property-save wiring, tag-indexed reads (§3.4) | S |

**Needs a design pass first (greenfield — §2):**

| Theme | Pulls in | Why design-first |
|---|---|---|
| **Gameplay Systems** | hireable mobs, follow, party/grouping | no port reference; needs pre-decisions before a spec. (Visibility, hidden exits, faction, biomes, and gathering are now **specced** and moved to §1; hireable mobs is best designed alongside/after grouping, and **follow** is the shared movement primitive under grouping + hirelings + onboarding.) |
| **Gameplay content / activities** | procedural missions (escort), fast-travel waypoints, gambling, fishing→gathering, leaderboards, onboarding-guide NPC, world cleanup/decay | the GoMud-module cluster — repeatable "things to do." Each is a small standalone spec; best delivered as **feature-modules** if that seam lands first. Cleanup/decay is also overdue debt (M22.5). |
| **Player Economy depth** | mail (push delivery / attachment escrow), banking (gold-bank **+ item vault = GoMud `storage`**) + a gold-at-risk rule, zone-tax→coffer gold-sink (from elections) | extends the now-specced trade; banking wants gold-at-risk to matter; zone-tax is a reusable sink worth extracting from elections |
| **OLC (online creation)** | in-game world building — `redit`/`medit`/`oedit`/`aedit` for builders | collides with the boot-immutable, file-authored content model; needs the source-of-truth + runtime-mutable-registry pre-decisions first |
| **Feature-module system** | code-level feature packaging + web admin console; reshapes how the gameplay-module cluster (gambling, leaderboards, alt-characters, …) ships | architectural — `Module` contract + enable/disable model are pre-decisions; the runtime substrate (commands/events/scripting/packs) already exists. GoMud's plugin system is the reference |

**Background:** **Ops** (§4) — container/metrics/traces/dashboards/repo-hygiene; never a foreground theme.

### Picking rubric (from the retired theme-axis method)

| If yes → | start with |
|---|---|
| You want a real item economy — players selling loot to each other | **Player trade** *(specced — ready)*; then Economy depth (mail/banking, greenfield) |
| You want to deepen the crafting loop | **Crafting & Cooking** shipped (M27); add **Gathering** + **Biomes** *(specced — the real ingredient source)* |
| The world/character sheet feels mechanically thin | **Gameplay Systems** *(greenfield — design first)* |
| You want more "things to do" — repeatable activities, destinations, prestige | **Gameplay content / activities** *(greenfield — small standalone specs)* |
| You want a fast, low-stakes win to re-enter the codebase | take one **§1 warmup** (tag-indexed reads, container caps, …) |
| Accreting code debt is blocking a feature you want | **Engine Debt III** *(specced)* |
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

# Web client — enriched gameplay over the shared wire (design)

**Status:** design / fork resolved. Theme, not a slice. Consumes the **existing**
`internal/conn/ws` WebSocket transport + the `{type,package,data}` JSON envelope
and all engine GMCP packages (`internal/gmcp`) — nothing new on the server is
required to *start*. The Mudlet HUD is paused in favor of this
(`web-client-direction` memory, decision 2026-07-08).

Prereqs landed: WS+GMCP transport (M16), the coordinate `Room.Info` (validated
live), and the **generalized `Char.Vitals.pools` map** (2026-07-15) so a resource
HUD has data. This doc resolves the one architectural fork the theme raised
before any client code is written.

## 1. The fork (Decision-0 style)

How rich should the web client's wire be, relative to the GMCP contract telnet
clients already speak?

- **A — Strict shared-GMCP contract.** Telnet and web consume the *exact same*
  packages; the web client may render them richly but the wire never grows a
  web-specific message. **Rejected:** GMCP's flat state-push packages don't
  express rich client→server intents (submit a craft form, drag an item to a
  slot, click a map node to walk there) or richer server→client structures
  (an interactive map beyond `Room.Info`, structured inventory with affordances).
  Forcing those into the shared packages makes telnet *pay* for web features and
  bends GMCP out of shape.
- **B — A richer web-only protocol replacing GMCP.** The web client speaks its
  own wire. **Rejected:** forks the contract, makes the engine serve two
  transports' worth of message semantics, duplicates work, and throws away the
  reuse this whole direction was chosen *for*. It also tempts logic into the
  client (two sources of truth).
- **C — GMCP as the shared baseline + additive rich-client messages (CHOSEN).**
  The web client is a **superset**: it receives every GMCP package a Mudlet
  client would (reused wholesale) **plus** additional messages a richer client
  understands and a plain client harmlessly ignores. Telnet never breaks; the
  engine stays the sole authority; the web client is a *view + a richer input
  surface*, never a second rules path.

**Decision: C.** The rest of this doc pins the mechanism, which refines the
memory/BACKLOG's generic "additive web-only message types" into a concrete,
guard-railed contract.

## 2. The additive contract

### 2.1 Primary seam — new GMCP packages in a reserved namespace, not new envelope types

The additive messages ride the **existing GMCP model** (`type: "gmcp"`
envelopes, both directions), as **new packages**, *not* as new envelope `type`s.
Rationale:

- GMCP already works **both directions on both transports** — server→client
  state-push and client→server intents (the shipped `Input.Complete` tab-complete
  package is exactly a client→server GMCP intent). One dispatch path, already
  built.
- These packages are **rich-CLIENT, not web-EXCLUSIVE**: a future rich *telnet*
  client could subscribe to them over GMCP subneg. Framing them as a WS-only
  envelope `type` would wrongly weld "rich" to "web transport."
- **Unknown-package safety is free.** GMCP's contract is "a client ignores
  packages it doesn't understand," and WS treats every package as supported
  (networking §5.2). So a Mudlet-on-WS or a minimal client silently drops the new
  packages — additive by construction, no negotiation needed.
- **Versioning is free** — GMCP's `<name> <version>` convention already exists.

New envelope **`type`s** (beyond today's `text`/`command`/`gmcp`) are reserved
for the rare payload that genuinely *can't* be GMCP — a future binary frame
(audio, tile images), or a client-local control message that never touches engine
state. Envelope §6.1 already drops unknown inbound types silently, so that door
is open when needed, but it is not the default tool.

### 2.2 Namespace convention

Extend the **existing** GMCP namespaces where the message is a richer view of an
existing concept; reserve `Client.*` for client-UI-control concerns. Do **not**
introduce a `Web.*` namespace — it mis-frames rich-client capability as
web-exclusive and bakes the transport into the semantics.

Illustrative (per-feature design settles the exact names/shapes):

| Concept | Baseline (shared) | Rich additive package |
|---|---|---|
| Map | `Room.Info` (id, exits, coords, ambience) | `Room.Map` — a walkable neighborhood graph / fog state for the interactive map |
| Inventory | `Char.Items.List` | `Char.Inventory` — structured items with slots + affordances for drag-drop |
| Journal | (`quests` verb text) | `Char.Quests` — active quests with stage + per-objective progress and an abandon action |
| Trade | (`trade` verb text) | `Char.Trade` — live two-party staging (both offers + confirm flags), rescind/confirm/decline actions |
| Auction | (`browse`/`collect` verb text) | `Char.Auction` — active listings (priced, closing countdown) + pending collectibles, buyout/unlist/collect actions |
| Forms | (typed commands) | `Client.Form` (server→client form spec) + `Client.Form.Submit` (client→server) |
| Intents | (typed commands) | `Client.Do` — a structured intent (e.g. `{verb:"equip", item, slot}`) |

### 2.3 Client→server intents reduce to existing engine commands (the authority invariant)

Every inbound rich intent **reduces to an existing engine command or service
call** — the same one a typed command would reach. A drag-to-equip is the
`equip` service; a craft-form submit is the `craft` command; a click-to-walk is a
sequence of `move`s. The engine's command registry / service layer is the single
choke point where validation, permissions, and events already live.

**The web client adds NO new authority and NO new game logic.** It is a richer
*input encoding* over the same command surface, not a bypass. This keeps the
"engine is the sole source of truth" invariant intact and means a web feature
can't diverge from what telnet can do — it can only make the same action easier
to express.

### 2.4 No handshake to start

WS is already "everything supported." Rely on GMCP's ignore-unknown to make the
new packages safe for any client, and **do not** build a capability handshake up
front (mirrors how the engine already skips `Core.Supports` on WS). Add an opt-in
`Client.Hello` capability announce **only if** wasted bandwidth to non-web clients
becomes a real cost — i.e. to *gate* rich emissions, an optimization, never a
correctness precondition.

## 3. Invariants (the guardrails)

1. **Telnet is never a second-class citizen and never breaks.** Every additive
   message is optional; a client that ignores all of them is still fully
   playable. No engine behavior may become reachable *only* via a web message.
2. **Engine is the sole authority; the web client is a view + input surface.** No
   game state is computed client-side; every intent flows through the existing
   command/service validation. No new rules path.
3. **Ruleset-agnostic wire.** Additive packages must not bake one setting's
   vocabulary — follow the generalized `Char.Vitals.pools` model (kind-keyed, not
   `one_power`). Alignment/One-Power/Essence-isms stay out of the shared wire; the
   client labels/skins from generic, kind-tagged data.
4. **Additive, versioned, ignorable.** New packages only; never repurpose or
   break a shipped package's shape. Ride GMCP versioning; unknown = ignored.
5. **One dispatch model.** Prefer new GMCP packages over new envelope types;
   reserve envelope types for genuinely non-GMCP transport needs.

## 4. Phases (rough)

- **P0 — DONE.** WS+GMCP transport, coordinate `Room.Info`, generalized
  `Char.Vitals.pools`. The baseline data a HUD/map needs is already on the wire.
- **P1 — the superset baseline (no new packages). DONE (`f179749`) → `clients/web/`.**
  A pure-browser shell (no build step): connect over WS, send `command`, render
  `text` (ANSI→HTML), and consume the existing packages — a resource HUD from
  `Char.Vitals` (incl. the `pools` map), a coordinate minimap from `Room.Info`,
  and combat/effects/experience/identity panels. Clicking an exit sends the move
  command (authority invariant). Proved C end-to-end with **zero server change**;
  guarded by `TestLive_WebClientWSFrontDoor` (a real WS client hits the running
  `/mud`). De-risked the theme — the wire as it stands already drives a real HUD.
- **P2 — first rich additive package + intent. DONE.** `Room.Map` (server→client):
  the local neighbourhood graph from `world.LocalWindow` — placed same-area rooms
  within `ANOTHERMUD_ROOM_MAP_RADIUS` (default 3), each with coords, exits, and
  the viewer's fog-of-war `visited` flag — emitted alongside `Room.Info` on every
  transition (a baseline client ignores it). The web client draws it as a
  neighbourhood map (unentered rooms hollow) and **click-to-walk**: the client
  paths on the graph and sends move commands step by step, so every step is
  server-validated and a locked door stops the walk (the intent reduces to
  existing commands — no new server verb). Guarded by `TestLive_GmcpRoomMap` (real
  login → Room.Map on the wire → re-centres on a step). This establishes the
  additive-package pattern; further packages (structured inventory, forms) follow
  the same shape.
- **P3 — enriched surfaces.**
  - **Slice A — structured inventory. DONE (full `inventory`/`equipment`
    parity).** `Char.Inventory` (server→client): the actor's carried + worn
    items, mirroring the two CLI verbs. Carried items are **stacked** (M21
    `stacking.Service` — 18 bolts on one row), except ammunition holders (clips),
    listed individually with their own load state. Worn is the **full slot
    layout in registration order, empty slots included** (via the slot registry),
    a spanning item under each slot it fills. Every item carries an optional
    plain-text **detail** (a clip's `15/15 APDS`, `Armor 4`, `+1 Intuition`, a
    wielded gun's `7 rds APDS`) and its **affordances** as `{label, cmd}` pairs:
    `equip`/`unequip`/`drop` plus `reload` (a holder-fed firearm) / `load` (a
    reload-gated crossbow). Each `cmd` is the FULL command string (bare `reload`
    for a wielded gun vs `reload <clip>` for a carried clip), so the client sends
    exactly what a player would type — the authority invariant, no new server
    verb. A rich superset of `Char.Items.List` (which stays for baseline
    clients), emitted on the same `gmcp-items-flush` poll pass with its own
    marshaled-bytes shadow; a baseline client ignores it. Ruleset-agnostic wire.
    Guarded by `internal/gmcp` payload-shape tests, `internal/session`
    builder/flusher tests (`gmcp_inventory_test.go`), and the env-gated
    `TestLive_GmcpInventory` (live boot: carried + the 9-slot worn layout on the
    wire).
  - **Slice B — the craft form. DONE (`Char.Recipes`).** The first form:
    `Char.Recipes` (server→client) — the character's KNOWN recipes with, per
    recipe, the ingredients it needs (have/need counts), the required station
    tier + whether the present station (room ∪ carried tools) meets it, whether
    the skill floor is met, an overall craftable-now flag, a short plain-text
    block reason when not, and the full `craft <recipe>` submit command. A rich
    superset of the `craft` no-arg listing (which stays plain text for a
    baseline client). Built read-only by `crafting.Service.CraftForm` (mirrors
    Craft's skill/station/ingredient gates without mutating), emitted
    poll-and-diff on the same `gmcp-items-flush` pass as `Char.Inventory` (so
    have/need updates as ingredients are gathered and station-met flips at a
    forge). **Naming note:** the §2.2 sketch said `Client.Form`, but its own
    rule — "extend `Char.*` for a richer view of an existing concept; reserve
    `Client.*` for UI-control" — puts craftable recipes under `Char.*`. Submit
    is NOT a new inbound package: each row's `cmd` is a plain `craft <local-part>`
    the client sends verbatim (the authority invariant, identical to Slice A's
    affordances). The web client renders a Crafting panel (greying out
    unmakeable rows, ingredient shortfalls in red) that a non-crafter simply
    hides. Guarded by `internal/gmcp` payload-shape tests,
    `internal/crafting` CraftForm unit tests, `internal/session`
    builder/flusher tests (`gmcp_recipes_test.go`), and the env-gated
    `TestLive_GmcpRecipes` (login → Char.Recipes on the wire). Trade forms and
    the web-admin console (BACKLOG) follow the same concrete-package pattern.
  - **Slice B+ — the shop form. DONE (`Char.Shop`).** The trade form, and the
    first CONTEXTUAL package: `Char.Shop` (server→client) carries an `open` flag
    (false + empty offers when the player is not at a shop, so the client hides
    the panel) plus, when open, the shop NPC's name, the shopper's formatted
    balance, a `buy` list (the shop's skill-passable stock, each row priced +
    marked affordable), and a `sell` list (the player's carried sellable items,
    grouped by kind with a qty + sell price). Faction access-refusal surfaces as
    `refused`. Built read-only by `economy.ShopService.ShopForm` (mirrors the
    buy/sell gates — faction §6, the §7 skill gate, no-sell/zero-value — and
    prices through the same `buyPrice`/`sellPrice`); the session formats prices
    via the world `CurrencyLabel` so the wire carries no currency vocabulary.
    Emitted poll-and-diff on the same items pass as the craft form, so entering/
    leaving a shop, spending money, or picking up a sellable item all re-emit.
    Submit is a plain `buy <token>`/`sell <token>` command (authority invariant —
    no new server verb). The web client renders a two-column shop panel. Guarded
    by `internal/gmcp` payload-shape tests, `internal/economy` ShopForm unit
    tests, `internal/session` flusher tests (`gmcp_shop_test.go`), and the
    env-gated `TestLive_GmcpShop`.
  - **Slice C — the quest journal. DONE (`Char.Quests`).** The journal form:
    `Char.Quests` (server→client) carries the character's ACTIVE quests, each
    with its display name + classification, the current stage's description +
    optional hint, and the per-objective progress rows (current/required + a
    `complete` flag) — mirroring the `quests` verb's journal panel. An
    awaiting-turn-in quest surfaces an `awaitingTurnIn` flag (the client shows a
    "ready to turn in" badge; turn-in is done by returning to the giver, not a
    bare command, so there is no submit button for it). Each abandonable quest
    carries `abandonable: true` + an `abandonCmd` (`abandon <id>`) the client
    sends verbatim (the authority invariant — no new server verb). Built
    read-only in the session flusher from `quest.Service.Snapshot` +
    `Definition` (the SAME projection `command.QuestsHandler` renders, so the
    panel and the CLI never drift). Emitted poll-and-diff on the same items pass
    as the craft/shop forms, so accepting a quest, advancing an objective, or
    completing a stage all re-emit. The web client renders a Journal panel that
    hides when there are no active quests. Guarded by `internal/gmcp`
    payload-shape tests, `internal/session` flusher tests (`gmcp_quests_test.go`),
    and the env-gated `TestLive_GmcpQuests`.
  - **Slice B++ — the direct-trade form. DONE (`Char.Trade`).** The live two-party
    trade panel, and the second CONTEXTUAL package: `Char.Trade` (server→client)
    carries an `open` flag (false + empty sides when the player has no trade, so the
    client hides the panel) plus, when open, both sides' staged offers from the
    VIEWER's perspective — each side's items, pre-formatted coin, and a `confirmed`
    flag that ticks as either party stages value (the surface plain text serves
    worst — you must re-type `trade` to re-read it). Built read-only by
    `trade.Manager.View` (the same offer data the `trade` verb prints, structured).
    Submit is plain: the viewer's own items carry a `rescind <name>` command, and the
    whole trade is confirmed/cancelled with the fixed `confirm` / `decline` verbs the
    client sends literally (authority invariant — the partner's items are display-
    only; the server is the sole judge of the swap, requiring both confirms). Emitted
    poll-and-diff on the same items pass, so an offer added on EITHER side re-emits.
    The web client renders a two-column staging panel. Guarded by `internal/gmcp`
    payload-shape tests, an `internal/trade` `View` unit test, and `internal/session`
    flusher tests (`gmcp_trade_test.go`).
  - **Slice B++ — the auction-house form. DONE (`Char.Auction`).** The marketplace
    form, and the third CONTEXTUAL package: `Char.Auction` (server→client) carries an
    `open` flag (false + empty listings when no auctioneer is present, so the client
    hides the panel) plus, when open, the shopper's formatted balance, the active
    listings (first page, soonest-closing — each priced + marked affordable + carrying
    a compact closing countdown, the viewer's own flagged), the total active count,
    and the viewer's pending pickups + proceeds. Built read-only by
    `auction.Manager.Form` (composing the existing `Browse` + pending-pickup queries);
    the session formats prices via the world `CurrencyLabel` + judges affordability
    against the viewer's balance (like `Char.Shop`). Submit is plain: `buyout <ref>`
    for another seller's listing, `unlist <ref>` for your own, and the fixed `collect`
    for pending pickups (authority invariant — no new server verb). Emitted poll-and-
    diff on the same items pass, so a new listing / buyout / spent coin re-emits;
    closing times count down at per-minute granularity so the panel ticks without
    re-emitting every tick. The web client renders a listings panel with a collect
    banner. Guarded by `internal/gmcp` payload-shape tests, an `internal/auction`
    `Form` unit test, and `internal/session` flusher tests (`gmcp_auction_test.go`).
  - **Slice B++ — remaining forms.** Portraits, responsive/mobile — each one additive
    package (server state) + a plain-command submit, never new authority. Six concrete
    form packages now exist (`Char.Recipes`, `Char.Shop`, `Char.Quests`, `Char.Trade`,
    `Char.Auction`, and any next); if a shared shape earns its keep, generalize then —
    not before.

## 5. Open questions (non-blocking; settle when the phase reaches them)

- **Auth/session over WS for a public web client.** Today WS logs in through the
  same account flow as telnet. A public browser client wants token/cookie
  session resumption, CSRF/origin checks on the upgrade, and rate limiting — the
  `m16-5` WebSocket TLS/rate-limit/IP-cap deferral is the hook. Design when P1
  goes past localhost.
- **High-frequency state (positions, combat ticks).** GMCP poll-and-diff per tick
  is fine for vitals; a real-time map with many movers may want coalescing or a
  delta channel. Measure in P2 before optimizing; a new envelope `type` for a
  binary/delta channel is the escape hatch if JSON-per-tick is too chatty.
- **Client rendering stack.** Framework/build choices are a client concern, out
  of this (server-side-contract) doc. Constraint from the invariants: the client
  is a *view* — no rules, no authoritative state.
- **`Client.Hello` capability announce.** Deferred (§2.4). Add only to gate rich
  emissions when bandwidth to non-web clients is measurably wasteful.
- **Where do aliases / triggers / macros live?** RESOLVED (2026-07-17) →
  **server-side + GMCP**: aliases/macros/speedwalk go in the player save + a new
  `Char.Aliases`-style package so config roams across Mudlet ↔ web; expansion
  runs server-side (authority invariant intact); trigger *patterns* roam but
  match/render stays per-client. Full rationale + division-of-labor in
  [`../clients/parity-matrix.md`](../clients/parity-matrix.md).

## 6. What this does NOT change

No server code changes fall out of *this decision* — it resolves the fork so P1
can start against the wire as it stands. The contract above is the rule every
future web slice follows; the first code is the P1 browser shell, which needs
nothing new from the engine. When P2's first additive package lands, the
networking spec (§5/§6) gets a short forward note that the GMCP package set is
open-ended and rich packages are additive + ignorable — but that waits for
shipped behavior, per the specs-describe-what-exists convention.

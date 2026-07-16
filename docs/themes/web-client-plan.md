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
- **P3+ — enriched surfaces.** Structured inventory (drag-drop → equip/drop),
  crafting/trade **forms** (`Client.Form` + submit → the craft/trade services),
  portraits, responsive/mobile. Each is one additive package (server state) + one
  intent (client→command), never new authority. The web-admin console (BACKLOG)
  is a natural tenant.

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

## 6. What this does NOT change

No server code changes fall out of *this decision* — it resolves the fork so P1
can start against the wire as it stands. The contract above is the rule every
future web slice follows; the first code is the P1 browser shell, which needs
nothing new from the engine. When P2's first additive package lands, the
networking spec (§5/§6) gets a short forward note that the GMCP package set is
open-ended and rich packages are additive + ignorable — but that waits for
shipped behavior, per the specs-describe-what-exists convention.

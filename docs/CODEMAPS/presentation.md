<!-- Generated: 2026-07-08 | Client-facing layer (telnet/WS + GMCP; web client = recorded direction, not built) | Token estimate: ~860 -->

# Presentation & Networking

No web frontend exists **yet** — clients are line-oriented terminals + GMCP-aware
clients (Mudlet). A browser **web UI over the existing WS+GMCP channel is the
recorded long-term rich-client direction** (docs/BACKLOG.md, "Web client"); the
Mudlet HUD path is paused. This is the "frontend" analog: transports, protocol
negotiation, GMCP data channel, and server-side rendering.

## Transports
```
TCP :4000  ─▶ internal/conn/telnet  ─▶ session.Actor ─▶ command.Dispatch
WS  :4001  ─▶ internal/conn/ws      ─┘   (one-text-frame JSON envelopes)
             internal/server (listeners, WS HTTP upgrade)
```
- **telnet** (`conn/telnet`): full IAC negotiation, GMCP subneg, TTYPE-derived
  client identity, capability/color-tier detection.
- **ws** (`conn/ws`): `coder/websocket`; always GMCP + ANSI, no per-client
  negotiation. JSON envelopes (`{type,package,data}`).
- Both implement `conn.GmcpConn` (SetGmcpHandler/SendGmcp/SupportsPackage) — the
  seam the session installs an inbound handler through.

## GMCP (bidirectional)
`internal/gmcp` — wire shapes + package-name constants. **Server→client** (push):
Char.Vitals/Status/StatusVars/Login/Combat/Effects/Experience/Items.List,
Room.Info (per-viewer `light` level + optional area-local `x`/`y`/`z` room
coordinates, omitted when unplaced; `name`/`details` run through `gmcpPlain`
so `{color}` brace/angle markup never leaks into a graphical client's labels),
Comm.Channel.Text — flushed on cadence-1 tick handlers (poll-and-diff)
in `main.go`. Identical frames go over telnet SB and the WS envelope.
**Client→server** (request/response): `Input.Complete` /
`Input.Complete.List` (tab-completion §13) — inbound frames dispatched on both
transports to a session handler (`session/gmcp_complete.go`), per-connection
rate-limited (token bucket, never disconnects). `internal/mssp` = MUD server
status vars on connect.

**Known gap:** `Char.Vitals` emits only hp/maxhp/sustenance today; the
`mp/maxmp/mv/maxmv` fields exist (`omitempty`) but `flushGmcpVitals` doesn't
populate the generalized `internal/pool` currents (mana/movement/One Power) —
a prerequisite before any resource-bar HUD (web or Mudlet).

## Rich clients & GMCP tooling
- **Mudlet mapper** (`clients/mudlet/AnotherMud-Mapper.lua` + README): a
  coordinate-accurate GMCP mapper driven by `Room.Info` x/y/z. **Validated live**
  (Mudlet 4.21, 2026-07-08) — rooms place geographically, fog-of-war exit stubs,
  terrain/`light`-fallback env colors. Requires disabling Mudlet's bundled
  `generic_mapper` (both bind `gmcp.Room.Info` and fight). Paste-in Lua, not an
  `.mpackage`.
- **GMCP verification** (`cmd/telnet-smoke -gmcp`): headless probe — activates
  GMCP, logs in, dumps captured frames, walks one exit to confirm per-transition
  `Room.Info`. Backed by `internal/telnettest` GMCP capture
  (`WithGMCPCapture`/`ActivateGMCP`): the send/expect harness now parses `SB GMCP`
  frames instead of dropping them, and actively sends `IAC DO GMCP` (which the
  plain harness never did, so pre-existing live tests never exercised GMCP).

## Rendering
- `internal/render` (1.4k LOC) — room/look output, exits, item listings,
  decoration + stacking integration, weather ambience line.
- **Panel renderer** (`render/panel.go`, ui-rendering-help §8) — framed,
  width-aware, tag-aware ASCII boxes (`| = -` wrapped in `<frame>`, so no
  glyph-fallback debt). Powers the `score`/`sc` character sheet: a bento layout
  (Character|Combat and Attributes|Purse&Training two-column sections, full-width
  Equipment, XP footer). `equipment`/`eq` shares the sheet's equipment gatherer
  (`command.gatherScoreEquip`); both color item names by `item.*` rarity.
- **Semantic color tags** (`<title>/<subtle>/<highlight>/hp/mana/mv/good/warning/
  danger/gold/frame/item.*/exit/present.*/weather.*/time.*>`) are emitted by the
  renderers and defined in `content/core/theme/theme.yaml` (pack-overridable;
  unknown tags pass through as literal text).
- **Light-and-darkness render states** (`command.RenderRoom` branches on the
  per-viewer effective light): `lit` = full render; `dim` = full body, prose
  muted (`{dim}`); `gloom` = obscured (terse dark line, anonymous occupants,
  bare-direction exits); `black` = single "you can see nothing" line. All
  degrade to clean text (markup the renderer strips). `daylight` verb probes
  time + light.
- **Visibility filtering** (M28): the room render also filters the occupant +
  exit lists through the per-observer `visibility` predicate — concealed
  (hidden/sneaking/invisible) occupants the viewer fails to detect are omitted,
  and an undiscovered hidden exit is unlisted (and unwalkable). Composes with the
  light gate above.
- **Builder room-data overlay** (`command.AppendRoomData`): a single seam
  every "you see the room" render routes through — `look`, movement, recall,
  teleport, flee, login spawn, link-dead reattach — appending an admin
  metadata block (room/area ids, coordinate + source, terrain + flags, tags,
  properties, healing, per-exit targets with door state) **outside** the light
  gate. Double-gated: viewer holds the admin role **and** has the persisted
  `roomdata` toggle on (`player.Save.ShowRoomData`). Builder role wiring is
  deferred to OLC.
- `internal/ansi` — tiered ANSI emission (plain / 16 / 256 / truecolor) keyed off
  the connection's detected ColorTier; `{X}`-style pack color markup expansion.
- `internal/help` — help topics + categories (auto-synthesis from arg defs is
  backlog, not built).

## Capability tiers
TTYPE / terminal negotiation → ColorTier enum → `ansi` emits the matching tier;
dumb telnet degrades gracefully. No committed first-class raw-`telnet`/`nc`
parity beyond graceful degradation.

## Key files
`internal/conn/telnet/` (negotiator, gmcp, color), `internal/conn/ws/ws.go`,
`internal/server/`, `internal/gmcp/gmcp.go`, `internal/session/gmcp_*.go`
(payload builders + `flushGmcp*`), `internal/render/`, `internal/ansi/`.
Clients/tooling: `clients/mudlet/`, `cmd/telnet-smoke/` (`-gmcp` probe),
`internal/telnettest/` (send/expect + GMCP capture).

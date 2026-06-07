<!-- Generated: 2026-06-06 | Client-facing layer (no web frontend) | Token estimate: ~640 -->

# Presentation & Networking

There is **no web frontend** — clients are line-oriented terminals. This is the
"frontend" analog: transports, protocol negotiation, and server-side rendering.

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
Room.Info (carries a per-viewer `light` level field), Comm.Channel.Text —
flushed on cadence-1 tick handlers (poll-and-diff)
in `main.go`. **Client→server** (request/response): `Input.Complete` /
`Input.Complete.List` (tab-completion §13) — inbound frames dispatched on both
transports to a session handler (`session/gmcp_complete.go`), per-connection
rate-limited (token bucket, never disconnects). `internal/mssp` = MUD server
status vars on connect.

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
`internal/server/`, `internal/gmcp/gmcp.go`, `internal/render/`, `internal/ansi/`.

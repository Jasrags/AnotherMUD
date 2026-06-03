<!-- Generated: 2026-06-03 | Client-facing layer (no web frontend) | Token estimate: ~600 -->

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
Room.Info, Comm.Channel.Text — flushed on cadence-1 tick handlers (poll-and-diff)
in `main.go`. **Client→server** (request/response): `Input.Complete` /
`Input.Complete.List` (tab-completion §13) — inbound frames dispatched on both
transports to a session handler (`session/gmcp_complete.go`), per-connection
rate-limited (token bucket, never disconnects). `internal/mssp` = MUD server
status vars on connect.

## Rendering
- `internal/render` (1.4k LOC) — room/look output, exits, item listings,
  decoration + stacking integration, weather ambience line.
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

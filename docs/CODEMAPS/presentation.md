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
  client identity, capability/color-tier detection. Inbound non-Core.Supports
  GMCP handler exists but is unwired (client→server GMCP is server-receive-only
  for Core.Supports today).
- **ws** (`conn/ws`): `coder/websocket`; always GMCP + ANSI, no per-client
  negotiation. Inbound GMCP frames currently dropped.

## GMCP (server→client only today)
`internal/gmcp` — packages: Char.Vitals/Status/StatusVars/Login/Combat/Effects/
Experience/Items.List, Room.Info, Comm.Channel.Text. Flushed on cadence-1 tick
handlers (poll-and-diff) wired in `main.go`. `internal/mssp` = MUD server status
vars on connect.

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

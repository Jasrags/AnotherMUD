# Theme B — Modern Client (plan)

**Hook:** Mudlet, MUSHclient, Blightmud, and browser clients see
real HUDs and panels instead of just scrolling text.

**Source:** `docs/THEME-AXIS-PLAN.md` §"Theme B — Modern Client".
**Roadmap milestone:** M16 (to be added when first slice ships).
**Status:** spec phase complete — `docs/specs/networking-protocols.md`
covers everything; implementation slices to be sequenced.

---

## What the spec already says

`networking-protocols.md` is the most complete spec we have. The
relevant sections:

- **§2 IConnection contract** — already implemented; the
  per-connection telnet substrate exists at `internal/conn/telnet`
  with line-oriented reads and basic IAC strip on input.
- **§3 Telnet transport** — listener, accept independence, per-
  connection state, read/write/echo control. M0-era code covers
  the basics; subnegotiation buffer + IAC-in-IAC escaping are the
  gaps.
- **§4 Telnet negotiation** — TTYPE, NAWS, MSSP, GMCP, ECHO. The
  full sequence (§4.3) is documented; nothing is implemented today.
- **§5 GMCP** — wire format (telnet + WebSocket), `Core.Supports`
  protocol, activation gating, inbound packages.
- **§6 WebSocket transport** — listener, envelope, GMCP route.
- **§7 GMCP packages** — `Char.Vitals`, `Room.Info`, `Char.Items`,
  `Char.Combat`, `Char.Effects`, `Char.Experience`, `Comm.Channel`,
  `Char.Login`, `Char.StatusVars`, etc.
- **§8 MSSP** — variable table + reply format.

---

## The six items (with sequence rationale)

Per `docs/THEME-AXIS-PLAN.md` Theme B, the suggested order is:

```
M16.1 — Telnet IAC + TTYPE + NAWS   (cheapest, immediate win)
   ↓
M16.2 — MSSP variables               (gets us on MUD listings)
   ↓
M16.3 — GMCP option negotiation + envelope
   ↓
M16.4 — GMCP packages (priority order — multiple sub-slices)
   ↓
M16.5 — WebSocket transport          (parallel-shippable)
   ↓
M16.6 — 256/truecolor follow-up      (consumable once clients advertise)
```

Each item below carries its shape estimate and a "what visible
demo lands when this ships" note.

### M16.1 — Telnet IAC negotiation: TTYPE + NAWS

**Spec:** §4.1-§4.4 (option codes, budget, sequence), §3.3-§3.4
(per-connection state, read-loop subnegotiation handling).

Implement the IAC subnegotiation state machine. Add the two
cheapest options: TTYPE (client identifies itself — `Mudlet`,
`Blightmud`, `xterm`, …) and NAWS (client window size — width
and height in characters).

**Shape:** small-medium. ~1 week. The state machine is fiddly
(spec §3.4 covers IAC mid-line, IAC-in-IAC, IAC-then-SE
boundary) but bounded; once it exists, options are config
table entries.

**Demo target:** `who` listing shows each session's client type;
the prompt-flush path uses NAWS width to soft-wrap long lines.
Even bare telnet clients benefit because the negotiation is
silent on the wire.

### M16.2 — MSSP variables

**Spec:** §8 (variable table + reply format), §4.1 (MSSP option
code).

Add the MSSP option handler. Reply with the spec §8 variable
list (NAME, PLAYERS, UPTIME, CODEBASE, FAMILY, GENRE,
LANGUAGE, CONTACT, CHARSET, ANSI, MCCP=0, GMCP=0-until-M16.3,
…). Driven by a small `internal/mssp` package that reads from
the composition root's config + live session manager.

**Shape:** small. ~3-5 days. Mostly content — the protocol is
trivial once §4's subnegotiation buffer exists.

**Demo target:** Run `tin-server-check` (or similar MUD-listing
crawler) against the server and see the variable list. Submit
to grapevine.haus / MUD lists.

### M16.3 — GMCP option negotiation + envelope

**Spec:** §5.1 (telnet wire format), §5.3 (Core.Supports),
§5.5 (activation).

Implement the GMCP option handshake and the
`<package>.<command> <json-payload>` envelope. Wire the
`Core.Supports.Set` / `Add` / `Remove` package which is how
clients tell us which other packages they want. No packages
shipped yet — this slice is the transport.

**Shape:** medium. ~1-2 weeks. The wire format is mechanical;
the Core.Supports state machine + per-connection subscription
set is the real work.

**Demo target:** A Mudlet client negotiates GMCP, sends
`Core.Supports.Set ["Char.Vitals 1"]`, the server logs the
subscription but emits no `Char.Vitals` packets yet.

### M16.4 — GMCP packages (multiple sub-slices)

**Spec:** §7 (per-package contracts) + the
feature-spec each package draws from (combat, inventory,
progression, …).

Priority order per the Theme B plan:

```
M16.4a — Char.Vitals       (most-watched, the headline package)
M16.4b — Room.Info         (renderer + map panel)
M16.4c — Char.Items        (inventory panel)
M16.4d — Char.Combat       (combat HUD)
M16.4e — Char.Effects      (active effects panel)
M16.4f — Char.Experience   (XP bar)
M16.4g — Comm.Channel      (chat panel — already half-built; spec
                            chat-channels-and-tells §11 has the route)
M16.4h — Char.Login + Char.StatusVars  (boot/identity)
```

Each sub-slice is small (~3-5 days) once M16.3's transport
exists. The exception is **Char.Vitals** (M16.4a) because of the
dirty-batching pre-decision — see PD-3 below. Vitals is the
stampede vector: it fires on every HP / mana / sustenance /
movement tick across every active session, and naive
"publish-on-every-change" would saturate the wire.

**Shape per sub-slice:** small. ~3-5 days each. The whole §7
batch is 3-5 weeks total.

**Demo target:** Mudlet user connects with the bundled MUD
profile, sees HP/Mana bars update in real time, room name +
exits in a panel, inventory updates as `get` / `drop` fire.

### M16.5 — WebSocket transport

**Spec:** §6 (listener, envelope), §5.2 (GMCP wire format on
WebSocket — JSON envelope instead of IAC subnegotiation).

Add an HTTP listener that upgrades to a WebSocket and adapts to
the existing `internal/conn.Connection` interface. GMCP packages
defined in M16.3+ route through the same emit path; only the
envelope changes.

**Shape:** medium. ~1-2 weeks. Parallel-shippable with the
M16.4 sub-slices because `internal/conn.Connection` is clean.

**Demo target:** Browser at `ws://server:port/mud` connects,
upgrades, sees the same GMCP packages a Mudlet client sees over
telnet.

### M16.6 — 256/truecolor follow-up

**Spec:** `ui-rendering-help` §3 (theme + color tier), plus
TTYPE values that advertise capability (`xterm-256color`, etc.).

The renderer already supports semantic tags via
`render.ColorRenderer` (M10.2). Today it emits ANSI-16. Once
TTYPE (M16.1) tells us a client supports 256 or truecolor, the
renderer can switch tiers per-session.

**Shape:** small-medium. ~1 week. Per-session render tier
selection + a few content theme updates that take advantage of
the wider palette.

**Demo target:** Mudlet (advertises `Mudlet` TTYPE which we know
supports truecolor) sees richer room descriptions; bare
`xterm-color` client still gets the ANSI-16 path.

---

## Open pre-decisions

Lifted from `docs/THEME-AXIS-PLAN.md` Theme B. Decide ones marked
**BEFORE** their slice; defer the rest.

### PD-1 — Per-client subscribe model: every package on by default vs. opt-in

**Affects:** M16.4 (every package).

The spec §5.3 `Core.Supports` is "explicit opt-in" by contract —
clients name the packages they want. But the engine has freedom
in how it *defaults* before the client has sent its Supports set:
silent until first opt-in, or push everything until the client
narrows.

Recommended: opt-in only (silent until `Core.Supports.Set`).
Matches the spec letter and avoids burning bandwidth on packages
the client will discard. **Decide before M16.3 ships.**

### PD-2 — Payload shape: clone Tapestry's JSON or design our own

**Affects:** M16.4 (every package).

Tapestry has a stable GMCP payload schema with deep history.
Cloning gets us instant compatibility with existing client
plugins. Designing our own JSON saves us from carrying
warts but loses every existing Mudlet profile out there.

Recommended: clone Tapestry's where they have one; design our own
only for packages Tapestry never shipped. **Decide before M16.4a
ships** (Char.Vitals).

### PD-3 — Dirty-batching strategy for Char.Vitals

**Affects:** M16.4a.

Vitals fires per-tick across every session. Naive emit-on-every-
change is a stampede; emit-once-per-tick-with-coalesce is the
known good shape. The implementation MUST land at v1 because
retrofitting "now actually batch" later means every consumer
needs to re-test against the new cadence.

Three options:
- Per-session dirty bit + tick-boundary flush (recommended;
  matches the existing `prompt-flush` cadence handler).
- Bus-event-coalesced flush (every emitter publishes, the GMCP
  layer dedupes per tick).
- Time-window batching (every Nms flush).

Recommended: per-session dirty bit, flushed by a new
`gmcp-vitals-flush` cadence-1 tick handler. Mirrors the
existing prompt-flush pattern. **Decide before M16.4a ships.**

### PD-4 — HTTP listener: same process as telnet vs. separate binary

**Affects:** M16.5.

Same-process is simpler (one set of locks, one bus, one
session manager) and matches the current `internal/server` shape.
Separate binary is operationally cleaner (independent scaling,
TLS termination outside).

Recommended: same process. We're not at scale where ops
separation matters; the simplicity wins. Revisit if the WebSocket
session count outgrows the telnet session count by 10×.
**Decide before M16.5 ships.**

### PD-5 — TTYPE rotation policy

**Affects:** M16.1.

TTYPE clients return multiple identifiers when re-queried (RFC
1091 + 2066). Some clients (Mudlet) report `Mudlet` first then
the terminal type on subsequent queries; others (TinTin++) report
one and stop. The §4.3 negotiation budget caps how many queries
we make.

Recommended: query twice, capture both, store as
`primary` + `secondary` on the connection's capability set. If
the second query returns the same value as the first, treat that
as "client stopped rotating." Matches spec §4.3. **Decide
inside M16.1.**

---

## Shape estimate

6-10 weeks per the theme plan. Breakdown:

| Slice | Weeks |
|---|---|
| M16.1 IAC + TTYPE + NAWS | 1 |
| M16.2 MSSP | 0.5 |
| M16.3 GMCP transport | 1-2 |
| M16.4 GMCP packages (8 sub-slices) | 3-5 |
| M16.5 WebSocket (parallel) | 1-2 |
| M16.6 256/truecolor | 1 |

The M16.4 sub-slices run sequentially because they all build on
M16.3, but each is small enough to land in one session.

---

## What blocks what

- M16.2-M16.6 all depend on M16.1's IAC subnegotiation machinery.
- M16.4 depends on M16.3's GMCP envelope.
- M16.5 is parallel-shippable with M16.4 (different transport,
  same package payloads).
- M16.6 depends on M16.1's TTYPE capture (so the renderer knows
  what tier the client supports).

---

## Demo target (whole theme)

A new player launches Mudlet, connects to the public server,
loads the bundled MUD profile, and immediately sees:

- HP / Mana / Movement bars updating as they take damage and rest.
- Room name + description in a side panel that updates on move.
- Inventory panel that reflects `get` / `drop` / `equip` in real
  time.
- Chat channel tab that respects the same notification queue the
  CLI uses (no double-receipt).
- The same server discoverable on grapevine.haus via MSSP.

And a separate browser-based client at `ws://server/mud` shows
the same panels — no per-client adapter code, just transport
differences.

# Networking and Protocols — Feature Specification

**Status:** Draft · **Scope:** The connection abstraction shared by
telnet and websocket transports; the telnet listener and per-
connection lifecycle; telnet option negotiation (TTYPE, NAWS, MSSP,
GMCP, ECHO); input parsing and output framing including subnegotiation;
the websocket transport with its JSON envelope; GMCP package semantics
on both transports; the MSSP variable table; client capability
derivation; and ANSI text presentation · **Audience:** Anyone
reimplementing or porting this feature in any language.

This document describes *what* the networking feature must do, not
*how* to implement it. The telnet option code numbers and the MSSP /
GMCP wire formats are interoperability contracts (they're defined by
external RFCs and conventions, not by this codebase) and are therefore
specified concretely; everything above the wire is policy.

---

## 1. Overview

The networking feature is the boundary between the outside world and
the engine. It accepts TCP telnet connections and HTTP-upgraded
websocket connections, negotiates whatever protocol options each
client wants, parses bytes into command lines, and frames text and
out-of-band messages back the other way. It produces a uniform
**IConnection** value that the engine uses without caring which
transport is underneath.

### Core concepts

- **IConnection** — the engine-facing interface a transport
  produces. Carries identity, capability, send / disconnect /
  echo-control primitives, and event hooks for input and
  disconnect.
- **Transport** — telnet (raw TCP with IAC option codes) or
  websocket (JSON envelopes over HTTP-upgrade).
- **Protocol handler** — a registered handler for one telnet
  option code (TTYPE, NAWS, MSSP, GMCP). Handlers negotiate at
  connection start and (when marked session-long) continue to
  receive their option's subnegotiation bytes for the rest of
  the session.
- **Capabilities** — the immutable record produced once
  negotiation completes: client name (terminal type), window
  size, GMCP support, color support tier, and the
  server-vs-client echo decision.
- **GMCP package** — a named out-of-band message channel
  (e.g. `Char.Vitals`, `Room.Info`) with JSON payloads. The
  client may declare which packages it supports via
  `Core.Supports.Set` / `Core.Supports.Remove`.
- **MSSP variable** — a server-discovery datum (NAME, PLAYERS,
  UPTIME, …) served once on demand to crawlers.

### Goals

1. Present a single, transport-independent IConnection to the
   engine.
2. Negotiate telnet options correctly enough that mainstream MUD
   clients (Mudlet, MUSHclient, TinTin++, etc.) get a clean
   session.
3. Time-box negotiation so a misbehaving client cannot stall the
   listener.
4. Sand off the telnet wire's edge cases — IAC escaping, mid-
   stream subnegotiation, echo control — so the engine never
   sees raw IAC bytes or sentinels.
5. Provide GMCP as a uniform engine-facing channel on both
   telnet (via subneg) and websocket (via JSON envelope).
6. Cap per-connection memory: input lines, total buffered input,
   single websocket messages.
7. Emit MSSP and ANSI in a way that doesn't depend on engine
   state above the transport layer.

### Non-goals

- TLS / WebSocket-Secure termination. The reference
  implementation listens plaintext; a TLS layer goes in front
  (reverse proxy, TLS terminator).
- Compression (MCCP). Advertised as "0" in MSSP and unimplemented.
- Telnet ENVIRON / MNES / CHARSET / MXP. The current option set
  is TTYPE, NAWS, ECHO, MSSP, GMCP only.
- The shape and semantics of specific GMCP packages
  (`Char.Vitals`, `Room.Info`, etc.). Those are content /
  feature concerns; this spec covers only the GMCP transport.
- Rate limiting, flood control, or session phase. Those are the
  session-layer features (see `docs/specs/login.md` and the
  session/connection-lifecycle spec).
- IDS / abuse heuristics beyond the basic per-connection size
  caps.
- IPv6-specific behavior. The current listener binds `IPAddress.Any`
  which is IPv4; v6 is a configuration / deployment concern.

---

## 2. The IConnection contract

Every transport produces an IConnection. The contract requires:

### 2.1 Identity and state

- **Id.** A stable, opaque per-connection identifier (any
  format; today GUID-like). Used by session bookkeeping.
- **IsConnected.** Boolean true while the transport believes the
  client is reachable.
- **SupportsAnsi.** Boolean indicating whether ANSI escape
  sequences will render meaningfully. Driven by negotiation
  (telnet) or set true (websocket).
- **RemoteAddress.** Optional string of the client's network
  address (IP portion only, no port). May be null when unknown.

### 2.2 Send primitives

- **SendText(s).** Send `s` verbatim. The caller chooses line
  endings.
- **SendLine(s).** Send `s` followed by CR-LF. The CR-LF pair
  is the canonical end-of-line on both transports — telnet
  clients expect it; websocket clients receive it inside the
  JSON `data` field.
- **ClearScreen.** Send the platform's clear-screen sequence
  when SupportsAnsi; no-op otherwise.

### 2.3 Echo control

- **SuppressEcho.** Stop echoing input back to the client.
- **RestoreEcho.** Resume echoing.

Echo control is required by the login flow (passwords). The
mechanism is transport-specific (§3.6, §5.4) but the engine-
facing contract is uniform.

### 2.4 Disconnect and events

- **Disconnect(reason).** Begin connection teardown with a
  short reason string. Idempotent — multiple calls collapse to
  one teardown.
- **OnInput(line).** Fires once per completed input line, with
  the line's content (line-ending removed).
- **OnDisconnected.** Fires once after disconnect.
- **OnDisconnectedWithReason(reason).** Fires once with the
  reason string, before or with OnDisconnected.

Order of events: zero or more OnInput; then
OnDisconnectedWithReason; then OnDisconnected. Implementations
MUST ensure both disconnect events fire exactly once even if
multiple teardown paths race.

**Acceptance criteria**

- [ ] Id is stable for the connection's lifetime.
- [ ] SupportsAnsi reflects negotiation result for telnet.
- [ ] SendLine produces CR-LF on both transports.
- [ ] Disconnect is idempotent.
- [ ] OnInput fires exactly once per completed line.
- [ ] Both disconnect events fire exactly once per connection.

---

## 3. Telnet transport

### 3.1 Listener

The telnet listener:

1. Binds a TCP socket on the configured port to all addresses.
2. Logs that it is listening.
3. In an accept loop, awaits each new TCP client.
4. For each accepted client, constructs a TelnetConnection,
   logs the remote endpoint, runs negotiation (§4), and starts
   the connection's read loop (§3.4). Emits the
   `OnConnectionAccepted` event so the session layer can bind
   the new IConnection to a pre-login session.

The listener owns a list of active connections (under a mutex)
solely for shutdown: on cancellation it stops the underlying
listener and calls Disconnect("server shutdown") on every
active connection.

### 3.2 Acceptance independence

Negotiation failure for one connection MUST NOT block the
listener. If negotiation throws (timeout, broken client), the
listener logs and sets the connection to defaults
(§4.5), then proceeds to start the read loop normally. A
client that cannot negotiate but can still type is a usable
session.

### 3.3 Per-connection state

A TelnetConnection holds:

- The TCP client and its stream.
- A capabilities record (initially default; replaced after
  negotiation).
- A protocol router (attached after negotiation) for routing
  subnegotiation bytes to handlers.
- An input buffer (StringBuilder) accumulating the current
  line.
- A subnegotiation state-machine triplet: `inSubneg` boolean,
  current option code, and a byte buffer.
- An echo flag.

The TCP socket is configured with `NoDelay = true` so single-
character interactive input echoes back without Nagle delay.

### 3.4 Read loop

The read loop reads up to a fixed-size buffer (today 1 KiB) at
a time. For each byte it consults the state machine:

- **In subneg.** Append to the subneg buffer unless the byte is
  IAC followed by SE — in that case, end subneg, hand the
  collected payload to the router, clear the buffer, and skip
  the SE byte.
- **Start of IAC.** Inspect the next byte:
  - `SB` (subneg begin) — read the next byte as the option
    code, clear the subneg buffer, set `inSubneg = true`.
  - WILL / WONT / DO / DONT — these are 3-byte option commands;
    skip them. (After negotiation, the engine does not
    react to further negotiate-style messages; protocol
    handlers are wired during negotiation only.)
  - Any other byte — treat the IAC + byte as a 2-byte sentinel
    and skip both.
- **End of line.** A `\n` byte completes the current input line.
  If the echo flag is set, emit a CR-LF echo to the client. The
  current input buffer (with any trailing `\r` trimmed) becomes
  the line; the buffer is cleared and OnInput fires.
- **Carriage return.** Ignored. Line completion happens on `\n`.
- **Backspace / DEL.** Bytes 8 and 127. Remove one character
  from the input buffer; if echo is enabled, send BS-space-BS
  so the client visually erases.
- **Printable.** Bytes ≥ 32. Append to the input buffer; echo
  back when enabled. If the buffer reaches the line-length cap
  (today 4096), discard it and send "Input too long,
  discarded." to the client.

The read loop also enforces a **total buffered input cap** (today
65536 bytes). If exceeded, the connection is closed with reason
`input buffer overflow`. Client-closed (zero-length read) ends
the loop with reason `connection closed`. Exceptions are logged
and end the loop with `read error: <message>`.

### 3.5 Write

The transport exposes:

- **SendRawBytes(bytes).** Internal; writes verbatim to the
  stream, swallowing errors and logging.
- **SendText(s).** UTF-8 encodes and writes synchronously.
- **SendLine(s).** Appends CR-LF before sending.
- **SendSubnegotiation(option, data).** Builds the canonical
  IAC SB &lt;option&gt; ... IAC SE frame. Every IAC byte in
  the data payload MUST be escaped by doubling (IAC IAC),
  because IAC inside a subnegotiation cannot be confused with
  IAC SE.

### 3.6 Echo control

Telnet echo is asymmetric:

- **Server echo** (server prints each character back). Used for
  clients that don't echo locally (cooked telnet).
- **Client echo** (client prints each character locally). Used
  for MUD clients, which always do their own input rendering.

Capability derivation (§7) picks one based on terminal type. On
SetCapabilities:

- If server echo is in use, the server sends IAC WILL ECHO
  (taking responsibility for echo) and sets its internal echo
  flag.
- If client echo is in use, the server does not send WILL ECHO;
  the internal flag is set false.

SuppressEcho / RestoreEcho during play:

- Under server echo, the internal flag is toggled. The read
  loop checks the flag for every printable byte.
- Under client echo, the server sends IAC WILL ECHO (suppress)
  / IAC WONT ECHO (restore). MUD clients honor these to mask
  password input.

The login flow expects echo to be silenced before password
prompts and restored before any subsequent output (see
`docs/specs/login.md` §6.2).

**Acceptance criteria**

- [ ] The listener processes negotiation failures without
      blocking subsequent accepts.
- [ ] The read loop handles partial multi-byte IAC sequences
      across multiple reads (the state machine is byte-stream
      safe).
- [ ] Backspace/DEL erases visibly only when echo is enabled.
- [ ] IAC bytes in subnegotiation payloads are doubled when
      sent.
- [ ] Line ≥ 4096 bytes clears the buffer with a message; total
      ≥ 65536 bytes disconnects.
- [ ] Server echo vs client echo is selected once during
      negotiation and is consistent for the connection's
      lifetime, except for SuppressEcho/RestoreEcho.

---

## 4. Telnet negotiation

### 4.1 Option codes

Negotiation honors the standard RFC option codes (this spec
specifies them because they are interoperability contracts):

| Option | Code | RFC | Purpose |
|---|---|---|---|
| ECHO | 1 | 857 | server-side echo of input |
| TTYPE | 24 | 1091 | terminal type (client identification) |
| NAWS | 31 | 1073 | window size (width / height) |
| MSSP | 70 | conv. | mud server status protocol |
| GMCP | 201 | conv. | generic mud communication protocol |

IAC (255), SB (250), SE (240), WILL (251), WONT (252), DO (253),
DONT (254) are the standard telnet control bytes.

### 4.2 Negotiation budget

Negotiation is time-boxed by a configurable timeout (today 500 ms).
The timeout is cooperative: it stops the negotiation phase only,
not the connection itself. On expiry, whatever the negotiator has
collected becomes the result; the read loop proceeds.

### 4.3 Sequence

1. **Server sends `IAC DO TTYPE`** and `IAC DO NAWS` back to
   back to ask the client to identify itself and its window.
2. **Server invokes each registered protocol handler's
   `NegotiateAsync`** in order. The handler may send its own
   advertisement (e.g. GMCP sends `IAC WILL GMCP`; MSSP sends
   `IAC WILL MSSP`).
3. **Server reads** until both TTYPE and NAWS values have
   arrived OR the timeout fires.

During reads, the negotiator interprets:

- **`WILL TTYPE`** from the client → server sends
  `IAC SB TTYPE 1 IAC SE` (subneg with sub-command "SEND",
  asking for the terminal type string). Only sent once.
- **`DO <option>`** from the client where the option matches a
  registered handler → call the handler's `HandleRemoteDo`.
  When the option is GMCP, the negotiator also marks
  GMCP-active so the resulting capabilities reflect it.
- **`SB TTYPE IS <bytes> IAC SE`** → the bytes after `IS` (sub-
  command byte 0) are the ASCII terminal type. Recorded.
- **`SB NAWS <width-hi> <width-lo> <height-hi> <height-lo> IAC
  SE`** → two big-endian unsigned shorts. Recorded.
- **`SB <option> <payload> IAC SE`** for any other registered
  option → dispatched to the handler's `HandleSubnegotiation`.

Any other IAC sequences are skipped.

### 4.4 Handler registration vs router attachment

A protocol handler declares whether it is **session-long** via a
flag. Session-long handlers (e.g. GMCP) are attached to the
connection's router after negotiation completes — they continue
receiving subnegotiation bytes for the rest of the connection.
One-shot handlers (e.g. MSSP, which serves its variable table
once on DO) are NOT attached to the router; they do their work
during negotiation and are then discarded.

The router is attached to the connection at the end of the
negotiator's run, including the empty router case where no
session-long handlers exist.

### 4.5 Result

After negotiation, the negotiator builds a ClientCapabilities
record:

- If any TTYPE or NAWS value was received, capabilities derive
  from negotiation (§7).
- Otherwise capabilities are the default values (server echo,
  80×24, no color, no GMCP).

The capabilities are then handed to the connection via
`SetCapabilities`, which in turn applies the echo policy (§3.6).

**Acceptance criteria**

- [ ] Negotiation respects the configured timeout and does not
      block forever on a silent client.
- [ ] TTYPE-SEND is sent at most once even if the client
      WILLs TTYPE multiple times.
- [ ] NAWS values are parsed as big-endian unsigned shorts.
- [ ] Session-long handlers (GMCP) are wired to the router;
      one-shot handlers (MSSP) are not.
- [ ] An MSSP DO during negotiation produces a single MSSP
      subneg reply and is then done.
- [ ] Capabilities reflect every received signal (TTYPE, NAWS,
      GMCP-active) and default sensibly for anything missing.

---

## 5. GMCP

GMCP is the engine's structured-data side channel. It carries
named JSON packages in both directions and is exposed to the
engine through a uniform IGmcpHandler interface (`Send`,
`SupportsPackage`, `OnGmcpMessage`, `GmcpActive`) regardless of
transport.

### 5.1 Telnet GMCP wire format

Server-to-client: `IAC SB GMCP <utf-8 bytes> IAC SE`. The bytes
are a UTF-8 string consisting of the package name, a single
space, then the JSON payload. JSON is serialized with camelCase
property names and null-omitted fields. IAC bytes in the
payload are doubled (§3.5).

Client-to-server: same shape; the server reads via the router →
GMCP handler. Empty payloads are tolerated; absent payload is
read as JSON `null`.

### 5.2 WebSocket GMCP wire format

Server-to-client: a JSON envelope `{ "type": "gmcp", "package":
"<name>", "data": <payload> }`. Sent as one websocket text
frame.

Client-to-server: same shape. The websocket connection (§6)
dispatches `type: "gmcp"` envelopes to the GMCP handler.

The websocket GMCP handler treats every package as supported
(no `Core.Supports.Set` mechanism — the web client is expected
to handle all packages the engine sends).

### 5.3 Core.Supports protocol

Telnet clients may declare which packages they will process by
sending `Core.Supports.Set` with a JSON array of strings, each
of which is `<package name> <version>` (version optional,
defaults to 1). The server stores the set and the
`SupportsPackage(name)` query then returns true iff:

- The set has not yet been received (default permissive — the
  server emits everything until told otherwise), OR
- The package name exactly matches a key in the set, OR
- The package name starts with `<key>.` (dotted descendants
  match their ancestors — sending `Char.Vitals` is allowed when
  the client supports `Char`).

`Core.Supports.Remove` accepts a JSON array of package names
(version, if any, ignored) and removes each from the set.

### 5.4 Inbound packages

The handler exposes a single `OnGmcpMessage(package, payload)`
callback. The networking feature itself does NOT interpret
`Core.Supports.*` beyond updating the support set; the engine
features handle other packages.

### 5.5 GMCP activation

Telnet GMCP is active iff the client sent `DO GMCP` during
negotiation. The handler refuses to send if it isn't active —
silent no-op rather than error. WebSocket GMCP is always
active.

**Acceptance criteria**

- [ ] Telnet GMCP frames are emitted as `IAC SB GMCP <utf-8> IAC SE`
      with IAC escaping in the payload.
- [ ] WebSocket GMCP frames are emitted as the documented
      envelope.
- [ ] `Core.Supports.Set` initializes / replaces the supported
      package set; `Core.Supports.Remove` removes entries.
- [ ] Until `Core.Supports.Set` is received, every package
      passes the support check (permissive default).
- [ ] Dotted-prefix package matching works (`Char` supports
      `Char.Vitals`).
- [ ] Telnet `Send` is a silent no-op when GMCP is not active.

---

## 6. WebSocket transport

### 6.1 Wire format

Each direction uses one-text-frame JSON envelopes. Two envelope
types are recognized:

- **`{ "type": "text", "data": "<string>" }`** — server → client
  user-facing text (already terminated with CR-LF for line
  output). Client → server text input is not used; commands
  always use the `command` type.
- **`{ "type": "command", "data": "<string>" }`** — client →
  server typed command input.
- **`{ "type": "gmcp", "package": "<name>", "data": <any> }`** —
  GMCP package envelope, either direction.

Unknown `type` values are silently ignored on the inbound side.

### 6.2 Outbound queue

Outbound messages enqueue into an unbounded channel; a write
loop pulls from the channel and sends each as a single text
frame while the socket is open. Once the writer is closed (on
disconnect), the loop drains and exits.

### 6.3 Inbound size cap

Inbound message buffering caps at 64 KiB per message. A frame
that exceeds the cap closes the connection with reason
`message too large`. (Telnet has the same 65536-byte total cap;
the websocket cap is per-message.)

### 6.4 Connection lifecycle

`RunAsync(ct)` starts both the read and write loops and exits
when either completes. On exit it closes the outbound writer
and, unless disconnect already fired, calls Disconnect("connection
ended"). The socket close uses `NormalClosure` with the disconnect
reason as the close description.

### 6.5 Capabilities

WebSocket connections set `SupportsAnsi = true` unconditionally —
web clients render ANSI sequences. Capability derivation from
TTYPE / NAWS doesn't apply (there is no IAC negotiation); the
web client is expected to manage echo, window size, and color
locally. Echo suppress / restore are no-ops on the server side.

**Acceptance criteria**

- [ ] Inbound JSON is dispatched by `type` and unknown types
      are ignored.
- [ ] Inbound messages over 64 KiB disconnect cleanly.
- [ ] Outbound text and GMCP messages emit one text frame
      each.
- [ ] SuppressEcho / RestoreEcho do nothing on the server side
      (no-op).
- [ ] Disconnect fires reason-then-event exactly once even on
      racey close paths.

---

## 7. Client capabilities

Capabilities are derived once at negotiation completion and
stored immutable on the connection.

### 7.1 Fields

| Field | Source |
|---|---|
| ClientName | Terminal type string (TTYPE) |
| SupportsTtype | TTYPE was received |
| SupportsNaws | NAWS values were received |
| SupportsGmcp | Client sent DO GMCP during negotiation |
| WindowWidth / Height | NAWS values, or defaults (80 / 24) |
| ColorSupport | Derived from TTYPE (§7.2) |
| UseServerEcho | Derived from client identity (§7.3) |
| IsMudClient | Client name matches the known-MUD-client list |

### 7.2 Color tiers

The ColorSupport tier is one of {None, Basic, Extended,
TrueColor}, derived from the TTYPE string:

- **None** — no TTYPE received.
- **TrueColor** — TTYPE contains `TRUECOLOR` (case-insensitive).
- **Extended** — TTYPE contains `256COLOR`, OR the client is a
  known MUD client.
- **Basic** — any other TTYPE.

Known MUD clients are matched case-insensitively against a
hardcoded allowlist that includes the popular clients (Mudlet,
MUSHclient, TinTin++, ZMud / CMud, Atlantis, Mudlet, Potato,
BlowTorch, KildClient, BeIP, GnomeMUD). Adding new clients
requires editing the allowlist.

### 7.3 Echo policy

`UseServerEcho` is **false when the client is a known MUD client**
(MUD clients handle their own echo) and **true otherwise** (cooked
telnet / generic clients need the server to echo).

The echo policy is fixed at negotiation time and does not change
mid-session except via SuppressEcho / RestoreEcho (§3.6).

### 7.4 Defaults

When negotiation produces no data (timeout, raw connect, no
TTYPE / NAWS):

- ClientName null, no TTYPE / NAWS / GMCP support.
- 80×24, ColorSupport None.
- UseServerEcho true (assume cooked telnet).
- IsMudClient false.

**Acceptance criteria**

- [ ] Capabilities are immutable after construction.
- [ ] TrueColor and Extended hints in TTYPE upgrade the color
      tier.
- [ ] Known MUD clients get Extended color even without a
      TRUECOLOR / 256COLOR hint.
- [ ] Known MUD clients get UseServerEcho = false.
- [ ] Default capabilities reflect a safe 80×24, no color, server
      echo profile.

---

## 8. MSSP

MSSP is a one-shot crawler protocol: a crawler connects, sends
`DO MSSP`, the server replies with a single subneg containing
the variable table, and the crawler disconnects.

### 8.1 Variable table format

The reply is a subneg whose payload is a sequence of
`VAR <name> VAL <value>` records where VAR = byte 1 and VAL =
byte 2, and `<name>` and `<value>` are ASCII byte runs (no NUL
terminator, no length prefix — the bytes between VAR and the
next VAR or end-of-payload are the value).

### 8.2 Standard variables

The server MUST emit at least these variables from the
configured `MsspConfig`:

| Name | Source |
|---|---|
| NAME, CODEBASE, CONTACT, HOSTNAME, PORT, CREATED, LANGUAGE, FAMILY | static config |
| GAMEPLAY | pipe-joined list (e.g. `Hack and Slash|Roleplaying`) |
| CLASSES, RACES, LEVELS, EQUIPMENT, MULTIPLAYING, PLAYERKILLING | `1` / `0` booleans |
| PLAYERS | live player count (dynamic) |
| UPTIME | server uptime epoch (dynamic) |
| ANSI | `1` (server emits ANSI) |
| UTF-8 | `1` |
| GMCP | `1` |
| MCCP | `0` (compression unimplemented) |

Dynamic values are produced by a caller-supplied factory at
each request, so PLAYERS and UPTIME reflect current state.

### 8.3 No-op subneg

MSSP never receives subneg payloads from the client. If one
arrives (malformed crawler), the handler ignores it.

### 8.4 Not session-long

After replying once to a DO MSSP, the handler has no further
work for the connection. It is NOT registered with the router
(§4.4) and does not consume bytes for the rest of the session.

**Acceptance criteria**

- [ ] The variable table contains every required variable.
- [ ] PLAYERS and UPTIME are produced fresh on every emission.
- [ ] MCCP is "0" until compression is implemented.
- [ ] Inbound MSSP subneg payloads are silently ignored.

---

## 9. ANSI rendering

The networking feature exposes the ANSI escape sequences as
constants for the rest of the engine to use (Reset, Bold, the
8 basic colors, the 8 bright variants) and a `Colorize(text,
color)` convenience that wraps text in `<color><text>RESET`.

This is the *low-level* layer. The actual color tag
substitution (`<highlight>foo</highlight>` → ANSI) and the
per-client decision to strip vs render are policy that lives
in the engine's color theme / UI feature. The networking
feature provides the byte sequences and the `SupportsAnsi`
boolean; the engine renders.

Clear-screen is the one ANSI usage the connection itself emits
directly (§2.2).

**Acceptance criteria**

- [ ] The constants match the standard SGR sequences.
- [ ] Clear-screen emits `ESC[2J ESC[H` only when SupportsAnsi.

---

## 10. Observable events

The networking feature is a producer of two kinds of events:

- **Per-connection events** on IConnection (OnInput,
  OnDisconnected, OnDisconnectedWithReason) — the engine
  consumes these to drive sessions and command dispatch.
- **Listener events** (OnConnectionAccepted) — the engine
  consumes to attach a new connection to a pre-login session.

The feature does NOT emit engine-bus GameEvents. Higher
features (login, session manager) emit those after observing
the IConnection lifecycle.

GMCP `OnGmcpMessage` callbacks fire once per inbound package
on both transports.

---

## 11. Configuration surface

The following are externally configurable and not fixed by this
spec.

| Policy | Where it applies |
|---|---|
| Telnet listen port | §3.1 |
| WebSocket listen port (owned by the HTTP host) | §6 |
| Negotiation timeout (ms) | §4.2 |
| MSSP static configuration | §8.2 |
| MSSP dynamic value factory | §8.2 |
| Known-MUD-client allowlist | §7.2 |
| Per-line and total input buffer caps | §3.4 |
| WebSocket message size cap | §6.3 |

---

## 12. Open questions / future work

- **No MCCP / compression.** Telnet bandwidth is small but
  visible in mobile-network scenarios; MCCP2 is the standard
  answer. Today MSSP advertises `MCCP=0`.
- **No TLS in-feature.** Plaintext telnet and HTTP-upgrade
  websocket. Production deployments belong behind a TLS
  terminator. A native TLS option for telnet would simplify
  ops.
- **Hardcoded MUD-client allowlist.** Adding a client requires
  editing source. Externalize the list (config file) and the
  rule for color tier derivation could be data-driven.
- **One-shot vs session-long handler flag.** Today the
  distinction is binary. A handler that wants to participate
  in negotiation and then continue listening for one-off
  out-of-band messages later has no path.
- **Telnet negotiation reads in one phase.** After the
  negotiation timeout (or completion), no further WILL/WONT/DO/DONT
  is honored by the negotiator — the read loop simply skips
  them. Clients that try to renegotiate mid-session (e.g.
  upgrade to GMCP later) cannot.
- **Cooked-telnet edge cases.** The read loop assumes a stream
  of bytes; line editing beyond backspace (arrow keys, history)
  is the client's job. Servers that want to be friendly to
  raw-telnet users may want server-side line editing — out of
  scope here.
- **GMCP camelCase only.** Outbound GMCP serializes property
  names as camelCase. Clients expecting PascalCase or
  snake_case will see mismatched field names. A per-package
  serialization policy would be more flexible.
- **No JSON validation on inbound websocket frames.** Malformed
  JSON is caught and logged at debug level; type-coercion
  errors (e.g. `data` as a number when a string was expected)
  produce silent ignores. A stricter validation surface with a
  per-malformed-frame counter would aid abuse detection.
- **WindowWidth / Height as ints.** NAWS sends unsigned shorts;
  the implementation reads them as ints. Pathological values
  (`0`, very large) are accepted as-is and downstream renderers
  may not clamp. A defensive clamp at the negotiation layer
  would be safer.
- **Disconnect race.** The dual `OnDisconnected` / `OnDisconnectedWithReason`
  events exist because some older callers subscribe to one,
  some the other. Consolidating to a single
  `OnDisconnected(reason)` event would simplify the contract
  but is a breaking change for existing subscribers.

---

<!-- Generated: 2026-05-21 · Scope: IConnection + TelnetServer + TelnetConnection + TelnetNegotiator + TelnetProtocolRouter + GmcpProtocolHandler + MsspProtocolHandler + WebSocketConnection + WebSocketGmcpHandler + ClientCapabilities + AnsiColor · Spec style: narrative + acceptance criteria · Detail level: behavior only (with telnet wire codes specified for interoperability) -->

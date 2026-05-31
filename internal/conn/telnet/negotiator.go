package telnet

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Telnet command bytes per RFC 854 / RFC 855. Centralized here
// (telnet.go still defines a partial set for backward compat with
// stripIAC; both name the same hex value).
const (
	negIAC  byte = 0xFF
	negSB   byte = 0xFA
	negSE   byte = 0xF0
	negWILL byte = 0xFB
	negWONT byte = 0xFC
	negDO   byte = 0xFD
	negDONT byte = 0xFE
)

// ttypeMaxRotations bounds the number of times we'll query a peer
// for its TTYPE. RFC 1091 has no explicit cap; 3 covers the Mudlet
// rotation pattern (display-name → MTTS hex → terminal-name) plus
// a safety margin while preventing a degenerate client from making
// us query forever.
const ttypeMaxRotations = 3

// parserState is the IAC-level state machine the negotiator drives
// against each incoming byte.
type parserState int

const (
	stateNormal  parserState = iota // accumulating data bytes
	stateIAC                        // saw IAC; next byte is a command
	stateOption                     // saw IAC <WILL|WONT|DO|DONT>; next byte is option code
	stateSB                         // saw IAC SB; next byte is option code
	stateSBData                     // collecting subneg payload until IAC SE
	stateSBIAC                      // saw IAC inside SB; next byte is SE or escaped IAC
)

// optionPolicy is the "Q method" state for one (us, him) side of an
// option per RFC 1143. We track just enough to refuse cleanly and
// avoid negotiation loops. Full Q-method state would track separate
// us/him states + pending counters; the M16.1 surface only sends
// from the server side, so the simpler "is this option enabled" bit
// is enough for routing.
type optionPolicy struct {
	enabled bool // peer has agreed to do/will this option
}

// negotiator owns per-connection IAC state. One per *Conn. Methods
// are NOT safe for concurrent callers — the Read goroutine is the
// sole driver. Replies to the peer go through the conn's
// WriteCommand (Conn already holds the write mutex internally).
type negotiator struct {
	conn *Conn

	state parserState
	// pendingVerb stashes WILL/WONT/DO/DONT between the IAC and
	// option-code bytes so handleNegotiation knows which verb
	// to dispatch against. Dedicated field (rather than reusing
	// sbOption) so the two state machine slots can't accidentally
	// alias.
	pendingVerb byte
	sbOption    byte
	sbBuf       []byte

	options map[byte]*optionPolicy

	// capMu guards caps so the conn's Capabilities() accessor
	// (called from the session / render goroutines) can read
	// without blocking the read loop.
	capMu sync.Mutex
	caps  Capabilities

	// offersSent tracks whether the initial "IAC DO TTYPE / IAC DO
	// NAWS" round has been sent. Send lazily on first Read so New()
	// stays I/O-free and ctx-cancellation flows through the normal
	// Read path.
	offersSent bool
}

func newNegotiator(c *Conn) *negotiator {
	return &negotiator{
		conn:    c,
		state:   stateNormal,
		options: make(map[byte]*optionPolicy),
	}
}

// option returns the policy for code, creating it on first use.
func (n *negotiator) option(code byte) *optionPolicy {
	if p, ok := n.options[code]; ok {
		return p
	}
	p := &optionPolicy{}
	n.options[code] = p
	return p
}

// snapshot returns a copy of the current capabilities. Safe for
// concurrent callers; the read loop publishes through capMu.
func (n *negotiator) snapshot() Capabilities {
	n.capMu.Lock()
	defer n.capMu.Unlock()
	out := Capabilities{Width: n.caps.Width, Height: n.caps.Height}
	if len(n.caps.TerminalTypes) > 0 {
		out.TerminalTypes = make([]string, len(n.caps.TerminalTypes))
		copy(out.TerminalTypes, n.caps.TerminalTypes)
	}
	return out
}

// sendInitialOffers writes the server-initiated negotiation offers
// for the options M16.1 supports. Lazy — invoked from feed() on the
// first byte the read loop processes. Caller MUST NOT hold any
// lock; WriteCommand takes the conn's write mutex.
func (n *negotiator) sendInitialOffers(ctx context.Context) {
	if n.offersSent {
		return
	}
	n.offersSent = true
	// IAC DO TTYPE + IAC DO NAWS. The conn write mutex serializes
	// against other senders; a write error here is logged but not
	// fatal (a peer that closes mid-negotiation will surface the
	// error on the next regular Write).
	pkt := []byte{
		negIAC, negDO, optTTYPE,
		negIAC, negDO, optNAWS,
	}
	if _, err := n.conn.WriteCommand(ctx, pkt); err != nil {
		logging.From(ctx).Debug("telnet.negotiator: initial offer write failed",
			slog.String("session_id", n.conn.ID()),
			slog.Any("err", err))
	}
}

// feed runs one input byte through the IAC state machine. Returns
// (dataByte, isData) — when isData is true, b is a payload byte
// the caller should accumulate into its line buffer; otherwise the
// byte was consumed by the protocol.
//
// Side effects: completed negotiations emit replies via the conn's
// WriteCommand, and completed subnegotiations dispatch to per-
// option handlers that update n.caps.
func (n *negotiator) feed(ctx context.Context, b byte) (byte, bool) {
	switch n.state {
	case stateNormal:
		if b == negIAC {
			n.state = stateIAC
			return 0, false
		}
		return b, true

	case stateIAC:
		switch b {
		case negIAC:
			// Escaped literal 0xFF — emit as data.
			n.state = stateNormal
			return negIAC, true
		case negWILL, negWONT, negDO, negDONT:
			n.pendingVerb = b
			n.state = stateOption
			return 0, false
		case negSB:
			n.state = stateSB
			return 0, false
		default:
			// Standalone command (NOP, GA, …) — ignore.
			n.state = stateNormal
			return 0, false
		}

	case stateOption:
		n.handleNegotiation(ctx, n.pendingVerb, b)
		n.state = stateNormal
		return 0, false

	case stateSB:
		n.sbOption = b
		n.sbBuf = n.sbBuf[:0]
		n.state = stateSBData
		return 0, false

	case stateSBData:
		if b == negIAC {
			n.state = stateSBIAC
			return 0, false
		}
		// Bound the subneg buffer so a peer streaming SB without SE
		// can't grow memory without limit. The cap is generous for
		// real options (TTYPE name max ~40 bytes, NAWS is 4 bytes,
		// MSSP variable list ~1KB) but stops at the same MaxLineBytes
		// the line-level reader uses.
		if len(n.sbBuf) >= MaxLineBytes {
			// Discard the runaway block; resetting state breaks any
			// future SE match for this block, which is the safer
			// failure mode than allowing unbounded growth.
			n.sbBuf = n.sbBuf[:0]
			n.state = stateNormal
			return 0, false
		}
		n.sbBuf = append(n.sbBuf, b)
		return 0, false

	case stateSBIAC:
		switch b {
		case negSE:
			n.handleSubneg(ctx, n.sbOption, n.sbBuf)
			n.sbBuf = n.sbBuf[:0]
			n.state = stateNormal
			return 0, false
		case negIAC:
			// IAC-in-SB escape: keep one 0xFF in the buffer, stay
			// in SBData.
			if len(n.sbBuf) < MaxLineBytes {
				n.sbBuf = append(n.sbBuf, negIAC)
			}
			n.state = stateSBData
			return 0, false
		default:
			// Unexpected byte after IAC inside SB — treat as
			// end-of-block and process whatever we collected.
			n.handleSubneg(ctx, n.sbOption, n.sbBuf)
			n.sbBuf = n.sbBuf[:0]
			n.state = stateNormal
			return 0, false
		}
	}
	// Unreachable; defensive.
	n.state = stateNormal
	return 0, false
}

// handleNegotiation processes a completed IAC <verb> <opt> 3-byte
// command. The Q-method (RFC 1143) calls for tracking both ends'
// states with pending counters to avoid loops; the M16.1 surface
// is simple enough to skip that machinery — for options we know
// (TTYPE, NAWS) we send a follow-up; for options we don't, we
// refuse with the opposite verb.
func (n *negotiator) handleNegotiation(ctx context.Context, verb, opt byte) {
	logging.From(ctx).Debug("telnet.negotiator: option exchange",
		slog.String("session_id", n.conn.ID()),
		slog.String("verb", verbName(verb)),
		slog.Int("option", int(opt)))

	switch verb {
	case negWILL:
		// Peer offers to do option themselves. Accept known options;
		// refuse unknown to break loops.
		switch opt {
		case optTTYPE:
			n.option(opt).enabled = true
			// Request the first terminal name.
			n.sendSubneg(ctx, optTTYPE, []byte{ttypeSEND})
		case optNAWS:
			n.option(opt).enabled = true
			// NAWS: client emits SB unsolicited on resize; nothing
			// further to request.
		default:
			n.sendCommand(ctx, negDONT, opt)
		}
	case negWONT:
		// Peer refuses or rescinds. Mark disabled; spec requires we
		// reply DONT only if state was active.
		if p, ok := n.options[opt]; ok && p.enabled {
			p.enabled = false
			n.sendCommand(ctx, negDONT, opt)
		}
	case negDO:
		// Peer asks us to enable an option. We don't enable any of
		// our own options in M16.1 (no WILL ECHO offer here — login
		// owns that), so refuse blanket.
		n.sendCommand(ctx, negWONT, opt)
	case negDONT:
		// Peer refuses to let us do an option. We weren't doing any,
		// so silently acknowledge by ignoring (per Q-method: only
		// reply WONT if we were enabled, which we never are).
	}
}

// handleSubneg dispatches a completed IAC SB <opt> ... IAC SE block.
// payload is the bytes between the option code and the closing
// IAC SE, with any IAC-IAC escapes already collapsed back to a
// single 0xFF.
func (n *negotiator) handleSubneg(ctx context.Context, opt byte, payload []byte) {
	switch opt {
	case optTTYPE:
		n.handleTTYPESubneg(ctx, payload)
	case optNAWS:
		n.handleNAWSSubneg(ctx, payload)
	default:
		logging.From(ctx).Debug("telnet.negotiator: ignoring unknown subneg",
			slog.String("session_id", n.conn.ID()),
			slog.Int("option", int(opt)),
			slog.Int("payload_len", len(payload)))
	}
}

// handleTTYPESubneg parses an IS <ascii name> payload (RFC 1091).
// Implements PD-5: query twice (or more) until the same name comes
// back two times in a row, at which point the rotation has wrapped.
// Capture each distinct name in order — the first one is the
// most-specific client identifier.
func (n *negotiator) handleTTYPESubneg(ctx context.Context, payload []byte) {
	if len(payload) < 1 || payload[0] != ttypeIS {
		// Malformed; ignore.
		return
	}
	name := string(payload[1:])
	if name == "" {
		return
	}

	n.capMu.Lock()
	// Wrap detection: stop if name has been seen before. Catches
	// both "client returns one and stops" (matches the most recent)
	// and "client cycles through a list" (matches the first or any
	// earlier entry). The RFC 1091 §3 protocol expects the cycle to
	// terminate when the same name appears twice; "twice in a row"
	// is the special case of cycle-length-1.
	for _, seen := range n.caps.TerminalTypes {
		if seen == name {
			n.capMu.Unlock()
			return
		}
	}
	n.caps.TerminalTypes = append(n.caps.TerminalTypes, name)
	rotations := len(n.caps.TerminalTypes)
	n.capMu.Unlock()

	logging.From(ctx).Debug("telnet.negotiator: ttype",
		slog.String("session_id", n.conn.ID()),
		slog.String("name", name),
		slog.Int("rotation", rotations))

	if rotations >= ttypeMaxRotations {
		return
	}
	// Query again.
	n.sendSubneg(ctx, optTTYPE, []byte{ttypeSEND})
}

// handleNAWSSubneg parses a NAWS payload (RFC 1073): four bytes,
// width-MSB width-LSB height-MSB height-LSB. Any other length is
// malformed and ignored.
func (n *negotiator) handleNAWSSubneg(ctx context.Context, payload []byte) {
	if len(payload) != 4 {
		return
	}
	w := int(payload[0])<<8 | int(payload[1])
	h := int(payload[2])<<8 | int(payload[3])

	n.capMu.Lock()
	n.caps.Width = w
	n.caps.Height = h
	n.capMu.Unlock()

	logging.From(ctx).Debug("telnet.negotiator: naws",
		slog.String("session_id", n.conn.ID()),
		slog.Int("width", w),
		slog.Int("height", h))
}

// sendCommand writes IAC <verb> <opt>. Best-effort; write errors
// fall through the conn's normal error path on later writes.
func (n *negotiator) sendCommand(ctx context.Context, verb, opt byte) {
	pkt := []byte{negIAC, verb, opt}
	if _, err := n.conn.WriteCommand(ctx, pkt); err != nil {
		logging.From(ctx).Debug("telnet.negotiator: command write failed",
			slog.String("session_id", n.conn.ID()),
			slog.Any("err", err))
	}
}

// sendSubneg writes IAC SB <opt> <payload> IAC SE. Payload bytes
// are NOT escaped here because the M16.1 subneg payloads are
// fixed shapes (TTYPE SEND is one byte; we never send NAWS).
// Adding IAC-doubling for payloads with a literal 0xFF would be
// the place to put it when a future option requires it.
func (n *negotiator) sendSubneg(ctx context.Context, opt byte, payload []byte) {
	pkt := make([]byte, 0, 5+len(payload))
	pkt = append(pkt, negIAC, negSB, opt)
	pkt = append(pkt, payload...)
	pkt = append(pkt, negIAC, negSE)
	if _, err := n.conn.WriteCommand(ctx, pkt); err != nil {
		logging.From(ctx).Debug("telnet.negotiator: subneg write failed",
			slog.String("session_id", n.conn.ID()),
			slog.Any("err", err))
	}
}

// verbName returns the human-readable mnemonic for a negotiation
// verb byte. Used only by debug logs.
func verbName(b byte) string {
	switch b {
	case negWILL:
		return "WILL"
	case negWONT:
		return "WONT"
	case negDO:
		return "DO"
	case negDONT:
		return "DONT"
	}
	return "?"
}

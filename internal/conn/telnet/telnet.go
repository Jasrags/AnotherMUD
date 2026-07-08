// Package telnet implements conn.Connection over a raw TCP socket.
//
// Line-oriented reads with minimal IAC awareness. We do not negotiate
// telnet options, but we DO strip IAC sequences from the input stream
// so the server-initiated WILL/WONT ECHO bytes (used by the login
// password prompt) don't poison subsequent reads with the client's
// reflexive DO/DONT reply. Full option negotiation, GMCP, MSSP, and
// the like land with the networking-protocols milestone per
// docs/specs/networking-protocols.md.
package telnet

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/mssp"
	"github.com/Jasrags/AnotherMUD/internal/render"
)

// MaxLineBytes caps the number of bytes a single Read will buffer
// before returning conn.ErrLineTooLong. Prevents a peer that streams
// bytes without ever sending a newline (slow-loris style) from growing
// the per-connection read buffer without bound.
//
// Sized for normal MUD input including long command lines, GMCP frames
// will get their own path when added in a later milestone.
const MaxLineBytes = 1024

// Conn adapts a net.Conn to the conn.Connection interface.
type Conn struct {
	id   string
	raw  net.Conn
	r    *bufio.Reader
	wmu  sync.Mutex
	once sync.Once
	done chan struct{}

	// neg owns the M16.1 IAC negotiation state machine — option
	// state, subnegotiation buffer, captured Capabilities. Driven
	// from Read (single goroutine); accessors take their own lock.
	neg *negotiator

	// mssp is the M16.2 MUD-server-status-protocol config the
	// negotiator reads to build the SB MSSP payload on DO MSSP
	// from a crawler. nil = no MSSP support; the handler refuses
	// with WONT MSSP. Set via WithMssp at construction; the
	// pointer is shared across connections (the composition root
	// builds one Config per server).
	mssp *mssp.Config

	// gmcp is the M16.3 GMCP per-connection state — the active
	// flag, the supports set, and the inbound dispatch callback.
	// Always non-nil so SendGmcp / SupportsPackage / handleSubneg
	// can read fields without nil-checking.
	gmcp *gmcpState

	// Char-at-a-time line editing (tab-completion Phase 2). When charMode
	// is on, Read echoes keystrokes and runs the editor (backspace, Tab
	// completion) instead of buffering a raw line. completionProvider is
	// the Tab callback the session installs. Both are touched only by the
	// read/dispatch goroutine in practice; cmu guards them for the race
	// detector and any future caller.
	cmu                sync.Mutex
	charMode           bool
	completionProvider conn.CompletionProvider
}

// New wraps an established net.Conn. id should be a stable identifier
// (typically a UUID or monotonic counter) assigned by the server.
//
// opts apply in order at construction so the negotiator and any
// per-connection state see them before the first Read. See
// WithMssp for the first option this surface ships.
func New(id string, c net.Conn, opts ...Option) *Conn {
	// bufio over LimitReader caps total bytes read across the connection
	// lifetime, which would be wrong here — we want a per-Read cap. Track
	// the limit in Read itself via bufio.Reader.Buffered / Peek instead.
	tc := &Conn{
		id:   id,
		raw:  c,
		r:    bufio.NewReaderSize(c, MaxLineBytes),
		done: make(chan struct{}),
		gmcp: newGmcpState(),
	}
	tc.neg = newNegotiator(tc)
	for _, opt := range opts {
		opt(tc)
	}
	return tc
}

// Capabilities returns the per-connection client-capability snapshot
// populated by the IAC negotiator (M16.1). Safe for concurrent
// callers; returns the zero value before negotiation completes (or
// if the client refused both TTYPE and NAWS).
func (c *Conn) Capabilities() Capabilities {
	return c.neg.snapshot()
}

// ColorTier implements the conn-agnostic color-tier interface
// the session layer consumes (M16.6a). Reads through the live
// negotiator snapshot so a TTYPE that arrives mid-session is
// observable on the next access. Returns render.ColorTierNone
// before negotiation completes — a pre-TTYPE render call gets
// the safe no-color path.
func (c *Conn) ColorTier() render.ColorTier {
	return c.neg.snapshot().ColorSupport
}

// TerminalWidth reports the client's last-reported window width in
// columns (RFC 1073 NAWS). Reads through the live negotiator snapshot
// so a resize that arrives mid-session is observable on the next
// access. Returns 0 before NAWS completes (or if the client refused
// the option) — renderers fall back to a configured default width.
func (c *Conn) TerminalWidth() int {
	return c.neg.snapshot().Width
}

// ID implements conn.Connection.
func (c *Conn) ID() string { return c.id }

// SetCompletionProvider installs the Tab-completion callback used in
// char-mode (conn.CharModeConn). nil clears it.
func (c *Conn) SetCompletionProvider(p conn.CompletionProvider) {
	c.cmu.Lock()
	c.completionProvider = p
	c.cmu.Unlock()
}

// SetCharMode turns server-side character-at-a-time editing on or off
// (conn.CharModeConn). Enabling negotiates WILL SGA + WILL ECHO so the
// client streams keystrokes and stops local echo; disabling reverses it.
// Idempotent.
func (c *Conn) SetCharMode(ctx context.Context, on bool) {
	c.cmu.Lock()
	if c.charMode == on {
		c.cmu.Unlock()
		return
	}
	c.charMode = on
	c.cmu.Unlock()

	if on {
		_, _ = c.WriteCommand(ctx, []byte{negIAC, negWILL, optSGA, negIAC, negWILL, optEcho})
	} else {
		_, _ = c.WriteCommand(ctx, []byte{negIAC, negWONT, optEcho, negIAC, negWONT, optSGA})
	}
}

// CharModeActive reports whether char-mode editing is on (conn.CharModeConn).
func (c *Conn) CharModeActive() bool { return c.charModeActive() }

func (c *Conn) charModeActive() bool {
	c.cmu.Lock()
	defer c.cmu.Unlock()
	return c.charMode
}

// Read implements conn.Connection. It returns one line at a time with
// the trailing CR/LF stripped.
//
// M16.1: driven through the per-connection IAC negotiator
// (negotiator.feed) so subnegotiations arriving mid-line, between
// lines, or unsolicited (NAWS resize) are consumed without
// polluting the line buffer. Data bytes — anything the negotiator
// surfaces as "payload" — accumulate into the line builder; the
// call returns on \n with the trailing CR stripped.
//
// On the first Read of a connection, the negotiator emits the
// initial server-side offers (IAC DO TTYPE, IAC DO NAWS). Doing it
// lazily here keeps New() I/O-free and lets ctx cancellation flow
// through the regular Read error path.
func (c *Conn) Read(ctx context.Context) (string, error) {
	// Honor ctx cancellation by closing the underlying conn so the
	// blocked ReadByte returns. Standard Go pattern for ctx-aware
	// net.Conn reads without an extra goroutine per Read.
	stop := c.watchCancel(ctx)
	defer stop()

	c.neg.sendInitialOffers(ctx)

	// Line buffer caps at MaxLineBytes of POST-protocol data. A peer
	// streaming subneg bytes can't fill it; only real payload counts.
	buf := make([]byte, 0, 64)
	for {
		b, err := c.r.ReadByte()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", ctxErr
			}
			if errors.Is(err, io.EOF) {
				// Return whatever payload we'd accumulated so a peer
				// that closed mid-line still surfaces a final line on
				// the EOF (matches the pre-M16.1 behavior).
				return strings.TrimRight(string(buf), "\r"), io.EOF
			}
			select {
			case <-c.done:
				return "", conn.ErrClosed
			default:
			}
			return "", fmt.Errorf("telnet.Read: %w", err)
		}

		data, isData := c.neg.feed(ctx, b)
		if !isData {
			continue
		}
		// Char-at-a-time mode (post-login, raw clients): the editor owns
		// echo + line editing and returns a line on Enter. Read's full-
		// line contract is unchanged.
		if c.charModeActive() {
			if line, done := c.charModeByte(ctx, &buf, data); done {
				return line, nil
			}
			continue
		}
		if data == '\n' {
			return strings.TrimRight(string(buf), "\r"), nil
		}
		if len(buf) >= MaxLineBytes {
			// Drain remaining bytes of the oversized line so the
			// next Read aligns to a clean line boundary. Subneg
			// state is preserved across this drain — the negotiator
			// keeps consuming protocol bytes correctly.
			c.discardToNewline(ctx)
			return "", conn.ErrLineTooLong
		}
		buf = append(buf, data)
	}
}

// Telnet command bytes we recognize. Defined in RFC 854 / RFC 855.
// Duplicated here against negotiator.go's neg* constants so the
// write-side helpers (escapeIAC, IAC byte literals in
// WriteCommand callers) read in terms of the same names without
// taking a cross-file dependency.
const (
	tnIAC  byte = 0xFF
	tnSB   byte = 0xFA // start of subnegotiation
	tnSE   byte = 0xF0 // end of subnegotiation
	tnWILL byte = 0xFB
	tnWONT byte = 0xFC
	tnDO   byte = 0xFD
	tnDONT byte = 0xFE
)

// stripIAC is retained for the write-side round-trip test below;
// production reads go through the byte-level negotiator
// (negotiator.feed) since M16.1. Kept rather than deleted because
// the round-trip property test still validates that escapeIAC's
// output decodes back to the original on the receive path.
func stripIAC(s string) string {
	if strings.IndexByte(s, tnIAC) < 0 {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != tnIAC {
			out = append(out, s[i])
			continue
		}
		if i+1 >= len(s) {
			break
		}
		cmd := s[i+1]
		switch cmd {
		case tnIAC:
			out = append(out, tnIAC)
			i++
		case tnWILL, tnWONT, tnDO, tnDONT:
			if i+2 >= len(s) {
				return string(out)
			}
			i += 2
		case tnSB:
			i += 2
			for i < len(s)-1 {
				if s[i] == tnIAC && s[i+1] == tnSE {
					i++
					break
				}
				i++
			}
		default:
			i++
		}
	}
	return string(out)
}

// discardToNewline reads bytes through the negotiator until a data
// '\n' surfaces or any error occurs. Called after ErrLineTooLong so
// the buffer realigns to the next real line boundary. Drives the
// negotiator (rather than ReadSlice) so subnegotiations that arrive
// in the discarded tail keep updating option state cleanly.
func (c *Conn) discardToNewline(ctx context.Context) {
	for {
		b, err := c.r.ReadByte()
		if err != nil {
			return
		}
		data, isData := c.neg.feed(ctx, b)
		if isData && data == '\n' {
			return
		}
	}
}

// Write implements conn.Connection. Safe for concurrent callers.
//
// Any literal 0xFF byte in p is doubled before reaching the wire so
// the client cannot interpret it as a telnet IAC. Symmetric to
// stripIAC on the read path. Without this escape, a mob/room/player
// name that contains 0xFF (whether by content-pack bug or a
// malicious crafted input) injects a negotiation command into the
// downstream protocol — RFC 854 §SP-IAC.
//
// The escape allocates only when an IAC byte is actually present,
// so the common ASCII / UTF-8-without-0xFF text path is zero-copy.
// The return value reports bytes-from-the-original-p that were
// covered by the raw write, NOT the post-escape length, so callers
// that compare n against len(p) keep their invariant.
func (c *Conn) Write(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// Proper NVT line endings first (RFC 854): expand bare '\n' to "\r\n"
	// so multi-line output renders correctly on a raw / character-at-a-time
	// client (char-mode), which does no ONLCR translation. Then escape IAC.
	out := crlfNormalize(p)
	if idx := bytesIndexByte(out, tnIAC); idx >= 0 {
		out = escapeIAC(out, idx)
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	n, err := c.raw.Write(out)
	if err != nil {
		select {
		case <-c.done:
			return n, conn.ErrClosed
		default:
		}
		return n, fmt.Errorf("telnet.Write: %w", err)
	}
	// Report bytes-of-original p, not the post-escape length, so the
	// io.Writer contract holds for callers that do not know about the
	// escape. The only way err is nil and n < len(out) is a partial
	// raw write — propagate the partial-write semantics by scaling.
	if n == len(out) {
		return len(p), nil
	}
	// Partial write across escape — approximate the original-p
	// coverage by mapping back through the escape. Worst case the
	// caller sees n < len(p) and retries; the raw conn already
	// committed `n` bytes of `out`, so a retry covers what was
	// missed without producing a torn IAC pair (escapeIAC keeps
	// each 0xFF pair contiguous, and net.Conn writes are
	// committed-prefix on success).
	return mapEscapedWriteCount(p, n), nil
}

// WriteCommand writes a pre-formed telnet command sequence verbatim,
// WITHOUT the IAC-doubling that Write applies. The bytes ARE telnet
// protocol (a leading IAC 0xFF followed by a command), so escaping them
// would corrupt the command — e.g. the login echo-suppression sequence
// IAC WILL ECHO {0xFF,0xFB,0x01} would reach the client as
// {0xFF,0xFF,0xFB,0x01}, which it reads as a literal 0xFF (rendered as
// garbage) plus a malformed negotiation, leaving password masking off.
//
// Callers must pass only well-formed telnet command sequences; arbitrary
// user/content text must go through Write so its 0xFF bytes are escaped.
func (c *Conn) WriteCommand(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	n, err := c.raw.Write(p)
	if err != nil {
		select {
		case <-c.done:
			return n, conn.ErrClosed
		default:
		}
		return n, fmt.Errorf("telnet.WriteCommand: %w", err)
	}
	return n, nil
}

// bytesIndexByte mirrors bytes.IndexByte without forcing the import
// graph to add `bytes` for one call. Sentinel return -1 matches the
// stdlib convention.
func bytesIndexByte(s []byte, b byte) int {
	for i := range s {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// crlfNormalize expands every bare '\n' (one not already preceded by '\r')
// into "\r\n" so telnet output uses proper NVT line endings (RFC 854)
// regardless of the client's terminal mode. Without it, a multi-line render
// (room descriptions join their lines with '\n') "staircases" on a raw /
// character-at-a-time client: the line feed drops a row but the cursor never
// returns to column 0, because a raw client does no ONLCR translation. A
// cooked/line-mode client that DOES translate just sees a harmless extra CR.
// Idempotent on existing "\r\n". Allocates only when a bare '\n' is present,
// so the already-CRLF and no-newline paths stay zero-copy. The "preceded by
// '\r'" test reads the INPUT byte, so a source "\r\n" is left intact.
func crlfNormalize(p []byte) []byte {
	bare := false
	for i := range p {
		if p[i] == '\n' && (i == 0 || p[i-1] != '\r') {
			bare = true
			break
		}
	}
	if !bare {
		return p
	}
	out := make([]byte, 0, len(p)+8)
	for i := range p {
		if p[i] == '\n' && (i == 0 || p[i-1] != '\r') {
			out = append(out, '\r', '\n')
		} else {
			out = append(out, p[i])
		}
	}
	return out
}

// escapeIAC returns a fresh slice where every 0xFF byte from p has
// been doubled. firstIAC is the index of the first 0xFF in p so the
// allocation runs only on the suffix that needs work.
func escapeIAC(p []byte, firstIAC int) []byte {
	// Worst case: every remaining byte is 0xFF → doubled. Size the
	// allocation pessimistically once rather than appending in a
	// loop with reallocations.
	out := make([]byte, 0, len(p)+(len(p)-firstIAC))
	out = append(out, p[:firstIAC]...)
	for i := firstIAC; i < len(p); i++ {
		if p[i] == tnIAC {
			out = append(out, tnIAC, tnIAC)
		} else {
			out = append(out, p[i])
		}
	}
	return out
}

// mapEscapedWriteCount approximates the count of bytes from p that
// the raw conn covered, given that nWritten bytes of the escaped
// form were committed. Walks p, counting one byte of "escaped
// budget" for ordinary bytes and two for 0xFF, stopping when the
// budget runs out.
func mapEscapedWriteCount(p []byte, nWritten int) int {
	remain := nWritten
	for i := range p {
		cost := 1
		if p[i] == tnIAC {
			cost = 2
		}
		if remain < cost {
			return i
		}
		remain -= cost
	}
	return len(p)
}

// Close implements conn.Connection. Safe to call more than once.
func (c *Conn) Close() error {
	var err error
	c.once.Do(func() {
		close(c.done)
		err = c.raw.Close()
	})
	return err
}

// watchCancel arranges for ctx cancellation to interrupt a blocked
// Read by closing the connection through Close — which is sync.Once
// guarded — rather than touching c.raw directly. Direct c.raw.Close
// would race with the deferred Close in the handler and risk closing
// an fd that the OS has already recycled to a new connection.
// The returned func must be called when the Read completes so the
// watcher goroutine exits.
func (c *Conn) watchCancel(ctx context.Context) func() {
	if ctx.Done() == nil {
		return func() {}
	}
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-stop:
		case <-c.done:
		}
	}()
	return func() { close(stop) }
}

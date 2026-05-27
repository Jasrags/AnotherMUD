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
}

// New wraps an established net.Conn. id should be a stable identifier
// (typically a UUID or monotonic counter) assigned by the server.
func New(id string, c net.Conn) *Conn {
	// bufio over LimitReader caps total bytes read across the connection
	// lifetime, which would be wrong here — we want a per-Read cap. Track
	// the limit in Read itself via bufio.Reader.Buffered / Peek instead.
	return &Conn{
		id:   id,
		raw:  c,
		r:    bufio.NewReaderSize(c, MaxLineBytes),
		done: make(chan struct{}),
	}
}

// ID implements conn.Connection.
func (c *Conn) ID() string { return c.id }

// Read implements conn.Connection. It returns one line at a time with
// the trailing CR/LF stripped.
//
// Uses bufio.Reader.ReadSlice rather than ReadString/ReadBytes so the
// per-line buffer is hard-capped at MaxLineBytes — ReadString grows
// the buffer past its initial size, which would defeat the cap.
func (c *Conn) Read(ctx context.Context) (string, error) {
	// Honor ctx cancellation by closing the underlying conn so the
	// blocked ReadSlice returns. Standard Go pattern for ctx-aware
	// net.Conn reads without an extra goroutine per Read.
	stop := c.watchCancel(ctx)
	defer stop()

	slice, err := c.r.ReadSlice('\n')
	// slice aliases the bufio buffer, so copy what we need before
	// any subsequent Read invalidates it.
	line := stripIAC(strings.TrimRight(string(slice), "\r\n"))

	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			// Peer sent MaxLineBytes without a newline. Drain whatever
			// remains of this oversized line so a subsequent Read doesn't
			// return its tail as a phantom "line".
			c.discardToNewline()
			return "", conn.ErrLineTooLong
		}
		if errors.Is(err, io.EOF) {
			return line, io.EOF
		}
		select {
		case <-c.done:
			return "", conn.ErrClosed
		default:
		}
		return "", fmt.Errorf("telnet.Read: %w", err)
	}
	return line, nil
}

// Telnet command bytes we recognize. Defined in RFC 854 / RFC 855.
const (
	tnIAC  byte = 0xFF
	tnSB   byte = 0xFA // start of subnegotiation
	tnSE   byte = 0xF0 // end of subnegotiation
	tnWILL byte = 0xFB
	tnWONT byte = 0xFC
	tnDO   byte = 0xFD
	tnDONT byte = 0xFE
)

// stripIAC removes telnet IAC sequences from line. We do not maintain
// option state — we simply skip command bytes so they don't leak into
// the line-oriented input the server sees.
//
// Recognized shapes:
//
//   - IAC IAC          → literal 0xFF byte (kept)
//   - IAC WILL/WONT/DO/DONT <opt> → 3-byte negotiation, dropped
//   - IAC SB ... IAC SE → subnegotiation block, dropped
//   - IAC <other>      → 2-byte command, dropped
//
// A trailing lone IAC at the end of the buffer is dropped; the
// likeliest cause is a truncated read, and emitting a stray 0xFF byte
// would confuse later validators (e.g. login's ASCII check).
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
		// We saw IAC. Decide based on the following byte.
		if i+1 >= len(s) {
			break // truncated IAC; drop
		}
		cmd := s[i+1]
		switch cmd {
		case tnIAC:
			// Escaped literal 0xFF.
			out = append(out, tnIAC)
			i++
		case tnWILL, tnWONT, tnDO, tnDONT:
			// 3-byte negotiation: IAC CMD OPT. Skip OPT.
			if i+2 >= len(s) {
				return string(out)
			}
			i += 2
		case tnSB:
			// Subnegotiation: skip until IAC SE.
			i += 2
			for i < len(s)-1 {
				if s[i] == tnIAC && s[i+1] == tnSE {
					i++
					break
				}
				i++
			}
		default:
			// 2-byte command (NOP, GA, etc).
			i++
		}
	}
	return string(out)
}

// discardToNewline reads and discards bytes until the next newline or
// any error. Called after ErrLineTooLong so the buffer is realigned to
// the next real line boundary.
func (c *Conn) discardToNewline() {
	for {
		_, err := c.r.ReadSlice('\n')
		if err == nil || !errors.Is(err, bufio.ErrBufferFull) {
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
	out := p
	if idx := bytesIndexByte(p, tnIAC); idx >= 0 {
		out = escapeIAC(p, idx)
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

// bytesIndexByte mirrors bytes.IndexByte without forcing the import
// graph to add `bytes` for one call. Sentinel return -1 matches the
// stdlib convention.
func bytesIndexByte(s []byte, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
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
	for i := 0; i < len(p); i++ {
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

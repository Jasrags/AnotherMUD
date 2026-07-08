// Package telnettest is a small, engine-agnostic telnet send/expect client for
// driving a MUD-style line server in tests and ad-hoc smoke scripts.
//
// It is deliberately decoupled from any AnotherMUD internals: the core operates
// on a net.Conn, so it can be exercised in isolation against an in-memory
// net.Pipe() fake (see client_test.go) with no engine running — satisfying the
// "unit-level send/expect must not require the full engine" constraint. It
// strips telnet IAC negotiation from the read stream (see iac.go) so expect
// matching sees clean text.
//
// The API is layered: this package provides only low-level primitives (Send,
// SendLine, Expect, Drain, Interact). Engine-specific helpers — login flows,
// prompt patterns, named scenarios — compose these primitives one layer up
// (today in cmd/telnet-smoke) and never modify this core.
//
// A Client is intended for single-goroutine use (one test, one script). Send
// and Expect serialize through an internal lock for memory safety, but a Client
// is not designed for concurrent Send-while-Expect from multiple goroutines.
package telnettest

import (
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout bounds an Expect that doesn't specify its own deadline.
const DefaultTimeout = 5 * time.Second

// TB is the minimal subset of testing.TB that DialT needs. Declaring it here
// (rather than importing "testing") keeps the testing package — and its global
// flags — out of any non-test binary that links this package, e.g. the
// cmd/telnet-smoke tool. *testing.T and *testing.B satisfy it.
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
	Cleanup(func())
}

// Client is a telnet send/expect session over a single connection.
type Client struct {
	conn       net.Conn
	r          io.Reader  // IAC-filtered view of conn (or conn itself, see WithoutIACFilter)
	iac        *iacReader // the filtering reader (nil path unused; kept for GMCP capture wiring)
	mu         sync.Mutex
	buf        []byte // clean bytes read but not yet consumed by an Expect
	timeout    time.Duration
	transcript io.Writer
}

// Option configures a Client at construction.
type Option func(*Client)

// WithTimeout sets the default Expect timeout (ignored if d <= 0).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithTranscript tees all clean (IAC-stripped) server output to w as it
// arrives — useful for dumping a failing scenario's full exchange.
func WithTranscript(w io.Writer) Option {
	return func(c *Client) { c.transcript = w }
}

// WithoutIACFilter disables telnet IAC stripping, so Expect sees the raw byte
// stream. For non-telnet servers or for testing the raw path.
func WithoutIACFilter() Option {
	return func(c *Client) { c.r = c.conn }
}

// WithGMCPCapture registers a callback invoked once per complete GMCP frame the
// server sends, with the package name and its JSON payload split apart (the
// payload is "" for a bare package name). The callback fires on the reader
// goroutine while an Expect/Drain read is in flight, so it MUST NOT call back
// into this Client (append to your own synchronized store instead). Requires
// the IAC filter (the default); a no-op under WithoutIACFilter. Pair with
// ActivateGMCP so the server actually starts sending frames.
func WithGMCPCapture(fn func(pkg, json string)) Option {
	return func(c *Client) {
		if c.iac == nil || fn == nil {
			return
		}
		c.iac.onGMCP = func(raw string) {
			pkg, payload := raw, ""
			if i := strings.IndexByte(raw, ' '); i >= 0 {
				pkg, payload = raw[:i], raw[i+1:]
			}
			fn(pkg, payload)
		}
	}
}

// ActivateGMCP sends IAC DO GMCP, telling the server this client accepts GMCP
// frames. The engine advertises WILL GMCP at boot and starts emitting only
// after the client's DO; the plain send/expect harness never answers
// negotiation, so a capture client must call this explicitly after connecting.
func (c *Client) ActivateGMCP() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.conn.Write([]byte{iac, doo, optGMCP}); err != nil {
		return fmt.Errorf("telnettest: activate GMCP: %w", err)
	}
	return nil
}

// New wraps an existing connection. Exposed primarily so tests can drive the
// client against a net.Pipe() fake server with no real socket.
func New(conn net.Conn, opts ...Option) *Client {
	c := &Client{conn: conn, timeout: DefaultTimeout}
	c.iac = newIACReader(conn)
	c.r = c.iac
	for _, o := range opts {
		o(c)
	}
	return c
}

// Dial opens a TCP connection to addr ("host:port").
func Dial(addr string, opts ...Option) (*Client, error) {
	return DialContext(context.Background(), addr, opts...)
}

// DialContext opens a TCP connection to addr, honoring ctx for the dial.
func DialContext(ctx context.Context, addr string, opts ...Option) (*Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("telnettest: dial %s: %w", addr, err)
	}
	return New(conn, opts...), nil
}

// DialT dials addr for a test: it fails the test on a dial error and registers
// Close as cleanup, so callers don't repeat that boilerplate.
func DialT(tb TB, addr string, opts ...Option) *Client {
	tb.Helper()
	c, err := Dial(addr, opts...)
	if err != nil {
		tb.Fatalf("telnettest: %v", err)
		return nil // unreachable in real testing.T, but keeps the type checker happy
	}
	tb.Cleanup(func() { _ = c.Close() })
	return c
}

// Send writes s verbatim (no line terminator).
func (c *Client) Send(s string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := io.WriteString(c.conn, s); err != nil {
		return fmt.Errorf("telnettest: send %q: %w", s, err)
	}
	return nil
}

// SendLine writes s followed by CRLF — the telnet line terminator. (AnotherMUD
// accepts a bare LF during login/wizard but wants CRLF for in-game commands;
// always sending CRLF is the safe universal choice.)
func (c *Client) SendLine(s string) error {
	return c.Send(s + "\r\n")
}

// Expect waits up to the client's default timeout for the server output to
// match re, returning the text up to and including the match. Matched bytes are
// consumed; trailing bytes remain buffered for the next Expect.
func (c *Client) Expect(re *regexp.Regexp) (string, error) {
	return c.ExpectTimeout(re, c.timeout)
}

// ExpectString is Expect for a literal substring.
func (c *Client) ExpectString(substr string) (string, error) {
	return c.ExpectTimeout(regexp.MustCompile(regexp.QuoteMeta(substr)), c.timeout)
}

// ExpectStringTimeout is ExpectTimeout for a literal substring.
func (c *Client) ExpectStringTimeout(substr string, d time.Duration) (string, error) {
	return c.ExpectTimeout(regexp.MustCompile(regexp.QuoteMeta(substr)), d)
}

// ExpectTimeout waits up to d for re to match. On timeout it returns whatever
// was buffered plus an error naming the pattern and a tail of the last output,
// so a failing test points at what the server actually said.
func (c *Client) ExpectTimeout(re *regexp.Regexp, d time.Duration) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	deadline := time.Now().Add(d)
	for {
		if matched, ok := c.consumeMatchLocked(re); ok {
			return matched, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return string(c.buf), fmt.Errorf("telnettest: timeout after %s waiting for /%s/; last output: %q", d, re, tailString(c.buf, 200))
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(remaining))
		n, err := c.readChunkLocked()
		if err != nil {
			if matched, ok := c.consumeMatchLocked(re); ok {
				return matched, nil
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue // deadline accounting happens at the loop top
			}
			return string(c.buf), fmt.Errorf("telnettest: read while waiting for /%s/: %w", re, err)
		}
		_ = n
	}
}

// Drain reads and returns whatever arrives within d, with no pattern to match.
// It clears the buffer. Useful to flush a banner or capture trailing output.
func (c *Client) Drain(d time.Duration) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	deadline := time.Now().Add(d)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(remaining))
		if _, err := c.readChunkLocked(); err != nil {
			break // timeout or EOF: stop draining
		}
	}
	out := string(c.buf)
	c.buf = nil
	return out
}

// Interact streams the connection to/from the given reader/writer until either
// side closes or ctx is cancelled — the standalone interactive mode. Any
// already-buffered server output is flushed to out first.
func (c *Client) Interact(ctx context.Context, in io.Reader, out io.Writer) error {
	c.mu.Lock()
	if len(c.buf) > 0 {
		_, _ = out.Write(c.buf)
		c.buf = nil
	}
	c.mu.Unlock()
	_ = c.conn.SetReadDeadline(time.Time{}) // stream without a deadline
	errc := make(chan error, 2)
	go func() { _, err := io.Copy(c.conn, in); errc <- err }() // local stdin → server
	go func() { _, err := io.Copy(out, c.r); errc <- err }()   // server → local stdout (filtered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

// Close closes the underlying connection.
func (c *Client) Close() error { return c.conn.Close() }

// consumeMatchLocked checks the buffer for re; on a hit it consumes through the
// match end and returns the matched prefix. Caller holds c.mu.
func (c *Client) consumeMatchLocked(re *regexp.Regexp) (string, bool) {
	loc := re.FindIndex(c.buf)
	if loc == nil {
		return "", false
	}
	matched := string(c.buf[:loc[1]])
	c.buf = append([]byte(nil), c.buf[loc[1]:]...)
	return matched, true
}

// readChunkLocked does one read into the buffer, teeing to the transcript.
// Caller holds c.mu and has set the read deadline.
func (c *Client) readChunkLocked() (int, error) {
	chunk := make([]byte, 4096)
	n, err := c.r.Read(chunk)
	if n > 0 {
		c.buf = append(c.buf, chunk[:n]...)
		if c.transcript != nil {
			_, _ = c.transcript.Write(chunk[:n])
		}
	}
	return n, err
}

func tailString(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

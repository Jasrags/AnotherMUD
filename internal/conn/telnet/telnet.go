// Package telnet implements conn.Connection over a raw TCP socket.
//
// M0 scope: line-oriented reads, no telnet option negotiation. Anything
// that arrives is treated as 7-bit ASCII text up to the next CRLF or LF.
// Telnet IAC negotiation, GMCP, and MSSP land in a later milestone per
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
	return &Conn{
		id:   id,
		raw:  c,
		r:    bufio.NewReader(c),
		done: make(chan struct{}),
	}
}

// ID implements conn.Connection.
func (c *Conn) ID() string { return c.id }

// Read implements conn.Connection. It returns one line at a time with
// the trailing CR/LF stripped.
func (c *Conn) Read(ctx context.Context) (string, error) {
	// Honor ctx cancellation by closing the underlying conn so the
	// blocked ReadString returns. This is the standard Go pattern for
	// making net.Conn ctx-aware without an extra goroutine per Read.
	stop := c.watchCancel(ctx)
	defer stop()

	line, err := c.r.ReadString('\n')
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if errors.Is(err, io.EOF) {
			return strings.TrimRight(line, "\r\n"), io.EOF
		}
		select {
		case <-c.done:
			return "", conn.ErrClosed
		default:
		}
		return "", fmt.Errorf("telnet.Read: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// Write implements conn.Connection. Safe for concurrent callers.
func (c *Conn) Write(ctx context.Context, p []byte) (int, error) {
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
		return n, fmt.Errorf("telnet.Write: %w", err)
	}
	return n, nil
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
// Read by closing the underlying connection. The returned func must
// be called when the Read completes so the watcher goroutine exits.
func (c *Conn) watchCancel(ctx context.Context) func() {
	if ctx.Done() == nil {
		return func() {}
	}
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = c.raw.Close()
		case <-stop:
		case <-c.done:
		}
	}()
	return func() { close(stop) }
}

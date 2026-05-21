// Package conn defines the Connection abstraction used by the server.
//
// Spec note: networking-protocols calls this IConnection; Go convention
// drops the "I" prefix, so the type is simply Connection. Behavior
// matches the spec — Read, Write, Close, ID — nothing more in M0.
// Telnet negotiation, GMCP, MSSP, and WebSocket envelopes land later.
package conn

import (
	"context"
	"errors"
	"io"
)

// ErrClosed is returned by Read/Write on a Connection whose Close has
// already been called or whose underlying transport has gone away.
var ErrClosed = errors.New("connection closed")

// Connection is the minimal byte-stream abstraction over a player's
// transport (telnet today; WebSocket, etc. later).
//
// Implementations must be safe for concurrent Write from multiple
// goroutines. Read is expected to be called from a single goroutine
// (typically the connection's own read loop).
type Connection interface {
	// ID returns a stable identifier for this connection, used as the
	// session_id field in structured logs at this layer.
	ID() string

	// Read reads a single line of input (newline-terminated, newline
	// stripped). Returns io.EOF when the peer closes cleanly, ErrClosed
	// when Close has been called, or ctx.Err() on cancellation.
	Read(ctx context.Context) (string, error)

	// Write sends bytes to the peer. Implementations should not assume
	// any line-framing of the input; the caller adds line endings.
	Write(ctx context.Context, p []byte) (int, error)

	// Close releases transport resources. Safe to call more than once.
	Close() error
}

// Compile-time check that io.EOF is the documented sentinel for peer
// close; if we ever swap to a custom EOF this will catch the mistake.
var _ error = io.EOF

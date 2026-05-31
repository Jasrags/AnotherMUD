// Package ws implements conn.Connection over a WebSocket transport
// per docs/specs/networking-protocols.md §6. Each direction uses
// one-text-frame JSON envelopes (`{type, data}` for text/command,
// `{type, package, data}` for GMCP). WebSocket clients always have
// GMCP available and always render ANSI (§6.5 — no per-client
// negotiation).
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/coder/websocket"

	"github.com/Jasrags/AnotherMUD/internal/conn"
)

// maxInboundBytes caps inbound message size (§6.3). A frame over
// the cap closes the connection cleanly with reason
// "message too large".
const maxInboundBytes = 64 * 1024

// envelope is the on-wire JSON shape. `Data` is RawMessage so a
// `gmcp` payload can be any JSON value (object, array, string,
// number) without a second marshal pass — the GMCP encoders ship
// pre-marshalled bytes.
//
// `Package` is populated only for `gmcp` envelopes; the
// `omitempty` keeps text/command payloads minimal on the wire.
type envelope struct {
	Type    string          `json:"type"`
	Package string          `json:"package,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Conn implements conn.Connection + the gmcpSender interface
// over a coder/websocket connection. coder/websocket serializes
// concurrent Writes internally, so Write / SendGmcp don't need
// their own mutex; the read path is single-goroutine by the
// conn.Connection contract.
type Conn struct {
	id string
	ws *websocket.Conn
}

// New wraps a freshly-accepted websocket.Conn. The caller (the
// HTTP upgrade handler in internal/server) owns the lifecycle —
// Close on this type closes the underlying socket cleanly.
//
// Sets the inbound size cap at construction so any frame past
// the limit triggers a clean close with "message too large".
func New(id string, ws *websocket.Conn) *Conn {
	ws.SetReadLimit(maxInboundBytes)
	return &Conn{id: id, ws: ws}
}

// ID implements conn.Connection.
func (c *Conn) ID() string { return c.id }

// Read implements conn.Connection. Pulls the next `command`
// envelope from the socket, ignoring any other envelope type
// (per §6.1 unknown types are silently dropped on the inbound
// side; `text` is not used client→server; `gmcp` inbound
// handling is out of M16.5 scope — it returns to the loop).
//
// Returns io.EOF on a normal peer close so the session loop's
// EOF handling fires the same path it does for telnet.
func (c *Conn) Read(ctx context.Context) (string, error) {
	for {
		msgType, data, err := c.ws.Read(ctx)
		if err != nil {
			return "", normalizeReadError(err)
		}
		if msgType != websocket.MessageText {
			// Binary frames are not part of the spec wire format;
			// drop and resume.
			continue
		}
		var env envelope
		if err := json.Unmarshal(data, &env); err != nil {
			// Malformed JSON — silently drop per §6.1.
			continue
		}
		switch env.Type {
		case "command":
			var s string
			if err := json.Unmarshal(env.Data, &s); err != nil {
				continue
			}
			return s, nil
		case "text", "gmcp":
			// `text` is server→client only; client `text` is
			// silently ignored. Inbound `gmcp` handling (Core.
			// Supports etc.) is out of M16.5 scope; WebSocket GMCP
			// is always supported per §5.2 so no subscription
			// tracking is needed.
			continue
		default:
			// Unknown type — silent drop per §6.1.
			continue
		}
	}
}

// Write implements conn.Connection. Each Write becomes one
// `{type:"text"}` text frame. coder/websocket internally
// serializes concurrent Write calls, so callers don't need an
// external mutex.
//
// Returns the original byte count on success so callers' "wrote
// N bytes" accounting matches the input — the JSON envelope is
// an implementation detail.
func (c *Conn) Write(ctx context.Context, p []byte) (int, error) {
	body, err := json.Marshal(string(p))
	if err != nil {
		return 0, err
	}
	env, err := json.Marshal(envelope{Type: "text", Data: body})
	if err != nil {
		return 0, err
	}
	if err := c.ws.Write(ctx, websocket.MessageText, env); err != nil {
		return 0, normalizeWriteError(err)
	}
	return len(p), nil
}

// Close implements conn.Connection. Safe to call more than once
// — coder/websocket.Close is idempotent on a closed socket.
func (c *Conn) Close() error {
	return c.ws.Close(websocket.StatusNormalClosure, "connection ended")
}

// GmcpActive implements the session.gmcpSender interface. Per
// §6.5 WebSocket GMCP is always supported — no negotiation
// handshake required.
func (c *Conn) GmcpActive() bool { return true }

// SupportsPackage implements the gmcpSender interface. WebSocket
// treats every package as supported (§5.2); the engine emits
// every package and the client filters client-side.
func (c *Conn) SupportsPackage(_ string) bool { return true }

// SendGmcp implements the gmcpSender interface. Ships one
// `{type:"gmcp"}` text frame carrying the package name + the
// pre-marshalled payload as a raw JSON value.
//
// `data` MUST be valid JSON — the GMCP encoders in internal/gmcp
// produce well-formed JSON, and the json.RawMessage path
// preserves it byte-for-byte on the wire.
func (c *Conn) SendGmcp(ctx context.Context, pkg string, data []byte) error {
	env, err := json.Marshal(envelope{Type: "gmcp", Package: pkg, Data: data})
	if err != nil {
		return err
	}
	if err := c.ws.Write(ctx, websocket.MessageText, env); err != nil {
		return normalizeWriteError(err)
	}
	return nil
}

// normalizeReadError maps coder/websocket close errors to the
// conn package's contract — a clean close returns io.EOF, an
// over-limit frame returns a distinct sentinel.
func normalizeReadError(err error) error {
	if err == nil {
		return nil
	}
	status := websocket.CloseStatus(err)
	if status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
		return io.EOF
	}
	if status == websocket.StatusMessageTooBig {
		return conn.ErrLineTooLong
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// Other close codes / network errors surface to the caller
	// verbatim so the session loop can log the cause.
	return err
}

// normalizeWriteError maps a write to a closed socket back to the
// conn.ErrClosed sentinel callers expect.
func normalizeWriteError(err error) error {
	if err == nil {
		return nil
	}
	status := websocket.CloseStatus(err)
	if status != -1 {
		return conn.ErrClosed
	}
	return err
}

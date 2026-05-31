package server

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"

	"github.com/Jasrags/AnotherMUD/internal/conn/ws"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// WebSocketOptions tunes the HTTP-upgrade handler returned by
// NewWebSocketHandler. The zero value is a sensible default:
// no Subprotocols restriction, accept the default origins (same-
// origin only — the caller MUST configure InsecureSkipVerify or
// OriginPatterns explicitly for cross-origin use).
type WebSocketOptions struct {
	// Subprotocols passes through to websocket.AcceptOptions. Empty
	// = no subprotocol negotiation; the server accepts whatever
	// the client offers.
	Subprotocols []string

	// OriginPatterns passes through to websocket.AcceptOptions.
	// Empty = same-origin only. A development setup that runs the
	// web client on a different port should explicitly add the
	// expected origin patterns rather than disabling the check.
	OriginPatterns []string

	// InsecureSkipVerify passes through to websocket.AcceptOptions.
	// Set to true ONLY in development; production deployments
	// should enumerate origins via OriginPatterns instead.
	InsecureSkipVerify bool
}

// NewWebSocketHandler returns an http.Handler that upgrades each
// incoming HTTP request to a WebSocket, wraps it in a ws.Conn,
// and dispatches to the Server's Handler.
//
// Connection ids reuse the Server's nextID counter so telnet and
// websocket sessions share one numbering space — useful for cross-
// transport log correlation.
//
// Shutdown drain: handler in-flight tracking is owned by the
// caller's http.Server.Shutdown, NOT by Server.wg. (Server.Serve's
// `defer s.wg.Wait()` runs synchronously inside Serve and only
// drains telnet handlers — the http.Server runs in a separate
// goroutine.) The composition root in cmd/anothermud calls
// wsHTTP.Shutdown on ctx-cancel; Shutdown blocks until every
// in-flight WS handler returns.
func NewWebSocketHandler(s *Server, opts WebSocketOptions) http.Handler {
	acceptOpts := &websocket.AcceptOptions{
		Subprotocols:       opts.Subprotocols,
		OriginPatterns:     opts.OriginPatterns,
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Handler == nil {
			http.Error(w, "no handler", http.StatusInternalServerError)
			return
		}
		wsConn, err := websocket.Accept(w, r, acceptOpts)
		if err != nil {
			// websocket.Accept already wrote the HTTP error response.
			return
		}
		id := s.nextID()
		c := ws.New(id, wsConn)
		defer c.Close()

		ctx := r.Context()
		connCtx := logging.With(ctx, slog.String("session_id", id))
		logging.From(connCtx).Info("ws connection accepted",
			slog.String("remote_addr", r.RemoteAddr))

		if err := s.Handler(connCtx, c); err != nil && !errors.Is(err, io.EOF) {
			logging.From(connCtx).Warn("ws handler exited with error", slog.Any("err", err))
		}

		logging.From(connCtx).Info("ws connection closed")
	})
}

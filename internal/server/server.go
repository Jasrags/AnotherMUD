// Package server runs the accept loop and dispatches each accepted
// connection to a handler. In M0 the handler is a simple echo loop;
// later milestones replace it with the session/login pipeline.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/conn/telnet"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// ErrServerClosed is returned by Serve when the listener is closed or
// the context is cancelled — distinguishes a clean shutdown from a
// genuine accept error.
var ErrServerClosed = errors.New("server closed")

// Handler is called once per accepted connection in its own goroutine.
// The ctx passed in carries a per-connection logger with session_id
// attached (Foundations F2).
type Handler func(ctx context.Context, c conn.Connection) error

// Server accepts connections from a net.Listener and runs Handler on
// each one. The connection-id factory is overridable for tests.
type Server struct {
	Handler Handler

	// NewID assigns connection identifiers. Defaults to a monotonic
	// counter; tests can swap it for a deterministic source.
	NewID func() string

	idCounter atomic.Uint64
	wg        sync.WaitGroup
}

// Serve accepts connections on ln until ctx is cancelled or ln is
// closed, then waits for in-flight handlers to return. The handler
// drain runs on every exit path — including a genuine Accept error —
// so callers never see Serve return while handler goroutines are
// still holding live connections.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	if s.Handler == nil {
		return errors.New("server.Serve: Handler is nil")
	}

	// Drain handlers on every exit path. Without this, the genuine
	// Accept-error branch below would orphan in-flight connections.
	defer s.wg.Wait()

	// Close the listener when ctx is cancelled so Accept unblocks.
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = ln.Close()
		case <-stop:
		}
	}()
	defer close(stop)

	logging.From(ctx).Info("server listening", slog.String("addr", ln.Addr().String()))

	for {
		nc, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return ErrServerClosed
			}
			return fmt.Errorf("server.Accept: %w", err)
		}

		id := s.nextID()
		s.wg.Add(1)
		go s.handle(ctx, id, nc)
	}
}

func (s *Server) handle(ctx context.Context, id string, nc net.Conn) {
	defer s.wg.Done()

	c := telnet.New(id, nc)
	defer c.Close()

	connCtx := logging.With(ctx, slog.String("session_id", id))
	logging.From(connCtx).Info("connection accepted",
		slog.String("remote_addr", nc.RemoteAddr().String()))

	if err := s.Handler(connCtx, c); err != nil && !errors.Is(err, io.EOF) {
		logging.From(connCtx).Warn("handler exited with error", slog.Any("err", err))
	}

	logging.From(connCtx).Info("connection closed")
}

func (s *Server) nextID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	return "c" + strconv.FormatUint(s.idCounter.Add(1), 10)
}

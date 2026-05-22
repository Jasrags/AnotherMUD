// Package session bridges the connection layer (internal/conn) to the
// command dispatcher (internal/command) and the world (internal/world).
//
// In M1 a "session" is the thinnest possible thing: one accepted
// connection, one player in one room, one read→dispatch→render loop.
// docs/specs/session-lifecycle.md splits PlayerSession from Connection
// properly in M4; what lives here today is the seam that loop will
// occupy.
package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"unicode"

	"github.com/Jasrags/AnotherMUD/internal/ansi"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Config is the per-server wiring the session loop needs.
type Config struct {
	World    *world.World
	Commands *command.Registry
	StartID  world.RoomID
	// ColorEnabled is the per-session default for ANSI color output.
	// Wired from NO_COLOR / ANOTHERMUD_COLOR in cmd/anothermud.
	ColorEnabled bool
}

// Handler returns a server.Handler-compatible function that runs the
// M1 game loop on each accepted connection. It is a method-less builder
// so the server package does not need to depend on this package.
func Handler(cfg Config) func(ctx context.Context, c conn.Connection) error {
	return func(ctx context.Context, c conn.Connection) error {
		return run(ctx, c, cfg)
	}
}

func run(ctx context.Context, c conn.Connection, cfg Config) error {
	start, err := cfg.World.Room(cfg.StartID)
	if err != nil {
		return fmt.Errorf("session: load starting room: %w", err)
	}

	a := &connActor{id: c.ID(), conn: c, room: start, colorEnabled: cfg.ColorEnabled}

	if err := a.Write(ctx, "Welcome to AnotherMUD."); err != nil {
		return fmt.Errorf("greet: %w", err)
	}
	if err := a.Write(ctx, command.RenderRoom(start)); err != nil {
		return fmt.Errorf("greet render: %w", err)
	}

	for {
		line, err := c.Read(ctx)
		if err != nil {
			switch {
			case errors.Is(err, io.EOF), errors.Is(err, conn.ErrClosed):
				return nil
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				return nil
			case errors.Is(err, conn.ErrLineTooLong):
				_ = a.Write(ctx, "Input too long; truncated.")
				continue
			default:
				return fmt.Errorf("read: %w", err)
			}
		}

		logging.From(ctx).Debug("input received", slog.String("line", sanitizeForLog(line)))

		if err := cfg.Commands.Dispatch(ctx, cfg.World, a, line); err != nil {
			if errors.Is(err, command.ErrQuit) {
				return nil
			}
			logging.From(ctx).Warn("command handler error", slog.Any("err", err))
		}
	}
}

// connActor adapts a conn.Connection to the command.Actor interface.
// The room pointer is guarded by a mutex so a future tick-driven event
// (e.g. a mob shove from another goroutine) can safely re-locate the
// player; M1 only mutates it from the read loop, but the seam is here.
type connActor struct {
	id   string
	conn conn.Connection

	mu           sync.Mutex
	room         *world.Room
	colorEnabled bool
}

func (a *connActor) ID() string { return a.id }

func (a *connActor) Room() *world.Room {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.room
}

func (a *connActor) SetRoom(r *world.Room) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.room = r
}

func (a *connActor) ColorEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.colorEnabled
}

func (a *connActor) SetColorEnabled(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.colorEnabled = v
}

// Write expands any color markup in msg (per internal/ansi) according
// to the actor's current color preference, then writes the rendered
// text plus CRLF to the connection.
func (a *connActor) Write(ctx context.Context, msg string) error {
	rendered := ansi.Render(msg, a.ColorEnabled())
	_, err := a.conn.Write(ctx, []byte(rendered+"\r\n"))
	return err
}

// sanitizeForLog scrubs raw client input before it lands in a structured
// log record. The peer is unauthenticated and can send arbitrary bytes,
// including ANSI escapes or other control characters that could corrupt
// downstream log viewers or hide content from operators. We coerce to
// valid UTF-8 and replace any C0/C1 control rune (except tab) with U+FFFD
// before the value reaches slog.
func sanitizeForLog(s string) string {
	s = strings.ToValidUTF8(s, string(unicode.ReplacementChar))
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return unicode.ReplacementChar
		}
		return r
	}, s)
}

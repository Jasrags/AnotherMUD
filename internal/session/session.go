// Package session bridges the connection layer (internal/conn) to the
// command dispatcher (internal/command), the world (internal/world), and
// the M3 login + persistence layer.
//
// In M3 a session is still one connection ↔ one character, but logged-
// in characters are tracked in a Manager so autosave and shutdown can
// iterate them. The connection/session split proper lands in M4 per
// docs/specs/session-lifecycle.md.
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
	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Config is the per-server wiring the session loop needs.
type Config struct {
	World    *world.World
	Commands *command.Registry
	Players  *player.Store
	Login    login.Config

	// StartID is the fallback starting room when a character's saved
	// location is not present in the loaded world (e.g. a room was
	// removed from content between restarts).
	StartID world.RoomID

	// ColorEnabled is the per-session default for ANSI color output.
	ColorEnabled bool

	// Manager tracks logged-in sessions for autosave + shutdown sweeps.
	// Required.
	Manager *Manager
}

// Handler returns a server.Handler-compatible function that drives one
// connection through login and into the game loop.
func Handler(cfg Config) func(ctx context.Context, c conn.Connection) error {
	return func(ctx context.Context, c conn.Connection) error {
		return run(ctx, c, cfg)
	}
}

func run(ctx context.Context, c conn.Connection, cfg Config) error {
	loaded, err := login.Run(ctx, c, cfg.Login)
	if err != nil {
		if errors.Is(err, login.ErrAborted) {
			return nil
		}
		// ErrPasswordCap / ErrEmailCap are terminal but not actionable
		// for the server — log + close.
		logging.From(ctx).Info("login ended", slog.Any("err", err))
		return nil
	}

	ctx = logging.With(ctx,
		slog.String("player", loaded.Player.Name),
		slog.String("account_id", loaded.Account.ID))

	start, err := resolveStartRoom(cfg, loaded.Player.Location)
	if err != nil {
		return fmt.Errorf("session: resolve start room: %w", err)
	}

	a := &connActor{
		id:           c.ID(),
		conn:         c,
		room:         start,
		colorEnabled: cfg.ColorEnabled,
		save:         loaded.Player,
		players:      cfg.Players,
	}

	// Keep the save's location in sync with the room we actually placed
	// the player in. Mark dirty so the corrected location is flushed
	// even if the player idles and disconnects before issuing any
	// movement command (covers the saved-room-removed fallback case).
	if a.save.Location != string(start.ID) {
		a.save.Location = string(start.ID)
		a.dirty = true
	}

	cfg.Manager.Add(a)
	defer cfg.Manager.Remove(a)

	if err := a.Write(ctx, command.RenderRoom(start)); err != nil {
		return fmt.Errorf("first render: %w", err)
	}

	// Best-effort save on exit so quitting commits the player's final
	// location. Errors are logged, not returned — the connection is
	// already being torn down.
	defer func() {
		if err := a.Persist(ctx); err != nil {
			logging.From(ctx).Warn("save on disconnect failed", slog.Any("err", err))
		}
	}()

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

func resolveStartRoom(cfg Config, savedLoc string) (*world.Room, error) {
	if savedLoc != "" {
		if r, err := cfg.World.Room(world.RoomID(savedLoc)); err == nil {
			return r, nil
		}
	}
	return cfg.World.Room(cfg.StartID)
}

// connActor adapts a conn.Connection to the command.Actor interface and
// carries the player save record so SetRoom can mark it dirty.
type connActor struct {
	id   string
	conn conn.Connection

	players *player.Store

	mu           sync.Mutex
	room         *world.Room
	colorEnabled bool
	save         *player.Save
	dirty        bool
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
	if a.save != nil {
		a.save.Location = string(r.ID)
		a.dirty = true
	}
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

// Write expands any color markup in msg per the actor's color
// preference and writes the rendered text plus CRLF.
func (a *connActor) Write(ctx context.Context, msg string) error {
	rendered := ansi.Render(msg, a.ColorEnabled())
	_, err := a.conn.Write(ctx, []byte(rendered+"\r\n"))
	return err
}

// Persist writes the current player save through the store, but only
// when something has changed since the last write. Safe to call from
// any goroutine.
//
// The dirty flag is only cleared if the saved state has not been
// mutated again while we were writing — otherwise an interleaved
// SetRoom would be silently dropped: it would set dirty=true *while*
// our Save is in flight, and we'd then clear that flag after writing
// the older snapshot.
func (a *connActor) Persist(ctx context.Context) error {
	a.mu.Lock()
	if !a.dirty || a.save == nil || a.players == nil {
		a.mu.Unlock()
		return nil
	}
	snapshot := *a.save
	a.mu.Unlock()

	if err := a.players.Save(ctx, &snapshot); err != nil {
		return err
	}
	a.mu.Lock()
	// Only clear dirty if no later mutation occurred. M3 tracks just
	// Location; expand the comparison when the save grows more fields.
	if a.save != nil && a.save.Location == snapshot.Location {
		a.dirty = false
	}
	a.mu.Unlock()
	return nil
}

// PlayerName returns the loaded character's name, used by the autosave
// loop's structured logs. Returns "" for an actor that never finished
// login (shouldn't happen after Manager.Add, but defensive).
func (a *connActor) PlayerName() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return ""
	}
	return a.save.Name
}

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

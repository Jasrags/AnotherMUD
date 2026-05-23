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
	"github.com/Jasrags/AnotherMUD/internal/clock"
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

	// Clock is the time source for time-dependent session machinery
	// (flood protection refill, idle timeouts). Defaults to
	// clock.RealClock when nil so existing tests don't have to wire it.
	Clock clock.Clock

	// Flood is the per-session rate-limit policy. Zero value disables
	// flood protection (the test default). Production wires
	// DefaultFloodConfig.
	Flood FloodConfig
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

	clk := cfg.Clock
	if clk == nil {
		clk = clock.RealClock{}
	}
	a := &connActor{
		id:           c.ID(),
		conn:         c,
		playerID:     loaded.Player.ID,
		accountID:    loaded.Account.ID,
		room:         start,
		colorEnabled: cfg.ColorEnabled,
		save:         loaded.Player,
		players:      cfg.Players,
		flood:        newFloodGate(cfg.Flood, clk),
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

	// Announce arrival to the start room (excluding self) so anyone
	// already there sees the new player materialize.
	cfg.Manager.SendToRoom(ctx, start.ID,
		fmt.Sprintf("%s has arrived.", a.Name()), a.PlayerID())

	if err := a.Write(ctx, command.RenderRoom(start)); err != nil {
		return fmt.Errorf("first render: %w", err)
	}

	// Best-effort save on exit so quitting commits the player's final
	// location. Errors are logged, not returned — the connection is
	// already being torn down. Also announce departure to the room the
	// player was in at disconnect time.
	defer func() {
		room := a.Room()
		if room != nil {
			cfg.Manager.SendToRoom(ctx, room.ID,
				fmt.Sprintf("%s has left.", a.Name()), a.PlayerID())
		}
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

		// Flood gate runs before dispatch. The warn write happens after
		// the gate returns so no caller can accidentally re-enter the
		// gate from inside the Write path.
		decision, warn := a.flood.Check()
		if warn {
			_ = a.Write(ctx, "Slow down.")
		}
		switch decision {
		case floodAllow:
			// proceed
		case floodDrop:
			continue
		case floodDisconnect:
			_ = a.Write(ctx, "Disconnected: command flooding.")
			logging.From(ctx).Warn("session: disconnect on flood threshold",
				slog.String("player", a.PlayerName()))
			return nil
		}

		if err := cfg.Commands.Dispatch(ctx, cfg.World, cfg.Manager, a, line); err != nil {
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
//
// The manager back-reference is set by Manager.Add and used by SetRoom
// to keep the per-room broadcast index synchronized. Lock order is
// Manager → actor: the broadcast path (which takes actor locks via
// Write) snapshots its recipient list under the Manager lock and then
// releases before writing. SetRoom releases the actor lock before
// calling moveRoom for symmetry.
//
// playerID / accountID are immutable shadows of the save fields,
// cached at construction so the manager can read them lock-free
// during snapshot iteration. The Save record itself is only mutated
// for Location today.
type connActor struct {
	id   string
	conn conn.Connection

	playerID  string
	accountID string

	players *player.Store

	mu           sync.Mutex
	room         *world.Room
	colorEnabled bool
	save         *player.Save
	dirty        bool
	manager      *Manager
	flood        *floodGate
}

func (a *connActor) ID() string { return a.id }

func (a *connActor) Name() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.save == nil {
		return ""
	}
	return a.save.Name
}

// PlayerID is the immutable account-scoped identity of the character.
// Read lock-free from the shadow field so manager snapshots don't have
// to take the actor mutex.
func (a *connActor) PlayerID() string { return a.playerID }

func (a *connActor) Room() *world.Room {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.room
}

func (a *connActor) SetRoom(r *world.Room) {
	a.mu.Lock()
	var oldID world.RoomID
	if a.room != nil {
		oldID = a.room.ID
	}
	a.room = r
	if a.save != nil {
		a.save.Location = string(r.ID)
		a.dirty = true
	}
	mgr := a.manager
	a.mu.Unlock()

	if mgr != nil && oldID != r.ID {
		mgr.moveRoom(a, a.playerID, oldID, r.ID)
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

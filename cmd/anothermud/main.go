// Command anothermud is the MUD server entrypoint.
//
// M2 scope: configure logging, install signal-cancelled root ctx,
// load the content packs into a world, start the tick loop, open a TCP
// listener, hand it to server.Serve with the session handler.
// Replaced piece by piece as later milestones land (M3 wires
// persistence and login; M4 splits the session manager out of the
// connection layer).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/pack"
	"github.com/Jasrags/AnotherMUD/internal/server"
	"github.com/Jasrags/AnotherMUD/internal/session"
	"github.com/Jasrags/AnotherMUD/internal/tick"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// version is set via -ldflags "-X main.version=..." by the Makefile.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "anothermud: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()

	logger := newLogger(cfg)
	slog.SetDefault(logger)
	logging.Default = logger

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctx = logging.With(ctx,
		slog.String("version", version),
		slog.String("component", "server"),
	)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}

	w := world.New()
	if err := pack.Load(ctx, cfg.ContentDir, nil, w); err != nil {
		return fmt.Errorf("loading content from %s: %w", cfg.ContentDir, err)
	}
	if _, err := w.Room(cfg.StartRoom); err != nil {
		return fmt.Errorf("starting room %q not in loaded world: %w", cfg.StartRoom, err)
	}

	cmds := command.New()
	if err := command.RegisterBuiltins(cmds); err != nil {
		return fmt.Errorf("register builtins: %w", err)
	}

	loop := tick.New(clock.RealClock{}, cfg.TickInterval)
	// M1 has no real tick consumers yet; register a no-op so the loop
	// has something to demonstrate the seam. Removed when the first
	// genuine handler (mob AI, autosave, …) arrives.
	if err := loop.Register("noop", 1, func(ctx context.Context, n uint64) {}); err != nil {
		return fmt.Errorf("register noop tick: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := loop.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logging.From(ctx).Warn("tick loop exited with error", slog.Any("err", err))
		}
	}()

	logging.From(ctx).Info("anothermud starting",
		slog.String("addr", ln.Addr().String()),
		slog.String("log_format", cfg.LogFormat),
		slog.String("log_level", cfg.LogLevel),
		slog.Duration("tick_interval", cfg.TickInterval),
		slog.String("content_dir", cfg.ContentDir),
		slog.String("start_room", string(cfg.StartRoom)),
	)

	handler := session.Handler(session.Config{
		World:    w,
		Commands: cmds,
		StartID:  cfg.StartRoom,
	})
	srv := &server.Server{Handler: handler}
	if err := srv.Serve(ctx, ln); err != nil && !errors.Is(err, server.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}

	wg.Wait()
	logging.From(ctx).Info("anothermud stopped cleanly")
	return nil
}

// config is the M0 config knobs — env-only until we have more than
// ~5 of them per the ROADMAP "not front-loaded" list.
type config struct {
	Addr         string
	LogLevel     string
	LogFormat    string
	TickInterval time.Duration
	ContentDir   string
	StartRoom    world.RoomID
}

func loadConfig() config {
	return config{
		Addr:         envOr("ANOTHERMUD_ADDR", ":4000"),
		LogLevel:     strings.ToLower(envOr("ANOTHERMUD_LOG_LEVEL", "info")),
		LogFormat:    strings.ToLower(envOr("ANOTHERMUD_LOG_FORMAT", "text")),
		TickInterval: envDurationOr("ANOTHERMUD_TICK_INTERVAL", 100*time.Millisecond),
		ContentDir:   envOr("ANOTHERMUD_CONTENT_DIR", "./content"),
		StartRoom:    world.RoomID(envOr("ANOTHERMUD_START_ROOM", "tapestry-core:town-square")),
	}
}

func envDurationOr(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func newLogger(cfg config) *slog.Logger {
	var lvl slog.Level
	switch cfg.LogLevel {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lvl}

	var h slog.Handler
	if cfg.LogFormat == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h)
}

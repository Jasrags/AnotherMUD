// Command anothermud is the MUD server entrypoint.
//
// M0 scope: configure logging, install signal-cancelled root ctx,
// open a TCP listener, hand it to server.Serve with the echo handler.
// Replaced piece by piece as later milestones land (M1 wires the tick
// loop and world; M3 wires persistence and login; M4 splits the
// session manager out of the connection layer).
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
	"syscall"

	"github.com/Jasrags/AnotherMUD/internal/logging"
	"github.com/Jasrags/AnotherMUD/internal/server"
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

	logging.From(ctx).Info("anothermud starting",
		slog.String("addr", ln.Addr().String()),
		slog.String("log_format", cfg.LogFormat),
		slog.String("log_level", cfg.LogLevel),
	)

	srv := &server.Server{Handler: server.EchoHandler}
	if err := srv.Serve(ctx, ln); err != nil && !errors.Is(err, server.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}

	logging.From(ctx).Info("anothermud stopped cleanly")
	return nil
}

// config is the M0 config knobs — env-only until we have more than
// ~5 of them per the ROADMAP "not front-loaded" list.
type config struct {
	Addr      string
	LogLevel  string
	LogFormat string
}

func loadConfig() config {
	return config{
		Addr:      envOr("ANOTHERMUD_ADDR", ":4000"),
		LogLevel:  strings.ToLower(envOr("ANOTHERMUD_LOG_LEVEL", "info")),
		LogFormat: strings.ToLower(envOr("ANOTHERMUD_LOG_FORMAT", "text")),
	}
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

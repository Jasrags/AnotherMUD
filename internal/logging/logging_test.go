package logging_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/logging"
)

func TestFromReturnsDefaultWhenNoLogger(t *testing.T) {
	got := logging.From(context.Background())
	if got == nil {
		t.Fatal("From returned nil")
	}
}

func TestWithAddsAttrsToLogger(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := logging.WithLogger(context.Background(), l)
	ctx = logging.With(ctx, slog.String("session_id", "abc"))

	logging.From(ctx).Info("hi")

	out := buf.String()
	if !strings.Contains(out, "session_id=abc") {
		t.Fatalf("expected session_id=abc in log output, got: %s", out)
	}
}

func TestWithNilCtxReturnsDefault(t *testing.T) {
	got := logging.From(nil) //nolint:staticcheck // testing nil-tolerance
	if got == nil {
		t.Fatal("From(nil) returned nil")
	}
}

package login

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
)

// blockingConn is a minimal conn.Connection for idle-timeout tests. Read
// returns a queued line if one is set, otherwise it signals (once) that
// it has been entered and blocks until the context is cancelled.
type blockingConn struct {
	enterOnce sync.Once
	entered   chan struct{}

	line    string
	hasLine bool

	mu      sync.Mutex
	written strings.Builder
}

func (b *blockingConn) ID() string { return "test-conn" }

func (b *blockingConn) Read(ctx context.Context) (string, error) {
	if b.hasLine {
		return b.line, nil
	}
	if b.entered != nil {
		b.enterOnce.Do(func() { close(b.entered) })
	}
	<-ctx.Done()
	return "", ctx.Err()
}

func (b *blockingConn) Write(ctx context.Context, p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.written.Write(p)
}

func (b *blockingConn) Close() error { return nil }

func (b *blockingConn) output() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.written.String()
}

// TestReadln_IdleTimeout: a read that never receives input fires the
// idle timeout once the clock advances past the configured window, and
// readln surfaces ErrIdleTimeout (not collapsed into ErrAborted).
func TestReadln_IdleTimeout(t *testing.T) {
	mc := clock.NewManual(time.Unix(0, 0))
	entered := make(chan struct{})
	fc := &blockingConn{entered: entered}
	lio := &lineIO{c: fc, clock: mc, idle: 50 * time.Millisecond}

	type res struct {
		s   string
		err error
	}
	done := make(chan res, 1)
	go func() {
		s, err := lio.readln(context.Background())
		done <- res{s, err}
	}()

	<-entered // Read goroutine is running ⇒ the ticker is registered.
	mc.Advance(50 * time.Millisecond)

	select {
	case r := <-done:
		if !errors.Is(r.err, ErrIdleTimeout) {
			t.Fatalf("readln err = %v, want ErrIdleTimeout", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readln did not return after the idle window elapsed")
	}
}

// TestReadln_NoTimeoutWhenDisabled: idle <= 0 keeps readln a plain
// blocking read, so existing callers that set no timeout are unchanged.
func TestReadln_NoTimeoutWhenDisabled(t *testing.T) {
	fc := &blockingConn{line: "Alice", hasLine: true}
	lio := &lineIO{c: fc, idle: 0}

	got, err := lio.readln(context.Background())
	if err != nil {
		t.Fatalf("readln err = %v, want nil", err)
	}
	if got != "Alice" {
		t.Fatalf("readln = %q, want %q", got, "Alice")
	}
}

// TestRun_IdleTimeout: an end-to-end login that idles at the very first
// prompt returns ErrIdleTimeout and writes a goodbye line to the peer.
func TestRun_IdleTimeout(t *testing.T) {
	mc := clock.NewManual(time.Unix(0, 0))
	entered := make(chan struct{})
	fc := &blockingConn{entered: entered}
	cfg := Config{Clock: mc, IdleTimeout: 50 * time.Millisecond}

	type res struct {
		loaded *Loaded
		err    error
	}
	done := make(chan res, 1)
	go func() {
		l, err := Run(context.Background(), fc, cfg)
		done <- res{l, err}
	}()

	<-entered
	mc.Advance(50 * time.Millisecond)

	select {
	case r := <-done:
		if !errors.Is(r.err, ErrIdleTimeout) {
			t.Fatalf("Run err = %v, want ErrIdleTimeout", r.err)
		}
		if r.loaded != nil {
			t.Fatalf("Run loaded = %v, want nil on timeout", r.loaded)
		}
		if out := fc.output(); !strings.Contains(strings.ToLower(out), "too long") {
			t.Errorf("expected a goodbye line mentioning the timeout, got %q", out)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the idle window elapsed")
	}
}

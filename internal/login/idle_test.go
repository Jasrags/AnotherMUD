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
	lio := &lineIO{c: fc, clock: mc}

	type res struct {
		s   string
		err error
	}
	done := make(chan res, 1)
	go func() {
		s, err := lio.readln(context.Background(), 50*time.Millisecond)
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
	lio := &lineIO{c: fc}

	got, err := lio.readln(context.Background(), 0)
	if err != nil {
		t.Fatalf("readln err = %v, want nil", err)
	}
	if got != "Alice" {
		t.Fatalf("readln = %q, want %q", got, "Alice")
	}
}

// TestConfig_phaseIdle: a per-phase override with a positive value wins;
// an absent phase, a zero, or a nil map falls back to the global
// IdleTimeout (spec §6.1).
func TestConfig_phaseIdle(t *testing.T) {
	cfg := Config{
		IdleTimeout: 60 * time.Second,
		PhaseIdleTimeouts: map[Phase]time.Duration{
			PhaseName:     10 * time.Second,
			PhasePassword: 0, // non-positive → fall back to global
		},
	}
	if got := cfg.phaseIdle(PhaseName); got != 10*time.Second {
		t.Errorf("phaseIdle(Name) = %v, want 10s (the override)", got)
	}
	if got := cfg.phaseIdle(PhasePassword); got != 60*time.Second {
		t.Errorf("phaseIdle(Password) = %v, want 60s (zero override falls back to global)", got)
	}
	if got := cfg.phaseIdle(PhaseEmail); got != 60*time.Second {
		t.Errorf("phaseIdle(Email) = %v, want 60s (absent from map → global)", got)
	}

	// A nil map must not panic and always yields the global fallback.
	nilCfg := Config{IdleTimeout: 45 * time.Second}
	if got := nilCfg.phaseIdle(PhaseName); got != 45*time.Second {
		t.Errorf("phaseIdle with nil map = %v, want 45s (global)", got)
	}
}

// TestRun_PerPhaseIdleTimeout: a short per-phase override on the Name
// phase fires at the override window even though the global fallback is
// far longer — proof the Name read is bounded by its own timeout, not the
// global one (spec §6.1). The clock advances by only the override; if the
// read were using the 10s global it would not fire.
func TestRun_PerPhaseIdleTimeout(t *testing.T) {
	mc := clock.NewManual(time.Unix(0, 0))
	entered := make(chan struct{})
	fc := &blockingConn{entered: entered}
	cfg := Config{
		Clock:             mc,
		IdleTimeout:       10 * time.Second, // global fallback (long)
		PhaseIdleTimeouts: map[Phase]time.Duration{PhaseName: 50 * time.Millisecond},
	}

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
	mc.Advance(50 * time.Millisecond) // only the Name-phase override, not the global

	select {
	case r := <-done:
		if !errors.Is(r.err, ErrIdleTimeout) {
			t.Fatalf("Run err = %v, want ErrIdleTimeout at the per-phase window", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not time out at the Name-phase override window")
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

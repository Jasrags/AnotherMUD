// Package tick is the game loop and tick-handler scheduling primitive
// described in docs/specs/time-and-clock.md §4.
//
// A Loop runs in its own goroutine, advanced by a clock.Clock. Other
// features Register named handlers that fire when the tick count is a
// multiple of their interval. Handler panics are caught so one
// misbehaving handler cannot stop the simulation (§4.3).
//
// M1 scope: registration + cadence + panic isolation + ctx cancellation.
// Slow-tick observability (§5) and the in-game game-clock handler (§3)
// are deferred — they land when the first consumer needs them.
package tick

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Handler is the per-tick callback registered against a Loop.
type Handler func(ctx context.Context, tickCount uint64)

// PreTick runs once per tick before any Handler.
type PreTick func(ctx context.Context, tickCount uint64)

type registration struct {
	name     string
	interval uint64
	handler  Handler
}

// Loop drives the simulation tick clock. Construct with New, attach
// handlers via Register, then call Run in a goroutine.
//
// Run is not safe to call more than once on the same Loop.
type Loop struct {
	clock    clock.Clock
	interval time.Duration

	mu       sync.Mutex
	handlers []registration
	preTick  PreTick
	count    uint64
	started  bool

	ready chan struct{} // closed once the ticker is live; tests sync on it
}

// New builds a Loop driven by clk that ticks every interval.
func New(clk clock.Clock, interval time.Duration) *Loop {
	return &Loop{
		clock:    clk,
		interval: interval,
		ready:    make(chan struct{}),
	}
}

// Ready returns a channel that is closed once Run has registered its
// ticker against the clock. Tests block on this before advancing a
// ManualClock so the first tick is not lost to a startup race.
func (l *Loop) Ready() <-chan struct{} { return l.ready }

// Register attaches a handler. Handler names MUST be unique within a
// Loop. Returns an error if a handler with the same name is already
// registered or if interval is zero. Registration after Run has been
// called is rejected — handlers register at boot, not during play
// (§4.4).
func (l *Loop) Register(name string, intervalTicks uint64, h Handler) error {
	if intervalTicks == 0 {
		return errors.New("tick.Register: intervalTicks must be > 0")
	}
	if h == nil {
		return errors.New("tick.Register: handler is nil")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.started {
		return errors.New("tick.Register: loop already started")
	}
	for _, r := range l.handlers {
		if r.name == name {
			return fmt.Errorf("tick.Register: duplicate handler name %q", name)
		}
	}
	l.handlers = append(l.handlers, registration{name, intervalTicks, h})
	return nil
}

// SetPreTick installs the per-tick action that runs before any
// handler. Passing nil clears it. Must be called before Run.
func (l *Loop) SetPreTick(p PreTick) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.preTick = p
}

// TickCount returns the monotonic tick count. Safe for concurrent
// reads while Run is executing.
func (l *Loop) TickCount() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.count
}

// Run blocks driving the loop until ctx is cancelled. It returns
// ctx.Err() (nil if ctx had no error at exit).
func (l *Loop) Run(ctx context.Context) error {
	l.mu.Lock()
	if l.started {
		l.mu.Unlock()
		return errors.New("tick.Run: already started")
	}
	l.started = true
	handlers := append([]registration(nil), l.handlers...)
	preTick := l.preTick
	l.mu.Unlock()

	ch, stop := l.clock.Ticker(l.interval)
	defer stop()
	close(l.ready)

	log := logging.From(ctx)
	log.Info("tick loop started",
		slog.Duration("interval", l.interval),
		slog.Int("handlers", len(handlers)),
	)
	defer log.Info("tick loop stopped")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
			l.mu.Lock()
			l.count++
			n := l.count
			l.mu.Unlock()

			if preTick != nil {
				l.safeCallPre(ctx, preTick, n)
			}
			for _, r := range handlers {
				if n%r.interval == 0 {
					l.safeCall(ctx, r, n)
				}
			}
		}
	}
}

func (l *Loop) safeCall(ctx context.Context, r registration, n uint64) {
	defer func() {
		if rec := recover(); rec != nil {
			logging.From(ctx).Error("tick handler panicked",
				slog.String("handler", r.name),
				slog.Uint64("tick", n),
				slog.Any("panic", rec),
			)
		}
	}()
	r.handler(ctx, n)
}

func (l *Loop) safeCallPre(ctx context.Context, p PreTick, n uint64) {
	defer func() {
		if rec := recover(); rec != nil {
			logging.From(ctx).Error("pre-tick panicked",
				slog.Uint64("tick", n),
				slog.Any("panic", rec),
			)
		}
	}()
	p(ctx, n)
}

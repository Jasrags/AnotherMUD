package tick_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/tick"
)

func TestLoop_HandlerFiresOnCadence(t *testing.T) {
	t.Parallel()
	m := clock.NewManual(time.Unix(0, 0))
	loop := tick.New(m, 100*time.Millisecond)

	var every1, every3 atomic.Int64
	mustRegister(t, loop, "every1", 1, func(ctx context.Context, n uint64) { every1.Add(1) })
	mustRegister(t, loop, "every3", 3, func(ctx context.Context, n uint64) { every3.Add(1) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = loop.Run(ctx) }()
	<-loop.Ready()

	// Six ticks: handler "every1" fires 6 times, "every3" fires 2 times
	// (at tick 3 and tick 6). Advance one tick at a time and wait for
	// the loop to consume each before issuing the next so we don't
	// collide with the ticker channel's drop-on-full semantics.
	for i := uint64(1); i <= 6; i++ {
		m.Advance(100 * time.Millisecond)
		waitFor(t, func() bool { return loop.TickCount() >= i })
	}

	if got := every1.Load(); got != 6 {
		t.Fatalf("every1 fired %d times, want 6", got)
	}
	if got := every3.Load(); got != 2 {
		t.Fatalf("every3 fired %d times, want 2", got)
	}

	cancel()
	wg.Wait()

	if got := loop.TickCount(); got != 6 {
		t.Fatalf("tickCount = %d, want 6", got)
	}
}

func TestLoop_PreTickRunsBeforeHandlers(t *testing.T) {
	t.Parallel()
	m := clock.NewManual(time.Unix(0, 0))
	loop := tick.New(m, 10*time.Millisecond)

	var mu sync.Mutex
	var order []string

	loop.SetPreTick(func(ctx context.Context, n uint64) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "pre")
	})
	mustRegister(t, loop, "h", 1, func(ctx context.Context, n uint64) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "h")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = loop.Run(ctx) }()
	<-loop.Ready()

	m.Advance(10 * time.Millisecond)
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) >= 2
	})

	cancel()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if order[0] != "pre" || order[1] != "h" {
		t.Fatalf("expected pre before handler, got %v", order)
	}
}

func TestLoop_PanicIsIsolated(t *testing.T) {
	t.Parallel()
	m := clock.NewManual(time.Unix(0, 0))
	loop := tick.New(m, time.Millisecond)

	var sane atomic.Int64
	mustRegister(t, loop, "bomb", 1, func(ctx context.Context, n uint64) { panic("boom") })
	mustRegister(t, loop, "sane", 1, func(ctx context.Context, n uint64) { sane.Add(1) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = loop.Run(ctx) }()
	<-loop.Ready()

	for i := uint64(1); i <= 3; i++ {
		m.Advance(time.Millisecond)
		waitFor(t, func() bool { return loop.TickCount() >= i })
	}
	if got := sane.Load(); got != 3 {
		t.Fatalf("sane handler fired %d times, want 3", got)
	}

	cancel()
	wg.Wait()
}

func TestLoop_RejectsBadRegistration(t *testing.T) {
	t.Parallel()
	loop := tick.New(clock.RealClock{}, time.Second)
	noop := func(ctx context.Context, n uint64) {}

	if err := loop.Register("a", 0, noop); err == nil {
		t.Fatal("expected error on zero interval")
	}
	if err := loop.Register("a", 1, nil); err == nil {
		t.Fatal("expected error on nil handler")
	}
	if err := loop.Register("a", 1, noop); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := loop.Register("a", 1, noop); err == nil {
		t.Fatal("expected error on duplicate name")
	}
}

func mustRegister(t *testing.T, loop *tick.Loop, name string, interval uint64, h tick.Handler) {
	t.Helper()
	if err := loop.Register(name, interval, h); err != nil {
		t.Fatalf("register %q: %v", name, err)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timeout waiting for condition")
}

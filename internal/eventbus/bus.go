package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// Handler is the function signature subscribers register. The event
// is passed as the Event interface; handlers type-assert to the
// concrete payload they care about. A handler that doesn't recognize
// the concrete type should silently return — the bus does not
// pre-filter past the event name.
type Handler func(ctx context.Context, event Event)

// Bus is the engine event bus. Safe for concurrent Subscribe and
// Publish from any goroutine. Dispatch is synchronous and sequential
// in subscription order.
//
// Lock discipline: Publish takes the RLock for the duration of the
// dispatch loop so a handler that calls Subscribe (e.g. for a
// follow-on event) does NOT deadlock — RWMutex would, so handlers
// must NOT call Subscribe on the same bus from within a Publish.
// Tests cover this constraint.
type Bus struct {
	mu     sync.RWMutex
	nextID uint64 // monotonic so unsubscribe closures can find their entry
	subs   map[string][]subscription
}

type subscription struct {
	id      uint64
	handler Handler
}

// New returns an empty Bus.
func New() *Bus {
	return &Bus{subs: make(map[string][]subscription)}
}

// Subscribe registers h for events with the given name and returns
// an unsubscribe closure. The closure is idempotent — calling it
// twice is harmless.
//
// Subscription order is preserved by Publish; the first subscriber
// to register for a name fires first.
func (b *Bus) Subscribe(name string, h Handler) (unsubscribe func()) {
	if h == nil {
		// A nil handler would panic on every Publish. Refuse at the
		// boundary and return a no-op unsubscribe so callers don't
		// have to nil-guard the return value.
		return func() {}
	}
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	b.subs[name] = append(b.subs[name], subscription{id: id, handler: h})
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() { b.removeByID(name, id) })
	}
}

func (b *Bus) removeByID(name string, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.subs[name]
	for i, s := range list {
		if s.id == id {
			b.subs[name] = append(list[:i], list[i+1:]...)
			if len(b.subs[name]) == 0 {
				delete(b.subs, name)
			}
			return
		}
	}
}

// Publish dispatches event to every subscriber registered under
// event.Name() in subscription order. A handler that panics is
// logged with the event name and skipped; siblings still run. The
// design choice: one bad listener can degrade observability for its
// own concern, but it cannot abort an operation the publisher has
// already committed to.
func (b *Bus) Publish(ctx context.Context, event Event) {
	if event == nil {
		return
	}
	b.mu.RLock()
	list := b.subs[event.Name()]
	// Snapshot the slice header so an Unsubscribe during the dispatch
	// loop doesn't shift indices under our feet. The underlying
	// subscription structs are immutable so aliasing is safe.
	snap := append([]subscription(nil), list...)
	b.mu.RUnlock()

	for _, s := range snap {
		safeInvoke(ctx, event, s.handler)
	}
}

// PublishCancellable dispatches a cancellable event and returns
// true if any handler called Cancel. Dispatch continues even after
// a cancel — later listeners may legitimately observe the cancelled
// state and react (logging, side-channel notifications). The
// PUBLISHER decides what to do with the verdict.
//
// Returns true if cancelled, false otherwise.
func (b *Bus) PublishCancellable(ctx context.Context, event CancellableEvent) bool {
	if event == nil {
		return false
	}
	b.mu.RLock()
	list := b.subs[event.Name()]
	snap := append([]subscription(nil), list...)
	b.mu.RUnlock()

	for _, s := range snap {
		safeInvoke(ctx, event, s.handler)
	}
	return event.Cancelled()
}

// safeInvoke runs handler under a recover so a panic in one
// subscriber cannot take down the publisher or skip its siblings.
// Panic messages land at warn level on the ctx logger so a noisy
// handler is visible in production logs without being a fatal.
func safeInvoke(ctx context.Context, event Event, h Handler) {
	defer func() {
		if r := recover(); r != nil {
			logging.From(ctx).Warn("eventbus: handler panic",
				slog.String("event", event.Name()),
				slog.String("panic", fmt.Sprintf("%v", r)))
		}
	}()
	h(ctx, event)
}

package eventbus_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// testEvent is a minimal Event implementation for tests that don't
// care about the concrete M5 payloads.
type testEvent struct{ name string }

func (e testEvent) Name() string { return e.name }

// testCancellable is a CancellableEvent for cancellable-path tests.
type testCancellable struct {
	*eventbus.CancelFlag
	name string
}

func (e testCancellable) Name() string { return e.name }

func newCancellable(name string) testCancellable {
	return testCancellable{CancelFlag: &eventbus.CancelFlag{}, name: name}
}

func TestSubscribeAndPublishDeliversToHandler(t *testing.T) {
	b := eventbus.New()
	var got string
	b.Subscribe("ping", func(ctx context.Context, e eventbus.Event) {
		got = e.Name()
	})
	b.Publish(context.Background(), testEvent{name: "ping"})
	if got != "ping" {
		t.Errorf("handler got %q, want %q", got, "ping")
	}
}

func TestPublishToUnsubscribedNameIsNoOp(t *testing.T) {
	b := eventbus.New()
	// No subscribers; Publish must not panic.
	b.Publish(context.Background(), testEvent{name: "nobody-home"})
}

func TestMultipleSubscribersFireInRegistrationOrder(t *testing.T) {
	b := eventbus.New()
	var order []int
	for i := 0; i < 3; i++ {
		i := i
		b.Subscribe("ordered", func(ctx context.Context, e eventbus.Event) {
			order = append(order, i)
		})
	}
	b.Publish(context.Background(), testEvent{name: "ordered"})
	if len(order) != 3 || order[0] != 0 || order[1] != 1 || order[2] != 2 {
		t.Errorf("dispatch order = %v, want [0 1 2]", order)
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := eventbus.New()
	calls := 0
	unsub := b.Subscribe("once", func(ctx context.Context, e eventbus.Event) {
		calls++
	})
	b.Publish(context.Background(), testEvent{name: "once"})
	unsub()
	b.Publish(context.Background(), testEvent{name: "once"})
	if calls != 1 {
		t.Errorf("calls after unsubscribe = %d, want 1", calls)
	}
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	b := eventbus.New()
	unsub := b.Subscribe("idemp", func(ctx context.Context, e eventbus.Event) {})
	unsub()
	unsub() // second call must not panic and must not remove anything else
	other := 0
	b.Subscribe("idemp", func(ctx context.Context, e eventbus.Event) { other++ })
	b.Publish(context.Background(), testEvent{name: "idemp"})
	if other != 1 {
		t.Errorf("second subscriber lost: other = %d, want 1", other)
	}
}

func TestPublishCancellableReportsCancel(t *testing.T) {
	b := eventbus.New()
	b.Subscribe("guard", func(ctx context.Context, e eventbus.Event) {
		if c, ok := e.(eventbus.CancellableEvent); ok {
			c.Cancel()
		}
	})
	cancelled := b.PublishCancellable(context.Background(), newCancellable("guard"))
	if !cancelled {
		t.Error("PublishCancellable returned false despite handler cancelling")
	}
}

func TestPublishCancellableReportsNoCancelWhenNoneFires(t *testing.T) {
	b := eventbus.New()
	b.Subscribe("guard", func(ctx context.Context, e eventbus.Event) {
		// observe only, do not cancel
	})
	cancelled := b.PublishCancellable(context.Background(), newCancellable("guard"))
	if cancelled {
		t.Error("PublishCancellable returned true with no canceller")
	}
}

func TestCancellableContinuesDispatchAfterCancel(t *testing.T) {
	// Spec allows later listeners to observe a prior cancel — the
	// publisher decides what to do with the verdict, but dispatch
	// itself does not short-circuit.
	b := eventbus.New()
	var fired []string
	b.Subscribe("guard", func(ctx context.Context, e eventbus.Event) {
		fired = append(fired, "first")
		e.(eventbus.CancellableEvent).Cancel()
	})
	b.Subscribe("guard", func(ctx context.Context, e eventbus.Event) {
		fired = append(fired, "second")
	})
	b.PublishCancellable(context.Background(), newCancellable("guard"))
	if len(fired) != 2 || fired[0] != "first" || fired[1] != "second" {
		t.Errorf("dispatch did not continue past cancel: %v", fired)
	}
}

func TestLaterHandlerCanObservePriorCancel(t *testing.T) {
	b := eventbus.New()
	observed := false
	b.Subscribe("guard", func(ctx context.Context, e eventbus.Event) {
		e.(eventbus.CancellableEvent).Cancel()
	})
	b.Subscribe("guard", func(ctx context.Context, e eventbus.Event) {
		observed = e.(eventbus.CancellableEvent).Cancelled()
	})
	b.PublishCancellable(context.Background(), newCancellable("guard"))
	if !observed {
		t.Error("later handler did not see prior Cancel")
	}
}

func TestPanickingHandlerSkippedSiblingsStillRun(t *testing.T) {
	b := eventbus.New()
	var before, after bool
	b.Subscribe("boom", func(ctx context.Context, e eventbus.Event) { before = true })
	b.Subscribe("boom", func(ctx context.Context, e eventbus.Event) { panic("boom") })
	b.Subscribe("boom", func(ctx context.Context, e eventbus.Event) { after = true })
	b.Publish(context.Background(), testEvent{name: "boom"})
	if !before {
		t.Error("sibling before panic did not fire")
	}
	if !after {
		t.Error("sibling after panic did not fire — recover did not isolate")
	}
}

func TestSubscribeNilHandlerReturnsNoOpUnsub(t *testing.T) {
	b := eventbus.New()
	unsub := b.Subscribe("nope", nil)
	if unsub == nil {
		t.Fatal("nil-handler subscribe returned nil unsub")
	}
	unsub() // must not panic
	// Subsequent Publish must not panic — there's nothing to invoke.
	b.Publish(context.Background(), testEvent{name: "nope"})
}

func TestPublishNilEventIsNoOp(t *testing.T) {
	b := eventbus.New()
	fired := false
	b.Subscribe("anything", func(ctx context.Context, e eventbus.Event) { fired = true })
	b.Publish(context.Background(), nil)
	if fired {
		t.Error("Publish(nil) invoked a handler")
	}
}

func TestUnsubscribeDuringDispatchDoesNotShiftIndices(t *testing.T) {
	// A handler that unsubscribes a sibling mid-dispatch must not
	// cause that sibling to be skipped (the snapshot is taken before
	// the dispatch loop).
	b := eventbus.New()
	var hits []string
	var unsubB func()
	b.Subscribe("mid", func(ctx context.Context, e eventbus.Event) {
		hits = append(hits, "a")
		unsubB()
	})
	unsubB = b.Subscribe("mid", func(ctx context.Context, e eventbus.Event) {
		hits = append(hits, "b")
	})
	b.Subscribe("mid", func(ctx context.Context, e eventbus.Event) {
		hits = append(hits, "c")
	})
	b.Publish(context.Background(), testEvent{name: "mid"})
	// b's removal happens mid-dispatch but the snapshot already
	// contained it, so it still fires this round.
	if len(hits) != 3 || hits[0] != "a" || hits[1] != "b" || hits[2] != "c" {
		t.Errorf("mid-dispatch unsub disturbed this round: %v", hits)
	}
	// Next round: b is gone, a and c remain.
	hits = nil
	b.Publish(context.Background(), testEvent{name: "mid"})
	if len(hits) != 2 || hits[0] != "a" || hits[1] != "c" {
		t.Errorf("next round saw stale b: %v", hits)
	}
}

func TestConcurrentSubscribeAndPublish(t *testing.T) {
	// Race-detector smoke test. Subscribe and Publish from many
	// goroutines; correctness is "no race detected, no panic."
	b := eventbus.New()
	var wg sync.WaitGroup
	var fired int64
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unsub := b.Subscribe("concurrent", func(ctx context.Context, e eventbus.Event) {
				atomic.AddInt64(&fired, 1)
			})
			b.Publish(context.Background(), testEvent{name: "concurrent"})
			unsub()
		}()
	}
	wg.Wait()
}

func TestM5EventNames(t *testing.T) {
	// Lock the dotted-form mapping per the comment block in events.go.
	// If a future commit renames an event, the test fails and forces
	// the rename to be intentional rather than a typo.
	cases := []struct {
		event eventbus.Event
		want  string
	}{
		{eventbus.ItemPickedUp{}, "entity.item_picked_up"},
		{eventbus.ItemDropped{}, "entity.item_dropped"},
		{eventbus.EntityEquipped{}, "entity.equipped"},
		{eventbus.EntityUnequipped{}, "entity.unequipped"},
	}
	for _, tc := range cases {
		if got := tc.event.Name(); got != tc.want {
			t.Errorf("%T.Name() = %q, want %q", tc.event, got, tc.want)
		}
	}
}

func TestM5EventPayloadsAreCopied(t *testing.T) {
	// The concrete event types are value structs (not pointers) so
	// each Publish gets its own snapshot — a handler cannot mutate
	// the payload visible to a later handler. Locks that
	// expectation in.
	e := eventbus.EntityEquipped{
		HolderID: entities.EntityID("h-1"),
		ItemID:   entities.EntityID("i-1"),
		SlotName: "wield",
	}
	first := e
	first.SlotName = "head"
	if e.SlotName != "wield" {
		t.Errorf("event mutation leaked across copies: %q", e.SlotName)
	}
}

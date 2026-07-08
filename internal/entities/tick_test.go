package entities

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/tick"
)

func TestRegisterTagSwapPublishesWritesEveryTick(t *testing.T) {
	t.Parallel()

	m := clock.NewManual(time.Unix(0, 0))
	loop := tick.New(m, 10*time.Millisecond)

	s := NewStore()
	if err := RegisterTagSwap(loop, s); err != nil {
		t.Fatalf("RegisterTagSwap: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Go(func() { ; _ = loop.Run(ctx) })
	<-loop.Ready()

	tpl := &item.Template{ID: "x", Name: "n", Type: "item", Tags: []string{"weapon"}}
	if _, err := s.Spawn(tpl); err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Drive one tick. The handler runs SwapTagIndex; after it, GetByTag
	// must see the spawned entity.
	m.Advance(10 * time.Millisecond)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(s.GetByTag("weapon")) == 1 {
			cancel()
			wg.Wait()
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	wg.Wait()
	t.Fatal("GetByTag never reflected the post-Spawn write")
}

func TestRegisterTagSwapDuplicateNameFails(t *testing.T) {
	t.Parallel()
	loop := tick.New(clock.NewManual(time.Unix(0, 0)), time.Millisecond)
	s := NewStore()
	if err := RegisterTagSwap(loop, s); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := RegisterTagSwap(loop, s); err == nil {
		t.Error("second RegisterTagSwap returned nil, want duplicate-name error")
	}
}

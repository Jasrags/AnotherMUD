package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// hideActor is a named test actor with the concealer capability (testActor
// already implements IsHidden/HideScore/Hide/Reveal).
func hideActor(f *invFixture) *namedActor {
	return &namedActor{testActor: newTestActor(f.room), name: "Sneak", playerID: "p-hide"}
}

func TestHide_ConcealsAndEmitsConcealed(t *testing.T) {
	f := newInvFixture(t)
	a := hideActor(f)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityConcealed)
	env := f.env()
	env.Bus = bus

	if err := newRegistry(t).Dispatch(context.Background(), env, a, "hide"); err != nil {
		t.Fatalf("dispatch hide: %v", err)
	}
	if !a.IsHidden() {
		t.Error("actor should be hidden after `hide`")
	}
	if len(*got) != 1 {
		t.Fatalf("EntityConcealed published %d times, want 1", len(*got))
	}
	ev := (*got)[0].(eventbus.EntityConcealed)
	if ev.EntityID != "p-hide" || ev.SourceType != "hide" {
		t.Errorf("EntityConcealed = %+v, want {p-hide, hide}", ev)
	}
	if last := lastLine(a); !strings.Contains(strings.ToLower(last), "shadow") {
		t.Errorf("hide message = %q, want a concealment line", last)
	}
}

func TestHide_AlreadyHidden(t *testing.T) {
	f := newInvFixture(t)
	a := hideActor(f)
	a.Hide(10) // pre-hidden
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityConcealed)
	env := f.env()
	env.Bus = bus

	if err := newRegistry(t).Dispatch(context.Background(), env, a, "hide"); err != nil {
		t.Fatalf("dispatch hide: %v", err)
	}
	if len(*got) != 0 {
		t.Error("re-hiding while hidden must not publish EntityConcealed")
	}
	if last := lastLine(a); !strings.Contains(strings.ToLower(last), "already") {
		t.Errorf("message = %q, want an already-hidden line", last)
	}
}

func TestHide_CancelledPreEventAborts(t *testing.T) {
	f := newInvFixture(t)
	a := hideActor(f)
	bus := eventbus.New()
	// A pack-style veto: cancel concealment.before (e.g. a lit room).
	bus.Subscribe(eventbus.EventConcealmentBefore, func(ctx context.Context, ev eventbus.Event) {
		if cb, ok := ev.(*eventbus.ConcealmentBefore); ok {
			cb.Cancel()
		}
	})
	concealed := captureEvents(t, bus, eventbus.EventEntityConcealed)
	env := f.env()
	env.Bus = bus

	if err := newRegistry(t).Dispatch(context.Background(), env, a, "hide"); err != nil {
		t.Fatalf("dispatch hide: %v", err)
	}
	if a.IsHidden() {
		t.Error("a cancelled concealment.before must leave the actor unhidden")
	}
	if len(*concealed) != 0 {
		t.Error("a cancelled pre-event must not publish EntityConcealed")
	}
	if last := lastLine(a); !strings.Contains(strings.ToLower(last), "can't hide") {
		t.Errorf("message = %q, want a refusal", last)
	}
}

func TestUnhide_RevealsAndEmitsRevealed(t *testing.T) {
	f := newInvFixture(t)
	a := hideActor(f)
	a.Hide(10)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityRevealed)
	env := f.env()
	env.Bus = bus

	if err := newRegistry(t).Dispatch(context.Background(), env, a, "unhide"); err != nil {
		t.Fatalf("dispatch unhide: %v", err)
	}
	if a.IsHidden() {
		t.Error("actor should not be hidden after `unhide`")
	}
	if len(*got) != 1 {
		t.Fatalf("EntityRevealed published %d times, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.EntityRevealed); ev.Reason != "emerged" || ev.EntityID != "p-hide" {
		t.Errorf("EntityRevealed = %+v, want reason=emerged entity=p-hide", ev)
	}
}

func TestUnhide_NotHidden(t *testing.T) {
	f := newInvFixture(t)
	a := hideActor(f)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityRevealed)
	env := f.env()
	env.Bus = bus

	if err := newRegistry(t).Dispatch(context.Background(), env, a, "unhide"); err != nil {
		t.Fatalf("dispatch unhide: %v", err)
	}
	if len(*got) != 0 {
		t.Error("unhide while not hidden must not publish EntityRevealed")
	}
	if last := lastLine(a); !strings.Contains(strings.ToLower(last), "aren't hidden") {
		t.Errorf("message = %q, want a not-hidden line", last)
	}
}

// lastLine returns the actor's most recent output line (trimmed), or "".
func lastLine(a *namedActor) string {
	return strings.TrimSpace(a.testActor.lastLine())
}

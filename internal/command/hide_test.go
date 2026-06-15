package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
)

// breaksRegistry builds a registry with one no-op command whose
// BreaksConcealment flag is `breaks`, for reveal-on-action tests.
func breaksRegistry(t *testing.T, breaks bool) *command.Registry {
	t.Helper()
	r := command.New()
	if err := r.RegisterCommand(command.Command{
		Keyword:           "act",
		Handler:           func(ctx context.Context, c *command.Context) error { return nil },
		BreaksConcealment: breaks,
	}); err != nil {
		t.Fatalf("RegisterCommand: %v", err)
	}
	return r
}

// A breaks_concealment command reveals a hidden actor before its handler
// runs and emits entity.revealed(acted) (visibility §4.5).
func TestDispatch_BreaksConcealmentRevealsHidden(t *testing.T) {
	f := newInvFixture(t)
	a := hideActor(f)
	a.Hide(10)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityRevealed)
	env := f.env()
	env.Bus = bus

	if err := breaksRegistry(t, true).Dispatch(context.Background(), env, a, "act"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if a.IsHidden() {
		t.Error("a breaks_concealment command must reveal a hidden actor")
	}
	if len(*got) != 1 {
		t.Fatalf("EntityRevealed published %d times, want 1", len(*got))
	}
	if ev := (*got)[0].(eventbus.EntityRevealed); ev.Reason != "acted" || ev.EntityID != "p-hide" {
		t.Errorf("EntityRevealed = %+v, want reason=acted entity=p-hide", ev)
	}
}

// A non-breaks command leaves a hidden actor hidden.
func TestDispatch_QuietCommandKeepsConcealment(t *testing.T) {
	f := newInvFixture(t)
	a := hideActor(f)
	a.Hide(10)
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityRevealed)
	env := f.env()
	env.Bus = bus

	if err := breaksRegistry(t, false).Dispatch(context.Background(), env, a, "act"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !a.IsHidden() {
		t.Error("a quiet command must not reveal a hidden actor")
	}
	if len(*got) != 0 {
		t.Error("a quiet command must not emit entity.revealed")
	}
}

// A breaks_concealment command run by an actor who isn't hidden is a no-op
// (no event, no error).
func TestDispatch_BreaksConcealmentNoopWhenNotHidden(t *testing.T) {
	f := newInvFixture(t)
	a := hideActor(f) // not hidden
	bus := eventbus.New()
	got := captureEvents(t, bus, eventbus.EventEntityRevealed)
	env := f.env()
	env.Bus = bus

	if err := breaksRegistry(t, true).Dispatch(context.Background(), env, a, "act"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(*got) != 0 {
		t.Error("a breaks_concealment command must not emit revealed when the actor isn't hidden")
	}
}

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

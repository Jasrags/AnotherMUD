package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/eventbus"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// recallActor wraps a namedActor with the recallController surface
// so the verb path resolves and exercises read/write of the bound
// recall id without needing a real session.connActor.
type recallActor struct {
	*namedActor
	recall string
}

func newRecallActor(name, playerID string, room *world.Room) *recallActor {
	return &recallActor{
		namedActor: &namedActor{
			testActor: newTestActor(room),
			name:      name,
			playerID:  playerID,
		},
	}
}

func (r *recallActor) Recall() string          { return r.recall }
func (r *recallActor) SetRecall(roomID string) { r.recall = roomID }

// twoRoomWorld returns a tiny world with rooms "home" and "field"
// (no exits — recall teleports, it doesn't walk) for the recall
// verb tests.
func twoRoomWorld(t *testing.T) (*world.World, *world.Room, *world.Room) {
	t.Helper()
	w := world.New()
	home := &world.Room{ID: "home", Name: "Home"}
	field := &world.Room{ID: "field", Name: "Field"}
	w.AddRoom(home)
	w.AddRoom(field)
	return w, home, field
}

func TestSetRecall_BindsCurrentRoomAndPublishes(t *testing.T) {
	w, home, _ := twoRoomWorld(t)
	bus := eventbus.New()
	set := captureEvents(t, bus, eventbus.EventRecallSet)

	a := newRecallActor("Alice", "p-1", home)
	r := newRegistry(t)
	env := command.Env{World: w, Bus: bus}

	if err := r.Dispatch(context.Background(), env, a, "recall set"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if a.recall != "home" {
		t.Errorf("recall = %q, want %q", a.recall, "home")
	}
	if got := a.lastLine(); !strings.Contains(got, "bind your recall") {
		t.Errorf("confirmation = %q", got)
	}
	if len(*set) != 1 {
		t.Fatalf("recall.set count = %d, want 1", len(*set))
	}
	ev := (*set)[0].(eventbus.RecallSet)
	if ev.RoomID != home.ID || ev.PlayerID != "p-1" {
		t.Errorf("recall.set payload = %+v", ev)
	}
}

func TestSetRecall_Idempotent(t *testing.T) {
	w, home, _ := twoRoomWorld(t)
	bus := eventbus.New()
	set := captureEvents(t, bus, eventbus.EventRecallSet)
	a := newRecallActor("Alice", "p-1", home)
	a.recall = "home"
	r := newRegistry(t)
	env := command.Env{World: w, Bus: bus}

	if err := r.Dispatch(context.Background(), env, a, "recall set"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "already bound") {
		t.Errorf("expected already-bound line, got %q", got)
	}
	if len(*set) != 0 {
		t.Errorf("idempotent re-bind should not publish, got %d events", len(*set))
	}
}

func TestRecall_NoPoint_EmitsMessageAndEvent(t *testing.T) {
	w, home, _ := twoRoomWorld(t)
	bus := eventbus.New()
	noPoint := captureEvents(t, bus, eventbus.EventRecallNoPoint)
	a := newRecallActor("Alice", "p-1", home)
	r := newRegistry(t)
	env := command.Env{World: w, Bus: bus}

	if err := r.Dispatch(context.Background(), env, a, "recall"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "no recall point") {
		t.Errorf("message = %q", got)
	}
	if len(*noPoint) != 1 {
		t.Errorf("recall.no_point count = %d, want 1", len(*noPoint))
	}
	if a.room.ID != home.ID {
		t.Errorf("actor moved despite no recall: room=%q", a.room.ID)
	}
}

func TestRecall_Unresolved_EmitsMessageAndEvent(t *testing.T) {
	w, home, _ := twoRoomWorld(t)
	bus := eventbus.New()
	unresolved := captureEvents(t, bus, eventbus.EventRecallUnresolved)
	a := newRecallActor("Alice", "p-1", home)
	a.recall = "nowhere"
	r := newRegistry(t)
	env := command.Env{World: w, Bus: bus}

	if err := r.Dispatch(context.Background(), env, a, "recall"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "no longer there") {
		t.Errorf("message = %q", got)
	}
	if len(*unresolved) != 1 {
		t.Fatalf("recall.unresolved count = %d, want 1", len(*unresolved))
	}
	ev := (*unresolved)[0].(eventbus.RecallUnresolved)
	if ev.MissingRoom != "nowhere" {
		t.Errorf("MissingRoom = %q", ev.MissingRoom)
	}
	if a.room.ID != home.ID {
		t.Errorf("actor moved despite unresolved recall")
	}
}

func TestRecall_SamePoint_NoOpNoEvents(t *testing.T) {
	w, home, _ := twoRoomWorld(t)
	bus := eventbus.New()
	before := captureEvents(t, bus, eventbus.EventRecallBefore)
	after := captureEvents(t, bus, eventbus.EventRecallAfter)
	a := newRecallActor("Alice", "p-1", home)
	a.recall = "home"
	r := newRegistry(t)
	env := command.Env{World: w, Bus: bus}

	if err := r.Dispatch(context.Background(), env, a, "recall"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := a.lastLine(); !strings.Contains(got, "already at your recall") {
		t.Errorf("message = %q", got)
	}
	if len(*before) != 0 || len(*after) != 0 {
		t.Errorf("same-point no-op should not publish events: before=%d after=%d",
			len(*before), len(*after))
	}
}

func TestRecall_Teleports_BroadcastsAndPublishesAfter(t *testing.T) {
	w, home, field := twoRoomWorld(t)
	bus := eventbus.New()
	before := captureEvents(t, bus, eventbus.EventRecallBefore)
	after := captureEvents(t, bus, eventbus.EventRecallAfter)
	a := newRecallActor("Alice", "p-1", field)
	a.recall = "home"
	rec := &recordingBroadcaster{}
	r := newRegistry(t)
	env := command.Env{World: w, Bus: bus, Broadcaster: rec}

	if err := r.Dispatch(context.Background(), env, a, "recall"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if a.room.ID != home.ID {
		t.Fatalf("after recall: room = %q, want %q", a.room.ID, home.ID)
	}
	if len(*before) != 1 {
		t.Errorf("recall.before count = %d, want 1", len(*before))
	}
	if len(*after) != 1 {
		t.Errorf("recall.after count = %d, want 1", len(*after))
	}
	// Two broadcasts: vanish in source, appear in destination.
	if len(rec.calls) != 2 {
		t.Fatalf("broadcast count = %d, want 2", len(rec.calls))
	}
	if rec.calls[0].roomID != field.ID || !strings.Contains(rec.calls[0].text, "vanishes") {
		t.Errorf("source broadcast = %+v", rec.calls[0])
	}
	if rec.calls[1].roomID != home.ID || !strings.Contains(rec.calls[1].text, "appears") {
		t.Errorf("dest broadcast = %+v", rec.calls[1])
	}
}

func TestRecall_Cancelled_StaysPutAndNoAfter(t *testing.T) {
	w, _, field := twoRoomWorld(t)
	bus := eventbus.New()
	bus.Subscribe(eventbus.EventRecallBefore, func(_ context.Context, e eventbus.Event) {
		if pre, ok := e.(*eventbus.RecallBefore); ok {
			pre.Cancel()
		}
	})
	after := captureEvents(t, bus, eventbus.EventRecallAfter)
	a := newRecallActor("Alice", "p-1", field)
	a.recall = "home"
	rec := &recordingBroadcaster{}
	r := newRegistry(t)
	env := command.Env{World: w, Bus: bus, Broadcaster: rec}

	if err := r.Dispatch(context.Background(), env, a, "recall"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if a.room.ID != field.ID {
		t.Errorf("actor moved despite cancel: %q", a.room.ID)
	}
	if got := a.lastLine(); !strings.Contains(got, "can't recall") {
		t.Errorf("cancel message = %q", got)
	}
	if len(*after) != 0 {
		t.Errorf("recall.after should not fire when cancelled")
	}
	if len(rec.calls) != 0 {
		t.Errorf("broadcasts should not fire on cancel: %+v", rec.calls)
	}
}

// `recall foo` (a non-`set` trailing token) is treated as a bare recall,
// not a binding — here with no recall point set, so it reports the
// no-point path rather than binding anything.
func TestRecall_NonSetTokenIsBareRecall(t *testing.T) {
	w, home, _ := twoRoomWorld(t)
	a := newRecallActor("Alice", "p-1", home)
	r := newRegistry(t)
	env := command.Env{World: w}

	if err := r.Dispatch(context.Background(), env, a, "recall wibble"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if a.recall != "" {
		t.Errorf("`recall wibble` should not bind, recall = %q", a.recall)
	}
	if got := a.lastLine(); !strings.Contains(got, "no recall point") {
		t.Errorf("message = %q, want the no-point path", got)
	}
}

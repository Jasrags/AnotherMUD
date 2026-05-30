package command_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/emote"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// roomBroadcaster captures SendToRoom calls so emote tests can assert
// the room-view line and the exclusion set without needing a full
// session.Manager.
type roomBroadcaster struct {
	mu     sync.Mutex
	calls  []broadcastCall
}

type broadcastCall struct {
	RoomID   world.RoomID
	Text     string
	Excludes []string
}

func (b *roomBroadcaster) SendToRoom(_ context.Context, roomID world.RoomID, text string, exclude ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(exclude))
	copy(out, exclude)
	b.calls = append(b.calls, broadcastCall{RoomID: roomID, Text: text, Excludes: out})
}

func (b *roomBroadcaster) last() broadcastCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.calls) == 0 {
		return broadcastCall{}
	}
	return b.calls[len(b.calls)-1]
}

// emoteLocator returns a configured actor when its name matches.
type emoteLocator struct{ byName map[string]command.Actor }

func (s emoteLocator) FindInRoom(_ world.RoomID, name string) command.Actor {
	a, ok := s.byName[strings.ToLower(name)]
	if !ok {
		return nil
	}
	return a
}

func smileEmote() *emote.Emote {
	e := emote.Emote{
		ID:          "x:smile",
		DisplayName: "smile",
		NoTarget: emote.View{
			ActorView: "You smile.",
			RoomView:  "$n smiles.",
		},
		Targeted: emote.View{
			ActorView:  "You smile at $N.",
			TargetView: "$n smiles at you.",
			RoomView:   "$n smiles at $N.",
		},
	}
	return &e
}

func emoteEnv(b *roomBroadcaster, locator command.Locator) command.Env {
	return command.Env{
		Broadcaster: b,
		Locator:     locator,
	}
}

func TestEmote_NoTargetActorAndRoomViews(t *testing.T) {
	room := &world.Room{ID: "x:1"}
	alice := newNamedTestActor("Alice", "p-alice", room)
	b := &roomBroadcaster{}

	r := newRegistry(t)
	if err := r.Register("smile", command.MakeEmoteHandler(smileEmote())); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Dispatch(context.Background(), emoteEnv(b, nil), alice, "smile"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); got != "You smile." {
		t.Errorf("actor view = %q", got)
	}
	if last := b.last(); last.Text != "Alice smiles." {
		t.Errorf("room view = %q", last.Text)
	}
	if last := b.last(); len(last.Excludes) != 1 || last.Excludes[0] != "p-alice" {
		t.Errorf("excludes = %v, want [p-alice]", last.Excludes)
	}
}

func TestEmote_TargetedPlayerThreeViews(t *testing.T) {
	room := &world.Room{ID: "x:1"}
	alice := newNamedTestActor("Alice", "p-alice", room)
	bob := newNamedTestActor("Bob", "p-bob", room)
	b := &roomBroadcaster{}
	locator := emoteLocator{byName: map[string]command.Actor{"bob": bob}}

	r := newRegistry(t)
	if err := r.Register("smile", command.MakeEmoteHandler(smileEmote())); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Dispatch(context.Background(), emoteEnv(b, locator), alice, "smile Bob"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); got != "You smile at Bob." {
		t.Errorf("actor view = %q", got)
	}
	if got := bob.lastLine(); got != "Alice smiles at you." {
		t.Errorf("target view = %q", got)
	}
	last := b.last()
	if last.Text != "Alice smiles at Bob." {
		t.Errorf("room view = %q", last.Text)
	}
	// Excludes contain both actor and target.
	wantSet := map[string]bool{"p-alice": true, "p-bob": true}
	if len(last.Excludes) != 2 {
		t.Errorf("excludes count = %d, want 2", len(last.Excludes))
	}
	for _, e := range last.Excludes {
		if !wantSet[e] {
			t.Errorf("unexpected exclude %q", e)
		}
	}
}

func TestEmote_SelfTargetSkipsSeparateTargetWrite(t *testing.T) {
	room := &world.Room{ID: "x:1"}
	alice := newNamedTestActor("Alice", "p-alice", room)
	b := &roomBroadcaster{}

	r := newRegistry(t)
	if err := r.Register("smile", command.MakeEmoteHandler(smileEmote())); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Dispatch(context.Background(), emoteEnv(b, nil), alice, "smile Alice"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	// Only one Write to Alice (the actor view) — no duplicate from
	// the target-view branch.
	if got := alice.lastLine(); got != "You smile at Alice." {
		t.Errorf("actor view = %q", got)
	}
	// Room view excludes only actor (target == actor).
	last := b.last()
	if len(last.Excludes) != 1 || last.Excludes[0] != "p-alice" {
		t.Errorf("excludes = %v, want [p-alice] only", last.Excludes)
	}
}

func TestEmote_TargetNotPresent(t *testing.T) {
	room := &world.Room{ID: "x:1"}
	alice := newNamedTestActor("Alice", "p-alice", room)
	b := &roomBroadcaster{}

	r := newRegistry(t)
	if err := r.Register("smile", command.MakeEmoteHandler(smileEmote())); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Dispatch(context.Background(), emoteEnv(b, emoteLocator{byName: map[string]command.Actor{}}), alice, "smile Ghost"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "don't see") {
		t.Errorf("missing target = %q", got)
	}
	if len(b.calls) != 0 {
		t.Errorf("broadcast fired despite missing target: %v", b.calls)
	}
}

func TestEmote_RequiresTargetRejectsBareForm(t *testing.T) {
	room := &world.Room{ID: "x:1"}
	alice := newNamedTestActor("Alice", "p-alice", room)
	b := &roomBroadcaster{}

	e := smileEmote()
	e.RequiresTarget = true
	e.NoTarget = emote.View{} // never used

	r := newRegistry(t)
	if err := r.Register("smile", command.MakeEmoteHandler(e)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Dispatch(context.Background(), emoteEnv(b, nil), alice, "smile"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "smile whom") {
		t.Errorf("requires-target bare = %q", got)
	}
}

func TestEmoteFreeform_BroadcastsAndEchoes(t *testing.T) {
	room := &world.Room{ID: "x:1"}
	alice := newNamedTestActor("Alice", "p-alice", room)
	b := &roomBroadcaster{}

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), emoteEnv(b, nil), alice, "emote claps slowly"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); got != "Alice claps slowly" {
		t.Errorf("actor freeform = %q", got)
	}
	if last := b.last(); last.Text != "Alice claps slowly" {
		t.Errorf("room freeform = %q", last.Text)
	}
}

func TestEmoteFreeform_NoArgPrompt(t *testing.T) {
	room := &world.Room{ID: "x:1"}
	alice := newNamedTestActor("Alice", "p-alice", room)
	b := &roomBroadcaster{}

	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), emoteEnv(b, nil), alice, "emote"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "Emote what") {
		t.Errorf("freeform no-arg = %q", got)
	}
}

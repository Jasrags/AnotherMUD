package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/chat"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
)

// stubSubscribers returns a fixed online-subscriber map regardless of
// channel id. v1 substrate: every online player is auto-tuned in.
type stubSubscribers struct {
	subs map[string]string
}

func (s stubSubscribers) Subscribers(_ string) map[string]string {
	out := make(map[string]string, len(s.subs))
	for k, v := range s.subs {
		out[k] = v
	}
	return out
}

// stubScrollbacks looks up the ring buffer for a channel id.
type stubScrollbacks struct {
	m map[string]*chat.Scrollback
}

func (s stubScrollbacks) Scrollback(id string) *chat.Scrollback {
	return s.m[id]
}

// chatFixture wires the smallest channel environment: a registry
// with one ooc channel, an in-memory scrollback, a notifications
// manager, and a per-channel verb already registered.
type chatFixture struct {
	reg       *chat.Registry
	scroll    *chat.Scrollback
	scrolls   map[string]*chat.Scrollback
	notifMgr  *notifications.Manager
	subs      *stubSubscribers
	registry  *command.Registry
	channelID string
	channelV  string
}

func newChatFixture(t *testing.T) *chatFixture {
	t.Helper()
	reg := chat.NewRegistry()
	ooc := chat.Channel{
		ID: "tapestry-core:ooc", DisplayName: "ooc",
		Kind: chat.KindPublic, DefaultOn: true, Persisted: true, BufferCap: 10,
	}
	if err := reg.Register(ooc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	sb := chat.NewScrollback(ooc.BufferCap)
	scrolls := map[string]*chat.Scrollback{ooc.ID: sb}

	store := notifications.NewStore(t.TempDir(), 50)
	mgr := notifications.NewManager(store, 50, clock.RealClock{})

	r := newRegistry(t)
	ch, _ := reg.Get(ooc.ID)
	if err := r.Register("ooc", command.MakeChannelHandler(ch)); err != nil {
		t.Fatalf("Register ooc handler: %v", err)
	}

	return &chatFixture{
		reg:       reg,
		scroll:    sb,
		scrolls:   scrolls,
		notifMgr:  mgr,
		subs:      &stubSubscribers{subs: map[string]string{}},
		registry:  r,
		channelID: ooc.ID,
		channelV:  "ooc",
	}
}

func (f *chatFixture) env() command.Env {
	return command.Env{
		Notifications:   f.notifMgr,
		ChatRegistry:    f.reg,
		ChatSubscribers: f.subs,
		ChatScrollbacks: stubScrollbacks{m: f.scrolls},
	}
}

func (f *chatFixture) registerActor(t *testing.T, a *tellsActor) {
	t.Helper()
	sink := &writerSink{a: a.testActor, ta: a}
	if err := f.notifMgr.Register(context.Background(), a.PlayerID(), a.Name(), sink); err != nil {
		t.Fatalf("notif Register: %v", err)
	}
	f.subs.subs[a.PlayerID()] = strings.ToLower(a.Name())
	t.Cleanup(func() {
		_ = f.notifMgr.Unregister(context.Background(), a.PlayerID())
	})
}

func TestChannel_OocBroadcasts(t *testing.T) {
	f := newChatFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	bob := newTellsActor("Bob", "p-bob")
	f.registerActor(t, alice)
	f.registerActor(t, bob)

	if err := f.registry.Dispatch(context.Background(), f.env(), alice, "ooc hi everyone"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	// Alice sees the "You ooc: ..." confirmation, NOT the broadcast.
	if got := alice.lastLine(); !strings.Contains(got, "You ooc: hi everyone") {
		t.Errorf("alice confirmation = %q", got)
	}
	// Bob sees the rendered broadcast line.
	if got := bob.lastLine(); !strings.Contains(got, "[ooc] Alice: hi everyone") {
		t.Errorf("bob broadcast = %q", got)
	}

	// Scrollback recorded one message.
	if f.scroll.Len() != 1 {
		t.Errorf("scrollback len = %d, want 1", f.scroll.Len())
	}
}

func TestChannel_NoSubscribersStillEchoesAndArchives(t *testing.T) {
	f := newChatFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	// Don't register Alice with the subscribers map (simulates her
	// being the only one online and self-echo-only).
	f.subs.subs[alice.PlayerID()] = strings.ToLower(alice.Name())
	// No notifications register either: confirmation goes through
	// the Actor.Write path, not the substrate, so no Register is
	// required for the echo to work.

	if err := f.registry.Dispatch(context.Background(), f.env(), alice, "ooc anyone?"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "You ooc: anyone?") {
		t.Errorf("solo confirmation = %q", got)
	}
	if f.scroll.Len() != 1 {
		t.Errorf("scrollback len = %d, want 1 (record even with no recipients)", f.scroll.Len())
	}
}

func TestChannel_NoArgPrompt(t *testing.T) {
	f := newChatFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	f.registerActor(t, alice)

	if err := f.registry.Dispatch(context.Background(), f.env(), alice, "ooc"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "ooc what?") {
		t.Errorf("no-arg ooc = %q", got)
	}
}

func TestChatList_RendersRegistry(t *testing.T) {
	f := newChatFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	if err := f.registry.Dispatch(context.Background(), f.env(), alice, "channels"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got := alice.lastLine()
	if !strings.Contains(got, "Available channels:") || !strings.Contains(got, "ooc") {
		t.Errorf("channels output = %q", got)
	}
}

func TestChatHistory_RendersScrollback(t *testing.T) {
	f := newChatFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	f.registerActor(t, alice)

	// Publish a couple of lines.
	_ = f.registry.Dispatch(context.Background(), f.env(), alice, "ooc first")
	_ = f.registry.Dispatch(context.Background(), f.env(), alice, "ooc second")

	if err := f.registry.Dispatch(context.Background(), f.env(), alice, "chathistory ooc"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got := alice.lastLine()
	if !strings.Contains(got, "Recent on ooc:") ||
		!strings.Contains(got, "[ooc] Alice: first") ||
		!strings.Contains(got, "[ooc] Alice: second") {
		t.Errorf("chathistory output = %q", got)
	}
}

func TestChatHistory_NoArgPrompt(t *testing.T) {
	f := newChatFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	if err := f.registry.Dispatch(context.Background(), f.env(), alice, "chathistory"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "which channel") {
		t.Errorf("no-arg history = %q", got)
	}
}

func TestChatHistory_UnknownChannel(t *testing.T) {
	f := newChatFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	if err := f.registry.Dispatch(context.Background(), f.env(), alice, "chathistory bogus"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "No channel named") {
		t.Errorf("unknown history = %q", got)
	}
}

func TestChatHistory_EmptyChannel(t *testing.T) {
	f := newChatFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	if err := f.registry.Dispatch(context.Background(), f.env(), alice, "chathistory ooc"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "No history for ooc") {
		t.Errorf("empty history = %q", got)
	}
}

func TestChannel_DisabledWhenEnvNotWired(t *testing.T) {
	alice := newTellsActor("Alice", "p-alice")
	r := newRegistry(t)
	ch := chat.Channel{ID: "x:y", DisplayName: "y"}
	if err := r.Register("y", command.MakeChannelHandler(&ch)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Dispatch(context.Background(), command.Env{}, alice, "y hello"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "not enabled") {
		t.Errorf("unwired = %q", got)
	}
}

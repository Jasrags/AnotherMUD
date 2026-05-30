package command_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/notifications"
)

// tellsActor wraps testActor with TellsBuffer state so the tell /
// reply / tells handlers can exercise the in-memory reply slot and
// recent-tells ring. Mirrors the production connActor behavior
// without dragging the session package into command tests.
type tellsActor struct {
	*testActor
	bmu     sync.Mutex
	partner string
	recent  []string
}

func newTellsActor(name, pid string) *tellsActor {
	return &tellsActor{testActor: newNamedTestActor(name, pid, nil)}
}

func (a *tellsActor) LastTellPartner() string {
	a.bmu.Lock()
	defer a.bmu.Unlock()
	return a.partner
}

func (a *tellsActor) SetLastTellPartner(name string) {
	if name == "" {
		return
	}
	a.bmu.Lock()
	defer a.bmu.Unlock()
	a.partner = name
}

func (a *tellsActor) RecentTells() []string {
	a.bmu.Lock()
	defer a.bmu.Unlock()
	if len(a.recent) == 0 {
		return nil
	}
	out := make([]string, len(a.recent))
	copy(out, a.recent)
	return out
}

func (a *tellsActor) AppendRecentTell(line string) {
	if line == "" {
		return
	}
	a.bmu.Lock()
	defer a.bmu.Unlock()
	a.recent = append(a.recent, line)
}

// stubResolver implements command.TellResolver for tests.
type stubResolver struct {
	online  map[string]*tellsActor
	offline map[string][2]string // name → [id, canonical]
}

func (r *stubResolver) ResolveOnline(name string) (command.Actor, bool) {
	a, ok := r.online[strings.ToLower(name)]
	if !ok {
		return nil, false
	}
	return a, true
}

func (r *stubResolver) ResolveOffline(_ context.Context, name string) (string, string, bool) {
	off, ok := r.offline[strings.ToLower(name)]
	if !ok {
		return "", "", false
	}
	return off[0], off[1], true
}

func newTellFixture(t *testing.T) (*notifications.Manager, *stubResolver) {
	t.Helper()
	store := notifications.NewStore(t.TempDir(), 50)
	mgr := notifications.NewManager(store, 50, clock.RealClock{})
	return mgr, &stubResolver{
		online:  make(map[string]*tellsActor),
		offline: make(map[string][2]string),
	}
}

func dispatchTell(t *testing.T, sender *tellsActor, mgr *notifications.Manager, res *stubResolver, line string) {
	t.Helper()
	r := newRegistry(t)
	env := command.Env{Notifications: mgr, TellResolver: res}
	if err := r.Dispatch(context.Background(), env, sender, line); err != nil {
		t.Fatalf("dispatch %q: %v", line, err)
	}
}

func TestTell_NoArgsPrompt(t *testing.T) {
	mgr, res := newTellFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	dispatchTell(t, alice, mgr, res, "tell")
	if got := alice.lastLine(); !strings.Contains(got, "Tell whom") {
		t.Errorf("no-arg tell = %q, want prompt", got)
	}
}

func TestTell_OnlineRecipientImmediateDelivery(t *testing.T) {
	mgr, res := newTellFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	bob := newTellsActor("Bob", "p-bob")
	res.online["bob"] = bob

	if err := mgr.Register(context.Background(), bob.PlayerID(), bob.Name(), &writerSink{a: bob.testActor, ta: bob}); err != nil {
		t.Fatalf("Register Bob: %v", err)
	}
	defer mgr.Unregister(context.Background(), bob.PlayerID())

	dispatchTell(t, alice, mgr, res, "tell Bob hi there")

	if got := alice.lastLine(); !strings.Contains(got, "You tell Bob: hi there") {
		t.Errorf("alice confirmation = %q", got)
	}
	if got := bob.lastLine(); !strings.Contains(got, "Alice tells you: hi there") {
		t.Errorf("bob delivery = %q", got)
	}
	if bob.LastTellPartner() != "Alice" {
		t.Errorf("bob LastTellPartner = %q, want Alice", bob.LastTellPartner())
	}
	if alice.LastTellPartner() != "Bob" {
		t.Errorf("alice LastTellPartner = %q, want Bob", alice.LastTellPartner())
	}
}

func TestTell_NoSuchPlayer(t *testing.T) {
	mgr, res := newTellFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	dispatchTell(t, alice, mgr, res, "tell Ghost hi")
	if got := alice.lastLine(); !strings.Contains(got, "No player named") {
		t.Errorf("no-such = %q", got)
	}
}

func TestTell_OfflineKnownEnqueuesThroughStore(t *testing.T) {
	mgr, res := newTellFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	res.offline["bob"] = [2]string{"p-bob", "bob"}

	dispatchTell(t, alice, mgr, res, "tell Bob i'll be back")
	if got := alice.lastLine(); !strings.Contains(got, "You tell Bob") {
		t.Errorf("alice confirmation = %q", got)
	}

	bob := newTellsActor("Bob", "p-bob")
	sink := &writerSink{a: bob.testActor, ta: bob}
	if err := mgr.Register(context.Background(), bob.PlayerID(), "bob", sink); err != nil {
		t.Fatalf("Register bob: %v", err)
	}
	if err := mgr.Drain(context.Background(), bob.PlayerID()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if got := bob.lastLine(); !strings.Contains(got, "Alice tells you: i'll be back") {
		t.Errorf("bob post-drain = %q", got)
	}
}

func TestReply_UsesLastTellPartner(t *testing.T) {
	mgr, res := newTellFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	bob := newTellsActor("Bob", "p-bob")
	res.online["bob"] = bob
	res.online["alice"] = alice
	if err := mgr.Register(context.Background(), bob.PlayerID(), bob.Name(), &writerSink{a: bob.testActor, ta: bob}); err != nil {
		t.Fatalf("Register Bob: %v", err)
	}
	defer mgr.Unregister(context.Background(), bob.PlayerID())
	if err := mgr.Register(context.Background(), alice.PlayerID(), alice.Name(), &writerSink{a: alice.testActor, ta: alice}); err != nil {
		t.Fatalf("Register Alice: %v", err)
	}
	defer mgr.Unregister(context.Background(), alice.PlayerID())

	dispatchTell(t, alice, mgr, res, "tell Bob hello")
	dispatchTell(t, bob, mgr, res, "reply hey alice")

	if got := alice.lastLine(); !strings.Contains(got, "Bob tells you: hey alice") {
		t.Errorf("alice received reply = %q", got)
	}
}

func TestReply_NoSlotIsClearError(t *testing.T) {
	mgr, res := newTellFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	dispatchTell(t, alice, mgr, res, "reply hi")
	if got := alice.lastLine(); !strings.Contains(got, "no one to reply") {
		t.Errorf("no-slot reply = %q", got)
	}
}

func TestTells_ShowsRecentHistory(t *testing.T) {
	mgr, res := newTellFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	alice.AppendRecentTell("Bob tells you: 1")
	alice.AppendRecentTell("Cara tells you: 2")

	dispatchTell(t, alice, mgr, res, "tells")
	got := alice.lastLine()
	if !strings.Contains(got, "Recent tells:") ||
		!strings.Contains(got, "Bob tells you: 1") ||
		!strings.Contains(got, "Cara tells you: 2") {
		t.Errorf("tells output = %q", got)
	}
}

func TestTells_EmptyRing(t *testing.T) {
	mgr, res := newTellFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	dispatchTell(t, alice, mgr, res, "tells")
	if got := alice.lastLine(); !strings.Contains(got, "No recent tells") {
		t.Errorf("empty tells = %q", got)
	}
}

func TestTell_NotificationsDisabledFallsThrough(t *testing.T) {
	alice := newTellsActor("Alice", "p-alice")
	r := newRegistry(t)
	if err := r.Dispatch(context.Background(), command.Env{}, alice, "tell Bob hi"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := alice.lastLine(); !strings.Contains(got, "not enabled") {
		t.Errorf("no-notif = %q", got)
	}
}

func TestTell_NameOnlyRejected(t *testing.T) {
	mgr, res := newTellFixture(t)
	alice := newTellsActor("Alice", "p-alice")
	dispatchTell(t, alice, mgr, res, "tell Bob")
	if got := alice.lastLine(); !strings.Contains(got, "Tell whom what") {
		t.Errorf("name-only tell = %q", got)
	}
}

// writerSink is a notifications.Sink that delivers to the actor's
// Write and also mirrors the production tell-state recording so the
// command tests can assert the recipient's slot updates.
type writerSink struct {
	a  *testActor
	ta *tellsActor
}

func (s *writerSink) Deliver(ctx context.Context, n notifications.Notification) error {
	if err := s.a.Write(ctx, n.Text); err != nil {
		return err
	}
	if n.Kind == "tell" && s.ta != nil {
		s.ta.SetLastTellPartner(n.Sender)
		s.ta.AppendRecentTell(n.Text)
	}
	return nil
}

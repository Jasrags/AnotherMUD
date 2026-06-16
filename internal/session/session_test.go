package session_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/account"
	"github.com/Jasrags/AnotherMUD/internal/clock"
	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/server"
	"github.com/Jasrags/AnotherMUD/internal/session"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// testRig stands up an account+player+session stack against a real TCP
// listener with a single throwaway temp save dir. Each test gets a
// fresh dir so they don't share login state.
type testRig struct {
	t        *testing.T
	world    *world.World
	cmds     *command.Registry
	accounts *account.Service
	players  *player.Store
	mgr      *session.Manager
	ln       net.Listener
	cancel   context.CancelFunc
	done     chan error
	startID  world.RoomID
	clock    clock.Clock // populated by startRigOpts; nil for the default rig
}

type rigOpts struct {
	color    bool
	flood    session.FloodConfig
	clk      clock.Clock
	linkDead session.LinkDeadConfig
}

func startRig(t *testing.T, w *world.World, startID world.RoomID, color bool) *testRig {
	return startRigOpts(t, w, startID, rigOpts{color: color})
}

func startRigWithFlood(t *testing.T, w *world.World, startID world.RoomID, color bool, flood session.FloodConfig) *testRig {
	return startRigOpts(t, w, startID, rigOpts{color: color, flood: flood})
}

func startRigOpts(t *testing.T, w *world.World, startID world.RoomID, opts rigOpts) *testRig {
	t.Helper()
	dir := t.TempDir()
	accs, err := account.NewService(dir, account.WithBcryptCost(account.MinBcryptCostForTests))
	if err != nil {
		t.Fatalf("account.NewService: %v", err)
	}
	plrs, err := player.NewStore(dir)
	if err != nil {
		t.Fatalf("player.NewStore: %v", err)
	}
	cmds := command.New()
	if err := command.RegisterBuiltins(cmds); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	mgr := session.NewManager()
	cfg := session.Config{
		World:        w,
		Commands:     cmds,
		Players:      plrs,
		Manager:      mgr,
		StartID:      startID,
		ColorEnabled: opts.color,
		Flood:        opts.flood,
		Clock:        opts.clk,
		LinkDead:     opts.linkDead,
		Login: login.Config{
			Accounts:        accs,
			Players:         plrs,
			DefaultLocation: string(startID),
		},
	}
	srv := &server.Server{Handler: session.Handler(cfg)}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln) }()

	return &testRig{
		t: t, world: w, cmds: cmds, accounts: accs, players: plrs, mgr: mgr,
		ln: ln, cancel: cancel, done: done, startID: startID,
		clock: opts.clk,
	}
}

func (r *testRig) stop(t *testing.T) {
	t.Helper()
	r.cancel()
	select {
	case err := <-r.done:
		if err != nil && !errors.Is(err, server.ErrServerClosed) {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop in time")
	}
}

// driveClient bundles a dialed connection with helpers that drain until
// a marker arrives. Each call resets the read deadline.
type driveClient struct {
	t  *testing.T
	c  net.Conn
	br *bufio.Reader
}

func dial(t *testing.T, addr string) *driveClient {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return &driveClient{t: t, c: c, br: bufio.NewReader(c)}
}

func (d *driveClient) close() { _ = d.c.Close() }

func (d *driveClient) writeLine(s string) {
	d.t.Helper()
	if _, err := d.c.Write([]byte(s + "\r\n")); err != nil {
		d.t.Fatalf("write %q: %v", s, err)
	}
}

func (d *driveClient) drainUntil(marker string) string {
	d.t.Helper()
	if err := d.c.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		d.t.Fatalf("deadline: %v", err)
	}
	var buf strings.Builder
	for buf.Len() < 8192 {
		// ReadByte rather than ReadString since password masking IAC
		// bytes are not newline-terminated.
		b, err := d.br.ReadByte()
		if err == nil {
			buf.WriteByte(b)
		}
		if strings.Contains(buf.String(), marker) {
			return buf.String()
		}
		if err != nil {
			d.t.Fatalf("read for %q: %v\nbuf: %q", marker, err, buf.String())
		}
	}
	d.t.Fatalf("did not see %q\nbuf: %q", marker, buf.String())
	return ""
}

// loginNew drives the account-first new-account + first-character flow
// (character-select.md): account username, password (+ confirm), then the
// character name (empty roster → create). `name` is used as both the
// account username and the character name; `email` is ignored (email is no
// longer collected — character-select §2.1). Leaves the connection at the
// first in-game prompt.
func (d *driveClient) loginNew(name, email, password string) {
	d.t.Helper()
	_ = email
	d.drainUntil("Account username:")
	d.writeLine(name)
	d.drainUntil("Choose a password")
	d.writeLine(password)
	d.drainUntil("Confirm password")
	d.writeLine(password)
	d.drainUntil("new character's name")
	d.writeLine(name)
	d.drainUntil(fmt.Sprintf("Welcome, %s.", name))
}

// loginReturning drives the account-first returning flow: account username,
// password, then selecting the character from the roster by name.
func (d *driveClient) loginReturning(name, password string) {
	d.t.Helper()
	d.drainUntil("Account username:")
	d.writeLine(name)
	d.drainUntil("Password")
	d.writeLine(password)
	d.drainUntil("Select a character")
	d.writeLine(name)
}

// TestSessionColorFallback proves the plain-text color fallback after
// login.
func TestSessionColorFallback(t *testing.T) {
	w := world.New()
	a := &world.Room{
		ID:          "a",
		Name:        "{Y}Bright Room{x}",
		Description: "The walls glow {R}red hot{x}.",
	}
	w.AddRoom(a)

	rig := startRig(t, w, a.ID, true)
	defer rig.stop(t)

	d := dial(t, rig.ln.Addr().String())
	defer d.close()
	d.loginNew("Alice", "alice@example.com", "hunter22")

	// First post-login render should contain escape bytes.
	greeting := d.drainUntil("Bright Room")
	if !strings.Contains(greeting, "\x1b[") {
		t.Errorf("color-on greeting missing escape bytes: %q", greeting)
	}
	if strings.Contains(greeting, "{Y}") {
		t.Errorf("raw markup leaked: %q", greeting)
	}

	d.writeLine("color off")
	d.drainUntil("Color disabled.")
	d.writeLine("look")
	plain := d.drainUntil("Bright Room")
	if strings.Contains(plain, "\x1b") {
		t.Errorf("color-off render leaked escape bytes: %q", plain)
	}

	d.writeLine("quit")
}

// TestSessionEndToEnd exercises connect → login → look → n → look → quit.
func TestSessionEndToEnd(t *testing.T) {
	w := world.New()
	a := &world.Room{ID: "a", Name: "Room Alpha", Description: "the alpha room"}
	b := &world.Room{ID: "b", Name: "Room Beta", Description: "the beta room"}
	a.Exits = map[world.Direction]world.Exit{world.DirNorth: {Target: b.ID}}
	b.Exits = map[world.Direction]world.Exit{world.DirSouth: {Target: a.ID}}
	w.AddRoom(a)
	w.AddRoom(b)

	rig := startRig(t, w, a.ID, false)
	defer rig.stop(t)

	d := dial(t, rig.ln.Addr().String())
	defer d.close()
	d.loginNew("Bob", "bob@example.com", "hunter22")

	d.drainUntil("Room Alpha")
	d.writeLine("look")
	d.drainUntil("Room Alpha")
	d.writeLine("n")
	d.drainUntil("Room Beta")
	d.writeLine("xyzzy")
	d.drainUntil("Huh?")
	d.writeLine("quit")
	d.drainUntil("Goodbye.")
}

// TestSessionIdleTimeoutDisconnects is the M4.3 end-to-end check that
// the IdleSweep → write → conn.Close → read loop unwind chain works
// through a real TCP socket. Uses ManualClock so the test doesn't
// have to wait minutes. Note: this test drives IdleSweep manually
// rather than going through the tick loop — the tick-handler wiring
// in main.go is intentionally not exercised here, just the session
// teardown path.
func TestSessionIdleTimeoutDisconnects(t *testing.T) {
	w := world.New()
	r := &world.Room{ID: "a", Name: "Room", Description: "."}
	w.AddRoom(r)

	mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	idle := session.IdleConfig{
		WarnAfter:      30 * time.Second,
		TimeoutAfter:   60 * time.Second,
		WarnMessage:    "(Idle warning.)",
		TimeoutMessage: "Disconnected: idle timeout.",
	}
	rig := startRigOpts(t, w, r.ID, rigOpts{clk: mc})
	defer rig.stop(t)

	d := dial(t, rig.ln.Addr().String())
	defer d.close()
	d.loginNew("Idle", "idle@example.com", "hunter22")
	d.drainUntil("Room")

	// Manually drive the idle sweep: jump past warn, then past timeout.
	mc.Advance(31 * time.Second)
	rig.mgr.IdleSweep(context.Background(), idle, mc)
	d.drainUntil("Idle warning.")

	mc.Advance(31 * time.Second)
	rig.mgr.IdleSweep(context.Background(), idle, mc)
	d.drainUntil("Disconnected: idle timeout.")

	// Server should close the connection promptly.
	_ = d.c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	if _, err := d.c.Read(buf); err == nil || !(errors.Is(err, io.EOF) || isNetClosed(err)) {
		t.Errorf("expected EOF after idle disconnect, got err=%v", err)
	}
}

// TestSessionFloodDisconnect is the M4.2 integration check: an abusive
// client whose input rate blows past the bucket should receive
// "Slow down." and ultimately be disconnected with the canonical
// "Disconnected: command flooding." message.
func TestSessionFloodDisconnect(t *testing.T) {
	w := world.New()
	r := &world.Room{ID: "a", Name: "Room", Description: "."}
	w.AddRoom(r)

	// Tiny config: 1 token burst, very slow refill so consecutive
	// inputs strike out fast.
	flood := session.FloodConfig{
		CommandsPerSecond:  0.01,
		BurstSize:          1,
		StrikeThreshold:    2,
		StrikeDecaySeconds: 30,
	}
	rig := startRigWithFlood(t, w, r.ID, false, flood)
	defer rig.stop(t)

	d := dial(t, rig.ln.Addr().String())
	defer d.close()
	d.loginNew("Mallory", "mallory@example.com", "hunter22")
	d.drainUntil("Room")

	// First "look" consumes the only token (allow). Subsequent inputs
	// strike: drop+warn, then drop+disconnect.
	d.writeLine("look")
	d.drainUntil("Room")
	d.writeLine("look")
	d.drainUntil("Slow down.")
	d.writeLine("look")
	d.drainUntil("Disconnected: command flooding.")

	// Server closes the connection after the message; subsequent reads
	// should hit EOF promptly.
	_ = d.c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	if _, err := d.c.Read(buf); err == nil || !(errors.Is(err, io.EOF) || isNetClosed(err)) {
		t.Errorf("expected EOF after flood disconnect, got err=%v", err)
	}
}

// TestSessionLinkDeadReconnect is the M4.4 visible-payoff test: Alice
// logs in, her TCP socket drops, a peer sees the "lost their
// connection" broadcast, the cleanup sweep does NOT reap her within
// the grace window, and a fresh login on her character reattaches the
// session (room render returns, dispatch works again).
func TestSessionLinkDeadReconnect(t *testing.T) {
	w := world.New()
	r := &world.Room{ID: "a", Name: "Room Alpha", Description: "alpha"}
	w.AddRoom(r)

	mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	rig := startRigOpts(t, w, r.ID, rigOpts{
		clk:      mc,
		linkDead: session.LinkDeadConfig{Enabled: true, TimeoutSeconds: 120},
	})
	defer rig.stop(t)

	alice := dial(t, rig.ln.Addr().String())
	alice.loginNew("Alice", "alice@example.com", "hunter22")
	alice.drainUntil("Room Alpha")

	bob := dial(t, rig.ln.Addr().String())
	defer bob.close()
	bob.loginNew("Bob", "bob@example.com", "hunter22")
	bob.drainUntil("Room Alpha")
	alice.drainUntil("Bob has arrived.") // peer sequencing sanity

	// Drop Alice's TCP socket. The server's read loop sees EOF and
	// (with link-dead enabled) parks her instead of full teardown.
	alice.close()

	// Bob must see the lost-connection broadcast. Then Bob must NOT
	// see a "left" message until cleanup runs.
	bob.drainUntil("Alice has lost their connection.")

	// Wait for the link-dead transition to land (the read loop unwinds
	// asynchronously). Poll for byPlayerID["alice"] to enter phase
	// linkDead.
	deadline := time.Now().Add(2 * time.Second)
	var parked bool
	for time.Now().Before(deadline) {
		if rig.mgr.IsPlayerLinkDead("Alice") {
			parked = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !parked {
		t.Fatal("Alice never entered link-dead phase")
	}

	// Within the grace window, cleanup is a no-op.
	mc.Advance(60 * time.Second)
	rig.mgr.LinkDeadCleanup(context.Background(), session.LinkDeadConfig{Enabled: true, TimeoutSeconds: 120}, mc)
	if _, ok := rig.mgr.GetByName("Alice"); !ok {
		t.Fatal("Alice reaped within grace window")
	}

	// Reconnect: a new TCP socket logs in as Alice. Should hit the
	// reconnect path and receive "Reconnected." plus a room render.
	alice2 := dial(t, rig.ln.Addr().String())
	defer alice2.close()
	alice2.loginReturning("Alice", "hunter22")
	alice2.drainUntil("Reconnected.")
	alice2.drainUntil("Room Alpha")

	// Pump is alive: an in-game command produces a response.
	alice2.writeLine("look")
	alice2.drainUntil("Room Alpha")

	alice2.writeLine("quit")
	alice2.drainUntil("Goodbye.")
}

// TestSessionLinkDeadCleanupReaps verifies the reaper actually removes
// a parked session after the timeout, persisting the player and
// broadcasting departure to room peers.
func TestSessionLinkDeadCleanupReaps(t *testing.T) {
	w := world.New()
	r := &world.Room{ID: "a", Name: "Room Alpha", Description: "alpha"}
	w.AddRoom(r)

	mc := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := session.LinkDeadConfig{Enabled: true, TimeoutSeconds: 30}
	rig := startRigOpts(t, w, r.ID, rigOpts{clk: mc, linkDead: cfg})
	defer rig.stop(t)

	alice := dial(t, rig.ln.Addr().String())
	alice.loginNew("Alice", "alice@example.com", "hunter22")
	alice.drainUntil("Room Alpha")

	bob := dial(t, rig.ln.Addr().String())
	defer bob.close()
	bob.loginNew("Bob", "bob@example.com", "hunter22")
	bob.drainUntil("Room Alpha")
	alice.drainUntil("Bob has arrived.")

	alice.close()
	bob.drainUntil("Alice has lost their connection.")

	// Wait for park.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rig.mgr.IsPlayerLinkDead("Alice") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Past the timeout, the sweep must reap.
	mc.Advance(31 * time.Second)
	rig.mgr.LinkDeadCleanup(context.Background(), cfg, mc)

	bob.drainUntil("Alice has left.")
	if _, ok := rig.mgr.GetByName("Alice"); ok {
		t.Error("Alice still indexed after timeout cleanup")
	}

	bob.writeLine("quit")
	bob.drainUntil("Goodbye.")
}

// TestSessionLinkDeadDisabledFallsBackToFullTeardown: with link-dead
// off, a socket drop still produces the M3 "has left." broadcast and
// removes the session immediately.
func TestSessionLinkDeadDisabledFallsBackToFullTeardown(t *testing.T) {
	w := world.New()
	r := &world.Room{ID: "a", Name: "Room Alpha", Description: "alpha"}
	w.AddRoom(r)

	rig := startRigOpts(t, w, r.ID, rigOpts{
		linkDead: session.LinkDeadConfig{Enabled: false},
	})
	defer rig.stop(t)

	alice := dial(t, rig.ln.Addr().String())
	alice.loginNew("Alice", "alice@example.com", "hunter22")
	alice.drainUntil("Room Alpha")

	bob := dial(t, rig.ln.Addr().String())
	defer bob.close()
	bob.loginNew("Bob", "bob@example.com", "hunter22")
	bob.drainUntil("Room Alpha")
	alice.drainUntil("Bob has arrived.")

	alice.close()
	bob.drainUntil("Alice has left.")

	if _, ok := rig.mgr.GetByName("Alice"); ok {
		t.Error("Alice still indexed after disabled-linkdead disconnect")
	}

	bob.writeLine("quit")
	bob.drainUntil("Goodbye.")
}

func isNetClosed(err error) bool {
	if err == nil {
		return false
	}
	// net.OpError wrapping "use of closed network connection" or
	// reset-by-peer also counts as the server tearing down.
	return strings.Contains(err.Error(), "closed") ||
		strings.Contains(err.Error(), "reset")
}

// TestTwoPlayersSeeEachOther is the M4.1 visible-payoff test:
// player A is already online when B logs in; A must observe B's
// arrival, and when B walks away A must observe the departure.
func TestTwoPlayersSeeEachOther(t *testing.T) {
	w := world.New()
	a := &world.Room{ID: "a", Name: "Room Alpha", Description: "alpha"}
	b := &world.Room{ID: "b", Name: "Room Beta", Description: "beta"}
	a.Exits = map[world.Direction]world.Exit{world.DirNorth: {Target: b.ID}}
	b.Exits = map[world.Direction]world.Exit{world.DirSouth: {Target: a.ID}}
	w.AddRoom(a)
	w.AddRoom(b)

	rig := startRig(t, w, a.ID, false)
	defer rig.stop(t)

	alice := dial(t, rig.ln.Addr().String())
	defer alice.close()
	alice.loginNew("Alice", "alice@example.com", "hunter22")
	alice.drainUntil("Room Alpha")

	bob := dial(t, rig.ln.Addr().String())
	defer bob.close()
	bob.loginNew("Bob", "bob@example.com", "hunter22")
	bob.drainUntil("Room Alpha")

	// Alice must have seen Bob arrive.
	arrived := alice.drainUntil("Bob has arrived.")
	if !strings.Contains(arrived, "Bob has arrived.") {
		t.Errorf("alice did not see arrival, got: %q", arrived)
	}

	// Bob walks north. Alice should see him head off; Bob's render is
	// the destination room.
	bob.writeLine("n")
	bob.drainUntil("Room Beta")
	heads := alice.drainUntil("Bob heads north.")
	if !strings.Contains(heads, "Bob heads north.") {
		t.Errorf("alice did not see Bob depart, got: %q", heads)
	}

	// Alice walks north too — Bob should see her arrive from the south.
	alice.writeLine("n")
	alice.drainUntil("Room Beta")
	bobSees := bob.drainUntil("Alice arrives from the south.")
	if !strings.Contains(bobSees, "Alice arrives from the south.") {
		t.Errorf("bob did not see Alice arrive, got: %q", bobSees)
	}

	// Alice quits: Bob (same room) must see her leave.
	alice.writeLine("quit")
	alice.drainUntil("Goodbye.")
	left := bob.drainUntil("Alice has left.")
	if !strings.Contains(left, "Alice has left.") {
		t.Errorf("bob did not see Alice quit, got: %q", left)
	}

	bob.writeLine("quit")
	bob.drainUntil("Goodbye.")
}

// TestSessionTakeoverYesPath is the M4.5 happy path: Alice is online and
// walks to a second room (state that has NOT yet been autosaved); a
// second login for Alice answers "yes" to the takeover prompt; the
// original socket sees the notice and is closed; the new socket lands
// in-game on the same character at the *post-walk* room — proving the
// live in-memory save was transferred to the new session rather than a
// stale on-disk record being reloaded.
func TestSessionTakeoverYesPath(t *testing.T) {
	w := world.New()
	a := &world.Room{ID: "a", Name: "Room Alpha", Description: "alpha"}
	b := &world.Room{ID: "b", Name: "Room Beta", Description: "beta"}
	a.Exits = map[world.Direction]world.Exit{world.DirNorth: {Target: b.ID}}
	b.Exits = map[world.Direction]world.Exit{world.DirSouth: {Target: a.ID}}
	w.AddRoom(a)
	w.AddRoom(b)

	rig := startRigOpts(t, w, a.ID, rigOpts{
		// LinkDead disabled so the test cannot accidentally select the
		// reconnect branch — takeover requires the existing session to
		// be in Playing phase.
		linkDead: session.LinkDeadConfig{Enabled: false},
	})
	defer rig.stop(t)

	alice1 := dial(t, rig.ln.Addr().String())
	alice1.loginNew("Alice", "alice@example.com", "hunter22")
	alice1.drainUntil("Room Alpha")

	// Walk north so the live in-memory save no longer matches the disk
	// snapshot login.Run will load for the second connection.
	alice1.writeLine("n")
	alice1.drainUntil("Room Beta")

	alice2 := dial(t, rig.ln.Addr().String())
	defer alice2.close()
	alice2.loginReturning("Alice", "hunter22")
	alice2.drainUntil("Take over the existing session")
	alice2.writeLine("yes")

	// Old socket sees the takeover notice; new socket renders the room
	// Alice was actually in (Beta), not the on-disk Alpha.
	alice1.drainUntil("Another connection has taken over this character.")
	alice2.drainUntil("Room Beta")

	// Belt-and-braces: explicit "look" must also show Beta.
	alice2.writeLine("look")
	alice2.drainUntil("Room Beta")

	alice1.close()

	// Manager invariant: exactly one Alice indexed.
	if got := rig.mgr.Count(); got != 1 {
		t.Errorf("manager count = %d, want 1", got)
	}
	if _, ok := rig.mgr.GetByName("Alice"); !ok {
		t.Fatal("Alice not indexed after takeover")
	}

	alice2.writeLine("quit")
	alice2.drainUntil("Goodbye.")
}

// TestSessionTakeoverNoPath: the second login answers "no" to the
// takeover prompt; the new connection is closed and the existing
// session is untouched.
func TestSessionTakeoverNoPath(t *testing.T) {
	w := world.New()
	r := &world.Room{ID: "a", Name: "Room Alpha", Description: "alpha"}
	w.AddRoom(r)

	rig := startRigOpts(t, w, r.ID, rigOpts{
		linkDead: session.LinkDeadConfig{Enabled: false},
	})
	defer rig.stop(t)

	alice1 := dial(t, rig.ln.Addr().String())
	defer alice1.close()
	alice1.loginNew("Alice", "alice@example.com", "hunter22")
	alice1.drainUntil("Room Alpha")

	alice2 := dial(t, rig.ln.Addr().String())
	defer alice2.close()
	alice2.loginReturning("Alice", "hunter22")
	alice2.drainUntil("Take over the existing session")
	alice2.writeLine("no")
	alice2.drainUntil("Login cancelled")

	// Existing session must still be in-game and responsive.
	alice1.writeLine("look")
	alice1.drainUntil("Room Alpha")

	// Manager still has exactly one Alice.
	if got := rig.mgr.Count(); got != 1 {
		t.Errorf("manager count = %d, want 1", got)
	}

	alice1.writeLine("quit")
	alice1.drainUntil("Goodbye.")
}

// TestSessionPersistsLocationAcrossRestart is the M3 integration
// criterion: create, walk to room b, restart server (same save dir),
// log in, verify the player is in b.
func TestSessionPersistsLocationAcrossRestart(t *testing.T) {
	w := world.New()
	a := &world.Room{ID: "a", Name: "Room Alpha", Description: "alpha"}
	b := &world.Room{ID: "b", Name: "Room Beta", Description: "beta"}
	a.Exits = map[world.Direction]world.Exit{world.DirNorth: {Target: b.ID}}
	b.Exits = map[world.Direction]world.Exit{world.DirSouth: {Target: a.ID}}
	w.AddRoom(a)
	w.AddRoom(b)

	// Share a save dir across two server lifecycles. Each lifecycle is
	// built inline so cancellation scope is obvious.
	dir := t.TempDir()

	// --- first lifecycle: create Carol, walk north ---
	{
		accs, err := account.NewService(dir, account.WithBcryptCost(account.MinBcryptCostForTests))
		if err != nil {
			t.Fatalf("accs: %v", err)
		}
		plrs, err := player.NewStore(dir)
		if err != nil {
			t.Fatalf("plrs: %v", err)
		}
		cmds := command.New()
		if err := command.RegisterBuiltins(cmds); err != nil {
			t.Fatalf("RegisterBuiltins: %v", err)
		}
		mgr := session.NewManager()
		cfg := session.Config{
			World: w, Commands: cmds, Players: plrs, Manager: mgr,
			StartID: a.ID, ColorEnabled: false,
			Login: login.Config{
				Accounts:        accs,
				Players:         plrs,
				DefaultLocation: string(a.ID),
			},
		}
		srv := &server.Server{Handler: session.Handler(cfg)}
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- srv.Serve(ctx, ln) }()

		d := dial(t, ln.Addr().String())
		d.loginNew("Carol", "carol@example.com", "hunter22")
		d.drainUntil("Room Alpha")
		d.writeLine("n")
		d.drainUntil("Room Beta")
		d.writeLine("quit")
		d.drainUntil("Goodbye.")
		d.close()

		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("server 1 did not stop")
		}
	}

	// --- second lifecycle: log Carol back in, verify in Room Beta ---
	{
		accs, err := account.NewService(dir, account.WithBcryptCost(account.MinBcryptCostForTests))
		if err != nil {
			t.Fatalf("accs2: %v", err)
		}
		plrs, err := player.NewStore(dir)
		if err != nil {
			t.Fatalf("plrs2: %v", err)
		}
		cmds := command.New()
		if err := command.RegisterBuiltins(cmds); err != nil {
			t.Fatalf("RegisterBuiltins: %v", err)
		}
		mgr := session.NewManager()
		cfg := session.Config{
			World: w, Commands: cmds, Players: plrs, Manager: mgr,
			StartID: a.ID, ColorEnabled: false,
			Login: login.Config{
				Accounts:        accs,
				Players:         plrs,
				DefaultLocation: string(a.ID),
			},
		}
		srv := &server.Server{Handler: session.Handler(cfg)}
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen 2: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- srv.Serve(ctx, ln) }()

		d := dial(t, ln.Addr().String())
		d.loginReturning("Carol", "hunter22")
		// First post-login render is where Carol left off — Room Beta.
		first := d.drainUntil("Exits:")
		if !strings.Contains(first, "Room Beta") {
			t.Errorf("expected to wake in Room Beta after restart, got: %q", first)
		}
		d.writeLine("quit")
		d.drainUntil("Goodbye.")
		d.close()

		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("server 2 did not stop")
		}
	}
}

// TestBootstrapAdmin_FirstCharacterOnly proves the very first character
// created in a fresh deployment is auto-granted the admin role, while the
// next character is not — the gate is the empty player store, so it fires
// exactly once ever. The rig leaves Config.AdminRole empty, so the
// bootstrap falls back to the "admin" dispatch default.
func TestBootstrapAdmin_FirstCharacterOnly(t *testing.T) {
	w := world.New()
	room := &world.Room{ID: "a", Name: "Square"}
	w.AddRoom(room)

	rig := startRig(t, w, room.ID, false)
	defer rig.stop(t)

	// First character: store is empty at construction, so it's admin.
	d1 := dial(t, rig.ln.Addr().String())
	defer d1.close()
	d1.loginNew("Alice", "alice@example.com", "hunter22")
	a, ok := rig.mgr.GetByName("Alice")
	if !ok {
		t.Fatal("first character not registered with manager")
	}
	if !a.HasRole("admin") {
		t.Errorf("first character should be bootstrapped as admin, roles=%v", a.Roles())
	}

	// Second character: Alice is now on disk, so the store is no longer
	// empty and the bootstrap does not fire.
	d2 := dial(t, rig.ln.Addr().String())
	defer d2.close()
	d2.loginNew("Bob", "bob@example.com", "hunter22")
	b, ok := rig.mgr.GetByName("Bob")
	if !ok {
		t.Fatal("second character not registered with manager")
	}
	if b.HasRole("admin") {
		t.Errorf("second character should NOT be admin, roles=%v", b.Roles())
	}
}

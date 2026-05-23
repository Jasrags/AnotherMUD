package session_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/account"
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
}

func startRig(t *testing.T, w *world.World, startID world.RoomID, color bool) *testRig {
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
		ColorEnabled: color,
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

// loginNew drives the new-character flow: name, email, password (and
// confirmation). Leaves the connection at the first in-game prompt.
func (d *driveClient) loginNew(name, email, password string) {
	d.t.Helper()
	d.drainUntil("name shall we know")
	d.writeLine(name)
	d.drainUntil("Email address")
	d.writeLine(email)
	d.drainUntil("Choose a password")
	d.writeLine(password)
	d.drainUntil("Confirm password")
	d.writeLine(password)
	d.drainUntil(fmt.Sprintf("Welcome, %s.", name))
}

// loginReturning drives the returning-character flow.
func (d *driveClient) loginReturning(name, password string) {
	d.t.Helper()
	d.drainUntil("name shall we know")
	d.writeLine(name)
	d.drainUntil("Password")
	d.writeLine(password)
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

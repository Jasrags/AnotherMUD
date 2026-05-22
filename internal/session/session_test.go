package session_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/server"
	"github.com/Jasrags/AnotherMUD/internal/session"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// TestSessionColorFallback proves the plain-text fallback: a room
// description authored with ANSI markup renders with escape bytes when
// the session has color enabled, and with NO escape bytes once the
// player runs `color off`. This is the integration test M2 phase 4's
// exit criterion explicitly calls for.
func TestSessionColorFallback(t *testing.T) {
	w := world.New()
	// Description is authored with brace markup — exactly what pack
	// YAML files contain after Phase 4.
	a := &world.Room{
		ID:          "a",
		Name:        "{Y}Bright Room{x}",
		Description: "The walls glow {R}red hot{x}.",
	}
	w.AddRoom(a)

	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}

	srv := &server.Server{Handler: session.Handler(session.Config{
		World: w, Commands: r, StartID: a.ID, ColorEnabled: true,
	})}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln) }()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	br := bufio.NewReader(c)

	drainUntil := func(t *testing.T, marker string) string {
		t.Helper()
		if err := c.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatalf("deadline: %v", err)
		}
		var buf strings.Builder
		for buf.Len() < 4096 {
			line, err := br.ReadString('\n')
			if line != "" {
				buf.WriteString(line)
			}
			if strings.Contains(buf.String(), marker) {
				return buf.String()
			}
			if err != nil {
				t.Fatalf("read for %q: %v\nbuf: %q", marker, err, buf.String())
			}
		}
		t.Fatalf("did not see %q\nbuf: %q", marker, buf.String())
		return ""
	}

	// Initial greeting includes the room render — should contain ESC bytes.
	greeting := drainUntil(t, "Bright Room")
	if !strings.Contains(greeting, "\x1b[") {
		t.Errorf("color-on greeting missing escape bytes: %q", greeting)
	}
	if strings.Contains(greeting, "{Y}") || strings.Contains(greeting, "{R}") {
		t.Errorf("raw markup leaked through color-on render: %q", greeting)
	}

	// Turn color off and look again.
	if _, err := c.Write([]byte("color off\r\n")); err != nil {
		t.Fatalf("write color off: %v", err)
	}
	drainUntil(t, "Color disabled.")
	if _, err := c.Write([]byte("look\r\n")); err != nil {
		t.Fatalf("write look: %v", err)
	}
	plain := drainUntil(t, "Bright Room")
	if strings.Contains(plain, "\x1b") {
		t.Errorf("color-off render leaked escape bytes: %q", plain)
	}
	if strings.Contains(plain, "{Y}") || strings.Contains(plain, "{R}") {
		t.Errorf("raw markup leaked through color-off render: %q", plain)
	}

	if _, err := c.Write([]byte("quit\r\n")); err != nil {
		t.Fatalf("write quit: %v", err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, server.ErrServerClosed) {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop in time")
	}
}

// TestSessionEndToEnd exercises connect → look → n → look → quit
// against a real TCP listener with the M1 wiring (world, command
// registry, session handler). It is the single integration test that
// proves the M1 slice runs end-to-end.
func TestSessionEndToEnd(t *testing.T) {
	w := world.New()
	a := &world.Room{ID: "a", Name: "Room Alpha", Description: "the alpha room"}
	b := &world.Room{ID: "b", Name: "Room Beta", Description: "the beta room"}
	a.Exits = map[world.Direction]world.Exit{world.DirNorth: {Target: b.ID}}
	b.Exits = map[world.Direction]world.Exit{world.DirSouth: {Target: a.ID}}
	w.AddRoom(a)
	w.AddRoom(b)

	r := command.New()
	if err := command.RegisterBuiltins(r); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}

	srv := &server.Server{Handler: session.Handler(session.Config{
		World: w, Commands: r, StartID: a.ID,
	})}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln) }()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	br := bufio.NewReader(c)

	mustRead := func(t *testing.T, want string) {
		t.Helper()
		if err := c.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatalf("set deadline: %v", err)
		}
		// Drain until we see the substring or hit deadline. Each
		// dispatch produces multi-line output (name + desc + exits).
		var buf strings.Builder
		for buf.Len() < 4096 {
			line, err := br.ReadString('\n')
			if line != "" {
				buf.WriteString(line)
			}
			if strings.Contains(buf.String(), want) {
				return
			}
			if err != nil {
				t.Fatalf("read while waiting for %q: %v\nbuffer: %q", want, err, buf.String())
			}
		}
		t.Fatalf("did not see %q in output\nbuffer: %q", want, buf.String())
	}

	mustRead(t, "Welcome")
	mustRead(t, "Room Alpha")

	if _, err := c.Write([]byte("look\r\n")); err != nil {
		t.Fatalf("write look: %v", err)
	}
	mustRead(t, "Room Alpha")

	if _, err := c.Write([]byte("n\r\n")); err != nil {
		t.Fatalf("write n: %v", err)
	}
	mustRead(t, "Room Beta")

	if _, err := c.Write([]byte("look\r\n")); err != nil {
		t.Fatalf("write look: %v", err)
	}
	mustRead(t, "Room Beta")

	if _, err := c.Write([]byte("xyzzy\r\n")); err != nil {
		t.Fatalf("write unknown: %v", err)
	}
	mustRead(t, "Huh?")

	if _, err := c.Write([]byte("quit\r\n")); err != nil {
		t.Fatalf("write quit: %v", err)
	}
	mustRead(t, "Goodbye.")

	// Server should close the connection after quit.
	if err := c.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := br.ReadByte(); err == nil {
		t.Fatal("expected EOF after quit, got data")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, server.ErrServerClosed) {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop in time")
	}
}

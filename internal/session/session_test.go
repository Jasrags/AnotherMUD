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

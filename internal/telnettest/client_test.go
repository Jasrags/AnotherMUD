package telnettest

import (
	"net"
	"regexp"
	"strings"
	"testing"
	"time"
)

// fakeServer drives the server end of a net.Pipe with a scripted exchange, so
// the client's send/expect logic is tested with NO engine running. Each step is
// either a write (canned server output) or a read (consume one client line).
func TestClient_SendExpect_NoEngine(t *testing.T) {
	cli, srv := net.Pipe()
	c := New(cli, WithTimeout(2*time.Second))
	t.Cleanup(func() { _ = c.Close() })

	go func() {
		defer srv.Close()
		// Greeting with leading telnet negotiation (IAC WILL ECHO) that must be
		// stripped before matching.
		_, _ = srv.Write([]byte{iac, will, 1})
		_, _ = srv.Write([]byte("By what name shall we know you? "))
		// Read the client's line.
		buf := make([]byte, 64)
		n, _ := srv.Read(buf)
		if got := strings.TrimSpace(string(buf[:n])); got != "Alice" {
			t.Errorf("server received %q, want %q", got, "Alice")
		}
		_, _ = srv.Write([]byte("Welcome, Alice.\r\n[HP 20/20]> "))
	}()

	if _, err := c.ExpectString("name shall we know you?"); err != nil {
		t.Fatalf("expect greeting: %v", err)
	}
	if err := c.SendLine("Alice"); err != nil {
		t.Fatalf("send name: %v", err)
	}
	out, err := c.Expect(regexp.MustCompile(`\[HP \d+/\d+\]`))
	if err != nil {
		t.Fatalf("expect prompt: %v", err)
	}
	if strings.ContainsRune(out, 0xFF) {
		t.Errorf("matched output still contains a raw IAC byte: %q", out)
	}
	if !strings.Contains(out, "Welcome, Alice.") {
		t.Errorf("matched output = %q, want it to include the welcome line", out)
	}
}

func TestClient_ExpectTimeout_ReportsLastOutput(t *testing.T) {
	cli, srv := net.Pipe()
	c := New(cli, WithTimeout(200*time.Millisecond))
	t.Cleanup(func() { _ = c.Close() })

	go func() {
		_, _ = srv.Write([]byte("partial output, no match here"))
		// then go silent so the Expect times out
	}()

	_, err := c.ExpectString("THIS NEVER APPEARS")
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error = %v, want it to mention a timeout", err)
	}
	if !strings.Contains(err.Error(), "partial output") {
		t.Errorf("error = %v, want it to include the last server output for debugging", err)
	}
}

// fakeTB captures Fatalf without aborting, so DialT's failure path is testable.
type fakeTB struct {
	failed   bool
	cleanups []func()
}

func (f *fakeTB) Helper()               {}
func (f *fakeTB) Fatalf(string, ...any) { f.failed = true }
func (f *fakeTB) Cleanup(fn func())     { f.cleanups = append(f.cleanups, fn) }

func TestDialT_FailsOnBadAddress(t *testing.T) {
	tb := &fakeTB{}
	// Port 0 on an explicit address is not connectable.
	_ = DialT(tb, "127.0.0.1:0", WithTimeout(200*time.Millisecond))
	if !tb.failed {
		t.Error("DialT should have called Fatalf on a dial failure")
	}
}

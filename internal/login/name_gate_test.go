package login

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// ReservedNameGate refuses (with a reprompt) a name that matches the
// reserved blocklist case-insensitively, and trims blocklist entries.
func TestReservedNameGate(t *testing.T) {
	gate := ReservedNameGate([]string{"Admin", "  Guard  ", "SYSTEM"})

	rejected := []string{"Admin", "admin", "ADMIN", "guard", "system"}
	for _, name := range rejected {
		t.Run("reject/"+name, func(t *testing.T) {
			d, reason := gate(name)
			if d != NameReject {
				t.Fatalf("gate(%q) decision = %v, want NameReject", name, d)
			}
			if reason == "" {
				t.Errorf("gate(%q) gave no user-facing reason", name)
			}
		})
	}

	allowed := []string{"Alice", "Bob", "Adminny", "Guardian"}
	for _, name := range allowed {
		t.Run("allow/"+name, func(t *testing.T) {
			if d, _ := gate(name); d != NameAllow {
				t.Fatalf("gate(%q) decision = %v, want NameAllow", name, d)
			}
		})
	}
}

// runNameGates returns the first non-allow decision in order.
func TestRunNameGates_FirstNonAllowWins(t *testing.T) {
	allow := func(string) (NameDecision, string) { return NameAllow, "" }
	reject := func(string) (NameDecision, string) { return NameReject, "nope" }
	disconnect := func(string) (NameDecision, string) { return NameDisconnect, "bye" }

	if d, _ := runNameGates("x", []NameGate{allow, allow}); d != NameAllow {
		t.Errorf("all-allow → %v, want NameAllow", d)
	}
	if d, reason := runNameGates("x", []NameGate{allow, reject, disconnect}); d != NameReject || reason != "nope" {
		t.Errorf("allow,reject,disconnect → (%v,%q), want (NameReject,\"nope\")", d, reason)
	}
	if d, _ := runNameGates("x", []NameGate{disconnect, reject}); d != NameDisconnect {
		t.Errorf("disconnect-first → %v, want NameDisconnect", d)
	}
	if d, _ := runNameGates("x", nil); d != NameAllow {
		t.Errorf("no gates → %v, want NameAllow", d)
	}
}

// nameGates() falls back to a reserved-names gate built from
// ReservedNames when no explicit gate list is configured, and uses the
// explicit list when one is set.
func TestConfigNameGates_DefaultAndExplicit(t *testing.T) {
	def := Config{ReservedNames: []string{"Admin"}}
	if d, _ := runNameGates("admin", def.nameGates()); d != NameReject {
		t.Errorf("default gate did not reject reserved name; got %v", d)
	}
	if d, _ := runNameGates("Alice", def.nameGates()); d != NameAllow {
		t.Errorf("default gate rejected a normal name; got %v", d)
	}

	explicit := Config{
		ReservedNames: []string{"Admin"}, // should be ignored when NameGates set
		NameGates:     []NameGate{func(string) (NameDecision, string) { return NameDisconnect, "x" }},
	}
	if d, _ := runNameGates("anything", explicit.nameGates()); d != NameDisconnect {
		t.Errorf("explicit gate list not used; got %v", d)
	}
}

// scriptConn returns queued lines in order, then io.EOF; it captures
// everything written so a test can assert on server output.
type scriptConn struct {
	lines []string
	i     int

	mu  sync.Mutex
	out strings.Builder
}

func (c *scriptConn) ID() string { return "script-conn" }

func (c *scriptConn) Read(ctx context.Context) (string, error) {
	if c.i < len(c.lines) {
		s := c.lines[c.i]
		c.i++
		return s, nil
	}
	return "", io.EOF
}

func (c *scriptConn) Write(ctx context.Context, p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.out.Write(p)
}

func (c *scriptConn) Close() error { return nil }

func (c *scriptConn) output() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.out.String()
}

// TestRun_ReservedNameReprompts drives the whole login state machine: a
// reserved name on the new-player path is rejected with a reprompt (the
// phase does not advance), and the connection then closing cleanly is an
// abort — not a character created (spec §3 acceptance).
func TestRun_ReservedNameReprompts(t *testing.T) {
	store, err := player.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	conn := &scriptConn{lines: []string{"Admin"}} // reserved; then EOF on reprompt
	cfg := Config{Players: store, ReservedNames: []string{"admin"}}

	loaded, runErr := Run(context.Background(), conn, cfg)
	if loaded != nil {
		t.Fatalf("Run loaded = %v, want nil (reserved name must not create a character)", loaded)
	}
	if !errors.Is(runErr, ErrAborted) {
		t.Fatalf("Run err = %v, want ErrAborted (EOF after the reprompt)", runErr)
	}
	if out := strings.ToLower(conn.output()); !strings.Contains(out, "reserved") {
		t.Errorf("expected a 'reserved' reprompt message, got: %q", conn.output())
	}
}

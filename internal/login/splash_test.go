package login

import (
	"context"
	"strings"
	"testing"
)

// TestSplash_RenderedBeforePrompt: a configured splash is emitted (line by
// line) ahead of the account-username prompt (character-select §4.1).
func TestSplash_RenderedBeforePrompt(t *testing.T) {
	cfg, _ := menuRig(t)
	cfg.Splash = "=== WELCOME ===\nthe second line"
	conn := &scriptConn{lines: []string{"q"}} // EOF-free: quit not reachable pre-auth, so just disconnect
	// Username prompt reads "q" as a username attempt; it's invalid length-wise
	// only if <3, "q" is 1 char → reprompt, then EOF aborts. We only care about
	// the splash output, captured regardless.
	_, _ = Run(context.Background(), conn, cfg)

	out := conn.output()
	if !strings.Contains(out, "=== WELCOME ===") || !strings.Contains(out, "the second line") {
		t.Errorf("splash not emitted before prompt; got: %q", out)
	}
	if strings.Contains(out, "Welcome to AnotherMUD.") {
		t.Errorf("fallback greeting shown despite a configured splash: %q", out)
	}
	// The splash precedes the first account prompt.
	if i, j := strings.Index(out, "WELCOME"), strings.Index(out, "Account username:"); i < 0 || j < 0 || i > j {
		t.Errorf("splash should precede the username prompt; out=%q", out)
	}
}

// TestSplash_FallbackWhenUnset: with no splash configured, the one-line
// greeting is used (tests / non-pack callers).
func TestSplash_FallbackWhenUnset(t *testing.T) {
	cfg, _ := menuRig(t)
	cfg.Splash = ""
	conn := &scriptConn{lines: []string{}} // immediate EOF at username
	_, _ = Run(context.Background(), conn, cfg)
	if !strings.Contains(conn.output(), "Welcome to AnotherMUD.") {
		t.Errorf("fallback greeting missing when splash unset: %q", conn.output())
	}
}

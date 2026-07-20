//go:build unix

package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunCheckpoint proves the SIN/license spectrum at the Corporate
// Enclave turnstile (north of Fifth Avenue), which gates on a `corporate` access
// permit: a SINless Street Kid is refused (no credential), an Ex-Security runner
// with a firearms-only national SIN is refused (wrong permit), and a Corporate
// Dropout's corporate SIN clears the gate.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunCheckpoint -v
func TestLive_ShadowrunCheckpoint(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	// Spawn every fresh runner AT the turnstile's south side, so `north` hits the
	// checkpoint with no admin/teleport needed.
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:fifth-avenue",
	})

	// northAt creates a fresh Street Samurai of the given origin and returns the
	// output of trying to cross the turnstile north.
	northAt := func(name, origin string) string {
		t.Helper()
		c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
		if err != nil {
			t.Fatalf("dial %s: %v", name, err)
		}
		t.Cleanup(func() { c.Close() })
		isNew, err := doLogin(c, name)
		if err != nil {
			t.Fatalf("login %s: %v", name, err)
		}
		if !isNew {
			t.Fatalf("%s: expected a fresh character", name)
		}
		if err := finishLogin(c, name, isNew, map[string]string{"class": "street samurai", "background": origin}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if err := c.SendLine("north"); err != nil {
			t.Fatalf("%s north: %v", name, err)
		}
		out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("%s: no prompt after north: %v", name, err)
		}
		return strings.ToLower(out)
	}

	// Street Kid — SINless: no credential to present.
	if out := northAt("Sinless", "street kid"); !strings.Contains(out, "no valid credentials") && !strings.Contains(out, "gate stays shut") {
		t.Errorf("SINless Street Kid should be refused with no valid credentials; got:\n%s", out)
	}

	// Ex-Security — national SIN, firearms permit only: real papers, wrong access.
	if out := northAt("Grunt", "ex-security"); !strings.Contains(out, "access this gate demands") {
		t.Errorf("Ex-Security (firearms SIN, no corporate permit) should be refused for lacking the gate's access; got:\n%s", out)
	}

	// Corporate Dropout — corporate SIN with the corporate permit: clears.
	if out := northAt("Wageslave", "corporate dropout"); !strings.Contains(out, "corporate enclave") || !strings.Contains(out, "turnstile") {
		t.Errorf("Corporate Dropout (corporate permit) should clear into the enclave; got:\n%s", out)
	}

	t.Logf("checkpoint verified live: SINless refused (no credential), firearms-SIN refused (wrong permit), corporate SIN cleared")
}

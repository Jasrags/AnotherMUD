//go:build unix

package main

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ChannelerAffinity is a self-contained regression test for WoT S2
// Phase 3 (gender-derived affinity → soft potency scaling). Unlike the
// externally-pointed smoke test, it BOOTS ITS OWN engine subprocess with the
// deterministic env this assertion needs, drives two channelers over telnet,
// and tears the engine down — runnable with a single command:
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ChannelerAffinity -v
//
// It is gated on ANOTHERMUD_LIVE because it shells out to `go run` (a compile +
// a spawned server), which is too heavy for the default `go test ./...`.
//
// The proof: at weak factor 0.1 a Fire-weak firebolt (2d4 × 0.1, floored at 1)
// is ALWAYS 1, while a Fire-strong firebolt is ALWAYS ≥ 2 (2d4 min). So a female
// (saidar, Fire weak) firebolt of exactly 1 and a male (saidin, Fire strong)
// firebolt of ≥ 2 — female strictly weaker than male — is dice-proof evidence
// the gender affinity split bit, with no statistical sampling.
func TestLive_ChannelerAffinity(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":                "wot",
		"ANOTHERMUD_START_ROOM":           "wot:deep-westwood",
		"ANOTHERMUD_AFFINITY_WEAK_FACTOR": "0.1",
	})

	// Female — saidar — Fire is a WEAK Power → firebolt pinned to the floor (1).
	cf, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial female: %v", err)
	}
	if err := createChanneler(cf, "Saidar", "female"); err != nil {
		cf.Close()
		t.Fatalf("create female channeler: %v", err)
	}
	femaleDmg, ferr := fireboltBoarDamage(cf)
	cf.Close() // end her session so her auto-attacks stop before the male engages
	if ferr != nil {
		t.Fatalf("female firebolt: %v", ferr)
	}

	// Male — saidin — Fire is a STRONG Power → firebolt unpenalized (≥ 2).
	cm, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial male: %v", err)
	}
	defer cm.Close()
	if err := createChanneler(cm, "Saidin", "male"); err != nil {
		t.Fatalf("create male channeler: %v", err)
	}
	maleDmg, merr := fireboltBoarDamage(cm)
	if merr != nil {
		t.Fatalf("male firebolt: %v", merr)
	}

	if femaleDmg != 1 {
		t.Errorf("female (saidar) Firebolt = %d, want 1 (Fire weak, floored at weak-factor 0.1)", femaleDmg)
	}
	if maleDmg < 2 {
		t.Errorf("male (saidin) Firebolt = %d, want >=2 (Fire strong, unpenalized)", maleDmg)
	}
	if femaleDmg >= maleDmg {
		t.Errorf("female Fire weave (%d) was not weaker than male (%d) — the gender affinity split did not bite", femaleDmg, maleDmg)
	}
	t.Logf("affinity verified: female saidar Fire=%d (weak) < male saidin Fire=%d (strong)", femaleDmg, maleDmg)
}

// bootEngine launches the engine via `go run ./cmd/anothermud` from the module
// root with the given env overrides, waits until it accepts connections, and
// registers teardown (kills the whole process group, since `go run` spawns the
// compiled server as a child). Returns the listen address.
func bootEngine(t *testing.T, extraEnv map[string]string) string {
	t.Helper()
	addr := freePort(t)

	cmd := exec.Command("go", "run", "./cmd/anothermud")
	cmd.Dir = moduleRoot(t)
	env := append(os.Environ(),
		"ANOTHERMUD_ADDR="+addr,
		"ANOTHERMUD_SAVE_DIR="+t.TempDir(),
	)
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own process group → group-kill on cleanup
	var logs syncBuffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		_ = cmd.Wait()
	})

	deadline := time.Now().Add(60 * time.Second) // first `go run` compiles
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return addr
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("engine never became ready at %s; log:\n%s", addr, logs.String())
	return ""
}

// freePort reserves an ephemeral port and returns its address. There is a small
// window between releasing it and the engine binding it; acceptable for a gated
// integration test.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().String()
}

// moduleRoot walks up from the test's working directory to the directory
// holding go.mod, so `go run ./cmd/anothermud` (and its default ./content) work.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("module root (go.mod) not found")
			return ""
		}
		dir = parent
	}
}

// syncBuffer is a goroutine-safe bytes.Buffer — the exec pipe copier writes to
// it concurrently with the test reading it on the failure path.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

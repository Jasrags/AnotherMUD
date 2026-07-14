//go:build unix

package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_ShadowrunCyberwareCluster proves the cyberware-CLUSTER path
// (item-modification.md — the capacity rule as a third host domain): a cybereye
// is a capacity host, an ENHANCEMENT (mod_host: cybereye) installs into its
// capacity with `modify`, and while the eyes are worn the enhancement's effect
// stacks on top of the shell's — the whole item-modification machinery reused,
// zero engine changes. Cluster enhancements are essence-free (capacity-gated).
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_ShadowrunCyberwareCluster -v
func TestLive_ShadowrunCyberwareCluster(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "shadowrun",
		"ANOTHERMUD_START_ROOM": "shadowrun:street-corner",
		"ANOTHERMUD_ROLE_SEED":  "Runner:admin",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := createAndLogin(c, "Runner"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	send := func(line string) string {
		t.Helper()
		if err := c.SendLine(line); err != nil {
			t.Fatalf("send %q: %v", line, err)
		}
		out, err := c.ExpectTimeout(gamePrompt, 8*time.Second)
		if err != nil {
			t.Fatalf("no prompt after %q: %v", line, err)
		}
		return out
	}
	intRe := regexp.MustCompile(`(?i)INT\s+(\d+)`)
	intuition := func(sheet string) int {
		t.Helper()
		m := intRe.FindStringSubmatch(sheet)
		if m == nil {
			t.Fatalf("no INT attribute on the score sheet:\n%s", sheet)
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}

	base := intuition(send("score"))

	// Assemble the cluster: a cybereye shell (capacity 4) + a vision-enhancement
	// chip (cost 2), installed into the eye's capacity BEFORE the eyes go in.
	send("spawn item cybereyes me")
	send("spawn item cybereye-vision-enhancement me")

	if out := send("modify cybereyes vision"); !strings.Contains(strings.ToLower(out), "install") {
		t.Fatalf("could not install the vision enhancement into the cybereyes:\n%s", out)
	}
	// The capacity budget is spent (2 of 4).
	if out := send("modify cybereyes"); !strings.Contains(out, "capacity 4") || !strings.Contains(out, "2 free") {
		t.Fatalf("cybereye capacity not accounted after install:\n%s", out)
	}

	// Install the eyes: the shell's +1 Intuition AND the enhancement's +1 both
	// apply while worn — INT rises by 2 over base.
	if out := send("equip cybereyes"); !strings.Contains(strings.ToLower(out), "cybereyes") {
		t.Fatalf("equip cybereyes did not confirm:\n%s", out)
	}
	if got := intuition(send("score")); got != base+2 {
		t.Fatalf("cluster INT wrong: INT %d after eyes + enhancement, want %d (base %d + shell 1 + chip 1)", got, base+2, base)
	}

	// Removing the enhancement (while the eyes are worn) drops the extra +1 live.
	if out := send("unmodify cybereyes vision"); !strings.Contains(strings.ToLower(out), "pocket") {
		t.Fatalf("could not remove the enhancement from the worn cybereyes:\n%s", out)
	}
	if got := intuition(send("score")); got != base+1 {
		t.Fatalf("removing the chip while worn did not drop INT live: INT %d, want %d (base + shell 1)", got, base+1)
	}
}

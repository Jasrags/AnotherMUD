//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_InitiateWilderSplit is the WoT S2 Phase 4 regression test for the
// Initiate/Wilder class split. It boots its own engine subprocess on the WoT
// pack, creates one Initiate and one Wilder with IDENTICAL wizard answers except
// the class, and reads the Fortitude save off each score sheet:
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_InitiateWilderSplit -v
//
// The proof is the split's signature mechanical asymmetry: a Wilder has a STRONG
// Fortitude save (the translation of d20's "wilders are more practiced at
// overchanneling"), an Initiate a WEAK one. At level 1 the strong curve is +2 and
// the weak curve +0; everything else about the two characters is identical
// (same gender → same race/stat mods, default stats, no roll), so the CON
// modifier cancels and the Wilder's Fort is exactly 2 higher than the Initiate's.
// That gap is RNG-free and dice-proof — it can only come from the divergent
// save_progression the split introduced.
func TestLive_InitiateWilderSplit(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "wot",
		"ANOTHERMUD_START_ROOM": "wot:the-green",
	})

	// Initiate — White-Tower-trained, brittle: WEAK Fortitude.
	ci, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial initiate: %v", err)
	}
	defer ci.Close()
	if err := createChannelerClass(ci, "Initf", "male", "initiate"); err != nil {
		t.Fatalf("create initiate: %v", err)
	}
	initiateFort, err := scoreFortitude(ci)
	if err != nil {
		t.Fatalf("initiate Fortitude: %v", err)
	}

	// Wilder — self-taught, hardy: STRONG Fortitude.
	cw, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial wilder: %v", err)
	}
	defer cw.Close()
	if err := createChannelerClass(cw, "Wildf", "male", "wilder"); err != nil {
		t.Fatalf("create wilder: %v", err)
	}
	wilderFort, err := scoreFortitude(cw)
	if err != nil {
		t.Fatalf("wilder Fortitude: %v", err)
	}

	// The direction is the essential invariant — a Wilder must resist overchannel
	// backlash (a Fort save) better than an Initiate, always.
	if wilderFort <= initiateFort {
		t.Errorf("wilder Fort (%+d) not greater than initiate Fort (%+d) — the Initiate/Wilder save split did not bite",
			wilderFort, initiateFort)
	}
	// At a fresh level-1 character the strong/weak curve gap is exactly 2.
	if got := wilderFort - initiateFort; got != 2 {
		t.Errorf("wilder Fort − initiate Fort = %d, want 2 (strong +2 vs weak +0 at level 1, identical CON)", got)
	}
	t.Logf("Initiate/Wilder split verified: wilder Fort %+d (strong) > initiate Fort %+d (weak)", wilderFort, initiateFort)
}

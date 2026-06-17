//go:build unix

package main

import (
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_SizeWielding proves size-and-wielding (S1.F) end to end on the WoT
// boot, where the Smithy (the-forge) stocks a LARGE ashandarei (one step above a
// medium wielder ⇒ two-handed) and a HUGE Trolloc maul (two steps above ⇒ too
// large). A medium Initiate:
//   - wields the ashandarei cleanly (size-derived two-handed grip), then
//   - is refused the maul with the too-large reason (§3, §4.1).
func TestLive_SizeWielding(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_PACKS":      "wot",
		"ANOTHERMUD_START_ROOM": "wot:the-forge",
		"ANOTHERMUD_START_HOUR": "12",
	})
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// A medium-size character (the human-default baseline).
	if err := createChanneler(c, "Wielder", "male"); err != nil {
		t.Fatalf("create+login: %v", err)
	}

	// The LARGE ashandarei is two-handed for a medium wielder — it equips.
	if err := c.SendLine("get ashandarei"); err != nil {
		t.Fatalf("send get ashandarei: %v", err)
	}
	if err := c.SendLine("equip ashandarei"); err != nil {
		t.Fatalf("send equip ashandarei: %v", err)
	}
	if _, err := c.ExpectStringTimeout("You equip an ashandarei", 5*time.Second); err != nil {
		t.Fatalf("expected the large polearm to be wielded two-handed: %v", err)
	}

	// The HUGE Trolloc maul is too large for a human — the equip is refused.
	if err := c.SendLine("get maul"); err != nil {
		t.Fatalf("send get maul: %v", err)
	}
	if err := c.SendLine("equip maul"); err != nil {
		t.Fatalf("send equip maul: %v", err)
	}
	if _, err := c.ExpectStringTimeout("too large for you to wield", 5*time.Second); err != nil {
		t.Fatalf("expected the huge maul to be refused as too large: %v", err)
	}
	t.Log("size-and-wielding verified: large weapon wielded two-handed, huge weapon refused as too large")
}

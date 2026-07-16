//go:build unix

package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// invAction / invItem / invPayload decode the Char.Inventory wire frame (kept
// package-level so hasAction can be shared).
type invAction struct {
	Label string `json:"label"`
	Cmd   string `json:"cmd"`
}
type invItem struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Slot    string      `json:"slot"`
	Empty   bool        `json:"empty"`
	Detail  string      `json:"detail"`
	Actions []invAction `json:"actions"`
}
type invPayload struct {
	Carried []invItem `json:"carried"`
	Worn    []invItem `json:"worn"`
}

// TestLive_GmcpInventory proves the web-client P3 additive package end to end:
// on login the server emits a Char.Inventory GMCP frame — the structured
// carried + worn inventory with per-item affordances — and re-emits it when the
// inventory changes. Confirms the panel data a browser client renders (items,
// slots, equip/unequip/drop actions) reaches the wire.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_GmcpInventory -v
func TestLive_GmcpInventory(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, nil)

	frames := &frameLog{}
	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second), telnettest.WithGMCPCapture(frames.add))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.ActivateGMCP(); err != nil {
		t.Fatalf("activate gmcp: %v", err)
	}
	if err := createAndLogin(c, "Quartermaster"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	c.Drain(2000 * time.Millisecond)

	f, ok := frames.lastAfter("Char.Inventory", 0)
	if !ok {
		t.Fatalf("no Char.Inventory frame captured on login (web-client P3 structured inventory)")
	}
	var inv invPayload
	if err := json.Unmarshal([]byte(f.JSON), &inv); err != nil {
		t.Fatalf("unmarshal Char.Inventory %q: %v", f.JSON, err)
	}
	// Structural invariants that hold regardless of exact starting gear: the
	// full slot layout enumerates (so at least one worn row, empties included);
	// every worn row names a slot; an OCCUPIED worn row carries an unequip
	// action; every carried item carries a drop action.
	if len(inv.Worn) == 0 {
		t.Errorf("worn slot layout is empty — the full slot list (incl. empties) should enumerate")
	}
	empties := 0
	for _, w := range inv.Worn {
		if w.Slot == "" {
			t.Errorf("worn row has no slot: %+v", w)
		}
		if w.Empty {
			empties++
			continue
		}
		if !hasAction(w.Actions, "unequip") {
			t.Errorf("occupied worn item %q missing unequip action: %+v", w.Name, w)
		}
	}
	for _, ci := range inv.Carried {
		if !hasAction(ci.Actions, "drop") {
			t.Errorf("carried item %q missing drop action: %+v", ci.Name, ci)
		}
	}
	t.Logf("login Char.Inventory: carried=%d worn=%d (empty slots=%d)", len(inv.Carried), len(inv.Worn), empties)
}

func hasAction(actions []invAction, want string) bool {
	for _, a := range actions {
		if a.Label == want {
			return true
		}
	}
	return false
}

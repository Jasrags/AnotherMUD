//go:build unix

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// recipeIngredient / recipeRow / recipesPayload decode the Char.Recipes wire
// frame (web-client-plan P3 Slice B — the craft form).
type recipeIngredient struct {
	Name string `json:"name"`
	Need int    `json:"need"`
	Have int    `json:"have"`
}
type recipeRow struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Ingredients []recipeIngredient `json:"ingredients"`
	StationMet  bool               `json:"stationMet"`
	SkillMet    bool               `json:"skillMet"`
	Craftable   bool               `json:"craftable"`
	Blocked     string             `json:"blocked"`
	Cmd         string             `json:"cmd"`
}
type recipesPayload struct {
	Recipes []recipeRow `json:"recipes"`
}

// TestLive_GmcpRecipes proves the web-client P3 Slice B craft-form package
// reaches the wire: on login the server emits a Char.Recipes GMCP frame (the
// additive craft form). Every row carries a `craft <recipe>` submit command and
// a self-consistent craftable flag, and a blocked row carries a reason — the
// data a browser client renders an interactive craft panel from.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_GmcpRecipes -v
func TestLive_GmcpRecipes(t *testing.T) {
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
	if err := createAndLogin(c, "Fletcher"); err != nil {
		t.Fatalf("create+login: %v", err)
	}
	c.Drain(2000 * time.Millisecond)

	f, ok := frames.lastAfter("Char.Recipes", 0)
	if !ok {
		t.Fatalf("no Char.Recipes frame captured on login (web-client P3 Slice B craft form)")
	}
	var form recipesPayload
	if err := json.Unmarshal([]byte(f.JSON), &form); err != nil {
		t.Fatalf("unmarshal Char.Recipes %q: %v", f.JSON, err)
	}

	// Structural invariants that hold regardless of what the starting character
	// knows (an empty form is valid for a non-crafter): every row carries a
	// `craft <recipe>` submit command, craftable ⇒ no block reason and all gates
	// met, and every ingredient has need >= 1.
	for _, r := range form.Recipes {
		if !strings.HasPrefix(r.Cmd, "craft ") {
			t.Errorf("recipe %q cmd = %q, want a `craft <recipe>` command", r.Name, r.Cmd)
		}
		if r.Craftable {
			if r.Blocked != "" {
				t.Errorf("craftable recipe %q carries a block reason %q", r.Name, r.Blocked)
			}
			if !r.SkillMet || !r.StationMet {
				t.Errorf("craftable recipe %q has an unmet gate: skill=%v station=%v", r.Name, r.SkillMet, r.StationMet)
			}
		} else if r.Blocked == "" {
			t.Errorf("blocked recipe %q carries no reason", r.Name)
		}
		for _, ing := range r.Ingredients {
			if ing.Need < 1 {
				t.Errorf("recipe %q ingredient %q need = %d, want >= 1", r.Name, ing.Name, ing.Need)
			}
		}
	}
	t.Logf("login Char.Recipes: %d known recipe(s) on the wire", len(form.Recipes))
}

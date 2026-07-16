//go:build unix

package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/AnotherMUD/internal/telnettest"
)

// TestLive_CoreWalkthrough drives a single default-boot (starter-world) engine
// through the bulk of the deterministic PLAYTEST.md sections on one admin
// connection. It is a breadth smoke: each sub-step round-trips a real command
// and asserts a distinctive cue from PLAYTEST.md (the engine's own quoted
// output), proving the verb works end to end against a live boot. RNG/timing-
// heavy paths (combat outcomes, lockpicking success) live in their own tests.
//
//	ANOTHERMUD_LIVE=1 go test ./cmd/telnet-smoke -run TestLive_CoreWalkthrough -v
//
// The character is seeded admin (ANOTHERMUD_ROLE_SEED) so the §17 admin verbs,
// the §0 xp/restore bootstrap, AND `teleport` (used to self-locate each
// subtest, so they don't depend on each other's final room) are reachable.
//
// NOTE on the name: character names are letters-only (§2), so the seed/char is
// "Adminone", not PLAYTEST's "admin1" — see the BUG note in the run report;
// "admin1" is a valid ACCOUNT username but an invalid CHARACTER name, yet the
// role seed keys on the character name.
func TestLive_CoreWalkthrough(t *testing.T) {
	if os.Getenv("ANOTHERMUD_LIVE") == "" {
		t.Skip("set ANOTHERMUD_LIVE=1 to run (boots a real engine subprocess via `go run`)")
	}
	addr := bootEngine(t, map[string]string{
		"ANOTHERMUD_ROLE_SEED":                 "Adminone:admin",
		"ANOTHERMUD_SUSTENANCE_DRAIN_INTERVAL": "5m", // keep sustenance steady during the run
	})

	c, err := telnettest.Dial(addr, telnettest.WithTimeout(12*time.Second))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// Create a HUMAN (not the wizard's default Dwarf): a dwarf has darkvision and
	// would see the black Forge Vault at gloom, masking the §21 "pitch black"
	// render this walkthrough asserts.
	isNew, err := doLogin(c, "Adminone")
	if err != nil {
		t.Fatalf("login admin: %v", err)
	}
	if err := finishLogin(c, "Adminone", isNew, map[string]string{"race": "human"}); err != nil {
		t.Fatalf("create admin character: %v", err)
	}

	// cmd sends a line and returns everything up to the next in-game prompt.
	cmd := func(line string) string {
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
	// want asserts every cue is present in out; otherwise flags a PLAYTEST BUG.
	want := func(label, out string, cues ...string) {
		t.Helper()
		lo := strings.ToLower(out)
		for _, cue := range cues {
			if cue == "" {
				continue
			}
			if !strings.Contains(lo, strings.ToLower(cue)) {
				t.Errorf("PLAYTEST FAIL [%s]: missing %q in:\n%s", label, cue, out)
			}
		}
	}
	noHuh := func(label, out string) {
		t.Helper()
		if strings.Contains(strings.ToLower(out), "huh?") {
			t.Errorf("PLAYTEST FAIL [%s]: verb not recognized:\n%s", label, out)
		}
	}
	at := func(room string) { cmd("teleport " + room) }

	// §2 Creation — the score sheet shows the chosen identity + the save lines.
	t.Run("score-identity-and-saves", func(t *testing.T) {
		out := cmd("score")
		want("§2 score identity", out, "Fighter")
		want("§24 saves row", out, "Fort", "Ref", "Will")
		want("§28 MV pool", out, "MV")
	})

	// §0 bootstrap — bank XP so later level-gated probes have headroom.
	t.Run("admin-xp-bootstrap", func(t *testing.T) {
		noHuh("§17 xp", cmd("xp 5000"))
	})

	// §3 Movement & rooms.
	t.Run("movement", func(t *testing.T) {
		at("town-square")
		out := cmd("north")
		want("§3 north→forge", out, "Forge")
		want("§3 forge trainer", out, "Maerys")
		cmd("south")
		want("§3 no-exit", cmd("up"), "cannot go that way")
		want("§3 chaining", cmd("n;s"), "Forge")
	})

	// §4 Items & inventory (ground items are in Town Square).
	t.Run("items", func(t *testing.T) {
		at("town-square")
		want("§4 get sword", cmd("get sword"), "sword")
		want("§4 inventory", cmd("inventory"), "sword")
		noHuh("§4 equip", cmd("equip sword"))
		noHuh("§4 look sword", cmd("look sword"))
		noHuh("§4 equipment", cmd("equipment"))
		cmd("get coins")
		want("§4 coins→gold", cmd("gold"), "gold")
	})

	// §5 Containers.
	t.Run("containers", func(t *testing.T) {
		at("town-square")
		want("§5 get sack", cmd("get sack"), "sack")
		cmd("unequip sword")
		noHuh("§5 put in sack", cmd("put sword in sack"))
		want("§5 look in sack", cmd("look in sack"), "sword")
		noHuh("§5 get from sack", cmd("get sword from sack"))
		want("§5 get waterskin", cmd("get waterskin"), "waterskin")
		noHuh("§5 fill", cmd("fill waterskin from well"))
	})

	// §8 Progression & abilities.
	t.Run("progression", func(t *testing.T) {
		noHuh("§8 abilities", cmd("abilities"))
		noHuh("§8 train", cmd("train str"))
	})

	// §9 Economy & survival (Market Row).
	t.Run("economy", func(t *testing.T) {
		at("market")
		noHuh("§9 list", cmd("list"))
		noHuh("§9 buy", cmd("buy healing draught"))
		noHuh("§9 drink", cmd("drink waterskin"))
		noHuh("§9 rest", cmd("rest"))
		cmd("stand")
	})

	// §13 Recall.
	t.Run("recall", func(t *testing.T) {
		at("town-square")
		noHuh("§13 recall set", cmd("recall set"))
		at("meadow")
		noHuh("§13 recall", cmd("recall"))
	})

	// §10 Quests — the Forge Errand auto-grant loop. Maerys the training-master
	// stands in the training yard (west of the square), not the forge.
	t.Run("quests", func(t *testing.T) {
		at("training-yard")
		want("§10 talk master", cmd("talk master"), "Forge Errand")
		noHuh("§10 accept", cmd("accept Forge Errand"))
		want("§10 journal", cmd("quests"), "Forge Errand")
		at("town-square")
		cmd("drop ration") // ensure not already holding one
		want("§10 auto-grant complete", cmd("get ration"), "Forge Errand")
	})

	// §31 Feats — take one and confirm it lands on the score sheet.
	t.Run("feats", func(t *testing.T) {
		noHuh("§31 feats", cmd("feats"))
		before := fortValue(t, cmd("score"))
		noHuh("§31 feat take", cmd("feat great-fortitude"))
		after := fortValue(t, cmd("score"))
		if after <= before {
			t.Errorf("PLAYTEST FAIL [§31 great-fortitude]: Fort did not rise (%d → %d)", before, after)
		}
	})

	// §25 Conditions — admin afflict/affects/cure on self.
	t.Run("conditions", func(t *testing.T) {
		want("§25 afflict", cmd("afflict Adminone stunned"), "stun")
		want("§25 affects", cmd("affects"), "stun")
		cmd("cure Adminone")
	})

	// §26 Skills listing (lockpicking success is its own test).
	t.Run("skills", func(t *testing.T) {
		want("§26 skills list", cmd("skills"), "Open Lock")
	})

	// §22 Crafting — learn a discipline at the forge / market.
	t.Run("crafting", func(t *testing.T) {
		at("forge")
		want("§22 learn smithing", cmd("learn smithing"), "Smithing")
		noHuh("§22 craft list", cmd("craft"))
		at("market")
		want("§22 learn cooking", cmd("learn cooking"), "Cooking")
	})

	// §23 Player maps.
	t.Run("maps", func(t *testing.T) {
		noHuh("§23 map", cmd("map"))
		noHuh("§23 minimap on", cmd("minimap on"))
		cmd("minimap off")
	})

	// §15 Help & UI.
	t.Run("help-ui", func(t *testing.T) {
		noHuh("§15 help", cmd("help"))
		noHuh("§15 help get", cmd("help get"))
		cmd("prompt default")
		noHuh("§15 color off", cmd("color off"))
		cmd("color on")
	})

	// §20 Tab-completion surfaces (suggest = anyone, complete = admin).
	t.Run("tab-completion", func(t *testing.T) {
		at("town-square")
		want("§20 suggest get s", cmd("suggest get s"), "sword")
		want("§20 complete loo", cmd("complete loo"), "look")
	})

	// §17 Admin verbs.
	t.Run("admin-verbs", func(t *testing.T) {
		noHuh("§17 restore", cmd("restore"))
		want("§17 teleport", cmd("teleport meadow"), "Meadow")
		at("town-square")
		want("§17 announce", cmd("announce Smoke test in progress"), "Smoke test")
		want("§17 reload count", cmd("reload"), "Reloaded")
		cmd("xyzzy")
		cmd("xyzzy")
		want("§17 badinput", cmd("badinput"), "xyzzy")
		cmd("badinput clear")
	})

	// §12 + §21 Doors, locks, light — the cellar/vault branch below the forge.
	t.Run("doors-and-light", func(t *testing.T) {
		at("forge")
		want("§12 open oak", cmd("open down"), "open")
		want("§12 descend", cmd("down"), "Cellar")
		want("§12 get key", cmd("get key"), "key")
		noHuh("§12 unlock iron", cmd("unlock down"))
		cmd("open down")
		want("§21 pitch black", cmd("down"), "pitch black")
		want("§21 escape up", cmd("up"), "Cellar")
	})
}

// fortValue extracts the Fortitude save integer from a score sheet capture.
func fortValue(t *testing.T, sheet string) int {
	t.Helper()
	m := regexp.MustCompile(`Fort\s*([+-]?\d+)`).FindStringSubmatch(sheet)
	if m == nil {
		t.Fatalf("no Fortitude value in score sheet:\n%s", sheet)
	}
	return atoiSigned(m[1])
}

func atoiSigned(s string) int {
	neg := false
	n := 0
	for _, r := range s {
		switch {
		case r == '+':
		case r == '-':
			neg = true
		case r >= '0' && r <= '9':
			n = n*10 + int(r-'0')
		}
	}
	if neg {
		return -n
	}
	return n
}

package login

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/account"
)

// character-identity §3/§5: world stamping at creation + the active-world gate.

func TestWorldOf(t *testing.T) {
	cases := map[string]string{
		"starter-world:town-square": "starter-world",
		"wot:the-green":             "wot",
		"town-square":               "", // no namespace
		"":                          "",
		"a:b:c":                     "a", // first segment only (Cut on first ':')
	}
	for in, want := range cases {
		if got := worldOf(in); got != want {
			t.Errorf("worldOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConfigWorldActive(t *testing.T) {
	// Empty active set disables the gate — every world passes.
	empty := Config{}
	for _, w := range []string{"wot", "starter-world", ""} {
		if !empty.worldActive(w) {
			t.Errorf("empty ActiveWorlds: worldActive(%q) = false, want true (gate disabled)", w)
		}
	}
	// A configured set admits only its members.
	cfg := Config{ActiveWorlds: []string{"starter-world"}}
	if !cfg.worldActive("starter-world") {
		t.Error("worldActive(starter-world) = false, want true (in set)")
	}
	if cfg.worldActive("wot") {
		t.Error("worldActive(wot) = true, want false (not in set)")
	}
}

func TestBuildNewCharacterStampsWorldID(t *testing.T) {
	ctx := context.Background()
	acc := &account.Account{ID: "acct-1"}

	loaded, err := buildNewCharacter(ctx, nil, Config{DefaultLocation: "wot:the-green"}, acc, "Rand")
	if err != nil {
		t.Fatalf("buildNewCharacter: %v", err)
	}
	if loaded.Player.WorldID != "wot" {
		t.Errorf("WorldID = %q, want wot (from DefaultLocation namespace)", loaded.Player.WorldID)
	}

	// A start location with no namespace leaves WorldID empty (test configs).
	loaded2, err := buildNewCharacter(ctx, nil, Config{DefaultLocation: "limbo"}, acc, "Nyn")
	if err != nil {
		t.Fatalf("buildNewCharacter: %v", err)
	}
	if loaded2.Player.WorldID != "" {
		t.Errorf("WorldID = %q, want empty (no namespace on DefaultLocation)", loaded2.Player.WorldID)
	}
}

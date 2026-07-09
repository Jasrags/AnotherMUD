package login

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/account"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// TestPartitionRoster: in-world and broken-save entries stay in the shown list;
// out-of-world entries (loaded fine, world not active) are hidden and counted.
func TestPartitionRoster(t *testing.T) {
	all := []rosterEntry{
		{name: "Ina", world: "alpha", available: true},   // in-world → shown
		{name: "Otto", world: "beta", available: false},  // out-of-world → hidden+counted
		{name: "Broke", world: "", available: false},     // load failed → shown (visible)
		{name: "Nora", world: "", available: true},       // worldless → shown
		{name: "Elsa", world: "gamma", available: false}, // out-of-world → hidden+counted
	}

	shown, other := partitionRoster(all)

	if other != 2 {
		t.Errorf("otherWorld = %d, want 2", other)
	}
	got := make([]string, len(shown))
	for i, e := range shown {
		got[i] = e.name
	}
	want := []string{"Ina", "Broke", "Nora"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("shown = %v, want %v", got, want)
	}
}

// worldRig seeds one account with two characters, one per world, and returns a
// Config whose active-world set contains only `activeWorld` — so the other
// character is out-of-world.
func worldRig(t *testing.T, activeWorld string) (Config, string) {
	t.Helper()
	store, err := player.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	accts, err := account.NewService(t.TempDir(), account.WithBcryptCost(account.MinBcryptCostForTests))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx := context.Background()
	acc, err := accts.CreateWithUsername(ctx, "bob", "", "secret123")
	if err != nil {
		t.Fatalf("CreateWithUsername: %v", err)
	}
	for _, c := range []struct{ name, world string }{{"Ina", "alpha"}, {"Otto", "beta"}} {
		if err := accts.AddCharacter(ctx, acc.ID, c.name); err != nil {
			t.Fatalf("AddCharacter %s: %v", c.name, err)
		}
		if err := store.Save(ctx, &player.Save{Name: c.name, WorldID: c.world}); err != nil {
			t.Fatalf("Save %s: %v", c.name, err)
		}
	}
	return Config{Players: store, Accounts: accts, ActiveWorlds: []string{activeWorld}}, acc.ID
}

// TestRoster_HidesOutOfWorld_ShowsFootnote: on an alpha boot, the roster lists
// only the alpha character, omits the beta one, and footnotes the hidden count.
func TestRoster_HidesOutOfWorld_ShowsFootnote(t *testing.T) {
	cfg, _ := worldRig(t, "alpha")
	conn := &scriptConn{lines: []string{"bob", "secret123", "q"}}

	if _, err := Run(context.Background(), conn, cfg); !errors.Is(err, ErrQuit) {
		t.Fatalf("Run err = %v, want ErrQuit", err)
	}
	out := conn.output()
	if !strings.Contains(out, "Ina") {
		t.Errorf("roster omitted the in-world character Ina:\n%s", out)
	}
	if strings.Contains(out, "Otto") {
		t.Errorf("roster listed the out-of-world character Otto (should be hidden):\n%s", out)
	}
	if !strings.Contains(out, "You also have 1 character in other worlds") {
		t.Errorf("roster missing the out-of-world footnote:\n%s", out)
	}
}

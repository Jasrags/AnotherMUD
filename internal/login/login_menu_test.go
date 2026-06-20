package login

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/account"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

// menuRig seeds an account with one character and returns a Config wired to
// fresh stores. The character has an empty WorldID so it is always available
// (the world gate is disabled when ActiveWorlds is empty).
func menuRig(t *testing.T) (cfg Config, accID string) {
	t.Helper()
	store, err := player.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	accts, err := account.NewService(t.TempDir(), account.WithBcryptCost(account.MinBcryptCostForTests))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	acc, err := accts.CreateWithUsername(context.Background(), "bob", "", "secret123")
	if err != nil {
		t.Fatalf("CreateWithUsername: %v", err)
	}
	if err := accts.AddCharacter(context.Background(), acc.ID, "Bob"); err != nil {
		t.Fatalf("AddCharacter: %v", err)
	}
	if err := store.Save(context.Background(), &player.Save{Name: "Bob"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return Config{Players: store, Accounts: accts}, acc.ID
}

// TestCharacterMenu_Enter: selecting a character then "1) Enter the game"
// returns the loaded character (character-select §8).
func TestCharacterMenu_Enter(t *testing.T) {
	cfg, _ := menuRig(t)
	conn := &scriptConn{lines: []string{"bob", "secret123", "Bob", "1"}}

	loaded, err := Run(context.Background(), conn, cfg)
	if err != nil {
		t.Fatalf("Run err = %v, want nil", err)
	}
	if loaded == nil || loaded.Player == nil || loaded.Player.Name != "Bob" {
		t.Fatalf("Run loaded = %+v, want character Bob", loaded)
	}
	if loaded.New {
		t.Errorf("loaded.New = true, want false (returning character)")
	}
}

// TestCharacterMenu_Delete: "2) Delete" + name confirmation hard-deletes the
// save and unlinks it from the account; the now-empty roster routes to create,
// where EOF aborts (character-select §8 roster operations).
func TestCharacterMenu_Delete(t *testing.T) {
	cfg, accID := menuRig(t)
	conn := &scriptConn{lines: []string{"bob", "secret123", "Bob", "2", "Bob"}}

	loaded, err := Run(context.Background(), conn, cfg)
	if loaded != nil {
		t.Fatalf("Run loaded = %+v, want nil after delete→create→EOF", loaded)
	}
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("Run err = %v, want ErrAborted (EOF at the create prompt)", err)
	}
	if cfg.Players.Exists("Bob") {
		t.Errorf("character save still exists after delete")
	}
	acc, lerr := cfg.Accounts.LoadByID(context.Background(), accID)
	if lerr != nil {
		t.Fatalf("LoadByID: %v", lerr)
	}
	if len(acc.Characters) != 0 {
		t.Errorf("account still lists characters %v after delete", acc.Characters)
	}
	if !strings.Contains(conn.output(), "has been deleted") {
		t.Errorf("missing deletion confirmation in output: %q", conn.output())
	}
}

// TestCharacterMenu_DeleteCancelled: a non-matching confirmation cancels the
// delete; the character survives. Back (0) then quit (q) exits cleanly.
func TestCharacterMenu_DeleteCancelled(t *testing.T) {
	cfg, _ := menuRig(t)
	conn := &scriptConn{lines: []string{"bob", "secret123", "Bob", "2", "notbob", "0", "q"}}

	_, err := Run(context.Background(), conn, cfg)
	if !errors.Is(err, ErrQuit) {
		t.Fatalf("Run err = %v, want ErrQuit", err)
	}
	if !cfg.Players.Exists("Bob") {
		t.Errorf("character was deleted despite a cancelled confirmation")
	}
	if !strings.Contains(strings.ToLower(conn.output()), "cancelled") {
		t.Errorf("missing cancellation message in output: %q", conn.output())
	}
}

// TestRoster_Quit: "q" at the roster is a clean ErrQuit, no character loaded.
func TestRoster_Quit(t *testing.T) {
	cfg, _ := menuRig(t)
	conn := &scriptConn{lines: []string{"bob", "secret123", "q"}}

	loaded, err := Run(context.Background(), conn, cfg)
	if loaded != nil {
		t.Fatalf("Run loaded = %+v, want nil on quit", loaded)
	}
	if !errors.Is(err, ErrQuit) {
		t.Fatalf("Run err = %v, want ErrQuit", err)
	}
}

// TestRoster_ChangePassword: the roster-level "p" action re-verifies the
// current password and applies the new one (character-select §8).
func TestRoster_ChangePassword(t *testing.T) {
	cfg, _ := menuRig(t)
	conn := &scriptConn{lines: []string{
		"bob", "secret123", // auth
		"p",                      // change password
		"secret123",              // current
		"newsecret", "newsecret", // new + confirm
		"q", // quit
	}}

	if _, err := Run(context.Background(), conn, cfg); !errors.Is(err, ErrQuit) {
		t.Fatalf("Run err = %v, want ErrQuit", err)
	}
	if !strings.Contains(conn.output(), "Password changed.") {
		t.Errorf("missing 'Password changed.' in output: %q", conn.output())
	}
	// The new password authenticates; the old one no longer does.
	if _, err := cfg.Accounts.AuthenticateByUsername(context.Background(), "bob", "newsecret"); err != nil {
		t.Errorf("new password failed to authenticate: %v", err)
	}
	if _, err := cfg.Accounts.AuthenticateByUsername(context.Background(), "bob", "secret123"); !errors.Is(err, account.ErrAuthFailed) {
		t.Errorf("old password still authenticates: err=%v", err)
	}
}

// TestRoster_ChangePassword_WrongCurrent: a wrong current password leaves the
// credential unchanged and stays on the roster (soft failure).
func TestRoster_ChangePassword_WrongCurrent(t *testing.T) {
	cfg, _ := menuRig(t)
	conn := &scriptConn{lines: []string{
		"bob", "secret123",
		"p",
		"wrongpw",                // wrong current
		"newsecret", "newsecret", // never reached for the change
		"q",
	}}

	if _, err := Run(context.Background(), conn, cfg); !errors.Is(err, ErrQuit) {
		t.Fatalf("Run err = %v, want ErrQuit", err)
	}
	if !strings.Contains(strings.ToLower(conn.output()), "incorrect") {
		t.Errorf("missing 'incorrect' message: %q", conn.output())
	}
	if _, err := cfg.Accounts.AuthenticateByUsername(context.Background(), "bob", "secret123"); err != nil {
		t.Errorf("original password broke after a failed change: %v", err)
	}
}

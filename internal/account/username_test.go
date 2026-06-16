package account_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/account"
)

// character-select §2: account login by username — creation, uniqueness,
// auth, the email-derived legacy path, and the pre-username backfill.

func TestValidUsername(t *testing.T) {
	for _, ok := range []string{"rand", "Mat_C", "perrin99", "abc"} {
		if !account.ValidUsername(account.NormalizeUsername(ok)) {
			t.Errorf("ValidUsername(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "ab", "has space", "bad!", "with@at"} {
		if account.ValidUsername(account.NormalizeUsername(bad)) {
			t.Errorf("ValidUsername(%q) = true, want false", bad)
		}
	}
}

func TestCreateWithUsername_AuthByUsername(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	acc, err := svc.CreateWithUsername(ctx, "Rand", "rand@example.com", "pw")
	if err != nil {
		t.Fatalf("CreateWithUsername: %v", err)
	}
	if acc.Username != "rand" {
		t.Errorf("Username = %q, want rand (normalized)", acc.Username)
	}
	// Auth by username (case-insensitive) succeeds.
	got, err := svc.AuthenticateByUsername(ctx, "RAND", "pw")
	if err != nil || got.ID != acc.ID {
		t.Fatalf("AuthenticateByUsername = (%v, %v), want acct %s", got, err, acc.ID)
	}
	// Wrong password and unknown username both fail with ErrAuthFailed.
	if _, err := svc.AuthenticateByUsername(ctx, "rand", "nope"); !errors.Is(err, account.ErrAuthFailed) {
		t.Errorf("wrong pw err = %v, want ErrAuthFailed", err)
	}
	if _, err := svc.AuthenticateByUsername(ctx, "nobody", "pw"); !errors.Is(err, account.ErrAuthFailed) {
		t.Errorf("unknown username err = %v, want ErrAuthFailed", err)
	}
}

func TestCreateWithUsername_TakenAndEmailOptional(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if _, err := svc.CreateWithUsername(ctx, "mat", "", "pw"); err != nil {
		t.Fatalf("CreateWithUsername (no email): %v", err)
	}
	// Email is optional — the account exists and authenticates by username.
	if _, err := svc.AuthenticateByUsername(ctx, "mat", "pw"); err != nil {
		t.Fatalf("auth emailless account: %v", err)
	}
	// A duplicate username is rejected (case-insensitively).
	if _, err := svc.CreateWithUsername(ctx, "MAT", "other@example.com", "pw"); !errors.Is(err, account.ErrUsernameTaken) {
		t.Errorf("duplicate username err = %v, want ErrUsernameTaken", err)
	}
}

func TestCreate_DerivesUsernameFromEmail(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	acc, err := svc.Create(ctx, "Perrin@example.com", "pw")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if acc.Username != "perrin" {
		t.Errorf("derived Username = %q, want perrin (email local part)", acc.Username)
	}
	if !svc.UsernameExists("perrin") {
		t.Error("UsernameExists(perrin) = false after Create")
	}
	// A second account whose email local part collides gets a unique username.
	acc2, err := svc.Create(ctx, "perrin@other.test", "pw")
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if acc2.Username == acc.Username {
		t.Errorf("colliding derived usernames not de-duplicated: both %q", acc2.Username)
	}
}

// A pre-username account on disk (index has only email entries) is backfilled
// at NewService: every account gets a username derived from its email, and the
// username index is (re)built (character-select §2.1 migration).
func TestBackfillUsernames_FromRawPreUsernameAccount(t *testing.T) {
	root := t.TempDir()
	accDir := filepath.Join(root, "accounts")
	id := "acct-legacy-1"
	if err := os.MkdirAll(filepath.Join(accDir, id), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A pre-username account file (no `username:`) + a pre-username index
	// (only `entries:`).
	if err := os.WriteFile(filepath.Join(accDir, id, "account.yaml"),
		[]byte("id: "+id+"\nemail: olduser@example.com\npassword_hash: x\n"), 0o600); err != nil {
		t.Fatalf("write account: %v", err)
	}
	if err := os.WriteFile(filepath.Join(accDir, "index.yaml"),
		[]byte("entries:\n  olduser@example.com: "+id+"\n"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}

	svc, err := account.NewService(root, account.WithBcryptCost(account.MinBcryptCostForTests))
	if err != nil {
		t.Fatalf("NewService (runs backfill): %v", err)
	}
	if !svc.UsernameExists("olduser") {
		t.Error("UsernameExists(olduser) = false; backfill did not index the derived username")
	}
	got, err := svc.LoadByID(context.Background(), id)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if got.Username != "olduser" {
		t.Errorf("backfilled Username = %q, want olduser", got.Username)
	}
}

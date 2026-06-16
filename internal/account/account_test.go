package account_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/account"
)

func newService(t *testing.T) *account.Service {
	t.Helper()
	svc, err := account.NewService(t.TempDir(), account.WithBcryptCost(account.MinBcryptCostForTests))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestCreate_PersistsAndNormalizesEmail(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	acc, err := svc.Create(ctx, " Alice@Example.COM ", "hunter2")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if acc.Email != "alice@example.com" {
		t.Errorf("email = %q, want normalized lowercased+trimmed", acc.Email)
	}
	if acc.ID == "" {
		t.Errorf("ID empty")
	}
	if acc.PasswordHash == "" || acc.PasswordHash == "hunter2" {
		t.Errorf("password hash leaked or empty: %q", acc.PasswordHash)
	}
}

func TestCreate_RejectsDuplicateEmail(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if _, err := svc.Create(ctx, "bob@example.com", "pw"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.Create(ctx, "BOB@example.com", "pw")
	if !errors.Is(err, account.ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
}

func TestAuthenticate_Success(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	created, err := svc.Create(ctx, "carol@example.com", "correct horse")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.AuthenticateByEmail(ctx, "Carol@example.com", "correct horse")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("id = %q, want %q", got.ID, created.ID)
	}
}

func TestAuthenticate_FailsOnWrongPassword(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	if _, err := svc.Create(ctx, "dave@example.com", "right"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := svc.AuthenticateByEmail(ctx, "dave@example.com", "wrong")
	if !errors.Is(err, account.ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed", err)
	}
}

func TestAuthenticate_UnknownEmailReturnsSameError(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	_, err := svc.AuthenticateByEmail(ctx, "nobody@example.com", "anything")
	if !errors.Is(err, account.ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed (uniform failure for unknown email)", err)
	}
}

func TestAddCharacter_IdempotentCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	acc, err := svc.Create(ctx, "eve@example.com", "pw")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.AddCharacter(ctx, acc.ID, "Eve"); err != nil {
		t.Fatalf("AddCharacter: %v", err)
	}
	if err := svc.AddCharacter(ctx, acc.ID, "eve"); err != nil {
		t.Fatalf("AddCharacter (case dup): %v", err)
	}

	got, err := svc.LoadByID(ctx, acc.ID)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if len(got.Characters) != 1 {
		t.Errorf("characters = %v, want 1 entry", got.Characters)
	}
}

// One account holds several distinct characters (the substrate behind a
// per-account roster split across worlds — character-identity).
func TestAddCharacter_MultipleDistinctPersist(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	acc, err := svc.Create(ctx, "house@example.com", "pw")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, name := range []string{"Rand", "Mat", "Perrin"} {
		if err := svc.AddCharacter(ctx, acc.ID, name); err != nil {
			t.Fatalf("AddCharacter(%s): %v", name, err)
		}
	}
	got, err := svc.LoadByID(ctx, acc.ID)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if len(got.Characters) != 3 {
		t.Fatalf("characters = %v, want 3 distinct entries", got.Characters)
	}
	for _, want := range []string{"Rand", "Mat", "Perrin"} {
		if !slices.Contains(got.Characters, want) {
			t.Errorf("characters %v missing %q", got.Characters, want)
		}
	}
}

func TestPersistence_SurvivesReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	svc1, err := account.NewService(dir, account.WithBcryptCost(account.MinBcryptCostForTests))
	if err != nil {
		t.Fatalf("NewService 1: %v", err)
	}
	created, err := svc1.Create(ctx, "frank@example.com", "pw")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc1.AddCharacter(ctx, created.ID, "Frank"); err != nil {
		t.Fatalf("AddCharacter: %v", err)
	}

	svc2, err := account.NewService(dir, account.WithBcryptCost(account.MinBcryptCostForTests))
	if err != nil {
		t.Fatalf("NewService 2: %v", err)
	}
	reopened, err := svc2.AuthenticateByEmail(ctx, "frank@example.com", "pw")
	if err != nil {
		t.Fatalf("Authenticate after reopen: %v", err)
	}
	if reopened.ID != created.ID || len(reopened.Characters) != 1 {
		t.Errorf("reopened = %+v", reopened)
	}
}

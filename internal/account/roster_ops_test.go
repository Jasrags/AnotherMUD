package account_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/account"
)

func TestRemoveCharacter(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	acc, err := svc.CreateWithUsername(ctx, "bob", "", "secret123")
	if err != nil {
		t.Fatalf("CreateWithUsername: %v", err)
	}
	for _, n := range []string{"Bob", "Bobby"} {
		if err := svc.AddCharacter(ctx, acc.ID, n); err != nil {
			t.Fatalf("AddCharacter %q: %v", n, err)
		}
	}

	// Case-insensitive removal of one character leaves the other.
	if err := svc.RemoveCharacter(ctx, acc.ID, "bob"); err != nil {
		t.Fatalf("RemoveCharacter: %v", err)
	}
	got, err := svc.LoadByID(ctx, acc.ID)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if len(got.Characters) != 1 || got.Characters[0] != "Bobby" {
		t.Errorf("characters = %v, want [Bobby]", got.Characters)
	}

	// Removing a name that isn't present is a no-op, not an error.
	if err := svc.RemoveCharacter(ctx, acc.ID, "Nobody"); err != nil {
		t.Errorf("RemoveCharacter(absent) = %v, want nil", err)
	}
}

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	acc, err := svc.CreateWithUsername(ctx, "bob", "", "secret123")
	if err != nil {
		t.Fatalf("CreateWithUsername: %v", err)
	}

	// Wrong current password is refused with ErrAuthFailed; credential intact.
	if err := svc.ChangePassword(ctx, acc.ID, "wrong", "newsecret"); !errors.Is(err, account.ErrAuthFailed) {
		t.Fatalf("ChangePassword(wrong current) = %v, want ErrAuthFailed", err)
	}
	if _, err := svc.AuthenticateByUsername(ctx, "bob", "secret123"); err != nil {
		t.Errorf("original password broke after a failed change: %v", err)
	}

	// Correct current password rotates the credential.
	if err := svc.ChangePassword(ctx, acc.ID, "secret123", "newsecret"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := svc.AuthenticateByUsername(ctx, "bob", "newsecret"); err != nil {
		t.Errorf("new password failed to authenticate: %v", err)
	}
	if _, err := svc.AuthenticateByUsername(ctx, "bob", "secret123"); !errors.Is(err, account.ErrAuthFailed) {
		t.Errorf("old password still authenticates after change")
	}
}

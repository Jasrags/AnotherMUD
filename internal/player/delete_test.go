package player_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

func TestDelete(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)

	if err := st.Save(ctx, &player.Save{Name: "Bob"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// A sibling file in the player dir (e.g. quest.yaml) must go too.
	sibling := filepath.Join(dir, "players", "bob", "quest.yaml")
	if err := os.WriteFile(sibling, []byte("x: 1\n"), 0o644); err != nil {
		t.Fatalf("write sibling: %v", err)
	}
	if !st.Exists("Bob") {
		t.Fatalf("precondition: character should exist")
	}

	if err := st.Delete(ctx, "Bob"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if st.Exists("Bob") {
		t.Errorf("character still exists after Delete")
	}
	if _, err := os.Stat(filepath.Join(dir, "players", "bob")); !os.IsNotExist(err) {
		t.Errorf("player dir survived Delete: stat err = %v", err)
	}

	// Deleting a non-existent record is idempotent (no error).
	if err := st.Delete(ctx, "Ghost"); err != nil {
		t.Errorf("Delete(absent) = %v, want nil", err)
	}
}

package session

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/login"
	"github.com/Jasrags/AnotherMUD/internal/player"
)

func newCreationStore(t *testing.T) *player.Store {
	t.Helper()
	st, err := player.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return st
}

// A fresh name commits: the character lands on disk and the save's dirty
// bit is cleared (the on-disk copy is now authoritative).
func TestCommitCreation_PersistsFreshCharacter(t *testing.T) {
	st := newCreationStore(t)
	cfg := Config{Players: st} // Login.Accounts nil → AddCharacter skipped

	a := &connActor{
		accountID: "acct-1",
		players:   st,
		dirty:     true,
		save: &player.Save{
			Version: player.CurrentVersion, ID: "p-1", AccountID: "acct-1",
			Name: "Fresh", Location: "tapestry-core:town-square",
		},
	}

	if err := commitCreation(context.Background(), cfg, a); err != nil {
		t.Fatalf("commitCreation: %v", err)
	}
	if !st.Exists("Fresh") {
		t.Error("character was not persisted")
	}
	a.mu.Lock()
	dirty := a.dirty
	a.mu.Unlock()
	if dirty {
		t.Error("dirty bit should be cleared after commit")
	}
}

// A name already on disk at commit time is the §6.4 last-chance
// conflict: ErrNameConflict, and nothing about the loser is written.
func TestCommitCreation_NameConflictRejected(t *testing.T) {
	st := newCreationStore(t)
	// Simulate a concurrent new-player who committed "Taken" first.
	if err := st.Save(context.Background(), &player.Save{
		Version: player.CurrentVersion, ID: "winner", AccountID: "acct-w",
		Name: "Taken", Location: "x:1",
	}); err != nil {
		t.Fatalf("seed winner: %v", err)
	}

	cfg := Config{Players: st, Login: login.Config{}}
	a := &connActor{
		accountID: "acct-loser",
		players:   st,
		save: &player.Save{
			Version: player.CurrentVersion, ID: "loser", AccountID: "acct-loser",
			Name: "Taken", Location: "x:1",
		},
	}

	err := commitCreation(context.Background(), cfg, a)
	if !errors.Is(err, ErrNameConflict) {
		t.Fatalf("err = %v, want ErrNameConflict", err)
	}
	// The winner's record must be untouched (loser id never written).
	got, loadErr := st.Load(context.Background(), "Taken")
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if got.ID != "winner" {
		t.Errorf("conflict commit clobbered the winner: id = %q, want winner", got.ID)
	}
}

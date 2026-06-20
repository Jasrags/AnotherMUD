package player_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// reputation.md §10: the renown score round-trips through save/load — both a
// fame (positive) and an infamy (negative) value, since renown is signed.
func TestSave_ReputationRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		renown int
	}{
		{"Lan", 850},    // famous
		{"Padan", -700}, // infamous
	} {
		save := &player.Save{
			Version: player.CurrentVersion,
			ID:      "p-" + tc.name, AccountID: "acct-" + tc.name, Name: tc.name,
			Reputation: tc.renown,
		}
		if err := st.Save(ctx, save); err != nil {
			t.Fatalf("Save(%s): %v", tc.name, err)
		}
		got, err := st.Load(ctx, strings.ToLower(tc.name))
		if err != nil {
			t.Fatalf("Load(%s): %v", tc.name, err)
		}
		if got.Version != player.CurrentVersion {
			t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
		}
		if got.Reputation != tc.renown {
			t.Errorf("Reputation = %d, want %d", got.Reputation, tc.renown)
		}
	}
}

// reputation.md §2/§10: a fresh character defaults to 0 (Unknown) — the
// omitempty zero value, indistinguishable from an absent (pre-v32) field, which
// is correct since the engine's default starting renown is 0.
func TestSave_ReputationDefaultsZero(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-fresh-rep", AccountID: "acct-fresh-rep", Name: "Nynaeve",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "nynaeve")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Reputation != 0 {
		t.Errorf("Reputation = %d, want 0 (Unknown) for a fresh character", got.Reputation)
	}
}

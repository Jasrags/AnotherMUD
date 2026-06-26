package player_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// The v33 owned-hireling list round-trips through a save/load (hireable-mobs.md
// §9 — the hire contract is durable). Each record's identity (its template) must
// survive relogin so the hireling can be re-materialized.
func TestSave_HirelingsRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-boss", AccountID: "acct-boss", Name: "Boss",
		Hirelings: []player.HirelingRecord{
			{TemplateID: "starter-world:sellsword"},
			{TemplateID: "wot:mercenary"},
		},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "boss")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.Hirelings) != 2 {
		t.Fatalf("Hirelings len = %d, want 2", len(got.Hirelings))
	}
	if got.Hirelings[0].TemplateID != "starter-world:sellsword" || got.Hirelings[1].TemplateID != "wot:mercenary" {
		t.Errorf("Hirelings = %+v, want the two owned templates in order", got.Hirelings)
	}
}

// A character with no hirelings loads with a nil/empty list — the omitempty
// default, the common case (and what a pre-v33 save migrates to).
func TestSave_HirelingsDefaultEmpty(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-solo", AccountID: "acct-solo", Name: "Solo",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "solo")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Hirelings) != 0 {
		t.Errorf("Hirelings = %+v, want empty (a soloist owns none)", got.Hirelings)
	}
}

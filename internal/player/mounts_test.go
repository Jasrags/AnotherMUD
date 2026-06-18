package player_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// The v26 owned-mount list round-trips through a save/load (mounts.md §10 —
// ownership is durable, colocated with the owner). Each record's identity (its
// template) must survive relogin so the mount can be re-materialized.
func TestSave_MountsRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-rider", AccountID: "acct-rider", Name: "Rider",
		Mounts: []player.MountRecord{
			{TemplateID: "starter-world:riding-horse"},
			{TemplateID: "wot:warhorse"},
		},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "rider")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.Mounts) != 2 {
		t.Fatalf("Mounts len = %d, want 2", len(got.Mounts))
	}
	if got.Mounts[0].TemplateID != "starter-world:riding-horse" || got.Mounts[1].TemplateID != "wot:warhorse" {
		t.Errorf("Mounts = %+v, want the two owned templates in order", got.Mounts)
	}
}

// A character who owns no mount loads with a nil/empty list — the omitempty
// default, the common case (and what a pre-v26 save migrates to).
func TestSave_MountsDefaultEmpty(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-walker", AccountID: "acct-walker", Name: "Walker",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "walker")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Mounts) != 0 {
		t.Errorf("Mounts = %+v, want empty (a mountless character owns none)", got.Mounts)
	}
}

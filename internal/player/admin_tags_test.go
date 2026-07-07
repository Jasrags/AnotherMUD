package player_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// The v34 admin-tag bag round-trips through a save/load: an admin-applied tag
// is character state (admin-verbs §4), so it must survive relogin — a player
// is not transient, unlike a room mob whose tags are live-only.
func TestSave_AdminTagsRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-tag", AccountID: "acct-tag", Name: "Flagged",
		AdminTags: []string{"cursed", "watch"},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "flagged")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.AdminTags) != 2 || got.AdminTags[0] != "cursed" || got.AdminTags[1] != "watch" {
		t.Errorf("AdminTags = %v, want [cursed watch] (admin tags must persist across relogin)", got.AdminTags)
	}
}

// A character no admin has tagged loads with no admin tags — the omitempty
// zero value, never a surprise flag.
func TestSave_AdminTagsDefaultEmpty(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-clean", AccountID: "acct-clean", Name: "Untagged",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "untagged")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.AdminTags) != 0 {
		t.Errorf("AdminTags = %v, want empty (an untagged character carries none)", got.AdminTags)
	}
}

// A pre-v34 save on disk migrates forward cleanly: the v33→v34 step is a
// no-op, so the character comes back at CurrentVersion carrying no admin tags
// (there was no way to author one before the field existed).
func TestLoad_V33MigratesToV34NoAdminTags(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v33user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 33\nid: p-1\naccount_id: acct-1\nname: V33User\nlocation: tapestry-core:town-square\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v33user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version after migrate = %d, want %d", got.Version, player.CurrentVersion)
	}
	if len(got.AdminTags) != 0 {
		t.Errorf("AdminTags = %v, want empty after v33→v34 migrate", got.AdminTags)
	}
	if got.Name != "V33User" {
		t.Errorf("preserved fields wrong: %+v", got)
	}
}

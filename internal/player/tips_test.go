package player_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// The v36 contextual-tips fields round-trip: a character's opt-out and
// shown-once set are per-character state that must survive relogin
// (ui-rendering-help §12).
func TestSave_TipsRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-tip", AccountID: "acct-tip", Name: "Hinted",
		TipsDisabled: true,
		TipsSeen:     []string{"help", "shop"},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "hinted")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.TipsDisabled {
		t.Error("TipsDisabled did not persist")
	}
	if len(got.TipsSeen) != 2 || got.TipsSeen[0] != "help" || got.TipsSeen[1] != "shop" {
		t.Errorf("TipsSeen = %v, want [help shop]", got.TipsSeen)
	}
}

// A fresh character loads with tips enabled and none seen — the omitempty zero
// values (TipsDisabled=false ⇒ enabled).
func TestSave_TipsDefaultEnabled(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-new", AccountID: "acct-new", Name: "Fresh",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "fresh")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.TipsDisabled {
		t.Error("a fresh character should have tips enabled (TipsDisabled=false)")
	}
	if len(got.TipsSeen) != 0 {
		t.Errorf("TipsSeen = %v, want empty", got.TipsSeen)
	}
}

// A pre-v36 save migrates forward cleanly: v35→v36 is a no-op, so the character
// returns at CurrentVersion with tips enabled and none seen.
func TestLoad_V35MigratesToV36TipsEnabled(t *testing.T) {
	ctx := context.Background()
	st, dir := newStore(t)

	playerDir := filepath.Join(dir, "players", "v35user")
	if err := os.MkdirAll(playerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(playerDir, "player.yaml"),
		[]byte("version: 35\nid: p-1\naccount_id: acct-1\nname: V35User\nlocation: tapestry-core:town-square\n"),
		0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := st.Load(ctx, "v35user")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version after migrate = %d, want %d", got.Version, player.CurrentVersion)
	}
	if got.TipsDisabled {
		t.Error("pre-v36 save should migrate with tips enabled")
	}
	if len(got.TipsSeen) != 0 {
		t.Errorf("TipsSeen = %v, want empty after v35→v36 migrate", got.TipsSeen)
	}
	if got.Name != "V35User" {
		t.Errorf("preserved fields wrong: %+v", got)
	}
}

package player_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// The v28 channeling gift round-trips through a save/load — it's a durable
// creation origin (the WoT pack's spark/learn/none), so it survives relogin.
func TestSave_ChannelingGiftRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-cg", AccountID: "acct-cg", Name: "Rand",
		ChannelingGift: "spark",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "rand")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if got.ChannelingGift != "spark" {
		t.Errorf("ChannelingGift = %q, want spark (must persist across relogin)", got.ChannelingGift)
	}
}

// A non-WoT character (or any pre-v28 save) loads with an empty gift — the
// omitempty zero value, no surprise affinity.
func TestSave_ChannelingGiftDefaultsEmpty(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-mundane", AccountID: "acct-mun", Name: "Perrin",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "perrin")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ChannelingGift != "" {
		t.Errorf("ChannelingGift = %q, want empty (unset by default)", got.ChannelingGift)
	}
}

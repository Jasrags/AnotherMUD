package player_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// The v27 Power Attack stance round-trips through a save/load — it's a
// persistent combat posture (feats Bucket C), so it survives relogin.
func TestSave_PowerAttackRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-pa", AccountID: "acct-pa", Name: "Brawler",
		PowerAttackActive: true,
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "brawler")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != player.CurrentVersion {
		t.Errorf("Version = %d, want %d", got.Version, player.CurrentVersion)
	}
	if !got.PowerAttackActive {
		t.Error("PowerAttackActive = false, want true (the stance must persist across relogin)")
	}
}

// A character who never entered the stance (or any pre-v27 save) loads with the
// stance off — the omitempty zero value, no surprise posture.
func TestSave_PowerAttackDefaultsOff(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-measured", AccountID: "acct-m", Name: "Measured",
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "measured")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.PowerAttackActive {
		t.Error("PowerAttackActive = true, want false (stance off by default)")
	}
}

package player_test

import (
	"context"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/player"
)

// sin-and-legality.md §7: a burned fake SIN stays burned across relog. The
// InventoryEntry.Burned flag round-trips through save/load so a scan-caught
// credential can't be un-burned by logging out.
func TestSave_CredentialBurnedRoundTrip(t *testing.T) {
	st, _ := newStore(t)
	ctx := context.Background()
	save := &player.Save{
		Version: player.CurrentVersion,
		ID:      "p-sin", AccountID: "acct-sin", Name: "Jax",
		Inventory: []player.InventoryEntry{
			{Template: "shadowrun:fake-sin-basic", Burned: true},
			{Template: "shadowrun:fake-sin-premium"}, // unburned
		},
	}
	if err := st.Save(ctx, save); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := st.Load(ctx, "jax")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Inventory) != 2 {
		t.Fatalf("Inventory len = %d, want 2", len(got.Inventory))
	}
	if !got.Inventory[0].Burned {
		t.Error("burned fake SIN did not round-trip as burned")
	}
	if got.Inventory[1].Burned {
		t.Error("unburned fake SIN round-tripped as burned")
	}
}

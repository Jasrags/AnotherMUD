package corpse

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
)

// makeCorpse mints a corpse-tagged container with the given owner set
// and creation tick.
func makeCorpse(t *testing.T, owners []string, createdTick uint64, coins int) *entities.ItemInstance {
	t.Helper()
	s := entities.NewStore()
	c, err := s.SpawnContainer("the corpse of a goblin",
		[]string{TagCorpse, TagNoGet, TagNoPut},
		[]string{"corpse", "goblin"},
		map[string]any{
			PropOwners:      owners,
			PropCreatedTick: createdTick,
			PropCoins:       coins,
		})
	if err != nil {
		t.Fatalf("SpawnContainer: %v", err)
	}
	return c
}

func TestMayLoot_OwnerDuringWindow(t *testing.T) {
	c := makeCorpse(t, []string{"player:bob"}, 100, 0)
	// now=110, window=50 → window not elapsed (110 < 150).
	if !MayLoot(c, "player:bob", 110, 50) {
		t.Error("owner should be allowed during window")
	}
	if MayLoot(c, "player:eve", 110, 50) {
		t.Error("non-owner should be refused during window")
	}
}

func TestMayLoot_OpenAfterWindow(t *testing.T) {
	c := makeCorpse(t, []string{"player:bob"}, 100, 0)
	// now=160, created+window=150 → elapsed → open to anyone.
	if !MayLoot(c, "player:eve", 160, 50) {
		t.Error("after window, anyone may loot")
	}
}

func TestMayLoot_EmptyOwnerSetOpenImmediately(t *testing.T) {
	c := makeCorpse(t, []string{}, 100, 0)
	if !MayLoot(c, "player:eve", 100, 50) {
		t.Error("no killer → open immediately")
	}
}

func TestMayLoot_ZeroWindowOpen(t *testing.T) {
	c := makeCorpse(t, []string{"player:bob"}, 100, 0)
	if !MayLoot(c, "player:eve", 100, 0) {
		t.Error("zero window → no reservation → open")
	}
}

func TestAccessors(t *testing.T) {
	c := makeCorpse(t, []string{"player:bob"}, 42, 7)
	if !IsCorpse(c) {
		t.Error("IsCorpse should be true")
	}
	if got := Coins(c); got != 7 {
		t.Errorf("Coins = %d, want 7", got)
	}
	if got := CreatedTick(c); got != 42 {
		t.Errorf("CreatedTick = %d, want 42", got)
	}
	if got := Owners(c); len(got) != 1 || got[0] != "player:bob" {
		t.Errorf("Owners = %v", got)
	}
	SetCoins(c, 0)
	if got := Coins(c); got != 0 {
		t.Errorf("after SetCoins(0): Coins = %d", got)
	}
}

func TestAccessors_NilSafe(t *testing.T) {
	if IsCorpse(nil) || Coins(nil) != 0 || CreatedTick(nil) != 0 || Owners(nil) != nil {
		t.Error("accessors must be nil-safe")
	}
}

package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/channel"
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
)

// TestStats_MitigationReadsWornArmor is the player-side mirror of
// entities.TestMobMitigationReadsArmorInput (sr-m3c-deferred-fixes): a connActor
// under a Shadowrun `mitigation: body + armor` channel map reads its summed
// worn-armour rating through the channel.InputArmor synthetic input. Equipping
// an armor_bonus:4 jacket over Body 3 must raise mitigation from 3 (body alone)
// to 7 (body + armor), proving the player-side InputArmor branch and the
// recomputeWeaponLocked → wornArmorBonus pipeline are wired end-to-end.
func TestStats_MitigationReadsWornArmor(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	a.statBlock = progression.NewWithBase(map[progression.StatType]int{"body": 3})
	m, err := channel.NewMapping(map[channel.Channel]string{channel.Mitigation: "body + armor"})
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	a.channelMap = m

	// Unarmored: the armor input resolves to 0, so mitigation = body alone.
	if got := a.Stats().Mitigation; got != 3 {
		t.Fatalf("unarmored Mitigation = %d, want 3 (body only)", got)
	}

	jacket, err := store.Spawn(&item.Template{
		ID: "shadowrun:test-jacket", Name: "an armored jacket", Type: "item",
		ArmorBonus: 4,
	})
	if err != nil {
		t.Fatalf("Spawn jacket: %v", err)
	}
	a.AddToInventory(jacket.ID())
	if !a.Equip([]string{"body"}, jacket.ID(), nil) {
		t.Fatal("Equip jacket returned false")
	}

	// Equipping drove recomputeWeaponLocked, which summed ArmorBonus into
	// wornArmorBonus; the InputArmor branch now feeds 4 into the formula.
	if got := a.wornArmorBonus.Load(); got != 4 {
		t.Fatalf("wornArmorBonus = %d, want 4", got)
	}
	if got := a.Stats().Mitigation; got != 7 {
		t.Errorf("armored Mitigation = %d, want 7 (body 3 + armor 4)", got)
	}
}

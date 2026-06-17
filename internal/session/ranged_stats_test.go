package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/player"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// newRangedActor builds a connActor with a chosen STR so the ranged-combat
// §4 Strength rule is observable through Stats() (STR 14 → base damage
// bonus +2 via the unmapped STRBonus path).
func newRangedActor(t *testing.T, store *entities.Store) *connActor {
	t.Helper()
	return &connActor{
		save:       &player.Save{Version: player.CurrentVersion, Name: "Archer"},
		items:      store,
		equipment:  make(map[string]entities.EntityID),
		footprints: make(map[entities.EntityID][]string),
		statBlock:  progression.NewWithBase(map[progression.StatType]int{progression.StatSTR: 14}),
	}
}

func TestRangedStrengthRule_FlowsThroughStats(t *testing.T) {
	intp := func(v int) *int { return &v }
	cases := []struct {
		name        string
		class       string
		ammoKind    string
		strRating   *int
		wantBonus   int
		wantClass   string
		wantAmmo    string
	}{
		{"thrown adds full STR bonus", item.RangedThrown, "", nil, 2, "thrown", ""},
		{"plain projectile drops the positive STR bonus", item.RangedProjectile, "arrow", nil, 0, "projectile", "arrow"},
		{"strength-rated bow caps the STR bonus", item.RangedProjectile, "arrow", intp(1), 1, "projectile", "arrow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := entities.NewStore()
			a := newRangedActor(t, store)

			// Baseline: a melee weapon keeps the full +2 STR bonus.
			inst, _ := store.Spawn(&item.Template{
				ID: "test:ranged", Name: "a ranged weapon", Type: "weapon",
				WeaponDamage: "1d6",
				RangedClass:  tc.class, AmmoKind: tc.ammoKind, StrRating: tc.strRating,
			})
			a.AddToInventory(inst.ID())
			if !a.Equip([]string{"wield"}, inst.ID(), []stats.Modifier{}) {
				t.Fatal("Equip returned false")
			}

			got := a.Stats()
			if got.DamageBonus != tc.wantBonus {
				t.Errorf("DamageBonus = %d, want %d", got.DamageBonus, tc.wantBonus)
			}
			if got.RangedClass != tc.wantClass {
				t.Errorf("RangedClass = %q, want %q", got.RangedClass, tc.wantClass)
			}
			if got.AmmoKind != tc.wantAmmo {
				t.Errorf("AmmoKind = %q, want %q", got.AmmoKind, tc.wantAmmo)
			}
		})
	}
}

// A melee weapon is unaffected by the ranged Strength rule — the full +2
// bonus rides through, and the ranged fields stay empty.
func TestMeleeWeapon_KeepsFullStrengthBonus(t *testing.T) {
	store := entities.NewStore()
	a := newRangedActor(t, store)
	inst, _ := store.Spawn(&item.Template{
		ID: "test:club", Name: "a club", Type: "weapon", WeaponDamage: "1d6",
	})
	a.AddToInventory(inst.ID())
	if !a.Equip([]string{"wield"}, inst.ID(), []stats.Modifier{}) {
		t.Fatal("Equip returned false")
	}
	got := a.Stats()
	if got.DamageBonus != 2 {
		t.Errorf("melee DamageBonus = %d, want 2 (full STR bonus)", got.DamageBonus)
	}
	if got.RangedClass != "" || got.AmmoKind != "" {
		t.Errorf("melee weapon should have empty ranged fields, got class=%q kind=%q", got.RangedClass, got.AmmoKind)
	}
}

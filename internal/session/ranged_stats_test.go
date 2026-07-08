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
		name      string
		class     string
		ammoKind  string
		strRating *int
		wantBonus int
		wantClass string
		wantAmmo  string
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

// ConsumeAmmo (the AmmoConsumer seam) removes one matching ammo instance
// per call, returns the consumed unit's grade key, and reports no-match /
// blank-kind correctly (ranged-combat §3).
func TestConsumeAmmo(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)

	// Two plain arrows + one masterwork arrow + a non-ammo item.
	for range 2 {
		inst, _ := store.Spawn(&item.Template{
			ID: "test:arrow", Name: "an arrow", Type: "item", AmmoKind: "arrow",
		})
		a.AddToInventory(inst.ID())
	}
	mw, _ := store.Spawn(&item.Template{
		ID: "test:mw-arrow", Name: "a fine arrow", Type: "item", AmmoKind: "arrow", Grade: "masterwork",
	})
	a.AddToInventory(mw.ID())
	other, _ := store.Spawn(&item.Template{ID: "test:rock", Name: "a rock", Type: "item"})
	a.AddToInventory(other.ID())

	startLen := len(a.Inventory())

	// Blank kind never matches.
	if grade, ok := a.ConsumeAmmo(""); ok || grade != "" {
		t.Errorf("ConsumeAmmo(\"\") = (%q,%v), want (\"\",false)", grade, ok)
	}
	// A kind with no matching ammo.
	if grade, ok := a.ConsumeAmmo("bolt"); ok || grade != "" {
		t.Errorf("ConsumeAmmo(bolt) = (%q,%v), want (\"\",false)", grade, ok)
	}
	if len(a.Inventory()) != startLen {
		t.Fatalf("inventory changed on a no-match consume: %d → %d", startLen, len(a.Inventory()))
	}

	// Consume three arrows: each removes exactly one matching instance.
	// At least one of the three carries the masterwork grade key.
	sawGrade := false
	for i := range 3 {
		grade, ok := a.ConsumeAmmo("arrow")
		if !ok {
			t.Fatalf("ConsumeAmmo(arrow) #%d = not consumed, want consumed", i+1)
		}
		if grade == "masterwork" {
			sawGrade = true
		}
		if got, want := len(a.Inventory()), startLen-(i+1); got != want {
			t.Errorf("after consume #%d inventory = %d, want %d", i+1, got, want)
		}
	}
	if !sawGrade {
		t.Error("never observed the masterwork arrow's grade key across three consumes")
	}
	// All arrows gone — only the rock remains, so a further consume fails.
	if grade, ok := a.ConsumeAmmo("arrow"); ok || grade != "" {
		t.Errorf("ConsumeAmmo(arrow) after exhaustion = (%q,%v), want (\"\",false)", grade, ok)
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

package session

import (
	"fmt"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/stats"
)

// cappedDexAC is the synthetic `dex_ac` channel input (armor-depth §3): the
// wearer's Dex modifier, capped by the most restrictive worn armor's max-Dex.
// No cap ⇒ the full modifier (the d20 unarmored case).
func TestCappedDexAC(t *testing.T) {
	cap0, cap1, cap3 := 0, 1, 3
	tests := []struct {
		name string
		dex  int
		cap  *int
		want int
	}{
		{"unarmored: full positive Dex", 14, nil, 2},
		{"unarmored: full negative Dex", 8, nil, -1},
		{"cap above modifier: full Dex", 14, &cap3, 2},
		{"cap below modifier: clamped", 18, &cap1, 1},
		{"cap equal to modifier: unchanged", 14, &cap3, 2},
		{"zero cap (heavy): no Dex to AC", 18, &cap0, 0},
		{"cap never raises a low Dex", 8, &cap3, -1},
	}
	for _, tt := range tests {
		a := &connActor{statBlock: progression.NewWithBase(map[progression.StatType]int{progression.StatDEX: tt.dex})}
		if tt.cap != nil {
			a.armorDexCap.Store(tt.cap)
		}
		if got := a.cappedDexAC(); got != tt.want {
			t.Errorf("%s: cappedDexAC() = %d, want %d", tt.name, got, tt.want)
		}
	}
}

// IsArmorProficient gates on whether every worn tiered armor is one the
// actor's class(es) grant (armor-depth §5); unarmored and untiered are free.
func TestIsArmorProficient(t *testing.T) {
	reg := progression.NewClassRegistry()
	if err := reg.Register(&progression.Class{ID: "initiate", ArmorProficiencyTiers: []string{"light"}}); err != nil {
		t.Fatalf("register initiate: %v", err)
	}

	tests := []struct {
		name    string
		classID string
		worn    []string
		want    bool
	}{
		{"no armor worn is proficient", "initiate", nil, true},
		{"granted tier is proficient", "initiate", []string{"light"}, true},
		{"ungranted tier is non-proficient", "initiate", []string{"heavy"}, false},
		{"mixed: any ungranted piece fails", "initiate", []string{"light", "heavy"}, false},
		{"classless actor with tiered armor is non-proficient", "", []string{"light"}, false},
	}
	for _, tt := range tests {
		var classIDs []string
		if tt.classID != "" {
			classIDs = []string{tt.classID}
		}
		a := &connActor{classes: reg, classIDs: classIDs}
		if tt.worn != nil {
			worn := append([]string(nil), tt.worn...)
			a.armorTiers.Store(&worn)
		}
		if got := a.IsArmorProficient(); got != tt.want {
			t.Errorf("%s: IsArmorProficient() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// NonProficientArmorCheckPenalty sums the (grade-reduced) check penalty of only
// the worn pieces the actor is NOT proficient with (armor-depth §5). A
// proficient piece — and an untiered one — contributes nothing, so a mixed
// loadout no longer over-penalizes to-hit with the proficient pieces' penalty.
func TestNonProficientArmorCheckPenalty(t *testing.T) {
	reg := progression.NewClassRegistry()
	if err := reg.Register(&progression.Class{ID: "initiate", ArmorProficiencyTiers: []string{"light"}}); err != nil {
		t.Fatalf("register initiate: %v", err)
	}

	type piece struct {
		tier    string
		penalty int
	}
	tests := []struct {
		name    string
		classID string
		pieces  []piece
		want    int
	}{
		{"no armor worn", "initiate", nil, 0},
		{"proficient light only", "initiate", []piece{{"light", 1}}, 0},
		{"non-proficient heavy only", "initiate", []piece{{"heavy", 6}}, 6},
		{"mixed: only the non-proficient piece counts", "initiate", []piece{{"light", 1}, {"heavy", 6}}, 6},
		{"untiered piece never counts", "initiate", []piece{{"", 2}, {"light", 1}}, 0},
		{"classless: every tiered piece counts", "", []piece{{"light", 1}, {"heavy", 6}}, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := entities.NewStore()
			a := newEqActor(t, store)
			a.classes = reg
			if tt.classID != "" {
				a.classIDs = []string{tt.classID}
			}
			for i, p := range tt.pieces {
				inst, err := store.Spawn(&item.Template{
					ID: item.TemplateID(fmt.Sprintf("t:armor-%d", i)), Name: "armor", Type: "item",
					ArmorTier: p.tier, ArmorCheckPenalty: p.penalty,
				})
				if err != nil {
					t.Fatalf("spawn piece %d: %v", i, err)
				}
				a.AddToInventory(inst.ID())
				if !a.Equip([]string{fmt.Sprintf("slot%d", i)}, inst.ID(), nil) {
					t.Fatalf("equip piece %d returned false", i)
				}
			}
			if got := a.NonProficientArmorCheckPenalty(); got != tt.want {
				t.Errorf("NonProficientArmorCheckPenalty() = %d, want %d", got, tt.want)
			}
		})
	}
}

// recomputeWeaponLocked (via Equip/Unequip) snapshots the most restrictive
// max-Dex cap and the distinct worn tiers across all equipped armor.
func TestRecomputeArmorState(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)

	cap3, cap1 := 3, 1
	helm, err := store.Spawn(&item.Template{
		ID: "t:helm", Name: "a helm", Type: "item",
		ArmorBonus: 1, ArmorMaxDex: &cap3, ArmorTier: "light",
	})
	if err != nil {
		t.Fatalf("Spawn helm: %v", err)
	}
	shield, err := store.Spawn(&item.Template{
		ID: "t:shield", Name: "a shield", Type: "item",
		ArmorBonus: 2, ArmorMaxDex: &cap1, ArmorTier: "heavy",
	})
	if err != nil {
		t.Fatalf("Spawn shield: %v", err)
	}
	a.AddToInventory(helm.ID())
	a.AddToInventory(shield.ID())

	// Nothing worn yet → no cap, no tiers.
	if a.armorDexCap.Load() != nil || a.armorTiers.Load() != nil {
		t.Fatalf("before equip: cap=%v tiers=%v, want nil/nil", a.armorDexCap.Load(), a.armorTiers.Load())
	}

	if !a.Equip([]string{"head"}, helm.ID(), []stats.Modifier{{Stat: "ac", Value: 1}}) {
		t.Fatal("Equip helm returned false")
	}
	if cap := a.armorDexCap.Load(); cap == nil || *cap != 3 {
		t.Fatalf("after helm: cap = %v, want 3", cap)
	}

	if !a.Equip([]string{"offhand"}, shield.ID(), []stats.Modifier{{Stat: "ac", Value: 2}}) {
		t.Fatal("Equip shield returned false")
	}
	// Most restrictive (lowest) cap wins across the two pieces.
	if cap := a.armorDexCap.Load(); cap == nil || *cap != 1 {
		t.Fatalf("after helm+shield: cap = %v, want 1 (most restrictive)", cap)
	}
	tiers := a.armorTiers.Load()
	if tiers == nil || len(*tiers) != 2 {
		t.Fatalf("worn tiers = %v, want 2 distinct (light, heavy)", tiers)
	}
	seen := map[string]bool{}
	for _, tt := range *tiers {
		seen[tt] = true
	}
	if !seen["light"] || !seen["heavy"] {
		t.Errorf("worn tiers = %v, want light+heavy", *tiers)
	}

	// Unequipping everything clears the snapshot.
	a.Unequip("head")
	a.Unequip("offhand")
	if a.armorDexCap.Load() != nil || a.armorTiers.Load() != nil {
		t.Errorf("after unequip-all: cap=%v tiers=%v, want nil/nil", a.armorDexCap.Load(), a.armorTiers.Load())
	}
}

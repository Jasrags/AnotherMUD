package session

import (
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/pool"
)

// seedEssence gives the test actor a full Shadowrun Essence pool (SR-M4): 60
// tenths == 6.0, floor 0, degrading a `magic` pool (inert unless a magic pool
// is also seeded). Mirrors what content-driven seedResourcePools does for a
// real boot.
func seedEssence(a *connActor) *pool.Pool {
	if a.pools == nil {
		a.pools = pool.NewSet()
	}
	p := pool.New(poolKindEssence, 60, pool.Rules{Floor: 0, Degrades: string(poolKindMagic)})
	a.pools.Add(p)
	return p
}

// poolKindMagic is a test-only pool kind standing in for the Awakened Magic
// attribute a future mage metatype would carry. It is authored on the SAME
// tenths scale as Essence so the degrades cap reconciles (see the SR-M4
// applyEssenceDegradesLocked / essence.yaml same-scale contract).
const poolKindMagic = pool.Kind("magic")

func cyberwareTpl(id string, cost int, stat string, val int) *item.Template {
	return &item.Template{
		ID:            item.TemplateID("shadowrun:" + id),
		Name:          id,
		Type:          "item",
		EligibleSlots: []string{"cyberware"},
		EssenceCost:   cost,
		Modifiers:     []item.Modifier{{Stat: stat, Value: val}},
	}
}

// TestEssence_InstallLowersRestoreRaises is the SR-M4 core: Essence is DERIVED
// from installed chrome (current = max − Σ cost). Installing wired reflexes
// (2.0 == 20 tenths) drops Essence to 4.0; removing it restores 6.0.
func TestEssence_InstallLowersRestoreRaises(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	seedEssence(a)

	// Full at rest.
	if got := a.Essence(); got != 60 {
		t.Fatalf("initial Essence = %d, want 60 (6.0)", got)
	}

	wired, err := store.Spawn(cyberwareTpl("wired-reflexes", 20, "reaction", 2))
	if err != nil {
		t.Fatalf("Spawn wired: %v", err)
	}
	a.AddToInventory(wired.ID())
	if !a.Equip([]string{"cyberware"}, wired.ID(), nil) {
		t.Fatal("Equip wired reflexes returned false")
	}
	if got := a.Essence(); got != 40 {
		t.Errorf("Essence after install = %d, want 40 (6.0 − 2.0)", got)
	}

	// Removing the chrome restores the Essence (symmetric with the reversible
	// cyberware slot).
	if _, ok := a.Unequip("cyberware"); !ok {
		t.Fatal("Unequip cyberware returned false")
	}
	if got := a.Essence(); got != 60 {
		t.Errorf("Essence after removal = %d, want 60 (restored)", got)
	}
}

// TestEssence_MultipleImplantsSum: installed cost is summed across the slot, so
// wired reflexes (2.0) + cybereyes (0.2) leave 6.0 − 2.2 = 3.8 (38 tenths).
func TestEssence_MultipleImplantsSum(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	seedEssence(a)

	wired, _ := store.Spawn(cyberwareTpl("wired-reflexes", 20, "reaction", 2))
	eyes, _ := store.Spawn(cyberwareTpl("cybereyes", 2, "intuition", 1))
	a.AddToInventory(wired.ID())
	a.AddToInventory(eyes.ID())
	if !a.Equip([]string{"cyberware"}, wired.ID(), nil) {
		t.Fatal("Equip wired returned false")
	}
	if !a.Equip([]string{"cyberware2"}, eyes.ID(), nil) {
		t.Fatal("Equip eyes returned false")
	}
	if got := a.Essence(); got != 38 {
		t.Errorf("Essence with two implants = %d, want 38 (6.0 − 2.2)", got)
	}
}

// TestEssence_NoPoolInertCost: a world with no essence pool ignores essence_cost
// entirely — Essence()/EssenceMax() read 0 and equipping the item is unaffected.
func TestEssence_NoPoolInertCost(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store) // no seedEssence — no essence pool

	imp, _ := store.Spawn(cyberwareTpl("wired-reflexes", 20, "reaction", 2))
	a.AddToInventory(imp.ID())
	if !a.Equip([]string{"cyberware"}, imp.ID(), nil) {
		t.Fatal("Equip should succeed with no essence pool")
	}
	if got := a.EssenceMax(); got != 0 {
		t.Errorf("EssenceMax with no pool = %d, want 0", got)
	}
	if got := a.Essence(); got != 0 {
		t.Errorf("Essence with no pool = %d, want 0", got)
	}
}

// TestEssence_DegradesCapsMagicPool proves the pool.Rules.Degrades honoring
// (SR-M4): Essence's current caps the MAX of the pool it degrades. Seeds a
// synthetic `magic` pool (the Awakened stand-in) at 6.0 alongside Essence;
// installing 2.0 of chrome ratchets Magic's ceiling down to 4.0. Inert for a
// mundane runner (no magic pool → the earlier tests never touch this path).
func TestEssence_DegradesCapsMagicPool(t *testing.T) {
	store := entities.NewStore()
	a := newEqActor(t, store)
	seedEssence(a)
	magic := pool.New(poolKindMagic, 60, pool.Rules{Floor: 0}) // 6.0 Magic, same tenths scale
	a.pools.Add(magic)

	// Recompute once so the degrades hook runs against the freshly-added magic
	// pool (Add does not re-run the equip recompute).
	a.mu.Lock()
	a.recomputeWeaponLocked()
	a.mu.Unlock()
	if got := magic.Max(); got != 60 {
		t.Fatalf("Magic max at full Essence = %d, want 60 (uncapped)", got)
	}

	wired, _ := store.Spawn(cyberwareTpl("wired-reflexes", 20, "reaction", 2))
	a.AddToInventory(wired.ID())
	if !a.Equip([]string{"cyberware"}, wired.ID(), nil) {
		t.Fatal("Equip wired returned false")
	}
	if got := magic.Max(); got != 40 {
		t.Errorf("Magic max after 2.0 Essence loss = %d, want 40 (ratcheted to Essence current)", got)
	}
}

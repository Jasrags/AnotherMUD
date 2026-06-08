package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/slot"
)

// lootPack writes a minimal pack carrying a loot table file plus a mob
// that references it (bodies supplied per test).
func lootPack(t *testing.T, lootBody, mobBody string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  loot_tables: [loot_tables/*.yaml]
  mobs: [mobs/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "loot_tables/t.yaml"), lootBody)
	writeFile(t, filepath.Join(pack, "mobs/m.yaml"), mobBody)
	return root
}

// Load decodes a loot table into the registry with its pools intact and
// every item id namespace-qualified against the pack.
func TestLoad_DecodesLootTable(t *testing.T) {
	root := lootPack(t, `
id: guard-loot
guaranteed:
  - { item: trail-ration, count: 2 }
weighted:
  - { item: healing-draught, weight: 3 }
  - { item: other-pack:leather-cap, weight: 1 }
pool_rolls: 1
rare_bonus:
  chance: 20
  entries:
    - { item: short-sword, weight: 1 }
coin:
  min: 2
  max: 8
`, `
id: village-guard
name: a village guard
behavior: stationary
loot_table: guard-loot
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Table id is qualified against the pack.
	tbl, ok := regs.Loot.Get("tapestry-core:guard-loot")
	if !ok {
		t.Fatal("Loot.Get(tapestry-core:guard-loot) miss")
	}
	if tbl.PoolRolls != 1 {
		t.Errorf("PoolRolls = %d, want 1", tbl.PoolRolls)
	}
	// Unqualified item id qualifies to this pack; an already-qualified
	// id passes through unchanged.
	if len(tbl.Guaranteed) != 1 || tbl.Guaranteed[0].ItemID != "tapestry-core:trail-ration" || tbl.Guaranteed[0].Count != 2 {
		t.Errorf("guaranteed = %+v", tbl.Guaranteed)
	}
	if len(tbl.Weighted) != 2 || tbl.Weighted[0].ItemID != "tapestry-core:healing-draught" || tbl.Weighted[1].ItemID != "other-pack:leather-cap" {
		t.Errorf("weighted = %+v", tbl.Weighted)
	}
	if tbl.RareBonus == nil || tbl.RareBonus.Chance != 20 || tbl.RareBonus.Entries[0].ItemID != "tapestry-core:short-sword" {
		t.Errorf("rare bonus = %+v", tbl.RareBonus)
	}
	if tbl.Coin == nil || tbl.Coin.Min != 2 || tbl.Coin.Max != 8 {
		t.Errorf("coin = %+v", tbl.Coin)
	}

	// The mob carries the qualified loot-table reference.
	m, err := regs.Mobs.Get("tapestry-core:village-guard")
	if err != nil {
		t.Fatalf("Mobs.Get: %v", err)
	}
	if m.LootTable != "tapestry-core:guard-loot" {
		t.Errorf("mob LootTable = %q, want tapestry-core:guard-loot", m.LootTable)
	}
}

// A loot table with a blank item id fails the boot with an
// ErrInvalidContent wrap so a no-drop typo can't pass silently.
func TestLoad_RejectsBlankLootItemID(t *testing.T) {
	root := lootPack(t, `
id: bad
weighted:
  - { item: "", weight: 1 }
`, `
id: m
name: a thing
behavior: stationary
`)
	err := Load(context.Background(), root, nil, NewRegistries(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error on blank loot item id")
	}
	if !errors.Is(err, ErrInvalidContent) {
		t.Errorf("err = %v, want ErrInvalidContent wrap", err)
	}
}

// The real core pack's village-guard table loads and is referenced.
func TestLoad_CoreGuardLoot(t *testing.T) {
	root, err := filepath.Abs("../../content")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("register engine baseline properties: %v", err)
	}
	if err := slot.RegisterEngineBaseline(regs.Slots); err != nil {
		t.Fatalf("register engine baseline slots: %v", err)
	}
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load core: %v", err)
	}
	if !regs.Loot.Has("tapestry-core:guard-loot") {
		t.Fatal("core guard-loot table not registered")
	}
	m, err := regs.Mobs.Get("tapestry-core:village-guard")
	if err != nil {
		t.Fatalf("Mobs.Get village-guard: %v", err)
	}
	if m.LootTable != "tapestry-core:guard-loot" {
		t.Errorf("village-guard LootTable = %q, want tapestry-core:guard-loot", m.LootTable)
	}
}

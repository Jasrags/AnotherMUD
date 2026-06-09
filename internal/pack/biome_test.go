package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/biome"
)

// biomePack writes a minimal pack whose manifest declares a biomes glob,
// with one biome file at biomes/<file>.
func biomePack(t *testing.T, file, body string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  biomes: [biomes/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "biomes", file), body)
	return root
}

func TestLoad_RegistersPackBiome(t *testing.T) {
	root := biomePack(t, "swamp.yaml", `
id: swamp
name: fetid swamp
weather_shielded: true
ambience: ["A frog croaks."]
forage_table: swamp-forage
`)
	regs := NewRegistries()
	if err := biome.RegisterEngineBaseline(regs.Biomes); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	b, ok := regs.Biomes.Get("swamp")
	if !ok {
		t.Fatal("swamp biome not registered from pack content")
	}
	// forage_table is namespace-qualified by the loader (bare → pack:id).
	if !b.WeatherShielded || b.ForageTable != "tapestry-core:swamp-forage" || b.Pack == "" {
		t.Errorf("swamp biome = %+v (want shielded, qualified forage table, pack set)", b)
	}
	// Engine baseline still resolvable alongside the pack biome.
	if _, ok := regs.Biomes.Get("outdoors"); !ok {
		t.Error("engine baseline outdoors missing after pack load")
	}
}

func TestLoad_RegistersForageTableNamespaced(t *testing.T) {
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  biomes: [biomes/*.yaml]
  forage_tables: [forage_tables/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "biomes", "forest.yaml"), `
id: forest
forage_table: forest-forage
`)
	writeFile(t, filepath.Join(pack, "forage_tables", "forest-forage.yaml"), `
id: forest-forage
richness: 60
ceiling: uncommon
entries:
  - {item: wild-herb, weight: 3}
  - {item: berries, weight: 1, qty: 2}
`)
	regs := NewRegistries()
	if err := biome.RegisterEngineBaseline(regs.Biomes); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Table registered under the namespaced id; the biome references it.
	tbl, ok := regs.ForageTables.Get("tapestry-core:forest-forage")
	if !ok {
		t.Fatal("forage table not registered under namespaced id")
	}
	if b, _ := regs.Biomes.Get("forest"); b.ForageTable != "tapestry-core:forest-forage" {
		t.Errorf("biome forage_table = %q, want the qualified table id", b.ForageTable)
	}
	// Entry item ids are namespace-qualified, qty defaulted/parsed.
	if len(tbl.Entries) != 2 || tbl.Entries[0].Item != "tapestry-core:wild-herb" {
		t.Errorf("entries = %+v, want qualified item ids", tbl.Entries)
	}
	if tbl.Entries[0].Qty != 1 || tbl.Entries[1].Qty != 2 {
		t.Errorf("qty parse = %d,%d want 1,2", tbl.Entries[0].Qty, tbl.Entries[1].Qty)
	}
	if tbl.Richness != 60 || tbl.Ceiling != "uncommon" {
		t.Errorf("table = richness %d ceiling %q", tbl.Richness, tbl.Ceiling)
	}
}

func TestLoad_PackBiomeShadowingEngineFails(t *testing.T) {
	root := biomePack(t, "outdoors.yaml", `
id: outdoors
name: a pack override of outdoors
`)
	regs := NewRegistries()
	if err := biome.RegisterEngineBaseline(regs.Biomes); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, biome.ErrShadow) {
		t.Fatalf("Load err = %v, want biome.ErrShadow (pack can't shadow engine biome)", err)
	}
}

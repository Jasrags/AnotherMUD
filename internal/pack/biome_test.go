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
	if !b.WeatherShielded || b.ForageTable != "swamp-forage" || b.Pack == "" {
		t.Errorf("swamp biome = %+v (want shielded, forage table, pack set)", b)
	}
	// Engine baseline still resolvable alongside the pack biome.
	if _, ok := regs.Biomes.Get("outdoors"); !ok {
		t.Error("engine baseline outdoors missing after pack load")
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

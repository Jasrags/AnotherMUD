package pack

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// propPack writes a minimal core pack: an area (given body), a room, and an
// optional set of content-declared property files (name → YAML body).
func propPack(t *testing.T, areaBody string, propFiles map[string]string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	manifest := "name: tapestry-core\ncontent:\n  areas: [areas/*.yaml]\n  rooms: [rooms/*.yaml]\n"
	if len(propFiles) > 0 {
		manifest += "  properties: [properties/*.yaml]\n"
	}
	writeFile(t, filepath.Join(pack, "pack.yaml"), manifest)
	writeFile(t, filepath.Join(pack, "areas/town.yaml"), areaBody)
	writeFile(t, filepath.Join(pack, "rooms/square.yaml"), `
id: square
area: town
name: The Square
description: stones
`)
	for name, body := range propFiles {
		writeFile(t, filepath.Join(pack, "properties", name+".yaml"), body)
	}
	return root
}

func loadPropPack(t *testing.T, root string) (*Registries, error) {
	t.Helper()
	regs := NewRegistries()
	if err := RegisterEngineBaselineProperties(regs.Properties); err != nil {
		t.Fatalf("baseline properties: %v", err)
	}
	return regs, Load(context.Background(), root, nil, regs, nil, nil, nil)
}

// The area property bag round-trips the generic engine-baseline keys (region,
// level_range), readable through the typed accessors on world.Area.
func TestLoad_AreaPropertyBag_RoundTrips(t *testing.T) {
	root := propPack(t, `
id: town
name: Town
properties:
  region: seattle
  level_range: "1-10"
`, nil)
	regs, err := loadPropPack(t, root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	area, err := regs.World.Area("tapestry-core:town")
	if err != nil {
		t.Fatalf("Area: %v", err)
	}
	if got, ok := area.PropertyString("region"); !ok || got != "seattle" {
		t.Errorf("region = %q (ok=%v), want seattle", got, ok)
	}
	if got, ok := area.PropertyString("level_range"); !ok || got != "1-10" {
		t.Errorf("level_range = %q (ok=%v), want 1-10", got, ok)
	}
}

// An unregistered area property is a load error, naming the offending key.
func TestLoad_AreaProperty_UnregisteredIsError(t *testing.T) {
	root := propPack(t, `
id: town
name: Town
properties:
  not_a_real_property: nope
`, nil)
	_, err := loadPropPack(t, root)
	if err == nil || !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("want ErrInvalidContent for an unregistered area property, got %v", err)
	}
	if !strings.Contains(err.Error(), "not_a_real_property") {
		t.Errorf("error %q should name the offending property", err)
	}
}

// A registered property authored with the wrong value type is a load error
// (region is a string; an int must bounce).
func TestLoad_AreaProperty_TypeMismatchIsError(t *testing.T) {
	root := propPack(t, `
id: town
name: Town
properties:
  region: 5
`, nil)
	_, err := loadPropPack(t, root)
	if err == nil || !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("want ErrInvalidContent for a type-mismatched area property, got %v", err)
	}
}

// A pack can DECLARE its own property in content and use it — the content-side
// property-declaration path. The declared key loads, validates, and is queryable.
func TestLoad_PackDeclaredProperty(t *testing.T) {
	root := propPack(t, `
id: town
name: Town
properties:
  zone_tier: AAA
`, map[string]string{
		"zone_tier": "name: zone_tier\ntype: string\napplies_to: [area]\ndescription: a pack-declared tier\n",
	})
	regs, err := loadPropPack(t, root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	area, err := regs.World.Area("tapestry-core:town")
	if err != nil {
		t.Fatalf("Area: %v", err)
	}
	if got, ok := area.PropertyString("zone_tier"); !ok || got != "AAA" {
		t.Errorf("zone_tier = %q (ok=%v), want AAA", got, ok)
	}
}

// A pack-declared property that shadows an engine baseline key is a load error.
func TestLoad_PackProperty_ShadowsEngineIsError(t *testing.T) {
	root := propPack(t, `
id: town
name: Town
`, map[string]string{
		"region": "name: region\ntype: string\n",
	})
	_, err := loadPropPack(t, root)
	if err == nil || !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("want ErrInvalidContent when a pack property shadows an engine baseline, got %v", err)
	}
	if !strings.Contains(err.Error(), "shadows") {
		t.Errorf("error %q should explain the shadow", err)
	}
}

// A pack can reference a property DECLARED BY A PACK IT DEPENDS ON via the bare
// shorthand — the dependency resolver (property Get §2.4 step 3) wired at load.
func TestLoad_CrossPackDeclaredProperty(t *testing.T) {
	root := t.TempDir()
	// Library pack declares zone_tier.
	lib := filepath.Join(root, "libpack")
	writeFile(t, filepath.Join(lib, "pack.yaml"), "name: libpack\ncontent:\n  properties: [properties/*.yaml]\n")
	writeFile(t, filepath.Join(lib, "properties/zone_tier.yaml"), "name: zone_tier\ntype: string\napplies_to: [area]\n")
	// World pack depends on libpack and uses zone_tier on an area via shorthand.
	world := filepath.Join(root, "worldpack")
	writeFile(t, filepath.Join(world, "pack.yaml"), "name: worldpack\ndependencies:\n  libpack: \"*\"\ncontent:\n  areas: [areas/*.yaml]\n  rooms: [rooms/*.yaml]\n")
	writeFile(t, filepath.Join(world, "areas/downtown.yaml"), "id: downtown\nname: Downtown\nproperties:\n  zone_tier: AAA\n")
	writeFile(t, filepath.Join(world, "rooms/plaza.yaml"), "id: plaza\narea: downtown\nname: Plaza\ndescription: neon\n")

	regs, err := loadPropPack(t, root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	area, err := regs.World.Area("worldpack:downtown")
	if err != nil {
		t.Fatalf("Area: %v", err)
	}
	if got, ok := area.PropertyString("zone_tier"); !ok || got != "AAA" {
		t.Errorf("zone_tier = %q (ok=%v), want AAA (resolved via the dependency on libpack)", got, ok)
	}
}

// Without declaring the dependency, the shorthand does NOT resolve a foreign
// pack's property — it fails as unregistered (the resolver only walks declared deps).
func TestLoad_CrossPackProperty_UndeclaredDepIsError(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "libpack")
	writeFile(t, filepath.Join(lib, "pack.yaml"), "name: libpack\ncontent:\n  properties: [properties/*.yaml]\n")
	writeFile(t, filepath.Join(lib, "properties/zone_tier.yaml"), "name: zone_tier\ntype: string\n")
	world := filepath.Join(root, "worldpack")
	// No `dependencies:` on libpack.
	writeFile(t, filepath.Join(world, "pack.yaml"), "name: worldpack\ncontent:\n  areas: [areas/*.yaml]\n  rooms: [rooms/*.yaml]\n")
	writeFile(t, filepath.Join(world, "areas/downtown.yaml"), "id: downtown\nname: Downtown\nproperties:\n  zone_tier: AAA\n")
	writeFile(t, filepath.Join(world, "rooms/plaza.yaml"), "id: plaza\narea: downtown\nname: Plaza\ndescription: neon\n")

	_, err := loadPropPack(t, root)
	if err == nil || !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("want ErrInvalidContent for an undeclared cross-pack property, got %v", err)
	}
}

// An unknown property type in a declaration is a load error.
func TestLoad_PackProperty_BadTypeIsError(t *testing.T) {
	root := propPack(t, `
id: town
name: Town
`, map[string]string{
		"weird": "name: weird\ntype: banana\n",
	})
	_, err := loadPropPack(t, root)
	if err == nil || !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("want ErrInvalidContent for an unknown property type, got %v", err)
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error %q should name the bad type", err)
	}
}

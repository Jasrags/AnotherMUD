package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// recipePack writes a minimal pack carrying recipe files (bodies supplied
// per test).
func recipePack(t *testing.T, bodies map[string]string) string {
	t.Helper()
	root := t.TempDir()
	pkg := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pkg, "pack.yaml"), `
name: tapestry-core
content:
  recipes: [recipes/*.yaml]
`)
	for name, body := range bodies {
		writeFile(t, filepath.Join(pkg, "recipes", name), body)
	}
	return root
}

// Load decodes a recipe into the registry with ids namespace-qualified and
// defaults (quantity, acquisition) applied.
func TestLoad_DecodesRecipe(t *testing.T) {
	root := recipePack(t, map[string]string{
		"stew.yaml": `
id: campfire-stew
name: a hearty campfire stew
discipline: cooking
skill_floor: 5
station_tier: 1
tool: cooking-pot
time_pulses: 20
acquisition: common
inputs:
  - { template: raw-meat, quantity: 2, min_quality: common }
  - { template: other-pack:wild-herb }
output:
  template: campfire-stew-item
`,
	})
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}

	r, err := regs.Recipes.Get("tapestry-core:campfire-stew")
	if err != nil {
		t.Fatalf("Recipes.Get: %v", err)
	}
	if r.DisplayName != "a hearty campfire stew" {
		t.Errorf("DisplayName = %q", r.DisplayName)
	}
	if r.Discipline != "cooking" || r.SkillFloor != 5 || r.StationTier != 1 {
		t.Errorf("gates = %+v", r)
	}
	if r.Acquisition != recipe.AcqCommon {
		t.Errorf("Acquisition = %q, want common", r.Acquisition)
	}
	if len(r.Inputs) != 2 {
		t.Fatalf("inputs = %d, want 2", len(r.Inputs))
	}
	// Unqualified input qualifies to this pack; qualified passes through.
	if r.Inputs[0].Template != "tapestry-core:raw-meat" || r.Inputs[0].Quantity != 2 || r.Inputs[0].MinQuality != "common" {
		t.Errorf("inputs[0] = %+v", r.Inputs[0])
	}
	if r.Inputs[1].Template != "other-pack:wild-herb" || r.Inputs[1].Quantity != 1 {
		t.Errorf("inputs[1] = %+v (quantity should default to 1)", r.Inputs[1])
	}
	// Output qualifies; quantity defaults to 1.
	if r.Output.Template != "tapestry-core:campfire-stew-item" || r.Output.Quantity != 1 {
		t.Errorf("output = %+v", r.Output)
	}
}

func TestLoad_RecipeDefaultsAcquisitionBaseline(t *testing.T) {
	root := recipePack(t, map[string]string{
		"r.yaml": `
id: r
name: a recipe
discipline: smithing
inputs:
  - { template: iron }
output:
  template: nail
`,
	})
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, err := regs.Recipes.Get("tapestry-core:r")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if r.Acquisition != recipe.AcqBaseline {
		t.Errorf("default Acquisition = %q, want baseline", r.Acquisition)
	}
}

func TestLoad_RecipeRejectsBadContent(t *testing.T) {
	cases := map[string]string{
		"missing id": `
name: x
discipline: cooking
inputs: [{template: a}]
output: {template: b}
`,
		"missing discipline": `
id: x
name: x
inputs: [{template: a}]
output: {template: b}
`,
		"no inputs": `
id: x
name: x
discipline: cooking
output: {template: b}
`,
		"missing output": `
id: x
name: x
discipline: cooking
inputs: [{template: a}]
`,
		"bad acquisition": `
id: x
name: x
discipline: cooking
acquisition: mythic
inputs: [{template: a}]
output: {template: b}
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := recipePack(t, map[string]string{"bad.yaml": body})
			regs := NewRegistries()
			err := Load(context.Background(), root, nil, regs, nil, nil, nil)
			if !errors.Is(err, ErrInvalidContent) {
				t.Errorf("Load err = %v, want ErrInvalidContent", err)
			}
		})
	}
}

func TestLoad_RecipeDuplicateID(t *testing.T) {
	root := recipePack(t, map[string]string{
		"a.yaml": `
id: dup
name: a
discipline: cooking
inputs: [{template: x}]
output: {template: y}
`,
		"b.yaml": `
id: dup
name: b
discipline: cooking
inputs: [{template: x}]
output: {template: y}
`,
	})
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if !errors.Is(err, recipe.ErrDuplicateID) {
		t.Errorf("Load err = %v, want recipe.ErrDuplicateID", err)
	}
}

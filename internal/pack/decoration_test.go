package pack

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/decoration"
)

// decorationPack writes a minimal pack carrying a rarity file and an
// essence file (bodies supplied per test).
func decorationPack(t *testing.T, rarityBody, essenceBody string) string {
	t.Helper()
	root := t.TempDir()
	pack := filepath.Join(root, "core")
	writeFile(t, filepath.Join(pack, "pack.yaml"), `
name: tapestry-core
content:
  rarity: [rarity/*.yaml]
  essence: [essence/*.yaml]
`)
	writeFile(t, filepath.Join(pack, "rarity/tiers.yaml"), rarityBody)
	writeFile(t, filepath.Join(pack, "essence/essences.yaml"), essenceBody)
	return root
}

// Load decodes the rarity ladder and essence set into the registries with
// their fields intact.
func TestLoad_DecodesRarityAndEssence(t *testing.T) {
	root := decorationPack(t, `
tiers:
  - { key: common, order: 10 }
  - { key: rare, order: 30, display: RARE, left: "[", right: "]", fg: blue, visible: true }
`, `
essences:
  - { key: fire, glyph: "✦", fg: red }
`)
	regs := NewRegistries()
	if err := Load(context.Background(), root, nil, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load: %v", err)
	}

	rare, ok := regs.Rarity.Get("rare")
	if !ok {
		t.Fatal("Rarity.Get(rare) miss")
	}
	if rare.Order != 30 || rare.VisibleText() != "[RARE]" || rare.Color.FG != "blue" {
		t.Errorf("rare tier = %+v", rare)
	}
	// The baseline common tier loads and renders nothing.
	common, ok := regs.Rarity.Get("common")
	if !ok || common.VisibleText() != "" {
		t.Errorf("common tier = %+v, want loaded + blank", common)
	}

	fire, ok := regs.Essence.Get("fire")
	if !ok {
		t.Fatal("Essence.Get(fire) miss")
	}
	if fire.Glyph != "✦" || fire.Color.FG != "red" || fire.Markup() != "<essence.fire>(✦)</essence.fire>" {
		t.Errorf("fire essence = %+v", fire)
	}
}

// A rarity key with markup-significant characters fails the boot with a
// decoration.ErrInvalidKey wrap + path attribution.
func TestLoad_RejectsInvalidRarityKey(t *testing.T) {
	root := decorationPack(t, `
tiers:
  - { key: "ra>re", order: 30, display: X, left: "[", right: "]", visible: true }
`, "essences: []\n")
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error on markup-bearing rarity key")
	}
	if !errors.Is(err, decoration.ErrInvalidKey) {
		t.Errorf("err = %v, want decoration.ErrInvalidKey wrap", err)
	}
}

// An essence key with whitespace is likewise rejected at the load boundary.
func TestLoad_RejectsInvalidEssenceKey(t *testing.T) {
	root := decorationPack(t, "tiers: []\n", `
essences:
  - { key: "ice cold", glyph: "❄" }
`)
	regs := NewRegistries()
	err := Load(context.Background(), root, nil, regs, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error on whitespace essence key")
	}
	if !errors.Is(err, decoration.ErrInvalidKey) {
		t.Errorf("err = %v, want decoration.ErrInvalidKey wrap", err)
	}
}

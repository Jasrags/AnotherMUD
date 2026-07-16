package command

import "testing"

// TestShopConfigFromProperties_RequiresLicense verifies the requires_license
// shop-block flag round-trips into ShopConfig (sin-and-legality.md §4). The YAML
// loader normalizes the block to map[string]any; a Go bool decodes straight
// through.
func TestShopConfigFromProperties_RequiresLicense(t *testing.T) {
	t.Run("present true", func(t *testing.T) {
		props := map[string]any{"shop": map[string]any{
			"sells":            []any{"shadowrun:ares-predator-v"},
			"requires_license": true,
		}}
		cfg, ok := ShopConfigFromProperties(props)
		if !ok {
			t.Fatal("ShopConfigFromProperties ok = false, want true")
		}
		if !cfg.RequiresLicense {
			t.Error("RequiresLicense = false, want true")
		}
	})

	t.Run("omitted defaults false", func(t *testing.T) {
		props := map[string]any{"shop": map[string]any{"sells": []any{"x"}}}
		cfg, _ := ShopConfigFromProperties(props)
		if cfg.RequiresLicense {
			t.Error("RequiresLicense = true, want false when omitted (shadow vendor)")
		}
	})
}

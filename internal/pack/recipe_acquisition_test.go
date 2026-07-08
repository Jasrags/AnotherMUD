package pack

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Jasrags/AnotherMUD/internal/loot"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/slot"
)

// TestLoad_CoreRecipeAcquisitionTiers loads the real core pack and asserts
// the Phase 6.3 content wires all three recipe-acquisition tiers
// (crafting-and-cooking §7): a common scroll in a shop, an uncommon quest
// reward, and a rare loot drop. It guards against content drift in the
// scroll→recipe links and the placements.
func TestLoad_CoreRecipeAcquisitionTiers(t *testing.T) {
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
	// Select the demo world explicitly (a boot loads ONE world; the content
	// dir holds starter-world + wot with colliding bare biome ids).
	if err := Load(context.Background(), root, []string{"starter-world"}, regs, nil, nil, nil); err != nil {
		t.Fatalf("Load core: %v", err)
	}

	// The three non-baseline recipes are registered under namespaced ids.
	for _, id := range []string{
		"starter-world:forge-iron-dagger",
		"starter-world:cook-trail-stew",
		"starter-world:reforge-greatsword",
	} {
		if _, err := regs.Recipes.Get(recipe.RecipeID(id)); err != nil {
			t.Errorf("recipe %s not registered: %v", id, err)
		}
	}

	// Common tier: the dagger scroll carries the qualified recipe id + a
	// smithing purchase gate, and the blacksmith sells it.
	scroll, err := regs.Items.Get("starter-world:scroll-forge-iron-dagger")
	if err != nil {
		t.Fatalf("dagger scroll not registered: %v", err)
	}
	if got := scroll.Properties["recipe"]; got != "starter-world:forge-iron-dagger" {
		t.Errorf("dagger scroll recipe = %v, want starter-world:forge-iron-dagger", got)
	}
	if got := scroll.Properties["requires_skill"]; got != "smithing" {
		t.Errorf("dagger scroll requires_skill = %v, want smithing", got)
	}
	smith, err := regs.Mobs.Get("starter-world:blacksmith")
	if err != nil {
		t.Fatalf("blacksmith not registered: %v", err)
	}
	if !shopSells(smith.Properties, "starter-world:scroll-forge-iron-dagger") {
		t.Error("blacksmith does not sell the common recipe scroll")
	}

	// Uncommon tier: the Gate Patrol quest rewards the trail-stew recipe
	// (qualified by the loader).
	q, ok := regs.Quests.Lookup("starter-world:gate-patrol")
	if !ok {
		t.Fatal("gate-patrol quest not registered")
	}
	if !contains(q.Reward.Recipes, "starter-world:cook-trail-stew") {
		t.Errorf("gate-patrol recipe reward = %v, want starter-world:cook-trail-stew", q.Reward.Recipes)
	}

	// Rare tier: the weathered scroll teaches the greatsword recipe and is
	// in the guard loot table's rare bonus.
	rare, err := regs.Items.Get("starter-world:scroll-reforge-greatsword")
	if err != nil {
		t.Fatalf("greatsword scroll not registered: %v", err)
	}
	if got := rare.Properties["recipe"]; got != "starter-world:reforge-greatsword" {
		t.Errorf("greatsword scroll recipe = %v, want starter-world:reforge-greatsword", got)
	}
	tbl, ok := regs.Loot.Get("starter-world:guard-loot")
	if !ok {
		t.Fatal("guard-loot table missing")
	}
	if !rareBonusContains(tbl, "starter-world:scroll-reforge-greatsword") {
		t.Error("guard-loot rare_bonus does not drop the rare recipe scroll")
	}
}

// rareBonusContains reports whether the loot table's rare-bonus pool can
// drop itemID — the actual link that makes the rare tier reachable.
func rareBonusContains(tbl *loot.Table, itemID string) bool {
	if tbl == nil || tbl.RareBonus == nil {
		return false
	}
	for _, e := range tbl.RareBonus.Entries {
		if e.ItemID == itemID {
			return true
		}
	}
	return false
}

func shopSells(props map[string]any, id string) bool {
	shop, ok := props["shop"].(map[string]any)
	if !ok {
		return false
	}
	return slices.Contains(stringList(shop["sells"]), id)
}

func stringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func contains(xs []string, want string) bool {
	return slices.Contains(xs, want)
}

package crafting

import "testing"

// findForm returns the FormRecipe for id, failing the test if it is absent.
func findForm(t *testing.T, rows []FormRecipe, id string) FormRecipe {
	t.Helper()
	for _, r := range rows {
		if string(r.ID) == id {
			return r
		}
	}
	t.Fatalf("recipe %q not in form (%d rows)", id, len(rows))
	return FormRecipe{}
}

func TestCraftForm_CraftableWhenAllGatesMet(t *testing.T) {
	// craftFixture: knows core:forge-sword (smithing floor 1, prof 50), carries
	// 1 iron bar, station tier 0 (anywhere). No station func → tier 0, which
	// meets the recipe's StationTier 0.
	s, crafter, _, _ := craftFixture(t, 1, -1)

	rows := s.CraftForm(crafter, nil)
	r := findForm(t, rows, "core:forge-sword")

	if !r.Craftable {
		t.Fatalf("recipe not craftable despite all gates met: %+v", r)
	}
	if !r.SkillMet || !r.StationMet || !r.HaveAll {
		t.Errorf("gate flags = skill:%v station:%v haveAll:%v, want all true", r.SkillMet, r.StationMet, r.HaveAll)
	}
	if r.Name != "forge a sword" || r.Discipline != "smithing" {
		t.Errorf("name/discipline = %q/%q, want forge a sword/smithing", r.Name, r.Discipline)
	}
	if len(r.Ingredients) != 1 {
		t.Fatalf("ingredients = %d, want 1", len(r.Ingredients))
	}
	ing := r.Ingredients[0]
	if ing.Need != 1 || ing.Have != 1 {
		t.Errorf("ingredient have/need = %d/%d, want 1/1", ing.Have, ing.Need)
	}
	if ing.Name != "an iron bar" {
		t.Errorf("ingredient name = %q, want the template Name 'an iron bar'", ing.Name)
	}
}

func TestCraftForm_MissingIngredientShowsShortfall(t *testing.T) {
	// Recipe needs 2 iron bars; the crafter carries only 1.
	s, crafter, _, _ := craftFixture(t, 2, -1)
	crafter.inv = crafter.inv[:1] // drop one input

	rows := s.CraftForm(crafter, nil)
	r := findForm(t, rows, "core:forge-sword")

	if r.Craftable || r.HaveAll {
		t.Fatalf("recipe reports craftable/haveAll despite a shortfall: %+v", r)
	}
	ing := r.Ingredients[0]
	if ing.Need != 2 || ing.Have != 1 {
		t.Errorf("ingredient have/need = %d/%d, want 1/2", ing.Have, ing.Need)
	}
	// Skill + station still met — only the ingredient gate fails.
	if !r.SkillMet || !r.StationMet {
		t.Errorf("skill/station should stay met: skill:%v station:%v", r.SkillMet, r.StationMet)
	}
}

func TestCraftForm_StationGateBelowRequirement(t *testing.T) {
	s, crafter, _, _ := craftFixture(t, 1, -1)
	// Raise the recipe's station requirement above the present tier (0).
	rec, err := s.recipes.Get("core:forge-sword")
	if err != nil {
		t.Fatal(err)
	}
	rec.StationTier = 2

	rows := s.CraftForm(crafter, nil) // nil station func → tier 0
	r := findForm(t, rows, "core:forge-sword")

	if r.StationMet {
		t.Error("StationMet true despite present tier 0 < required 2")
	}
	if r.Craftable {
		t.Error("recipe craftable despite unmet station")
	}
	// The present-station func is consulted per discipline.
	rows = s.CraftForm(crafter, func(discipline string) int {
		if discipline == "smithing" {
			return 2
		}
		return 0
	})
	r = findForm(t, rows, "core:forge-sword")
	if !r.StationMet || !r.Craftable {
		t.Errorf("recipe should be craftable at station tier 2: station:%v craftable:%v", r.StationMet, r.Craftable)
	}
}

func TestCraftForm_SkillGateBelowFloor(t *testing.T) {
	s, crafter, _, _ := craftFixture(t, 1, -1)
	s.prof.Forget("p1", "smithing") // prof 0 < floor 1

	rows := s.CraftForm(crafter, nil)
	r := findForm(t, rows, "core:forge-sword")

	if r.SkillMet {
		t.Error("SkillMet true despite proficiency below floor")
	}
	if r.Craftable {
		t.Error("recipe craftable despite unmet skill floor")
	}
	// Ingredient possession is orthogonal — it is still satisfied.
	if !r.HaveAll {
		t.Error("HaveAll should stay true; only the skill gate fails")
	}
}

func TestCraftForm_UnknownCrafterHasNoRows(t *testing.T) {
	s, _, _, _ := craftFixture(t, 1, -1)
	stranger := &fakeCrafter{id: "nobody", failRemoveAt: -1}
	if rows := s.CraftForm(stranger, nil); len(rows) != 0 {
		t.Errorf("a crafter who knows no recipes returned %d rows, want 0", len(rows))
	}
}

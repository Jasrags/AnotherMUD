package crafting

import (
	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// FormRecipe is one known recipe's read-only craft-form projection: everything a
// rich client (web-client-plan P3, Slice B) needs to render + validate a craft
// row without a round-trip. It is the same gate data Craft would compute, laid
// out for display instead of executed — nothing here mutates inventory.
type FormRecipe struct {
	ID          recipe.RecipeID
	Name        string
	Discipline  string
	Ingredients []FormIngredient
	StationReq  int  // the recipe's minimum station tier (0 = anywhere)
	StationMet  bool // present station (room ∪ tools) meets StationReq
	SkillMet    bool // the crafter's proficiency meets SkillFloor
	HaveAll     bool // every ingredient is present in the required quantity
	Craftable   bool // SkillMet && StationMet && HaveAll
}

// FormIngredient is one input line of a FormRecipe: the display name plus the
// needed and currently-held quantities (Have < Need marks a shortfall).
type FormIngredient struct {
	Name string
	Need int
	Have int
}

// CraftForm projects the crafter's KNOWN recipes into read-only form rows
// (web-client-plan P3, Slice B). For each recipe it mirrors Craft's gates —
// skill floor (§3), station tier (§4), and per-ingredient presence/min-quality
// (§5) — but computes have/need counts instead of consuming, so a client can
// render an interactive craft panel that greys out unmakeable recipes.
//
// station reports the present station tier for a discipline at the crafter's
// location (room ∪ carried tools, §4); a nil func means no station (tier 0),
// matching Craft. Rows are returned in the KnownManager's sorted-id order for a
// stable poll-and-diff shadow. A nil/unwired service returns nil.
func (s *Service) CraftForm(c Crafter, station StationTierFunc) []FormRecipe {
	if s == nil || s.recipes == nil || s.known == nil {
		return nil
	}
	eid := entityID(c)
	ids := s.known.Recipes(eid) // already sorted → deterministic order

	// Count the crafter's inventory once by (template, rarity-eligibility) as we
	// go: an instance may satisfy at most one ingredient line, matching Craft's
	// no-double-count rule (gatherInputs' `used` set). We recompute per recipe
	// (recipes are independent attempts), but never double-count within a recipe.
	inv := c.Inventory()

	out := make([]FormRecipe, 0, len(ids))
	for _, id := range ids {
		rec, err := s.recipes.Get(id)
		if err != nil || rec == nil {
			continue // §9: known id no longer in content — skip
		}
		out = append(out, s.formRecipe(c, rec, inv, evalStationTier(station, rec.Discipline)))
	}
	return out
}

// formRecipe builds one FormRecipe: resolves the proficiency vs skill floor, the
// present station vs the recipe's requirement, and the per-ingredient have/need
// counts, then folds those into the overall craftable flag.
func (s *Service) formRecipe(c Crafter, rec *recipe.Recipe, inv []entities.EntityID, present int) FormRecipe {
	eid := entityID(c)

	prof := 0
	if s.prof != nil {
		prof, _ = s.prof.Proficiency(eid, rec.Discipline)
	}
	skillMet := prof >= rec.SkillFloor
	stationMet := present >= rec.StationTier

	ings, haveAll := s.formIngredients(rec, inv)

	return FormRecipe{
		ID:          rec.ID,
		Name:        rec.DisplayName,
		Discipline:  rec.Discipline,
		Ingredients: ings,
		StationReq:  rec.StationTier,
		StationMet:  stationMet,
		SkillMet:    skillMet,
		HaveAll:     haveAll,
		Craftable:   skillMet && stationMet && haveAll,
	}
}

// formIngredients computes each input's have/need line without mutating. It
// mirrors gatherInputs' matching (template id + min-quality) and its
// no-double-count rule (an instance counts toward at most one line), but keeps
// counting past the needed quantity so the client sees the true shortfall.
// haveAll is true when every line's Have >= Need.
func (s *Service) formIngredients(rec *recipe.Recipe, inv []entities.EntityID) (out []FormIngredient, haveAll bool) {
	used := make(map[entities.EntityID]struct{})
	haveAll = true
	out = make([]FormIngredient, 0, len(rec.Inputs))
	for _, ing := range rec.Inputs {
		need := max(ing.Quantity, 1)
		have := 0
		for _, id := range inv {
			if _, taken := used[id]; taken {
				continue
			}
			inst := s.itemInstance(id)
			if inst == nil || string(inst.TemplateID()) != ing.Template {
				continue
			}
			if !s.meetsMinQuality(inst, ing.MinQuality) {
				continue
			}
			used[id] = struct{}{}
			have++
		}
		if have < need {
			haveAll = false
		}
		out = append(out, FormIngredient{Name: s.ingredientName(ing.Template), Need: need, Have: have})
	}
	return out, haveAll
}

// ingredientName resolves a nicer display name for an input template — the
// item template's Name when it resolves, else the id's local part (the
// fail-soft fallback loot tables/craft resolution use for unknown template ids).
func (s *Service) ingredientName(templateID string) string {
	if s.tpls != nil {
		if tpl, err := s.tpls.Get(item.TemplateID(templateID)); err == nil && tpl != nil && tpl.Name != "" {
			return tpl.Name
		}
	}
	return localPart(templateID)
}

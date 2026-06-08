package crafting

import (
	"context"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/entities"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
)

// propRarity is the reserved item-instance property the decoration system
// reads to render an item's tier (internal/command/decorate.go). A crafted
// output is stamped with its rolled tier under this key.
const propRarity = "rarity"

// Crafter is the inventory-bearing actor a craft operates on — the player.
// The command layer's Actor interface satisfies this structurally, so no
// adapter is needed. All inventory mutation flows through these methods.
type Crafter interface {
	PlayerID() string
	ID() string
	Inventory() []entities.EntityID
	AddToInventory(entities.EntityID)
	RemoveFromInventory(entities.EntityID) bool
}

// CraftOutcome classifies a craft attempt for the caller to render.
type CraftOutcome int

const (
	CraftOK CraftOutcome = iota
	CraftUnknownRecipe
	CraftNotSkilled
	CraftMissingIngredients
	CraftOutputUndefined
	CraftInterrupted // a race removed an input mid-craft (rolled back)
	CraftFailed      // output spawn failed (rolled back)
	CraftNotEnabled
)

// CraftResult is the structured outcome of Craft.
type CraftResult struct {
	Outcome    CraftOutcome
	Message    string
	RecipeID   recipe.RecipeID
	OutputName string
	QualityKey string // the rolled rarity tier key ("" = none)
	Gained     bool   // proficiency increased on this craft
}

// entityID resolves the crafter's progression/known-recipe key (PlayerID,
// falling back to the connection id), matching the command-layer convention.
func entityID(c Crafter) string {
	if id := c.PlayerID(); id != "" {
		return id
	}
	return c.ID()
}

// Craft resolves query to a known recipe, atomically consumes its inputs,
// produces the quality-rolled output, and rolls a use-based skill gain
// (crafting-and-cooking §3, §5). Inventory is mutated all-or-nothing: no
// input is destroyed until the output exists and is in the crafter's bag.
//
// Station tier is fixed at 0 in this MVP (stations land in Phases 4–5), so
// output is clamped to the Tier-0 ceiling and the recipe's station_tier
// attempt-gate is not yet enforced.
func (s *Service) Craft(ctx context.Context, c Crafter, query string) CraftResult {
	if s == nil || s.recipes == nil || s.known == nil || s.store == nil || s.tpls == nil {
		return CraftResult{Outcome: CraftNotEnabled, Message: "Crafting is not enabled in this build."}
	}
	eid := entityID(c)

	rec, ok := s.resolveKnownRecipe(eid, query)
	if !ok {
		return CraftResult{Outcome: CraftUnknownRecipe, Message: "You don't know how to craft that."}
	}

	// Skill floor (§3): the discipline proficiency must meet the recipe's
	// minimum. Kept low by design — the ceiling is the real lever.
	prof := 0
	if s.prof != nil {
		prof, _ = s.prof.Proficiency(eid, rec.Discipline)
	}
	if prof < rec.SkillFloor {
		return CraftResult{
			Outcome: CraftNotSkilled, RecipeID: rec.ID,
			Message: "You aren't skilled enough to attempt that yet.",
		}
	}

	// Gate: gather the exact input instances (no mutation yet).
	consume, ingKeys, missing := s.gatherInputs(c, rec)
	if missing != "" {
		return CraftResult{
			Outcome: CraftMissingIngredients, RecipeID: rec.ID,
			Message: "You don't have the ingredients for that (need " + missing + ").",
		}
	}

	// Gate: the output template must resolve.
	tpl, err := s.tpls.Get(item.TemplateID(rec.Output.Template))
	if err != nil || tpl == nil {
		return CraftResult{
			Outcome: CraftOutputUndefined, RecipeID: rec.ID,
			Message: "That recipe's output is missing from the world; tell an admin.",
		}
	}
	qty := rec.Output.Quantity
	if qty < 1 {
		qty = 1
	}

	// Roll quality BEFORE any mutation (so a panic-free roll happens with
	// inputs intact). Station tier 0 in the pre-stations MVP.
	qualityKey := s.rollQuality(QualityInputs{
		Skill:              prof,
		IngredientTierKeys: ingKeys,
		StationTier:        0,
	})

	// CONSUME — remove all inputs; roll back (re-add) on a partial failure
	// so a lost race never destroys items.
	removed := make([]entities.EntityID, 0, len(consume))
	for _, id := range consume {
		if !c.RemoveFromInventory(id) {
			for _, r := range removed {
				c.AddToInventory(r)
			}
			return CraftResult{
				Outcome: CraftInterrupted, RecipeID: rec.ID,
				Message: "Something slips from your grasp and the work is spoiled.",
			}
		}
		removed = append(removed, id)
	}

	// PRODUCE — spawn the output(s). On failure, untrack any partial
	// output and re-add the consumed inputs (still live, only removed from
	// the bag), then bail. No loss.
	produced := make([]*entities.ItemInstance, 0, qty)
	for i := 0; i < qty; i++ {
		inst, err := s.store.Spawn(tpl)
		if err != nil {
			for _, p := range produced {
				_ = s.store.Untrack(p.ID())
			}
			for _, r := range removed {
				c.AddToInventory(r)
			}
			return CraftResult{
				Outcome: CraftFailed, RecipeID: rec.ID,
				Message: "The craft fails and nothing comes of it.",
			}
		}
		if qualityKey != "" {
			inst.SetProperty(propRarity, qualityKey)
		}
		produced = append(produced, inst)
	}

	// COMMIT — file the output into the bag, then destroy the consumed
	// inputs. Past this point nothing can fail.
	for _, p := range produced {
		c.AddToInventory(p.ID())
	}
	for _, r := range removed {
		_ = s.store.Untrack(r)
	}

	// Skill rises through use (§3.5). nil stats → no gain-stat scaling yet
	// (the crafter's stat reader isn't threaded to this layer in the MVP).
	gained := false
	if s.prof != nil {
		gained = s.prof.RollUseGain(eid, rec.Discipline, true, lockedRoller{mu: &s.rollMu, r: s.roller}, nil)
	}

	out := produced[0].Name()
	msg := "You craft " + out + "."
	if qualityKey != "" {
		if tier, ok := s.rarity.Get(qualityKey); ok && tier.VisibleText() != "" {
			msg = "You craft " + out + " " + tier.VisibleText() + "."
		}
	}
	return CraftResult{
		Outcome: CraftOK, RecipeID: rec.ID, OutputName: out,
		QualityKey: qualityKey, Gained: gained, Message: msg,
	}
}

// gatherInputs finds the exact item instances to consume for rec, honoring
// per-ingredient quantity and optional min-quality (§5). Returns the
// instance ids to consume, their rarity keys (for the quality roll), and a
// non-empty "missing" description naming the first unsatisfiable input.
// No mutation; an instance is never double-counted across ingredients.
func (s *Service) gatherInputs(c Crafter, rec *recipe.Recipe) (consume []entities.EntityID, ingKeys []string, missing string) {
	used := make(map[entities.EntityID]struct{})
	for _, ing := range rec.Inputs {
		want := ing.Quantity
		if want < 1 {
			want = 1
		}
		found := 0
		for _, id := range c.Inventory() {
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
			consume = append(consume, id)
			ingKeys = append(ingKeys, s.rarityKeyOf(inst))
			found++
			if found == want {
				break
			}
		}
		if found < want {
			return nil, nil, ingredientLabel(ing)
		}
	}
	return consume, ingKeys, ""
}

// meetsMinQuality reports whether inst's rarity is at least the recipe
// ingredient's min-quality floor. An empty floor accepts anything; an
// unstamped item is treated as the lowest tier.
func (s *Service) meetsMinQuality(inst *entities.ItemInstance, minKey string) bool {
	minKey = strings.TrimSpace(minKey)
	if minKey == "" {
		return true
	}
	minPos := ladderPosition(s.rarity, minKey)
	if minPos < 0 {
		return true // unknown floor key — don't gate (fail-soft)
	}
	have := ladderPosition(s.rarity, s.rarityKeyOf(inst))
	if have < 0 {
		have = 0
	}
	return have >= minPos
}

func (s *Service) rarityKeyOf(inst *entities.ItemInstance) string {
	if v, ok := inst.Property(propRarity); ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

func (s *Service) itemInstance(id entities.EntityID) *entities.ItemInstance {
	e, ok := s.store.GetByID(id)
	if !ok {
		return nil
	}
	inst, ok := e.(*entities.ItemInstance)
	if !ok {
		return nil
	}
	return inst
}

// resolveKnownRecipe matches query against the crafter's KNOWN recipes
// (breadth gate §1.2): exact local-part / display name first, then a prefix
// / substring fallback. Returns the single best match.
func (s *Service) resolveKnownRecipe(eid, query string) (*recipe.Recipe, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, false
	}
	var prefix, substr *recipe.Recipe
	for _, id := range s.known.Recipes(eid) {
		rec, err := s.recipes.Get(id)
		if err != nil || rec == nil {
			continue // §9: known id no longer in content — skip
		}
		local := localPart(string(id))
		name := strings.ToLower(rec.DisplayName)
		if local == q || name == q {
			return rec, true // exact wins immediately
		}
		if prefix == nil && (strings.HasPrefix(local, q) || strings.HasPrefix(name, q)) {
			prefix = rec
		}
		if substr == nil && strings.Contains(name, q) {
			substr = rec
		}
	}
	if prefix != nil {
		return prefix, true
	}
	if substr != nil {
		return substr, true
	}
	return nil, false
}

// KnownRecipeNames returns the display names of the crafter's known
// recipes, sorted, for the no-arg `craft` listing.
func (s *Service) KnownRecipeNames(eid string) []string {
	if s.known == nil || s.recipes == nil {
		return nil
	}
	var out []string
	for _, id := range s.known.Recipes(eid) {
		if rec, err := s.recipes.Get(id); err == nil && rec != nil {
			out = append(out, rec.DisplayName)
		}
	}
	return out
}

func localPart(id string) string {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return strings.ToLower(id[i+1:])
	}
	return strings.ToLower(id)
}

func ingredientLabel(ing recipe.Ingredient) string {
	return localPart(ing.Template)
}

// lockedRoller serializes roller use for the gain roll (Craft runs on
// per-session goroutines). Mirrors the inline guard in rollQuality.
type lockedRoller struct {
	mu interface{ Lock(); Unlock() }
	r  Roller
}

func (l lockedRoller) IntN(n int) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.r.IntN(n)
}

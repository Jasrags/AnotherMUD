// Package recipe owns content-side crafting data: the recipe template
// type and the registry the pack loader populates at boot. A recipe turns
// input items into an output item, gated by a crafting discipline
// (a proficiency), a station tier, and the quality of its inputs.
//
// This package is a content leaf, mirroring internal/item: it holds the
// decoded shape of a recipe and the boot-time registry, and imports no
// engine runtime layer. The crafting resolution + quality roll (Phase 2)
// and the per-character known-recipe state live elsewhere.
//
// Spec: docs/specs/crafting-and-cooking.md §3 (recipes), §5 (the quality
// roll inputs a recipe declares), §7 (acquisition tier — metadata only).
package recipe

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// RecipeID is a namespace-qualified recipe identifier
// (e.g. "tapestry-core:campfire-stew"), mirroring item.TemplateID. A
// recipe is concrete pack content, so its id is namespaced and unique
// across packs (not a global override-able definition like an ability).
type RecipeID string

// PropRecipeID is the item-instance property a recipe scroll/page carries
// (crafting-and-cooking §7): a recipe id the `read` verb teaches and then
// consumes the scroll for. A recipe-bearing item is otherwise ordinary, so
// the common (shop) and rare (loot) tiers are pure content placement of the
// same item kind. The value is a (namespaced) recipe id string.
const PropRecipeID = "recipe"

// AcquisitionTier is the §7 metadata describing how a recipe is obtained
// (baseline with the skill, bought, quest reward, dropped, or
// region-locked). It is metadata for content/economy review — NEVER a
// runtime gate. The runtime gate is the player's known-recipe set
// (breadth) and the station/skill/ingredient ceiling (quality); §1.1.
type AcquisitionTier string

const (
	AcqBaseline AcquisitionTier = "baseline"
	AcqCommon   AcquisitionTier = "common"
	AcqUncommon AcquisitionTier = "uncommon"
	AcqRare     AcquisitionTier = "rare"
	AcqRegional AcquisitionTier = "regional"
)

// ParseAcquisitionTier resolves a content string to a tier. An empty
// string defaults to baseline (the recipes that arrive with a discipline,
// §2). An unrecognized value returns (AcqBaseline, false) so the decoder
// can reject it with file attribution.
func ParseAcquisitionTier(s string) (AcquisitionTier, bool) {
	switch AcquisitionTier(strings.ToLower(strings.TrimSpace(s))) {
	case "", AcqBaseline:
		return AcqBaseline, true
	case AcqCommon:
		return AcqCommon, true
	case AcqUncommon:
		return AcqUncommon, true
	case AcqRare:
		return AcqRare, true
	case AcqRegional:
		return AcqRegional, true
	default:
		return AcqBaseline, false
	}
}

// Ingredient is one input a recipe consumes (§3). Template is the
// namespace-qualified item template id consumed; Quantity is how many
// (≥1). MinQuality, when set, is a rarity-tier key floor on the ingredient
// (§5: "you cannot make a masterwork stew from rotten meat"); empty means
// any quality is accepted. The tier key references the decoration rarity
// ladder; it is not validated here (resolution reads it against the live
// rarity registry, the same fail-soft stance loot tables take with item
// ids).
//
// Inputs reference an item template id (a concrete kind) rather than a tag
// in this first cut; tag/keyword-class matching ("any meat") is a later
// refinement and would attach here as an alternative match field.
type Ingredient struct {
	Template   string
	Quantity   int
	MinQuality string
}

// Output is the item a successful craft produces (§3). Template is the
// namespace-qualified item template id; Quantity is how many (≥1). The
// crafted instance is stamped with its rolled rarity-tier property at
// resolution time (§5), so the output template carries the item's base
// identity and the quality is an instance property.
type Output struct {
	Template string
	Quantity int
}

// Recipe is the decoded, registry-owned shape of one crafting recipe
// (§3). Fields map directly to the spec's recipe declaration; numeric
// levers (skill floor, station tier, craft time) come from content per the
// §10 configuration surface.
type Recipe struct {
	// ID is the stable namespace-qualified id (§3); the per-character
	// known-recipe set keys on it.
	ID RecipeID
	// DisplayName is the player-facing recipe name.
	DisplayName string
	// Discipline is the crafting proficiency (ability id) this recipe
	// uses — smithing, cooking, … (§2). Bare id (abilities are not
	// namespaced); resolution reads the crafter's proficiency for it.
	Discipline string
	// SkillFloor is the minimum proficiency to attempt the recipe at all
	// (§3). Kept low by design — the ceiling, not the floor, is the main
	// lever (§1.1).
	SkillFloor int
	// StationTier is the minimum station tier required to attempt the
	// recipe (§4): 0 = anywhere, 1 = improvised (campfire), 2 = fixed
	// (forge/kitchen). A higher present station never refuses; it only
	// raises the achievable quality ceiling.
	StationTier int
	// Tool is the tool kind/tag the recipe uses, if any (§5). Empty means
	// no tool is required. Tool quality is a separate weight from skill in
	// the quality roll.
	Tool string
	// TimePulses is how long the craft occupies the player, in tick pulses
	// (§3). Tier 0 is fast; higher tiers slower.
	TimePulses int
	// Acquisition is the §7 acquisition-tier metadata (not a runtime gate).
	Acquisition AcquisitionTier
	// Inputs are the ingredients consumed atomically (§3).
	Inputs []Ingredient
	// Output is the item produced (§3).
	Output Output
	// Pack records the pack that registered this recipe — diagnostic only,
	// mirroring item/mob template provenance.
	Pack string
}

// Errors callers may distinguish at the boundary, mirroring item.
var (
	ErrRecipeNotFound = errors.New("recipe not found")
	ErrDuplicateID    = errors.New("duplicate recipe id")
)

// Registry is the boot-time registry of recipes. Safe for concurrent
// reads; mutations (Add, TryAdd) MUST happen at boot before serving —
// the same boot-immutable invariant as item.Templates and world.World.
type Registry struct {
	mu  sync.RWMutex
	all map[RecipeID]*Recipe
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{all: make(map[RecipeID]*Recipe)}
}

// Add registers r, replacing any existing recipe with the same id.
func (reg *Registry) Add(r *Recipe) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.all[r.ID] = r
}

// TryAdd registers r and returns ErrDuplicateID if a recipe with that id
// is already present. Used by the pack loader to catch cross-pack id
// collisions before they silently overwrite — mirrors item.Templates.
func (reg *Registry) TryAdd(r *Recipe) error {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if _, exists := reg.all[r.ID]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateID, r.ID)
	}
	reg.all[r.ID] = r
	return nil
}

// Get returns the recipe with id and ErrRecipeNotFound if absent.
func (reg *Registry) Get(id RecipeID) (*Recipe, error) {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	r, ok := reg.all[id]
	if !ok {
		return nil, fmt.Errorf("recipe.Registry.Get(%q): %w", id, ErrRecipeNotFound)
	}
	return r, nil
}

// Has reports whether id is registered. The §9 "a known-but-now-unknown
// recipe id is ignored, never an error" rule is enforced by callers
// filtering a player's known set through Has at restore time.
func (reg *Registry) Has(id RecipeID) bool {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	_, ok := reg.all[id]
	return ok
}

// Count returns the number of registered recipes.
func (reg *Registry) Count() int {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	return len(reg.all)
}

// All returns a snapshot of every registered recipe. Order is
// unspecified; callers that need determinism must sort.
func (reg *Registry) All() []*Recipe {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	out := make([]*Recipe, 0, len(reg.all))
	for _, r := range reg.all {
		out = append(out, r)
	}
	return out
}

// ByDiscipline returns every registered recipe whose Discipline matches
// (case-insensitive). Order is unspecified. Useful for "what can I make
// with this skill" listings.
func (reg *Registry) ByDiscipline(discipline string) []*Recipe {
	want := strings.ToLower(strings.TrimSpace(discipline))
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	var out []*Recipe
	for _, r := range reg.all {
		if strings.ToLower(r.Discipline) == want {
			out = append(out, r)
		}
	}
	return out
}

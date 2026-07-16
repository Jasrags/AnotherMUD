package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/crafting"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// flushGmcpRecipes snapshots the actor's craftable recipes into the rich
// Char.Recipes payload (web-client-plan P3, Slice B) and emits a frame when it
// differs from the last-sent shadow. Rides the same gmcp-items-flush tick pass
// as flushGmcpInventory (recipe craftability tracks ingredient possession, so it
// shares the inventory poll's inputs) with the same no-op guards: non-GMCP conn,
// GMCP inactive, or no crafting service wired.
//
// The station-met flag also depends on the actor's ROOM (a forge nearby), which
// the items pass does not otherwise watch — but because the payload is rebuilt
// and byte-diffed every tick, a room change that flips station-met still
// re-emits. The diff is a marshaled-bytes compare (like Char.Inventory), guarded
// by gmcpItemsMu (shared with its siblings on the same flush pass).
func (a *connActor) flushGmcpRecipes(ctx context.Context, svc *crafting.Service) {
	if svc == nil {
		return
	}
	sender, ok := a.conn.(gmcpSender)
	if !ok || !sender.GmcpActive() {
		return
	}
	// Unlike flushGmcpInventory (which needs a.items to resolve the bag and bails
	// when it's nil), the craft form resolves ingredient instances through the
	// crafting service's own store, and CraftStationTier/Inventory both degrade
	// gracefully with a nil a.items — so we build unconditionally. A nil store
	// just yields a less-complete form (station read from room props only), never
	// a crash, and a valid (possibly empty) frame still ships.

	payload := gmcp.CharRecipes{Recipes: a.buildCraftForm(svc)}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	a.gmcpItemsMu.Lock()
	unchanged := a.gmcpRecipesValid && string(a.gmcpRecipesLast) == string(data)
	if !unchanged {
		a.gmcpRecipesLast = data
		a.gmcpRecipesValid = true
	}
	a.gmcpItemsMu.Unlock()
	if unchanged {
		return
	}

	if err := sender.SendGmcp(ctx, gmcp.PackageCharRecipes, data); err != nil {
		logging.From(ctx).Debug("gmcp recipes send failed",
			slog.String("player", a.PlayerName()),
			slog.Any("err", err))
	}
}

// buildCraftForm projects the actor's known recipes into Char.Recipes rows via
// the crafting service's read-only CraftForm, supplying the same station-tier
// computation the `craft` verb uses (room ∪ carried tools) through the exported
// command.CraftStationTier so the two never drift. The result is always non-nil
// (make in the loop) so the wire carries `[]`, not `null`.
func (a *connActor) buildCraftForm(svc *crafting.Service) []gmcp.CraftRecipe {
	stationFn := func(discipline string) int {
		return command.CraftStationTier(a.Room(), a.items, a.placement, a.Inventory(), discipline)
	}
	rows := svc.CraftForm(a, stationFn)
	out := make([]gmcp.CraftRecipe, 0, len(rows))
	for _, r := range rows {
		out = append(out, craftRecipeRow(r))
	}
	return out
}

// craftRecipeRow converts one crafting.FormRecipe into the wire CraftRecipe:
// ingredient have/need lines, the station/skill/craftable gate flags, a short
// plain-text block reason when not craftable, and the full submit command.
func craftRecipeRow(r crafting.FormRecipe) gmcp.CraftRecipe {
	ings := make([]gmcp.RecipeIngredient, 0, len(r.Ingredients))
	for _, ing := range r.Ingredients {
		ings = append(ings, gmcp.RecipeIngredient{Name: ing.Name, Need: ing.Need, Have: ing.Have})
	}
	return gmcp.CraftRecipe{
		ID:          string(r.ID),
		Name:        r.Name,
		Discipline:  r.Discipline,
		Ingredients: ings,
		Station:     r.StationReq,
		StationMet:  r.StationMet,
		SkillMet:    r.SkillMet,
		Craftable:   r.Craftable,
		Blocked:     craftBlockedReason(r),
		Cmd:         "craft " + recipeLocalPart(string(r.ID)),
	}
}

// craftBlockedReason returns a short plain-text reason a recipe isn't craftable
// now, in the same precedence the `craft` verb refuses (skill floor, then
// station, then ingredients). "" when the recipe is craftable. Ruleset-agnostic
// wording — no setting vocabulary.
func craftBlockedReason(r crafting.FormRecipe) string {
	switch {
	case r.Craftable:
		return ""
	case !r.SkillMet:
		return "not skilled enough"
	case !r.StationMet:
		return "need a crafting station"
	default:
		return "missing ingredients"
	}
}

// recipeLocalPart returns the part of a namespaced recipe id after the ":" (the
// token a player types after `craft`), or the whole id when unqualified. Mirrors
// the crafting resolver's exact-match key.
func recipeLocalPart(id string) string {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			return id[i+1:]
		}
	}
	return id
}

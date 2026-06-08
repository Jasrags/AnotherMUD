package pack

import (
	"github.com/Jasrags/AnotherMUD/internal/decoration"
	"github.com/Jasrags/AnotherMUD/internal/effect"
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/loot"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/property"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/recipe"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/script"
	"github.com/Jasrags/AnotherMUD/internal/slot"
	"github.com/Jasrags/AnotherMUD/internal/weather"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Registries bundles the boot-time content registries the pack loader
// writes into. Adding a new registry (mobs, abilities, …) means
// adding a field here, not widening Load's signature.
//
// All fields MUST be non-nil. NewRegistries is the supported way to
// construct one.
type Registries struct {
	World     *world.World
	Items     *item.Templates
	Slots     *slot.Registry
	Mobs      *mob.Templates
	Tracks    *progression.TrackRegistry
	Races     *progression.RaceRegistry
	Classes   *progression.ClassRegistry
	Abilities *progression.AbilityRegistry
	// Theme is the M10 UI theme registry. Packs register semantic
	// tag → {fg,bg,html} entries; the composition root compiles it
	// once after Load and binds a render.ColorRenderer to it.
	Theme *render.ThemeRegistry
	// Help is the M10.5 help-topic service. Packs register topics from
	// their help/*.yaml files; the help command queries it.
	Help *help.Service
	// Quests is the M10.6 quest-definition registry. Packs register
	// quests from their quests/*.yaml files; the quest service (M10.7)
	// reads it.
	Quests *quest.Registry
	// Effects is the M14.2 effect-template registry. Packs register
	// effect templates from their effects/*.yaml files; the
	// item.consumed subscriber resolves an event's effect_id through
	// this registry before calling EffectManager.Apply.
	Effects *effect.Registry
	// Properties is the M14.4 property-key registry. Engine code
	// registers known property keys at boot (RegisterEngine); pack
	// init code may register pack-scoped keys (RegisterPack). The
	// room loader validates each room's Properties bag against this
	// registry — snake_case + known-name + type-match.
	Properties *property.Registry
	// Weather is the M15.4 weather-zone registry. Packs register
	// zones from their weather_zones/*.yaml files; the weather
	// Service resolves area weather_zone ids through this registry
	// at HourChanged time.
	Weather *weather.Registry
	// Scripts is the M17.1b script-source registry. Packs register
	// Lua source bodies from their `scripts:` manifest glob; the
	// composition root (M17.1c) reads from here to install handlers
	// at boot. Compile-checked at load time so syntax errors
	// surface with pack + path attribution before the engine
	// starts ticking.
	Scripts *script.Registry
	// Rarity / Essence are the M20 item-decoration registries. Packs
	// register tier ladders + essence glyphs from their `rarity:` /
	// `essence:` manifest globs; the composition root seeds their colors
	// into Theme (RegisterTheme, if-absent) before Compile. Item display
	// (M20.5) resolves an item's rarity/essence key through these.
	Rarity  *decoration.RarityRegistry
	Essence *decoration.EssenceRegistry
	// Loot is the M22.1 loot-table registry. Packs register tables from
	// their `loot_tables:` manifest glob; the spawn pipeline rolls a
	// mob's referenced table into its contents at spawn time
	// (mobs-ai-spawning §6.3).
	Loot *loot.Registry
	// Recipes is the crafting-recipe registry (crafting-and-cooking §3).
	// Packs register recipes from their `recipes:` manifest glob; the
	// crafting resolution path (Phase 2) reads it, and the per-character
	// known-recipe set keys on recipe ids.
	Recipes *recipe.Registry
}

// NewRegistries returns a Registries with every field initialized.
// Callers that want the engine-baseline slot set should call
// slot.RegisterEngineBaseline on the returned Slots registry before
// invoking Load so packs can supplement (and collide cleanly if they
// try to redefine baseline slots).
func NewRegistries() *Registries {
	return &Registries{
		World:      world.New(),
		Items:      item.NewTemplates(),
		Slots:      slot.NewRegistry(),
		Mobs:       mob.NewTemplates(),
		Tracks:     progression.NewTrackRegistry(),
		Races:      progression.NewRaceRegistry(),
		Classes:    progression.NewClassRegistry(),
		Abilities:  progression.NewAbilityRegistry(),
		Theme:      render.NewThemeRegistry(),
		Help:       help.NewService(),
		Quests:     quest.NewRegistry(),
		Effects:    effect.NewRegistry(),
		Properties: property.NewRegistry(),
		Weather:    weather.NewRegistry(),
		Scripts:    script.New(),
		Rarity:     decoration.NewRarityRegistry(),
		Essence:    decoration.NewEssenceRegistry(),
		Loot:       loot.NewRegistry(),
		Recipes:    recipe.NewRegistry(),
	}
}

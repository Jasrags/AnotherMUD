package pack

import (
	"github.com/Jasrags/AnotherMUD/internal/help"
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/progression"
	"github.com/Jasrags/AnotherMUD/internal/quest"
	"github.com/Jasrags/AnotherMUD/internal/render"
	"github.com/Jasrags/AnotherMUD/internal/slot"
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
}

// NewRegistries returns a Registries with every field initialized.
// Callers that want the engine-baseline slot set should call
// slot.RegisterEngineBaseline on the returned Slots registry before
// invoking Load so packs can supplement (and collide cleanly if they
// try to redefine baseline slots).
func NewRegistries() *Registries {
	return &Registries{
		World:     world.New(),
		Items:     item.NewTemplates(),
		Slots:     slot.NewRegistry(),
		Mobs:      mob.NewTemplates(),
		Tracks:    progression.NewTrackRegistry(),
		Races:     progression.NewRaceRegistry(),
		Classes:   progression.NewClassRegistry(),
		Abilities: progression.NewAbilityRegistry(),
		Theme:     render.NewThemeRegistry(),
		Help:      help.NewService(),
		Quests:    quest.NewRegistry(),
	}
}

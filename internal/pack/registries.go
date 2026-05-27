package pack

import (
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/mob"
	"github.com/Jasrags/AnotherMUD/internal/progression"
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
	World  *world.World
	Items  *item.Templates
	Slots  *slot.Registry
	Mobs   *mob.Templates
	Tracks *progression.TrackRegistry
}

// NewRegistries returns a Registries with every field initialized.
// Callers that want the engine-baseline slot set should call
// slot.RegisterEngineBaseline on the returned Slots registry before
// invoking Load so packs can supplement (and collide cleanly if they
// try to redefine baseline slots).
func NewRegistries() *Registries {
	return &Registries{
		World:  world.New(),
		Items:  item.NewTemplates(),
		Slots:  slot.NewRegistry(),
		Mobs:   mob.NewTemplates(),
		Tracks: progression.NewTrackRegistry(),
	}
}

package pack

import (
	"github.com/Jasrags/AnotherMUD/internal/item"
	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Registries bundles the boot-time content registries the pack loader
// writes into. Adding a new registry (slots, mobs, abilities, …) means
// adding a field here, not widening Load's signature.
//
// All fields MUST be non-nil. NewRegistries is the supported way to
// construct one.
type Registries struct {
	World *world.World
	Items *item.Templates
}

// NewRegistries returns a Registries with every field initialized.
func NewRegistries() *Registries {
	return &Registries{
		World: world.New(),
		Items: item.NewTemplates(),
	}
}

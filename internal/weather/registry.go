package weather

import (
	"errors"
	"fmt"
	"sync"
)

// ErrZoneNotFound is returned by Registry.Get when no zone with the
// requested id is registered.
var ErrZoneNotFound = errors.New("weather: zone not found")

// ErrDuplicateZone is returned by Registry.Add when a zone id is
// already registered (collisions are content errors, not
// last-write-wins).
var ErrDuplicateZone = errors.New("weather: duplicate zone id")

// Registry is the boot-time lookup table for weather zones.
//
// Construction shape:
//
//   - pack.Load builds the Registry (M15.4b) from zone YAML files.
//   - Areas reference zones by id (world.Area.WeatherZone).
//   - The Service consults the Registry once per HourChanged call
//     to resolve the area's zone.
//
// Today (M15.4a) the loader piece isn't wired; tests and any
// composition root that wants weather instantiates the registry
// in-process.
type Registry struct {
	mu    sync.RWMutex
	zones map[string]*Zone
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{zones: make(map[string]*Zone)}
}

// Add registers z under z.ID. Returns ErrDuplicateZone if the id
// is already present. Nil z or empty id is rejected with a
// validation error rather than ignored.
func (r *Registry) Add(z *Zone) error {
	if z == nil {
		return errors.New("weather.Registry.Add: nil zone")
	}
	if z.ID == "" {
		return errors.New("weather.Registry.Add: empty zone id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.zones[z.ID]; ok {
		return fmt.Errorf("%w: %q", ErrDuplicateZone, z.ID)
	}
	r.zones[z.ID] = z
	return nil
}

// Get returns the zone registered under id, or ErrZoneNotFound.
func (r *Registry) Get(id string) (*Zone, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	z, ok := r.zones[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrZoneNotFound, id)
	}
	return z, nil
}

// Len reports the number of registered zones. Useful for tests
// and boot logging.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.zones)
}

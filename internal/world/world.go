package world

import (
	"errors"
	"fmt"
	"sync"
)

// RoomID is an opaque content-defined room identifier (spec §2.1).
// Pack-loaded ids are namespaced: "tapestry-core:town-square".
type RoomID string

// AreaID is an opaque content-defined area identifier (spec
// world-rooms-movement §2.4). Pack-loaded ids are namespaced like rooms.
type AreaID string

// Area groups rooms for spawn-reset cadence, weather, and presentation
// (spec world-rooms-movement §2.4 + mobs-ai-spawning §3.5).
//
// SpawnRules + ResetInterval drive the M6.6 respawn pipeline. Empty
// SpawnRules means "no area-driven spawns" (legitimate: a quiet
// roleplay zone). ResetInterval == 0 means "use the engine default";
// each SpawnRule may also carry its own override.
type Area struct {
	ID            AreaID
	Name          string
	Description   string
	SpawnRules    []SpawnRule
	ResetInterval uint64 // ticks; 0 → engine default
}

// SpawnRule is one entry in an area's spawn config (spec
// mobs-ai-spawning §3.5). It declares which mob to populate, where,
// and to what target count. A single rule maps to a single
// (room, template, count) triple; multiple rules per area let one
// area carry a varied population.
//
// Rare is an optional alternate template; when set, each "missing
// slot" computed during a reset rolls independently against
// RareChance (0.0–1.0) to decide whether to spawn the rare instead
// of the default. Independent rolls per slot match spec §3.6
// step "Rare-swap rolls are independent per missing slot."
//
// ResetInterval, when non-zero, overrides the area's default cadence
// for this specific rule. Tags carry rule-level flags; today the
// only flag the engine inspects is `persistent` (§3.6: when at or
// above target, skip — i.e. the count is a ceiling).
type SpawnRule struct {
	RoomID        RoomID
	MobTemplateID string
	Count         int
	Rare          string
	RareChance    float64
	ResetInterval uint64 // ticks; 0 → use area's
	Tags          []string
}

// HasTag reports whether r carries the named tag. O(n) scan; rules
// typically carry ≤2 tags so this stays cheap.
func (r SpawnRule) HasTag(tag string) bool {
	for _, t := range r.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Exit is a directed edge from one room to a target room id.
// M1 scope: target only. Doors / display name / conditions land later.
type Exit struct {
	Target RoomID
}

// Room is a node in the world graph (spec §2.2).
//
// M1 fields: id, name, description, exit map. Entity placement,
// keyword exits, tags, properties, alignment, area, ambience flags all
// land in later milestones.
type Room struct {
	ID          RoomID
	AreaID      AreaID
	Name        string
	Description string
	Exits       map[Direction]Exit
	// HealingRate is the §5.7 additive room-level regen bonus
	// (economy-survival): inns / infirmaries / shrines set it so the
	// M11.5 vitals-regen heartbeat heals resting occupants faster. Zero
	// (the default) means no bonus. A typed field rather than a generic
	// property bag — rooms have no property map and this is the only
	// room-scoped numeric knob so far.
	HealingRate int
}

// Errors that callers may distinguish at the boundary.
var (
	ErrRoomNotFound = errors.New("room not found")
	ErrAreaNotFound = errors.New("area not found")
	ErrNoExit       = errors.New("no exit in that direction")
	ErrDuplicateID  = errors.New("duplicate id in world registry")
)

// World is the room + area registry. Safe for concurrent reads;
// mutations (AddRoom, AddArea) MUST happen at boot before serving.
type World struct {
	mu    sync.RWMutex
	rooms map[RoomID]*Room
	areas map[AreaID]*Area
}

// New returns an empty World.
func New() *World {
	return &World{
		rooms: make(map[RoomID]*Room),
		areas: make(map[AreaID]*Area),
	}
}

// AddArea registers a, replacing any existing entry with the same id.
// Use TryAddArea when collisions must be detected.
func (w *World) AddArea(a *Area) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.areas[a.ID] = a
}

// TryAddArea registers a and returns ErrDuplicateID if an area with
// that id is already registered. Used by the pack loader to catch
// cross-pack id collisions before they silently overwrite.
func (w *World) TryAddArea(a *Area) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.areas[a.ID]; exists {
		return fmt.Errorf("%w: area %q", ErrDuplicateID, a.ID)
	}
	w.areas[a.ID] = a
	return nil
}

// Area returns the area with id and ErrAreaNotFound if absent.
func (w *World) Area(id AreaID) (*Area, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	a, ok := w.areas[id]
	if !ok {
		return nil, fmt.Errorf("world.Area(%q): %w", id, ErrAreaNotFound)
	}
	return a, nil
}

// AddRoom inserts r into the registry. Adding a room with an existing
// id replaces the prior entry (spec §2.1). Use TryAddRoom when
// collisions must be detected.
func (w *World) AddRoom(r *Room) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if r.Exits == nil {
		r.Exits = make(map[Direction]Exit)
	}
	w.rooms[r.ID] = r
}

// TryAddRoom registers r and returns ErrDuplicateID if a room with
// that id is already registered.
func (w *World) TryAddRoom(r *Room) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.rooms[r.ID]; exists {
		return fmt.Errorf("%w: room %q", ErrDuplicateID, r.ID)
	}
	if r.Exits == nil {
		r.Exits = make(map[Direction]Exit)
	}
	w.rooms[r.ID] = r
	return nil
}

// Rooms returns a snapshot of every registered room. Order is
// unspecified; callers that need determinism must sort.
func (w *World) Rooms() []*Room {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]*Room, 0, len(w.rooms))
	for _, r := range w.rooms {
		out = append(out, r)
	}
	return out
}

// RoomsInArea returns a snapshot of every room whose AreaID matches.
// Order is unspecified. Used by the area-tick scheduler to count
// player presence per area (spec mobs-ai-spawning §3.7 occupied
// modifier).
func (w *World) RoomsInArea(id AreaID) []*Room {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]*Room, 0)
	for _, r := range w.rooms {
		if r.AreaID == id {
			out = append(out, r)
		}
	}
	return out
}

// Areas returns a snapshot of every registered area. Order is
// unspecified. Used by the area-tick scheduler to iterate per-area
// cadences at boot.
func (w *World) Areas() []*Area {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]*Area, 0, len(w.areas))
	for _, a := range w.areas {
		out = append(out, a)
	}
	return out
}

// Room returns the room with id and ErrRoomNotFound if absent.
func (w *World) Room(id RoomID) (*Room, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	r, ok := w.rooms[id]
	if !ok {
		return nil, fmt.Errorf("world.Room(%q): %w", id, ErrRoomNotFound)
	}
	return r, nil
}

// Move resolves the exit in dir from src and returns the target room.
// This is the M1 cut of the move primitive (spec §3.3): no entity
// list, no door check, no event emission. The caller (session layer)
// is responsible for tracking the player's current room and rendering.
//
// Errors:
//   - ErrRoomNotFound if src or the target room is unregistered.
//   - ErrNoExit if src has no exit in dir.
func (w *World) Move(srcID RoomID, dir Direction) (*Room, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	src, ok := w.rooms[srcID]
	if !ok {
		return nil, fmt.Errorf("world.Move from %q: %w", srcID, ErrRoomNotFound)
	}
	exit, ok := src.Exits[dir]
	if !ok {
		return nil, fmt.Errorf("world.Move %s from %q: %w", dir, srcID, ErrNoExit)
	}
	dst, ok := w.rooms[exit.Target]
	if !ok {
		return nil, fmt.Errorf("world.Move %s from %q to %q: %w", dir, srcID, exit.Target, ErrRoomNotFound)
	}
	return dst, nil
}

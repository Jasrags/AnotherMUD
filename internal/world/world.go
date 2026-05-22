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
// (spec world-rooms-movement §2.4). M2 fields cover only what the
// pack loader populates and the cross-pack validator needs.
type Area struct {
	ID          AreaID
	Name        string
	Description string
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

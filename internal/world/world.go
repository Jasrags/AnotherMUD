package world

import (
	"errors"
	"fmt"
	"sync"
)

// RoomID is an opaque content-defined room identifier (spec §2.1).
type RoomID string

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
	Name        string
	Description string
	Exits       map[Direction]Exit
}

// Errors that callers may distinguish at the boundary.
var (
	ErrRoomNotFound = errors.New("room not found")
	ErrNoExit       = errors.New("no exit in that direction")
)

// World is the room registry. Safe for concurrent reads; mutations
// (AddRoom) MUST happen at boot before serving.
type World struct {
	mu    sync.RWMutex
	rooms map[RoomID]*Room
}

// New returns an empty World.
func New() *World {
	return &World{rooms: make(map[RoomID]*Room)}
}

// AddRoom inserts r into the registry. Adding a room with an existing
// id replaces the prior entry (spec §2.1).
func (w *World) AddRoom(r *Room) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if r.Exits == nil {
		r.Exits = make(map[Direction]Exit)
	}
	w.rooms[r.ID] = r
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

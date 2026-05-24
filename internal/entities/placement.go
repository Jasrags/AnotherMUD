package entities

import (
	"sync"

	"github.com/Jasrags/AnotherMUD/internal/world"
)

// Placement tracks which room each item entity is currently in. It is
// the runtime bridge between world.Room (a static post-boot registry)
// and the entity store (live mutable instances). Per the M5 storage
// decision in ROADMAP, the lookup tables live here rather than on
// world.World so the world's boot-only-mutation invariant stays intact.
//
// The index is bidirectional:
//
//   - byRoom: RoomID → ordered slice of EntityIDs currently in that room.
//     Order is insertion order, which the keyword resolver relies on for
//     stable ordinal selection ("2.sword").
//   - byItem: EntityID → current RoomID. Used by drop / Untrack to know
//     where to evict an item from without scanning every room.
//
// All public methods are safe for concurrent use. Mutations and queries
// share a single RWMutex.
type Placement struct {
	mu     sync.RWMutex
	byRoom map[world.RoomID][]EntityID
	byItem map[EntityID]world.RoomID
}

// NewPlacement returns an empty Placement.
func NewPlacement() *Placement {
	return &Placement{
		byRoom: make(map[world.RoomID][]EntityID),
		byItem: make(map[EntityID]world.RoomID),
	}
}

// Place asserts that item is currently in room. If item was previously
// placed in a different room, it is first removed from the old room's
// list. Placing into the same room is a no-op (no duplicate entry).
func (p *Placement) Place(item EntityID, room world.RoomID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if old, ok := p.byItem[item]; ok {
		if old == room {
			return
		}
		p.removeFromRoomLocked(item, old)
	}
	p.byItem[item] = room
	p.byRoom[room] = append(p.byRoom[room], item)
}

// Remove clears any room placement for item. Returns true if a
// placement was removed, false if item wasn't placed anywhere.
func (p *Placement) Remove(item EntityID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	room, ok := p.byItem[item]
	if !ok {
		return false
	}
	p.removeFromRoomLocked(item, room)
	delete(p.byItem, item)
	return true
}

// RoomOf returns the room item is currently placed in.
func (p *Placement) RoomOf(item EntityID) (world.RoomID, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	r, ok := p.byItem[item]
	return r, ok
}

// InRoom returns a snapshot of the entity ids in room, in insertion
// order. The returned slice is freshly allocated and safe to mutate.
func (p *Placement) InRoom(room world.RoomID) []EntityID {
	p.mu.RLock()
	defer p.mu.RUnlock()
	src := p.byRoom[room]
	if len(src) == 0 {
		return nil
	}
	out := make([]EntityID, len(src))
	copy(out, src)
	return out
}

// removeFromRoomLocked deletes item from room's slice, preserving the
// order of remaining entries. Empty buckets are pruned so InRoom never
// hands back a zero-length slice for a room nothing is in. Caller MUST
// hold p.mu for writing.
func (p *Placement) removeFromRoomLocked(item EntityID, room world.RoomID) {
	bucket := p.byRoom[room]
	for i, id := range bucket {
		if id == item {
			p.byRoom[room] = append(bucket[:i], bucket[i+1:]...)
			break
		}
	}
	if len(p.byRoom[room]) == 0 {
		delete(p.byRoom, room)
	}
}

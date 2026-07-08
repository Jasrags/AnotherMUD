package entities

import "sync"

// Contents tracks which container entity an item is currently inside.
// It is the runtime substrate for spec inventory-equipment-items §4.5
// (put in container) and the read side of any "what's in this bag?"
// query a stacking or inventory renderer needs.
//
// Per the M5 storage decision in ROADMAP, Contents lives next to
// Placement rather than on world.World or on the container instance:
//
//   - world.World stays a boot-only registry (no per-item mutation).
//   - ItemInstance stays a plain entity record with no children
//     field, so a container created by Spawn() is identical in shape
//     to a non-container — the "is this a container?" decision is
//     a type check at the handler boundary.
//   - The {inventory, equipment, Placement, Contents} invariant
//     (see AddToInventory's doc comment in internal/session) extends
//     cleanly: an item id is in EXACTLY ONE of the four at any time.
//
// The index is bidirectional:
//
//   - byContainer: container EntityID → ordered slice of contained
//     item EntityIDs. Order is insertion order, matching Placement's
//     contract so the keyword resolver's ordinal selection ("2.gem")
//     behaves consistently whether the items are on the floor, in a
//     pouch, or in a chest.
//   - byItem: contained item EntityID → container EntityID. Lets
//     Take and accessibility checks resolve "where does this item
//     live?" without scanning every container.
//
// All public methods are safe for concurrent use. Contents owns a
// single RWMutex. The {…, Contents, …} invariant is what keeps the
// model consistent across the discrete steps a handler takes when
// moving items between rooms / inventory / containers — see
// command.PutHandler for the canonical remove-then-add ordering.
//
// Lock order (load-bearing — future code MUST preserve it):
//
//	actor.mu  →  contents.mu  →  entities.Store.mu
//
// The inventory tree builder in internal/session
// (buildSaveEntriesLocked) reads Contents while holding the actor
// mutex; that fixes the actor→contents direction. No path in the
// engine takes contents.mu and then attempts to acquire an actor
// mutex. If a future listener for ContainerItemAdded /
// ContainerItemAdded mutates an actor, it MUST do so after
// returning from the bus dispatch (which holds contents.mu only
// transitively through the publisher's call stack), or by
// scheduling work onto the actor's pump goroutine. Acquiring
// actor.mu under contents.mu would deadlock against a concurrent
// autosave Persist on that actor.
//
// Placement uses an independent RWMutex with the same one-way
// rule: actor.mu may be held when calling into Placement, but not
// the reverse.
type Contents struct {
	mu          sync.RWMutex
	byContainer map[EntityID][]EntityID
	byItem      map[EntityID]EntityID
}

// NewContents returns an empty Contents.
func NewContents() *Contents {
	return &Contents{
		byContainer: make(map[EntityID][]EntityID),
		byItem:      make(map[EntityID]EntityID),
	}
}

// Put records that item is now inside container. If item was already
// in another container, it is first removed from that container's
// list — Put is total, not additive. Putting into the same container
// is a no-op (no duplicate entry).
//
// Put does NOT validate that container is itself a container-typed
// entity, nor does it enforce capacity/weight — those are policy that
// belongs to the handler. Contents is the storage primitive only.
func (c *Contents) Put(container, item EntityID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.byItem[item]; ok {
		if old == container {
			return
		}
		c.removeFromContainerLocked(item, old)
	}
	c.byItem[item] = container
	c.byContainer[container] = append(c.byContainer[container], item)
}

// Take removes item from whichever container holds it. Returns true
// if item was in some container, false if it wasn't tracked.
func (c *Contents) Take(item EntityID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	container, ok := c.byItem[item]
	if !ok {
		return false
	}
	c.removeFromContainerLocked(item, container)
	delete(c.byItem, item)
	return true
}

// In returns a snapshot of the entity ids currently inside container,
// in insertion order. Freshly allocated, safe to mutate. Empty if the
// container is empty or unknown.
func (c *Contents) In(container EntityID) []EntityID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	src := c.byContainer[container]
	if len(src) == 0 {
		return nil
	}
	out := make([]EntityID, len(src))
	copy(out, src)
	return out
}

// ContainerOf returns the container currently holding item, if any.
func (c *Contents) ContainerOf(item EntityID) (EntityID, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	con, ok := c.byItem[item]
	return con, ok
}

// removeFromContainerLocked deletes item from container's slice
// preserving order. Empty buckets are pruned so In never returns a
// zero-length slice for a container nothing is in. Caller MUST hold
// c.mu for writing.
func (c *Contents) removeFromContainerLocked(item, container EntityID) {
	bucket := c.byContainer[container]
	for i, id := range bucket {
		if id == item {
			c.byContainer[container] = append(bucket[:i], bucket[i+1:]...)
			break
		}
	}
	if len(c.byContainer[container]) == 0 {
		delete(c.byContainer, container)
	}
}

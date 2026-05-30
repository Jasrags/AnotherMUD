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

// Exit is a directed edge from one room to a target room id. Doors
// land on the optional Door field (M15.1 — spec §5).
type Exit struct {
	Target RoomID
	// Door is the optional per-exit door state. nil means "no
	// door"; movement passes freely. A closed door blocks
	// movement; a locked door prevents the unlock-then-open
	// transition without a key. Reverse-side synchronization is
	// the World's responsibility (spec §5.2 step 4).
	Door *DoorState
}

// DoorState carries the per-exit door state (spec §5.1). Mutations
// flow through World.OpenDoor / CloseDoor / LockDoor / UnlockDoor
// so the paired reverse-side invariant is maintained centrally.
type DoorState struct {
	// Name is the door's display name. The space-split tokens
	// double as match keywords for command-layer resolution
	// ("open iron gate" matches a door named "iron gate"); see
	// ResolveDoorTarget.
	Name string
	// Keywords is the matchable-tokens slice. Derived from Name
	// at load time when not explicitly supplied so content
	// authors do not have to write both.
	Keywords []string
	// Closed reports the current closed/open state. Movement
	// through a closed door is blocked (spec §3.3 step 4).
	Closed bool
	// Locked reports the current lock state. A locked door is
	// always closed and cannot be opened without unlocking first.
	Locked bool
	// KeyID names the item template id required to unlock the
	// door. Empty means the door has no key — it can be
	// unlocked by anyone who can reach it.
	KeyID string
	// Pickable reports whether a lockpick-style verb can attempt
	// to bypass the lock without the key. v1 wires the flag but
	// the pick verb itself is deferred.
	Pickable bool
	// PickDifficulty is the per-door pick check threshold;
	// meaningful only when Pickable is true.
	PickDifficulty int
	// DefaultClosed / DefaultLocked are the values area reset
	// restores the door to (spec §5.4).
	DefaultClosed bool
	DefaultLocked bool
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

	// Tags are content-defined room flags consulted by the combat
	// safe-room engage refusal (combat §2.1, "safe-room"), the training
	// safe-room gate (progression §7.4, "safe"), and any future
	// room-scoped rule. Loaded from the room YAML `tags:` key. Mirrors
	// MobInstance.Tags. Empty for an untagged room.
	Tags []string

	// Properties is the free-form property bag (spec §2.2). Keys are
	// snake_case and validated against the engine-wide property
	// registry at load time (M14.4). Values are stored as raw
	// `any` and read via the typed Property* accessors so callers do
	// not type-assert in line. Empty by default; mutations are
	// content-load-only today (no runtime SetProperty path).
	Properties map[string]any
}

// Property returns the raw value stored under key. Returns
// (nil, false) when the key is absent. Use the typed helpers
// (PropertyString, PropertyInt, PropertyBool) when the registered
// type is known — they handle the type assertion in one place.
func (r *Room) Property(key string) (any, bool) {
	if r == nil || r.Properties == nil {
		return nil, false
	}
	v, ok := r.Properties[key]
	return v, ok
}

// PropertyString returns the string value under key. Returns
// ("", false) when the key is absent OR the stored value is not
// a string. The property registry's load-time validation prevents
// the "stored as int but read as string" failure mode from
// reaching production; this guard is defense-in-depth for tests
// that bypass the loader.
func (r *Room) PropertyString(key string) (string, bool) {
	v, ok := r.Property(key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// PropertyInt returns the int value under key. Mirrors
// PropertyString — returns (0, false) on absent or wrong-typed.
func (r *Room) PropertyInt(key string) (int, bool) {
	v, ok := r.Property(key)
	if !ok {
		return 0, false
	}
	n, ok := v.(int)
	return n, ok
}

// PropertyBool returns the bool value under key. Same shape as
// the other typed accessors.
func (r *Room) PropertyBool(key string) (bool, bool) {
	v, ok := r.Property(key)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// HasTag reports whether the room carries tag. O(n) scan; rooms carry a
// handful of tags so this stays cheap. Mirrors SpawnRule.HasTag.
func (r *Room) HasTag(tag string) bool {
	for _, t := range r.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Errors that callers may distinguish at the boundary.
var (
	ErrRoomNotFound = errors.New("room not found")
	ErrAreaNotFound = errors.New("area not found")
	ErrNoExit       = errors.New("no exit in that direction")
	ErrDuplicateID  = errors.New("duplicate id in world registry")
	// ErrDoorClosed is returned by Move when the exit has a door
	// and the door is currently closed (spec §3.3 step 4). The
	// command layer translates this into a "the door is closed"
	// user message and emits the door.blocked event.
	ErrDoorClosed = errors.New("the door is closed")
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
// The move primitive (spec §3.3) is otherwise pure: no entity list,
// no event emission. The caller (session layer) is responsible for
// tracking the player's current room and rendering.
//
// Errors:
//   - ErrRoomNotFound if src or the target room is unregistered.
//   - ErrNoExit if src has no exit in dir.
//   - ErrDoorClosed if the exit has a door that is currently closed
//     (M15.1 — spec §3.3 step 4). A locked door is also closed, so
//     callers that want to distinguish lock from close should query
//     GetDoor before attempting the move.
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
	if exit.Door != nil && exit.Door.Closed {
		return nil, fmt.Errorf("world.Move %s from %q: %w", dir, srcID, ErrDoorClosed)
	}
	dst, ok := w.rooms[exit.Target]
	if !ok {
		return nil, fmt.Errorf("world.Move %s from %q to %q: %w", dir, srcID, exit.Target, ErrRoomNotFound)
	}
	return dst, nil
}

// CanPass reports whether a move from srcID in dir is unblocked at
// this instant. True iff the exit exists AND (it has no door OR the
// door is not closed). Used by the command layer to surface a
// "you can't go that way" / "the door is closed" distinction without
// racing the actual Move (which also re-checks under the world
// lock).
//
// Spec: §5.3 first query.
func (w *World) CanPass(srcID RoomID, dir Direction) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	src, ok := w.rooms[srcID]
	if !ok {
		return false
	}
	exit, ok := src.Exits[dir]
	if !ok {
		return false
	}
	return exit.Door == nil || !exit.Door.Closed
}

// GetDoor returns a snapshot of the door state on the exit in dir
// from srcID, or (nil, false) if the exit or its door is absent.
// The returned value is a shallow copy — modifying it does not
// affect the world; mutations must go through Open/Close/Lock/Unlock.
//
// Spec: §5.3 second query.
func (w *World) GetDoor(srcID RoomID, dir Direction) (DoorState, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	src, ok := w.rooms[srcID]
	if !ok {
		return DoorState{}, false
	}
	exit, ok := src.Exits[dir]
	if !ok || exit.Door == nil {
		return DoorState{}, false
	}
	return *exit.Door, true
}

// OpenDoor opens the door on the exit in dir from srcID. Per spec
// §5.2:
//
//   - No-ops silently when the exit is absent, the exit has no door,
//     or the door is already open.
//   - Synchronizes the reverse-side door on the destination room's
//     opposite-direction exit, when one exists. A one-way door
//     (reverse exit absent or doorless) is allowed and not an error.
//   - Returns true when the operation actually transitioned a door
//     from closed to open (one side or both); false otherwise. The
//     caller uses the bool to decide whether to emit the
//     door.opened event.
func (w *World) OpenDoor(srcID RoomID, dir Direction) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.mutateDoorLocked(srcID, dir, doorOpen)
}

// CloseDoor closes the door on the exit in dir from srcID, with
// the same shape as OpenDoor.
func (w *World) CloseDoor(srcID RoomID, dir Direction) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.mutateDoorLocked(srcID, dir, doorClose)
}

// LockDoor locks the door on the exit in dir from srcID. Per spec
// §5.2, lock requires the door to be currently closed AND not
// already locked; otherwise the op is a silent no-op.
func (w *World) LockDoor(srcID RoomID, dir Direction) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.mutateDoorLocked(srcID, dir, doorLock)
}

// UnlockDoor unlocks the door on the exit in dir from srcID.
// Key-holder check is NOT enforced here per spec §5.2 ("key-holder
// check is a query exposed to the command layer; whether a command
// requires a key for a given operation is policy"). The command
// verb calls HasKey via the caller-supplied resolver first.
func (w *World) UnlockDoor(srcID RoomID, dir Direction) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.mutateDoorLocked(srcID, dir, doorUnlock)
}

// doorOp enumerates the four door mutations. Internal to keep the
// public Open/Close/Lock/Unlock surface stable while sharing the
// reverse-sync code path.
type doorOp int

const (
	doorOpen doorOp = iota
	doorClose
	doorLock
	doorUnlock
)

// mutateDoorLocked applies op to the door at (srcID, dir) and the
// paired reverse-side door (if any). Caller MUST hold w.mu write-
// locked. Returns true iff at least one side actually transitioned.
func (w *World) mutateDoorLocked(srcID RoomID, dir Direction, op doorOp) bool {
	src, ok := w.rooms[srcID]
	if !ok {
		return false
	}
	exit, ok := src.Exits[dir]
	if !ok || exit.Door == nil {
		return false
	}
	changed := applyDoorOp(exit.Door, op)

	// Reverse-side sync: look up the destination room's exit in
	// the opposite direction. If the reverse exit is doorless
	// (one-way door) or absent, that's allowed — spec §5.2
	// step 4 calls reverse-side absence "not an error".
	if dst, ok := w.rooms[exit.Target]; ok {
		if rev, ok := dst.Exits[dir.Opposite()]; ok && rev.Door != nil {
			if applyDoorOp(rev.Door, op) {
				changed = true
			}
		}
	}
	return changed
}

// applyDoorOp performs the precondition check + mutation for op on
// d. Returns true iff the door's state actually changed. Spec §5.2
// step 2: every op fails silently on its precondition.
func applyDoorOp(d *DoorState, op doorOp) bool {
	switch op {
	case doorOpen:
		if !d.Closed {
			return false
		}
		d.Closed = false
		return true
	case doorClose:
		if d.Closed {
			return false
		}
		d.Closed = true
		return true
	case doorLock:
		if !d.Closed || d.Locked {
			return false
		}
		d.Locked = true
		return true
	case doorUnlock:
		if !d.Locked {
			return false
		}
		d.Locked = false
		return true
	}
	return false
}

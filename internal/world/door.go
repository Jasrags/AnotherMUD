package world

import (
	"errors"
	"strconv"
	"strings"
)

// ErrDoorClosed is returned by Move when the exit has a door and
// the door is currently closed (spec §3.3 step 4). The command
// layer translates this into a "the door is closed" user message
// and emits the door.blocked event.
var ErrDoorClosed = errors.New("the door is closed")

// DoorState carries the per-exit door state (spec §5.1). Mutations
// flow through World.OpenDoor / CloseDoor / LockDoor / UnlockDoor
// so the paired reverse-side invariant is maintained centrally.
//
// Fields are exported because content loaders write them at boot
// (pack decode → buildDoorState in internal/pack) and renderers
// read them ("(closed)" / "(locked)" decorators in renderExits).
// The contract: callers MUST NOT write Closed / Locked / KeyID
// after the world has finished loading — only the four
// World.{Open,Close,Lock,Unlock}Door methods do that, and they
// synchronize the paired reverse-side door under the world lock.
// Writing these fields directly bypasses both the reverse-side
// sync and the world lock. Reads are safe (the room renderer
// reads Closed/Locked from the returned Exit value without going
// through GetDoor).
//
// This mirrors world.Room.Properties — also exported, also
// load-time-mutable-only by convention. If a future feature needs
// runtime DoorState mutation outside the four methods, add a
// proper API rather than reaching for the field directly.
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

// DoorTargetResolution is the outcome of resolving a command-layer
// text input ("open gate", "close 2.door", "open north") to a
// direction. Ok=false with Ambiguous=true means the keyword matched
// multiple doors and no ordinal was supplied — the command layer
// reports the ambiguity; otherwise an Ok=false result means
// "nothing matched".
//
// Spec: docs/specs/world-rooms-movement.md §5.5.
type DoorTargetResolution struct {
	Direction Direction
	Ok        bool
	Ambiguous bool
}

// ResolveDoorTarget translates a free-form text argument into a
// Direction. Resolution order per spec §5.5:
//
//  1. Direction parse — if the input parses to a Direction AND the
//     room has an exit there, that direction wins regardless of
//     whether the exit has a door (so `open north` keeps working
//     on a doorless north exit; the verb's own
//     "no-such-door" message handles that downstream).
//  2. Ordinal split — `<int>.<keyword>` separates the disambiguator
//     from the keyword; bare keyword means ordinal 0.
//  3. Candidate collection — every direction in the room whose exit
//     has a door whose Keywords contain the (lowercased) keyword.
//  4. Disambiguate — 0 candidates → Ok=false; ordinal 0 + multiple
//     candidates → Ambiguous; ordinal 0 + 1 candidate → that
//     direction; in-range ordinal → 1-indexed pick.
func (w *World) ResolveDoorTarget(srcID RoomID, arg string) DoorTargetResolution {
	w.mu.RLock()
	defer w.mu.RUnlock()
	src, ok := w.rooms[srcID]
	if !ok {
		return DoorTargetResolution{}
	}

	trimmed := strings.TrimSpace(arg)
	if trimmed == "" {
		return DoorTargetResolution{}
	}

	// 1. Direction parse.
	if d, ok := ParseDirection(trimmed); ok {
		if _, exists := src.Exits[d]; exists {
			return DoorTargetResolution{Direction: d, Ok: true}
		}
	}

	// 2. Ordinal split.
	ordinal := 0
	keyword := trimmed
	if idx := strings.IndexByte(trimmed, '.'); idx > 0 && idx < len(trimmed)-1 {
		head, tail := trimmed[:idx], trimmed[idx+1:]
		if n, err := strconv.Atoi(head); err == nil && n > 0 {
			ordinal = n
			keyword = tail
		}
	}
	keyword = strings.ToLower(keyword)

	// 3. Candidate collection.
	type candidate struct {
		dir Direction
	}
	candidates := make([]candidate, 0, 4)
	// Iterate in canonical direction order so ordinal picks are
	// deterministic across map iteration randomization.
	for _, d := range canonicalDirections {
		exit, ok := src.Exits[d]
		if !ok || exit.Door == nil {
			continue
		}
		if doorKeywordMatch(exit.Door, keyword) {
			candidates = append(candidates, candidate{dir: d})
		}
	}

	// 4. Disambiguate.
	switch {
	case len(candidates) == 0:
		return DoorTargetResolution{}
	case ordinal == 0 && len(candidates) > 1:
		return DoorTargetResolution{Ambiguous: true}
	case ordinal == 0:
		return DoorTargetResolution{Direction: candidates[0].dir, Ok: true}
	case ordinal > 0 && ordinal <= len(candidates):
		return DoorTargetResolution{Direction: candidates[ordinal-1].dir, Ok: true}
	default:
		return DoorTargetResolution{}
	}
}

// doorKeywordMatch reports whether keyword (already lowercased) is
// one of the door's keywords. Match is case-insensitive on stored
// keywords as well so content that writes mixed-case keywords
// still resolves under lowercase command input.
func doorKeywordMatch(d *DoorState, keyword string) bool {
	for _, k := range d.Keywords {
		if strings.EqualFold(k, keyword) {
			return true
		}
	}
	return false
}

// canonicalDirections is the iteration order ResolveDoorTarget
// uses to give deterministic ordinal picks (map iteration in Go
// is randomized).
var canonicalDirections = []Direction{
	DirNorth, DirEast, DirSouth, DirWest, DirUp, DirDown,
}

// ResetDoorsInArea restores every door in the area to its default
// (DefaultClosed, DefaultLocked) state. Per spec §5.4 every room
// whose id is prefixed by the area id (or equals it for a
// singleton-room area) is in scope. Reverse-side sync runs the
// same way as the verb-driven mutations.
//
// Subscribed to area.tick from the composition root so reset
// happens at the same cadence as mob respawn (spec §3.7).
//
// Returns the number of door SIDES that actually transitioned —
// a door whose Closed flag flipped on both sides counts twice.
// Used for slog observability.
func (w *World) ResetDoorsInArea(areaID AreaID) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	transitions := 0
	prefix := string(areaID) + ":"
	bare := string(areaID)
	for roomID, room := range w.rooms {
		idStr := string(roomID)
		if idStr != bare && !strings.HasPrefix(idStr, prefix) {
			continue
		}
		for dir, exit := range room.Exits {
			if exit.Door == nil {
				continue
			}
			// Closed flag — restore the default if it differs.
			if exit.Door.Closed != exit.Door.DefaultClosed {
				op := doorClose
				if !exit.Door.DefaultClosed {
					op = doorOpen
				}
				if applyDoorOp(exit.Door, op) {
					transitions++
					w.syncReverseLocked(roomID, dir, op, &transitions)
				}
			}
			// Locked flag — same shape.
			if exit.Door.Locked != exit.Door.DefaultLocked {
				op := doorLock
				if !exit.Door.DefaultLocked {
					op = doorUnlock
				}
				if applyDoorOp(exit.Door, op) {
					transitions++
					w.syncReverseLocked(roomID, dir, op, &transitions)
				}
			}
		}
	}
	return transitions
}

// syncReverseLocked propagates a door op to the paired reverse-side
// door without rerunning the near-side mutation. Caller MUST hold
// w.mu.Lock; counts a transition into *transitions if the reverse
// door actually flipped.
func (w *World) syncReverseLocked(srcID RoomID, dir Direction, op doorOp, transitions *int) {
	src, ok := w.rooms[srcID]
	if !ok {
		return
	}
	exit, ok := src.Exits[dir]
	if !ok {
		return
	}
	dst, ok := w.rooms[exit.Target]
	if !ok {
		return
	}
	rev, ok := dst.Exits[dir.Opposite()]
	if !ok || rev.Door == nil {
		return
	}
	if applyDoorOp(rev.Door, op) {
		*transitions++
	}
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


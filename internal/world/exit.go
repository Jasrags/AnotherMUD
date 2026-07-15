package world

// SetExit creates or replaces the directional exit from srcID toward targetID
// in direction dir, with an optional door. It is the runtime counterpart to
// content-authored exits, for systems that own a dynamic directional exit — the
// transit service builds each elevator landing's doorway (a directional exit
// carrying a lockable door it toggles as the car arrives and leaves). Returns
// false if srcID or targetID is not a registered room. Taken under the write
// lock, mirroring AddKeywordExit's contract.
//
// door may be nil for a doorless exit; a non-nil door is stored by reference, so
// the caller must not mutate it afterward outside the OpenDoor/CloseDoor/etc.
// world methods (which take the lock).
func (w *World) SetExit(srcID RoomID, dir Direction, targetID RoomID, door *DoorState) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	src, ok := w.rooms[srcID]
	if !ok {
		return false
	}
	if _, ok := w.rooms[targetID]; !ok {
		return false
	}
	if src.Exits == nil {
		src.Exits = make(map[Direction]Exit)
	}
	src.Exits[dir] = Exit{Target: targetID, Door: door}
	return true
}

package world

import (
	"fmt"
	"strings"
)

// MoveByKeyword resolves a keyword exit (e.g., "gate", "portal")
// from srcID and returns the target room. Keyword lookup is
// case-insensitive. Doors are NOT honored on keyword exits today
// (spec §5.6 portals are doorless by design); future doored keyword
// exits would add a Door check here.
//
// Errors:
//   - ErrRoomNotFound if src or target is unregistered.
//   - ErrNoExit if no keyword exit matches.
func (w *World) MoveByKeyword(srcID RoomID, keyword string) (*Room, error) {
	key := strings.ToLower(strings.TrimSpace(keyword))
	if key == "" {
		return nil, fmt.Errorf("world.MoveByKeyword from %q: %w", srcID, ErrNoExit)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	src, ok := w.rooms[srcID]
	if !ok {
		return nil, fmt.Errorf("world.MoveByKeyword from %q: %w", srcID, ErrRoomNotFound)
	}
	exit, ok := src.KeywordExits[key]
	if !ok {
		return nil, fmt.Errorf("world.MoveByKeyword %q from %q: %w", key, srcID, ErrNoExit)
	}
	dst, ok := w.rooms[exit.Target]
	if !ok {
		return nil, fmt.Errorf("world.MoveByKeyword %q from %q to %q: %w", key, srcID, exit.Target, ErrRoomNotFound)
	}
	return dst, nil
}

// AddKeywordExit registers a new keyword exit on srcID, mapping
// the lowercased keyword to a target. Refuses (returns false) if
// srcID is absent, the target room is absent, or the keyword is
// already taken on srcID (case-insensitive collision).
//
// Spec: §5.6 "create single-direction exit" precondition checks.
// The portal service builds on this primitive for TTL bookkeeping
// (recording the expiry tick on the side it's responsible for).
func (w *World) AddKeywordExit(srcID RoomID, keyword string, targetID RoomID) bool {
	key := strings.ToLower(strings.TrimSpace(keyword))
	if key == "" {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	src, ok := w.rooms[srcID]
	if !ok {
		return false
	}
	if _, ok := w.rooms[targetID]; !ok {
		return false
	}
	if _, taken := src.KeywordExits[key]; taken {
		return false
	}
	if src.KeywordExits == nil {
		src.KeywordExits = make(map[string]Exit)
	}
	src.KeywordExits[key] = Exit{Target: targetID}
	return true
}

// RemoveKeywordExit drops a keyword exit from srcID. Returns true
// when the exit was actually present and removed; false otherwise.
// nil-safe across missing room / missing keyword.
func (w *World) RemoveKeywordExit(srcID RoomID, keyword string) bool {
	key := strings.ToLower(strings.TrimSpace(keyword))
	if key == "" {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	src, ok := w.rooms[srcID]
	if !ok {
		return false
	}
	if _, present := src.KeywordExits[key]; !present {
		return false
	}
	delete(src.KeywordExits, key)
	return true
}

// HasKeywordExit reports whether srcID has a keyword exit registered
// under keyword (case-insensitive).
func (w *World) HasKeywordExit(srcID RoomID, keyword string) bool {
	key := strings.ToLower(strings.TrimSpace(keyword))
	if key == "" {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	src, ok := w.rooms[srcID]
	if !ok {
		return false
	}
	_, present := src.KeywordExits[key]
	return present
}

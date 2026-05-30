package chat

import (
	"fmt"
	"strings"
	"sync"
)

// DefaultBufferCap is the per-channel ring buffer size when a
// Channel does not override (spec §10).
const DefaultBufferCap = 50

// Registry holds every channel registered with the engine, indexed
// by ID and by lowercased display name. Safe for concurrent reads;
// Register is the only writer and is expected to be called at boot.
//
// Spec: docs/specs/chat-channels-and-tells.md §2.1, §3.1.
type Registry struct {
	mu          sync.RWMutex
	byID        map[string]*Channel
	byDispLower map[string]*Channel
	order       []string // registration order, for deterministic iteration
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byID:        make(map[string]*Channel),
		byDispLower: make(map[string]*Channel),
	}
}

// Register inserts c. Duplicate ID or duplicate (lowercased)
// DisplayName is an error so two packs cannot collide silently
// (§3.2). The caller owns the *Channel after registration; the
// registry never mutates it.
func (r *Registry) Register(c Channel) error {
	if c.ID == "" {
		return fmt.Errorf("chat.Register: empty ID")
	}
	if c.DisplayName == "" {
		return fmt.Errorf("chat.Register: empty DisplayName for %q", c.ID)
	}
	disp := strings.ToLower(c.DisplayName)

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byID[c.ID]; dup {
		return fmt.Errorf("chat.Register: duplicate id %q", c.ID)
	}
	if other, dup := r.byDispLower[disp]; dup {
		return fmt.Errorf("chat.Register: display-name collision %q (already used by %q)",
			c.DisplayName, other.ID)
	}
	copy := c
	r.byID[c.ID] = &copy
	r.byDispLower[disp] = &copy
	r.order = append(r.order, c.ID)
	return nil
}

// Get returns the Channel registered under id, or false if absent.
func (r *Registry) Get(id string) (*Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byID[id]
	return c, ok
}

// ByDisplayName returns the Channel whose DisplayName matches name
// case-insensitively. Used by verb dispatch.
func (r *Registry) ByDisplayName(name string) (*Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.byDispLower[strings.ToLower(name)]
	return c, ok
}

// All returns every registered channel in registration order. The
// returned slice is a fresh copy; the caller may mutate it.
func (r *Registry) All() []*Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Channel, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.byID[id])
	}
	return out
}

// Len returns the number of registered channels.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.order)
}

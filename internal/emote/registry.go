package emote

import (
	"fmt"
	"strings"
	"sync"
)

// Registry holds the loaded emote set. Indexed by ID and by every
// lowercased verb form (display name + aliases) so verb dispatch is
// case-insensitive and consistent.
//
// Spec: docs/specs/emotes.md §2.3.
type Registry struct {
	mu      sync.RWMutex
	byID    map[string]*Emote
	byVerb  map[string]*Emote // display-name + aliases, lowercased
	ordered []string          // ID order, for deterministic iteration
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byID:   make(map[string]*Emote),
		byVerb: make(map[string]*Emote),
	}
}

// Register validates and inserts e. Duplicate ID, duplicate verb,
// or template-validation failure each return an error and leave
// the registry unchanged.
//
// Spec: §2.3 — pack-collision check is the same shape as channels.
func (r *Registry) Register(e Emote) error {
	if err := e.Validate(); err != nil {
		return err
	}
	verbs := append([]string{strings.ToLower(e.DisplayName)}, lowerAll(e.Aliases)...)

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byID[e.ID]; dup {
		return fmt.Errorf("emote.Register: duplicate id %q", e.ID)
	}
	// Pre-check every verb so a partial registration cannot leak
	// when the second verb collides.
	for _, v := range verbs {
		if other, dup := r.byVerb[v]; dup {
			return fmt.Errorf("emote.Register: verb collision %q (already used by %q)", v, other.ID)
		}
	}
	copy := e
	r.byID[e.ID] = &copy
	for _, v := range verbs {
		r.byVerb[v] = &copy
	}
	r.ordered = append(r.ordered, e.ID)
	return nil
}

// Get returns the Emote registered under id.
func (r *Registry) Get(id string) (*Emote, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byID[id]
	return e, ok
}

// ByVerb returns the Emote associated with verb (case-insensitive,
// matches DisplayName or any alias).
func (r *Registry) ByVerb(verb string) (*Emote, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byVerb[strings.ToLower(verb)]
	return e, ok
}

// All returns every registered emote in insertion order. Fresh
// slice; callers may mutate it.
func (r *Registry) All() []*Emote {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Emote, 0, len(r.ordered))
	for _, id := range r.ordered {
		out = append(out, r.byID[id])
	}
	return out
}

// Len returns the count of registered emotes.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.ordered)
}

func lowerAll(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strings.ToLower(s)
	}
	return out
}

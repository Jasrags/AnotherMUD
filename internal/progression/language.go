package progression

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Language is a content-defined tongue a character can speak, read, and write
// (docs/specs/languages.md §2): a stable id, a display name, an optional
// comprehension Family, and flavor text. Authored per pack alongside
// races/classes/feats so the engine stays setting-agnostic.
//
// Mirrors the Background/Race registry shape: value-typed for storage, the
// registry hands callers a pointer to its own copy (callers MUST NOT mutate
// it), higher priority wins on an id collision.
type Language struct {
	// ID is the stable case-insensitive identity; lowercased on Register.
	ID string

	// Name is the display name shown by `score` + the `languages` listing.
	// Falls back to ID when empty.
	Name string

	// Family is the comprehension group: languages sharing a family are
	// mutually intelligible (the regional dialects of one common tongue declare
	// the same family). Authored for correctness; a future comprehension check
	// keys on the family, not the id. Empty = the language stands alone.
	Family string

	// Description is flavor text for the listing.
	Description string

	// Pack records which pack registered this language (diagnostic only).
	Pack string

	// Priority drives override semantics: higher wins on an id collision; equal
	// priority is a no-op (existing entry retained).
	Priority int
}

// LanguageRegistry holds language definitions keyed by case-insensitive id.
// Mirrors BackgroundRegistry.
type LanguageRegistry struct {
	mu        sync.RWMutex
	languages map[string]*Language
}

// NewLanguageRegistry returns an empty registry.
func NewLanguageRegistry() *LanguageRegistry {
	return &LanguageRegistry{languages: make(map[string]*Language)}
}

// Register installs l. Returns nil on success; an error if the definition is
// malformed (nil or empty id). Id and Family are lowercased on registration.
// Higher priority replaces; equal priority no-ops.
func (lr *LanguageRegistry) Register(l *Language) error {
	if l == nil {
		return fmt.Errorf("progression: nil Language")
	}
	id := strings.ToLower(strings.TrimSpace(l.ID))
	if id == "" {
		return fmt.Errorf("progression: language missing id")
	}
	lr.mu.Lock()
	defer lr.mu.Unlock()
	existing, ok := lr.languages[id]
	if ok && l.Priority <= existing.Priority {
		return nil
	}
	clone := *l
	clone.ID = id
	clone.Family = strings.ToLower(strings.TrimSpace(l.Family))
	lr.languages[id] = &clone
	return nil
}

// Get returns the registered Language for id. Case-insensitive; (nil, false)
// on miss. Returns the registry-owned pointer — callers MUST NOT mutate it.
func (lr *LanguageRegistry) Get(id string) (*Language, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	if key == "" {
		return nil, false
	}
	lr.mu.RLock()
	defer lr.mu.RUnlock()
	l, ok := lr.languages[key]
	return l, ok
}

// Has reports whether a language is registered under id.
func (lr *LanguageRegistry) Has(id string) bool {
	_, ok := lr.Get(id)
	return ok
}

// DisplayName resolves an id to its registered display name, falling back to
// the id itself when unregistered (so a stale known-language id renders
// readably rather than crashing a sheet — languages.md §4).
func (lr *LanguageRegistry) DisplayName(id string) string {
	if l, ok := lr.Get(id); ok && l.Name != "" {
		return l.Name
	}
	return strings.ToLower(strings.TrimSpace(id))
}

// All returns every registered language in id-sorted order. Used by listing
// surfaces; not on a hot path.
func (lr *LanguageRegistry) All() []*Language {
	lr.mu.RLock()
	defer lr.mu.RUnlock()
	ids := make([]string, 0, len(lr.languages))
	for id := range lr.languages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Language, 0, len(ids))
	for _, id := range ids {
		out = append(out, lr.languages[id])
	}
	return out
}

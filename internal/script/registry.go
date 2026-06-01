// Package script holds the boot-time registry of content-pack
// scripts discovered by the pack loader (M17.1b). Each entry
// carries the pack namespace, the script's relative path inside
// the pack, and the raw source body. The runtime (M17.1c) reads
// from this registry to install handlers at boot.
//
// The registry is one-direction: packs ADD at load time; the
// runtime READS at boot and never mutates after. There is no
// per-script remove API — a pack that wants to retract a script
// updates its manifest.
package script

import (
	"sort"
	"strings"
	"sync"
)

// Entry is one script discovered by the pack loader. PackID is the
// pack's namespace string (e.g. "tapestry-core"); Path is the
// script's relative path inside the pack ("scripts/track_kills.
// lua"); Source is the raw Lua text. All three fields are set at
// Register time and are read-only thereafter.
//
// LoadOrder is the manifest-declared pack load order (low number
// = earlier load). The runtime iterates entries in (LoadOrder,
// Path) tuple order so a deterministic execution sequence is
// reproducible across runs.
type Entry struct {
	PackID    string
	Path      string
	Source    string
	LoadOrder int
}

// Registry is the boot-time collection of discovered scripts.
// Safe for concurrent reads after load is complete; concurrent
// Register calls during load are serialized by the internal
// mutex. Iteration via All returns a stable, sorted snapshot.
type Registry struct {
	mu      sync.RWMutex
	entries []Entry
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{}
}

// Register appends an Entry. PackID + Path together identify a
// script; a duplicate (same PackID + Path) is rejected with
// ErrDuplicate so a misconfigured manifest globbing the same
// file twice surfaces at load time rather than running the
// script twice at boot.
func (r *Registry) Register(e Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.entries {
		if existing.PackID == e.PackID && existing.Path == e.Path {
			return &DuplicateError{PackID: e.PackID, Path: e.Path}
		}
	}
	r.entries = append(r.entries, e)
	return nil
}

// All returns a sorted snapshot of every entry. Order:
// LoadOrder ascending (manifest declaration order), then Path
// lexicographic. Deterministic so a script that relies on an
// earlier script's side effects can express that via load_order
// in its pack's manifest.
func (r *Registry) All() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, len(r.entries))
	copy(out, r.entries)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LoadOrder != out[j].LoadOrder {
			return out[i].LoadOrder < out[j].LoadOrder
		}
		return strings.Compare(out[i].Path, out[j].Path) < 0
	})
	return out
}

// Len returns the number of registered scripts. Used by the
// composition root to log a single-line "loaded N scripts" at
// boot.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// DuplicateError is returned by Register when the same
// (PackID, Path) pair is registered twice. Wraps the identifying
// fields so admin tooling can format a precise diagnostic.
type DuplicateError struct {
	PackID string
	Path   string
}

// Error implements the error interface.
func (e *DuplicateError) Error() string {
	return "script: duplicate registration pack=" + e.PackID + " path=" + e.Path
}
